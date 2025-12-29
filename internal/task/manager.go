// Package task provides task management functionality for TAW.
package task

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/donghojung/taw/internal/claude"
	"github.com/donghojung/taw/internal/config"
	"github.com/donghojung/taw/internal/constants"
	"github.com/donghojung/taw/internal/git"
	"github.com/donghojung/taw/internal/github"
	"github.com/donghojung/taw/internal/logging"
	"github.com/donghojung/taw/internal/tmux"
)

// Manager handles task lifecycle operations.
type Manager struct {
	agentsDir   string
	projectDir  string
	tawDir      string
	isGitRepo   bool
	config      *config.Config
	tmuxClient  tmux.Client
	gitClient   git.Client
	ghClient    github.Client
	claudeClient claude.Client
}

// NewManager creates a new task manager.
func NewManager(agentsDir, projectDir, tawDir string, isGitRepo bool, cfg *config.Config) *Manager {
	return &Manager{
		agentsDir:   agentsDir,
		projectDir:  projectDir,
		tawDir:      tawDir,
		isGitRepo:   isGitRepo,
		config:      cfg,
		gitClient:   git.New(),
		ghClient:    github.New(),
		claudeClient: claude.New(),
	}
}

// SetTmuxClient sets the tmux client for the manager.
func (m *Manager) SetTmuxClient(client tmux.Client) {
	m.tmuxClient = client
}

// CreateTask creates a new task with the given content.
// It generates a task name using Claude and creates the task directory atomically.
func (m *Manager) CreateTask(content string) (*Task, error) {
	// Generate task name using Claude
	logging.Trace("Generating task name with Claude: content_length=%d", len(content))
	timer := logging.StartTimer("task name generation")

	name, err := m.claudeClient.GenerateTaskName(content)
	if err != nil {
		// Use fallback name if Claude fails
		fallbackName := fmt.Sprintf("task-%d", os.Getpid())
		timer.StopWithResult(false, fmt.Sprintf("error=%v, fallback=%s", err, fallbackName))
		logging.Warn("Claude name generation failed, using fallback: error=%v, fallback=%s", err, fallbackName)
		name = fallbackName
	} else {
		timer.StopWithResult(true, fmt.Sprintf("name=%s", name))
		logging.Log("Task name generated: %s", name)
	}

	// Create task directory atomically
	agentDir, err := m.createTaskDirectory(name)
	if err != nil {
		logging.Error("Failed to create task directory: %v", err)
		return nil, fmt.Errorf("failed to create task directory: %w", err)
	}
	logging.Log("Task directory created: %s", agentDir)

	task := New(name, agentDir)

	// Save task content
	if err := task.SaveContent(content); err != nil {
		task.Remove()
		logging.Error("Failed to save task content: %v", err)
		return nil, fmt.Errorf("failed to save task content: %w", err)
	}

	return task, nil
}

// createTaskDirectory creates a task directory atomically.
// If the name already exists, it appends a number.
func (m *Manager) createTaskDirectory(baseName string) (string, error) {
	for i := 0; i <= 100; i++ {
		name := baseName
		if i > 0 {
			name = fmt.Sprintf("%s-%d", baseName, i)
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
		if _, err := task.LoadWindowID(); err != nil {
			// Window ID file might be corrupted or missing - continue anyway
		}
	}

	// Load PR number if exists (error is non-fatal)
	if _, err := task.LoadPRNumber(); err != nil {
		// PR file might be corrupted - continue anyway
	}

	// Set worktree directory (with nil check for config)
	if m.isGitRepo && m.config != nil && m.config.WorkMode == config.WorkModeWorktree {
		task.WorktreeDir = task.GetWorktreeDir()
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
			continue
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// FindIncompleteTasks finds tasks that should have a window but don't.
// This includes tasks with tab-lock but no window, and tasks with worktree but no window.
func (m *Manager) FindIncompleteTasks(sessionName string) ([]*Task, error) {
	if m.tmuxClient == nil {
		return nil, fmt.Errorf("tmux client not set")
	}

	tasks, err := m.ListTasks()
	if err != nil {
		return nil, err
	}

	// Get list of active windows
	windows, err := m.tmuxClient.ListWindows()
	if err != nil {
		// Session might not exist yet
		windows = nil
	}

	// Build map of active window IDs and task names from window names
	activeWindowIDs := make(map[string]bool)
	activeTaskNames := make(map[string]bool)
	for _, w := range windows {
		activeWindowIDs[w.ID] = true
		if taskName, ok := constants.ExtractTaskName(w.Name); ok {
			activeTaskNames[taskName] = true
		}
	}

	// Get main branch for merged check
	mainBranch := ""
	if m.isGitRepo {
		mainBranch = m.gitClient.GetMainBranch(m.projectDir)
	}

	var incomplete []*Task
	for _, task := range tasks {
		// Skip if task already has a window (by task name)
		if activeTaskNames[task.Name] {
			continue
		}

		// Skip if task is merged
		if mainBranch != "" && m.isTaskMerged(task, mainBranch) {
			continue
		}

		// Check if this task should be reopened:
		// 1. Has tab-lock (was being handled) but window is gone
		// 2. Has worktree (in git mode) - task is active
		// 3. Has task content file - task exists
		shouldReopen := false

		if task.HasTabLock() {
			// Had a window before, check if it's still there
			windowID, err := task.LoadWindowID()
			if err != nil || !activeWindowIDs[windowID] {
				shouldReopen = true
			}
		} else if m.isGitRepo && m.config != nil && m.config.WorkMode == config.WorkModeWorktree {
			// In worktree mode, check if worktree exists
			worktreeDir := task.GetWorktreeDir()
			if _, err := os.Stat(worktreeDir); err == nil {
				// Worktree exists but no window - should reopen
				shouldReopen = true
			}
		}

		if shouldReopen {
			task.Status = StatusPending
			incomplete = append(incomplete, task)
		}
	}

	return incomplete, nil
}

// FindCorruptedTasks finds tasks with corrupted worktrees.
func (m *Manager) FindCorruptedTasks() ([]*Task, error) {
	if !m.isGitRepo || m.config == nil || m.config.WorkMode != config.WorkModeWorktree {
		return nil, nil
	}

	tasks, err := m.ListTasks()
	if err != nil {
		return nil, err
	}

	var corrupted []*Task
	for _, task := range tasks {
		reason := m.checkWorktreeStatus(task)
		if reason != "" {
			task.Status = StatusCorrupted
			task.CorruptedReason = reason
			corrupted = append(corrupted, task)
		}
	}

	return corrupted, nil
}

// checkWorktreeStatus checks the status of a task's worktree.
func (m *Manager) checkWorktreeStatus(task *Task) CorruptedReason {
	worktreeDir := task.GetWorktreeDir()

	// Check if worktree directory exists
	info, err := os.Stat(worktreeDir)
	if os.IsNotExist(err) {
		// Check if branch exists
		if m.gitClient.BranchExists(m.projectDir, task.Name) {
			return CorruptMissingWorktree
		}
		return "" // No worktree and no branch - task might be cleaned up
	}

	if !info.IsDir() {
		return CorruptInvalidGit
	}

	// Check if .git file exists
	gitFile := filepath.Join(worktreeDir, ".git")
	if _, err := os.Stat(gitFile); os.IsNotExist(err) {
		return CorruptInvalidGit
	}

	// Check if worktree is registered in git
	worktrees, err := m.gitClient.WorktreeList(m.projectDir)
	if err != nil {
		return CorruptNotInGit
	}

	registered := false
	for _, wt := range worktrees {
		if wt.Path == worktreeDir || strings.HasSuffix(wt.Path, "/"+filepath.Base(worktreeDir)) {
			registered = true
			break
		}
	}

	if !registered {
		return CorruptNotInGit
	}

	// Check if branch exists
	if !m.gitClient.BranchExists(m.projectDir, task.Name) {
		return CorruptMissingBranch
	}

	return "" // OK
}

// FindMergedTasks finds tasks whose branches have been merged.
func (m *Manager) FindMergedTasks() ([]*Task, error) {
	if !m.isGitRepo {
		return nil, nil
	}

	tasks, err := m.ListTasks()
	if err != nil {
		return nil, err
	}

	mainBranch := m.gitClient.GetMainBranch(m.projectDir)

	var merged []*Task
	for _, task := range tasks {
		if m.isTaskMerged(task, mainBranch) {
			task.Status = StatusDone
			merged = append(merged, task)
		}
	}

	return merged, nil
}

// isTaskMerged checks if a task has been merged or externally cleaned up.
func (m *Manager) isTaskMerged(task *Task, mainBranch string) bool {
	// Check if PR is merged
	if task.HasPR() {
		prNumber, err := task.LoadPRNumber()
		if err == nil && prNumber > 0 {
			merged, err := m.ghClient.IsPRMerged(m.projectDir, prNumber)
			if err == nil && merged {
				return true
			}
		}
	}

	// Check if branch is merged into main
	if m.gitClient.BranchMerged(m.projectDir, task.Name, mainBranch) {
		return true
	}

	// Check if task was externally cleaned up (branch and worktree both gone)
	// This handles cases where someone manually merged and cleaned up the task
	if m.config != nil && m.config.WorkMode == config.WorkModeWorktree {
		branchExists := m.gitClient.BranchExists(m.projectDir, task.Name)
		worktreeDir := task.GetWorktreeDir()
		_, worktreeErr := os.Stat(worktreeDir)
		worktreeExists := worktreeErr == nil

		if !branchExists && !worktreeExists {
			// Both branch and worktree are gone - task was cleaned up externally
			return true
		}
	}

	return false
}

// CleanupTask cleans up a task's resources.
func (m *Manager) CleanupTask(task *Task) error {
	if m.isGitRepo && m.config != nil && m.config.WorkMode == config.WorkModeWorktree {
		worktreeDir := task.GetWorktreeDir()

		// Remove worktree
		if _, err := os.Stat(worktreeDir); err == nil {
			if err := m.gitClient.WorktreeRemove(m.projectDir, worktreeDir, true); err != nil {
				logging.Trace("WorktreeRemove failed, trying force remove: %v", err)
				// Try force remove if normal remove fails
				if removeErr := os.RemoveAll(worktreeDir); removeErr != nil {
					logging.Warn("Force remove worktree failed: %v", removeErr)
				}
			}
		}

		// Prune worktrees (error is non-fatal)
		if err := m.gitClient.WorktreePrune(m.projectDir); err != nil {
			logging.Trace("WorktreePrune failed: %v", err)
		}

		// Delete branch (error is non-fatal)
		if m.gitClient.BranchExists(m.projectDir, task.Name) {
			if err := m.gitClient.BranchDelete(m.projectDir, task.Name, true); err != nil {
				logging.Trace("BranchDelete failed: %v", err)
			}
		}
	}

	// Remove agent directory
	return task.Remove()
}

// SetupWorktree creates a git worktree for the task.
func (m *Manager) SetupWorktree(task *Task) error {
	if !m.isGitRepo || m.config == nil || m.config.WorkMode != config.WorkModeWorktree {
		return nil
	}

	worktreeDir := task.GetWorktreeDir()
	task.WorktreeDir = worktreeDir

	// Stash any uncommitted changes (error is non-fatal)
	stashHash, _ := m.gitClient.StashCreate(m.projectDir)

	// Get untracked files (error is non-fatal)
	untrackedFiles, _ := m.gitClient.GetUntrackedFiles(m.projectDir)

	// Create worktree with new branch
	if err := m.gitClient.WorktreeAdd(m.projectDir, worktreeDir, task.Name, true); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	// Apply stash to worktree if there were changes (error is non-fatal)
	if stashHash != "" {
		if err := m.gitClient.StashApply(worktreeDir, stashHash); err != nil {
			// Stash apply can fail if there are conflicts - continue anyway
		}
	}

	// Copy untracked files to worktree (error is non-fatal)
	if len(untrackedFiles) > 0 {
		if err := git.CopyUntrackedFiles(untrackedFiles, m.projectDir, worktreeDir); err != nil {
			// Continue even if copying fails
		}
	}

	// Create .claude symlink in worktree (error is non-fatal)
	claudeLink := filepath.Join(worktreeDir, constants.ClaudeLink)
	claudeTarget := filepath.Join(m.tawDir, constants.ClaudeLink)
	if err := os.Symlink(claudeTarget, claudeLink); err != nil {
		// Symlink might already exist or fail for other reasons - continue anyway
	}

	// Execute worktree hook if configured (error is non-fatal)
	if m.config.WorktreeHook != "" {
		m.executeWorktreeHook(worktreeDir)
	}

	return nil
}

// executeWorktreeHook runs the configured worktree hook in the given directory.
func (m *Manager) executeWorktreeHook(worktreeDir string) {
	hook := m.config.WorktreeHook
	logging.Log("Executing worktree hook: %s", hook)

	cmd := exec.Command("sh", "-c", hook)
	cmd.Dir = worktreeDir
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Warn("Worktree hook failed: %v\n%s", err, string(output))
		return
	}

	logging.Log("Worktree hook completed successfully")
	if len(output) > 0 {
		logging.Trace("Worktree hook output: %s", string(output))
	}
}

// GetWorkingDirectory returns the working directory for a task.
func (m *Manager) GetWorkingDirectory(task *Task) string {
	if m.isGitRepo && m.config != nil && m.config.WorkMode == config.WorkModeWorktree {
		return task.GetWorktreeDir()
	}
	return m.projectDir
}

// FindOrphanedWindows finds tmux windows that have no corresponding agent directory.
// These are windows left behind after cleanup.
func (m *Manager) FindOrphanedWindows() ([]string, error) {
	if m.tmuxClient == nil {
		return nil, fmt.Errorf("tmux client not set")
	}

	windows, err := m.tmuxClient.ListWindows()
	if err != nil {
		return nil, err
	}

	var orphaned []string
	for _, w := range windows {
		taskName, isTaskWindow := constants.ExtractTaskName(w.Name)
		if !isTaskWindow {
			continue
		}

		// Check if agent directory exists
		agentDir := filepath.Join(m.agentsDir, taskName)
		if _, err := os.Stat(agentDir); os.IsNotExist(err) {
			// No agent directory - orphaned window
			orphaned = append(orphaned, w.ID)
		}
	}

	return orphaned, nil
}
