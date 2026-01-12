package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
)

func TestNew(t *testing.T) {
	tempDir := t.TempDir()

	// Create local .paw directory so it takes priority over global workspace
	localPawDir := filepath.Join(tempDir, constants.PawDirName)
	if err := os.MkdirAll(localPawDir, 0755); err != nil {
		t.Fatalf("Failed to create local .paw directory: %v", err)
	}

	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if app.ProjectDir != tempDir {
		t.Errorf("ProjectDir = %q, want %q", app.ProjectDir, tempDir)
	}

	expectedPawDir := filepath.Join(tempDir, constants.PawDirName)
	if app.PawDir != expectedPawDir {
		t.Errorf("PawDir = %q, want %q", app.PawDir, expectedPawDir)
	}

	expectedAgentsDir := filepath.Join(expectedPawDir, constants.AgentsDirName)
	if app.AgentsDir != expectedAgentsDir {
		t.Errorf("AgentsDir = %q, want %q", app.AgentsDir, expectedAgentsDir)
	}

	expectedSessionName := filepath.Base(tempDir)
	if app.SessionName != expectedSessionName {
		t.Errorf("SessionName = %q, want %q", app.SessionName, expectedSessionName)
	}
}

func TestNewWithGlobalWorkspace(t *testing.T) {
	tempDir := t.TempDir()

	// Without local .paw, should use global workspace location (default paw_in_project: false)
	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if app.ProjectDir != tempDir {
		t.Errorf("ProjectDir = %q, want %q", app.ProjectDir, tempDir)
	}

	// PawDir should be in global workspace location
	globalWorkspacesDir := config.GlobalWorkspacesDir()
	if !strings.HasPrefix(app.PawDir, globalWorkspacesDir) {
		t.Errorf("PawDir = %q, expected to be under %q", app.PawDir, globalWorkspacesDir)
	}

	// AgentsDir should be under PawDir
	expectedAgentsDir := filepath.Join(app.PawDir, constants.AgentsDirName)
	if app.AgentsDir != expectedAgentsDir {
		t.Errorf("AgentsDir = %q, want %q", app.AgentsDir, expectedAgentsDir)
	}
}

func TestAppInitialize(t *testing.T) {
	tempDir := t.TempDir()

	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Check directories were created
	dirs := []string{app.PawDir, app.AgentsDir}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Directory %q was not created: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("%q is not a directory", dir)
		}
	}

	// Check memory file was created
	memoryPath := filepath.Join(app.PawDir, constants.MemoryFileName)
	if _, err := os.Stat(memoryPath); err != nil {
		t.Errorf("Memory file was not created: %v", err)
	}
}

func TestAppIsInitialized(t *testing.T) {
	tempDir := t.TempDir()

	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Not initialized yet
	if app.IsInitialized() {
		t.Error("IsInitialized() = true, want false before Initialize()")
	}

	// Initialize
	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Now initialized
	if !app.IsInitialized() {
		t.Error("IsInitialized() = false, want true after Initialize()")
	}
}

func TestAppHasConfig(t *testing.T) {
	tempDir := t.TempDir()

	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// No config yet
	if app.HasConfig() {
		t.Error("HasConfig() = true, want false before config is created")
	}

	// Create config file
	configPath := filepath.Join(app.PawDir, constants.ConfigFileName)
	if err := os.WriteFile(configPath, []byte("work_mode: worktree\n"), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Now has config
	if !app.HasConfig() {
		t.Error("HasConfig() = false, want true after config is created")
	}
}

func TestAppGetPaths(t *testing.T) {
	tempDir := t.TempDir()

	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test GetLogPath
	expectedLogPath := filepath.Join(app.PawDir, constants.LogFileName)
	if app.GetLogPath() != expectedLogPath {
		t.Errorf("GetLogPath() = %q, want %q", app.GetLogPath(), expectedLogPath)
	}

	// Test GetHistoryDir
	expectedHistoryDir := filepath.Join(app.PawDir, constants.HistoryDirName)
	if app.GetHistoryDir() != expectedHistoryDir {
		t.Errorf("GetHistoryDir() = %q, want %q", app.GetHistoryDir(), expectedHistoryDir)
	}

	// Test GetPromptPath
	expectedPromptPath := filepath.Join(app.PawDir, constants.PromptFileName)
	if app.GetPromptPath() != expectedPromptPath {
		t.Errorf("GetPromptPath() = %q, want %q", app.GetPromptPath(), expectedPromptPath)
	}

	// Test GetGlobalPromptPath
	expectedGlobalPromptPath := filepath.Join(app.PawDir, constants.GlobalPromptLink)
	if app.GetGlobalPromptPath() != expectedGlobalPromptPath {
		t.Errorf("GetGlobalPromptPath() = %q, want %q", app.GetGlobalPromptPath(), expectedGlobalPromptPath)
	}

	// Test GetAgentDir
	taskName := "test-task"
	expectedAgentDir := filepath.Join(app.AgentsDir, taskName)
	if app.GetAgentDir(taskName) != expectedAgentDir {
		t.Errorf("GetAgentDir(%q) = %q, want %q", taskName, app.GetAgentDir(taskName), expectedAgentDir)
	}
}

func TestAppSetters(t *testing.T) {
	tempDir := t.TempDir()

	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test SetPawHome
	pawHome := "/usr/local/paw"
	app.SetPawHome(pawHome)
	if app.PawHome != pawHome {
		t.Errorf("PawHome = %q, want %q", app.PawHome, pawHome)
	}

	// Test SetGitRepo
	app.SetGitRepo(true)
	if !app.IsGitRepo {
		t.Error("IsGitRepo = false, want true")
	}

	app.SetGitRepo(false)
	if app.IsGitRepo {
		t.Error("IsGitRepo = true, want false")
	}
}

func TestAppUpdateSessionNameForGitRepo(t *testing.T) {
	tests := []struct {
		name        string
		projectDir  string
		repoRoot    string
		wantSession string
	}{
		{
			name:        "project at repo root",
			projectDir:  "/home/user/myrepo",
			repoRoot:    "/home/user/myrepo",
			wantSession: "myrepo",
		},
		{
			name:        "project in subdirectory",
			projectDir:  "/home/user/myrepo/packages/frontend",
			repoRoot:    "/home/user/myrepo",
			wantSession: "myrepo:frontend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				ProjectDir:  tt.projectDir,
				SessionName: filepath.Base(tt.projectDir),
			}

			app.UpdateSessionNameForGitRepo(tt.repoRoot)

			if app.SessionName != tt.wantSession {
				t.Errorf("SessionName = %q, want %q", app.SessionName, tt.wantSession)
			}
		})
	}
}

func TestAppGetEnvVars(t *testing.T) {
	tempDir := t.TempDir()

	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	app.PawHome = "/usr/local/paw"

	taskName := "my-task"
	worktreeDir := "/path/to/worktree"
	windowID := "@1"

	envVars := app.GetEnvVars(taskName, worktreeDir, windowID)

	// Check required env vars are present
	required := map[string]string{
		"TASK_NAME":    taskName,
		"PAW_DIR":      app.PawDir,
		"PROJECT_DIR":  app.ProjectDir,
		"WINDOW_ID":    windowID,
		"PAW_HOME":     app.PawHome,
		"SESSION_NAME": app.SessionName,
		"WORKTREE_DIR": worktreeDir,
	}

	for key, wantValue := range required {
		found := false
		for _, env := range envVars {
			if strings.HasPrefix(env, key+"=") {
				found = true
				value := strings.TrimPrefix(env, key+"=")
				if value != wantValue {
					t.Errorf("env %s = %q, want %q", key, value, wantValue)
				}
				break
			}
		}
		if !found {
			t.Errorf("env %s not found in envVars", key)
		}
	}
}

func TestAppGetEnvVarsWithoutWorktree(t *testing.T) {
	tempDir := t.TempDir()

	app, err := New(tempDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	envVars := app.GetEnvVars("task", "", "@1")

	// WORKTREE_DIR should not be present when empty
	for _, env := range envVars {
		if strings.HasPrefix(env, "WORKTREE_DIR=") {
			t.Error("WORKTREE_DIR should not be present when worktreeDir is empty")
		}
	}
}

func TestEnsureMemoryFile(t *testing.T) {
	tempDir := t.TempDir()
	memoryPath := filepath.Join(tempDir, "memory")

	// First call should create the file
	if err := ensureMemoryFile(memoryPath); err != nil {
		t.Fatalf("ensureMemoryFile() error = %v", err)
	}

	// File should exist
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("Memory file not created: %v", err)
	}

	if !strings.Contains(string(data), "PAW Memory") {
		t.Errorf("Memory file should contain template content")
	}

	// Modify the file
	if err := os.WriteFile(memoryPath, []byte("custom content"), 0644); err != nil {
		t.Fatalf("Failed to modify memory file: %v", err)
	}

	// Second call should not overwrite
	if err := ensureMemoryFile(memoryPath); err != nil {
		t.Fatalf("ensureMemoryFile() error = %v", err)
	}

	data, err = os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("Failed to read memory file: %v", err)
	}

	if string(data) != "custom content" {
		t.Errorf("Memory file was overwritten, got: %s", string(data))
	}
}

func TestAppIsWorktreeMode(t *testing.T) {
	tests := []struct {
		name      string
		isGitRepo bool
		config    *config.Config
		want      bool
	}{
		{
			name:      "worktree mode enabled",
			isGitRepo: true,
			config:    &config.Config{WorkMode: config.WorkModeWorktree},
			want:      true,
		},
		{
			name:      "main mode",
			isGitRepo: true,
			config:    &config.Config{WorkMode: config.WorkModeMain},
			want:      false,
		},
		{
			name:      "not a git repo",
			isGitRepo: false,
			config:    &config.Config{WorkMode: config.WorkModeWorktree},
			want:      false,
		},
		{
			name:      "nil config",
			isGitRepo: true,
			config:    nil,
			want:      false,
		},
		{
			name:      "not git repo with nil config",
			isGitRepo: false,
			config:    nil,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				IsGitRepo: tt.isGitRepo,
				Config:    tt.config,
			}

			if got := app.IsWorktreeMode(); got != tt.want {
				t.Errorf("IsWorktreeMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
