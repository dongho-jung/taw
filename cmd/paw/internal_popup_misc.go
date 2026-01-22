package main

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea/v2"

	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var loadingScreenCmd = &cobra.Command{
	Use:    "loading-screen [message]",
	Short:  "Show a loading screen with braille animation",
	Args:   cobra.MaximumNArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> loadingScreenCmd")
		defer logging.Debug("<- loadingScreenCmd")

		message := "Generating task name..."
		if len(args) > 0 {
			message = args[0]
		}

		// Run the spinner TUI
		spinner := tui.NewSpinner(message)
		p := tea.NewProgram(spinner)

		// Block forever until killed (spawn-task will kill the window when done)
		_, err := p.Run()
		return err
	},
}

var toggleCmdPaletteCmd = &cobra.Command{
	Use:   "toggle-cmd-palette [session]",
	Short: "Toggle command palette top pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleCmdPaletteCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleCmdPaletteCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Run command palette in top pane
		paletteCmd := fmt.Sprintf("%s internal cmd-palette-tui %s", getPawBin(), sessionName)

		result, err := displayTopPane(tm, "palette", paletteCmd, "")
		if err != nil {
			logging.Debug("toggleCmdPaletteCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("toggleCmdPaletteCmd: blocked by another top pane")
		}
		return nil
	},
}

var cmdPaletteTUICmd = &cobra.Command{
	Use:    "cmd-palette-tui [session]",
	Short:  "Run command palette TUI (called from popup)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "cmd-palette-tui", "")
		defer cleanup()

		logging.Debug("-> cmdPaletteTUICmd(session=%s)", sessionName)
		defer logging.Debug("<- cmdPaletteTUICmd")

		// Define available commands
		commands := []tui.Command{
			{
				Name:        "Show Current Task",
				Description: "Display current task content in shell pane",
				ID:          "show-current-task",
			},
			{
				Name:        "Restore Panes",
				Description: "Restore missing panes in current task window",
				ID:          "restore-panes",
			},
		}

		logging.Debug("cmdPaletteTUICmd: running command palette")
		action, selected, err := tui.RunCommandPalette(commands)
		if err != nil {
			logging.Debug("cmdPaletteTUICmd: RunCommandPalette failed: %v", err)
			return err
		}

		if action == tui.CommandPaletteCancel || selected == nil {
			logging.Debug("cmdPaletteTUICmd: cancelled or no selection")
			return nil
		}

		logging.Debug("cmdPaletteTUICmd: selected command=%s", selected.ID)

		pawBin := getPawBin()
		switch selected.ID {
		case "show-current-task":
			logging.Debug("cmdPaletteTUICmd: executing show-current-task")
			showTaskCmd := exec.Command(pawBin, "internal", "show-current-task", sessionName)
			return showTaskCmd.Run()
		case "restore-panes":
			logging.Debug("cmdPaletteTUICmd: executing restore-panes")
			restoreCmd := exec.Command(pawBin, "internal", "restore-panes", sessionName)
			return restoreCmd.Run()
		}

		return nil
	},
}

var finishPickerTUICmd = &cobra.Command{
	Use:    "finish-picker-tui [session] [window-id]",
	Short:  "Run finish picker TUI (called from popup)",
	Args:   cobra.ExactArgs(2),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "finish-picker-tui", "")
		defer cleanup()

		logging.Debug("-> finishPickerTUICmd(session=%s, windowID=%s)", sessionName, windowID)
		defer logging.Debug("<- finishPickerTUICmd")

		// Detect if there are commits to merge (only for git repos)
		hasCommits := false
		if appCtx.IsGitRepo {
			// Find task by window ID to get the branch name
			mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
			targetTask, err := mgr.FindTaskByWindowID(windowID)
			if err == nil {
				gitClient := git.New()
				mainBranch := gitClient.GetMainBranch(appCtx.ProjectDir)
				workDir := mgr.GetWorkingDirectory(targetTask)
				commits, _ := gitClient.GetBranchCommits(workDir, targetTask.Name, mainBranch, 1)
				hasCommits = len(commits) > 0
				logging.Debug("finishPickerTUICmd: hasCommits=%v (branch=%s, main=%s)", hasCommits, targetTask.Name, mainBranch)
			}
		}

		// Run the finish picker
		action, err := tui.RunFinishPicker(appCtx.IsGitRepo, hasCommits)
		if err != nil {
			logging.Debug("finishPickerTUICmd: RunFinishPicker failed: %v", err)
			return err
		}

		logging.Debug("finishPickerTUICmd: selected action=%s", action)

		if action == tui.FinishActionCancel {
			logging.Debug("finishPickerTUICmd: cancelled by user")
			return nil
		}

		// Execute the selected action
		pawBin := getPawBin()

		// Map TUI action to end-task action flag
		var endAction string
		switch action {
		case tui.FinishActionMergePush:
			endAction = "merge-push"
		case tui.FinishActionMerge:
			endAction = "merge"
		case tui.FinishActionPR:
			endAction = "pr"
		case tui.FinishActionKeep:
			endAction = "keep"
		case tui.FinishActionDone:
			endAction = "done"
		case tui.FinishActionDrop:
			endAction = "drop"
		default:
			logging.Debug("finishPickerTUICmd: unknown action=%s", action)
			return nil
		}

		// Call end-task-ui with the action flag
		logging.Debug("finishPickerTUICmd: calling end-task-ui with action=%s", endAction)
		endCmd := exec.Command(pawBin, "internal", "end-task-ui", sessionName, windowID, "--action", endAction)
		return endCmd.Run()
	},
}
