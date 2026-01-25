package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/github"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/notify"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

var watchPRCmd = &cobra.Command{
	Use:   "watch-pr [session] [window-id] [task-name] [pr-number]",
	Short: "Watch PR status and clean up when merged",
	Args:  cobra.ExactArgs(4),
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]
		taskName := args[2]
		prNumberStr := args[3]

		prNumber, err := strconv.Atoi(prNumberStr)
		if err != nil {
			return fmt.Errorf("invalid PR number: %s", prNumberStr)
		}

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		_, cleanup := setupLoggerFromApp(appCtx, "watch-pr", taskName)
		defer cleanup()

		logging.Debug("-> watchPRCmd(session=%s, windowID=%s, task=%s, pr=%d)", sessionName, windowID, taskName, prNumber)
		defer logging.Debug("<- watchPRCmd")

		ghClient := github.New()
		if !ghClient.IsInstalled() {
			logging.Warn("gh CLI not installed; PR watcher exiting")
			return nil
		}

		tm := tmux.New(sessionName)
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)

		ticker := time.NewTicker(constants.PRWatchInterval)
		defer ticker.Stop()

		for {
			windowName, err := getWindowName(tm, windowID)
			if err != nil {
				logging.Debug("Window %s no longer exists, stopping PR watcher", windowID)
				return nil
			}

			if extractedName, isTask := constants.ExtractTaskName(windowName); isTask {
				if !constants.MatchesWindowToken(extractedName, taskName) {
					logging.Debug("Window %s now belongs to different task (%s vs %s), stopping PR watcher",
						windowID, extractedName, taskName)
					return nil
				}
			}

			status, err := ghClient.GetPRStatus(appCtx.ProjectDir, prNumber)
			if err != nil {
				logging.Warn("Failed to check PR status: %v", err)
			} else if status.Merged {
				logging.Info("PR merged: task=%s pr=%d", taskName, prNumber)
				notify.PlaySound(notify.SoundTaskCompleted)
				_ = notify.Send("PR merged", fmt.Sprintf("✅ %s merged and cleaned up", taskName))

				t, err := mgr.GetTask(taskName)
				if err != nil {
					logging.Warn("Failed to load task for cleanup: %v", err)
					return nil
				}

				if err := mgr.CleanupTask(t); err != nil {
					logging.Warn("Failed to clean up task: %v", err)
				}

				if err := tm.KillWindow(windowID); err != nil {
					logging.Warn("Failed to kill window: %v", err)
				}
				return nil
			} else if status.State == "closed" {
				logging.Info("PR closed without merge: task=%s pr=%d", taskName, prNumber)
				warnName := constants.EmojiWarning + constants.TruncateForWindowName(taskName)
				if err := renameWindowWithStatus(tm, windowID, warnName, appCtx.PawDir, taskName, "watch-pr", task.StatusWaiting); err != nil {
					logging.Warn("Failed to rename window for PR warning: %v", err)
				}
				return nil
			} else if status.State == "open" && !strings.HasPrefix(windowName, constants.EmojiWorking) {
				if !strings.HasPrefix(windowName, constants.EmojiReview) && !strings.HasPrefix(windowName, constants.EmojiWarning) {
					reviewName := constants.EmojiReview + constants.TruncateForWindowName(taskName)
					if err := renameWindowWithStatus(tm, windowID, reviewName, appCtx.PawDir, taskName, "watch-pr", task.StatusWaiting); err != nil {
						logging.Warn("Failed to rename window for PR review: %v", err)
					}
				}
			}

			<-ticker.C
		}
	},
}
