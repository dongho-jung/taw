package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donghojung/taw/internal/constants"
)

func TestNewTask(t *testing.T) {
	name := "test-task"
	agentDir := "/path/to/agents/test-task"

	task := New(name, agentDir)

	if task.Name != name {
		t.Errorf("Name = %q, want %q", task.Name, name)
	}
	if task.AgentDir != agentDir {
		t.Errorf("AgentDir = %q, want %q", task.AgentDir, agentDir)
	}
	if task.Status != StatusPending {
		t.Errorf("Status = %q, want %q", task.Status, StatusPending)
	}
	if task.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestTaskGetPaths(t *testing.T) {
	name := "test-task"
	agentDir := "/path/to/agents/test-task"
	task := New(name, agentDir)

	// Test GetTaskFilePath
	expectedTaskFile := filepath.Join(agentDir, constants.TaskFileName)
	if task.GetTaskFilePath() != expectedTaskFile {
		t.Errorf("GetTaskFilePath() = %q, want %q", task.GetTaskFilePath(), expectedTaskFile)
	}

	// Test GetTabLockDir
	expectedTabLock := filepath.Join(agentDir, constants.TabLockDirName)
	if task.GetTabLockDir() != expectedTabLock {
		t.Errorf("GetTabLockDir() = %q, want %q", task.GetTabLockDir(), expectedTabLock)
	}

	// Test GetWindowIDPath
	expectedWindowID := filepath.Join(agentDir, constants.TabLockDirName, constants.WindowIDFileName)
	if task.GetWindowIDPath() != expectedWindowID {
		t.Errorf("GetWindowIDPath() = %q, want %q", task.GetWindowIDPath(), expectedWindowID)
	}

	// Test GetWorktreeDir (default)
	expectedWorktree := filepath.Join(agentDir, "worktree")
	if task.GetWorktreeDir() != expectedWorktree {
		t.Errorf("GetWorktreeDir() = %q, want %q", task.GetWorktreeDir(), expectedWorktree)
	}

	// Test GetWorktreeDir (custom)
	task.WorktreeDir = "/custom/worktree"
	if task.GetWorktreeDir() != "/custom/worktree" {
		t.Errorf("GetWorktreeDir() = %q, want %q", task.GetWorktreeDir(), "/custom/worktree")
	}

	// Test GetPRFilePath
	expectedPRFile := filepath.Join(agentDir, constants.PRFileName)
	if task.GetPRFilePath() != expectedPRFile {
		t.Errorf("GetPRFilePath() = %q, want %q", task.GetPRFilePath(), expectedPRFile)
	}

	// Test GetOriginPath
	expectedOrigin := filepath.Join(agentDir, "origin")
	if task.GetOriginPath() != expectedOrigin {
		t.Errorf("GetOriginPath() = %q, want %q", task.GetOriginPath(), expectedOrigin)
	}
}

func TestTaskTabLock(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	// Should not have tab lock initially
	if task.HasTabLock() {
		t.Error("HasTabLock() = true, want false initially")
	}

	// Create tab lock
	created, err := task.CreateTabLock()
	if err != nil {
		t.Fatalf("CreateTabLock() error = %v", err)
	}
	if !created {
		t.Error("CreateTabLock() returned false, want true")
	}

	// Should have tab lock now
	if !task.HasTabLock() {
		t.Error("HasTabLock() = false, want true after CreateTabLock()")
	}

	// Creating again should return false (already exists)
	created2, err := task.CreateTabLock()
	if err != nil {
		t.Fatalf("CreateTabLock() second call error = %v", err)
	}
	if created2 {
		t.Error("CreateTabLock() second call returned true, want false")
	}

	// Remove tab lock
	if err := task.RemoveTabLock(); err != nil {
		t.Fatalf("RemoveTabLock() error = %v", err)
	}

	// Should not have tab lock anymore
	if task.HasTabLock() {
		t.Error("HasTabLock() = true, want false after RemoveTabLock()")
	}
}

func TestTaskWindowID(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	// Create tab lock directory first
	if _, err := task.CreateTabLock(); err != nil {
		t.Fatalf("CreateTabLock() error = %v", err)
	}

	windowID := "@123"
	if err := task.SaveWindowID(windowID); err != nil {
		t.Fatalf("SaveWindowID() error = %v", err)
	}

	// Task should have the window ID set
	if task.WindowID != windowID {
		t.Errorf("WindowID = %q, want %q", task.WindowID, windowID)
	}

	// Load window ID
	loaded, err := task.LoadWindowID()
	if err != nil {
		t.Fatalf("LoadWindowID() error = %v", err)
	}
	if loaded != windowID {
		t.Errorf("LoadWindowID() = %q, want %q", loaded, windowID)
	}
}

func TestTaskContent(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	content := "This is my task content\nwith multiple lines"
	if err := task.SaveContent(content); err != nil {
		t.Fatalf("SaveContent() error = %v", err)
	}

	// Task should have the content set
	if task.Content != content {
		t.Errorf("Content = %q, want %q", task.Content, content)
	}

	// Create new task and load content
	task2 := New("test-task", agentDir)
	loaded, err := task2.LoadContent()
	if err != nil {
		t.Fatalf("LoadContent() error = %v", err)
	}
	if loaded != content {
		t.Errorf("LoadContent() = %q, want %q", loaded, content)
	}
}

func TestTaskPRNumber(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	// Should not have PR initially
	if task.HasPR() {
		t.Error("HasPR() = true, want false initially")
	}

	prNumber := 42
	if err := task.SavePRNumber(prNumber); err != nil {
		t.Fatalf("SavePRNumber() error = %v", err)
	}

	// Should have PR now
	if !task.HasPR() {
		t.Error("HasPR() = false, want true after SavePRNumber()")
	}

	// Task should have the PR number set
	if task.PRNumber != prNumber {
		t.Errorf("PRNumber = %d, want %d", task.PRNumber, prNumber)
	}

	// Load PR number
	loaded, err := task.LoadPRNumber()
	if err != nil {
		t.Fatalf("LoadPRNumber() error = %v", err)
	}
	if loaded != prNumber {
		t.Errorf("LoadPRNumber() = %d, want %d", loaded, prNumber)
	}
}

func TestTaskLoadPRNumberNotExists(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	// Load PR number when file doesn't exist should return 0
	loaded, err := task.LoadPRNumber()
	if err != nil {
		t.Fatalf("LoadPRNumber() error = %v", err)
	}
	if loaded != 0 {
		t.Errorf("LoadPRNumber() = %d, want 0 when file doesn't exist", loaded)
	}
}

func TestTaskGetWindowName(t *testing.T) {
	tests := []struct {
		name       string
		taskName   string
		status     Status
		wantPrefix string
	}{
		{
			name:       "pending status",
			taskName:   "my-task",
			status:     StatusPending,
			wantPrefix: constants.EmojiWorking,
		},
		{
			name:       "working status",
			taskName:   "my-task",
			status:     StatusWorking,
			wantPrefix: constants.EmojiWorking,
		},
		{
			name:       "waiting status",
			taskName:   "my-task",
			status:     StatusWaiting,
			wantPrefix: constants.EmojiWaiting,
		},
		{
			name:       "done status",
			taskName:   "my-task",
			status:     StatusDone,
			wantPrefix: constants.EmojiDone,
		},
		{
			name:       "corrupted status",
			taskName:   "my-task",
			status:     StatusCorrupted,
			wantPrefix: constants.EmojiWarning,
		},
		{
			name:       "long name truncated",
			taskName:   "very-long-task-name-that-exceeds-limit",
			status:     StatusWorking,
			wantPrefix: constants.EmojiWorking,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Name: tt.taskName, Status: tt.status}
			windowName := task.GetWindowName()

			if windowName[:len(tt.wantPrefix)] != tt.wantPrefix {
				t.Errorf("GetWindowName() prefix = %q, want %q", windowName[:len(tt.wantPrefix)], tt.wantPrefix)
			}

			// Check truncation
			if len(tt.taskName) > 12 {
				if len(windowName) > len(tt.wantPrefix)+12 {
					t.Errorf("GetWindowName() = %q, should be truncated", windowName)
				}
			}
		})
	}
}

func TestTaskExists(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")

	task := New("test-task", agentDir)

	// Should not exist initially
	if task.Exists() {
		t.Error("Exists() = true, want false initially")
	}

	// Create directory
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	// Should exist now
	if !task.Exists() {
		t.Error("Exists() = false, want true after creating directory")
	}
}

func TestTaskRemove(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	// Create some files
	if err := os.WriteFile(filepath.Join(agentDir, "task"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	task := New("test-task", agentDir)

	if err := task.Remove(); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if task.Exists() {
		t.Error("Task directory still exists after Remove()")
	}
}

func TestTaskSetupSymlinks(t *testing.T) {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "project")
	agentDir := filepath.Join(tempDir, "agents", "test-task")

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	if err := task.SetupSymlinks(projectDir); err != nil {
		t.Fatalf("SetupSymlinks() error = %v", err)
	}

	// Check origin symlink exists
	originPath := task.GetOriginPath()
	info, err := os.Lstat(originPath)
	if err != nil {
		t.Fatalf("Origin symlink not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Origin is not a symlink")
	}
}

func TestTaskSessionMarker(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	// Should not have session marker initially
	if task.HasSessionMarker() {
		t.Error("HasSessionMarker() = true, want false initially")
	}

	// Test GetSessionMarkerPath
	expectedPath := filepath.Join(agentDir, ".session-started")
	if task.GetSessionMarkerPath() != expectedPath {
		t.Errorf("GetSessionMarkerPath() = %q, want %q", task.GetSessionMarkerPath(), expectedPath)
	}

	// Create session marker
	if err := task.CreateSessionMarker(); err != nil {
		t.Fatalf("CreateSessionMarker() error = %v", err)
	}

	// Should have session marker now
	if !task.HasSessionMarker() {
		t.Error("HasSessionMarker() = false, want true after CreateSessionMarker()")
	}

	// Check marker file contains timestamp
	data, err := os.ReadFile(task.GetSessionMarkerPath())
	if err != nil {
		t.Fatalf("Failed to read session marker: %v", err)
	}
	if len(data) == 0 {
		t.Error("Session marker file is empty, should contain timestamp")
	}
}
