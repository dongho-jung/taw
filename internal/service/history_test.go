package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donghojung/taw/internal/tmux"
)

// mockClaudeClient is a mock implementation of claude.Client for testing.
type mockClaudeClient struct {
	summaryToReturn string
	summaryError    error
}

func (m *mockClaudeClient) GenerateTaskName(content string) (string, error) {
	return "test-task", nil
}

func (m *mockClaudeClient) GenerateSummary(paneContent string) (string, error) {
	if m.summaryError != nil {
		return "", m.summaryError
	}
	return m.summaryToReturn, nil
}

func (m *mockClaudeClient) WaitForReady(tm tmux.Client, target string) error {
	return nil
}

func (m *mockClaudeClient) SendInput(tm tmux.Client, target, input string) error {
	return nil
}

func (m *mockClaudeClient) SendInputWithRetry(tm tmux.Client, target, input string, maxRetries int) error {
	return nil
}

func (m *mockClaudeClient) SendTrustResponse(tm tmux.Client, target string) error {
	return nil
}

func (m *mockClaudeClient) VerifyPaneAlive(tm tmux.Client, target string, timeout time.Duration) error {
	return nil
}

func TestHistoryService_SaveCompleted(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "taw-history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create service with mock
	svc := NewHistoryService(tmpDir)
	svc.SetClaudeClient(&mockClaudeClient{
		summaryToReturn: "Test summary",
	})

	// Save completed task
	err = svc.SaveCompleted("test-task", "Task content here", "Pane content here")
	if err != nil {
		t.Fatalf("SaveCompleted failed: %v", err)
	}

	// Verify file was created
	files, err := svc.ListHistoryFiles()
	if err != nil {
		t.Fatalf("ListHistoryFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected 1 history file, got %d", len(files))
	}

	// Verify file contents
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("Failed to read history file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Task content here") {
		t.Error("History file should contain task content")
	}
	if !strings.Contains(contentStr, "Test summary") {
		t.Error("History file should contain summary")
	}
	if !strings.Contains(contentStr, "Pane content here") {
		t.Error("History file should contain pane content")
	}

	// Verify filename format (not cancelled)
	if IsCancelled(files[0]) {
		t.Error("Completed task should not have .cancelled extension")
	}
}

func TestHistoryService_SaveCancelled(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "taw-history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create service with mock
	svc := NewHistoryService(tmpDir)
	svc.SetClaudeClient(&mockClaudeClient{
		summaryToReturn: "Cancelled summary",
	})

	// Save cancelled task
	err = svc.SaveCancelled("cancelled-task", "Task content", "Pane content")
	if err != nil {
		t.Fatalf("SaveCancelled failed: %v", err)
	}

	// Verify file was created with .cancelled extension
	files, err := svc.ListHistoryFiles()
	if err != nil {
		t.Fatalf("ListHistoryFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected 1 history file, got %d", len(files))
	}

	if !IsCancelled(files[0]) {
		t.Error("Cancelled task should have .cancelled extension")
	}
}

func TestHistoryService_LoadTaskContent(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "taw-history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a history file manually
	historyContent := `Original task content
---summary---
Summary text
---capture---
Captured pane output`

	historyFile := filepath.Join(tmpDir, "241231_120000_test-task")
	if err := os.WriteFile(historyFile, []byte(historyContent), 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	// Create service and load content
	svc := NewHistoryService(tmpDir)
	taskContent, err := svc.LoadTaskContent(historyFile)
	if err != nil {
		t.Fatalf("LoadTaskContent failed: %v", err)
	}

	expected := "Original task content"
	if taskContent != expected {
		t.Errorf("Expected task content %q, got %q", expected, taskContent)
	}
}

func TestExtractTaskName(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"241231_120000_my-task", "my-task"},
		{"241231_120000_my-task.cancelled", "my-task"},
		{"/path/to/241231_120000_another-task", "another-task"},
		{"/path/to/241231_120000_task.cancelled", "task"},
	}

	for _, tc := range tests {
		result := ExtractTaskName(tc.filename)
		if result != tc.expected {
			t.Errorf("ExtractTaskName(%q) = %q, want %q", tc.filename, result, tc.expected)
		}
	}
}

func TestIsCancelled(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"241231_120000_task", false},
		{"241231_120000_task.cancelled", true},
		{"/path/to/history/241231_120000_task", false},
		{"/path/to/history/241231_120000_task.cancelled", true},
	}

	for _, tc := range tests {
		result := IsCancelled(tc.filename)
		if result != tc.expected {
			t.Errorf("IsCancelled(%q) = %v, want %v", tc.filename, result, tc.expected)
		}
	}
}

func TestHistoryService_EmptyPaneContent(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "taw-history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	svc := NewHistoryService(tmpDir)
	svc.SetClaudeClient(&mockClaudeClient{})

	// Saving with empty pane content should fail
	err = svc.SaveCompleted("task", "content", "")
	if err == nil {
		t.Error("Expected error for empty pane content")
	}
}
