package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var syncWithMainCmd = &cobra.Command{
	Use:   "sync-with-main [session] [window-id]",
	Short: "Sync task branch with main (fetch and rebase)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> syncWithMainCmd(session=%s, windowID=%s)", args[0], args[1])
		defer logging.Debug("<- syncWithMainCmd")

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
		_, cleanup := setupLoggerFromApp(app, "sync-with-main", targetTask.Name)
		defer cleanup()

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
		_, cleanup := setupLoggerFromApp(app, "sync-with-main-ui", "")
		defer cleanup()

		logging.Debug("-> syncWithMainUICmd(session=%s, windowID=%s)", sessionName, windowID)
		defer logging.Debug("<- syncWithMainUICmd")

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
		syncCmdStr := shellJoin(pawBin, "internal", "sync-with-main", sessionName, windowID)
		syncCmdStr += "; echo; echo 'Press Enter to close...'; read"

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
