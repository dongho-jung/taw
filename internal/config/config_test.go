package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dongho-jung/paw/internal/constants"
)

func TestParseConfig_SingleLineHook(t *testing.T) {
	content := `work_mode: worktree
pre_worktree_hook: npm install
`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.PreWorktreeHook != "npm install" {
		t.Errorf("expected 'npm install', got '%s'", cfg.PreWorktreeHook)
	}
}

func TestParseConfig_MultiLineHook(t *testing.T) {
	content := `work_mode: worktree
pre_worktree_hook: |
  npm install
  npm run build
`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	expected := "npm install\nnpm run build"
	if cfg.PreWorktreeHook != expected {
		t.Errorf("expected %q, got %q", expected, cfg.PreWorktreeHook)
	}
}

func TestParseConfig_MultiLineHookWithEmptyLines(t *testing.T) {
	content := `work_mode: worktree
pre_worktree_hook: |
  npm install

  npm run build
`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	expected := "npm install\n\nnpm run build"
	if cfg.PreWorktreeHook != expected {
		t.Errorf("expected %q, got %q", expected, cfg.PreWorktreeHook)
	}
}

func TestFormatHook_SingleLine(t *testing.T) {
	result := formatHook("pre_worktree_hook", "npm install")
	expected := "pre_worktree_hook: npm install\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatHook_MultiLine(t *testing.T) {
	result := formatHook("pre_worktree_hook", "npm install\nnpm run build")
	expected := "pre_worktree_hook: |\n  npm install\n  npm run build\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRoundTrip_SingleLineHook(t *testing.T) {
	hook := "npm install"
	formatted := formatHook("pre_worktree_hook", hook)

	// Prepend with required fields
	content := "work_mode: worktree\n" + formatted

	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.PreWorktreeHook != hook {
		t.Errorf("roundtrip failed: expected %q, got %q", hook, cfg.PreWorktreeHook)
	}
}

func TestRoundTrip_MultiLineHook(t *testing.T) {
	hook := "npm install\nnpm run build"
	formatted := formatHook("pre_worktree_hook", hook)

	// Prepend with required fields
	content := "work_mode: worktree\n" + formatted

	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.PreWorktreeHook != hook {
		t.Errorf("roundtrip failed: expected %q, got %q", hook, cfg.PreWorktreeHook)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.PreWorktreeHook != "" {
		t.Errorf("PreWorktreeHook = %q, want empty", cfg.PreWorktreeHook)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "text")
	}
	if cfg.LogMaxSizeMB != 10 {
		t.Errorf("LogMaxSizeMB = %d, want 10", cfg.LogMaxSizeMB)
	}
	if cfg.LogMaxBackups != 3 {
		t.Errorf("LogMaxBackups = %d, want 3", cfg.LogMaxBackups)
	}
}

func TestConfigNormalize_InvalidLogFormat(t *testing.T) {
	cfg := &Config{
		LogFormat: "invalid",
	}

	warnings := cfg.Normalize()

	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "text")
	}
	if len(warnings) != 1 {
		t.Errorf("warnings len = %d, want 1", len(warnings))
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	tempDir := t.TempDir()

	cfg, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should return default config
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "text")
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, constants.ConfigFileName)

	content := `pre_worktree_hook: npm install
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PreWorktreeHook != "npm install" {
		t.Errorf("PreWorktreeHook = %q, want %q", cfg.PreWorktreeHook, "npm install")
	}
}

func TestSave(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		PreWorktreeHook: "pnpm install",
	}

	if err := cfg.Save(tempDir); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load and verify
	loaded, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.PreWorktreeHook != cfg.PreWorktreeHook {
		t.Errorf("PreWorktreeHook = %q, want %q", loaded.PreWorktreeHook, cfg.PreWorktreeHook)
	}
}

func TestExists(t *testing.T) {
	tempDir := t.TempDir()

	// Should not exist initially
	if Exists(tempDir) {
		t.Error("Exists() = true, want false initially")
	}

	// Create config file
	configPath := filepath.Join(tempDir, constants.ConfigFileName)
	if err := os.WriteFile(configPath, []byte("work_mode: worktree\n"), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Should exist now
	if !Exists(tempDir) {
		t.Error("Exists() = false, want true after creating config")
	}
}

func TestParseConfig_Comments(t *testing.T) {
	content := `# This is a comment
pre_worktree_hook: npm install
# Another comment
`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.PreWorktreeHook != "npm install" {
		t.Errorf("PreWorktreeHook = %q, want %q", cfg.PreWorktreeHook, "npm install")
	}
}

func TestParseConfig_EmptyLines(t *testing.T) {
	content := `
pre_worktree_hook: npm install

`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.PreWorktreeHook != "npm install" {
		t.Errorf("PreWorktreeHook = %q, want %q", cfg.PreWorktreeHook, "npm install")
	}
}

func TestParseConfig_PawInProject(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected PawInProject
	}{
		{
			name:     "paw_in_project auto",
			content:  "paw_in_project: auto\n",
			expected: PawInProjectAuto,
		},
		{
			name:     "paw_in_project global",
			content:  "paw_in_project: global\n",
			expected: PawInProjectGlobal,
		},
		{
			name:     "paw_in_project local",
			content:  "paw_in_project: local\n",
			expected: PawInProjectLocal,
		},
		{
			name:     "paw_in_project true (legacy)",
			content:  "paw_in_project: true\n",
			expected: PawInProjectLocal, // legacy true = local
		},
		{
			name:     "paw_in_project false (legacy)",
			content:  "paw_in_project: false\n",
			expected: PawInProjectGlobal, // legacy false = global
		},
		{
			name:     "paw_in_project not set",
			content:  "",
			expected: PawInProjectAuto, // default is auto
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseConfig(tt.content)
			if err != nil {
				t.Fatalf("parseConfig failed: %v", err)
			}
			if cfg.PawInProject != tt.expected {
				t.Errorf("PawInProject = %v, want %v", cfg.PawInProject, tt.expected)
			}
		})
	}
}

func TestGlobalWorkspacesDir(t *testing.T) {
	dir := GlobalWorkspacesDir()
	if dir == "" {
		t.Skip("Could not determine home directory")
	}

	// Should be under ~/.local/share/paw/workspaces
	if !strings.Contains(dir, ".local/share/paw/workspaces") {
		t.Errorf("GlobalWorkspacesDir() = %q, expected to contain '.local/share/paw/workspaces'", dir)
	}
}

func TestProjectWorkspaceID(t *testing.T) {
	tests := []struct {
		projectDir string
	}{
		{"/home/user/myproject"},
		{"/tmp/test-project"},
		{"/Users/dev/projects/paw"},
	}

	for _, tt := range tests {
		t.Run(tt.projectDir, func(t *testing.T) {
			id := ProjectWorkspaceID(tt.projectDir)

			// ID should not be empty
			if id == "" {
				t.Error("ProjectWorkspaceID() returned empty string")
			}

			// ID should contain project name
			base := filepath.Base(tt.projectDir)
			if !strings.HasPrefix(id, base) {
				t.Errorf("ProjectWorkspaceID(%q) = %q, expected to start with %q", tt.projectDir, id, base)
			}

			// ID should have a hash suffix
			if !strings.Contains(id, "-") {
				t.Errorf("ProjectWorkspaceID(%q) = %q, expected to contain hash separator", tt.projectDir, id)
			}

			// Same input should produce same output
			id2 := ProjectWorkspaceID(tt.projectDir)
			if id != id2 {
				t.Errorf("ProjectWorkspaceID() not stable: got %q and %q", id, id2)
			}
		})
	}
}

func TestGetWorkspaceDir_LocalPriority(t *testing.T) {
	tempDir := t.TempDir()

	// Create local .paw directory
	localPawDir := filepath.Join(tempDir, constants.PawDirName)
	if err := os.MkdirAll(localPawDir, 0755); err != nil {
		t.Fatalf("Failed to create local .paw: %v", err)
	}

	// Even with pawInProject=global, local should take priority if exists
	result := GetWorkspaceDir(tempDir, PawInProjectGlobal, true)
	if result != localPawDir {
		t.Errorf("GetWorkspaceDir() = %q, want %q (local should take priority)", result, localPawDir)
	}
}

func TestGetWorkspaceDir_GlobalLocation(t *testing.T) {
	tempDir := t.TempDir()

	// No local .paw, pawInProject=global -> global workspace
	result := GetWorkspaceDir(tempDir, PawInProjectGlobal, true)

	globalDir := GlobalWorkspacesDir()
	if !strings.HasPrefix(result, globalDir) {
		t.Errorf("GetWorkspaceDir() = %q, expected to be under %q", result, globalDir)
	}
}

func TestGetWorkspaceDir_LocalLocation(t *testing.T) {
	tempDir := t.TempDir()

	// No local .paw, but pawInProject=local -> local
	result := GetWorkspaceDir(tempDir, PawInProjectLocal, true)

	expectedLocal := filepath.Join(tempDir, constants.PawDirName)
	if result != expectedLocal {
		t.Errorf("GetWorkspaceDir() = %q, want %q", result, expectedLocal)
	}
}

func TestGetWorkspaceDir_AutoGitRepo(t *testing.T) {
	tempDir := t.TempDir()

	// No local .paw, pawInProject=auto, isGitRepo=true -> global workspace
	result := GetWorkspaceDir(tempDir, PawInProjectAuto, true)

	globalDir := GlobalWorkspacesDir()
	if !strings.HasPrefix(result, globalDir) {
		t.Errorf("GetWorkspaceDir() = %q, expected to be under %q for git repo with auto mode", result, globalDir)
	}
}

func TestGetWorkspaceDir_AutoNonGit(t *testing.T) {
	tempDir := t.TempDir()

	// No local .paw, pawInProject=auto, isGitRepo=false -> local workspace
	result := GetWorkspaceDir(tempDir, PawInProjectAuto, false)

	expectedLocal := filepath.Join(tempDir, constants.PawDirName)
	if result != expectedLocal {
		t.Errorf("GetWorkspaceDir() = %q, want %q for non-git with auto mode", result, expectedLocal)
	}
}

func TestSave_PawInProject(t *testing.T) {
	tests := []struct {
		name string
		mode PawInProject
	}{
		{"auto", PawInProjectAuto},
		{"global", PawInProjectGlobal},
		{"local", PawInProjectLocal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			cfg := &Config{
				PawInProject: tt.mode,
			}

			if err := cfg.Save(tempDir); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			// Load and verify
			loaded, err := Load(tempDir)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if loaded.PawInProject != cfg.PawInProject {
				t.Errorf("PawInProject = %v, want %v", loaded.PawInProject, cfg.PawInProject)
			}
		})
	}
}
