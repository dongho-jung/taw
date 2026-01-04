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
	expectedFiles := []string{"HELP.md", "PROMPT.md", "PROMPT-nogit.md", "tmux.conf", "claude"}
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

func TestWriteClaudeFilesCreatesCommands(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, ".claude")

	if err := WriteClaudeFiles(targetDir); err != nil {
		t.Fatalf("WriteClaudeFiles() error = %v", err)
	}

	// Check that commands directory exists
	commandsDir := filepath.Join(targetDir, "commands")
	info, err := os.Stat(commandsDir)
	if err != nil {
		t.Fatalf("Commands directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Commands path is not a directory")
	}

	// Check for expected command files
	expectedCommands := []string{"commit.md", "test.md", "pr.md", "merge.md"}
	for _, cmd := range expectedCommands {
		cmdPath := filepath.Join(commandsDir, cmd)
		if _, err := os.Stat(cmdPath); err != nil {
			t.Errorf("Expected command file %q not found: %v", cmd, err)
		}
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
