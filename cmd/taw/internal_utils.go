package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/taw/internal/app"
	"github.com/dongho-jung/taw/internal/config"
	"github.com/dongho-jung/taw/internal/constants"
	"github.com/dongho-jung/taw/internal/git"
	"github.com/dongho-jung/taw/internal/logging"
	"github.com/dongho-jung/taw/internal/notify"
	"github.com/dongho-jung/taw/internal/task"
	"github.com/dongho-jung/taw/internal/tmux"
)

var ctrlCCmd = &cobra.Command{
	Use:   "ctrl-c [session]",
	Short: "Handle Ctrl+C (double-press to cancel task)",
	Args:  cobra.ExactArgs(1),
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
			logger.SetScript("ctrl-c")
			logging.SetGlobal(logger)
		}

		logging.Trace("ctrlCCmd: start session=%s", sessionName)
		defer logging.Trace("ctrlCCmd: end")

		tm := tmux.New(sessionName)

		// Get current window name to check if this is a task window
		windowName, _ := tm.Display("#{window_name}")
		windowName = strings.TrimSpace(windowName)

		// If not a task window, just send Ctrl+C to the pane
		if !strings.HasPrefix(windowName, constants.EmojiWorking) &&
			!strings.HasPrefix(windowName, constants.EmojiWaiting) &&
			!strings.HasPrefix(windowName, constants.EmojiDone) &&
			!strings.HasPrefix(windowName, constants.EmojiWarning) {
			return tm.SendKeys("", "C-c")
		}

		// Get pending cancel timestamp from tmux option
		pendingTimeStr, _ := tm.GetOption("@taw_cancel_pending")
		now := time.Now().Unix()

		// Check if there's a pending cancel within 2 seconds
		if pendingTimeStr != "" {
			pendingTime, err := parseUnixTime(pendingTimeStr)
			if err == nil && now-pendingTime <= constants.DoublePressIntervalSec {
				// Double-press detected, cancel the task
				_ = tm.SetOption("@taw_cancel_pending", "", true) // Clear pending state

				// Get current window ID
				windowID, err := tm.Display("#{window_id}")
				if err != nil {
					return fmt.Errorf("failed to get window ID: %w", err)
				}
				windowID = strings.TrimSpace(windowID)

				// Delegate to cancel-task-ui (shows progress in top pane)
				tawBin, _ := os.Executable()
				cancelCmd := exec.Command(tawBin, "internal", "cancel-task-ui", sessionName, windowID)
				return cancelCmd.Run()
			}
		}

		// First press: just show warning, don't send Ctrl+C to pane
		// (sending Ctrl+C would cause Claude to exit immediately)

		// Store current timestamp
		_ = tm.SetOption("@taw_cancel_pending", fmt.Sprintf("%d", now), true)

		// Play sound to indicate pending cancel state
		logging.Trace("ctrlCCmd: playing SoundCancelPending (first press, waiting for second)")
		notify.PlaySound(notify.SoundCancelPending)

		// Show message to user
		_ = tm.DisplayMessage("⌃K again to cancel task", 2000)

		return nil
	},
}

func parseUnixTime(s string) (int64, error) {
	var t int64
	_, err := fmt.Sscanf(s, "%d", &t)
	return t, err
}

var renameWindowCmd = &cobra.Command{
	Use:   "rename-window [window-id] [name]",
	Short: "Rename a tmux window (with logging and sound)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		windowID := args[0]
		name := args[1]

		// Try to get app for logging (use TAW_DIR or SESSION_NAME env)
		var logPath string
		var debug bool
		if tawDir := os.Getenv("TAW_DIR"); tawDir != "" {
			logPath = filepath.Join(tawDir, "log")
			debug = os.Getenv("TAW_DEBUG") == "1"
		}

		if logPath != "" {
			logger, _ := logging.New(logPath, debug)
			if logger != nil {
				defer func() { _ = logger.Close() }()
				logger.SetScript("rename-window")
				if taskName := os.Getenv("TASK_NAME"); taskName != "" {
					logger.SetTask(taskName)
				}
				logging.SetGlobal(logger)
			}
		}

		logging.Trace("renameWindowCmd: start windowID=%s name=%s", windowID, name)
		defer logging.Trace("renameWindowCmd: end")

		// Get session name from environment or use default
		sessionName := os.Getenv("SESSION_NAME")
		if sessionName == "" {
			sessionName = "taw" // fallback
		}

		tm := tmux.New(sessionName)
		err := tm.RenameWindow(windowID, name)
		if err != nil {
			return err
		}

		// Determine status from emoji prefix and save it
		var newStatus task.Status
		switch {
		case strings.HasPrefix(name, constants.EmojiDone):
			newStatus = task.StatusDone
			logging.Trace("renameWindowCmd: playing SoundTaskCompleted (done state)")
			notify.PlaySound(notify.SoundTaskCompleted)
		case strings.HasPrefix(name, constants.EmojiWaiting):
			newStatus = task.StatusWaiting
			logging.Trace("renameWindowCmd: playing SoundNeedInput (waiting state)")
			notify.PlaySound(notify.SoundNeedInput)
		case strings.HasPrefix(name, constants.EmojiWarning):
			newStatus = task.StatusCorrupted
			logging.Trace("renameWindowCmd: playing SoundError (warning state)")
			notify.PlaySound(notify.SoundError)
		case strings.HasPrefix(name, constants.EmojiWorking):
			newStatus = task.StatusWorking
			// No sound for EmojiWorking (🤖) - too frequent
		}

		// Save status to disk for resume
		if newStatus != "" {
			tawDir := os.Getenv("TAW_DIR")
			taskName := os.Getenv("TASK_NAME")
			if tawDir != "" && taskName != "" {
				agentDir := filepath.Join(tawDir, "agents", taskName)
				t := task.New(taskName, agentDir)
				if err := t.SaveStatus(newStatus); err != nil {
					logging.Trace("Failed to save status: %v", err)
				} else {
					logging.Trace("Status saved: %s", newStatus)
				}
			}
		}

		return nil
	},
}

// getAppFromSession creates an App from session name
func getAppFromSession(sessionName string) (*app.App, error) {
	// Session name is the project directory name
	// We need to find the project directory

	// First, try to get it from environment
	if tawDir := os.Getenv("TAW_DIR"); tawDir != "" {
		projectDir := filepath.Dir(tawDir)
		application, err := app.New(projectDir)
		if err != nil {
			return nil, err
		}
		return loadAppConfig(application)
	}

	// Try current directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Walk up to find .taw directory
	dir := cwd
	for {
		tawDir := filepath.Join(dir, ".taw")
		if _, err := os.Stat(tawDir); err == nil {
			application, err := app.New(dir)
			if err != nil {
				return nil, err
			}
			return loadAppConfig(application)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return nil, fmt.Errorf("could not find project directory for session %s", sessionName)
}

func loadAppConfig(application *app.App) (*app.App, error) {
	tawHome, _ := getTawHome()
	application.SetTawHome(tawHome)

	gitClient := git.New()
	application.SetGitRepo(gitClient.IsGitRepo(application.ProjectDir))

	if err := application.LoadConfig(); err != nil {
		application.Config = config.DefaultConfig()
	}

	return application, nil
}

// getShell returns the user's preferred shell
func getShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return shell
}

