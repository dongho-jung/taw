package config

import "testing"

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
