package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

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
	RunE: func(cmd *cobra.Command, args []string) error {
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
		if err := tm.WaitForPane(paneID, 5*time.Second, 1); err != nil {
			logging.Warn("toggleNewCmd: WaitForPane timed out, continuing anyway: %v", err)
		}

		// Send new-task command to the new window
		pawBin, _ := os.Executable()
		newTaskCmdStr := fmt.Sprintf("%s internal new-task %s", pawBin, sessionName)
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
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "new-task", "")
		defer cleanup()

		// Set project name for TUI display
		tui.SetProjectName(sessionName)

		// Initialize input history service (used for saving history after task creation)
		inputHistorySvc := service.NewInputHistoryService(appCtx.PawDir)

		// Get active task names for dependency selection
		activeTasks := getActiveTaskNames(appCtx.AgentsDir)

		// Loop continuously for task creation
		// History search (Ctrl+R) is handled via tmux popup, not inline
		for {
			// Use inline task input TUI with active task list
			result, err := tui.RunTaskInputWithTasks(activeTasks)
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

				// Create tmux client for target session and select the window first
				targetTm := tmux.New(target.Session)
				if err := targetTm.SelectWindow(target.WindowID); err != nil {
					logging.Warn("Failed to select target window: %v", err)
				}

				// Replace current process with tmux attach to target session
				// This switches the terminal from current project's socket to target project's socket
				socket := constants.TmuxSocketPrefix + target.Session
				tmuxPath, err := exec.LookPath("tmux")
				if err != nil {
					logging.Error("tmux not found: %v", err)
					fmt.Printf("Failed to find tmux: %v\n", err)
					continue
				}

				logging.Debug("Exec'ing into tmux attach: socket=%s, session=%s", socket, target.Session)
				err = syscall.Exec(tmuxPath,
					[]string{"tmux", "-L", socket, "attach-session", "-t", target.Session},
					os.Environ())
				if err != nil {
					// If exec fails (shouldn't normally happen), continue the loop
					logging.Error("Failed to exec tmux attach: %v", err)
					fmt.Printf("Failed to switch to session %s: %v\n", target.Session, err)
					continue
				}
				// If exec succeeds, this line is never reached (process is replaced)
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
				optsData, _ := json.Marshal(result.Options)
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
			pawBin, _ := os.Executable()
			spawnArgs := []string{"internal", "spawn-task", sessionName, tmpFile.Name()}
			if optsTmpPath != "" {
				spawnArgs = append(spawnArgs, optsTmpPath)
			}
			spawnCmd := exec.Command(pawBin, spawnArgs...)
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
	var names []string
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return names
	}
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
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> spawnTaskCmd(session=%s, contentFile=%s)", args[0], args[1])
		defer logging.Debug("<- spawnTaskCmd")

		sessionName := args[0]
		contentFile := args[1]
		var optsFile string
		if len(args) > 2 {
			optsFile = args[2]
		}

		// Read content from temp file
		contentBytes, err := os.ReadFile(contentFile)
		if err != nil {
			return fmt.Errorf("failed to read content file: %w", err)
		}
		content := string(contentBytes)

		// Read options from temp file if provided
		var taskOpts *config.TaskOptions
		if optsFile != "" {
			optsBytes, err := os.ReadFile(optsFile)
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
		pawBin, _ := os.Executable()

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
		loadingCmd := fmt.Sprintf("sh -c %q", fmt.Sprintf("%s internal loading-screen 'Generating task name...'", pawBin))
		if err := tm.RespawnPane(progressWindowID+".0", appCtx.ProjectDir, loadingCmd); err != nil {
			logging.Warn("Failed to run loading screen: %v", err)
		}

		// Create task (loading screen shows while this runs)
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		newTask, err := mgr.CreateTask(content)
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
				logging.Debug("Task options saved: model=%s, ultrathink=%v", taskOpts.Model, taskOpts.Ultrathink)
			}
		}

		// Handle task (creates actual window, starts Claude)
		handleCmd := exec.Command(pawBin, "internal", "handle-task", sessionName, newTask.AgentDir)
		if err := handleCmd.Start(); err != nil {
			logging.Warn("Failed to start handle-task: %v", err)
			return fmt.Errorf("failed to start task handler: %w", err)
		}

		// Wait for handle-task to create the window
		windowIDFile := filepath.Join(newTask.AgentDir, ".tab-lock", "window_id")
		for i := 0; i < 60; i++ { // 30 seconds max (60 * 500ms)
			if _, err := os.Stat(windowIDFile); err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		logging.Debug("Task window created for: %s", newTask.Name)

		return nil
	},
}
