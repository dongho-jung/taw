// Package task provides task management functionality for PAW.
package task

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/fileutil"
)

// Status represents the status of a task.
type Status string

// Task status values.
const (
	StatusPending   Status = "pending"   // Task created, not yet started
	StatusWorking   Status = "working"   // Agent is working on the task
	StatusWaiting   Status = "waiting"   // Waiting for user input (merge conflict, etc.)
	StatusDone      Status = "done"      // Task completed and merged
	StatusCorrupted Status = "corrupted" // Task has issues that need recovery
)

// statusTransitions defines allowed status transitions.
var statusTransitions = map[Status]map[Status]bool{
	StatusPending: {
		StatusPending:   true,
		StatusWorking:   true,
		StatusWaiting:   true,
		StatusDone:      true,
		StatusCorrupted: true,
	},
	StatusWorking: {
		StatusWorking:   true,
		StatusWaiting:   true,
		StatusDone:      true,
		StatusCorrupted: true,
	},
	StatusWaiting: {
		StatusWaiting:   true,
		StatusWorking:   true,
		StatusDone:      true,
		StatusCorrupted: true,
	},
	StatusCorrupted: {
		StatusCorrupted: true,
		StatusWorking:   true,
	},
	StatusDone: {
		StatusDone:    true,
		StatusWorking: true, // Allow resuming work on a completed task
		StatusWaiting: true, // Allow moving back to waiting (e.g., PR review)
	},
}

// CorruptedReason represents why a task is corrupted.
type CorruptedReason string

// Corruption reason values.
const (
	CorruptMissingWorktree CorruptedReason = "missing_worktree" // Worktree directory doesn't exist
	CorruptNotInGit        CorruptedReason = "not_in_git"       // Worktree exists but not registered in git
	CorruptInvalidGit      CorruptedReason = "invalid_git"      // .git file is corrupted
	CorruptMissingBranch   CorruptedReason = "missing_branch"   // Branch doesn't exist
)

// Task represents a PAW task.
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

// GetTaskContextPath returns the path to the task context file.
func (t *Task) GetTaskContextPath() string {
	return filepath.Join(t.AgentDir, constants.TaskContextFileName)
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
	return filepath.Join(t.AgentDir, constants.WorktreeDirName)
}

// GetPRFilePath returns the path to the PR number file.
func (t *Task) GetPRFilePath() string {
	return filepath.Join(t.AgentDir, constants.PRFileName)
}

// GetSystemPromptPath returns the path to the system prompt file.
func (t *Task) GetSystemPromptPath() string {
	return filepath.Join(t.AgentDir, constants.AgentSystemPromptFile)
}

// GetUserPromptPath returns the path to the user prompt file.
func (t *Task) GetUserPromptPath() string {
	return filepath.Join(t.AgentDir, constants.AgentUserPromptFile)
}

// GetSessionMarkerPath returns the path to the session started marker file.
func (t *Task) GetSessionMarkerPath() string {
	return filepath.Join(t.AgentDir, constants.SessionStartedFile)
}

// GetHookOutputPath returns the output path for a named hook.
func (t *Task) GetHookOutputPath(name string) string {
	return filepath.Join(t.AgentDir, fmt.Sprintf(".hook-%s.log", name))
}

// GetHookMetaPath returns the metadata path for a named hook.
func (t *Task) GetHookMetaPath(name string) string {
	return filepath.Join(t.AgentDir, fmt.Sprintf(".hook-%s.json", name))
}

// GetVerifyOutputPath returns the output path for verification results.
func (t *Task) GetVerifyOutputPath() string {
	return filepath.Join(t.AgentDir, constants.VerifyLogFile)
}

// GetVerifyMetaPath returns the metadata path for verification results.
func (t *Task) GetVerifyMetaPath() string {
	return filepath.Join(t.AgentDir, constants.VerifyJSONFile)
}

// GetStatusFilePath returns the path to the status file.
func (t *Task) GetStatusFilePath() string {
	return filepath.Join(t.AgentDir, constants.StatusFileName)
}

// GetStatusSignalPath returns the path to the status signal file.
// This is a temp file that Claude writes to signal status changes directly.
func (t *Task) GetStatusSignalPath() string {
	return filepath.Join(t.AgentDir, constants.StatusSignalFileName)
}

// SaveStatus saves the task status to the status file.
func (t *Task) SaveStatus(status Status) error {
	t.Status = status
	return fileutil.WriteFileAtomic(t.GetStatusFilePath(), []byte(string(status)), 0644)
}

// LoadStatus loads the task status from the status file.
// It first checks for a stale status signal file and recovers it if present.
// This handles the case where the stop hook didn't run (e.g., session killed).
func (t *Task) LoadStatus() (Status, error) {
	// Recover any stale status signal before loading
	t.recoverStatusSignal()

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

// recoverStatusSignal checks for a stale .status-signal file and applies it.
// This is a recovery mechanism for when the stop hook didn't run
// (e.g., tmux session killed, Claude forcefully terminated).
// If a valid signal file exists, it updates .status and deletes the signal file.
func (t *Task) recoverStatusSignal() {
	signalPath := t.GetStatusSignalPath()
	data, err := os.ReadFile(signalPath) //nolint:gosec // G304: signalPath is from GetStatusSignalPath()
	if err != nil {
		return // No signal file or read error - nothing to recover
	}

	statusStr := strings.TrimSpace(string(data))
	status := Status(statusStr)

	// Validate the status (only accept valid runtime states)
	switch status { //nolint:exhaustive // StatusPending/StatusCorrupted are not valid signal values
	case StatusWorking, StatusWaiting, StatusDone:
		// Valid status - apply it
		if saveErr := t.SaveStatus(status); saveErr != nil {
			// Failed to save, keep signal file for next attempt
			return
		}
		// Successfully recovered - delete the signal file
		_ = os.Remove(signalPath)
	default:
		// Invalid status - delete the corrupt signal file
		_ = os.Remove(signalPath)
	}
}

// TransitionStatus updates task status and validates the transition.
// Returns previous status, whether the transition is valid, and any save error.
func (t *Task) TransitionStatus(next Status) (Status, bool, error) {
	if next == "" {
		return t.Status, false, errors.New("empty status")
	}

	prev, err := t.LoadStatus()
	if err != nil && !os.IsNotExist(err) {
		return t.Status, false, err
	}

	valid := IsValidStatusTransition(prev, next)
	if saveErr := t.SaveStatus(next); saveErr != nil {
		return prev, valid, saveErr
	}

	return prev, valid, nil
}

// IsValidStatusTransition returns true if a transition is allowed.
func IsValidStatusTransition(from, to Status) bool {
	if from == "" || to == "" {
		return false
	}
	if transitions, ok := statusTransitions[from]; ok {
		return transitions[to]
	}
	return false
}

// HasSessionMarker returns true if the session marker file exists.
func (t *Task) HasSessionMarker() bool {
	_, err := os.Stat(t.GetSessionMarkerPath())
	return err == nil
}

// CreateSessionMarker creates the session marker file.
func (t *Task) CreateSessionMarker() error {
	return fileutil.WriteFileAtomic(t.GetSessionMarkerPath(), []byte(time.Now().Format(time.RFC3339)), 0644)
}

// GetOriginPath returns the path to the origin symlink.
func (t *Task) GetOriginPath() string {
	return filepath.Join(t.AgentDir, constants.OriginLinkName)
}

// HasTabLock returns true if the tab-lock directory exists.
func (t *Task) HasTabLock() bool {
	_, err := os.Stat(t.GetTabLockDir())
	return err == nil
}

// CreateTabLock creates the tab-lock directory atomically.
// Returns true if created successfully, false if it already exists.
func (t *Task) CreateTabLock() (bool, error) {
	err := os.Mkdir(t.GetTabLockDir(), 0755) //nolint:gosec // G301: standard directory permissions
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
	return fileutil.WriteFileAtomic(t.GetWindowIDPath(), []byte(windowID), 0644)
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
	return fileutil.WriteFileAtomic(t.GetTaskFilePath(), []byte(content), 0644)
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
	return fileutil.WriteFileAtomic(t.GetPRFilePath(), []byte(strconv.Itoa(prNumber)), 0644)
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
// Note: StatusCorrupted maps to Waiting emoji.
func (t *Task) GetWindowName() string {
	emoji := constants.EmojiWorking
	switch t.Status { //nolint:exhaustive // StatusPending/StatusWorking use default (Working emoji)
	case StatusWaiting, StatusCorrupted:
		// Corrupted status displays as Waiting.
		emoji = constants.EmojiWaiting
	case StatusDone:
		emoji = constants.EmojiDone
	}

	return emoji + constants.TruncateForWindowName(t.Name)
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

// SetupClaudeSymlink creates a .claude symlink pointing to the PAW directory's
// .claude folder. The symlink is created in the agent directory (outside git
// worktree) to avoid accidental commits.
//
// Note: Claude Code is started with --settings flag pointing to this location,
// so it doesn't need to be in the working directory.
func (t *Task) SetupClaudeSymlink(pawDir string) error {
	return t.SetupClaudeSymlinkInDir(pawDir, "")
}

// SetupClaudeSymlinkInDir creates a .claude symlink in the specified target
// directory. If targetDir is empty, it defaults to the agent directory
// (which is outside the worktree for git mode).
// This is used by SetupClaudeSymlink for worktree mode, and can be called
// directly with the project directory for non-worktree mode.
func (t *Task) SetupClaudeSymlinkInDir(pawDir, targetDir string) error {
	if targetDir == "" {
		// Default to agent directory (outside git worktree)
		targetDir = t.AgentDir
		if targetDir == "" {
			return nil // No agent dir, nothing to do
		}
	}

	// Check if target directory exists
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return nil // Target doesn't exist yet, will be called later
	}

	claudeSource := filepath.Join(pawDir, constants.ClaudeLink)
	claudeTarget := filepath.Join(targetDir, constants.ClaudeLink)

	// Check if source .claude directory exists
	if _, err := os.Stat(claudeSource); os.IsNotExist(err) {
		return nil // Source doesn't exist, nothing to link
	}

	// Remove existing symlink/dir if present
	if err := os.Remove(claudeTarget); err != nil && !os.IsNotExist(err) {
		// If it's a directory, try to remove it
		if err := os.RemoveAll(claudeTarget); err != nil {
			return fmt.Errorf("failed to remove old .claude: %w", err)
		}
	}

	// Create relative symlink
	relPath, err := filepath.Rel(targetDir, claudeSource)
	if err != nil {
		relPath = claudeSource // Fallback to absolute path
	}

	if err := os.Symlink(relPath, claudeTarget); err != nil {
		return fmt.Errorf("failed to create .claude symlink: %w", err)
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
