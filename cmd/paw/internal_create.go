package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var toggleNewCmd = &cobra.Command{
	Use:   "toggle-new [session]",
	Short: "Toggle the new task window",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "toggle-new", "")
		defer cleanup()

		logging.Debug("-> toggleNewCmd(session=%s)", sessionName)
		defer logging.Debug("<- toggleNewCmd")

		tm := tmux.New(sessionName)

		// Check if _ window exists
		windows, err := tm.ListWindows()
		if err != nil {
			return err
		}

		for _, w := range windows {
			if strings.HasPrefix(w.Name, constants.EmojiNew) {
				// Window exists, just select it (don't send command again to avoid pasting into vim/editor)
				logging.Trace("toggleNewCmd: new task window already exists, selecting windowID=%s", w.ID)
				return tm.SelectWindow(w.ID)
			}
		}

		// Create new window without command (keeps shell open)
		logging.Trace("toggleNewCmd: creating new task window name=%s", constants.NewWindowName)
		windowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     constants.NewWindowName,
			StartDir: appCtx.ProjectDir,
		})
		if err != nil {
			return err
		}
		logging.Trace("toggleNewCmd: new task window created windowID=%s", windowID)

		// Wait for shell to be ready before sending keys
		paneID := windowID + ".0"
		if err := tm.WaitForPane(paneID, constants.PaneWaitTimeout, 1); err != nil {
			logging.Warn("toggleNewCmd: WaitForPane timed out, continuing anyway: %v", err)
		}

		// Send new-task command to the new window
		// Include PAW_DIR, PROJECT_DIR, and DISPLAY_NAME so getAppFromSession can find the project
		newTaskCmdStr := buildNewTaskCommand(appCtx, getPawBin(), sessionName)
		if err := tm.SendKeysLiteral(windowID, newTaskCmdStr); err != nil {
			return fmt.Errorf("failed to send keys: %w", err)
		}
		if err := tm.SendKeys(windowID, "Enter"); err != nil {
			return fmt.Errorf("failed to send Enter: %w", err)
		}

		return nil
	},
}

var newTaskCmd = &cobra.Command{
	Use:   "new-task [session]",
	Short: "Create a new task",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}
		_ = os.Setenv("PAW_DIR", appCtx.PawDir)
		_ = os.Setenv("PROJECT_DIR", appCtx.ProjectDir)
		_ = os.Setenv("SESSION_NAME", appCtx.SessionName)

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "new-task", "")
		defer cleanup()

		// Set project name for TUI display
		// Use DISPLAY_NAME env var if set (for subdirectory context like "repo/subdir")
		displayName := os.Getenv("DISPLAY_NAME")
		if displayName == "" {
			displayName = sessionName
		}
		tui.SetProjectName(displayName)
		tui.SetSessionName(sessionName)

		// Initialize input history service (used for saving history after task creation)
		inputHistorySvc := service.NewInputHistoryService(appCtx.PawDir)

		// Get active task names for dependency selection
		activeTasks := getActiveTaskNames(appCtx.AgentsDir)

		// Loop continuously for task creation
		// History search (Ctrl+R) is handled via tmux popup, not inline
		for {
			// Use inline task input TUI with active task list and git mode flag
			result, err := tui.RunTaskInputWithOptions(activeTasks, appCtx.IsGitRepo, "")
			if err != nil {
				fmt.Printf("Failed to get task input: %v\n", err)
				continue
			}

			if result.Cancelled {
				fmt.Println("Task creation cancelled.")
				continue
			}

			// Handle cross-project jump request
			if result.JumpTarget != nil {
				target := result.JumpTarget
				logging.Debug("Cross-project jump requested: session=%s, window=%s", target.Session, target.WindowID)

				// Ensure main window exists in target session (recovery)
				// This is needed because we're bypassing PAW's normal attach flow
				if err := ensureMainWindowInSession(target.Session); err != nil {
					logging.Warn("Failed to ensure main window in target session: %v", err)
				}

				// Select the task window in target session
				targetTm := tmux.New(target.Session)
				if err := targetTm.SelectWindow(target.WindowID); err != nil {
					logging.Warn("Failed to select target window: %v", err)
				}

				// Use detach-client -E to replace the current client with a new attachment
				// to the target session. This works across different tmux sockets and
				// prevents nesting (unlike syscall.Exec which would create nested tmux).
				targetSocket := constants.TmuxSocketPrefix + target.Session
				switchCmd := fmt.Sprintf("tmux -L %s attach-session -t %s", shellQuote(targetSocket), shellQuote(target.Session))
				logging.Debug("Switching to session via detach-client -E: %s", switchCmd)

				tm := tmux.New(sessionName)
				if err := tm.Run("detach-client", "-E", switchCmd); err != nil {
					logging.Error("Failed to switch session: %v", err)
					fmt.Printf("Failed to switch to session %s: %v\n", target.Session, err)
					continue
				}
				// detach-client -E replaces the client, so this line may not be reached
				return nil
			}

			// Handle task name popup request (Alt+Enter with empty content)
			if result.RequestTaskNamePopup {
				logging.Debug("Task name popup requested")
				taskName := showTaskNamePopup(sessionName, appCtx)
				if taskName == "" {
					// User cancelled or popup failed
					continue
				}
				// Create task with the entered name as both branch name and content
				if err := createTaskWithName(sessionName, appCtx, taskName); err != nil {
					logging.Error("Failed to create task with name: %v", err)
					fmt.Printf("Failed to create task: %v\n", err)
				}
				// Refresh active tasks list
				activeTasks = getActiveTaskNames(appCtx.AgentsDir)
				continue
			}

			if result.Content == "" {
				fmt.Println("Task content is empty, try again.")
				continue
			}

			content := result.Content

			// Save content to temp file for spawn-task to read
			tmpFile, err := os.CreateTemp("", "paw-task-content-*.txt")
			if err != nil {
				fmt.Printf("Failed to create temp file: %v\n", err)
				continue
			}
			if _, err := tmpFile.WriteString(content); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
				fmt.Printf("Failed to write task content: %v\n", err)
				continue
			}
			_ = tmpFile.Close()

			// Save options to temp file
			var optsTmpPath string
			if result.Options != nil {
				optsTmpFile, err := os.CreateTemp("", "paw-task-opts-*.json")
				if err != nil {
					_ = os.Remove(tmpFile.Name())
					fmt.Printf("Failed to create options temp file: %v\n", err)
					continue
				}
				optsData, err := json.Marshal(result.Options)
				if err != nil {
					_ = optsTmpFile.Close()
					_ = os.Remove(optsTmpFile.Name())
					_ = os.Remove(tmpFile.Name())
					fmt.Printf("Failed to marshal task options: %v\n", err)
					continue
				}
				if _, err := optsTmpFile.Write(optsData); err != nil {
					_ = optsTmpFile.Close()
					_ = os.Remove(optsTmpFile.Name())
					_ = os.Remove(tmpFile.Name())
					fmt.Printf("Failed to write task options: %v\n", err)
					continue
				}
				_ = optsTmpFile.Close()
				optsTmpPath = optsTmpFile.Name()
			}

			// Spawn task creation in a separate window (non-blocking)
			spawnArgs := []string{"internal", "spawn-task", sessionName, tmpFile.Name()}
			if optsTmpPath != "" {
				spawnArgs = append(spawnArgs, optsTmpPath)
			}
			spawnCmd := exec.Command(getPawBin(), spawnArgs...) //nolint:gosec // G204: pawBin is from getPawBin()
			if err := spawnCmd.Start(); err != nil {
				_ = os.Remove(tmpFile.Name())
				if optsTmpPath != "" {
					_ = os.Remove(optsTmpPath)
				}
				logging.Warn("Failed to start spawn-task: %v", err)
				fmt.Printf("Failed to start task: %v\n", err)
				continue
			}

			// Save content to input history (after successful spawn)
			if err := inputHistorySvc.SaveInput(content); err != nil {
				logging.Warn("Failed to save input history: %v", err)
			}

			logging.Debug("Task spawned in background, content file: %s, opts file: %s", tmpFile.Name(), optsTmpPath)

			// Refresh active tasks list
			activeTasks = getActiveTaskNames(appCtx.AgentsDir)

			// Immediately loop back to create another task
		}
	},
}

// getActiveTaskNames returns a list of active task names from the agents directory.
func getActiveTaskNames(agentsDir string) []string {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

var spawnTaskCmd = &cobra.Command{
	Use:   "spawn-task [session] [content-file] [options-file]",
	Short: "Spawn a task in a separate window (shows progress)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(_ *cobra.Command, args []string) error {
		logging.Debug("-> spawnTaskCmd(session=%s, contentFile=%s)", args[0], args[1])
		defer logging.Debug("<- spawnTaskCmd")

		sessionName := args[0]
		contentFile := args[1]
		var optsFile string
		if len(args) > 2 {
			optsFile = args[2]
		}

		// Read content from temp file
		contentBytes, err := os.ReadFile(contentFile) //nolint:gosec // G304: contentFile is from cmd args, used for IPC
		if err != nil {
			return fmt.Errorf("failed to read content file: %w", err)
		}
		content := string(contentBytes)

		// Read options from temp file if provided
		var taskOpts *config.TaskOptions
		if optsFile != "" {
			optsBytes, err := os.ReadFile(optsFile) //nolint:gosec // G304: optsFile is from cmd args, used for IPC
			if err != nil {
				logging.Warn("Failed to read options file: %v", err)
			} else {
				taskOpts = &config.TaskOptions{}
				if err := json.Unmarshal(optsBytes, taskOpts); err != nil {
					logging.Warn("Failed to parse options file: %v", err)
					taskOpts = nil
				}
			}
			_ = os.Remove(optsFile)
		}

		// Clean up temp file
		_ = os.Remove(contentFile)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "spawn-task", "")
		defer cleanup()

		tm := tmux.New(sessionName)
		pawBin := getPawBin()

		// Create a temporary "⏳" window for progress display
		progressWindowName := "⏳..."
		logging.Trace("spawnTaskCmd: creating progress window name=%s", progressWindowName)
		progressWindowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     progressWindowName,
			StartDir: appCtx.ProjectDir,
			Detached: true,
		})
		if err != nil {
			return fmt.Errorf("failed to create progress window: %w", err)
		}
		logging.Trace("spawnTaskCmd: progress window created windowID=%s", progressWindowID)

		logging.Debug("Created progress window: %s", progressWindowID)

		// Clean up progress window on exit (success or failure)
		defer func() {
			if err := tm.KillWindow(progressWindowID); err != nil {
				logging.Trace("Failed to kill progress window (may already be closed): %v", err)
			}
		}()

		// Run loading screen inside the progress window
		loadingCmd := shellJoin(pawBin, "internal", "loading-screen", "Generating task name...")
		if err := tm.RespawnPane(progressWindowID+".0", appCtx.ProjectDir, loadingCmd); err != nil {
			logging.Warn("Failed to run loading screen: %v", err)
		}

		// Create task (loading screen shows while this runs)
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)

		// Pass custom branch name if provided in options
		var customBranchName string
		if taskOpts != nil && taskOpts.BranchName != "" {
			customBranchName = taskOpts.BranchName
			logging.Debug("Using custom branch name from options: %s", customBranchName)
		}

		newTask, err := mgr.CreateTask(content, customBranchName)
		if err != nil {
			logging.Error("Failed to create task: %v", err)
			return fmt.Errorf("failed to create task: %w", err)
		}

		logging.Log("Task created: %s", newTask.Name)

		// Save task options if provided
		if taskOpts != nil {
			if err := taskOpts.Save(newTask.AgentDir); err != nil {
				logging.Warn("Failed to save task options: %v", err)
			} else {
				logging.Debug("Task options saved: model=%s", taskOpts.Model)
			}
		}

		// Handle task (creates actual window, starts Claude)
		handleCmd := exec.Command(pawBin, "internal", "handle-task", sessionName, newTask.AgentDir) //nolint:gosec // G204: pawBin is from getPawBin()
		// Pass PAW_DIR and PROJECT_DIR so getAppFromSession can find the project
		// (required for global workspaces where there's no local .paw directory)
		handleCmd.Env = append(os.Environ(),
			"PAW_DIR="+appCtx.PawDir,
			"PROJECT_DIR="+appCtx.ProjectDir,
		)
		if err := handleCmd.Start(); err != nil {
			logging.Warn("Failed to start handle-task: %v", err)
			return fmt.Errorf("failed to start task handler: %w", err)
		}

		// Wait for handle-task to create the window
		windowIDFile := filepath.Join(newTask.AgentDir, constants.TabLockDirName, constants.WindowIDFileName)
		for i := 0; i < constants.WindowIDWaitMaxAttempts; i++ {
			if _, err := os.Stat(windowIDFile); err == nil {
				break
			}
			time.Sleep(constants.WindowIDWaitInterval)
		}

		logging.Debug("Task window created for: %s", newTask.Name)

		return nil
	},
}

// ensureMainWindowInSession ensures the main window (⭐️main) exists in the given session.
// If it doesn't exist, it creates one. This is used when jumping to another project
// to ensure the target session has a properly functioning main window.
func ensureMainWindowInSession(sessionName string) error {
	tm := tmux.New(sessionName)

	// Check if main window exists
	windows, err := tm.ListWindows()
	if err != nil {
		return fmt.Errorf("failed to list windows: %w", err)
	}

	for _, w := range windows {
		if strings.HasPrefix(w.Name, constants.EmojiNew) {
			// Main window exists - check if it needs respawning
			// (might have exited command, showing just shell prompt)
			logging.Debug("Main window already exists in session %s: %s", sessionName, w.ID)

			// Get app context to respawn with correct command
			appCtx, err := getAppFromSession(sessionName)
			if err != nil {
				logging.Warn("Failed to get app context for respawn: %v", err)
				return nil
			}
			syncSessionEnv(tm, appCtx)

			// Respawn the pane to ensure new-task is running
			// Include PAW_DIR, PROJECT_DIR, and DISPLAY_NAME so getAppFromSession can find the project
			newTaskCmd := buildNewTaskCommand(appCtx, getPawBin(), sessionName)
			if err := tm.RespawnPane(w.ID+".0", appCtx.ProjectDir, newTaskCmd); err != nil {
				logging.Warn("Failed to respawn main window: %v", err)
			}
			return nil
		}
	}

	// Main window doesn't exist - create it
	logging.Log("Main window not found in session %s, creating...", sessionName)

	// Get app context for the target session
	appCtx, err := getAppFromSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to get app context: %w", err)
	}
	syncSessionEnv(tm, appCtx)

	// Build the new-task command
	// Include PAW_DIR, PROJECT_DIR, and DISPLAY_NAME so getAppFromSession can find the project
	newTaskCmd := buildNewTaskCommand(appCtx, getPawBin(), sessionName)

	// Create new window with command directly (more reliable than sending keys)
	// Using Command option ensures the command runs immediately without race conditions
	windowID, err := tm.NewWindow(tmux.WindowOpts{
		Name:     constants.NewWindowName,
		StartDir: appCtx.ProjectDir,
		Command:  newTaskCmd,
	})
	if err != nil {
		return fmt.Errorf("failed to create main window: %w", err)
	}

	logging.Log("Main window created in session %s: %s", sessionName, windowID)
	return nil
}

// showTaskNamePopup shows a popup for entering a task name directly.
// Returns the entered task name, or empty string if cancelled/failed.
func showTaskNamePopup(sessionName string, appCtx *app.App) string {
	tm := tmux.New(sessionName)

	// Build the task name input TUI command
	tuiCmd := shellJoin(getPawBin(), "internal", "task-name-input-tui", sessionName)

	// Show the popup
	err := tm.DisplayPopup(tmux.PopupOpts{
		Width:     constants.PopupWidthTaskName,
		Height:    constants.PopupHeightTaskName,
		Title:     " Enter Task Name ",
		Close:     true,
		Style:     "fg=terminal,bg=terminal",
		Directory: appCtx.ProjectDir,
		Env: map[string]string{
			"PAW_DIR":     appCtx.PawDir,
			"PROJECT_DIR": appCtx.ProjectDir,
		},
	}, tuiCmd)
	if err != nil {
		logging.Debug("showTaskNamePopup: DisplayPopup failed: %v", err)
		return ""
	}

	// Read the selection file
	selectionPath := filepath.Join(appCtx.PawDir, constants.TaskNameSelectionFile)
	data, err := os.ReadFile(selectionPath) //nolint:gosec // G304: selectionPath is constructed from pawDir
	if err != nil {
		logging.Debug("showTaskNamePopup: no selection file: %v", err)
		return ""
	}

	// Delete the file after reading
	_ = os.Remove(selectionPath)

	taskName := strings.TrimSpace(string(data))
	logging.Debug("showTaskNamePopup: got task name: %s", taskName)
	return taskName
}

// createTaskWithName creates a task with the given name as both the branch name and content.
func createTaskWithName(sessionName string, appCtx *app.App, taskName string) error {
	logging.Debug("createTaskWithName: creating task with name: %s", taskName)

	// Create content from the task name (convert kebab-case to readable format)
	content := "Task: " + taskName

	// Save content to temp file for spawn-task to read
	tmpFile, err := os.CreateTemp("", "paw-task-content-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to write task content: %w", err)
	}
	_ = tmpFile.Close()

	// Create options with custom branch name
	taskOpts := config.DefaultTaskOptions()
	taskOpts.BranchName = taskName

	// Save options to temp file
	optsTmpFile, err := os.CreateTemp("", "paw-task-opts-*.json")
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to create options temp file: %w", err)
	}
	optsData, err := json.Marshal(taskOpts)
	if err != nil {
		_ = optsTmpFile.Close()
		_ = os.Remove(optsTmpFile.Name())
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to marshal task options: %w", err)
	}
	if _, err := optsTmpFile.Write(optsData); err != nil {
		_ = optsTmpFile.Close()
		_ = os.Remove(optsTmpFile.Name())
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to write task options: %w", err)
	}
	_ = optsTmpFile.Close()

	// Initialize input history service
	inputHistorySvc := service.NewInputHistoryService(appCtx.PawDir)

	// Spawn task creation in a separate window (non-blocking)
	spawnArgs := []string{"internal", "spawn-task", sessionName, tmpFile.Name(), optsTmpFile.Name()}
	spawnCmd := exec.Command(getPawBin(), spawnArgs...) //nolint:gosec // G204: pawBin is from getPawBin()
	if err := spawnCmd.Start(); err != nil {
		_ = os.Remove(tmpFile.Name())
		_ = os.Remove(optsTmpFile.Name())
		return fmt.Errorf("failed to start spawn-task: %w", err)
	}

	// Save content to input history
	if err := inputHistorySvc.SaveInput(content); err != nil {
		logging.Warn("Failed to save input history: %v", err)
	}

	logging.Debug("createTaskWithName: task spawned, content file: %s, opts file: %s", tmpFile.Name(), optsTmpFile.Name())
	return nil
}
