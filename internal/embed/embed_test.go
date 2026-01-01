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
	if !strings.Contains(content, "TAW") && !strings.Contains(content, "taw") {
		t.Error("Help content should mention TAW/taw")
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
	expectedFiles := []string{"HELP.md", "PROMPT.md", "PROMPT-nogit.md", "claude"}
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
