package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

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

var cancelTaskCmd = &cobra.Command{
	Use:   "cancel-task [session] [window-id]",
	Short: "Cancel a task (with revert if merged)",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "cancel-task", "")
		defer cleanup()

		logging.Debug("-> cancelTaskCmd(session=%s, windowID=%s)", sessionName, windowID)
		defer logging.Debug("<- cancelTaskCmd")

		tm := tmux.New(sessionName)

		fmt.Println()
		fmt.Println("  ╭─────────────────────────────────────╮")
		fmt.Println("  │         Cancelling Task...          │")
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

		revertNeeded := false
		revertSuccess := false

		// Check if task was merged and needs to be reverted
		if appCtx.IsGitRepo {
			gitClient := git.New()
			mainBranch := gitClient.GetMainBranch(appCtx.ProjectDir)

			// Check if branch was merged into main
			if gitClient.BranchMerged(appCtx.ProjectDir, targetTask.Name, mainBranch) {
				revertNeeded = true
				logging.Trace("cancelTaskCmd: task %s was merged, attempting revert", targetTask.Name)

				revertSpinner := tui.NewSimpleSpinner("Reverting merge")
				revertSpinner.Start()

				// Find the merge commit
				mergeCommit, err := gitClient.FindMergeCommit(appCtx.ProjectDir, targetTask.Name, mainBranch)
				if err != nil {
					revertSpinner.Stop(false, "Failed")
					logging.Warn("Failed to find merge commit: %v", err)
					fmt.Println("  ✗ Failed to find merge commit")
				} else if mergeCommit != "" {
					logging.Trace("cancelTaskCmd: found merge commit %s, reverting", mergeCommit)

					// Checkout main branch first
					if err := gitClient.Checkout(appCtx.ProjectDir, mainBranch); err != nil {
						revertSpinner.Stop(false, "Checkout failed")
						logging.Warn("Failed to checkout main branch: %v", err)
						fmt.Printf("  ✗ Failed to checkout %s\n", mainBranch)
					} else {
						// Revert the merge commit
						if err := gitClient.RevertCommit(appCtx.ProjectDir, mergeCommit, ""); err != nil {
							revertSpinner.Stop(false, "Conflict")
							logging.Warn("Failed to revert merge commit: %v", err)
							fmt.Println("  ✗ Revert failed (conflict?)")
							fmt.Println()
							fmt.Println("  ⚠️  Manual resolution required:")
							fmt.Printf("     cd %s\n", appCtx.ProjectDir)
							fmt.Printf("     git revert -m 1 %s\n", mergeCommit)
							fmt.Println("     # Resolve conflicts, then commit and push")
							fmt.Println()

							// Abort revert if in progress
							abortCmd := exec.Command("git", "revert", "--abort")
							abortCmd.Dir = appCtx.ProjectDir
							_ = abortCmd.Run()

							// Rename window to corrupted state and notify user
							corruptedName := windowNameForStatus(targetTask.Name, task.StatusCorrupted)
							_ = renameWindowWithStatus(tm, windowID, corruptedName, appCtx.PawDir, targetTask.Name, "cancel-task", task.StatusCorrupted)
							notify.PlaySound(notify.SoundError)
							_ = notify.Send("Revert conflict", fmt.Sprintf("⚠️ %s - manual resolution needed", targetTask.Name))
							return nil // Don't cleanup - keep task for manual resolution
						}
						revertSpinner.Stop(true, "Reverted")
						logging.Log("Reverted merge commit %s for task %s", mergeCommit, targetTask.Name)

						// Push the revert
						pushSpinner := tui.NewSimpleSpinner("Pushing revert")
						pushSpinner.Start()

						if err := gitClient.Push(appCtx.ProjectDir, "origin", mainBranch, false); err != nil {
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
				} else {
					revertSpinner.Stop(false, "Not found")
					fmt.Println("  ✗ Merge commit not found")
				}
			}
		}

		fmt.Println()

		if appCtx.Config != nil && appCtx.Config.PostTaskHook != "" {
			hookEnv := appCtx.GetEnvVars(targetTask.Name, mgr.GetWorkingDirectory(targetTask), windowID)
			if _, err := service.RunHook(
				"post-task",
				appCtx.Config.PostTaskHook,
				mgr.GetWorkingDirectory(targetTask),
				hookEnv,
				targetTask.GetHookOutputPath("post-task"),
				targetTask.GetHookMetaPath("post-task"),
				constants.DefaultHookTimeout,
			); err != nil {
				logging.Warn("Post-task hook failed: %v", err)
			}
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
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "cancel-task-ui", "")
		defer cleanup()

		logging.Debug("-> cancelTaskUICmd(session=%s, windowID=%s)", sessionName, windowID)
		defer logging.Debug("<- cancelTaskUICmd")

		tm := tmux.New(sessionName)

		// Get the paw binary path
		pawBin := getPawBin()

		// Get working directory from pane
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			panePath = appCtx.ProjectDir
		}

		// Build cancel-task command
		// CRITICAL: Pass PAW_DIR as env var so cancel-task can find the correct project
		// even if the agent changed its working directory (e.g., cd /tmp)
		cancelTaskCmdStr := strings.Join([]string{
			shellEnv("PAW_DIR", appCtx.PawDir),
			shellJoin(pawBin, "internal", "cancel-task", sessionName, windowID),
		}, " ")
		cancelTaskCmdStr += "; echo; echo 'Press Enter to close...'; read"

		// Create a top pane spanning full window width
		_, err = tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false,
			Size:       constants.TopPaneSize,
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
