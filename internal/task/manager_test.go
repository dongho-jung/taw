package task

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
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

	// Create config
	cfg := &config.Config{}

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

func TestFindTaskByTruncatedName(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "paw-task-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	agentsDir := filepath.Join(tempDir, ".paw", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("Failed to create agents dir: %v", err)
	}

	pawDir := filepath.Join(tempDir, ".paw")
	mgr := NewManager(agentsDir, tempDir, pawDir, false, config.DefaultConfig())

	// Create a task with a long name that exceeds MaxWindowNameLen (12 chars)
	longTaskName := "fix-restore-pane-truncated-window"
	taskDir := filepath.Join(agentsDir, longTaskName)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatalf("Failed to create task directory: %v", err)
	}
	// Create task file
	if err := os.WriteFile(filepath.Join(taskDir, "task"), []byte("test task"), 0644); err != nil {
		t.Fatalf("Failed to create task file: %v", err)
	}

	t.Run("find by full name", func(t *testing.T) {
		task, err := mgr.FindTaskByTruncatedName(constants.TruncateForWindowName(longTaskName))
		if err != nil {
			t.Fatalf("FindTaskByTruncatedName returned error: %v", err)
		}
		if task.Name != longTaskName {
			t.Fatalf("Expected task name %s, got %s", longTaskName, task.Name)
		}
	})

	t.Run("find by exact truncated name", func(t *testing.T) {
		task, err := mgr.FindTaskByTruncatedName(constants.TruncateForWindowName(longTaskName))
		if err != nil {
			t.Fatalf("FindTaskByTruncatedName returned error: %v", err)
		}
		if task.Name != longTaskName {
			t.Fatalf("Expected task name %s, got %s", longTaskName, task.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := mgr.FindTaskByTruncatedName("nonexistent-t")
		if !errors.Is(err, ErrTaskNotFound) {
			t.Fatalf("Expected ErrTaskNotFound, got %v", err)
		}
	})
}

func TestFindTaskByWindowID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "paw-task-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	agentsDir := filepath.Join(tempDir, ".paw", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("Failed to create agents dir: %v", err)
	}

	pawDir := filepath.Join(tempDir, ".paw")
	mgr := NewManager(agentsDir, tempDir, pawDir, false, config.DefaultConfig())

	// Create first task
	taskOne := New("task-one", filepath.Join(agentsDir, "task-one"))
	if err := os.MkdirAll(taskOne.AgentDir, 0755); err != nil {
		t.Fatalf("Failed to create task directory: %v", err)
	}
	if _, err := taskOne.CreateTabLock(); err != nil {
		t.Fatalf("Failed to create tab-lock: %v", err)
	}
	if err := taskOne.SaveWindowID("1"); err != nil {
		t.Fatalf("Failed to save window ID: %v", err)
	}
	if err := taskOne.SaveContent("first task"); err != nil {
		t.Fatalf("Failed to save task content: %v", err)
	}

	// Create second task
	taskTwo := New("task-two", filepath.Join(agentsDir, "task-two"))
	if err := os.MkdirAll(taskTwo.AgentDir, 0755); err != nil {
		t.Fatalf("Failed to create task directory: %v", err)
	}
	if _, err := taskTwo.CreateTabLock(); err != nil {
		t.Fatalf("Failed to create tab-lock: %v", err)
	}
	if err := taskTwo.SaveWindowID("2"); err != nil {
		t.Fatalf("Failed to save window ID: %v", err)
	}
	if err := taskTwo.SaveContent("second task"); err != nil {
		t.Fatalf("Failed to save task content: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		task, err := mgr.FindTaskByWindowID("2")
		if err != nil {
			t.Fatalf("FindTaskByWindowID returned error: %v", err)
		}
		if task.Name != taskTwo.Name {
			t.Fatalf("Expected task name %s, got %s", taskTwo.Name, task.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := mgr.FindTaskByWindowID("missing")
		if !errors.Is(err, ErrTaskNotFound) {
			t.Fatalf("Expected ErrTaskNotFound, got %v", err)
		}
	})
}
