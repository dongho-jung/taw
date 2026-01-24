package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildTaskInstruction(t *testing.T) {
	tempDir := t.TempDir()
	promptPath := filepath.Join(tempDir, "prompt.txt")
	expected := "Do the task.\n"
	if err := os.WriteFile(promptPath, []byte(expected), 0644); err != nil {
		t.Fatalf("failed to write prompt file: %v", err)
	}

	got, err := buildTaskInstruction(promptPath)
	if err != nil {
		t.Fatalf("buildTaskInstruction() error = %v", err)
	}
	if got != expected {
		t.Fatalf("buildTaskInstruction() = %q, want %q", got, expected)
	}
}

func TestBuildTaskInstructionMissingFile(t *testing.T) {
	tempDir := t.TempDir()
	promptPath := filepath.Join(tempDir, "missing.txt")

	if _, err := buildTaskInstruction(promptPath); err == nil {
		t.Fatal("buildTaskInstruction() expected error for missing file")
	}
}

func TestBuildTaskInstructionEmptyFile(t *testing.T) {
	tempDir := t.TempDir()
	promptPath := filepath.Join(tempDir, "empty.txt")
	if err := os.WriteFile(promptPath, []byte(" \n\t"), 0644); err != nil {
		t.Fatalf("failed to write prompt file: %v", err)
	}

	if _, err := buildTaskInstruction(promptPath); err == nil {
		t.Fatal("buildTaskInstruction() expected error for empty content")
	}
}
