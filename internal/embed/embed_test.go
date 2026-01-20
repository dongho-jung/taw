package embed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetHelp(t *testing.T) {
	content, err := GetHelp()
	if err != nil {
		t.Fatalf("GetHelp() error = %v", err)
	}

	if content == "" {
		t.Error("GetHelp() returned empty content")
	}

	// Help file should contain some expected content
	if !strings.Contains(content, "PAW") && !strings.Contains(content, "paw") {
		t.Error("Help content should mention PAW/paw")
	}
}

func TestGetPromptGit(t *testing.T) {
	content, err := GetPrompt(true)
	if err != nil {
		t.Fatalf("GetPrompt(true) error = %v", err)
	}

	if content == "" {
		t.Error("GetPrompt(true) returned empty content")
	}
}

func TestGetPromptNoGit(t *testing.T) {
	content, err := GetPrompt(false)
	if err != nil {
		t.Fatalf("GetPrompt(false) error = %v", err)
	}

	if content == "" {
		t.Error("GetPrompt(false) returned empty content")
	}
}

func TestWriteClaudeFiles(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, ".claude")

	if err := WriteClaudeFiles(targetDir); err != nil {
		t.Fatalf("WriteClaudeFiles() error = %v", err)
	}

	// Check that target directory was created
	info, err := os.Stat(targetDir)
	if err != nil {
		t.Fatalf("Target directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Target path is not a directory")
	}

	// Check that some files exist
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		t.Fatalf("Failed to read target directory: %v", err)
	}

	if len(entries) == 0 {
		t.Error("No files were written to target directory")
	}
}

func TestWriteClaudeFilesOverwrites(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, ".claude")

	// First write
	if err := WriteClaudeFiles(targetDir); err != nil {
		t.Fatalf("First WriteClaudeFiles() error = %v", err)
	}

	// Second write (should not error)
	if err := WriteClaudeFiles(targetDir); err != nil {
		t.Fatalf("Second WriteClaudeFiles() error = %v", err)
	}
}

func TestAssetsFS(t *testing.T) {
	// Test that we can read from the embedded filesystem
	entries, err := Assets.ReadDir("assets")
	if err != nil {
		t.Fatalf("Failed to read assets directory: %v", err)
	}

	if len(entries) == 0 {
		t.Error("Assets directory is empty")
	}

	// Check for expected files
	expectedFiles := []string{"HELP.md", "PROMPT.md", "PROMPT-nogit.md", "tmux.conf", "claude", "HELP-FOR-PAW.md"}
	for _, expected := range expectedFiles {
		found := false
		for _, entry := range entries {
			if entry.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected file/dir %q not found in assets", expected)
		}
	}
}

func TestGetTmuxConfig(t *testing.T) {
	content, err := GetTmuxConfig()
	if err != nil {
		t.Fatalf("GetTmuxConfig() error = %v", err)
	}

	if content == "" {
		t.Error("GetTmuxConfig() returned empty content")
	}

	// tmux.conf should contain some tmux configuration
	if !strings.Contains(content, "set") && !strings.Contains(content, "bind") {
		t.Error("tmux.conf should contain tmux configuration commands")
	}
}

func TestGetHelpContainsKeyboardShortcuts(t *testing.T) {
	content, err := GetHelp()
	if err != nil {
		t.Fatalf("GetHelp() error = %v", err)
	}

	// Help should contain keyboard shortcuts
	expectedTerms := []string{"Keyboard", "Shortcut", "⌃"}
	for _, term := range expectedTerms {
		if !strings.Contains(content, term) {
			t.Errorf("Help content should contain %q", term)
		}
	}
}

func TestGetPromptGitContainsGitInstructions(t *testing.T) {
	content, err := GetPrompt(true)
	if err != nil {
		t.Fatalf("GetPrompt(true) error = %v", err)
	}

	// Git mode prompt should mention git-related terms
	if !strings.Contains(content, "git") && !strings.Contains(content, "worktree") {
		t.Error("Git mode prompt should mention git or worktree")
	}
}

func TestGetPromptNoGitDifferent(t *testing.T) {
	gitPrompt, err := GetPrompt(true)
	if err != nil {
		t.Fatalf("GetPrompt(true) error = %v", err)
	}

	noGitPrompt, err := GetPrompt(false)
	if err != nil {
		t.Fatalf("GetPrompt(false) error = %v", err)
	}

	// The two prompts should be different
	if gitPrompt == noGitPrompt {
		t.Error("Git and non-git prompts should be different")
	}
}

func TestWriteClaudeFilesDoesNotCreateCommands(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, ".claude")

	if err := WriteClaudeFiles(targetDir); err != nil {
		t.Fatalf("WriteClaudeFiles() error = %v", err)
	}

	commandsDir := filepath.Join(targetDir, "commands")
	if _, err := os.Stat(commandsDir); err == nil || !os.IsNotExist(err) {
		t.Errorf("Commands directory should not exist, got err = %v", err)
	}
}

func TestWriteClaudeFilesCreatesSettings(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, ".claude")

	if err := WriteClaudeFiles(targetDir); err != nil {
		t.Fatalf("WriteClaudeFiles() error = %v", err)
	}

	// Check that settings.local.json exists
	settingsPath := filepath.Join(targetDir, "settings.local.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Errorf("settings.local.json not found: %v", err)
	}
}

func TestWriteClaudeFilesCreatesCLAUDEMd(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, ".claude")

	if err := WriteClaudeFiles(targetDir); err != nil {
		t.Fatalf("WriteClaudeFiles() error = %v", err)
	}

	// Check that CLAUDE.md exists
	claudeMdPath := filepath.Join(targetDir, "CLAUDE.md")
	if _, err := os.Stat(claudeMdPath); err != nil {
		t.Errorf("CLAUDE.md not found: %v", err)
	}

	// Check content
	content, err := os.ReadFile(claudeMdPath)
	if err != nil {
		t.Fatalf("Failed to read CLAUDE.md: %v", err)
	}

	// CLAUDE.md should mention HELP-FOR-PAW.md
	if !strings.Contains(string(content), "HELP-FOR-PAW.md") {
		t.Error("CLAUDE.md should reference HELP-FOR-PAW.md")
	}
}

func TestGetPawHelp(t *testing.T) {
	content, err := GetPawHelp()
	if err != nil {
		t.Fatalf("GetPawHelp() error = %v", err)
	}

	if content == "" {
		t.Error("GetPawHelp() returned empty content")
	}

	// Help file should contain PAW-specific terms
	expectedTerms := []string{"PAW", "config", "hooks", "pre_worktree_hook"}
	for _, term := range expectedTerms {
		if !strings.Contains(content, term) {
			t.Errorf("PAW help content should contain %q", term)
		}
	}
}

func TestWritePawHelpFile(t *testing.T) {
	tempDir := t.TempDir()

	if err := WritePawHelpFile(tempDir); err != nil {
		t.Fatalf("WritePawHelpFile() error = %v", err)
	}

	// Check that HELP-FOR-PAW.md exists
	helpPath := filepath.Join(tempDir, "HELP-FOR-PAW.md")
	if _, err := os.Stat(helpPath); err != nil {
		t.Errorf("HELP-FOR-PAW.md not found: %v", err)
	}

	// Check content matches GetPawHelp()
	content, err := os.ReadFile(helpPath)
	if err != nil {
		t.Fatalf("Failed to read HELP-FOR-PAW.md: %v", err)
	}

	expected, _ := GetPawHelp()
	if string(content) != expected {
		t.Error("Written content should match GetPawHelp()")
	}
}

func TestWritePawHelpFileOverwrites(t *testing.T) {
	tempDir := t.TempDir()

	// First write
	if err := WritePawHelpFile(tempDir); err != nil {
		t.Fatalf("First WritePawHelpFile() error = %v", err)
	}

	// Second write (should not error)
	if err := WritePawHelpFile(tempDir); err != nil {
		t.Fatalf("Second WritePawHelpFile() error = %v", err)
	}
}

func TestGetPreCommitHook(t *testing.T) {
	content, err := GetPreCommitHook()
	if err != nil {
		t.Fatalf("GetPreCommitHook() error = %v", err)
	}

	if len(content) == 0 {
		t.Error("GetPreCommitHook() returned empty content")
	}

	// Hook should contain expected content
	if !strings.Contains(string(content), "PAW pre-commit hook") {
		t.Error("pre-commit hook should contain PAW identifier")
	}

	if !strings.Contains(string(content), ".claude") {
		t.Error("pre-commit hook should mention .claude")
	}

	// Hook should be a valid shell script
	if !strings.HasPrefix(string(content), "#!/bin/sh") {
		t.Error("pre-commit hook should start with shebang")
	}
}

func TestInstallPreCommitHook(t *testing.T) {
	tempDir := t.TempDir()
	hooksDir := filepath.Join(tempDir, "hooks")

	if err := InstallPreCommitHook(hooksDir); err != nil {
		t.Fatalf("InstallPreCommitHook() error = %v", err)
	}

	// Check that hooks directory was created
	if _, err := os.Stat(hooksDir); err != nil {
		t.Errorf("Hooks directory not created: %v", err)
	}

	// Check that pre-commit hook exists
	hookPath := filepath.Join(hooksDir, "pre-commit")
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("pre-commit hook not created: %v", err)
	}

	// Check file permissions (should be executable)
	if info.Mode()&0111 == 0 {
		t.Error("pre-commit hook should be executable")
	}

	// Check content
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("Failed to read pre-commit hook: %v", err)
	}

	if !strings.Contains(string(content), "PAW pre-commit hook") {
		t.Error("pre-commit hook content should contain PAW identifier")
	}
}

func TestInstallPreCommitHookIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	hooksDir := filepath.Join(tempDir, "hooks")

	// First install
	if err := InstallPreCommitHook(hooksDir); err != nil {
		t.Fatalf("First InstallPreCommitHook() error = %v", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-commit")
	firstContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("Failed to read pre-commit hook: %v", err)
	}

	// Second install (should be idempotent)
	if err := InstallPreCommitHook(hooksDir); err != nil {
		t.Fatalf("Second InstallPreCommitHook() error = %v", err)
	}

	secondContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("Failed to read pre-commit hook: %v", err)
	}

	// Content should be the same (not duplicated)
	if string(firstContent) != string(secondContent) {
		t.Error("InstallPreCommitHook should be idempotent, content should not be duplicated")
	}
}

func TestInstallPreCommitHookAppendsToExisting(t *testing.T) {
	tempDir := t.TempDir()
	hooksDir := filepath.Join(tempDir, "hooks")
	hookPath := filepath.Join(hooksDir, "pre-commit")

	// Create hooks directory
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	// Create existing hook
	existingContent := "#!/bin/sh\necho 'existing hook'\n"
	if err := os.WriteFile(hookPath, []byte(existingContent), 0755); err != nil {
		t.Fatalf("Failed to create existing hook: %v", err)
	}

	// Install PAW hook
	if err := InstallPreCommitHook(hooksDir); err != nil {
		t.Fatalf("InstallPreCommitHook() error = %v", err)
	}

	// Check that both hooks are present
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("Failed to read pre-commit hook: %v", err)
	}

	if !strings.Contains(string(content), "existing hook") {
		t.Error("Existing hook content should be preserved")
	}

	if !strings.Contains(string(content), "PAW pre-commit hook") {
		t.Error("PAW hook should be appended")
	}
}

func TestAssetsContainsHooksDir(t *testing.T) {
	entries, err := Assets.ReadDir("assets")
	if err != nil {
		t.Fatalf("Failed to read assets directory: %v", err)
	}

	found := false
	for _, entry := range entries {
		if entry.Name() == "hooks" && entry.IsDir() {
			found = true
			break
		}
	}

	if !found {
		t.Error("Assets should contain 'hooks' directory")
	}
}
