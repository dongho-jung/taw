package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tui"
)

// resolveConflictsWithClaude attempts to resolve merge conflicts using Claude.
// It runs Claude with opus model and ultrathink enabled for better conflict resolution.
// Returns nil if conflicts were resolved, error otherwise.
func resolveConflictsWithClaude(projectDir, taskName, taskContent string, conflictFiles []string) error {
	if len(conflictFiles) == 0 {
		return nil
	}

	// Build the prompt for Claude with ultrathink prefix
	filesStr := strings.Join(conflictFiles, "\n  - ")
	prompt := fmt.Sprintf(`ultrathink You are resolving merge conflicts in a git repository.

## Conflicting Files
  - %s

## Task Context
Task name: %s
Task description:
%s

## Instructions
1. Read each conflicting file listed above
2. Look for conflict markers (<<<<<<< HEAD, =======, >>>>>>> branch)
3. Resolve each conflict by keeping the correct code that makes sense for the task
4. Save each resolved file using the Edit tool
5. After resolving ALL conflicts, run: git add -A

IMPORTANT:
- Do NOT abort or skip any files
- Resolve ALL conflicts before running git add
- Make sure the final code is valid and compiles
- If unsure, prefer keeping BOTH changes merged intelligently

Start resolving the conflicts now.`, filesStr, taskName, taskContent)

	logging.Debug("resolveConflictsWithClaude: starting conflict resolution for %d files with opus ultrathink", len(conflictFiles))
	logging.Trace("resolveConflictsWithClaude: prompt=%s", prompt)

	// Set a timeout for conflict resolution (10 minutes for ultrathink)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Run claude with opus model and ultrathink-enabled prompt
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", "opus", "--dangerously-skip-permissions")
	cmd.Dir = projectDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		logging.Warn("resolveConflictsWithClaude: claude command failed: %v", err)
		return fmt.Errorf("claude conflict resolution failed: %w", err)
	}

	logging.Debug("resolveConflictsWithClaude: claude completed successfully")
	return nil
}

// autoResolveMergeFailure attempts to resolve a general merge failure using Claude.
// This is called when merge fails but no explicit conflicts are detected, or when
// conflict resolution has failed. It uses opus model with ultrathink for comprehensive analysis.
// Returns nil if the issue was resolved, error otherwise.
func autoResolveMergeFailure(projectDir, taskName, taskContent, branchToMerge, mainBranch string, gitClient git.Client) error {
	// Get current git status for context
	statusOutput, _ := exec.Command("git", "-C", projectDir, "status").Output()

	// Build a comprehensive prompt for Claude with ultrathink prefix
	prompt := fmt.Sprintf(`ultrathink You are an expert at resolving git merge issues. A merge operation has failed and needs your help.

## Current Situation
- Project directory: %s
- Task branch: %s
- Target branch: %s

## Task Context
Task name: %s
Task description:
%s

## Current Git Status
%s

## Instructions
1. First, analyze the current git status and understand what went wrong
2. Check for any conflict markers in files (<<<<<<< HEAD, =======, >>>>>>> branch)
3. Check the git log to understand recent commits on both branches
4. Resolve any issues you find:
   - If there are conflicts, resolve them by editing the files
   - If there's a failed merge state, decide whether to complete or abort it
   - If files need to be staged, stage them with: git add -A
5. After resolving all issues, verify the repository is in a clean state
6. If you need to complete a merge, commit it with an appropriate message

IMPORTANT:
- Make sure the final code is valid and compiles
- Do NOT leave the repository in a broken state
- If you absolutely cannot resolve the issue, explain why clearly
- Prefer completing the merge over aborting if possible

Start analyzing and resolving the merge issue now.`, projectDir, branchToMerge, mainBranch, taskName, taskContent, string(statusOutput))

	logging.Debug("autoResolveMergeFailure: starting auto-resolution for task %s with opus ultrathink", taskName)
	logging.Trace("autoResolveMergeFailure: prompt length=%d", len(prompt))

	// Set a timeout for auto-resolution (10 minutes for ultrathink)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Run claude with opus model and ultrathink-enabled prompt
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", "opus", "--dangerously-skip-permissions")
	cmd.Dir = projectDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		logging.Warn("autoResolveMergeFailure: claude command failed: %v", err)
		return fmt.Errorf("auto merge resolution failed: %w", err)
	}

	// Verify the repository is in a clean state after Claude's intervention
	hasConflicts, conflictFiles, _ := gitClient.HasConflicts(projectDir)
	if hasConflicts && len(conflictFiles) > 0 {
		logging.Warn("autoResolveMergeFailure: conflicts still exist after Claude: %v", conflictFiles)
		return fmt.Errorf("conflicts still exist after auto-resolution: %v", conflictFiles)
	}

	hasOngoingMerge := gitClient.HasOngoingMerge(projectDir)
	if hasOngoingMerge {
		logging.Warn("autoResolveMergeFailure: merge still in progress after Claude")
		return fmt.Errorf("merge still in progress after auto-resolution")
	}

	logging.Debug("autoResolveMergeFailure: claude completed successfully, repository is clean")
	return nil
}

func loadTaskOptions(agentDir string) *config.TaskOptions {
	opts, err := config.LoadTaskOptions(agentDir)
	if err != nil {
		logging.Warn("Failed to load task options: %v", err)
		return config.DefaultTaskOptions()
	}
	return opts
}

// commitChangesIfNeeded commits any pending changes in the working directory.
// Returns true if changes were committed, false otherwise.
func commitChangesIfNeeded(gitClient git.Client, workDir string) bool {
	hasChanges := gitClient.HasChanges(workDir)
	logging.Trace("Git status: hasChanges=%v", hasChanges)

	if !hasChanges {
		fmt.Println("  ○ No changes to commit")
		return false
	}

	spinner := tui.NewSimpleSpinner("Committing changes")
	spinner.Start()

	commitTimer := logging.StartTimer("git commit")
	if err := gitClient.AddAll(workDir); err != nil {
		logging.Warn("Failed to add changes: %v", err)
	}

	// Layer 3: Prevent .claude symlink from being committed (final safety check)
	claudeStaged, err := gitClient.IsFileStaged(workDir, constants.ClaudeLink)
	if err != nil {
		logging.Warn("Failed to check if .claude is staged: %v", err)
	} else if claudeStaged {
		logging.Warn("Detected .claude in staging area, unstaging it to prevent commit")
		if err := gitClient.ResetPath(workDir, constants.ClaudeLink); err != nil {
			logging.Warn("Failed to unstage .claude: %v", err)
		} else {
			logging.Debug("Successfully unstaged .claude")
		}
	}

	diffStat, _ := gitClient.GetDiffStat(workDir)
	logging.Trace("Changes: %s", strings.ReplaceAll(diffStat, "\n", ", "))
	message := fmt.Sprintf(constants.CommitMessageAutoCommit, diffStat)
	if err := gitClient.Commit(workDir, message); err != nil {
		commitTimer.StopWithResult(false, err.Error())
		spinner.Stop(false, err.Error())
	} else {
		commitTimer.StopWithResult(true, "")
		spinner.Stop(true, "")
	}

	return true
}

func buildHistoryMetadata(appCtx *app.App, t *task.Task, opts *config.TaskOptions, gitClient git.Client, workDir string, verify *service.VerificationMetadata, hooks []service.HookMetadata) *service.HistoryMetadata {
	meta := &service.HistoryMetadata{
		TaskName:     t.Name,
		SessionName:  appCtx.SessionName,
		ProjectDir:   appCtx.ProjectDir,
		TaskOptions:  opts,
		Verification: verify,
		Hooks:        hooks,
	}

	now := time.Now()
	meta.FinishedAt = now.Format(time.RFC3339)
	if startedAt, ok := readSessionStart(t); ok {
		meta.StartedAt = startedAt.Format(time.RFC3339)
		meta.DurationSeconds = int64(now.Sub(startedAt).Seconds())
	}

	if appCtx.IsGitRepo && gitClient != nil && workDir != "" {
		commitHash, err := gitClient.GetHeadCommit(workDir)
		if err != nil {
			logging.Trace("Failed to read HEAD commit: %v", err)
		} else if commitHash != "" {
			branch, _ := gitClient.GetCurrentBranch(workDir)
			meta.Commit = &service.CommitMetadata{
				Hash:   commitHash,
				Branch: branch,
			}
		}
	}

	return meta
}

func readSessionStart(t *task.Task) (time.Time, bool) {
	data, err := os.ReadFile(t.GetSessionMarkerPath())
	if err != nil {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func collectHistoryArtifacts(t *task.Task) (*service.VerificationMetadata, []service.HookMetadata, map[string]string) {
	var hooks []service.HookMetadata
	outputs := make(map[string]string)

	hookNames := []string{"pre-task", "post-task", "pre-merge", "post-merge"}
	for _, name := range hookNames {
		metaPath := t.GetHookMetaPath(name)
		if meta := readHookMeta(metaPath); meta != nil {
			hooks = append(hooks, *meta)
		}
		outputPath := t.GetHookOutputPath(name)
		if output := readFileIfExists(outputPath); output != "" {
			outputs[name] = output
		}
	}

	verifyMeta := readVerificationMeta(t.GetVerifyMetaPath())
	if output := readFileIfExists(t.GetVerifyOutputPath()); output != "" {
		outputs["verify"] = output
	}

	return verifyMeta, hooks, outputs
}

func readHookMeta(path string) *service.HookMetadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var meta service.HookMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		logging.Trace("Failed to parse hook metadata %s: %v", path, err)
		return nil
	}
	return &meta
}

func readVerificationMeta(path string) *service.VerificationMetadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var meta service.VerificationMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		logging.Trace("Failed to parse verification metadata %s: %v", path, err)
		return nil
	}
	return &meta
}

func readFileIfExists(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func resolvePushBranch(gitClient git.Client, workDir, fallback string) (string, bool) {
	branchName, err := gitClient.GetCurrentBranch(workDir)
	if err == nil && branchName != "" && branchName != "HEAD" {
		return branchName, true
	}

	if err != nil {
		logging.Warn("Failed to determine current branch: %v", err)
	} else if branchName == "" {
		logging.Warn("Current branch name is empty for %s", workDir)
	} else {
		logging.Warn("Detected detached HEAD for %s", workDir)
	}

	if fallback == "" {
		return "", false
	}

	logging.Warn("Falling back to branch %s", fallback)
	return fallback, true
}

// isStaleLock checks if the merge lock file is stale (the process that created it is no longer running).
// Returns true if the lock is stale and should be removed.
func isStaleLock(lockFile string) bool {
	content, err := os.ReadFile(lockFile)
	if err != nil {
		// Can't read lock file - assume it's not stale
		return false
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) < 2 {
		// Invalid lock file format - consider it stale
		logging.Trace("Invalid lock file format (missing PID), treating as stale")
		return true
	}

	var pid int
	if _, err := fmt.Sscanf(lines[1], "%d", &pid); err != nil {
		// Can't parse PID - consider it stale
		logging.Trace("Invalid PID in lock file, treating as stale")
		return true
	}

	// Check if the process is still running by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		// Process doesn't exist - stale lock
		return true
	}

	// Send signal 0 to check if process exists (doesn't actually send a signal)
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process doesn't exist or we don't have permission (either way, treat as stale)
		logging.Trace("Lock holder process %d not running (err=%v), treating as stale", pid, err)
		return true
	}

	// Process is still running - lock is valid
	logging.Trace("Lock holder process %d is still running", pid)
	return false
}
