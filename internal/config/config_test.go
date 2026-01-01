package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donghojung/taw/internal/constants"
)

func TestParseConfig_SingleLineHook(t *testing.T) {
	content := `work_mode: worktree
on_complete: confirm
worktree_hook: npm install
`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.WorktreeHook != "npm install" {
		t.Errorf("expected 'npm install', got '%s'", cfg.WorktreeHook)
	}
}

func TestParseConfig_MultiLineHook(t *testing.T) {
	content := `work_mode: worktree
on_complete: confirm
worktree_hook: |
  npm install
  npm run build
`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	expected := "npm install\nnpm run build"
	if cfg.WorktreeHook != expected {
		t.Errorf("expected %q, got %q", expected, cfg.WorktreeHook)
	}
}

func TestParseConfig_MultiLineHookWithEmptyLines(t *testing.T) {
	content := `work_mode: worktree
worktree_hook: |
  npm install

  npm run build
on_complete: confirm
`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	expected := "npm install\n\nnpm run build"
	if cfg.WorktreeHook != expected {
		t.Errorf("expected %q, got %q", expected, cfg.WorktreeHook)
	}
	if cfg.OnComplete != OnCompleteConfirm {
		t.Errorf("expected on_complete to be 'confirm', got %s", cfg.OnComplete)
	}
}

func TestFormatWorktreeHook_SingleLine(t *testing.T) {
	result := formatWorktreeHook("npm install")
	expected := "worktree_hook: npm install\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatWorktreeHook_MultiLine(t *testing.T) {
	result := formatWorktreeHook("npm install\nnpm run build")
	expected := "worktree_hook: |\n  npm install\n  npm run build\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRoundTrip_SingleLineHook(t *testing.T) {
	hook := "npm install"
	formatted := formatWorktreeHook(hook)

	// Prepend with required fields
	content := "work_mode: worktree\non_complete: confirm\n" + formatted

	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.WorktreeHook != hook {
		t.Errorf("roundtrip failed: expected %q, got %q", hook, cfg.WorktreeHook)
	}
}

func TestRoundTrip_MultiLineHook(t *testing.T) {
	hook := "npm install\nnpm run build"
	formatted := formatWorktreeHook(hook)

	// Prepend with required fields
	content := "work_mode: worktree\non_complete: confirm\n" + formatted

	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.WorktreeHook != hook {
		t.Errorf("roundtrip failed: expected %q, got %q", hook, cfg.WorktreeHook)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.WorkMode != WorkModeWorktree {
		t.Errorf("WorkMode = %q, want %q", cfg.WorkMode, WorkModeWorktree)
	}
	if cfg.OnComplete != OnCompleteConfirm {
		t.Errorf("OnComplete = %q, want %q", cfg.OnComplete, OnCompleteConfirm)
	}
	if cfg.WorktreeHook != "" {
		t.Errorf("WorktreeHook = %q, want empty", cfg.WorktreeHook)
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	tempDir := t.TempDir()

	cfg, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should return default config
	if cfg.WorkMode != WorkModeWorktree {
		t.Errorf("WorkMode = %q, want %q", cfg.WorkMode, WorkModeWorktree)
	}
	if cfg.OnComplete != OnCompleteConfirm {
		t.Errorf("OnComplete = %q, want %q", cfg.OnComplete, OnCompleteConfirm)
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, constants.ConfigFileName)

	content := `work_mode: main
on_complete: auto-merge
worktree_hook: npm install
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.WorkMode != WorkModeMain {
		t.Errorf("WorkMode = %q, want %q", cfg.WorkMode, WorkModeMain)
	}
	if cfg.OnComplete != OnCompleteAutoMerge {
		t.Errorf("OnComplete = %q, want %q", cfg.OnComplete, OnCompleteAutoMerge)
	}
	if cfg.WorktreeHook != "npm install" {
		t.Errorf("WorktreeHook = %q, want %q", cfg.WorktreeHook, "npm install")
	}
}

func TestSave(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		WorkMode:     WorkModeWorktree,
		OnComplete:   OnCompleteAutoPR,
		WorktreeHook: "pnpm install",
	}

	if err := cfg.Save(tempDir); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load and verify
	loaded, err := Load(tempDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.WorkMode != cfg.WorkMode {
		t.Errorf("WorkMode = %q, want %q", loaded.WorkMode, cfg.WorkMode)
	}
	if loaded.OnComplete != cfg.OnComplete {
		t.Errorf("OnComplete = %q, want %q", loaded.OnComplete, cfg.OnComplete)
	}
	if loaded.WorktreeHook != cfg.WorktreeHook {
		t.Errorf("WorktreeHook = %q, want %q", loaded.WorktreeHook, cfg.WorktreeHook)
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

func TestValidWorkModes(t *testing.T) {
	modes := ValidWorkModes()

	if len(modes) != 2 {
		t.Errorf("Expected 2 work modes, got %d", len(modes))
	}

	expected := map[WorkMode]bool{
		WorkModeWorktree: true,
		WorkModeMain:     true,
	}

	for _, mode := range modes {
		if !expected[mode] {
			t.Errorf("Unexpected work mode: %q", mode)
		}
	}
}

func TestValidOnCompletes(t *testing.T) {
	completes := ValidOnCompletes()

	if len(completes) != 4 {
		t.Errorf("Expected 4 on_complete options, got %d", len(completes))
	}

	expected := map[OnComplete]bool{
		OnCompleteConfirm:    true,
		OnCompleteAutoCommit: true,
		OnCompleteAutoMerge:  true,
		OnCompleteAutoPR:     true,
	}

	for _, complete := range completes {
		if !expected[complete] {
			t.Errorf("Unexpected on_complete: %q", complete)
		}
	}
}

func TestParseConfig_Comments(t *testing.T) {
	content := `# This is a comment
work_mode: worktree
# Another comment
on_complete: confirm
`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.WorkMode != WorkModeWorktree {
		t.Errorf("WorkMode = %q, want %q", cfg.WorkMode, WorkModeWorktree)
	}
	if cfg.OnComplete != OnCompleteConfirm {
		t.Errorf("OnComplete = %q, want %q", cfg.OnComplete, OnCompleteConfirm)
	}
}

func TestParseConfig_EmptyLines(t *testing.T) {
	content := `
work_mode: worktree

on_complete: confirm

`
	cfg, err := parseConfig(content)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.WorkMode != WorkModeWorktree {
		t.Errorf("WorkMode = %q, want %q", cfg.WorkMode, WorkModeWorktree)
	}
	if cfg.OnComplete != OnCompleteConfirm {
		t.Errorf("OnComplete = %q, want %q", cfg.OnComplete, OnCompleteConfirm)
	}
}

func TestParseConfig_AllOnCompleteValues(t *testing.T) {
	tests := []struct {
		value    string
		expected OnComplete
	}{
		{"confirm", OnCompleteConfirm},
		{"auto-commit", OnCompleteAutoCommit},
		{"auto-merge", OnCompleteAutoMerge},
		{"auto-pr", OnCompleteAutoPR},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			content := "work_mode: worktree\non_complete: " + tt.value + "\n"
			cfg, err := parseConfig(content)
			if err != nil {
				t.Fatalf("parseConfig failed: %v", err)
			}
			if cfg.OnComplete != tt.expected {
				t.Errorf("OnComplete = %q, want %q", cfg.OnComplete, tt.expected)
			}
		})
	}
}
