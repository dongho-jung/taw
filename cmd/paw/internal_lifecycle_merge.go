package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/notify"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var mergeTaskCmd = &cobra.Command{
	Use:   "merge-task [session] [window-id]",
	Short: "Merge task to main branch (keeps task window)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "merge-task", "")
		defer cleanup()

		logging.Debug("-> mergeTaskCmd(session=%s, windowID=%s)", sessionName, windowID)
		defer logging.Debug("<- mergeTaskCmd")

		tm := tmux.New(sessionName)

		fmt.Println()
		fmt.Println("  ╭─────────────────────────────────────╮")
		fmt.Println("  │         Merging to Main...          │")
		fmt.Println("  ╰─────────────────────────────────────╯")
		fmt.Println()

		// Find task by window ID
		findSpinner := tui.NewSimpleSpinner("Finding task")
		findSpinner.Start()

		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		targetTask, err := mgr.FindTaskByWindowID(windowID)
		if err != nil {
			if errors.Is(err, task.ErrTaskNotFound) {
				findSpinner.Stop(false, "Not found")
				fmt.Println("  ✗ Task not found")
				return nil
			}
			findSpinner.Stop(false, "Failed")
			return fmt.Errorf("failed to find task: %w", err)
		}
		findSpinner.Stop(true, "Found")
		fmt.Printf("  Task: %s\n\n", targetTask.Name)

		if !appCtx.IsGitRepo {
			fmt.Println("  ✗ Not a git repository")
			return nil
		}
		if !appCtx.IsWorktreeMode() {
			fmt.Println("  ✗ Merge is only available in worktree mode")
			return nil
		}

		gitClient := git.New()
		workDir := mgr.GetWorkingDirectory(targetTask)
		mainBranch := gitClient.GetMainBranch(appCtx.ProjectDir)

		// Check if already merged
		if gitClient.BranchMerged(appCtx.ProjectDir, targetTask.Name, mainBranch) {
			fmt.Printf("  ○ Already merged to %s\n", mainBranch)
			return nil
		}

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

		// Commit any uncommitted changes first
		hasChanges := gitClient.HasChanges(workDir)
		if hasChanges {
			commitSpinner := tui.NewSimpleSpinner("Committing changes")
			commitSpinner.Start()

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
			message := fmt.Sprintf(constants.CommitMessageAutoCommitMerge, diffStat)
			if err := gitClient.Commit(workDir, message); err != nil {
				commitSpinner.Stop(false, err.Error())
			} else {
				commitSpinner.Stop(true, "")
			}
		}

		// Push task branch
		branchName, ok := resolvePushBranch(gitClient, workDir, targetTask.Name)
		if !ok {
			fmt.Println("  ⚠️  Skipping push: unable to determine branch")
		} else {
			pushSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Pushing %s to remote", branchName))
			pushSpinner.Start()

			if err := gitClient.Push(workDir, "origin", branchName, true); err != nil {
				pushSpinner.Stop(false, err.Error())
				logging.Warn("Failed to push task branch: %v", err)
			} else {
				pushSpinner.Stop(true, branchName)
			}
		}

		// Acquire merge lock
		lockSpinner := tui.NewSimpleSpinner("Acquiring merge lock")
		lockSpinner.Start()

		lockFile := filepath.Join(appCtx.PawDir, "merge.lock")
		lockAcquired := false
		for retries := 0; retries < constants.MergeLockMaxRetries; retries++ {
			f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
			if err == nil {
				_, writeErr := fmt.Fprintf(f, "%s\n%d", targetTask.Name, os.Getpid())
				closeErr := f.Close()
				if writeErr != nil || closeErr != nil {
					_ = os.Remove(lockFile)
					time.Sleep(constants.MergeLockRetryInterval)
					continue
				}
				lockAcquired = true
				break
			}

			if isStaleLock(lockFile) {
				_ = os.Remove(lockFile)
				continue
			}

			time.Sleep(constants.MergeLockRetryInterval)
		}

		if !lockAcquired {
			lockSpinner.Stop(false, "timeout")
			fmt.Println("  ✗ Failed to acquire merge lock")
			return nil
		}
		lockSpinner.Stop(true, "")
		defer func() { _ = os.Remove(lockFile) }()

		// Check for ongoing merge or conflicts
		hasConflicts, conflictFiles, _ := gitClient.HasConflicts(appCtx.ProjectDir)
		hasOngoingMerge := gitClient.HasOngoingMerge(appCtx.ProjectDir)

		if hasConflicts || hasOngoingMerge {
			fmt.Println("  ⚠️  Project has unresolved conflicts or ongoing merge")
			if hasConflicts && len(conflictFiles) > 0 {
				for _, f := range conflictFiles {
					fmt.Printf("    - %s\n", f)
				}
			}
			fmt.Printf("\n  Please resolve in: %s\n", appCtx.ProjectDir)
			return nil
		}

		// Stash local changes in project dir
		hasLocalChanges := gitClient.HasChanges(appCtx.ProjectDir)
		if hasLocalChanges {
			if err := gitClient.StashPush(appCtx.ProjectDir, constants.MergeStashMessage); err != nil {
				logging.Warn("Failed to stash changes: %v", err)
			}
		}

		// Remember current branch
		currentBranch, _ := gitClient.GetCurrentBranch(appCtx.ProjectDir)

		mergeSuccess := true

		// Fetch from origin
		fetchSpinner := tui.NewSimpleSpinner("Fetching from origin")
		fetchSpinner.Start()
		if err := gitClient.Fetch(appCtx.ProjectDir, "origin"); err != nil {
			fetchSpinner.Stop(false, err.Error())
		} else {
			fetchSpinner.Stop(true, "")
		}

		// Checkout main
		checkoutSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Checking out %s", mainBranch))
		checkoutSpinner.Start()
		if err := gitClient.Checkout(appCtx.ProjectDir, mainBranch); err != nil {
			checkoutSpinner.Stop(false, err.Error())
			mergeSuccess = false
		} else {
			checkoutSpinner.Stop(true, "")

			// Pull latest
			pullSpinner := tui.NewSimpleSpinner("Pulling latest")
			pullSpinner.Start()
			if err := gitClient.Pull(appCtx.ProjectDir); err != nil {
				pullSpinner.Stop(false, err.Error())
			} else {
				pullSpinner.Stop(true, "")
			}

			// Squash merge
			mergeSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Merging %s", targetTask.Name))
			mergeSpinner.Start()
			branchCommits, _ := gitClient.GetBranchCommits(appCtx.ProjectDir, targetTask.Name, mainBranch, 20)
			mergeMsg := git.GenerateMergeCommitMessage(targetTask.Name, branchCommits)
			mergeConflictOccurred := false
			if err := gitClient.MergeSquash(appCtx.ProjectDir, targetTask.Name, mergeMsg); err != nil {
				mergeSpinner.Stop(false, "conflict")
				mergeConflictOccurred = true

				// Check if this is a conflict situation
				hasConflicts, conflictFiles, _ := gitClient.HasConflicts(appCtx.ProjectDir)
				if hasConflicts && len(conflictFiles) > 0 {
					// Try to resolve conflicts with Claude
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
						_ = gitClient.MergeAbort(appCtx.ProjectDir)
						mergeSuccess = false
					} else {
						stillHasConflicts, remainingFiles, _ := gitClient.HasConflicts(appCtx.ProjectDir)
						if stillHasConflicts && len(remainingFiles) > 0 {
							logging.Warn("Conflicts still exist after Claude resolution: %v", remainingFiles)
							resolveSpinner.Stop(false, "unresolved")
							_ = gitClient.MergeAbort(appCtx.ProjectDir)
							mergeSuccess = false
						} else {
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
								mergeSuccess = false
							} else {
								logging.Log("Merge completed after conflict resolution")
							}
						}
					}
				} else {
					// No conflicts detected, but merge still failed
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
						_ = gitClient.MergeAbort(appCtx.ProjectDir)
						mergeSuccess = false
					} else {
						autoResolveSpinner.Stop(true, "resolved")
						logging.Log("Merge issue resolved by Claude auto-resolution")
					}
				}
			}

			// If merge succeeded, push
			if mergeSuccess {
				if !mergeConflictOccurred {
					mergeSpinner.Stop(true, "")
				}

				pushMainSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Pushing %s", mainBranch))
				pushMainSpinner.Start()
				if err := gitClient.Push(appCtx.ProjectDir, "origin", mainBranch, false); err != nil {
					pushMainSpinner.Stop(false, err.Error())
					mergeSuccess = false
				} else {
					pushMainSpinner.Stop(true, "")
				}
			}

			// Restore original branch
			if currentBranch != "" && currentBranch != mainBranch {
				_ = gitClient.Checkout(appCtx.ProjectDir, currentBranch)
			}
		}

		// Restore stashed changes by message (not blind pop)
		if hasLocalChanges {
			if err := gitClient.StashPopByMessage(appCtx.ProjectDir, constants.MergeStashMessage); err != nil {
				logging.Warn("Failed to restore stashed changes: %v", err)
			}
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

		fmt.Println()
		if mergeSuccess {
			fmt.Printf("  ✅ Merged to %s (task window kept)\n", mainBranch)
			logging.Log("Merged task %s to %s", targetTask.Name, mainBranch)
			notify.PlaySound(notify.SoundTaskCompleted)
			_ = tm.DisplayMessage(fmt.Sprintf("✅ Merged: %s → %s", targetTask.Name, mainBranch), 2000)
		} else {
			fmt.Println("  ✗ Merge failed - manual resolution needed")
			corruptedName := windowNameForStatus(targetTask.Name, task.StatusCorrupted)
			_ = renameWindowWithStatus(tm, windowID, corruptedName, appCtx.PawDir, targetTask.Name, "merge-task", task.StatusCorrupted)
			notify.PlaySound(notify.SoundError)
			_ = tm.DisplayMessage(fmt.Sprintf("⚠️ Merge failed: %s", targetTask.Name), 3000)
		}

		return nil
	},
}

var mergeTaskUICmd = &cobra.Command{
	Use:   "merge-task-ui [session]",
	Short: "Merge task with UI feedback (creates visible pane)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "merge-task-ui", "")
		defer cleanup()

		logging.Debug("-> mergeTaskUICmd(session=%s)", sessionName)
		defer logging.Debug("<- mergeTaskUICmd")

		tm := tmux.New(sessionName)

		// Get current window ID
		windowID, err := tm.Display("#{window_id}")
		if err != nil {
			return fmt.Errorf("failed to get window ID: %w", err)
		}
		windowID = strings.TrimSpace(windowID)

		// Get current window name
		windowName, _ := tm.Display("#{window_name}")
		windowName = strings.TrimSpace(windowName)

		// Check if this is a task window
		if !constants.IsTaskWindow(windowName) {
			_ = tm.DisplayMessage("Not a task window", 1500)
			return nil
		}

		// Get paw binary path
		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Get working directory from pane
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			panePath = appCtx.ProjectDir
		}

		// Build merge-task command
		mergeTaskCmdStr := fmt.Sprintf("%s internal merge-task %s %s; echo; echo 'Press Enter to close...'; read",
			pawBin, sessionName, windowID)

		// Create a top pane (40% height)
		_, err = tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false,
			Size:       "40%",
			StartDir:   panePath,
			Command:    mergeTaskCmdStr,
			Before:     true,
			Full:       true,
		})
		if err != nil {
			return fmt.Errorf("failed to create merge-task pane: %w", err)
		}

		return nil
	},
}
