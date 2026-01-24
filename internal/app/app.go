// Package app provides the main application context and dependency injection.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
)

// App represents the main application context with all dependencies.
type App struct {
	// Paths
	ProjectDir string // Root directory of the user's project
	PawDir     string // .paw directory path
	AgentsDir  string // agents directory path
	PawHome    string // PAW installation directory

	// Session
	SessionName string // tmux session name
	DisplayName string // Display name for UI (may include subdir context like "repo/subdir")

	// State
	IsGitRepo bool           // Whether the project is a git repository
	Config    *config.Config // Project configuration

	// Runtime
	Debug bool // Debug mode enabled
}

// New creates a new App instance for the given project directory.
// It resolves the PawDir based on auto mode (git repo -> global, non-git -> local)
// unless overridden.
func New(projectDir string) (*App, error) {
	return NewWithGitInfo(projectDir, false)
}

// NewWithGitInfo creates a new App instance with explicit git repo information.
// This is used when the caller already knows if the project is a git repo.
func NewWithGitInfo(projectDir string, isGitRepo bool) (*App, error) {
	return NewWithGitInfoWithWorkspace(projectDir, isGitRepo, config.PawInProjectAuto)
}

// NewWithGitInfoWithWorkspace creates a new App instance with explicit git repo
// information and workspace mode override.
// Use PawInProjectAuto for default behavior or PawInProjectLocal to force local workspace.
func NewWithGitInfoWithWorkspace(projectDir string, isGitRepo bool, pawInProject config.PawInProject) (*App, error) {
	absPath, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project path: %w", err)
	}

	// Determine session name from project directory name
	sessionName := filepath.Base(absPath)

	// Check if debug mode is enabled
	debug := os.Getenv("PAW_DEBUG") == "1"

	// Resolve workspace directory.
	pawDir := config.GetWorkspaceDir(absPath, pawInProject, isGitRepo)
	agentsDir := filepath.Join(pawDir, constants.AgentsDirName)

	app := &App{
		ProjectDir:  absPath,
		PawDir:      pawDir,
		AgentsDir:   agentsDir,
		SessionName: sessionName,
		Debug:       debug,
		IsGitRepo:   isGitRepo,
	}

	return app, nil
}

// Initialize sets up the .paw directory structure.
func (a *App) Initialize() error {
	// Create directories
	dirs := []string{
		a.PawDir,
		a.AgentsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Write project path file for global workspaces
	// This allows getAppFromSession to find the correct project directory
	// when PAW_DIR points to a global workspace path
	if a.IsGlobalWorkspace() {
		projectPathFile := filepath.Join(a.PawDir, constants.ProjectPathFileName)
		if err := os.WriteFile(projectPathFile, []byte(a.ProjectDir), 0644); err != nil {
			return fmt.Errorf("failed to write project path file: %w", err)
		}
	}

	return nil
}

// LoadConfig loads the project configuration.
func (a *App) LoadConfig() error {
	cfg, err := config.Load(a.PawDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	for _, warning := range cfg.Normalize() {
		logging.Warn("config: %s", warning)
	}
	a.Config = cfg
	return nil
}

// IsInitialized checks if the .paw directory exists.
func (a *App) IsInitialized() bool {
	_, err := os.Stat(a.PawDir)
	return err == nil
}

// HasConfig checks if a configuration file exists.
func (a *App) HasConfig() bool {
	return config.Exists(a.PawDir)
}

// GetLogPath returns the path to the unified log file.
func (a *App) GetLogPath() string {
	return filepath.Join(a.PawDir, constants.LogFileName)
}

// GetHistoryDir returns the path to the history directory.
func (a *App) GetHistoryDir() string {
	return filepath.Join(a.PawDir, constants.HistoryDirName)
}

// GetPromptPath returns the path to the project-specific prompt file.
func (a *App) GetPromptPath() string {
	return filepath.Join(a.PawDir, constants.PromptFileName)
}

// GetGlobalPromptPath returns the path to the global prompt symlink.
func (a *App) GetGlobalPromptPath() string {
	return filepath.Join(a.PawDir, constants.GlobalPromptLink)
}

// GetAgentDir returns the path to a specific agent's directory.
func (a *App) GetAgentDir(taskName string) string {
	return filepath.Join(a.AgentsDir, taskName)
}

// SetPawHome sets the PAW installation directory.
func (a *App) SetPawHome(path string) {
	a.PawHome = path
}

// SetGitRepo sets whether the project is a git repository.
func (a *App) SetGitRepo(isGit bool) {
	a.IsGitRepo = isGit
}

// IsWorktreeMode returns true if the app is configured to use git worktrees.
// PAW always uses worktree mode for git repositories.
func (a *App) IsWorktreeMode() bool {
	return a.IsGitRepo
}

// IsGlobalWorkspace returns true if the workspace is stored in the global location
// (not inside the project directory).
func (a *App) IsGlobalWorkspace() bool {
	localPawDir := filepath.Join(a.ProjectDir, constants.PawDirName)
	return a.PawDir != localPawDir
}

// UpdateSessionNameForGitRepo updates the session name to include repo name
// when the project is in a subdirectory of a git repository.
// Format: {repo-name}:{dir-name} when project is in a subdirectory
// Format: {repo-name} when project is at repo root
func (a *App) UpdateSessionNameForGitRepo(repoRoot string) {
	repoName := filepath.Base(repoRoot)
	dirName := filepath.Base(a.ProjectDir)

	// If project is in a subdirectory of the repo, show both
	if repoRoot != a.ProjectDir {
		a.SessionName = repoName + ":" + dirName
	} else {
		// At repo root, just use the repo name
		a.SessionName = repoName
	}
}

// SetSubdirectoryContext sets the display name and session name when running
// from a subdirectory of a git repository.
// DisplayName format: {repo-name}/{subdir-name} (for UI display)
// SessionName format: {repo-name}-{subdir-name} (tmux-safe, no special chars)
// When at repo root, both use just {repo-name}.
func (a *App) SetSubdirectoryContext(originalCwd, repoRoot string) {
	repoName := filepath.Base(repoRoot)

	// If original cwd is different from repo root, show context
	if originalCwd != repoRoot {
		subdirName := filepath.Base(originalCwd)
		// DisplayName with slash (for UI display)
		a.DisplayName = repoName + "/" + subdirName
		// SessionName with dash (for tmux - no special chars like / or :)
		a.SessionName = repoName + "-" + subdirName
	} else {
		// At repo root, just use the repo name
		a.DisplayName = repoName
		// SessionName stays as repo name (already set in NewWithGitInfo)
	}
}

// GetDisplayName returns the display name for UI.
// Falls back to SessionName if DisplayName is not set.
func (a *App) GetDisplayName() string {
	if a.DisplayName != "" {
		return a.DisplayName
	}
	return a.SessionName
}

// pawEnvVars lists environment variables managed by PAW.
// These are filtered from os.Environ() before adding new values to prevent duplicates.
var pawEnvVars = []string{
	"TASK_NAME",
	"PAW_DIR",
	"PROJECT_DIR",
	"WINDOW_ID",
	"PAW_HOME",
	"SESSION_NAME",
	"ON_COMPLETE",
	"WORKTREE_DIR",
	"PAW_LOG_FORMAT",
	"PAW_LOG_MAX_SIZE_MB",
	"PAW_LOG_MAX_BACKUPS",
}

// GetEnvVars returns environment variables to be passed to Claude.
func (a *App) GetEnvVars(taskName, worktreeDir, windowID string) []string {
	// Filter out existing PAW env vars to prevent duplicates
	env := filterEnv(os.Environ(), pawEnvVars)

	env = append(env,
		"TASK_NAME="+taskName,
		"PAW_DIR="+a.PawDir,
		"PROJECT_DIR="+a.ProjectDir,
		"WINDOW_ID="+windowID,
		"PAW_HOME="+a.PawHome,
		"SESSION_NAME="+a.SessionName,
	)

	if a.Config != nil {
		env = append(env,
			"PAW_LOG_FORMAT="+a.Config.LogFormat,
			"PAW_LOG_MAX_SIZE_MB="+fmt.Sprintf("%d", a.Config.LogMaxSizeMB),
			"PAW_LOG_MAX_BACKUPS="+fmt.Sprintf("%d", a.Config.LogMaxBackups),
		)
	}

	if worktreeDir != "" {
		env = append(env, "WORKTREE_DIR="+worktreeDir)
	}

	return env
}

// filterEnv removes specified environment variables from the list.
func filterEnv(env []string, exclude []string) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		excluded := false
		for _, prefix := range exclude {
			if strings.HasPrefix(e, prefix+"=") {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, e)
		}
	}
	return result
}
