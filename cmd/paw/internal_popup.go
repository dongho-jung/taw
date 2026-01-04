package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea/v2"

	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var popupShellCmd = &cobra.Command{
	Use:   "popup-shell [session]",
	Short: "Toggle shell pane at bottom 40%",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Check if shell pane exists - if so, close it (toggle off)
		paneID, _ := tm.GetOption("@paw_shell_pane_id")
		if paneID != "" && tm.HasPane(paneID) {
			_ = tm.KillPane(paneID)
			_ = tm.SetOption("@paw_shell_pane_id", "", true)
			return nil
		}

		// Get current pane's working directory
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			app, err := getAppFromSession(sessionName)
			if err != nil {
				return err
			}
			panePath = app.ProjectDir
		}
		panePath = strings.TrimSpace(panePath)

		// Create shell pane at bottom 40%
		newPaneID, err := tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false, // vertical split (top/bottom)
			Size:       "40%",
			StartDir:   panePath,
			Full:       true, // span entire window width
		})
		if err != nil {
			return fmt.Errorf("failed to create shell pane: %w", err)
		}

		// Store pane ID for toggle
		_ = tm.SetOption("@paw_shell_pane_id", strings.TrimSpace(newPaneID), true)

		return nil
	},
}

var toggleLogCmd = &cobra.Command{
	Use:   "toggle-log [session]",
	Short: "Toggle log viewer popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		logPath := app.GetLogPath()

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run log viewer in popup (closes with q/Esc/Ctrl+L)
		logCmd := fmt.Sprintf("%s internal log-viewer %s", pawBin, logPath)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:  "90%",
			Height: "80%",
			Title:  " Log Viewer ",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, logCmd)
		return nil
	},
}

var logViewerCmd = &cobra.Command{
	Use:    "log-viewer [logfile]",
	Short:  "Run the log viewer",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logFile := args[0]
		return tui.RunLogViewer(logFile)
	},
}

var toggleHelpCmd = &cobra.Command{
	Use:   "toggle-help [session]",
	Short: "Show help popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Get help content from embedded assets
		helpContent, err := embed.GetHelp()
		if err != nil {
			return fmt.Errorf("failed to get help content: %w", err)
		}

		// Write to temp file
		tmpFile, err := os.CreateTemp("", "paw-help-*.md")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		if _, err := tmpFile.WriteString(helpContent); err != nil {
			_ = tmpFile.Close()
			return fmt.Errorf("failed to write help content: %w", err)
		}
		_ = tmpFile.Close()

		// Build command (closes with q/Esc, temp file cleaned up on exit)
		popupCmd := fmt.Sprintf("less '%s'; rm -f '%s' 2>/dev/null || true", tmpPath, tmpPath)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:  "80%",
			Height: "80%",
			Title:  " Help ",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, popupCmd)
		return nil
	},
}

var toggleGitStatusCmd = &cobra.Command{
	Use:   "toggle-git-status [session]",
	Short: "Show git status popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Check if this is a git repo
		if !app.IsGitRepo {
			_ = tm.DisplayMessage("Not a git repository", 2000)
			return nil
		}

		// Get current pane's working directory (for worktree context)
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			panePath = app.ProjectDir
		}
		panePath = strings.TrimSpace(panePath)

		// Build command to show git status with color
		// Uses less -R to preserve colors and allow scrolling, closes with q
		popupCmd := fmt.Sprintf("cd '%s' && git -c color.status=always status | less -R", panePath)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     "80%",
			Height:    "80%",
			Title:     " Git Status ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: panePath,
		}, popupCmd)
		return nil
	},
}

var loadingScreenCmd = &cobra.Command{
	Use:    "loading-screen [message]",
	Short:  "Show a loading screen with braille animation",
	Args:   cobra.MaximumNArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
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

var toggleTaskListCmd = &cobra.Command{
	Use:   "toggle-task-list [session]",
	Short: "Toggle task list popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run task list viewer in popup (closes with q/Esc/Ctrl+T)
		listCmd := fmt.Sprintf("%s internal task-list-viewer %s", pawBin, sessionName)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     "90%",
			Height:    "80%",
			Title:     " Tasks ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: app.ProjectDir,
		}, listCmd)
		return nil
	},
}

var toggleSetupCmd = &cobra.Command{
	Use:   "toggle-setup [session]",
	Short: "Toggle setup wizard popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run setup wizard in popup (closes when done)
		// After setup completes, reload-config is called to apply changes
		setupCmd := fmt.Sprintf("%s internal setup-wizard %s", pawBin, sessionName)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     "60%",
			Height:    "50%",
			Title:     " Setup ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: app.ProjectDir,
		}, setupCmd)
		return nil
	},
}

var setupWizardCmd = &cobra.Command{
	Use:    "setup-wizard [session]",
	Short:  "Run the setup wizard (internal)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Run the setup wizard
		if err := runSetupWizard(app); err != nil {
			return err
		}

		// Reload config and re-apply tmux settings
		if err := app.LoadConfig(); err != nil {
			return fmt.Errorf("failed to reload config: %w", err)
		}

		// Re-apply tmux configuration
		tm := tmux.New(sessionName)
		if err := reapplyTmuxConfig(app, tm); err != nil {
			logging.Warn("Failed to re-apply tmux config: %v", err)
		}

		fmt.Println("\n✅ Settings applied!")
		fmt.Println("Press Enter to close...")
		_, _ = fmt.Scanln()

		return nil
	},
}

var taskListViewerCmd = &cobra.Command{
	Use:    "task-list-viewer [session]",
	Short:  "Run the task list viewer",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("task-list-viewer")
			logging.SetGlobal(logger)
		}

		logging.Trace("taskListViewerCmd: start session=%s", sessionName)
		defer logging.Trace("taskListViewerCmd: end")

		action, item, err := tui.RunTaskListUI(
			app.AgentsDir,
			app.GetHistoryDir(),
			app.ProjectDir,
			sessionName,
			app.PawDir,
			app.IsGitRepo,
		)
		if err != nil {
			return err
		}

		if item == nil {
			return nil
		}

		tm := tmux.New(sessionName)
		gitClient := git.New()
		pawBin, _ := os.Executable()

		logging.Trace("taskListViewerCmd: action=%v item=%s", action, item.Name)

		switch action {
		case tui.TaskListActionSelect:
			// Focus the task window
			logging.Trace("taskListViewerCmd: selecting window task=%s windowID=%s", item.Name, item.WindowID)
			if item.WindowID != "" {
				return tm.SelectWindow(item.WindowID)
			}

		case tui.TaskListActionCancel:
			// Trigger cancel-task-ui for cancellation with revert if needed
			logging.Trace("taskListViewerCmd: cancelling task=%s windowID=%s", item.Name, item.WindowID)
			if item.WindowID != "" {
				cancelCmd := exec.Command(pawBin, "internal", "cancel-task-ui", sessionName, item.WindowID)
				return cancelCmd.Start()
			}

		case tui.TaskListActionMerge:
			// Trigger end-task for merge
			logging.Trace("taskListViewerCmd: merging task=%s windowID=%s", item.Name, item.WindowID)
			if item.WindowID != "" {
				endCmd := exec.Command(pawBin, "internal", "end-task", sessionName, item.WindowID)
				return endCmd.Start()
			}

		case tui.TaskListActionPush:
			// Push the branch
			logging.Trace("taskListViewerCmd: pushing task=%s", item.Name)
			if item.AgentDir != "" {
				worktreeDir := filepath.Join(item.AgentDir, "worktree")
				if _, err := os.Stat(worktreeDir); err == nil {
					// Commit any changes first
					if gitClient.HasChanges(worktreeDir) {
						_ = gitClient.AddAll(worktreeDir)
						_ = gitClient.Commit(worktreeDir, "chore: auto-commit before push")
					}
					return gitClient.Push(worktreeDir, "origin", item.Name, true)
				}
			}

		case tui.TaskListActionResume:
			// Resume a completed task from history
			logging.Trace("taskListViewerCmd: resuming task=%s", item.Name)
			if item.HistoryFile != "" && item.Content != "" {
				// Create a new task with the same content
				mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
				newTask, err := mgr.CreateTask(item.Content)
				if err != nil {
					return fmt.Errorf("failed to create task: %w", err)
				}

				// Handle the task
				handleCmd := exec.Command(pawBin, "internal", "handle-task", sessionName, newTask.AgentDir)
				return handleCmd.Start()
			}
		}

		return nil
	},
}
