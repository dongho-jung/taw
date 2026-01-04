package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dongho-jung/paw/internal/config"
)

func TestFindMergedTasks_ExternallyCleanedUp(t *testing.T) {
	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "paw-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create .paw/agents directory
	agentsDir := filepath.Join(tempDir, ".paw", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("Failed to create agents dir: %v", err)
	}

	// Create a fake task directory (simulating externally cleaned up task)
	taskName := "test-task"
	taskDir := filepath.Join(agentsDir, taskName)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	// Create task file
	taskFile := filepath.Join(taskDir, "task")
	if err := os.WriteFile(taskFile, []byte("test task content"), 0644); err != nil {
		t.Fatalf("Failed to create task file: %v", err)
	}

	// Create config with worktree mode
	cfg := &config.Config{
		WorkMode: config.WorkModeWorktree,
	}

	// Create manager (git repo = true, but we'll use a mock git client)
	mgr := NewManager(agentsDir, tempDir, filepath.Join(tempDir, ".paw"), true, cfg)

	// Call FindMergedTasks
	merged, err := mgr.FindMergedTasks()
	if err != nil {
		t.Fatalf("FindMergedTasks failed: %v", err)
	}

	// Check if the task was found as merged
	if len(merged) != 1 {
		t.Errorf("Expected 1 merged task, got %d", len(merged))
	} else if merged[0].Name != taskName {
		t.Errorf("Expected task name %s, got %s", taskName, merged[0].Name)
	}
}
