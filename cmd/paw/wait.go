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
	waitCaptureLines = 200
	waitPollInterval = 2 * time.Second
	waitPopupWidth   = "70%"
	waitPopupHeight  = "50%"
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

// watchWaitCmd monitors the agent pane and sends notifications when user input is needed.
// NOTE: This watcher does NOT change task status. Status transitions are handled by hooks:
//   - PreToolUse (AskUserQuestion): WAITING
//   - PostToolUse (AskUserQuestion): WORKING
//   - UserPromptSubmit: WORKING
//   - Stop: DONE/WORKING (via .status-signal or Claude classification)
//
// This watcher only:
//  1. Detects when window is in WAITING state (set by hooks)
//  2. Parses prompt content and sends notifications
//  3. Handles notification action responses
var watchWaitCmd = &cobra.Command{
	Use:   "watch-wait [session] [window-id] [task-name]",
	Short: "Watch agent output and notify when user input is needed",
	Args:  cobra.ExactArgs(3),
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]
		taskName := args[2]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		_, cleanup := setupLoggerFromApp(app, "watch-wait", taskName)
		defer cleanup()

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

			isWaiting := isWaitingWindow(windowName)

			// Reset notified flag when window leaves waiting state
			// This allows re-notification when a new wait state begins
			if !isWaiting {
				notified = false
				lastPromptKey = ""
			}

			// Only process notifications when in WAITING state (set by hooks)
			if isWaiting && !notified {
				content, err := tm.CapturePane(paneID, waitCaptureLines)
				if err != nil {
					errStr := err.Error()
					// Check if the error indicates the window/pane is gone (expected during cleanup)
					if strings.Contains(errStr, "can't find window") || strings.Contains(errStr, "can't find pane") {
						logging.Debug("Window/pane no longer exists (capture failed), stopping wait watcher: %v", err)
						return nil
					}
					// Other unexpected errors - log as warning and continue
					logging.Warn("Failed to capture pane: %v", err)
					time.Sleep(waitPollInterval)
					continue
				}

				contentChanged := content != lastContent
				lastContent = content

				if contentChanged {
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
						// Only notify if: prompt is valid and it's a new prompt
						if promptKey != "" && promptKey != lastPromptKey {
							lastPromptKey = promptKey
							notified = true
							// Try notification actions for simple prompts
							choice := tryNotificationAction(taskName, prompt)
							// If user selects an action from notification, send it to the agent
							if choice != "" {
								if sendErr := sendAgentResponse(tm, paneID, choice); sendErr != nil {
									logging.Warn("Failed to send prompt response: %v", sendErr)
								} else {
									logging.Debug("Sent prompt response: %s", choice)
								}
							}
						}
					} else {
						// No parseable prompt, send simple notification
						logging.Debug("Wait state detected, sending notification")
						notifyWaitingWithDisplay(tm, taskName, "input needed")
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

// isFinalWindow returns true if the window is in a completed state.
func isFinalWindow(name string) bool {
	return strings.HasPrefix(name, constants.EmojiDone)
}
