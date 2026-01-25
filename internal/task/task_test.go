package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dongho-jung/paw/internal/constants"
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

	// Test GetTaskContextPath
	expectedTaskContext := filepath.Join(agentDir, constants.TaskContextFileName)
	if task.GetTaskContextPath() != expectedTaskContext {
		t.Errorf("GetTaskContextPath() = %q, want %q", task.GetTaskContextPath(), expectedTaskContext)
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
			name:       "corrupted status displays as waiting",
			taskName:   "my-task",
			status:     StatusCorrupted,
			wantPrefix: constants.EmojiWaiting, // Corrupted displays as Waiting.
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

			// Check truncation (uses constants.MaxWindowNameLen)
			if len(tt.taskName) > constants.MaxWindowNameLen {
				if len(windowName) > len(tt.wantPrefix)+constants.MaxWindowNameLen {
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

func TestTaskStatus(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	// Test GetStatusFilePath
	expectedPath := filepath.Join(agentDir, constants.StatusFileName)
	if task.GetStatusFilePath() != expectedPath {
		t.Errorf("GetStatusFilePath() = %q, want %q", task.GetStatusFilePath(), expectedPath)
	}

	// Load status when file doesn't exist should return Pending
	status, err := task.LoadStatus()
	if err != nil {
		t.Fatalf("LoadStatus() error = %v", err)
	}
	if status != StatusPending {
		t.Errorf("LoadStatus() = %q, want %q when file doesn't exist", status, StatusPending)
	}

	// Save status
	for _, testStatus := range []Status{StatusWorking, StatusWaiting, StatusDone} {
		if err := task.SaveStatus(testStatus); err != nil {
			t.Fatalf("SaveStatus(%q) error = %v", testStatus, err)
		}

		if task.Status != testStatus {
			t.Errorf("task.Status = %q, want %q after SaveStatus()", task.Status, testStatus)
		}

		// Load and verify
		loaded, err := task.LoadStatus()
		if err != nil {
			t.Fatalf("LoadStatus() error = %v", err)
		}
		if loaded != testStatus {
			t.Errorf("LoadStatus() = %q, want %q", loaded, testStatus)
		}
	}
}

func TestTransitionStatus(t *testing.T) {
	tempDir := t.TempDir()
	taskDir := filepath.Join(tempDir, "task")
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	task := New("test-task", taskDir)

	prev, valid, err := task.TransitionStatus(StatusWorking)
	if err != nil {
		t.Fatalf("TransitionStatus working failed: %v", err)
	}
	if prev != StatusPending {
		t.Fatalf("Expected previous status pending, got %s", prev)
	}
	if !valid {
		t.Fatalf("Expected valid transition pending->working")
	}

	prev, valid, err = task.TransitionStatus(StatusDone)
	if err != nil {
		t.Fatalf("TransitionStatus done failed: %v", err)
	}
	if prev != StatusWorking {
		t.Fatalf("Expected previous status working, got %s", prev)
	}
	if !valid {
		t.Fatalf("Expected valid transition working->done")
	}

	prev, valid, err = task.TransitionStatus(StatusWorking)
	if err != nil {
		t.Fatalf("TransitionStatus done->working failed: %v", err)
	}
	if prev != StatusDone {
		t.Fatalf("Expected previous status done, got %s", prev)
	}
	if !valid {
		t.Fatalf("Expected valid transition done->working (allows resuming completed tasks)")
	}

	prev, valid, err = task.TransitionStatus(StatusDone)
	if err != nil {
		t.Fatalf("TransitionStatus working->done failed: %v", err)
	}
	if prev != StatusWorking {
		t.Fatalf("Expected previous status working, got %s", prev)
	}
	if !valid {
		t.Fatalf("Expected valid transition working->done")
	}

	prev, valid, err = task.TransitionStatus(StatusWaiting)
	if err != nil {
		t.Fatalf("TransitionStatus done->waiting failed: %v", err)
	}
	if prev != StatusDone {
		t.Fatalf("Expected previous status done, got %s", prev)
	}
	if !valid {
		t.Fatalf("Expected valid transition done->waiting")
	}
}

func TestTaskGetSystemPromptPath(t *testing.T) {
	agentDir := "/path/to/agents/test-task"
	task := New("test-task", agentDir)

	expectedPath := filepath.Join(agentDir, ".system-prompt")
	if task.GetSystemPromptPath() != expectedPath {
		t.Errorf("GetSystemPromptPath() = %q, want %q", task.GetSystemPromptPath(), expectedPath)
	}
}

func TestTaskGetUserPromptPath(t *testing.T) {
	agentDir := "/path/to/agents/test-task"
	task := New("test-task", agentDir)

	expectedPath := filepath.Join(agentDir, ".user-prompt")
	if task.GetUserPromptPath() != expectedPath {
		t.Errorf("GetUserPromptPath() = %q, want %q", task.GetUserPromptPath(), expectedPath)
	}
}

func TestStatusConstants(t *testing.T) {
	// Verify status constants have expected string values
	statuses := map[Status]string{
		StatusPending:   "pending",
		StatusWorking:   "working",
		StatusWaiting:   "waiting",
		StatusDone:      "done",
		StatusCorrupted: "corrupted",
	}

	for status, expected := range statuses {
		if string(status) != expected {
			t.Errorf("Status %q = %q, want %q", status, string(status), expected)
		}
	}
}

func TestCorruptedReasonConstants(t *testing.T) {
	// Verify corrupted reason constants have expected string values
	reasons := map[CorruptedReason]string{
		CorruptMissingWorktree: "missing_worktree",
		CorruptNotInGit:        "not_in_git",
		CorruptInvalidGit:      "invalid_git",
		CorruptMissingBranch:   "missing_branch",
	}

	for reason, expected := range reasons {
		if string(reason) != expected {
			t.Errorf("CorruptedReason %q = %q, want %q", reason, string(reason), expected)
		}
	}
}

func TestTaskSetupSymlinksOverwrite(t *testing.T) {
	tempDir := t.TempDir()
	projectDir1 := filepath.Join(tempDir, "project1")
	projectDir2 := filepath.Join(tempDir, "project2")
	agentDir := filepath.Join(tempDir, "agents", "test-task")

	for _, dir := range []string{projectDir1, projectDir2, agentDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	task := New("test-task", agentDir)

	// Create first symlink
	if err := task.SetupSymlinks(projectDir1); err != nil {
		t.Fatalf("SetupSymlinks(project1) error = %v", err)
	}

	// Overwrite with second symlink
	if err := task.SetupSymlinks(projectDir2); err != nil {
		t.Fatalf("SetupSymlinks(project2) error = %v", err)
	}

	// Verify symlink points to second project
	target, err := os.Readlink(task.GetOriginPath())
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}

	// The target should be a relative path to projectDir2
	resolvedTarget := filepath.Join(agentDir, target)
	absTarget, _ := filepath.Abs(resolvedTarget)
	absProject2, _ := filepath.Abs(projectDir2)

	if absTarget != absProject2 {
		t.Errorf("Symlink target = %q, want %q", absTarget, absProject2)
	}
}

func TestTaskSetupClaudeSymlink(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agents", "test-task")
	worktreeDir := filepath.Join(agentDir, "worktree")
	pawDir := filepath.Join(tempDir, "paw")
	claudeDir := filepath.Join(pawDir, ".claude")

	// Create required directories
	for _, dir := range []string{agentDir, worktreeDir, claudeDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	// Create a test settings file in source .claude
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(settingsPath, []byte(`{"test": true}`), 0644); err != nil {
		t.Fatalf("Failed to write settings: %v", err)
	}

	task := New("test-task", agentDir)
	task.WorktreeDir = worktreeDir

	if err := task.SetupClaudeSymlink(pawDir); err != nil {
		t.Fatalf("SetupClaudeSymlink() error = %v", err)
	}

	// Check symlink exists in agent directory (outside git worktree)
	// Claude Code is started with --settings flag pointing to this location
	claudeTargetPath := filepath.Join(agentDir, ".claude")
	info, err := os.Lstat(claudeTargetPath)
	if err != nil {
		t.Fatalf("Claude symlink not created in agent dir: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Claude path is not a symlink")
	}

	// Verify symlink points to correct location
	target, err := os.Readlink(claudeTargetPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}

	// Resolve and verify the target
	resolvedTarget := filepath.Join(agentDir, target)
	absTarget, _ := filepath.Abs(resolvedTarget)
	absClaudeDir, _ := filepath.Abs(claudeDir)

	if absTarget != absClaudeDir {
		t.Errorf("Symlink target = %q, want %q", absTarget, absClaudeDir)
	}

	// Verify settings file is accessible through symlink
	symlinkSettings := filepath.Join(claudeTargetPath, "settings.local.json")
	if _, err := os.Stat(symlinkSettings); err != nil {
		t.Errorf("Settings file not accessible through symlink: %v", err)
	}

	// Verify symlink is NOT in worktree (to confirm it's outside git)
	worktreeClaudePath := filepath.Join(worktreeDir, ".claude")
	if _, err := os.Lstat(worktreeClaudePath); !os.IsNotExist(err) {
		t.Error("Symlink should NOT be created in worktree (must be outside git)")
	}
}

func TestTaskSetupClaudeSymlinkNoSource(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agents", "test-task")
	pawDir := filepath.Join(tempDir, "paw")

	// Create agent dir but NOT .claude source
	for _, dir := range []string{agentDir, pawDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	task := New("test-task", agentDir)

	// Should not error - just return nil when source doesn't exist
	if err := task.SetupClaudeSymlink(pawDir); err != nil {
		t.Errorf("SetupClaudeSymlink() should not error without source: %v", err)
	}

	// Verify no symlink was created in agent dir
	claudeTargetPath := filepath.Join(agentDir, ".claude")
	if _, err := os.Lstat(claudeTargetPath); !os.IsNotExist(err) {
		t.Error("Symlink should not be created when source doesn't exist")
	}
}

// TestSetupClaudeSymlinkInDir tests creating .claude symlink in a custom directory
// (used for non-worktree mode where symlink goes in project directory)
func TestSetupClaudeSymlinkInDir(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agents", "test-task")
	projectDir := filepath.Join(tempDir, "project")
	pawDir := filepath.Join(tempDir, ".paw")
	claudeSource := filepath.Join(pawDir, ".claude")

	// Create directories
	for _, dir := range []string{agentDir, projectDir, claudeSource} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	// Create settings file in source
	settingsFile := filepath.Join(claudeSource, "settings.local.json")
	if err := os.WriteFile(settingsFile, []byte(`{"test": true}`), 0644); err != nil {
		t.Fatalf("Failed to create settings file: %v", err)
	}

	task := New("test-task", agentDir)
	// Note: no WorktreeDir set (simulating non-worktree mode)

	// Setup symlink in project directory
	if err := task.SetupClaudeSymlinkInDir(pawDir, projectDir); err != nil {
		t.Fatalf("SetupClaudeSymlinkInDir() error = %v", err)
	}

	// Verify symlink was created in project directory
	claudeTargetPath := filepath.Join(projectDir, ".claude")
	fi, err := os.Lstat(claudeTargetPath)
	if err != nil {
		t.Fatalf("Failed to stat symlink: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("Expected symlink to be created in project directory")
	}

	// Verify symlink points to correct location
	target, err := os.Readlink(claudeTargetPath)
	if err != nil {
		t.Fatalf("Failed to readlink: %v", err)
	}
	// Should be relative path
	expected := filepath.Join("..", ".paw", ".claude")
	if target != expected {
		t.Errorf("Symlink target = %q, want %q", target, expected)
	}

	// Verify we can read the settings file through the symlink
	content, err := os.ReadFile(filepath.Join(claudeTargetPath, "settings.local.json"))
	if err != nil {
		t.Fatalf("Failed to read through symlink: %v", err)
	}
	if string(content) != `{"test": true}` {
		t.Errorf("Content through symlink = %q, want %q", content, `{"test": true}`)
	}
}

// TestSetupClaudeSymlinkInDirNonexistentTarget tests that no error is returned
// when target directory doesn't exist
func TestSetupClaudeSymlinkInDirNonexistentTarget(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agents", "test-task")
	projectDir := filepath.Join(tempDir, "nonexistent") // doesn't exist
	pawDir := filepath.Join(tempDir, ".paw")
	claudeSource := filepath.Join(pawDir, ".claude")

	// Create agent dir and claude source
	for _, dir := range []string{agentDir, claudeSource} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	task := New("test-task", agentDir)

	// Should not error - just return nil when target doesn't exist
	if err := task.SetupClaudeSymlinkInDir(pawDir, projectDir); err != nil {
		t.Errorf("SetupClaudeSymlinkInDir() should not error without target: %v", err)
	}
}

func TestRecoverStatusSignal(t *testing.T) {
	tests := []struct {
		name           string
		signalContent  string
		initialStatus  Status
		wantStatus     Status
		wantSignalGone bool
	}{
		{
			name:           "recover done signal",
			signalContent:  "done",
			initialStatus:  StatusWorking,
			wantStatus:     StatusDone,
			wantSignalGone: true,
		},
		{
			name:           "recover waiting signal",
			signalContent:  "waiting",
			initialStatus:  StatusWorking,
			wantStatus:     StatusWaiting,
			wantSignalGone: true,
		},
		{
			name:           "recover working signal",
			signalContent:  "working",
			initialStatus:  StatusWaiting,
			wantStatus:     StatusWorking,
			wantSignalGone: true,
		},
		{
			name:           "signal with whitespace",
			signalContent:  "  done\n",
			initialStatus:  StatusWorking,
			wantStatus:     StatusDone,
			wantSignalGone: true,
		},
		{
			name:           "invalid signal content deleted",
			signalContent:  "invalid-status",
			initialStatus:  StatusWorking,
			wantStatus:     StatusWorking, // unchanged
			wantSignalGone: true,          // invalid signals are deleted
		},
		{
			name:           "empty signal content deleted",
			signalContent:  "",
			initialStatus:  StatusWorking,
			wantStatus:     StatusWorking, // unchanged
			wantSignalGone: true,          // empty signals are deleted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			agentDir := filepath.Join(tempDir, "test-task")
			if err := os.MkdirAll(agentDir, 0755); err != nil {
				t.Fatalf("Failed to create agent dir: %v", err)
			}

			task := New("test-task", agentDir)

			// Set initial status
			if err := task.SaveStatus(tt.initialStatus); err != nil {
				t.Fatalf("Failed to save initial status: %v", err)
			}

			// Create signal file
			signalPath := task.GetStatusSignalPath()
			if err := os.WriteFile(signalPath, []byte(tt.signalContent), 0644); err != nil {
				t.Fatalf("Failed to write signal file: %v", err)
			}

			// Load status (should trigger recovery)
			status, err := task.LoadStatus()
			if err != nil {
				t.Fatalf("LoadStatus() error = %v", err)
			}

			// Check status
			if status != tt.wantStatus {
				t.Errorf("LoadStatus() = %q, want %q", status, tt.wantStatus)
			}

			// Check signal file was deleted
			_, err = os.Stat(signalPath)
			signalGone := os.IsNotExist(err)
			if signalGone != tt.wantSignalGone {
				t.Errorf("Signal file gone = %v, want %v", signalGone, tt.wantSignalGone)
			}
		})
	}
}

func TestRecoverStatusSignalNoFile(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "test-task")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("Failed to create agent dir: %v", err)
	}

	task := New("test-task", agentDir)

	// Set initial status
	if err := task.SaveStatus(StatusWorking); err != nil {
		t.Fatalf("Failed to save initial status: %v", err)
	}

	// No signal file exists

	// Load status
	status, err := task.LoadStatus()
	if err != nil {
		t.Fatalf("LoadStatus() error = %v", err)
	}

	// Status should remain unchanged
	if status != StatusWorking {
		t.Errorf("LoadStatus() = %q, want %q", status, StatusWorking)
	}
}

func TestGetStatusSignalPath(t *testing.T) {
	agentDir := "/path/to/agents/test-task"
	task := New("test-task", agentDir)

	expectedPath := filepath.Join(agentDir, constants.StatusSignalFileName)
	if task.GetStatusSignalPath() != expectedPath {
		t.Errorf("GetStatusSignalPath() = %q, want %q", task.GetStatusSignalPath(), expectedPath)
	}
}
