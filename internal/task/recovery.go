// Package task provides task management functionality for TAW.
package task

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/donghojung/taw/internal/git"
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

const (
	RecoveryRecover RecoveryAction = "recover" // Try to recover the task
	RecoveryCleanup RecoveryAction = "cleanup" // Clean up the task
	RecoveryCancel  RecoveryAction = "cancel"  // Do nothing
)

// RecoverTask attempts to recover a corrupted task.
func (r *RecoveryManager) RecoverTask(task *Task) error {
	switch task.CorruptedReason {
	case CorruptMissingWorktree:
		return r.recoverMissingWorktree(task)
	case CorruptNotInGit:
		return r.recoverNotInGit(task)
	case CorruptInvalidGit:
		return r.recoverInvalidGit(task)
	case CorruptMissingBranch:
		return r.recoverMissingBranch(task)
	default:
		return fmt.Errorf("unknown corruption reason: %s", task.CorruptedReason)
	}
}

// recoverMissingWorktree recreates a worktree from an existing branch.
func (r *RecoveryManager) recoverMissingWorktree(task *Task) error {
	worktreeDir := task.GetWorktreeDir()

	// Branch exists, just recreate the worktree
	if err := r.gitClient.WorktreeAdd(r.projectDir, worktreeDir, task.Name, false); err != nil {
		return fmt.Errorf("failed to recreate worktree: %w", err)
	}

	return nil
}

// recoverNotInGit removes the directory and recreates the worktree.
func (r *RecoveryManager) recoverNotInGit(task *Task) error {
	worktreeDir := task.GetWorktreeDir()

	// Remove the unregistered directory
	if err := os.RemoveAll(worktreeDir); err != nil {
		return fmt.Errorf("failed to remove directory: %w", err)
	}

	// Prune worktrees
	r.gitClient.WorktreePrune(r.projectDir)

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

	// Check if branch exists
	branchExists := r.gitClient.BranchExists(r.projectDir, task.Name)

	// Create backup
	if err := os.Rename(worktreeDir, backupDir); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Prune worktrees
	r.gitClient.WorktreePrune(r.projectDir)

	// Recreate worktree
	if err := r.gitClient.WorktreeAdd(r.projectDir, worktreeDir, task.Name, !branchExists); err != nil {
		// Restore backup on failure
		os.Rename(backupDir, worktreeDir)
		return fmt.Errorf("failed to recreate worktree: %w", err)
	}

	// Copy files from backup (excluding .git)
	if err := copyDirContents(backupDir, worktreeDir, []string{".git"}); err != nil {
		return fmt.Errorf("failed to restore files: %w", err)
	}

	// Remove backup
	os.RemoveAll(backupDir)

	return nil
}

// recoverMissingBranch creates a branch from the worktree HEAD.
func (r *RecoveryManager) recoverMissingBranch(task *Task) error {
	worktreeDir := task.GetWorktreeDir()

	// Get HEAD commit from worktree
	headCommit, err := r.getWorktreeHead(worktreeDir)
	if err != nil {
		return fmt.Errorf("failed to get worktree HEAD: %w", err)
	}

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
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("failed to read .git file: %w", err)
	}

	// Parse gitdir line - format is "gitdir: /path/to/gitdir"
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "gitdir: ") {
		return "", fmt.Errorf("invalid .git file format: missing 'gitdir:' prefix")
	}
	gitdir := strings.TrimPrefix(content, "gitdir: ")
	gitdir = strings.TrimSpace(gitdir)

	if gitdir == "" {
		return "", fmt.Errorf("invalid .git file format: empty gitdir path")
	}

	// Read HEAD file from gitdir
	headFile := filepath.Join(gitdir, "HEAD")
	headData, err := os.ReadFile(headFile)
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
		refData, err := os.ReadFile(refPath)
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
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
