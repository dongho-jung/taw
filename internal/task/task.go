// Package task provides task management functionality for TAW.
package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/donghojung/taw/internal/constants"
)

// Status represents the status of a task.
type Status string

const (
	StatusPending   Status = "pending"   // Task created, not yet started
	StatusWorking   Status = "working"   // Agent is working on the task
	StatusWaiting   Status = "waiting"   // Waiting for user input (merge conflict, etc.)
	StatusDone      Status = "done"      // Task completed and merged
	StatusCorrupted Status = "corrupted" // Task has issues that need recovery
)

// CorruptedReason represents why a task is corrupted.
type CorruptedReason string

const (
	CorruptMissingWorktree CorruptedReason = "missing_worktree" // Worktree directory doesn't exist
	CorruptNotInGit        CorruptedReason = "not_in_git"       // Worktree exists but not registered in git
	CorruptInvalidGit      CorruptedReason = "invalid_git"      // .git file is corrupted
	CorruptMissingBranch   CorruptedReason = "missing_branch"   // Branch doesn't exist
)

// Task represents a TAW task.
type Task struct {
	Name        string
	AgentDir    string
	WorktreeDir string
	WindowID    string
	Content     string
	Status      Status
	PRNumber    int
	CreatedAt   time.Time

	// For corrupted tasks
	CorruptedReason CorruptedReason
}

// New creates a new Task with the given name and agent directory.
func New(name, agentDir string) *Task {
	return &Task{
		Name:      name,
		AgentDir:  agentDir,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
}

// GetTaskFilePath returns the path to the task content file.
func (t *Task) GetTaskFilePath() string {
	return filepath.Join(t.AgentDir, constants.TaskFileName)
}

// GetTabLockDir returns the path to the tab-lock directory.
func (t *Task) GetTabLockDir() string {
	return filepath.Join(t.AgentDir, constants.TabLockDirName)
}

// GetWindowIDPath returns the path to the window_id file.
func (t *Task) GetWindowIDPath() string {
	return filepath.Join(t.GetTabLockDir(), constants.WindowIDFileName)
}

// GetWorktreeDir returns the path to the worktree directory.
func (t *Task) GetWorktreeDir() string {
	if t.WorktreeDir != "" {
		return t.WorktreeDir
	}
	return filepath.Join(t.AgentDir, "worktree")
}

// GetPRFilePath returns the path to the PR number file.
func (t *Task) GetPRFilePath() string {
	return filepath.Join(t.AgentDir, constants.PRFileName)
}

// GetSystemPromptPath returns the path to the system prompt file.
func (t *Task) GetSystemPromptPath() string {
	return filepath.Join(t.AgentDir, ".system-prompt")
}

// GetUserPromptPath returns the path to the user prompt file.
func (t *Task) GetUserPromptPath() string {
	return filepath.Join(t.AgentDir, ".user-prompt")
}

// GetSessionMarkerPath returns the path to the session started marker file.
func (t *Task) GetSessionMarkerPath() string {
	return filepath.Join(t.AgentDir, ".session-started")
}

// GetStatusFilePath returns the path to the status file.
func (t *Task) GetStatusFilePath() string {
	return filepath.Join(t.AgentDir, ".status")
}

// SaveStatus saves the task status to the status file.
func (t *Task) SaveStatus(status Status) error {
	t.Status = status
	return os.WriteFile(t.GetStatusFilePath(), []byte(string(status)), 0644)
}

// LoadStatus loads the task status from the status file.
func (t *Task) LoadStatus() (Status, error) {
	data, err := os.ReadFile(t.GetStatusFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return StatusPending, nil
		}
		return "", err
	}
	status := Status(strings.TrimSpace(string(data)))
	t.Status = status
	return status, nil
}

// HasSessionMarker returns true if the session marker file exists.
func (t *Task) HasSessionMarker() bool {
	_, err := os.Stat(t.GetSessionMarkerPath())
	return err == nil
}

// CreateSessionMarker creates the session marker file.
func (t *Task) CreateSessionMarker() error {
	return os.WriteFile(t.GetSessionMarkerPath(), []byte(time.Now().Format(time.RFC3339)), 0644)
}

// GetOriginPath returns the path to the origin symlink.
func (t *Task) GetOriginPath() string {
	return filepath.Join(t.AgentDir, "origin")
}

// HasTabLock returns true if the tab-lock directory exists.
func (t *Task) HasTabLock() bool {
	_, err := os.Stat(t.GetTabLockDir())
	return err == nil
}

// CreateTabLock creates the tab-lock directory atomically.
// Returns true if created successfully, false if it already exists.
func (t *Task) CreateTabLock() (bool, error) {
	err := os.Mkdir(t.GetTabLockDir(), 0755)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to create tab-lock: %w", err)
	}
	return true, nil
}

// RemoveTabLock removes the tab-lock directory.
func (t *Task) RemoveTabLock() error {
	return os.RemoveAll(t.GetTabLockDir())
}

// SaveWindowID saves the window ID to the window_id file.
func (t *Task) SaveWindowID(windowID string) error {
	t.WindowID = windowID
	return os.WriteFile(t.GetWindowIDPath(), []byte(windowID), 0644)
}

// LoadWindowID loads the window ID from the window_id file.
func (t *Task) LoadWindowID() (string, error) {
	data, err := os.ReadFile(t.GetWindowIDPath())
	if err != nil {
		return "", err
	}
	t.WindowID = strings.TrimSpace(string(data))
	return t.WindowID, nil
}

// SaveContent saves the task content to the task file.
func (t *Task) SaveContent(content string) error {
	t.Content = content
	return os.WriteFile(t.GetTaskFilePath(), []byte(content), 0644)
}

// LoadContent loads the task content from the task file.
func (t *Task) LoadContent() (string, error) {
	data, err := os.ReadFile(t.GetTaskFilePath())
	if err != nil {
		return "", err
	}
	t.Content = string(data)
	return t.Content, nil
}

// SavePRNumber saves the PR number to the .pr file.
func (t *Task) SavePRNumber(prNumber int) error {
	t.PRNumber = prNumber
	return os.WriteFile(t.GetPRFilePath(), []byte(fmt.Sprintf("%d", prNumber)), 0644)
}

// LoadPRNumber loads the PR number from the .pr file.
func (t *Task) LoadPRNumber() (int, error) {
	data, err := os.ReadFile(t.GetPRFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var prNumber int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &prNumber); err != nil {
		return 0, err
	}

	t.PRNumber = prNumber
	return prNumber, nil
}

// HasPR returns true if the task has a PR number.
func (t *Task) HasPR() bool {
	_, err := os.Stat(t.GetPRFilePath())
	return err == nil
}

// GetWindowName returns the window name with status emoji.
func (t *Task) GetWindowName() string {
	emoji := constants.EmojiWorking
	switch t.Status {
	case StatusWaiting:
		emoji = constants.EmojiWaiting
	case StatusDone:
		emoji = constants.EmojiDone
	case StatusCorrupted:
		emoji = constants.EmojiWarning
	}

	name := t.Name
	if len(name) > 12 {
		name = name[:12]
	}

	return emoji + name
}

// SetupSymlinks creates the origin symlink.
func (t *Task) SetupSymlinks(projectDir string) error {
	// Create origin symlink to project root
	originPath := t.GetOriginPath()
	if err := os.Remove(originPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old origin symlink: %w", err)
	}

	relPath, err := filepath.Rel(t.AgentDir, projectDir)
	if err != nil {
		relPath = projectDir
	}

	if err := os.Symlink(relPath, originPath); err != nil {
		return fmt.Errorf("failed to create origin symlink: %w", err)
	}

	return nil
}

// Exists checks if the task directory exists.
func (t *Task) Exists() bool {
	_, err := os.Stat(t.AgentDir)
	return err == nil
}

// Remove removes the task directory.
func (t *Task) Remove() error {
	return os.RemoveAll(t.AgentDir)
}
