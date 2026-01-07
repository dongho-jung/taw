package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/config"
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

var endTaskCmd = &cobra.Command{
	Use:   "end-task [session] [window-id]",
	Short: "Finish a task (commit, merge, cleanup)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Trace("endTaskCmd: start session=%s windowID=%s", args[0], args[1])
		defer logging.Trace("endTaskCmd: end")

		sessionName := args[0]
		windowID := args[1]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Find task by window ID
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
		targetTask, err := mgr.FindTaskByWindowID(windowID)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("end-task")
			logger.SetTask(targetTask.Name)
			logging.SetGlobal(logger)
		}

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
		logging.Debug("Configuration: ON_COMPLETE=%s, WorkMode=%s", app.Config.OnComplete, app.Config.WorkMode)

		gitClient := git.New()
		workDir := mgr.GetWorkingDirectory(targetTask)
		logging.Trace("Working directory: %s", workDir)

		// Commit changes if git mode
		if app.IsGitRepo {
			hasChanges := gitClient.HasChanges(workDir)
			logging.Trace("Git status: hasChanges=%v", hasChanges)

			if hasChanges {
				spinner := tui.NewSimpleSpinner("Committing changes")
				spinner.Start()

				commitTimer := logging.StartTimer("git commit")
				if err := gitClient.AddAll(workDir); err != nil {
					logging.Warn("Failed to add changes: %v", err)
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
			} else {
				fmt.Println("  ○ No changes to commit")
			}

			// Push changes
			shouldPush := app.Config != nil && app.Config.OnComplete != config.OnCompleteConfirm
			branchName := ""
			skipReason := ""
			if shouldPush {
				fallbackBranch := ""
				if app.IsWorktreeMode() {
					fallbackBranch = targetTask.Name
				}
				var ok bool
				branchName, ok = resolvePushBranch(gitClient, workDir, fallbackBranch)
				if !ok {
					shouldPush = false
					skipReason = "unable to determine branch"
				}
			} else {
				skipReason = "confirm mode"
			}

			if shouldPush {
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
			} else if skipReason != "" {
				fmt.Printf("  ○ Skipping push (%s)\n", skipReason)
			}

			// Handle auto-merge mode
			if app.Config != nil && app.Config.OnComplete == config.OnCompleteAutoMerge {
				if !app.IsWorktreeMode() {
					logging.Warn("auto-merge requested in non-worktree mode; skipping merge")
					fmt.Println()
					fmt.Println("  ⚠️  Auto-merge is only available in worktree mode")
				} else {
					mergeSuccess := true // Track merge result to decide cleanup
					logging.Log("auto-merge: starting merge process")
					fmt.Println()
					fmt.Println("  Auto-merge mode:")

					// Get main branch name
					mainBranch := gitClient.GetMainBranch(app.ProjectDir)
					logging.Debug("Main branch: %s", mainBranch)

					mergeTimer := logging.StartTimer("auto-merge")

					// Acquire merge lock to prevent concurrent merges
					// This is necessary because we need to checkout main in project dir
					lockSpinner := tui.NewSimpleSpinner("Acquiring merge lock")
					lockSpinner.Start()

					lockFile := filepath.Join(app.PawDir, "merge.lock")
					lockAcquired := false
					for retries := 0; retries < constants.MergeLockMaxRetries; retries++ {
						f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
						if err == nil {
							_, writeErr := fmt.Fprintf(f, "%s\n%d", targetTask.Name, os.Getpid())
							closeErr := f.Close()
							if writeErr != nil || closeErr != nil {
								// Failed to write lock info, remove and retry
								_ = os.Remove(lockFile)
								logging.Warn("Failed to write lock file: write=%v, close=%v", writeErr, closeErr)
								time.Sleep(constants.MergeLockRetryInterval)
								continue
							}
							lockAcquired = true
							break
						}

						// Lock file exists - check if the process holding it is still alive
						if isStaleLock(lockFile) {
							logging.Debug("Detected stale merge lock, removing...")
							if rmErr := os.Remove(lockFile); rmErr != nil {
								logging.Warn("Failed to remove stale lock: %v", rmErr)
							}
							// Try again immediately without sleeping
							continue
						}

						logging.Trace("Waiting for merge lock (attempt %d/%d)...", retries+1, constants.MergeLockMaxRetries)
						time.Sleep(constants.MergeLockRetryInterval)
					}

					if !lockAcquired {
						logging.Warn("Failed to acquire merge lock after %d seconds", constants.MergeLockMaxRetries)
						mergeTimer.StopWithResult(false, "lock timeout")
						lockSpinner.Stop(false, fmt.Sprintf("timeout after %ds", constants.MergeLockMaxRetries))
						mergeSuccess = false
					} else {
						lockSpinner.Stop(true, "")
						// Ensure lock is released on exit
						defer func() { _ = os.Remove(lockFile) }()

						// Check for ongoing merge or conflicts in project dir
						hasConflicts, conflictFiles, _ := gitClient.HasConflicts(app.ProjectDir)
						hasOngoingMerge := gitClient.HasOngoingMerge(app.ProjectDir)

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
							fmt.Printf("    cd %s\n", app.ProjectDir)
							fmt.Println("    git status  # View current state")
							fmt.Println("    # Resolve conflicts, then: git add . && git commit")
							fmt.Println("    # Or abort merge: git merge --abort")
							fmt.Println()
							mergeTimer.StopWithResult(false, "project has conflicts")
							mergeSuccess = false
						}

						// Stash any uncommitted changes in project dir (skip if conflicts detected)
						hasLocalChanges := false
						if mergeSuccess {
							hasLocalChanges = gitClient.HasChanges(app.ProjectDir)
							if hasLocalChanges {
								logging.Debug("Stashing local changes...")
								if err := gitClient.StashPush(app.ProjectDir, "paw-merge-temp"); err != nil {
									logging.Warn("Failed to stash changes: %v", err)
								}
							}
						}

						// Remember current branch to restore later
						currentBranch, _ := gitClient.GetCurrentBranch(app.ProjectDir)

						// Only proceed with merge if no conflicts were detected
						if mergeSuccess {
							// Fetch latest from origin
							fetchSpinner := tui.NewSimpleSpinner("Fetching from origin")
							fetchSpinner.Start()
							logging.Debug("Fetching from origin...")
							if err := gitClient.Fetch(app.ProjectDir, "origin"); err != nil {
								logging.Warn("Failed to fetch: %v", err)
								fetchSpinner.Stop(false, err.Error())
							} else {
								fetchSpinner.Stop(true, "")
							}

							// Checkout main
							checkoutSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Checking out %s", mainBranch))
							checkoutSpinner.Start()
							logging.Debug("Checking out %s...", mainBranch)
							if err := gitClient.Checkout(app.ProjectDir, mainBranch); err != nil {
								logging.Warn("Failed to checkout %s: %v", mainBranch, err)
								mergeTimer.StopWithResult(false, "checkout failed")
								checkoutSpinner.Stop(false, err.Error())
								mergeSuccess = false
							} else {
								checkoutSpinner.Stop(true, "")

								// Pull latest
								pullSpinner := tui.NewSimpleSpinner("Pulling latest changes")
								pullSpinner.Start()
								logging.Debug("Pulling latest changes...")
								if err := gitClient.Pull(app.ProjectDir); err != nil {
									logging.Warn("Failed to pull: %v", err)
									pullSpinner.Stop(false, err.Error())
								} else {
									pullSpinner.Stop(true, "")
								}

								// Merge task branch (squash)
								mergeSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Merging %s into %s", targetTask.Name, mainBranch))
								mergeSpinner.Start()
								logging.Debug("Squash merging branch %s into %s...", targetTask.Name, mainBranch)
								mergeMsg := fmt.Sprintf(constants.CommitMessageMerge, targetTask.Name)
								mergeConflictOccurred := false // Track if we had to resolve conflicts
								if err := gitClient.MergeSquash(app.ProjectDir, targetTask.Name, mergeMsg); err != nil {
									logging.Warn("Merge failed: %v - checking for conflicts", err)
									mergeSpinner.Stop(false, "conflict")
									mergeConflictOccurred = true

									// Check if this is a conflict situation
									hasConflicts, conflictFiles, _ := gitClient.HasConflicts(app.ProjectDir)
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

										// Get task content for context
										taskContent, _ := targetTask.LoadContent()

										// Run Claude to resolve conflicts
										if resolveErr := resolveConflictsWithClaude(app.ProjectDir, targetTask.Name, taskContent, conflictFiles); resolveErr != nil {
											logging.Warn("Claude conflict resolution failed: %v", resolveErr)
											resolveSpinner.Stop(false, "failed")
											// Abort merge
											if abortErr := gitClient.MergeAbort(app.ProjectDir); abortErr != nil {
												logging.Warn("Failed to abort merge: %v", abortErr)
											}
											mergeTimer.StopWithResult(false, "conflict resolution failed")
											mergeSuccess = false
										} else {
											// Check if conflicts are actually resolved
											stillHasConflicts, remainingFiles, _ := gitClient.HasConflicts(app.ProjectDir)
											if stillHasConflicts && len(remainingFiles) > 0 {
												logging.Warn("Conflicts still exist after Claude resolution: %v", remainingFiles)
												resolveSpinner.Stop(false, "unresolved")
												// Abort merge
												if abortErr := gitClient.MergeAbort(app.ProjectDir); abortErr != nil {
													logging.Warn("Failed to abort merge: %v", abortErr)
												}
												mergeTimer.StopWithResult(false, "conflicts remain")
												mergeSuccess = false
											} else {
												resolveSpinner.Stop(true, "resolved")
												logging.Log("Conflicts resolved by Claude, completing merge")

												// Stage all changes and commit
												if addErr := gitClient.AddAll(app.ProjectDir); addErr != nil {
													logging.Warn("Failed to stage resolved files: %v", addErr)
												}
												if commitErr := gitClient.Commit(app.ProjectDir, mergeMsg); commitErr != nil {
													logging.Warn("Failed to commit merge: %v", commitErr)
													mergeTimer.StopWithResult(false, "commit failed")
													mergeSuccess = false
												} else {
													// Merge completed successfully after conflict resolution
													logging.Log("Merge completed after conflict resolution")
												}
											}
										}
									} else {
										// No conflicts detected, but merge still failed - abort
										logging.Warn("Merge failed without conflicts - may need manual resolution")
										if abortErr := gitClient.MergeAbort(app.ProjectDir); abortErr != nil {
											logging.Warn("Failed to abort merge: %v", abortErr)
										}
										mergeTimer.StopWithResult(false, "merge failed")
										mergeSuccess = false
									}
								}
								// If merge succeeded (either directly or after conflict resolution), push
								if mergeSuccess {
									// Only stop merge spinner if we didn't have conflicts (already stopped above)
									if !mergeConflictOccurred {
										mergeSpinner.Stop(true, "")
									}

									// Push merged main
									pushMainSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Pushing %s to origin", mainBranch))
									pushMainSpinner.Start()
									logging.Debug("Pushing merged main to origin...")
									if err := gitClient.Push(app.ProjectDir, "origin", mainBranch, false); err != nil {
										logging.Warn("Failed to push merged main: %v", err)
										mergeTimer.StopWithResult(false, "push failed")
										pushMainSpinner.Stop(false, err.Error())
										mergeSuccess = false
									} else {
										mergeTimer.StopWithResult(true, fmt.Sprintf("squash merged %s into %s", targetTask.Name, mainBranch))
										pushMainSpinner.Stop(true, "")
									}
								}

								// Restore original branch if different from main
								if currentBranch != "" && currentBranch != mainBranch {
									logging.Debug("Restoring branch %s...", currentBranch)
									if err := gitClient.Checkout(app.ProjectDir, currentBranch); err != nil {
										logging.Warn("Failed to restore branch: %v", err)
									}
								}
							}

							// Restore stashed changes
							if hasLocalChanges {
								logging.Debug("Restoring stashed changes...")
								if err := gitClient.StashPop(app.ProjectDir); err != nil {
									logging.Warn("Failed to restore stashed changes: %v", err)
								}
							}
						}
					}

					// If merge failed, rename window to warning and skip cleanup
					if !mergeSuccess {
						logging.Warn("Merge failed - keeping task for manual resolution")
						fmt.Println()
						fmt.Println("  ✗ Merge failed - manual resolution needed")
						warningWindowName := constants.EmojiWarning + targetTask.Name
						logging.Trace("endTaskCmd: renaming window to warning state name=%s", warningWindowName)
						if err := tm.RenameWindow(windowID, warningWindowName); err != nil {
							logging.Warn("Failed to rename window: %v", err)
						}
						// Notify user of merge failure
						logging.Trace("endTaskCmd: playing SoundError for merge failure task=%s", targetTask.Name)
						notify.PlaySound(notify.SoundError)
						// Send to all configured notification channels (macOS, Slack, ntfy)
						notify.SendAll(app.Config.Notifications, "Merge failed", fmt.Sprintf("⚠️ %s - manual resolution needed", targetTask.Name))
						logging.Trace("endTaskCmd: displaying merge failure message for task=%s", targetTask.Name)
						if err := tm.DisplayMessage(fmt.Sprintf("⚠️ Merge failed: %s - manual resolution needed", targetTask.Name), 3000); err != nil {
							logging.Trace("Failed to display message: %v", err)
						}
						return nil // Exit without cleanup - keep worktree and branch
					}
				}
			}
		}
		fmt.Println()

		// Save to history using service
		historyService := service.NewHistoryService(app.GetHistoryDir())

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
		if err := historyService.SaveCompleted(targetTask.Name, taskContent, paneContent); err != nil {
			logging.Warn("Failed to save history: %v", err)
		}

		// Notify user that task completed successfully
		logging.Trace("endTaskCmd: playing SoundTaskCompleted for task=%s", targetTask.Name)
		notify.PlaySound(notify.SoundTaskCompleted)
		// Send to all configured notification channels (macOS, Slack, ntfy)
		notify.SendAll(app.Config.Notifications, "Task completed", fmt.Sprintf("✅ %s completed successfully", targetTask.Name))
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

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("end-task-ui")
			logging.SetGlobal(logger)
		}

		logging.Trace("endTaskUICmd: start session=%s windowID=%s", sessionName, windowID)
		defer logging.Trace("endTaskUICmd: end")

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
			app, err := getAppFromSession(sessionName)
			if err != nil {
				return err
			}
			panePath = app.ProjectDir
		}

		// Build end-task command that runs in the pane
		// Include pane-capture-file flag if we have pre-captured content
		var endTaskCmdStr string
		if capturePath != "" {
			endTaskCmdStr = fmt.Sprintf("%s internal end-task --user-initiated --pane-capture-file=%q %s %s; echo; echo 'Press Enter to close...'; read",
				pawBin, capturePath, sessionName, windowID)
		} else {
			endTaskCmdStr = fmt.Sprintf("%s internal end-task --user-initiated %s %s; echo; echo 'Press Enter to close...'; read",
				pawBin, sessionName, windowID)
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

var cancelTaskCmd = &cobra.Command{
	Use:   "cancel-task [session] [window-id]",
	Short: "Cancel a task (with revert if merged)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("cancel-task")
			logging.SetGlobal(logger)
		}

		logging.Trace("cancelTaskCmd: start session=%s windowID=%s", sessionName, windowID)
		defer logging.Trace("cancelTaskCmd: end")

		tm := tmux.New(sessionName)

		fmt.Println()
		fmt.Println("  ╭─────────────────────────────────────╮")
		fmt.Println("  │         Cancelling Task...          │")
		fmt.Println("  ╰─────────────────────────────────────╯")
		fmt.Println()

		// Find task by window ID
		findSpinner := tui.NewSimpleSpinner("Finding task")
		findSpinner.Start()

		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
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

		revertNeeded := false
		revertSuccess := false

		// Check if task was merged and needs to be reverted
		if app.IsGitRepo {
			gitClient := git.New()
			mainBranch := gitClient.GetMainBranch(app.ProjectDir)

			// Check if branch was merged into main
			if gitClient.BranchMerged(app.ProjectDir, targetTask.Name, mainBranch) {
				revertNeeded = true
				logging.Trace("cancelTaskCmd: task %s was merged, attempting revert", targetTask.Name)

				revertSpinner := tui.NewSimpleSpinner("Reverting merge")
				revertSpinner.Start()

				// Find the merge commit
				mergeCommit, err := gitClient.FindMergeCommit(app.ProjectDir, targetTask.Name, mainBranch)
				if err != nil {
					revertSpinner.Stop(false, "Failed")
					logging.Warn("Failed to find merge commit: %v", err)
					fmt.Println("  ✗ Failed to find merge commit")
				} else if mergeCommit != "" {
					logging.Trace("cancelTaskCmd: found merge commit %s, reverting", mergeCommit)

					// Checkout main branch first
					if err := gitClient.Checkout(app.ProjectDir, mainBranch); err != nil {
						revertSpinner.Stop(false, "Checkout failed")
						logging.Warn("Failed to checkout main branch: %v", err)
						fmt.Printf("  ✗ Failed to checkout %s\n", mainBranch)
					} else {
						// Revert the merge commit
						if err := gitClient.RevertCommit(app.ProjectDir, mergeCommit, ""); err != nil {
							revertSpinner.Stop(false, "Conflict")
							logging.Warn("Failed to revert merge commit: %v", err)
							fmt.Println("  ✗ Revert failed (conflict?)")
							fmt.Println()
							fmt.Println("  ⚠️  Manual resolution required:")
							fmt.Printf("     cd %s\n", app.ProjectDir)
							fmt.Printf("     git revert -m 1 %s\n", mergeCommit)
							fmt.Println("     # Resolve conflicts, then commit and push")
							fmt.Println()

							// Abort revert if in progress
							abortCmd := exec.Command("git", "revert", "--abort")
							abortCmd.Dir = app.ProjectDir
							_ = abortCmd.Run()

							// Rename window to warning state
							warningName := constants.EmojiWarning + targetTask.Name
							if len(warningName) > 14 {
								warningName = warningName[:14]
							}
							_ = tm.RenameWindow(windowID, warningName)
							notify.PlaySound(notify.SoundError)
							return nil // Don't cleanup - keep task for manual resolution
						} else {
							revertSpinner.Stop(true, "Reverted")
							logging.Log("Reverted merge commit %s for task %s", mergeCommit, targetTask.Name)

							// Push the revert
							pushSpinner := tui.NewSimpleSpinner("Pushing revert")
							pushSpinner.Start()

							if err := gitClient.Push(app.ProjectDir, "origin", mainBranch, false); err != nil {
								pushSpinner.Stop(false, "Push failed")
								logging.Warn("Failed to push revert: %v", err)
								fmt.Println("  ⚠️  Reverted locally but push failed")
								fmt.Printf("     Run: git push origin %s\n", mainBranch)
							} else {
								pushSpinner.Stop(true, "Pushed")
								logging.Log("Pushed revert for task %s", targetTask.Name)
								revertSuccess = true
							}
						}
					}
				} else {
					revertSpinner.Stop(false, "Not found")
					fmt.Println("  ✗ Merge commit not found")
				}
			}
		}

		fmt.Println()

		// Save to history using service
		historyService := service.NewHistoryService(app.GetHistoryDir())

		historySpinner := tui.NewSimpleSpinner("Saving history")
		historySpinner.Start()

		// Capture pane content
		paneContent, captureErr := tm.CapturePane(windowID+".0", constants.PaneCaptureLines)
		if captureErr != nil {
			logging.Warn("Failed to capture pane content: %v", captureErr)
			historySpinner.Stop(false, "capture failed")
		} else if paneContent != "" {
			taskContent, _ := targetTask.LoadContent()
			if err := historyService.SaveCancelled(targetTask.Name, taskContent, paneContent); err != nil {
				logging.Warn("Failed to save history: %v", err)
				historySpinner.Stop(false, "save failed")
			} else {
				historySpinner.Stop(true, "saved")
			}
		} else {
			historySpinner.Stop(false, "empty capture")
		}

		// Cleanup task
		cleanupSpinner := tui.NewSimpleSpinner("Cleaning up")
		cleanupSpinner.Start()

		if err := mgr.CleanupTask(targetTask); err != nil {
			cleanupSpinner.Stop(false, "Failed")
			logging.Warn("Failed to cleanup task: %v", err)
		} else {
			cleanupSpinner.Stop(true, "Done")
		}

		// Kill window (after cleanup to ensure we're done with task data)
		if err := tm.KillWindow(windowID); err != nil {
			logging.Warn("Failed to kill window: %v", err)
		}

		fmt.Println()
		if revertNeeded && revertSuccess {
			fmt.Println("  ✅ Task cancelled and merge reverted")
		} else if revertNeeded {
			fmt.Println("  ⚠️  Task cancelled (revert may need attention)")
		} else {
			fmt.Println("  ✅ Task cancelled")
		}

		notify.PlaySound(notify.SoundTaskCompleted)
		return nil
	},
}

var cancelTaskUICmd = &cobra.Command{
	Use:   "cancel-task-ui [session] [window-id]",
	Short: "Cancel task with UI feedback (creates visible pane)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("cancel-task-ui")
			logging.SetGlobal(logger)
		}

		logging.Trace("cancelTaskUICmd: start session=%s windowID=%s", sessionName, windowID)
		defer logging.Trace("cancelTaskUICmd: end")

		tm := tmux.New(sessionName)

		// Get the paw binary path
		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Get working directory from pane
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			panePath = app.ProjectDir
		}

		// Build cancel-task command
		cancelTaskCmdStr := fmt.Sprintf("%s internal cancel-task %s %s; echo; echo 'Press Enter to close...'; read",
			pawBin, sessionName, windowID)

		// Create a top pane (40% height) spanning full window width
		_, err = tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false,
			Size:       "40%",
			StartDir:   panePath,
			Command:    cancelTaskCmdStr,
			Before:     true,
			Full:       true,
		})
		if err != nil {
			return fmt.Errorf("failed to create cancel-task pane: %w", err)
		}

		return nil
	},
}

var mergeTaskCmd = &cobra.Command{
	Use:   "merge-task [session] [window-id]",
	Short: "Merge task to main branch (keeps task window)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("merge-task")
			logging.SetGlobal(logger)
		}

		logging.Trace("mergeTaskCmd: start session=%s windowID=%s", sessionName, windowID)
		defer logging.Trace("mergeTaskCmd: end")

		tm := tmux.New(sessionName)

		fmt.Println()
		fmt.Println("  ╭─────────────────────────────────────╮")
		fmt.Println("  │         Merging to Main...          │")
		fmt.Println("  ╰─────────────────────────────────────╯")
		fmt.Println()

		// Find task by window ID
		findSpinner := tui.NewSimpleSpinner("Finding task")
		findSpinner.Start()

		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
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

		if !app.IsGitRepo {
			fmt.Println("  ✗ Not a git repository")
			return nil
		}
		if !app.IsWorktreeMode() {
			fmt.Println("  ✗ Merge is only available in worktree mode")
			return nil
		}

		gitClient := git.New()
		workDir := mgr.GetWorkingDirectory(targetTask)
		mainBranch := gitClient.GetMainBranch(app.ProjectDir)

		// Check if already merged
		if gitClient.BranchMerged(app.ProjectDir, targetTask.Name, mainBranch) {
			fmt.Printf("  ○ Already merged to %s\n", mainBranch)
			return nil
		}

		// Commit any uncommitted changes first
		hasChanges := gitClient.HasChanges(workDir)
		if hasChanges {
			commitSpinner := tui.NewSimpleSpinner("Committing changes")
			commitSpinner.Start()

			if err := gitClient.AddAll(workDir); err != nil {
				logging.Warn("Failed to add changes: %v", err)
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

		lockFile := filepath.Join(app.PawDir, "merge.lock")
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
		hasConflicts, conflictFiles, _ := gitClient.HasConflicts(app.ProjectDir)
		hasOngoingMerge := gitClient.HasOngoingMerge(app.ProjectDir)

		if hasConflicts || hasOngoingMerge {
			fmt.Println("  ⚠️  Project has unresolved conflicts or ongoing merge")
			if hasConflicts && len(conflictFiles) > 0 {
				for _, f := range conflictFiles {
					fmt.Printf("    - %s\n", f)
				}
			}
			fmt.Printf("\n  Please resolve in: %s\n", app.ProjectDir)
			return nil
		}

		// Stash local changes in project dir
		hasLocalChanges := gitClient.HasChanges(app.ProjectDir)
		if hasLocalChanges {
			if err := gitClient.StashPush(app.ProjectDir, "paw-merge-temp"); err != nil {
				logging.Warn("Failed to stash changes: %v", err)
			}
		}

		// Remember current branch
		currentBranch, _ := gitClient.GetCurrentBranch(app.ProjectDir)

		mergeSuccess := true

		// Fetch from origin
		fetchSpinner := tui.NewSimpleSpinner("Fetching from origin")
		fetchSpinner.Start()
		if err := gitClient.Fetch(app.ProjectDir, "origin"); err != nil {
			fetchSpinner.Stop(false, err.Error())
		} else {
			fetchSpinner.Stop(true, "")
		}

		// Checkout main
		checkoutSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Checking out %s", mainBranch))
		checkoutSpinner.Start()
		if err := gitClient.Checkout(app.ProjectDir, mainBranch); err != nil {
			checkoutSpinner.Stop(false, err.Error())
			mergeSuccess = false
		} else {
			checkoutSpinner.Stop(true, "")

			// Pull latest
			pullSpinner := tui.NewSimpleSpinner("Pulling latest")
			pullSpinner.Start()
			if err := gitClient.Pull(app.ProjectDir); err != nil {
				pullSpinner.Stop(false, err.Error())
			} else {
				pullSpinner.Stop(true, "")
			}

			// Squash merge
			mergeSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Merging %s", targetTask.Name))
			mergeSpinner.Start()
			mergeMsg := fmt.Sprintf(constants.CommitMessageMerge, targetTask.Name)
			mergeConflictOccurred := false
			if err := gitClient.MergeSquash(app.ProjectDir, targetTask.Name, mergeMsg); err != nil {
				mergeSpinner.Stop(false, "conflict")
				mergeConflictOccurred = true

				// Check if this is a conflict situation
				hasConflicts, conflictFiles, _ := gitClient.HasConflicts(app.ProjectDir)
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

					// Get task content for context
					taskContent, _ := targetTask.LoadContent()

					// Run Claude to resolve conflicts
					if resolveErr := resolveConflictsWithClaude(app.ProjectDir, targetTask.Name, taskContent, conflictFiles); resolveErr != nil {
						logging.Warn("Claude conflict resolution failed: %v", resolveErr)
						resolveSpinner.Stop(false, "failed")
						_ = gitClient.MergeAbort(app.ProjectDir)
						mergeSuccess = false
					} else {
						// Check if conflicts are actually resolved
						stillHasConflicts, remainingFiles, _ := gitClient.HasConflicts(app.ProjectDir)
						if stillHasConflicts && len(remainingFiles) > 0 {
							logging.Warn("Conflicts still exist after Claude resolution: %v", remainingFiles)
							resolveSpinner.Stop(false, "unresolved")
							_ = gitClient.MergeAbort(app.ProjectDir)
							mergeSuccess = false
						} else {
							resolveSpinner.Stop(true, "resolved")
							logging.Log("Conflicts resolved by Claude, completing merge")

							// Stage all changes and commit
							if addErr := gitClient.AddAll(app.ProjectDir); addErr != nil {
								logging.Warn("Failed to stage resolved files: %v", addErr)
							}
							if commitErr := gitClient.Commit(app.ProjectDir, mergeMsg); commitErr != nil {
								logging.Warn("Failed to commit merge: %v", commitErr)
								mergeSuccess = false
							} else {
								logging.Log("Merge completed after conflict resolution")
							}
						}
					}
				} else {
					// No conflicts detected, but merge still failed - abort
					_ = gitClient.MergeAbort(app.ProjectDir)
					mergeSuccess = false
				}
			}

			// If merge succeeded (either directly or after conflict resolution), push
			if mergeSuccess {
				if !mergeConflictOccurred {
					mergeSpinner.Stop(true, "")
				}

				// Push main
				pushMainSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Pushing %s", mainBranch))
				pushMainSpinner.Start()
				if err := gitClient.Push(app.ProjectDir, "origin", mainBranch, false); err != nil {
					pushMainSpinner.Stop(false, err.Error())
					mergeSuccess = false
				} else {
					pushMainSpinner.Stop(true, "")
				}
			}

			// Restore original branch
			if currentBranch != "" && currentBranch != mainBranch {
				_ = gitClient.Checkout(app.ProjectDir, currentBranch)
			}
		}

		// Restore stashed changes
		if hasLocalChanges {
			_ = gitClient.StashPop(app.ProjectDir)
		}

		fmt.Println()
		if mergeSuccess {
			fmt.Printf("  ✅ Merged to %s (task window kept)\n", mainBranch)
			logging.Log("Merged task %s to %s", targetTask.Name, mainBranch)
			notify.PlaySound(notify.SoundTaskCompleted)
			_ = tm.DisplayMessage(fmt.Sprintf("✅ Merged: %s → %s", targetTask.Name, mainBranch), 2000)
		} else {
			fmt.Println("  ✗ Merge failed - manual resolution needed")
			warningName := constants.EmojiWarning + targetTask.Name
			_ = tm.RenameWindow(windowID, warningName)
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

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("merge-task-ui")
			logging.SetGlobal(logger)
		}

		logging.Trace("mergeTaskUICmd: start session=%s", sessionName)
		defer logging.Trace("mergeTaskUICmd: end")

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
			panePath = app.ProjectDir
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

var doneTaskCmd = &cobra.Command{
	Use:   "done-task [session]",
	Short: "Complete the current task (double-press to confirm)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("done-task")
			logging.SetGlobal(logger)
		}

		logging.Trace("doneTaskCmd: start session=%s", sessionName)
		defer logging.Trace("doneTaskCmd: end")

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

		// Check if this is a task window (has emoji prefix)
		if !constants.IsTaskWindow(windowName) {
			_ = tm.DisplayMessage("Not a task window", 1500)
			return nil
		}

		// Get pending done timestamp from tmux option
		pendingTimeStr, _ := tm.GetOption("@paw_done_pending")
		now := time.Now().Unix()

		// Check if there's a pending done within 2 seconds
		if pendingTimeStr != "" {
			pendingTime, err := parseUnixTime(pendingTimeStr)
			if err == nil {
				if now-pendingTime <= constants.DoublePressIntervalSec {
					// Double-press detected within 2 seconds, finish the task
					_ = tm.SetOption("@paw_done_pending", "", true) // Clear pending state

					// Delegate to end-task-ui
					pawBin, _ := os.Executable()
					endCmd := exec.Command(pawBin, "internal", "end-task-ui", sessionName, windowID)
					return endCmd.Run()
				}
				// Time window expired - clear pending state and ignore this press
				// User must press again to start a new double-press sequence
				_ = tm.SetOption("@paw_done_pending", "", true)
				return nil
			}
		}

		// First press: show warning
		// Store current timestamp
		_ = tm.SetOption("@paw_done_pending", fmt.Sprintf("%d", now), true)

		// Play sound to indicate pending state
		logging.Trace("doneTaskCmd: playing SoundCancelPending (first press, waiting for second)")
		notify.PlaySound(notify.SoundCancelPending)

		// Show message to user
		_ = tm.DisplayMessage("⌃F again to finish task", 2000)

		return nil
	},
}

var recoverTaskCmd = &cobra.Command{
	Use:   "recover-task [session] [task-name]",
	Short: "Recover a corrupted task",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		taskName := args[1]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
		t, err := mgr.GetTask(taskName)
		if err != nil {
			return err
		}

		recoveryMgr := task.NewRecoveryManager(app.ProjectDir)
		if err := recoveryMgr.RecoverTask(t); err != nil {
			return fmt.Errorf("failed to recover task: %w", err)
		}

		fmt.Printf("Task %s recovered successfully\n", taskName)
		return nil
	},
}

// resolveConflictsWithClaude attempts to resolve merge conflicts using Claude.
// It runs Claude with a prompt asking it to resolve the conflicts in the given files.
// Returns nil if conflicts were resolved, error otherwise.
func resolveConflictsWithClaude(projectDir, taskName, taskContent string, conflictFiles []string) error {
	if len(conflictFiles) == 0 {
		return nil
	}

	// Build the prompt for Claude
	filesStr := strings.Join(conflictFiles, "\n  - ")
	prompt := fmt.Sprintf(`You are resolving merge conflicts in a git repository.

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

	logging.Debug("resolveConflictsWithClaude: starting conflict resolution for %d files", len(conflictFiles))
	logging.Trace("resolveConflictsWithClaude: prompt=%s", prompt)

	// Set a timeout for conflict resolution (5 minutes)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Run claude with the conflict resolution prompt
	cmd := exec.CommandContext(ctx, "claude", "--dangerously-skip-permissions", prompt)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		logging.Warn("resolveConflictsWithClaude: claude command failed: %v", err)
		return fmt.Errorf("claude conflict resolution failed: %w", err)
	}

	logging.Debug("resolveConflictsWithClaude: claude completed successfully")
	return nil
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
	// os.FindProcess always succeeds on Unix, so we use the signal check
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

var resumeAgentCmd = &cobra.Command{
	Use:   "resume-agent [session] [window-id] [agent-dir]",
	Short: "Resume a stopped Claude agent in an existing window",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]
		agentDir := args[2]

		taskName := filepath.Base(agentDir)

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("resume-agent")
			logger.SetTask(taskName)
			logging.SetGlobal(logger)
		}

		logging.Log("=== Resuming agent: %s ===", taskName)

		tm := tmux.New(sessionName)

		// Get task
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
		t, err := mgr.GetTask(taskName)
		if err != nil {
			return fmt.Errorf("failed to get task: %w", err)
		}

		// Determine work directory
		workDir := mgr.GetWorkingDirectory(t)

		// Get paw binary path
		pawBin, _ := os.Executable()

		// Build start-agent script with --continue flag
		worktreeDirExport := ""
		if app.IsWorktreeMode() {
			worktreeDirExport = fmt.Sprintf("export WORKTREE_DIR='%s'\n", workDir)
		}

		startAgentContent := fmt.Sprintf(`#!/bin/bash
# Auto-generated start-agent script for this task (RESUME MODE)
export TASK_NAME='%s'
export PAW_DIR='%s'
export PROJECT_DIR='%s'
%sexport WINDOW_ID='%s'
export ON_COMPLETE='%s'
export PAW_HOME='%s'
export PAW_BIN='%s'
export SESSION_NAME='%s'

# Continue the previous Claude session (--continue auto-selects last session)
exec claude --continue --dangerously-skip-permissions
`, taskName, app.PawDir, app.ProjectDir, worktreeDirExport, windowID,
			app.Config.OnComplete, filepath.Dir(filepath.Dir(pawBin)), pawBin, sessionName)

		startAgentScriptPath := filepath.Join(t.AgentDir, "start-agent")
		if err := os.WriteFile(startAgentScriptPath, []byte(startAgentContent), 0755); err != nil {
			return fmt.Errorf("failed to write start-agent script: %w", err)
		}

		agentPane := windowID + ".0"

		// Respawn the agent pane with the resume script
		if err := tm.RespawnPane(agentPane, workDir, startAgentScriptPath); err != nil {
			return fmt.Errorf("failed to respawn agent pane: %w", err)
		}

		logging.Log("Agent resumed: task=%s, windowID=%s", taskName, windowID)

		// Start wait watcher
		watchCmd := exec.Command(pawBin, "internal", "watch-wait", sessionName, windowID, taskName)
		watchCmd.Dir = app.ProjectDir
		if err := watchCmd.Start(); err != nil {
			logging.Warn("Failed to start wait watcher: %v", err)
		} else {
			logging.Debug("Wait watcher started for windowID=%s", windowID)
		}

		// Notify user
		notify.PlaySound(notify.SoundTaskCreated)
		// Send to all configured notification channels (macOS, Slack, ntfy)
		notify.SendAll(app.Config.Notifications, "Session resumed", fmt.Sprintf("🔄 %s resumed", taskName))
		if err := tm.DisplayMessage(fmt.Sprintf("🔄 Session resumed: %s", taskName), 2000); err != nil {
			logging.Trace("Failed to display message: %v", err)
		}

		return nil
	},
}
