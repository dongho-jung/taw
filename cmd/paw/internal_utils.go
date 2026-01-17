package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		if err := renameWindowWithStatus(tm, windowID, name, pawDir, taskName, "rename-window", ""); err != nil {
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
	case strings.HasPrefix(name, constants.EmojiReview):
		return task.StatusWaiting
	case strings.HasPrefix(name, constants.EmojiWarning):
		return task.StatusWaiting
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

func renameWindowWithStatus(tm tmux.Client, windowID, name, pawDir, taskName, source string, statusOverride task.Status) error {
	if err := tm.RenameWindow(windowID, name); err != nil {
		return err
	}

	status := statusOverride
	if status == "" {
		status = statusFromWindowName(name)
	}
	if status == "" || pawDir == "" || taskName == "" {
		return nil
	}

	agentDir := filepath.Join(pawDir, "agents", taskName)
	t := task.New(taskName, agentDir)
	prevStatus, valid, err := t.TransitionStatus(status)
	if err != nil {
		logging.Warn("Failed to save status: %v", err)
	} else {
		logging.Info("Status saved: %s", status)
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
	// action buttons and prompt context. Corrupted states also display as WAITING.
	if prevStatus != status && status == task.StatusDone {
		logging.Info("renameWindowWithStatus: sending done notification for task=%s", taskName)
		notify.PlaySound(notify.SoundTaskCompleted)
		_ = notify.Send("Task ready", fmt.Sprintf("✅ %s is ready for review", taskName))
	}

	return nil
}

// getAppFromSession creates an App from session name
func getAppFromSession(sessionName string) (*app.App, error) {
	logging.Debug("-> getAppFromSession(session=%s)", sessionName)
	defer logging.Debug("<- getAppFromSession")

	// Session name is the project directory name
	// We need to find the project directory

	gitClient := git.New()

	// Get environment variables - both may be set
	projectDirEnv := os.Getenv("PROJECT_DIR")
	pawDirEnv := os.Getenv("PAW_DIR")
	if pawDirEnv != "" {
		// Clean the path to remove any trailing slashes
		// This is important because filepath.Dir("/a/b/c/") returns "/a/b/c" instead of "/a/b"
		pawDirEnv = filepath.Clean(pawDirEnv)
	}

	// First, try PROJECT_DIR environment variable (most reliable)
	if projectDirEnv != "" {
		logging.Debug("getAppFromSession: found PROJECT_DIR=%s", projectDirEnv)
		// Check if project is a git repo (needed for correct workspace location)
		isGitRepo := gitClient.IsGitRepo(projectDirEnv)
		logging.Debug("getAppFromSession: isGitRepo=%v", isGitRepo)
		application, err := app.NewWithGitInfo(projectDirEnv, isGitRepo)
		if err != nil {
			logging.Debug("getAppFromSession: app.NewWithGitInfo failed: %v", err)
			return nil, err
		}
		// If PAW_DIR is also set, use it directly instead of recalculating
		// This ensures we use the exact workspace path that was passed to us
		if pawDirEnv != "" {
			logging.Debug("getAppFromSession: overriding PawDir with PAW_DIR=%s", pawDirEnv)
			application.PawDir = pawDirEnv
			application.AgentsDir = filepath.Join(pawDirEnv, constants.AgentsDirName)
		}
		return loadAppConfig(application)
	}

	// Try PAW_DIR environment variable only
	// Note: For global workspaces, PAW_DIR is not at {project}/.paw but at
	// ~/.local/share/paw/workspaces/{project-id}, so filepath.Dir won't work.
	// We need to read the project path from the workspace itself.
	if pawDirEnv != "" {
		logging.Debug("getAppFromSession: found PAW_DIR=%s (no PROJECT_DIR)", pawDirEnv)

		// Check if PAW_DIR is a global workspace by looking for project-path file
		projectPathFile := filepath.Join(pawDirEnv, ".project-path")
		if data, err := os.ReadFile(projectPathFile); err == nil {
			projectDir := strings.TrimSpace(string(data))
			logging.Debug("getAppFromSession: found project-path=%s", projectDir)
			isGitRepo := gitClient.IsGitRepo(projectDir)
			application, err := app.NewWithGitInfo(projectDir, isGitRepo)
			if err != nil {
				logging.Debug("getAppFromSession: app.NewWithGitInfo failed: %v", err)
				return nil, err
			}
			// Use the PAW_DIR directly
			application.PawDir = pawDirEnv
			application.AgentsDir = filepath.Join(pawDirEnv, constants.AgentsDirName)
			return loadAppConfig(application)
		}

		// Fallback: assume PAW_DIR is at {project}/.paw (local workspace)
		projectDir := filepath.Dir(pawDirEnv)
		logging.Debug("getAppFromSession: assuming local workspace, projectDir=%s", projectDir)
		isGitRepo := gitClient.IsGitRepo(projectDir)
		application, err := app.NewWithGitInfo(projectDir, isGitRepo)
		if err != nil {
			logging.Debug("getAppFromSession: app.NewWithGitInfo failed: %v", err)
			return nil, err
		}
		// Use the PAW_DIR directly
		application.PawDir = pawDirEnv
		application.AgentsDir = filepath.Join(pawDirEnv, constants.AgentsDirName)
		return loadAppConfig(application)
	}

	// Try current directory - walk up to find .paw but stop at home directory
	// to avoid finding unrelated .paw directories in parent paths
	cwd, err := os.Getwd()
	if err != nil {
		logging.Debug("getAppFromSession: os.Getwd failed: %v", err)
		return nil, err
	}

	homeDir, _ := os.UserHomeDir()

	// Walk up to find .paw directory
	dir := cwd
	for {
		// Stop at home directory to avoid finding unrelated .paw
		if homeDir != "" && dir == homeDir {
			logging.Debug("getAppFromSession: reached home directory, stopping search")
			break
		}

		pawDir := filepath.Join(dir, ".paw")
		if _, err := os.Stat(pawDir); err == nil {
			logging.Debug("getAppFromSession: found .paw at %s", pawDir)
			isGitRepo := gitClient.IsGitRepo(dir)
			application, err := app.NewWithGitInfo(dir, isGitRepo)
			if err != nil {
				logging.Debug("getAppFromSession: app.NewWithGitInfo failed: %v", err)
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

	// No local .paw found - try using cwd directly
	// This handles the case where the workspace is stored globally (auto mode for git repos)
	// We need to check if it's a git repo to resolve the correct workspace path
	logging.Debug("getAppFromSession: no local .paw found, trying cwd=%s", cwd)
	isGitRepo := gitClient.IsGitRepo(cwd)
	projectDir := cwd
	if isGitRepo {
		if repoRoot, err := gitClient.GetRepoRoot(cwd); err == nil {
			projectDir = repoRoot
		}
	}
	logging.Debug("getAppFromSession: isGitRepo=%v, projectDir=%s", isGitRepo, projectDir)
	application, err := app.NewWithGitInfo(projectDir, isGitRepo)
	if err != nil {
		logging.Debug("getAppFromSession: app.NewWithGitInfo failed: %v", err)
		return nil, fmt.Errorf("could not find project directory for session %s", sessionName)
	}

	// Verify that the workspace exists (was initialized)
	if !application.IsInitialized() {
		logging.Debug("getAppFromSession: workspace not initialized at %s", application.PawDir)
		return nil, fmt.Errorf("could not find project directory for session %s", sessionName)
	}

	return loadAppConfig(application)
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

func buildTaskInstruction(userPromptPath string) string {
	return fmt.Sprintf("Read and execute the task from '%s'", userPromptPath)
}
