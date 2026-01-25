package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea/v2"

	"github.com/dongho-jung/paw/internal/constants"
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
	RunE: func(_ *cobra.Command, args []string) error {
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
	RunE: func(_ *cobra.Command, args []string) error {
		logging.Debug("-> toggleCmdPaletteCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleCmdPaletteCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Run command palette in top pane
		paletteCmd := shellJoin(getPawBin(), "internal", "cmd-palette-tui", sessionName)

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
	RunE: func(_ *cobra.Command, args []string) error {
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
				Description: "Display current task content in a popup",
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
			showTaskCmd := exec.Command(pawBin, "internal", "show-current-task", sessionName) //nolint:gosec // G204: pawBin is from getPawBin()
			return showTaskCmd.Run()
		case "restore-panes":
			logging.Debug("cmdPaletteTUICmd: executing restore-panes")
			restoreCmd := exec.Command(pawBin, "internal", "restore-panes", sessionName) //nolint:gosec // G204: pawBin is from getPawBin()
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
	RunE: func(_ *cobra.Command, args []string) error {
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
		switch action { //nolint:exhaustive // FinishActionCancel handled by early return above
		case tui.FinishActionMergePush:
			endAction = constants.ActionMergePush
		case tui.FinishActionMerge:
			endAction = constants.ActionMerge
		case tui.FinishActionPR:
			endAction = constants.ActionPR
		case tui.FinishActionKeep:
			endAction = constants.ActionKeep
		case tui.FinishActionDone:
			endAction = constants.ActionDone
		case tui.FinishActionDrop:
			endAction = constants.ActionDrop
		default:
			logging.Debug("finishPickerTUICmd: unknown action=%s", action)
			return nil
		}

		// Call end-task-ui with the action flag
		logging.Debug("finishPickerTUICmd: calling end-task-ui with action=%s", endAction)
		endCmd := exec.Command(pawBin, "internal", "end-task-ui", sessionName, windowID, "--action", endAction) //nolint:gosec // G204: pawBin is from getPawBin()
		return endCmd.Run()
	},
}

var taskNameInputTUICmd = &cobra.Command{
	Use:    "task-name-input-tui [session]",
	Short:  "Run task name input TUI (called from popup)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "task-name-input-tui", "")
		defer cleanup()

		logging.Debug("-> taskNameInputTUICmd(session=%s)", sessionName)
		defer logging.Debug("<- taskNameInputTUICmd")

		// Run the task name input TUI
		action, taskName, err := tui.RunTaskNameInput()
		if err != nil {
			logging.Debug("taskNameInputTUICmd: RunTaskNameInput failed: %v", err)
			return err
		}

		logging.Debug("taskNameInputTUICmd: action=%d, taskName=%s", action, taskName)

		if action == tui.TaskNameInputCancel || taskName == "" {
			logging.Debug("taskNameInputTUICmd: cancelled or empty name")
			return nil
		}

		// Write the task name to a selection file for the caller to read
		selectionPath := filepath.Join(appCtx.PawDir, constants.TaskNameSelectionFile)
		if err := os.WriteFile(selectionPath, []byte(taskName), 0644); err != nil { //nolint:gosec // G306: selection file needs to be readable
			logging.Warn("taskNameInputTUICmd: failed to write selection file: %v", err)
			return err
		}

		logging.Debug("taskNameInputTUICmd: wrote task name to %s", selectionPath)
		return nil
	},
}
