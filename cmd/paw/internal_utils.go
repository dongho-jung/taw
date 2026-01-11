package main

import (
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
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/notify"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
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
		_, cleanup := setupLoggerFromApp(app, "ctrl-c", "")
		defer cleanup()

		logging.Debug("-> ctrlCCmd(session=%s)", sessionName)
		defer logging.Debug("<- ctrlCCmd")

		tm := tmux.New(sessionName)

		// Get current window name to check if this is a task window
		windowName, _ := tm.Display("#{window_name}")
		windowName = strings.TrimSpace(windowName)

		// If not a task window, just send Ctrl+C to the pane
		if !constants.IsTaskWindow(windowName) {
			return tm.SendKeys("", "C-c")
		}

		// Get pending cancel timestamp from tmux option
		pendingTimeStr, _ := tm.GetOption("@paw_cancel_pending")
		now := time.Now().Unix()

		// Check if there's a pending cancel within 2 seconds
		if pendingTimeStr != "" {
			pendingTime, err := parseUnixTime(pendingTimeStr)
			if err == nil {
				if now-pendingTime <= constants.DoublePressIntervalSec {
					// Double-press detected within 2 seconds, cancel the task
					_ = tm.SetOption("@paw_cancel_pending", "", true) // Clear pending state

					// Get current window ID
					windowID, err := tm.Display("#{window_id}")
					if err != nil {
						return fmt.Errorf("failed to get window ID: %w", err)
					}
					windowID = strings.TrimSpace(windowID)

					// Delegate to cancel-task-ui (shows progress in top pane)
					pawBin, _ := os.Executable()
					cancelCmd := exec.Command(pawBin, "internal", "cancel-task-ui", sessionName, windowID)
					return cancelCmd.Run()
				}
				// Time window expired - clear pending state and ignore this press
				// User must press again to start a new double-press sequence
				_ = tm.SetOption("@paw_cancel_pending", "", true)
				return nil
			}
		}

		// First press: just show warning, don't send Ctrl+C to pane
		// (sending Ctrl+C would cause Claude to exit immediately)

		// Store current timestamp
		_ = tm.SetOption("@paw_cancel_pending", fmt.Sprintf("%d", now), true)

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
	Short: "Rename a tmux window (with logging and notifications)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		windowID := args[0]
		name := args[1]

		// Try to get app for logging (use PAW_DIR or SESSION_NAME env)
		var logPath string
		var debug bool
		if pawDir := os.Getenv("PAW_DIR"); pawDir != "" {
			logPath = filepath.Join(pawDir, "log")
			debug = os.Getenv("PAW_DEBUG") == "1"
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

		logging.Debug("-> renameWindowCmd(windowID=%s, name=%s)", windowID, name)
		defer logging.Debug("<- renameWindowCmd")

		// Get session name from environment or use default
		sessionName := os.Getenv("SESSION_NAME")
		if sessionName == "" {
			sessionName = "paw" // fallback
		}

		tm := tmux.New(sessionName)
		pawDir := os.Getenv("PAW_DIR")
		taskName := os.Getenv("TASK_NAME")

		// Sound and notifications are handled centrally in renameWindowWithStatus
		// to ensure all status change paths trigger alerts
		if err := renameWindowWithStatus(tm, windowID, name, pawDir, taskName, "rename-window"); err != nil {
			return err
		}

		return nil
	},
}

func statusFromWindowName(name string) task.Status {
	switch {
	case strings.HasPrefix(name, constants.EmojiDone):
		return task.StatusDone
	case strings.HasPrefix(name, constants.EmojiWaiting):
		return task.StatusWaiting
	case strings.HasPrefix(name, constants.EmojiWarning):
		return task.StatusCorrupted
	case strings.HasPrefix(name, constants.EmojiWorking):
		return task.StatusWorking
	}
	return ""
}

// validateRequiredParams checks if all required parameters are non-empty.
// Returns an error with a descriptive message if any parameter is empty.
func validateRequiredParams(params map[string]string) error {
	for name, value := range params {
		if value == "" {
			return fmt.Errorf("%s is required but was empty", name)
		}
	}
	return nil
}

// setupLogger creates and configures a logger for a command handler.
// Returns a logger and a cleanup function. The cleanup function should be
// deferred immediately after calling this function.
// If taskName is empty, the task context will not be set.
func setupLogger(logPath string, debug bool, scriptName string, taskName string) (logging.Logger, func()) {
	logger, _ := logging.New(logPath, debug)
	if logger == nil {
		return nil, func() {}
	}

	logger.SetScript(scriptName)
	if taskName != "" {
		logger.SetTask(taskName)
	}
	logging.SetGlobal(logger)

	return logger, func() { _ = logger.Close() }
}

// setupLoggerFromApp creates and configures a logger from app context.
// This is a convenience wrapper around setupLogger for the common case
// where an *app.App is available.
func setupLoggerFromApp(appCtx *app.App, scriptName, taskName string) (logging.Logger, func()) {
	return setupLogger(appCtx.GetLogPath(), appCtx.Debug, scriptName, taskName)
}

func renameWindowWithStatus(tm tmux.Client, windowID, name, pawDir, taskName, source string) error {
	if err := tm.RenameWindow(windowID, name); err != nil {
		return err
	}

	status := statusFromWindowName(name)
	if status == "" || pawDir == "" || taskName == "" {
		return nil
	}

	agentDir := filepath.Join(pawDir, "agents", taskName)
	t := task.New(taskName, agentDir)
	prevStatus, valid, err := t.TransitionStatus(status)
	if err != nil {
		logging.Warn("Failed to save status: %v", err)
	} else {
		logging.Debug("Status saved: %s", status)
	}
	if !valid {
		logging.Warn("Invalid status transition: %s -> %s", prevStatus, status)
	}

	historyService := service.NewHistoryService(filepath.Join(pawDir, constants.HistoryDirName))
	if err := historyService.RecordStatusTransition(taskName, prevStatus, status, source, "", valid); err != nil {
		logging.Warn("Failed to record status transition: %v", err)
	}

	// Send notifications only when status actually changes (avoid duplicates)
	// This centralized notification ensures DONE state always triggers alerts.
	//
	// NOTE: WAITING state is handled by watch-wait watcher (wait.go) which provides
	// action buttons and prompt context. Warning states now also display as WAITING.
	if prevStatus != status && status == task.StatusDone {
		// Load config to get notification settings
		var notificationsConfig *config.NotificationsConfig
		sessionName := os.Getenv("SESSION_NAME")
		if sessionName != "" {
			if appCtx, err := getAppFromSession(sessionName); err == nil && appCtx.Config != nil {
				notificationsConfig = appCtx.Config.Notifications
			}
		}

		logging.Trace("renameWindowWithStatus: sending done notification for task=%s", taskName)
		notify.PlaySound(notify.SoundTaskCompleted)
		notify.SendAll(notificationsConfig, "Task ready", fmt.Sprintf("✅ %s is ready for review", taskName))
	}

	return nil
}

// getAppFromSession creates an App from session name
func getAppFromSession(sessionName string) (*app.App, error) {
	logging.Debug("-> getAppFromSession(session=%s)", sessionName)
	defer logging.Debug("<- getAppFromSession")

	// Session name is the project directory name
	// We need to find the project directory

	// First, try to get it from environment
	if pawDir := os.Getenv("PAW_DIR"); pawDir != "" {
		projectDir := filepath.Dir(pawDir)
		logging.Debug("getAppFromSession: found PAW_DIR=%s, projectDir=%s", pawDir, projectDir)
		application, err := app.New(projectDir)
		if err != nil {
			logging.Debug("getAppFromSession: app.New failed: %v", err)
			return nil, err
		}
		return loadAppConfig(application)
	}

	// Try current directory
	cwd, err := os.Getwd()
	if err != nil {
		logging.Debug("getAppFromSession: os.Getwd failed: %v", err)
		return nil, err
	}

	// Walk up to find .paw directory
	dir := cwd
	for {
		pawDir := filepath.Join(dir, ".paw")
		if _, err := os.Stat(pawDir); err == nil {
			logging.Debug("getAppFromSession: found .paw at %s", pawDir)
			application, err := app.New(dir)
			if err != nil {
				logging.Debug("getAppFromSession: app.New failed: %v", err)
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

	logging.Debug("getAppFromSession: could not find project directory")
	return nil, fmt.Errorf("could not find project directory for session %s", sessionName)
}

func loadAppConfig(application *app.App) (*app.App, error) {
	pawHome, _ := getPawHome()
	application.SetPawHome(pawHome)

	gitClient := git.New()
	application.SetGitRepo(gitClient.IsGitRepo(application.ProjectDir))

	if err := application.LoadConfig(); err != nil {
		application.Config = config.DefaultConfig()
	}
	if application.Config != nil {
		_ = os.Setenv("PAW_LOG_FORMAT", application.Config.LogFormat)
		_ = os.Setenv("PAW_LOG_MAX_SIZE_MB", fmt.Sprintf("%d", application.Config.LogMaxSizeMB))
		_ = os.Setenv("PAW_LOG_MAX_BACKUPS", fmt.Sprintf("%d", application.Config.LogMaxBackups))
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
