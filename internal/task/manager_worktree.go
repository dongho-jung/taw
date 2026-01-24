package task

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
)

func pathsMatch(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	resolvedA, errA := filepath.EvalSymlinks(a)
	resolvedB, errB := filepath.EvalSymlinks(b)
	if errA == nil && errB == nil {
		return filepath.Clean(resolvedA) == filepath.Clean(resolvedB)
	}
	return false
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
		logging.Warn("checkWorktreeStatus: worktree list failed task=%s err=%v", task.Name, err)
		return CorruptNotInGit
	}

	registered := false
	for _, wt := range worktrees {
		if pathsMatch(wt.Path, worktreeDir) {
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

// CleanupTask cleans up a task's resources.
func (m *Manager) CleanupTask(task *Task) error {
	if m.shouldUseWorktree() {
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
	err := task.Remove()

	// Invalidate truncated name cache since we removed a task
	m.InvalidateTruncatedNameCache()

	return err
}

// PruneWorktrees removes stale worktree entries from git's database.
// This should be called before any git operations to prevent errors
// when worktree directories have been deleted but git still references them.
func (m *Manager) PruneWorktrees() {
	if !m.shouldUseWorktree() {
		return
	}

	if err := m.gitClient.WorktreePrune(m.projectDir); err != nil {
		logging.Trace("WorktreePrune failed: %v", err)
	}
}

// SetupWorktree creates a git worktree for the task.
func (m *Manager) SetupWorktree(task *Task) error {
	if !m.shouldUseWorktree() {
		return nil
	}

	worktreeDir := task.GetWorktreeDir()
	task.WorktreeDir = worktreeDir

	// Stash any uncommitted changes (error is non-fatal)
	stashHash, err := m.gitClient.StashCreate(m.projectDir)
	if err != nil {
		logging.Warn("SetupWorktree: stash create failed: %v", err)
		stashHash = ""
	}

	// Get untracked files (error is non-fatal)
	untrackedFiles, err := m.gitClient.GetUntrackedFiles(m.projectDir)
	if err != nil {
		logging.Warn("SetupWorktree: failed to list untracked files: %v", err)
		untrackedFiles = nil
	}

	// Create worktree with new branch
	if err := m.gitClient.WorktreeAdd(m.projectDir, worktreeDir, task.Name, true); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	// Apply stash to worktree if there were changes (error is non-fatal)
	if stashHash != "" {
		if err := m.gitClient.StashApply(worktreeDir, stashHash); err != nil {
			logging.Warn("SetupWorktree: stash apply failed: %v", err)
		}
	}

	// Copy untracked files to worktree (error is non-fatal)
	if len(untrackedFiles) > 0 {
		if err := git.CopyUntrackedFiles(untrackedFiles, m.projectDir, worktreeDir); err != nil {
			logging.Warn("SetupWorktree: failed to copy untracked files: %v", err)
		}
	}

	// Create .claude symlink in agent directory (outside worktree, avoids git tracking)
	// Claude Code searches parent directories, so it will find .claude in AgentDir
	claudeLink := filepath.Join(filepath.Dir(worktreeDir), constants.ClaudeLink)
	claudeTarget := filepath.Join(m.pawDir, constants.ClaudeLink)
	if err := os.Symlink(claudeTarget, claudeLink); err != nil && !os.IsExist(err) {
		logging.Warn("SetupWorktree: failed to create claude symlink: %v", err)
	} else {
		logging.Debug("SetupWorktree: created .claude symlink in agent directory (outside git)")
	}

	// Execute pre-worktree hook if configured (error is non-fatal)
	if m.config.PreWorktreeHook != "" {
		m.executePreWorktreeHook(worktreeDir)
	}

	return nil
}

// executePreWorktreeHook runs the configured pre-worktree hook in the given directory.
func (m *Manager) executePreWorktreeHook(worktreeDir string) {
	hook := m.config.PreWorktreeHook
	logging.Debug("Executing pre-worktree hook: %s", hook)

	cmd := exec.Command("sh", "-c", hook)
	cmd.Dir = worktreeDir
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Warn("Pre-worktree hook failed: %v\n%s", err, string(output))
		return
	}

	logging.Debug("Pre-worktree hook completed successfully")
	if len(output) > 0 {
		logging.Trace("Pre-worktree hook output: %s", string(output))
	}
}

// GetWorkingDirectory returns the working directory for a task.
// For worktree mode: returns the worktree directory (git worktree)
// For non-worktree mode: returns the project directory (shared workspace)
func (m *Manager) GetWorkingDirectory(task *Task) string {
	if m.shouldUseWorktree() {
		return task.GetWorktreeDir()
	}
	// Non-worktree mode: Claude runs in the project directory.
	return m.projectDir
}
