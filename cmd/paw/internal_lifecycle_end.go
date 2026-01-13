package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/notify"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var paneCaptureFile string
var endTaskUserInitiated bool
var endTaskAction string // merge, pr, keep (default), drop

var endTaskCmd = &cobra.Command{
	Use:   "end-task [session] [window-id]",
	Short: "Finish a task (commit, merge, cleanup)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> endTaskCmd(session=%s, windowID=%s)", args[0], args[1])
		defer logging.Debug("<- endTaskCmd")

		sessionName := args[0]
		windowID := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Find task by window ID
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		targetTask, err := mgr.FindTaskByWindowID(windowID)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "end-task", targetTask.Name)
		defer cleanup()

		tm := tmux.New(sessionName)
		if !endTaskUserInitiated {
			message := "Finish is user-initiated. Press Ctrl+F to finish this task."
			logging.Warn("endTaskCmd: blocked (not user initiated)")
			fmt.Printf("\n  ⚠️  %s\n\n", message)
			if paneCaptureFile != "" {
				_ = os.Remove(paneCaptureFile)
			}
			_ = tm.DisplayMessage(message, 3000)
			return nil
		}

		logging.Log("=== Finish task: %s ===", targetTask.Name)

		// Print task header for user feedback
		fmt.Printf("\n  Finishing task: %s\n\n", targetTask.Name)

		gitClient := git.New()
		workDir := mgr.GetWorkingDirectory(targetTask)
		logging.Trace("Working directory: %s", workDir)
		logging.Debug("Configuration: Action=%s", endTaskAction)

		// Handle drop action - discard changes and cleanup
		skipGitOps := (endTaskAction == "drop")
		if skipGitOps {
			fmt.Println("  Dropping task (discarding changes)...")
			logging.Log("drop action: discarding changes for task %s", targetTask.Name)
		}

		// Commit changes if git mode (skip for drop action)
		if appCtx.IsGitRepo && !skipGitOps {
			commitChangesIfNeeded(gitClient, workDir)

			// Handle action-based behavior
			switch endTaskAction {
			case "pr":
				// Push and create PR
				if !appCtx.IsWorktreeMode() {
					logging.Warn("PR creation requested in non-worktree mode; skipping")
					fmt.Println("  ⚠️  PR creation is only available in worktree mode")
				} else {
					fallbackBranch := targetTask.Name
					branchName, ok := resolvePushBranch(gitClient, workDir, fallbackBranch)
					if ok {
						pushSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Pushing %s to remote", branchName))
						pushSpinner.Start()

						pushTimer := logging.StartTimer("git push")
						if err := gitClient.Push(workDir, "origin", branchName, true); err != nil {
							pushTimer.StopWithResult(false, err.Error())
							pushSpinner.Stop(false, err.Error())
						} else {
							pushTimer.StopWithResult(true, fmt.Sprintf("branch=%s", branchName))
							pushSpinner.Stop(true, branchName)
						}
					}
				}

			case "merge":
				// Auto-merge
				if !appCtx.IsWorktreeMode() {
					logging.Warn("merge requested in non-worktree mode; skipping merge")
					fmt.Println()
					fmt.Println("  ⚠️  Merge is only available in worktree mode")
				} else {
					mergeSuccess := runAutoMerge(appCtx, targetTask, windowID, workDir, gitClient, tm)
					if !mergeSuccess {
						return nil // Exit without cleanup - keep worktree and branch
					}
				}

			default:
				// "keep" or empty - just commit (already done above)
				fmt.Println("  ○ Changes committed")
			}
		}
		fmt.Println()

		// Skip post-task processing for drop action
		if !skipGitOps {
			if appCtx.Config != nil && appCtx.Config.PostTaskHook != "" {
				hookEnv := appCtx.GetEnvVars(targetTask.Name, workDir, windowID)
				if _, err := service.RunHook(
					"post-task",
					appCtx.Config.PostTaskHook,
					workDir,
					hookEnv,
					targetTask.GetHookOutputPath("post-task"),
					targetTask.GetHookMetaPath("post-task"),
					constants.DefaultHookTimeout,
				); err != nil {
					logging.Warn("Post-task hook failed: %v", err)
				}
			}

			// Save to history using service
			historyService := service.NewHistoryService(appCtx.GetHistoryDir())

			// Get pane content: either from pre-captured file or capture now
			var paneContent string
			var captureErr error
			if paneCaptureFile != "" {
				// Use pre-captured content (from end-task-ui)
				content, err := os.ReadFile(paneCaptureFile)
				if err != nil {
					logging.Warn("Failed to read pane capture file: %v", err)
					// Try to capture directly as fallback
					paneContent, captureErr = tm.CapturePane(windowID+".0", constants.PaneCaptureLines)
				} else {
					paneContent = string(content)
					logging.Debug("Using pre-captured pane content from: %s", paneCaptureFile)
				}
				// Clean up temp file
				_ = os.Remove(paneCaptureFile)
			} else {
				// Capture pane content directly
				paneContent, captureErr = tm.CapturePane(windowID+".0", constants.PaneCaptureLines)
			}

			if captureErr != nil {
				logging.Warn("Failed to capture pane content: %v", captureErr)
			}

			// Save history
			taskContent, _ := targetTask.LoadContent()
			taskOpts := loadTaskOptions(targetTask.AgentDir)
			verifyMeta, hookMetas, hookOutputs := collectHistoryArtifacts(targetTask)
			meta := buildHistoryMetadata(appCtx, targetTask, taskOpts, gitClient, workDir, verifyMeta, hookMetas)
			if err := historyService.SaveCompletedWithDetails(targetTask.Name, taskContent, paneContent, meta, hookOutputs); err != nil {
				logging.Warn("Failed to save history: %v", err)
			}

			// Run self-improve if enabled
			if appCtx.Config != nil && appCtx.Config.SelfImprove && appCtx.IsGitRepo {
				selfImproveSpinner := tui.NewSimpleSpinner("Running self-improve analysis")
				selfImproveSpinner.Start()

				if err := runSelfImprove(appCtx.ProjectDir, targetTask.Name, taskContent, paneContent, gitClient); err != nil {
					selfImproveSpinner.Stop(false, err.Error())
					logging.Warn("Self-improve failed: %v", err)
				} else {
					selfImproveSpinner.Stop(true, "")
				}
			}
		} else {
			// Clean up temp file for drop action too
			if paneCaptureFile != "" {
				_ = os.Remove(paneCaptureFile)
			}
		}

		// Notify user that task completed successfully
		logging.Trace("endTaskCmd: playing SoundTaskCompleted for task=%s", targetTask.Name)
		notify.PlaySound(notify.SoundTaskCompleted)
		// Send desktop notification
		_ = notify.Send("Task completed", fmt.Sprintf("✅ %s completed successfully", targetTask.Name))
		logging.Trace("endTaskCmd: displaying completion message for task=%s", targetTask.Name)
		if err := tm.DisplayMessage(fmt.Sprintf("✅ Task completed: %s", targetTask.Name), 2000); err != nil {
			logging.Trace("Failed to display message: %v", err)
		}

		// Cleanup task (only reached if merge succeeded or not in auto-merge mode)
		cleanupSpinner := tui.NewSimpleSpinner("Cleaning up")
		cleanupSpinner.Start()

		cleanupTimer := logging.StartTimer("task cleanup")
		if err := mgr.CleanupTask(targetTask); err != nil {
			cleanupTimer.StopWithResult(false, err.Error())
			cleanupSpinner.Stop(false, err.Error())
		} else {
			cleanupTimer.StopWithResult(true, "")
			cleanupSpinner.Stop(true, "")
		}

		// Kill window
		if err := tm.KillWindow(windowID); err != nil {
			logging.Warn("Failed to kill window: %v", err)
		}

		fmt.Println()
		fmt.Println("  ✓ Done!")

		return nil
	},
}

var endTaskUICmd = &cobra.Command{
	Use:   "end-task-ui [session] [window-id]",
	Short: "Finish task with UI feedback (creates visible pane)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "end-task-ui", "")
		defer cleanup()

		logging.Debug("-> endTaskUICmd(session=%s, windowID=%s)", sessionName, windowID)
		defer logging.Debug("<- endTaskUICmd")

		tm := tmux.New(sessionName)

		// IMPORTANT: Capture the agent pane content BEFORE creating the split pane
		// This is necessary because splitting shifts pane indices, causing windowID+".0"
		// to no longer be the agent pane
		paneContent, err := tm.CapturePane(windowID+".0", constants.PaneCaptureLines)
		if err != nil {
			logging.Warn("Failed to pre-capture agent pane: %v", err)
			paneContent = "" // Continue anyway, end-task will try to capture directly
		}

		// Save captured content to temp file if we got it
		var capturePath string
		if paneContent != "" {
			tmpFile, err := os.CreateTemp("", "paw-pane-capture-*.txt")
			if err != nil {
				logging.Warn("Failed to create temp file for pane capture: %v", err)
			} else {
				if _, err := tmpFile.WriteString(paneContent); err != nil {
					logging.Warn("Failed to write pane capture to temp file: %v", err)
					_ = tmpFile.Close()
					_ = os.Remove(tmpFile.Name())
				} else {
					capturePath = tmpFile.Name()
					_ = tmpFile.Close()
					logging.Debug("Pre-captured agent pane to: %s", capturePath)
				}
			}
		}

		// Get the paw binary path
		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Get working directory from pane
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			panePath = appCtx.ProjectDir
		}

		// Build end-task command that runs in the pane
		// Include pane-capture-file flag if we have pre-captured content
		// CRITICAL: Pass PAW_DIR as env var so end-task can find the correct project
		// even if the agent changed its working directory (e.g., cd /tmp)
		var endTaskCmdStr string
		if capturePath != "" {
			endTaskCmdStr = fmt.Sprintf("PAW_DIR='%s' %s internal end-task --user-initiated --action=%s --pane-capture-file=%q %s %s; echo; echo 'Press Enter to close...'; read",
				appCtx.PawDir, pawBin, endTaskAction, capturePath, sessionName, windowID)
		} else {
			endTaskCmdStr = fmt.Sprintf("PAW_DIR='%s' %s internal end-task --user-initiated --action=%s %s %s; echo; echo 'Press Enter to close...'; read",
				appCtx.PawDir, pawBin, endTaskAction, sessionName, windowID)
		}

		// Create a top pane (40% height) spanning full window width
		_, err = tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false, // vertical split (top/bottom)
			Size:       "40%",
			StartDir:   panePath,
			Command:    endTaskCmdStr,
			Before:     true, // create pane above (top)
			Full:       true, // span entire window width
		})
		if err != nil {
			// Clean up temp file if we created one
			if capturePath != "" {
				_ = os.Remove(capturePath)
			}
			return fmt.Errorf("failed to create end-task pane: %w", err)
		}

		return nil
	},
}

// runAutoMerge performs the auto-merge process for a task.
// Returns true if merge succeeded, false if failed.
func runAutoMerge(appCtx *app.App, targetTask *task.Task, windowID, workDir string, gitClient git.Client, tm tmux.Client) bool {
	logging.Log("auto-merge: starting merge process")
	fmt.Println()
	fmt.Println("  Auto-merge mode:")

	// Get main branch name
	mainBranch := gitClient.GetMainBranch(appCtx.ProjectDir)
	logging.Debug("Main branch: %s", mainBranch)

	if appCtx.Config != nil && appCtx.Config.PreMergeHook != "" {
		hookEnv := appCtx.GetEnvVars(targetTask.Name, workDir, windowID)
		if _, err := service.RunHook(
			"pre-merge",
			appCtx.Config.PreMergeHook,
			appCtx.ProjectDir,
			hookEnv,
			targetTask.GetHookOutputPath("pre-merge"),
			targetTask.GetHookMetaPath("pre-merge"),
			constants.DefaultHookTimeout,
		); err != nil {
			logging.Warn("Pre-merge hook failed: %v", err)
		}
	}

	mergeTimer := logging.StartTimer("auto-merge")

	// Acquire merge lock to prevent concurrent merges
	lockSpinner := tui.NewSimpleSpinner("Acquiring merge lock")
	lockSpinner.Start()

	lockFile := filepath.Join(appCtx.PawDir, "merge.lock")
	lockAcquired := acquireMergeLock(lockFile, targetTask.Name)

	if !lockAcquired {
		logging.Warn("Failed to acquire merge lock after %d seconds", constants.MergeLockMaxRetries)
		mergeTimer.StopWithResult(false, "lock timeout")
		lockSpinner.Stop(false, fmt.Sprintf("timeout after %ds", constants.MergeLockMaxRetries))
		return handleMergeFailure(appCtx, targetTask, windowID, tm)
	}
	lockSpinner.Stop(true, "")
	defer func() { _ = os.Remove(lockFile) }()

	// Check for ongoing merge or conflicts in project dir
	hasConflicts, conflictFiles, _ := gitClient.HasConflicts(appCtx.ProjectDir)
	hasOngoingMerge := gitClient.HasOngoingMerge(appCtx.ProjectDir)

	if hasConflicts || hasOngoingMerge {
		logging.Warn("Project directory has ongoing merge or conflicts")
		fmt.Println()
		fmt.Println("  ⚠️  Project directory has unresolved conflicts or ongoing merge")
		if hasConflicts && len(conflictFiles) > 0 {
			fmt.Println("  Conflicting files:")
			for _, f := range conflictFiles {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println()
		fmt.Println("  Please resolve conflicts in the project directory first:")
		fmt.Printf("    cd %s\n", appCtx.ProjectDir)
		fmt.Println("    git status  # View current state")
		fmt.Println("    # Resolve conflicts, then: git add . && git commit")
		fmt.Println("    # Or abort merge: git merge --abort")
		fmt.Println()
		mergeTimer.StopWithResult(false, "project has conflicts")
		return handleMergeFailure(appCtx, targetTask, windowID, tm)
	}

	// Stash any uncommitted changes in project dir
	hasLocalChanges := gitClient.HasChanges(appCtx.ProjectDir)
	if hasLocalChanges {
		logging.Debug("Stashing local changes...")
		if err := gitClient.StashPush(appCtx.ProjectDir, "paw-merge-temp"); err != nil {
			logging.Warn("Failed to stash changes: %v", err)
		}
	}

	// Remember current branch to restore later
	currentBranch, _ := gitClient.GetCurrentBranch(appCtx.ProjectDir)

	// Perform the actual merge
	mergeSuccess := performMerge(appCtx, targetTask, windowID, workDir, mainBranch, currentBranch, hasLocalChanges, gitClient, mergeTimer)

	// Restore stashed changes
	if hasLocalChanges {
		logging.Debug("Restoring stashed changes...")
		if err := gitClient.StashPop(appCtx.ProjectDir); err != nil {
			logging.Warn("Failed to restore stashed changes: %v", err)
		}
	}

	if !mergeSuccess {
		return handleMergeFailure(appCtx, targetTask, windowID, tm)
	}

	return true
}

// acquireMergeLock attempts to acquire the merge lock file.
func acquireMergeLock(lockFile, taskName string) bool {
	for retries := 0; retries < constants.MergeLockMaxRetries; retries++ {
		f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			_, writeErr := fmt.Fprintf(f, "%s\n%d", taskName, os.Getpid())
			closeErr := f.Close()
			if writeErr != nil || closeErr != nil {
				_ = os.Remove(lockFile)
				logging.Warn("Failed to write lock file: write=%v, close=%v", writeErr, closeErr)
				time.Sleep(constants.MergeLockRetryInterval)
				continue
			}
			return true
		}

		if isStaleLock(lockFile) {
			logging.Debug("Detected stale merge lock, removing...")
			if rmErr := os.Remove(lockFile); rmErr != nil {
				logging.Warn("Failed to remove stale lock: %v", rmErr)
			}
			continue
		}

		logging.Trace("Waiting for merge lock (attempt %d/%d)...", retries+1, constants.MergeLockMaxRetries)
		time.Sleep(constants.MergeLockRetryInterval)
	}
	return false
}

// performMerge executes the git merge operation.
func performMerge(appCtx *app.App, targetTask *task.Task, windowID, workDir, mainBranch, currentBranch string, hasLocalChanges bool, gitClient git.Client, mergeTimer *logging.Timer) bool {
	// Fetch latest from origin
	fetchSpinner := tui.NewSimpleSpinner("Fetching from origin")
	fetchSpinner.Start()
	logging.Debug("Fetching from origin...")
	if err := gitClient.Fetch(appCtx.ProjectDir, "origin"); err != nil {
		logging.Warn("Failed to fetch: %v", err)
		fetchSpinner.Stop(false, err.Error())
	} else {
		fetchSpinner.Stop(true, "")
	}

	// Checkout main
	checkoutSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Checking out %s", mainBranch))
	checkoutSpinner.Start()
	logging.Debug("Checking out %s...", mainBranch)
	if err := gitClient.Checkout(appCtx.ProjectDir, mainBranch); err != nil {
		logging.Warn("Failed to checkout %s: %v", mainBranch, err)
		mergeTimer.StopWithResult(false, "checkout failed")
		checkoutSpinner.Stop(false, err.Error())
		return false
	}
	checkoutSpinner.Stop(true, "")

	// Pull latest
	pullSpinner := tui.NewSimpleSpinner("Pulling latest changes")
	pullSpinner.Start()
	logging.Debug("Pulling latest changes...")
	if err := gitClient.Pull(appCtx.ProjectDir); err != nil {
		logging.Warn("Failed to pull: %v", err)
		pullSpinner.Stop(false, err.Error())
	} else {
		pullSpinner.Stop(true, "")
	}

	// Merge task branch (squash)
	mergeSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Merging %s into %s", targetTask.Name, mainBranch))
	mergeSpinner.Start()
	logging.Debug("Squash merging branch %s into %s...", targetTask.Name, mainBranch)

	branchCommits, _ := gitClient.GetBranchCommits(appCtx.ProjectDir, targetTask.Name, mainBranch, 20)
	mergeMsg := git.GenerateMergeCommitMessage(targetTask.Name, branchCommits)
	mergeConflictOccurred := false
	mergeSuccess := true

	if err := gitClient.MergeSquash(appCtx.ProjectDir, targetTask.Name, mergeMsg); err != nil {
		logging.Warn("Merge failed: %v - checking for conflicts", err)
		mergeSpinner.Stop(false, "conflict")
		mergeConflictOccurred = true

		mergeSuccess = handleMergeConflicts(appCtx, targetTask, mainBranch, mergeMsg, gitClient, mergeTimer)
	}

	if mergeSuccess {
		if !mergeConflictOccurred {
			mergeSpinner.Stop(true, "")
		}
		mergeTimer.StopWithResult(true, fmt.Sprintf("squash merged %s into %s (local only)", targetTask.Name, mainBranch))
	}

	if mergeSuccess && appCtx.Config != nil && appCtx.Config.PostMergeHook != "" {
		hookEnv := appCtx.GetEnvVars(targetTask.Name, workDir, windowID)
		if _, err := service.RunHook(
			"post-merge",
			appCtx.Config.PostMergeHook,
			appCtx.ProjectDir,
			hookEnv,
			targetTask.GetHookOutputPath("post-merge"),
			targetTask.GetHookMetaPath("post-merge"),
			constants.DefaultHookTimeout,
		); err != nil {
			logging.Warn("Post-merge hook failed: %v", err)
		}
	}

	// Restore original branch if different from main
	if currentBranch != "" && currentBranch != mainBranch {
		logging.Debug("Restoring branch %s...", currentBranch)
		if err := gitClient.Checkout(appCtx.ProjectDir, currentBranch); err != nil {
			logging.Warn("Failed to restore branch: %v", err)
		}
	}

	return mergeSuccess
}

// handleMergeConflicts attempts to resolve merge conflicts.
func handleMergeConflicts(appCtx *app.App, targetTask *task.Task, mainBranch, mergeMsg string, gitClient git.Client, mergeTimer *logging.Timer) bool {
	hasConflicts, conflictFiles, _ := gitClient.HasConflicts(appCtx.ProjectDir)
	if hasConflicts && len(conflictFiles) > 0 {
		fmt.Println()
		fmt.Printf("  ⚠️  Merge conflicts detected in %d file(s):\n", len(conflictFiles))
		for _, f := range conflictFiles {
			fmt.Printf("      - %s\n", f)
		}
		fmt.Println()

		resolveSpinner := tui.NewSimpleSpinner("Resolving conflicts with Claude")
		resolveSpinner.Start()

		taskContent, _ := targetTask.LoadContent()

		if resolveErr := resolveConflictsWithClaude(appCtx.ProjectDir, targetTask.Name, taskContent, conflictFiles); resolveErr != nil {
			logging.Warn("Claude conflict resolution failed: %v", resolveErr)
			resolveSpinner.Stop(false, "failed")
			if abortErr := gitClient.MergeAbort(appCtx.ProjectDir); abortErr != nil {
				logging.Warn("Failed to abort merge: %v", abortErr)
			}
			mergeTimer.StopWithResult(false, "conflict resolution failed")
			return false
		}

		stillHasConflicts, remainingFiles, _ := gitClient.HasConflicts(appCtx.ProjectDir)
		if stillHasConflicts && len(remainingFiles) > 0 {
			logging.Warn("Conflicts still exist after Claude resolution: %v", remainingFiles)
			resolveSpinner.Stop(false, "unresolved")
			if abortErr := gitClient.MergeAbort(appCtx.ProjectDir); abortErr != nil {
				logging.Warn("Failed to abort merge: %v", abortErr)
			}
			mergeTimer.StopWithResult(false, "conflicts remain")
			return false
		}

		resolveSpinner.Stop(true, "resolved")
		logging.Log("Conflicts resolved by Claude, completing merge")

		if addErr := gitClient.AddAll(appCtx.ProjectDir); addErr != nil {
			logging.Warn("Failed to stage resolved files: %v", addErr)
		}

		// Layer 3: Prevent .claude symlink from being committed (final safety check)
		claudeStaged, err := gitClient.IsFileStaged(appCtx.ProjectDir, constants.ClaudeLink)
		if err != nil {
			logging.Warn("Failed to check if .claude is staged: %v", err)
		} else if claudeStaged {
			logging.Warn("Detected .claude in staging area (project dir), unstaging it to prevent commit")
			if err := gitClient.ResetPath(appCtx.ProjectDir, constants.ClaudeLink); err != nil {
				logging.Warn("Failed to unstage .claude: %v", err)
			} else {
				logging.Debug("Successfully unstaged .claude from project dir")
			}
		}

		if commitErr := gitClient.Commit(appCtx.ProjectDir, mergeMsg); commitErr != nil {
			logging.Warn("Failed to commit merge: %v", commitErr)
			mergeTimer.StopWithResult(false, "commit failed")
			return false
		}
		logging.Log("Merge completed after conflict resolution")
		return true
	}

	// No conflicts detected, but merge still failed - try auto-resolution
	logging.Warn("Merge failed without conflicts - attempting auto-resolution with Claude")
	fmt.Println()
	fmt.Println("  ⚠️  Merge failed without obvious conflicts")
	fmt.Println()

	autoResolveSpinner := tui.NewSimpleSpinner("Attempting auto-resolution with Claude")
	autoResolveSpinner.Start()

	taskContent, _ := targetTask.LoadContent()
	if autoResolveErr := autoResolveMergeFailure(appCtx.ProjectDir, targetTask.Name, taskContent, targetTask.Name, mainBranch, gitClient); autoResolveErr != nil {
		logging.Warn("Auto-resolution failed: %v", autoResolveErr)
		autoResolveSpinner.Stop(false, "failed")
		if abortErr := gitClient.MergeAbort(appCtx.ProjectDir); abortErr != nil {
			logging.Warn("Failed to abort merge: %v", abortErr)
		}
		mergeTimer.StopWithResult(false, "merge failed")
		return false
	}

	autoResolveSpinner.Stop(true, "resolved")
	logging.Log("Merge issue resolved by Claude auto-resolution")
	return true
}

// handleMergeFailure handles the case when merge fails.
func handleMergeFailure(appCtx *app.App, targetTask *task.Task, windowID string, tm tmux.Client) bool {
	logging.Warn("Merge failed - keeping task for manual resolution")
	fmt.Println()
	fmt.Println("  ✗ Merge failed - manual resolution needed")
	warningWindowName := windowNameForStatus(targetTask.Name, task.StatusCorrupted)
	logging.Trace("endTaskCmd: renaming window to warning state name=%s", warningWindowName)
	if err := renameWindowWithStatus(tm, windowID, warningWindowName, appCtx.PawDir, targetTask.Name, "end-task"); err != nil {
		logging.Warn("Failed to rename window: %v", err)
	}
	logging.Trace("endTaskCmd: playing SoundError for merge failure task=%s", targetTask.Name)
	notify.PlaySound(notify.SoundError)
	_ = notify.Send("Merge failed", fmt.Sprintf("⚠️ %s - manual resolution needed", targetTask.Name))
	logging.Trace("endTaskCmd: displaying merge failure message for task=%s", targetTask.Name)
	if err := tm.DisplayMessage(fmt.Sprintf("⚠️ Merge failed: %s - manual resolution needed", targetTask.Name), 3000); err != nil {
		logging.Trace("Failed to display message: %v", err)
	}
	return false
}
