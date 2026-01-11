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
