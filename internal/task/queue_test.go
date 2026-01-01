package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewQueueManager(t *testing.T) {
	queueDir := "/path/to/queue"
	qm := NewQueueManager(queueDir)

	if qm.queueDir != queueDir {
		t.Errorf("queueDir = %q, want %q", qm.queueDir, queueDir)
	}
}

func TestQueueManagerAdd(t *testing.T) {
	tempDir := t.TempDir()
	queueDir := filepath.Join(tempDir, ".queue")

	qm := NewQueueManager(queueDir)

	content := "My first task"
	if err := qm.Add(content); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Check file was created
	files, err := os.ReadDir(queueDir)
	if err != nil {
		t.Fatalf("Failed to read queue dir: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}
	if files[0].Name() != "001.task" {
		t.Errorf("File name = %q, want %q", files[0].Name(), "001.task")
	}

	// Check content
	data, err := os.ReadFile(filepath.Join(queueDir, "001.task"))
	if err != nil {
		t.Fatalf("Failed to read task file: %v", err)
	}
	if string(data) != content {
		t.Errorf("Content = %q, want %q", string(data), content)
	}
}

func TestQueueManagerList(t *testing.T) {
	tempDir := t.TempDir()
	queueDir := filepath.Join(tempDir, ".queue")

	qm := NewQueueManager(queueDir)

	// List empty queue
	tasks, err := qm.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(tasks))
	}

	// Add some tasks
	qm.Add("Task 1")
	qm.Add("Task 2")
	qm.Add("Task 3")

	tasks, err = qm.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(tasks))
	}

	// Check order
	if tasks[0].Number != 1 || tasks[0].Content != "Task 1" {
		t.Errorf("Task 0: number=%d, content=%q, want 1, 'Task 1'", tasks[0].Number, tasks[0].Content)
	}
	if tasks[1].Number != 2 || tasks[1].Content != "Task 2" {
		t.Errorf("Task 1: number=%d, content=%q, want 2, 'Task 2'", tasks[1].Number, tasks[1].Content)
	}
	if tasks[2].Number != 3 || tasks[2].Content != "Task 3" {
		t.Errorf("Task 2: number=%d, content=%q, want 3, 'Task 3'", tasks[2].Number, tasks[2].Content)
	}
}

func TestQueueManagerPop(t *testing.T) {
	tempDir := t.TempDir()
	queueDir := filepath.Join(tempDir, ".queue")

	qm := NewQueueManager(queueDir)

	// Pop from empty queue
	task, err := qm.Pop()
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if task != nil {
		t.Error("Pop() on empty queue should return nil")
	}

	// Add tasks
	qm.Add("Task 1")
	qm.Add("Task 2")

	// Pop first task
	task, err = qm.Pop()
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if task == nil {
		t.Fatal("Pop() returned nil, expected task")
	}
	if task.Content != "Task 1" {
		t.Errorf("Pop() content = %q, want %q", task.Content, "Task 1")
	}

	// Check only one task remains
	tasks, _ := qm.List()
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task remaining, got %d", len(tasks))
	}
	if tasks[0].Content != "Task 2" {
		t.Errorf("Remaining task content = %q, want %q", tasks[0].Content, "Task 2")
	}
}

func TestQueueManagerClear(t *testing.T) {
	tempDir := t.TempDir()
	queueDir := filepath.Join(tempDir, ".queue")

	qm := NewQueueManager(queueDir)

	qm.Add("Task 1")
	qm.Add("Task 2")
	qm.Add("Task 3")

	if err := qm.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	tasks, err := qm.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks after Clear(), got %d", len(tasks))
	}
}

func TestQueueManagerCount(t *testing.T) {
	tempDir := t.TempDir()
	queueDir := filepath.Join(tempDir, ".queue")

	qm := NewQueueManager(queueDir)

	count, err := qm.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Count() = %d, want 0", count)
	}

	qm.Add("Task 1")
	qm.Add("Task 2")

	count, err = qm.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 2 {
		t.Errorf("Count() = %d, want 2", count)
	}
}

func TestQueueManagerNextNumber(t *testing.T) {
	tempDir := t.TempDir()
	queueDir := filepath.Join(tempDir, ".queue")

	qm := NewQueueManager(queueDir)

	// Add two tasks (1, 2)
	qm.Add("Task 1")
	qm.Add("Task 2")

	// Pop first one
	qm.Pop()

	// Add another task - should get number 3 since task 2 still exists
	qm.Add("Task 3")

	tasks, _ := qm.List()
	if len(tasks) != 2 {
		t.Fatalf("Expected 2 tasks, got %d", len(tasks))
	}
	// First remaining task should be 2
	if tasks[0].Number != 2 {
		t.Errorf("First task number = %d, want 2", tasks[0].Number)
	}
	// New task should be 3
	if tasks[1].Number != 3 {
		t.Errorf("Second task number = %d, want 3", tasks[1].Number)
	}
}

func TestGenerateTaskNameFromContent(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		existingNames map[string]bool
		wantPrefix    string
	}{
		{
			name:          "simple content",
			content:       "Add login feature",
			existingNames: nil,
			wantPrefix:    "add-login-feature",
		},
		{
			name:          "multiline content uses first line",
			content:       "Fix the bug\nSecond line\nThird line",
			existingNames: nil,
			wantPrefix:    "fix-the-bug",
		},
		{
			name:          "special characters removed",
			content:       "Add user@email.com validation!",
			existingNames: nil,
			wantPrefix:    "add-user-email-com-validation",
		},
		{
			name:          "short content gets prefix",
			content:       "foo",
			existingNames: nil,
			wantPrefix:    "queue-task-foo",
		},
		{
			name:          "duplicate name gets suffix",
			content:       "implement-login-feature",
			existingNames: map[string]bool{"implement-login-feature": true},
			wantPrefix:    "implement-login-feature-1",
		},
		{
			name:          "multiple duplicates",
			content:       "implement-login-feature",
			existingNames: map[string]bool{"implement-login-feature": true, "implement-login-feature-1": true, "implement-login-feature-2": true},
			wantPrefix:    "implement-login-feature-3",
		},
		{
			name:          "long content truncated",
			content:       "This is a very long task description that should be truncated to fit the limit",
			existingNames: nil,
			wantPrefix:    "this-is-a-very-long-task-descr",
		},
		{
			name:          "uppercase converted to lowercase",
			content:       "CREATE New COMPONENT",
			existingNames: nil,
			wantPrefix:    "create-new-component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateTaskNameFromContent(tt.content, tt.existingNames)
			if got != tt.wantPrefix {
				t.Errorf("GenerateTaskNameFromContent() = %q, want %q", got, tt.wantPrefix)
			}
		})
	}
}

func TestQueueManagerListIgnoresNonTaskFiles(t *testing.T) {
	tempDir := t.TempDir()
	queueDir := filepath.Join(tempDir, ".queue")
	if err := os.MkdirAll(queueDir, 0755); err != nil {
		t.Fatalf("Failed to create queue dir: %v", err)
	}

	// Create valid task file
	if err := os.WriteFile(filepath.Join(queueDir, "001.task"), []byte("Valid task"), 0644); err != nil {
		t.Fatalf("Failed to create task file: %v", err)
	}

	// Create non-task files
	if err := os.WriteFile(filepath.Join(queueDir, "readme.md"), []byte("Readme"), 0644); err != nil {
		t.Fatalf("Failed to create readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queueDir, "abc.task"), []byte("Invalid number"), 0644); err != nil {
		t.Fatalf("Failed to create invalid task: %v", err)
	}

	// Create subdirectory
	if err := os.MkdirAll(filepath.Join(queueDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	qm := NewQueueManager(queueDir)
	tasks, err := qm.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Content != "Valid task" {
		t.Errorf("Task content = %q, want %q", tasks[0].Content, "Valid task")
	}
}
