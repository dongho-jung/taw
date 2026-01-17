// Package task provides task management functionality for PAW.
package task

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dongho-jung/paw/internal/claude"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/github"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

// ErrTaskNotFound indicates that a task could not be found for the given lookup criteria.
var ErrTaskNotFound = errors.New("task not found")

// Manager handles task lifecycle operations.
type Manager struct {
	agentsDir    string
	projectDir   string
	pawDir       string
	isGitRepo    bool
	config       *config.Config
	tmuxClient   tmux.Client
	gitClient    git.Client
	ghClient     github.Client
	claudeClient claude.Client
}

// NewManager creates a new task manager.
func NewManager(agentsDir, projectDir, pawDir string, isGitRepo bool, cfg *config.Config) *Manager {
	return &Manager{
		agentsDir:    agentsDir,
		projectDir:   projectDir,
		pawDir:       pawDir,
		isGitRepo:    isGitRepo,
		config:       cfg,
		gitClient:    git.New(),
		ghClient:     github.New(),
		claudeClient: claude.New(),
	}
}

// SetTmuxClient sets the tmux client for the manager.
func (m *Manager) SetTmuxClient(client tmux.Client) {
	m.tmuxClient = client
}

// shouldUseWorktree returns true if the manager is configured to use git worktrees.
// PAW always uses worktree mode for git repositories.
func (m *Manager) shouldUseWorktree() bool {
	return m.isGitRepo
}

func (m *Manager) preferredWorktreeDir(task *Task) string {
	return filepath.Join(task.AgentDir, m.projectWorktreeName())
}

func (m *Manager) resolveWorktreeDir(task *Task) string {
	return m.preferredWorktreeDir(task)
}

func (m *Manager) projectWorktreeName() string {
	base := sanitizeWorktreeBase(filepath.Base(m.projectDir))
	hashSuffix := m.projectDirHashSuffix()
	// Use a short hash suffix to keep project workspace names stable and unique.
	return base + "-" + hashSuffix
}

func sanitizeWorktreeBase(base string) string {
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "worktree"
	}
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, base)
	if clean == "" {
		return "worktree"
	}
	if len(clean) > 32 {
		clean = clean[:32]
	}
	return clean
}

// sanitizeCustomBranchName sanitizes a custom branch name to match git branch naming rules.
// It converts to lowercase, replaces invalid characters with hyphens, and ensures valid format.
func sanitizeCustomBranchName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace spaces and underscores with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove any characters that aren't lowercase letters, numbers, or hyphens
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	name = result.String()

	// Remove leading/trailing hyphens
	name = strings.Trim(name, "-")

	// Collapse multiple hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	// Truncate if too long (max 32 chars for branch names)
	if len(name) > 32 {
		name = name[:32]
		name = strings.TrimSuffix(name, "-")
	}

	// Ensure minimum length
	if len(name) < 3 {
		return ""
	}

	return name
}

func (m *Manager) projectDirHashSuffix() string {
	path := m.projectDir
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
		path = resolved
	}
	if abs, err := filepath.Abs(path); err == nil && abs != "" {
		path = abs
	}
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])[:5]
}

// CreateTask creates a new task with the given content.
// It generates a task name using Claude and creates the task directory atomically.
// If customBranchName is non-empty, it uses that instead of generating a name.
func (m *Manager) CreateTask(content string, customBranchName ...string) (*Task, error) {
	logging.Debug("-> Manager.CreateTask(content_len=%d)", len(content))
	defer logging.Debug("<- Manager.CreateTask")

	var name string

	// Check for custom branch name
	if len(customBranchName) > 0 && customBranchName[0] != "" {
		name = sanitizeCustomBranchName(customBranchName[0])
		logging.Debug("Using custom branch name: %s", name)
	} else {
		// Generate task name using Claude
		logging.Trace("Generating task name with Claude: content_length=%d", len(content))
		timer := logging.StartTimer("task name generation")

		var err error
		name, err = m.claudeClient.GenerateTaskName(content)
		if err != nil {
			// Use fallback name if Claude fails
			fallbackName := fmt.Sprintf("task-%d", os.Getpid())
			timer.StopWithResult(false, fmt.Sprintf("error=%v, fallback=%s", err, fallbackName))
			logging.Warn("Claude name generation failed, using fallback: error=%v, fallback=%s", err, fallbackName)
			name = fallbackName
		} else {
			timer.StopWithResult(true, fmt.Sprintf("name=%s", name))
			logging.Debug("Task name generated: %s", name)
		}
	}

	// Create task directory atomically
	agentDir, err := m.createTaskDirectory(name)
	if err != nil {
		logging.Error("Failed to create task directory: %v", err)
		return nil, fmt.Errorf("failed to create task directory: %w", err)
	}
	logging.Debug("Task directory created: %s", agentDir)

	task := New(name, agentDir)
	if m.shouldUseWorktree() {
		task.WorktreeDir = m.preferredWorktreeDir(task)
	}

	// Save task content
	if err := task.SaveContent(content); err != nil {
		_ = task.Remove()
		logging.Error("Failed to save task content: %v", err)
		return nil, fmt.Errorf("failed to save task content: %w", err)
	}

	return task, nil
}

// createTaskDirectory creates a task directory atomically.
// If the name already exists (directory or git branch), it appends a number.
func (m *Manager) createTaskDirectory(baseName string) (string, error) {
	for i := 0; i <= 100; i++ {
		name := baseName
		if i > 0 {
			name = fmt.Sprintf("%s-%d", baseName, i)
		}

		// For git repos, also check if a branch with this name already exists
		// to avoid worktree creation failures later
		if m.isGitRepo && m.gitClient.BranchExists(m.projectDir, name) {
			logging.Debug("Branch already exists, trying next suffix: %s", name)
			continue
		}

		dir := filepath.Join(m.agentsDir, name)
		err := os.Mkdir(dir, 0755)
		if err == nil {
			return dir, nil
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	return "", fmt.Errorf("failed to create unique task directory after 100 attempts")
}

// GetTask retrieves a task by name.
func (m *Manager) GetTask(name string) (*Task, error) {
	agentDir := filepath.Join(m.agentsDir, name)
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("task not found: %s", name)
	}

	task := New(name, agentDir)

	// Load task content
	if _, err := task.LoadContent(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load task content: %w", err)
	}

	// Load window ID if exists (error is non-fatal)
	if task.HasTabLock() {
		_, _ = task.LoadWindowID()
	}

	// Load PR number if exists (error is non-fatal)
	_, _ = task.LoadPRNumber()

	// Set worktree directory
	if m.shouldUseWorktree() {
		task.WorktreeDir = m.resolveWorktreeDir(task)
	}

	return task, nil
}

// ListTasks returns all tasks in the agents directory.
func (m *Manager) ListTasks() ([]*Task, error) {
	entries, err := os.ReadDir(m.agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read agents directory: %w", err)
	}

	var tasks []*Task
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		task, err := m.GetTask(entry.Name())
		if err != nil {
			logging.Trace("ListTasks: failed to load task %s: %v", entry.Name(), err)
			continue
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}
