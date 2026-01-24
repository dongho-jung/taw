package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dongho-jung/paw/internal/app"
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

func TestShellQuote(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "''"},
		{name: "simple", input: "value", want: "'value'"},
		{name: "spaces", input: "two words", want: "'two words'"},
		{name: "single-quote", input: "has'quote", want: "'has'\\''quote'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellQuote(tc.input); got != tc.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestShellJoinEnvAndCommand(t *testing.T) {
	if got, want := shellJoin("paw", "internal", "new task"), "'paw' 'internal' 'new task'"; got != want {
		t.Fatalf("shellJoin() = %q, want %q", got, want)
	}

	if got, want := shellEnv("KEY", "a b"), "KEY='a b'"; got != want {
		t.Fatalf("shellEnv() = %q, want %q", got, want)
	}

	if got, want := shellCommand("echo hi"), "sh -c 'echo hi'"; got != want {
		t.Fatalf("shellCommand() = %q, want %q", got, want)
	}
}

func TestBuildNewTaskCommand(t *testing.T) {
	appCtx := &app.App{
		PawDir:      "/tmp/paw dir",
		ProjectDir:  "/tmp/project",
		DisplayName: "My Project",
		SessionName: "session",
	}

	got := buildNewTaskCommand(appCtx, "/usr/local/bin/paw", "session")
	want := "PAW_DIR='/tmp/paw dir' PROJECT_DIR='/tmp/project' DISPLAY_NAME='My Project' '/usr/local/bin/paw' 'internal' 'new-task' 'session'"
	if got != want {
		t.Fatalf("buildNewTaskCommand() = %q, want %q", got, want)
	}

	appCtx.DisplayName = ""
	got = buildNewTaskCommand(appCtx, "paw", "session")
	if !strings.Contains(got, "DISPLAY_NAME='session'") {
		t.Fatalf("buildNewTaskCommand() missing fallback display name: %q", got)
	}
}
