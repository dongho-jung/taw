package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/notify"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var syncWithMainCmd = &cobra.Command{
	Use:   "sync-with-main [session] [window-id]",
	Short: "Sync task branch with main (fetch and rebase)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Trace("syncWithMainCmd: start session=%s windowID=%s", args[0], args[1])
		defer logging.Trace("syncWithMainCmd: end")

		sessionName := args[0]
		windowID := args[1]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// This command only works in git mode
		if !app.IsGitRepo {
			fmt.Println("\n  ✗ Not a git repository - sync not available")
			return nil
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
			logger.SetScript("sync-with-main")
			logger.SetTask(targetTask.Name)
			logging.SetGlobal(logger)
		}

		logging.Log("=== Sync with main: %s ===", targetTask.Name)

		// Print header for user feedback
		fmt.Printf("\n  Syncing task with main: %s\n\n", targetTask.Name)

		gitClient := git.New()
		workDir := mgr.GetWorkingDirectory(targetTask)
		logging.Trace("Working directory: %s", workDir)

		// Check for uncommitted changes
		if gitClient.HasChanges(workDir) {
			fmt.Println("  ⚠️  You have uncommitted changes")
			fmt.Println("  Please commit or stash them before syncing")
			return nil
		}

		// Check for ongoing rebase or merge
		if gitClient.HasOngoingRebase(workDir) {
			fmt.Println("  ⚠️  There's an ongoing rebase operation")
			fmt.Println("  Please complete or abort it first:")
			fmt.Println("    git rebase --continue  # to continue")
			fmt.Println("    git rebase --abort     # to abort")
			return nil
		}

		if gitClient.HasOngoingMerge(workDir) {
			fmt.Println("  ⚠️  There's an ongoing merge operation")
			fmt.Println("  Please complete or abort it first:")
			fmt.Println("    git merge --continue  # to continue")
			fmt.Println("    git merge --abort     # to abort")
			return nil
		}

		// Get main branch name
		mainBranch := gitClient.GetMainBranch(app.ProjectDir)
		logging.Debug("Main branch: %s", mainBranch)

		// Get current branch
		currentBranch, err := gitClient.GetCurrentBranch(workDir)
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}
		logging.Debug("Current branch: %s", currentBranch)

		// Fetch from origin
		fetchSpinner := tui.NewSimpleSpinner("Fetching from origin")
		fetchSpinner.Start()

		fetchTimer := logging.StartTimer("git fetch")
		if err := gitClient.Fetch(workDir, "origin"); err != nil {
			fetchTimer.StopWithResult(false, err.Error())
			fetchSpinner.Stop(false, err.Error())
			return fmt.Errorf("failed to fetch from origin: %w", err)
		}
		fetchTimer.StopWithResult(true, "")
		fetchSpinner.Stop(true, "")

		// Check if there are new commits on main
		remoteMain := fmt.Sprintf("origin/%s", mainBranch)
		behindCount, err := getBehindCount(workDir, currentBranch, remoteMain)
		if err != nil {
			logging.Warn("Failed to check commit count: %v", err)
			behindCount = "unknown"
		}

		if behindCount == "0" {
			fmt.Printf("  ○ Already up to date with %s\n", mainBranch)
			fmt.Println("\n  ✓ No sync needed!")
			return nil
		}

		fmt.Printf("  ℹ️  %s new commit(s) on %s\n\n", behindCount, mainBranch)

		// Rebase onto origin/main
		rebaseSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Rebasing %s onto origin/%s", currentBranch, mainBranch))
		rebaseSpinner.Start()

		rebaseTimer := logging.StartTimer("git rebase")
		if err := gitClient.Rebase(workDir, remoteMain); err != nil {
			rebaseTimer.StopWithResult(false, "conflict")
			rebaseSpinner.Stop(false, "conflict")

			logging.Warn("Rebase failed (likely conflict): %v", err)
			fmt.Println()
			fmt.Println("  ⚠️  Rebase conflict detected")
			fmt.Println()
			fmt.Println("  Please resolve the conflicts manually:")
			fmt.Printf("    cd %s\n", workDir)
			fmt.Println("    # Edit conflicting files")
			fmt.Println("    git add <resolved-files>")
			fmt.Println("    git rebase --continue")
			fmt.Println()
			fmt.Println("  Or abort the rebase:")
			fmt.Println("    git rebase --abort")
			fmt.Println()
			return nil
		}
		rebaseTimer.StopWithResult(true, fmt.Sprintf("rebased onto origin/%s", mainBranch))
		rebaseSpinner.Stop(true, "")

		logging.Log("Successfully synced %s with %s", targetTask.Name, mainBranch)
		fmt.Println()
		fmt.Printf("  ✓ Successfully synced with %s!\n", mainBranch)

		return nil
	},
}

var syncWithMainUICmd = &cobra.Command{
	Use:   "sync-with-main-ui [session] [window-id]",
	Short: "Sync with main (creates visible pane with progress)",
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
			logger.SetScript("sync-with-main-ui")
			logging.SetGlobal(logger)
		}

		logging.Trace("syncWithMainUICmd: start session=%s windowID=%s", sessionName, windowID)
		defer logging.Trace("syncWithMainUICmd: end")

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

		// Build sync-with-main command that runs in the pane
		syncCmdStr := fmt.Sprintf("%s internal sync-with-main %s %s; echo; echo 'Press Enter to close...'; read",
			pawBin, sessionName, windowID)

		// Create a top pane (40% height) spanning full window width
		_, err = tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false, // vertical split (top/bottom)
			Size:       "40%",
			StartDir:   panePath,
			Command:    syncCmdStr,
			Before:     true, // create pane above (top)
			Full:       true, // span entire window width
		})
		if err != nil {
			return fmt.Errorf("failed to create sync-with-main pane: %w", err)
		}

		return nil
	},
}

var syncTaskCmd = &cobra.Command{
	Use:   "sync-task [session]",
	Short: "Sync current task with main branch (double-press to confirm)",
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
			logger.SetScript("sync-task")
			logging.SetGlobal(logger)
		}

		logging.Trace("syncTaskCmd: start session=%s", sessionName)
		defer logging.Trace("syncTaskCmd: end")

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
		if !strings.HasPrefix(windowName, constants.EmojiWorking) &&
			!strings.HasPrefix(windowName, constants.EmojiWaiting) &&
			!strings.HasPrefix(windowName, constants.EmojiDone) &&
			!strings.HasPrefix(windowName, constants.EmojiWarning) {
			_ = tm.DisplayMessage("Not a task window", 1500)
			return nil
		}

		// Check if git mode
		if !app.IsGitRepo {
			_ = tm.DisplayMessage("Sync not available (not a git repo)", 1500)
			return nil
		}

		// Get pending sync timestamp from tmux option
		pendingTimeStr, _ := tm.GetOption("@paw_sync_pending")
		now := time.Now().Unix()

		// Check if there's a pending sync within 2 seconds
		if pendingTimeStr != "" {
			pendingTime, err := parseUnixTime(pendingTimeStr)
			if err == nil && now-pendingTime <= constants.DoublePressIntervalSec {
				// Double-press detected, sync the task
				_ = tm.SetOption("@paw_sync_pending", "", true) // Clear pending state

				// Delegate to sync-with-main-ui
				pawBin, _ := os.Executable()
				syncCmd := exec.Command(pawBin, "internal", "sync-with-main-ui", sessionName, windowID)
				return syncCmd.Run()
			}
		}

		// First press: show warning
		// Store current timestamp
		_ = tm.SetOption("@paw_sync_pending", fmt.Sprintf("%d", now), true)

		// Play sound to indicate pending state
		logging.Trace("syncTaskCmd: playing SoundCancelPending (first press, waiting for second)")
		notify.PlaySound(notify.SoundCancelPending)

		// Show message to user
		_ = tm.DisplayMessage("⌃↓ again to sync", 2000)

		return nil
	},
}

var toggleBranchCmd = &cobra.Command{
	Use:   "toggle-branch [session]",
	Short: "Toggle between task branch and main branch",
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
			logger.SetScript("toggle-branch")
			logging.SetGlobal(logger)
		}

		logging.Trace("toggleBranchCmd: start session=%s", sessionName)
		defer logging.Trace("toggleBranchCmd: end")

		tm := tmux.New(sessionName)

		// Check if git mode
		if !app.IsGitRepo {
			_ = tm.DisplayMessage("Not a git repository", 1500)
			return nil
		}

		// Get current window name
		windowName, _ := tm.Display("#{window_name}")
		windowName = strings.TrimSpace(windowName)

		// Check if this is a task window (has emoji prefix)
		if !strings.HasPrefix(windowName, constants.EmojiWorking) &&
			!strings.HasPrefix(windowName, constants.EmojiWaiting) &&
			!strings.HasPrefix(windowName, constants.EmojiDone) &&
			!strings.HasPrefix(windowName, constants.EmojiWarning) {
			_ = tm.DisplayMessage("Not a task window", 1500)
			return nil
		}

		// Get current window ID
		windowID, err := tm.Display("#{window_id}")
		if err != nil {
			return fmt.Errorf("failed to get window ID: %w", err)
		}
		windowID = strings.TrimSpace(windowID)

		// Find task by window ID
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
		tasks, err := mgr.ListTasks()
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		var targetTask *task.Task
		for _, t := range tasks {
			if id, _ := t.LoadWindowID(); id == windowID {
				targetTask = t
				break
			}
		}

		if targetTask == nil {
			_ = tm.DisplayMessage("Task not found", 1500)
			return nil
		}

		gitClient := git.New()
		workDir := mgr.GetWorkingDirectory(targetTask)

		// Get current branch
		currentBranch, err := gitClient.GetCurrentBranch(workDir)
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}

		// Get main branch name
		mainBranch := gitClient.GetMainBranch(app.ProjectDir)

		logging.Debug("Current branch: %s, Main branch: %s, Task: %s", currentBranch, mainBranch, targetTask.Name)

		// Toggle between task branch and main branch
		var targetBranch string
		if currentBranch == mainBranch {
			// Currently on main, switch to task branch
			targetBranch = targetTask.Name
		} else {
			// Currently on task branch (or other), switch to main
			targetBranch = mainBranch
		}

		// Check for uncommitted changes
		if gitClient.HasChanges(workDir) {
			_ = tm.DisplayMessage("Uncommitted changes - commit or stash first", 2000)
			return nil
		}

		// Checkout target branch
		if err := gitClient.Checkout(workDir, targetBranch); err != nil {
			_ = tm.DisplayMessage(fmt.Sprintf("Checkout failed: %v", err), 2000)
			return fmt.Errorf("failed to checkout %s: %w", targetBranch, err)
		}

		_ = tm.DisplayMessage(fmt.Sprintf("Switched to %s", targetBranch), 1500)
		logging.Info("Switched from %s to %s", currentBranch, targetBranch)

		return nil
	},
}

// getBehindCount returns how many commits the current branch is behind origin/main
func getBehindCount(workDir, currentBranch, remoteMain string) (string, error) {
	cmd := exec.Command("git", "rev-list", "--count", currentBranch+".."+remoteMain)
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		return "0", err
	}
	return strings.TrimSpace(string(output)), nil
}
