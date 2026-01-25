// Package task provides task management functionality for PAW.
package task

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
)

// RecoveryManager handles recovery of corrupted tasks.
type RecoveryManager struct {
	projectDir string
	gitClient  git.Client
}

// NewRecoveryManager creates a new recovery manager.
func NewRecoveryManager(projectDir string) *RecoveryManager {
	return &RecoveryManager{
		projectDir: projectDir,
		gitClient:  git.New(),
	}
}

// RecoveryAction represents what action to take for a corrupted task.
type RecoveryAction string

// Recovery action options.
const (
	RecoveryRecover RecoveryAction = "recover" // Try to recover the task
	RecoveryCleanup RecoveryAction = "cleanup" // Clean up the task
	RecoveryCancel  RecoveryAction = "cancel"  // Do nothing
)

// RecoverTask attempts to recover a corrupted task.
func (r *RecoveryManager) RecoverTask(task *Task) error {
	if task == nil {
		return errors.New("task is nil")
	}

	logging.Log("Recovery: start task=%s reason=%s", task.Name, task.CorruptedReason)
	var err error
	switch task.CorruptedReason {
	case CorruptMissingWorktree:
		err = r.recoverMissingWorktree(task)
	case CorruptNotInGit:
		err = r.recoverNotInGit(task)
	case CorruptInvalidGit:
		err = r.recoverInvalidGit(task)
	case CorruptMissingBranch:
		err = r.recoverMissingBranch(task)
	default:
		err = fmt.Errorf("unknown corruption reason: %s", task.CorruptedReason)
	}

	if err != nil {
		logging.Warn("Recovery: failed task=%s reason=%s err=%v", task.Name, task.CorruptedReason, err)
		return err
	}

	logging.Log("Recovery: completed task=%s reason=%s", task.Name, task.CorruptedReason)
	return nil
}

// recoverMissingWorktree recreates a worktree from an existing branch.
func (r *RecoveryManager) recoverMissingWorktree(task *Task) error {
	worktreeDir := task.GetWorktreeDir()
	logging.Debug("Recovery: recreating missing worktree task=%s path=%s", task.Name, worktreeDir)

	// Branch exists, just recreate the worktree
	if err := r.gitClient.WorktreeAdd(r.projectDir, worktreeDir, task.Name, false); err != nil {
		return fmt.Errorf("failed to recreate worktree: %w", err)
	}

	return nil
}

// recoverNotInGit removes the directory and recreates the worktree.
func (r *RecoveryManager) recoverNotInGit(task *Task) error {
	worktreeDir := task.GetWorktreeDir()
	logging.Debug("Recovery: removing unregistered worktree task=%s path=%s", task.Name, worktreeDir)

	// Remove the unregistered directory
	if err := os.RemoveAll(worktreeDir); err != nil {
		return fmt.Errorf("failed to remove directory: %w", err)
	}

	// Prune worktrees
	if err := r.gitClient.WorktreePrune(r.projectDir); err != nil {
		logging.Trace("Recovery: worktree prune failed task=%s err=%v", task.Name, err)
	}

	// Recreate worktree
	createBranch := !r.gitClient.BranchExists(r.projectDir, task.Name)
	if err := r.gitClient.WorktreeAdd(r.projectDir, worktreeDir, task.Name, createBranch); err != nil {
		return fmt.Errorf("failed to recreate worktree: %w", err)
	}

	return nil
}

// recoverInvalidGit backs up files, removes directory, and recreates worktree.
func (r *RecoveryManager) recoverInvalidGit(task *Task) error {
	worktreeDir := task.GetWorktreeDir()
	backupDir := worktreeDir + ".backup"
	logging.Debug("Recovery: backing up invalid worktree task=%s path=%s", task.Name, worktreeDir)

	// Check if branch exists
	branchExists := r.gitClient.BranchExists(r.projectDir, task.Name)

	// Create backup
	if err := os.Rename(worktreeDir, backupDir); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Prune worktrees
	if err := r.gitClient.WorktreePrune(r.projectDir); err != nil {
		logging.Trace("Recovery: worktree prune failed task=%s err=%v", task.Name, err)
	}

	// Recreate worktree
	if err := r.gitClient.WorktreeAdd(r.projectDir, worktreeDir, task.Name, !branchExists); err != nil {
		// Restore backup on failure
		if restoreErr := os.Rename(backupDir, worktreeDir); restoreErr != nil {
			logging.Warn("Recovery: failed to restore backup task=%s err=%v", task.Name, restoreErr)
		}
		return fmt.Errorf("failed to recreate worktree: %w", err)
	}

	// Copy files from backup (excluding .git)
	if err := copyDirContents(backupDir, worktreeDir, []string{".git"}); err != nil {
		return fmt.Errorf("failed to restore files: %w", err)
	}

	// Remove backup
	if err := os.RemoveAll(backupDir); err != nil {
		logging.Trace("Recovery: failed to remove backup task=%s err=%v", task.Name, err)
	}

	return nil
}

// recoverMissingBranch creates a branch from the worktree HEAD.
func (r *RecoveryManager) recoverMissingBranch(task *Task) error {
	worktreeDir := task.GetWorktreeDir()
	logging.Debug("Recovery: recreating missing branch task=%s", task.Name)

	// Get HEAD commit from worktree
	headCommit, err := r.getWorktreeHead(worktreeDir)
	if err != nil {
		return fmt.Errorf("failed to get worktree HEAD: %w", err)
	}
	logging.Trace("Recovery: worktree head task=%s commit=%s", task.Name, headCommit)

	// Create branch at HEAD
	if err := r.gitClient.BranchCreate(r.projectDir, task.Name, headCommit); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	return nil
}

// getWorktreeHead gets the HEAD commit of a worktree.
func (r *RecoveryManager) getWorktreeHead(worktreeDir string) (string, error) {
	gitFile := filepath.Join(worktreeDir, ".git")

	// Read .git file to get gitdir
	data, err := os.ReadFile(gitFile) //nolint:gosec // G304: gitFile is constructed from worktreeDir
	if err != nil {
		return "", fmt.Errorf("failed to read .git file: %w", err)
	}

	// Parse gitdir line - format is "gitdir: /path/to/gitdir"
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "gitdir: ") {
		return "", errors.New("invalid .git file format: missing 'gitdir:' prefix")
	}
	gitdir := strings.TrimPrefix(content, "gitdir: ")
	gitdir = strings.TrimSpace(gitdir)

	if gitdir == "" {
		return "", errors.New("invalid .git file format: empty gitdir path")
	}

	// Read HEAD file from gitdir
	headFile := filepath.Join(gitdir, "HEAD")
	headData, err := os.ReadFile(headFile) //nolint:gosec // G304: headFile is constructed from validated gitdir
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD file: %w", err)
	}

	// HEAD could be a ref or a commit hash
	head := strings.TrimSpace(string(headData))
	if strings.HasPrefix(head, "ref: ") {
		// It's a reference, resolve it
		refName := strings.TrimPrefix(head, "ref: ")
		refName = strings.TrimSpace(refName)
		refPath := filepath.Join(gitdir, "..", refName)
		refData, err := os.ReadFile(refPath) //nolint:gosec // G304: refPath is constructed from validated git paths
		if err != nil {
			return "", fmt.Errorf("failed to resolve ref %s: %w", refName, err)
		}
		return strings.TrimSpace(string(refData)), nil
	}

	return head, nil
}

// GetRecoveryDescription returns a human-readable description of the corruption.
func GetRecoveryDescription(reason CorruptedReason) string {
	switch reason {
	case CorruptMissingWorktree:
		return "Worktree directory is missing but branch exists"
	case CorruptNotInGit:
		return "Worktree directory exists but is not registered in git"
	case CorruptInvalidGit:
		return "Worktree .git file is corrupted or invalid"
	case CorruptMissingBranch:
		return "Worktree exists but the branch is missing"
	default:
		return "Unknown corruption"
	}
}

// GetRecoveryAction returns the recommended action for a corruption type.
func GetRecoveryAction(reason CorruptedReason) string {
	switch reason {
	case CorruptMissingWorktree:
		return "Recreate worktree from existing branch"
	case CorruptNotInGit:
		return "Remove directory and recreate worktree"
	case CorruptInvalidGit:
		return "Backup files, recreate worktree, restore files"
	case CorruptMissingBranch:
		return "Create branch from worktree HEAD"
	default:
		return "Unknown action"
	}
}

// copyDirContents copies contents from src to dst, excluding specified paths.
func copyDirContents(src, dst string, exclude []string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Check exclusions
		for _, ex := range exclude {
			if relPath == ex || filepath.Base(relPath) == ex {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src) //nolint:gosec // G304: src is from validated file walk
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode()) //nolint:gosec // G304: dst is from validated file walk
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
