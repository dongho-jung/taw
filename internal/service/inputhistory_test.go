package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInputHistoryService_SaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	svc := NewInputHistoryService(tmpDir)

	// Save some entries
	if err := svc.SaveInput("first task"); err != nil {
		t.Fatalf("SaveInput failed: %v", err)
	}
	if err := svc.SaveInput("second task"); err != nil {
		t.Fatalf("SaveInput failed: %v", err)
	}

	// Load and verify
	entries, err := svc.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// Most recent should be first
	if entries[0].Content != "second task" {
		t.Errorf("Expected 'second task' first, got '%s'", entries[0].Content)
	}
	if entries[1].Content != "first task" {
		t.Errorf("Expected 'first task' second, got '%s'", entries[1].Content)
	}
}

func TestInputHistoryService_DuplicateHandling(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewInputHistoryService(tmpDir)

	// Save same content twice
	if err := svc.SaveInput("duplicate task"); err != nil {
		t.Fatalf("SaveInput failed: %v", err)
	}
	if err := svc.SaveInput("other task"); err != nil {
		t.Fatalf("SaveInput failed: %v", err)
	}
	if err := svc.SaveInput("duplicate task"); err != nil {
		t.Fatalf("SaveInput failed: %v", err)
	}

	entries, err := svc.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	// Should have 2 entries (duplicate removed and moved to top)
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].Content != "duplicate task" {
		t.Errorf("Expected 'duplicate task' first, got '%s'", entries[0].Content)
	}
	if entries[1].Content != "other task" {
		t.Errorf("Expected 'other task' second, got '%s'", entries[1].Content)
	}
}

func TestInputHistoryService_MaxEntries(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewInputHistoryService(tmpDir)

	// Save more than max entries
	for i := 0; i < MaxInputHistoryEntries+10; i++ {
		if err := svc.SaveInput("task " + string(rune('A'+i%26)) + string(rune('0'+i%10))); err != nil {
			t.Fatalf("SaveInput failed: %v", err)
		}
	}

	entries, err := svc.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(entries) > MaxInputHistoryEntries {
		t.Errorf("Expected at most %d entries, got %d", MaxInputHistoryEntries, len(entries))
	}
}

func TestInputHistoryService_EmptyContent(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewInputHistoryService(tmpDir)

	// Empty content should be ignored
	if err := svc.SaveInput(""); err != nil {
		t.Fatalf("SaveInput failed: %v", err)
	}

	entries, err := svc.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(entries))
	}
}

func TestInputHistoryService_GetAllContents(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewInputHistoryService(tmpDir)

	if err := svc.SaveInput("task one"); err != nil {
		t.Fatalf("SaveInput failed: %v", err)
	}
	if err := svc.SaveInput("task two"); err != nil {
		t.Fatalf("SaveInput failed: %v", err)
	}

	contents, err := svc.GetAllContents()
	if err != nil {
		t.Fatalf("GetAllContents failed: %v", err)
	}

	if len(contents) != 2 {
		t.Fatalf("Expected 2 contents, got %d", len(contents))
	}

	if contents[0] != "task two" {
		t.Errorf("Expected 'task two' first, got '%s'", contents[0])
	}
}

func TestInputHistoryService_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewInputHistoryService(tmpDir)

	// Should return empty without error
	entries, err := svc.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(entries))
	}
}

func TestInputHistoryService_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewInputHistoryService(tmpDir)

	// Write corrupted data
	historyPath := filepath.Join(tmpDir, InputHistoryFile)
	if err := os.WriteFile(historyPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Should return error
	_, err := svc.LoadHistory()
	if err == nil {
		t.Error("Expected error for corrupted file, got nil")
	}
}
