package main

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

const (
	waitMarker             = "PAW_WAITING"
	waitCaptureLines       = 200
	waitPollInterval       = 2 * time.Second
	waitMarkerMaxDistance  = 8
	waitAskUserMaxDistance = 32
	doneMarkerMaxDistance  = 500 // Allow more distance since agent may continue talking after PAW_DONE
	waitPopupWidth         = "70%"
	waitPopupHeight        = "50%"
	// Maximum number of options for notification action buttons
	notifyMaxActions = 5
	// Timeout for waiting for notification response
	notifyTimeoutSec = 30
)

var askUserQuestionUIMarkers = []string{
	"Enter to select",
	"Tab/Arrow keys to navigate",
	"Esc to cancel",
	"Type something.",
}

var watchWaitCmd = &cobra.Command{
	Use:   "watch-wait [session] [window-id] [task-name]",
	Short: "Watch agent output and notify when user input is needed",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]
		taskName := args[2]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("watch-wait")
			logger.SetTask(taskName)
			logging.SetGlobal(logger)
		}

		logging.Debug("-> watchWaitCmd(session=%s, windowID=%s, task=%s)", sessionName, windowID, taskName)
		defer logging.Debug("<- watchWaitCmd")

		tm := tmux.New(sessionName)
		paneID := windowID + ".0"

		var lastContent string
		var lastPromptKey string
		notified := false

		for {
			if !tm.HasPane(paneID) {
				logging.Debug("Pane %s no longer exists, stopping wait watcher", paneID)
				return nil
			}

			isFinal := false
			windowName, err := getWindowName(tm, windowID)
			if err != nil {
				// Window doesn't exist anymore, stop watcher
				logging.Debug("Window %s no longer exists, stopping wait watcher", windowID)
				return nil
			}

			// Verify this window still belongs to this task (prevents stale watcher issues)
			if extractedName, isTask := constants.ExtractTaskName(windowName); isTask {
				if !constants.MatchesWindowToken(extractedName, taskName) {
					// Window was reassigned to a different task, stop this watcher
					logging.Debug("Window %s now belongs to different task (%s vs %s), stopping wait watcher",
						windowID, extractedName, taskName)
					return nil
				}
			}

			isFinal = isFinalWindow(windowName)
			if isWaitingWindow(windowName) {
				if !notified {
					notifyWaitingWithDisplay(tm, app.Config.Notifications, taskName, "window")
					notified = true
				}
			} else {
				notified = false
			}

			content, err := tm.CapturePane(paneID, waitCaptureLines)
			if err != nil {
				logging.Trace("Failed to capture pane: %v", err)
				time.Sleep(waitPollInterval)
				continue
			}

			contentChanged := content != lastContent
			if contentChanged {
				lastContent = content

				// Check for done marker first (takes priority over wait detection)
				doneDetected := detectDoneInContent(content)
				if doneDetected && !isFinal {
					logging.Debug("Done marker detected in pane content")
					if err := ensureDoneWindow(tm, windowID, taskName, app.PawDir); err != nil {
						logging.Trace("Failed to rename window to done: %v", err)
					}
					// Skip wait detection since task is done
					time.Sleep(waitPollInterval)
					continue
				}

				waitDetected, reason := detectWaitInContent(content)

				if waitDetected && !isFinal {
					if err := ensureWaitingWindow(tm, windowID, taskName, app.PawDir); err != nil {
						logging.Trace("Failed to rename window: %v", err)
					}
					// Try to parse the prompt - first YAML format, then rendered UI format
					prompt, ok := parseAskUserQuestion(content)
					if !ok {
						logging.Trace("parseAskUserQuestion failed, trying UI parser")
						prompt, ok = parseAskUserQuestionUI(content)
						if !ok {
							logging.Trace("parseAskUserQuestionUI also failed, content length=%d", len(content))
						}
					}
					if ok {
						promptKey := prompt.key()
						if promptKey != "" && promptKey != lastPromptKey {
							lastPromptKey = promptKey
							// Try notification actions for simple prompts
							// Note: tryNotificationAction always shows a notification (either with actions
							// or fallback to simple), so we mark notified=true before calling it
							notified = true
							choice := tryNotificationAction(taskName, prompt)
							// If user selects an action from notification, send it to the agent
							if choice != "" {
								if sendErr := sendAgentResponse(tm, paneID, choice); sendErr != nil {
									logging.Trace("Failed to send prompt response: %v", sendErr)
								} else {
									logging.Debug("Sent prompt response: %s", choice)
								}
							}
							// If dismissed/timeout, user will handle it directly in the UI
						}
					} else if !notified {
						logging.Debug("Wait detected: %s", reason)
						notifyWaitingWithDisplay(tm, app.Config.Notifications, taskName, reason)
						notified = true
					}
				}
			}

			time.Sleep(waitPollInterval)
		}
	},
}

func getWindowName(tm tmux.Client, windowID string) (string, error) {
	return tm.RunWithOutput("display-message", "-t", windowID, "-p", "#{window_name}")
}

func isWaitingWindow(name string) bool {
	return strings.HasPrefix(name, constants.EmojiWaiting)
}

func isFinalWindow(name string) bool {
	return strings.HasPrefix(name, constants.EmojiDone) ||
		strings.HasPrefix(name, constants.EmojiWarning)
}

func ensureWaitingWindow(tm tmux.Client, windowID, taskName, pawDir string) error {
	logging.Debug("-> ensureWaitingWindow(windowID=%s, task=%s)", windowID, taskName)
	defer logging.Debug("<- ensureWaitingWindow")

	windowName, err := getWindowName(tm, windowID)
	if err != nil {
		// Window doesn't exist, nothing to do
		return nil
	}

	// Check if this window belongs to this task (prevents cross-task renaming)
	extractedName, isTask := constants.ExtractTaskName(windowName)
	if !isTask {
		// Not a task window, don't rename
		return nil
	}

	if !constants.MatchesWindowToken(extractedName, taskName) {
		// Wrong task window, don't rename (prevents race condition)
		return nil
	}

	// Already in final state, don't change
	if isWaitingWindow(windowName) || isFinalWindow(windowName) {
		return nil
	}

	newName := waitingWindowName(taskName)
	logging.Trace("ensureWaitingWindow: renaming window from=%s to=%s", windowName, newName)
	return renameWindowWithStatus(tm, windowID, newName, pawDir, taskName, "watch-wait")
}

func waitingWindowName(taskName string) string {
	return constants.EmojiWaiting + constants.TruncateForWindowName(taskName)
}

func doneWindowName(taskName string) string {
	return constants.EmojiDone + constants.TruncateForWindowName(taskName)
}

// ensureDoneWindow renames the window to done status if it belongs to the task.
func ensureDoneWindow(tm tmux.Client, windowID, taskName, pawDir string) error {
	logging.Debug("-> ensureDoneWindow(windowID=%s, task=%s)", windowID, taskName)
	defer logging.Debug("<- ensureDoneWindow")

	windowName, err := getWindowName(tm, windowID)
	if err != nil {
		// Window doesn't exist, nothing to do
		return nil
	}

	// Check if this window belongs to this task (prevents cross-task renaming)
	extractedName, isTask := constants.ExtractTaskName(windowName)
	if !isTask {
		// Not a task window, don't rename
		return nil
	}

	if !constants.MatchesWindowToken(extractedName, taskName) {
		// Wrong task window, don't rename (prevents race condition)
		return nil
	}

	// Already in final state, don't change
	if isFinalWindow(windowName) {
		return nil
	}

	newName := doneWindowName(taskName)
	logging.Trace("ensureDoneWindow: renaming window from=%s to=%s", windowName, newName)
	return renameWindowWithStatus(tm, windowID, newName, pawDir, taskName, "watch-wait")
}

