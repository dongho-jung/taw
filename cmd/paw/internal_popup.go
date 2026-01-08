package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea/v2"

	"github.com/dongho-jung/paw/internal/claude"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
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
	Short: "Toggle help popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run help viewer in popup (closes with q/Esc/Ctrl+/)
		helpCmd := fmt.Sprintf("%s internal help-viewer", pawBin)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:  "80%",
			Height: "80%",
			Title:  " Help ",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, helpCmd)
		return nil
	},
}

var helpViewerCmd = &cobra.Command{
	Use:    "help-viewer",
	Short:  "Run the help viewer",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get help content from embedded assets
		helpContent, err := embed.GetHelp()
		if err != nil {
			return fmt.Errorf("failed to get help content: %w", err)
		}

		return tui.RunHelpViewer(helpContent)
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
				endCmd := exec.Command(pawBin, "internal", "end-task", "--user-initiated", sessionName, item.WindowID)
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
						_ = gitClient.Commit(worktreeDir, constants.CommitMessageAutoCommitPush)
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

var toggleCmdPaletteCmd = &cobra.Command{
	Use:   "toggle-cmd-palette [session]",
	Short: "Toggle command palette popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run command palette in popup
		paletteCmd := fmt.Sprintf("%s internal cmd-palette-tui %s", pawBin, sessionName)

		return tm.DisplayPopup(tmux.PopupOpts{
			Width:  "60",
			Height: "20",
			Title:  "",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, paletteCmd)
	},
}

var cmdPaletteTUICmd = &cobra.Command{
	Use:    "cmd-palette-tui [session]",
	Short:  "Run command palette TUI (called from popup)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		// Define available commands
		commands := []tui.Command{
			{
				Name:        "Restore Panes",
				Description: "Restore missing panes in current task window",
				ID:          "restore-panes",
			},
		}

		action, selected, err := tui.RunCommandPalette(commands)
		if err != nil {
			return err
		}

		if action == tui.CommandPaletteCancel || selected == nil {
			return nil
		}

		// Execute the selected command
		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		switch selected.ID {
		case "restore-panes":
			restoreCmd := exec.Command(pawBin, "internal", "restore-panes", sessionName)
			return restoreCmd.Run()
		}

		return nil
	},
}

var restorePanesCmd = &cobra.Command{
	Use:    "restore-panes [session]",
	Short:  "Restore missing panes in current task window",
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
			logger.SetScript("restore-panes")
			logging.SetGlobal(logger)
		}

		logging.Trace("restorePanesCmd: start session=%s", sessionName)
		defer logging.Trace("restorePanesCmd: end")

		tm := tmux.New(sessionName)

		// Get current window info
		windowName, err := tm.Display("#{window_name}")
		if err != nil {
			return fmt.Errorf("failed to get window name: %w", err)
		}
		windowName = strings.TrimSpace(windowName)

		windowID, err := tm.Display("#{window_id}")
		if err != nil {
			return fmt.Errorf("failed to get window ID: %w", err)
		}
		windowID = strings.TrimSpace(windowID)

		logging.Debug("Current window: name=%s, id=%s", windowName, windowID)

		// Check if this is a task window (has task emoji prefix)
		taskName, isTaskWindow := constants.ExtractTaskName(windowName)
		if !isTaskWindow {
			_ = tm.DisplayMessage("Not a task window", 2000)
			return nil
		}

		logging.Debug("Task name (may be truncated): %s", taskName)

		// Find task using truncated name (window names are limited to MaxWindowNameLen chars)
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
		t, err := mgr.FindTaskByTruncatedName(taskName)
		if err != nil {
			_ = tm.DisplayMessage(fmt.Sprintf("Task not found: %s", taskName), 2000)
			logging.Debug("Task not found for truncated name: %s", taskName)
			return nil
		}
		agentDir := t.AgentDir
		logging.Debug("Found task: name=%s, agentDir=%s", t.Name, agentDir)

		// Get current pane count
		paneCount, err := tm.Display("#{window_panes}")
		if err != nil {
			return fmt.Errorf("failed to get pane count: %w", err)
		}
		paneCount = strings.TrimSpace(paneCount)

		logging.Debug("Current pane count: %s", paneCount)

		// Task window should have 2 panes: agent (0) and user (1)
		// Get working directory (t and mgr already set above)
		workDir := mgr.GetWorkingDirectory(t)

		// Check which pane is missing and restore
		switch paneCount {
		case "2":
			_ = tm.DisplayMessage("All panes are present", 2000)
			return nil
		case "0":
			// Both panes missing - respawn the window
			logging.Info("Both panes missing, respawning agent pane")

			// Start agent pane
			startAgentScript := filepath.Join(agentDir, "start-agent")
			if _, err := os.Stat(startAgentScript); os.IsNotExist(err) {
				_ = tm.DisplayMessage("start-agent script not found", 2000)
				return nil
			}

			if err := tm.RespawnPane(windowID+".0", workDir, startAgentScript); err != nil {
				return fmt.Errorf("failed to respawn agent pane: %w", err)
			}

			// Create user pane
			taskFilePath := t.GetTaskFilePath()
			userPaneCmd := fmt.Sprintf("sh -c 'cat %s; echo; exec %s'", taskFilePath, getShell())
			if err := tm.SplitWindow(windowID, true, workDir, userPaneCmd); err != nil {
				logging.Warn("Failed to create user pane: %v", err)
			}

			_ = tm.DisplayMessage("Restored both panes", 2000)
		case "1":
			// One pane exists - need to determine which one is missing
			// Check if the existing pane is running claude (agent) or shell (user)
			paneCmd, err := tm.GetPaneCommand(windowID + ".0")
			if err != nil {
				paneCmd = ""
			}
			paneCmd = strings.TrimSpace(paneCmd)

			logging.Debug("Existing pane command: %s", paneCmd)

			if paneCmd == "claude" || strings.Contains(paneCmd, "start-agent") {
				// Agent pane exists, user pane is missing
				logging.Info("User pane missing, creating it")
				taskFilePath := t.GetTaskFilePath()
				userPaneCmd := fmt.Sprintf("sh -c 'cat %s; echo; exec %s'", taskFilePath, getShell())
				if err := tm.SplitWindow(windowID, true, workDir, userPaneCmd); err != nil {
					return fmt.Errorf("failed to create user pane: %w", err)
				}
				_ = tm.DisplayMessage("Restored user pane", 2000)
			} else {
				// User pane exists (or unknown), agent pane is missing
				logging.Info("Agent pane missing, creating it")

				// Need to create agent pane before the user pane
				startAgentScript := filepath.Join(agentDir, "start-agent")
				if _, err := os.Stat(startAgentScript); os.IsNotExist(err) {
					_ = tm.DisplayMessage("start-agent script not found", 2000)
					return nil
				}

				// Split before the current pane to create agent pane at position 0
				_, err := tm.SplitWindowPane(tmux.SplitOpts{
					Target:     windowID + ".0",
					Horizontal: true,
					Before:     true,
					StartDir:   workDir,
					Command:    startAgentScript,
				})
				if err != nil {
					return fmt.Errorf("failed to create agent pane: %w", err)
				}
				_ = tm.DisplayMessage("Restored agent pane", 2000)
			}
		default:
			logging.Warn("Unexpected pane count (%s), skipping restore", paneCount)
			_ = tm.DisplayMessage(fmt.Sprintf("Unexpected pane count: %s", paneCount), 2000)
			return nil
		}

		logging.Info("Panes restored for task: %s", t.Name)

		// Check for stdin injection failure: if agent pane exists but session marker doesn't,
		// it means the task instruction was never sent
		if err := checkAndRecoverStdinInjection(tm, t, windowID, agentDir); err != nil {
			logging.Warn("Failed to recover stdin injection: %v", err)
		}

		return nil
	},
}

// checkAndRecoverStdinInjection detects and recovers from failed stdin injection.
// If Claude is running but the session marker doesn't exist, it sends the task instruction.
func checkAndRecoverStdinInjection(tm tmux.Client, t *task.Task, windowID, agentDir string) error {
	agentPane := windowID + ".0"

	// Check if session marker exists (indicates task instruction was sent successfully)
	if t.HasSessionMarker() {
		logging.Trace("checkAndRecoverStdinInjection: session marker exists, skipping")
		return nil
	}

	// Check if user prompt file exists (required to send task instruction)
	userPromptPath := t.GetUserPromptPath()
	if _, err := os.Stat(userPromptPath); os.IsNotExist(err) {
		logging.Debug("checkAndRecoverStdinInjection: user prompt not found, skipping")
		return nil
	}

	// Check if Claude is running in the agent pane
	claudeClient := claude.New()
	if !claudeClient.IsClaudeRunning(tm, agentPane) {
		logging.Debug("checkAndRecoverStdinInjection: Claude not running, skipping")
		return nil
	}

	// Claude is running but session marker doesn't exist - stdin injection likely failed
	logging.Info("Detected failed stdin injection for task %s, recovering...", t.Name)

	// Load task options to get ultrathink setting
	taskOpts, err := config.LoadTaskOptions(agentDir)
	if err != nil {
		logging.Warn("Failed to load task options: %v", err)
		taskOpts = config.DefaultTaskOptions()
	}

	// Build and send task instruction
	var taskInstruction string
	if taskOpts.Ultrathink {
		taskInstruction = fmt.Sprintf("ultrathink Read and execute the task from '%s'", userPromptPath)
	} else {
		taskInstruction = fmt.Sprintf("Read and execute the task from '%s'", userPromptPath)
	}

	logging.Debug("Sending task instruction: ultrathink=%v", taskOpts.Ultrathink)

	if err := claudeClient.SendInputWithRetry(tm, agentPane, taskInstruction, 5); err != nil {
		// Try basic send as last resort
		logging.Warn("SendInputWithRetry failed, trying basic send: %v", err)
		if err := claudeClient.SendInput(tm, agentPane, taskInstruction); err != nil {
			return fmt.Errorf("failed to send task instruction: %w", err)
		}
	}

	// Create session marker to prevent re-sending on next restore
	if err := t.CreateSessionMarker(); err != nil {
		logging.Warn("Failed to create session marker: %v", err)
	} else {
		logging.Debug("Session marker created after stdin recovery")
	}

	_ = tm.DisplayMessage(fmt.Sprintf("Recovered task instruction for: %s", t.Name), 2000)
	logging.Info("Successfully recovered stdin injection for task: %s", t.Name)

	return nil
}
