// Package app provides the main application context and dependency injection.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donghojung/taw/internal/config"
	"github.com/donghojung/taw/internal/constants"
)

// App represents the main application context with all dependencies.
type App struct {
	// Paths
	ProjectDir string // Root directory of the user's project
	TawDir     string // .taw directory path
	AgentsDir  string // agents directory path
	TawHome    string // TAW installation directory

	// Session
	SessionName string // tmux session name

	// State
	IsGitRepo bool           // Whether the project is a git repository
	Config    *config.Config // Project configuration

	// Runtime
	Debug bool // Debug mode enabled
}

// New creates a new App instance for the given project directory.
func New(projectDir string) (*App, error) {
	absPath, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project path: %w", err)
	}

	tawDir := filepath.Join(absPath, constants.TawDirName)
	agentsDir := filepath.Join(tawDir, constants.AgentsDirName)

	// Determine session name from project directory name
	sessionName := filepath.Base(absPath)

	// Check if debug mode is enabled
	debug := os.Getenv("TAW_DEBUG") == "1"

	app := &App{
		ProjectDir:  absPath,
		TawDir:      tawDir,
		AgentsDir:   agentsDir,
		SessionName: sessionName,
		Debug:       debug,
	}

	return app, nil
}

// Initialize sets up the .taw directory structure.
func (a *App) Initialize() error {
	// Create directories
	dirs := []string{
		a.TawDir,
		a.AgentsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	if err := ensureMemoryFile(filepath.Join(a.TawDir, constants.MemoryFileName)); err != nil {
		return fmt.Errorf("failed to create memory file: %w", err)
	}

	return nil
}

// LoadConfig loads the project configuration.
func (a *App) LoadConfig() error {
	cfg, err := config.Load(a.TawDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	a.Config = cfg
	return nil
}

// IsInitialized checks if the .taw directory exists.
func (a *App) IsInitialized() bool {
	_, err := os.Stat(a.TawDir)
	return err == nil
}

// HasConfig checks if a configuration file exists.
func (a *App) HasConfig() bool {
	return config.Exists(a.TawDir)
}

// GetLogPath returns the path to the unified log file.
func (a *App) GetLogPath() string {
	return filepath.Join(a.TawDir, constants.LogFileName)
}

// GetHistoryDir returns the path to the history directory.
func (a *App) GetHistoryDir() string {
	return filepath.Join(a.TawDir, constants.HistoryDirName)
}

// GetPromptPath returns the path to the project-specific prompt file.
func (a *App) GetPromptPath() string {
	return filepath.Join(a.TawDir, constants.PromptFileName)
}

// GetGlobalPromptPath returns the path to the global prompt symlink.
func (a *App) GetGlobalPromptPath() string {
	return filepath.Join(a.TawDir, constants.GlobalPromptLink)
}

// GetAgentDir returns the path to a specific agent's directory.
func (a *App) GetAgentDir(taskName string) string {
	return filepath.Join(a.AgentsDir, taskName)
}

// SetTawHome sets the TAW installation directory.
func (a *App) SetTawHome(path string) {
	a.TawHome = path
}

// SetGitRepo sets whether the project is a git repository.
func (a *App) SetGitRepo(isGit bool) {
	a.IsGitRepo = isGit
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

// tawEnvVars lists environment variables managed by TAW.
// These are filtered from os.Environ() before adding new values to prevent duplicates.
var tawEnvVars = []string{
	"TASK_NAME",
	"TAW_DIR",
	"PROJECT_DIR",
	"WINDOW_ID",
	"TAW_HOME",
	"SESSION_NAME",
	"ON_COMPLETE",
	"WORKTREE_DIR",
}

// GetEnvVars returns environment variables to be passed to Claude.
func (a *App) GetEnvVars(taskName, worktreeDir, windowID string) []string {
	// Filter out existing TAW env vars to prevent duplicates
	env := filterEnv(os.Environ(), tawEnvVars)

	env = append(env,
		"TASK_NAME="+taskName,
		"TAW_DIR="+a.TawDir,
		"PROJECT_DIR="+a.ProjectDir,
		"WINDOW_ID="+windowID,
		"TAW_HOME="+a.TawHome,
		"SESSION_NAME="+a.SessionName,
	)

	if a.Config != nil {
		env = append(env, "ON_COMPLETE="+string(a.Config.OnComplete))
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
