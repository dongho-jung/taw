package task

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
)

// FindTaskByWindowID returns the task associated with the given tmux window ID.
func (m *Manager) FindTaskByWindowID(windowID string) (*Task, error) {
	tasks, err := m.ListTasks()
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	for _, t := range tasks {
		id, loadErr := t.LoadWindowID()
		if loadErr != nil {
			if !errors.Is(loadErr, os.ErrNotExist) {
				logging.Trace("FindTaskByWindowID: failed to load window ID for %s: %v", t.Name, loadErr)
			}
			continue
		}
		if id == windowID {
			return t, nil
		}
	}

	return nil, fmt.Errorf("%w: window %s", ErrTaskNotFound, windowID)
}

// FindIncompleteTasks finds tasks that should have a window but don't.
// This includes tasks with tab-lock but no window, and tasks with worktree but no window.
func (m *Manager) FindIncompleteTasks(sessionName string) ([]*Task, error) {
	if m.tmuxClient == nil {
		err := fmt.Errorf("tmux client not set")
		logging.Error("FindIncompleteTasks: %v", err)
		return nil, err
	}

	tasks, err := m.ListTasks()
	if err != nil {
		return nil, err
	}

	// Get list of active windows
	windows, err := m.tmuxClient.ListWindows()
	if err != nil {
		// Session might not exist yet
		logging.Trace("FindIncompleteTasks: failed to list windows: %v", err)
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
		tokenName := constants.TruncateForWindowName(task.Name)
		legacyName := constants.LegacyTruncateForWindowName(task.Name)
		if activeTaskNames[tokenName] || activeTaskNames[legacyName] {
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
			if err != nil {
				logging.Trace("FindIncompleteTasks: failed to load window ID task=%s err=%v", task.Name, err)
			}
			if err != nil || !activeWindowIDs[windowID] {
				shouldReopen = true
			}
		} else if m.shouldUseWorktree() {
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
	if !m.shouldUseWorktree() {
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
		if err != nil {
			logging.Trace("isTaskMerged: failed to load PR number task=%s err=%v", task.Name, err)
		} else if prNumber > 0 {
			merged, err := m.ghClient.IsPRMerged(m.projectDir, prNumber)
			if err != nil {
				logging.Trace("isTaskMerged: PR status check failed task=%s pr=%d err=%v", task.Name, prNumber, err)
			} else if merged {
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
	if m.shouldUseWorktree() {
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

// FindOrphanedWindows finds tmux windows that have no corresponding agent directory.
// These are windows left behind after cleanup.
func (m *Manager) FindOrphanedWindows() ([]string, error) {
	if m.tmuxClient == nil {
		err := fmt.Errorf("tmux client not set")
		logging.Error("FindOrphanedWindows: %v", err)
		return nil, err
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

		// Use findTaskByTruncatedName to handle truncated window names correctly.
		// Window names are limited to MaxWindowNameLen chars, so we need to find
		// the task by matching the truncated name against all task directories.
		task, _ := m.findTaskByTruncatedName(taskName)
		if task == nil {
			// No matching task found - orphaned window
			orphaned = append(orphaned, w.ID)
		}
	}

	return orphaned, nil
}

// StoppedTaskInfo contains information about a task with a stopped agent.
type StoppedTaskInfo struct {
	Task     *Task
	WindowID string
}

// FindStoppedTasks finds tasks that have a window but Claude has stopped running.
// These are tasks where the window exists but the agent pane shows a shell prompt.
func (m *Manager) FindStoppedTasks() ([]*StoppedTaskInfo, error) {
	if m.tmuxClient == nil {
		err := fmt.Errorf("tmux client not set")
		logging.Error("FindStoppedTasks: %v", err)
		return nil, err
	}

	windows, err := m.tmuxClient.ListWindows()
	if err != nil {
		return nil, err
	}

	var stopped []*StoppedTaskInfo
	for _, w := range windows {
		taskName, isTaskWindow := constants.ExtractTaskName(w.Name)
		if !isTaskWindow {
			continue
		}

		// Skip done windows (task already completed)
		// Also skip legacy warning windows for backward compatibility
		if strings.HasPrefix(w.Name, constants.EmojiDone) ||
			strings.HasPrefix(w.Name, constants.EmojiWarning) {
			continue
		}

		// Get full task name by finding matching agent directory
		task, fullTaskName := m.findTaskByTruncatedName(taskName)
		if task == nil {
			continue
		}

		// Check if Claude is running in the agent pane (pane .0)
		agentPane := w.ID + ".0"
		if !m.claudeClient.IsClaudeRunning(m.tmuxClient, agentPane) {
			logging.Debug("FindStoppedTasks: task %s has stopped agent in window %s", fullTaskName, w.ID)
			stopped = append(stopped, &StoppedTaskInfo{
				Task:     task,
				WindowID: w.ID,
			})
		}
	}

	return stopped, nil
}

// FindTaskByTruncatedName finds a task whose name matches the truncated window name.
// This is useful when looking up tasks from window names which are limited to
// MaxWindowNameLen characters. Returns ErrTaskNotFound if no matching task is found.
func (m *Manager) FindTaskByTruncatedName(truncatedName string) (*Task, error) {
	task, _ := m.findTaskByTruncatedName(truncatedName)
	if task == nil {
		return nil, fmt.Errorf("%w: truncated name %s", ErrTaskNotFound, truncatedName)
	}
	return task, nil
}

// findTaskByTruncatedName finds a task whose name matches the truncated window name.
// Returns the task and its full name, or nil if not found.
func (m *Manager) findTaskByTruncatedName(truncatedName string) (*Task, string) {
	entries, err := os.ReadDir(m.agentsDir)
	if err != nil {
		logging.Trace("findTaskByTruncatedName: failed to read agents dir %s: %v", m.agentsDir, err)
		return nil, ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		fullName := entry.Name()
		// Check if this task name matches the truncated name
		if constants.MatchesWindowToken(truncatedName, fullName) {
			task, err := m.GetTask(fullName)
			if err != nil {
				logging.Trace("findTaskByTruncatedName: failed to load task %s: %v", fullName, err)
				continue
			}
			return task, fullName
		}
	}

	return nil, ""
}
