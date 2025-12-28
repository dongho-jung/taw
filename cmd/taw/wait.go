package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/donghojung/taw/internal/constants"
	"github.com/donghojung/taw/internal/logging"
	"github.com/donghojung/taw/internal/notify"
	"github.com/donghojung/taw/internal/tmux"
)

const (
	waitMarker             = "TAW_WAITING"
	waitCaptureLines       = 200
	waitPollInterval       = 2 * time.Second
	waitMarkerMaxDistance  = 8
	waitAskUserMaxDistance = 32
)

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
			defer logger.Close()
			logger.SetScript("watch-wait")
			logger.SetTask(taskName)
			logging.SetGlobal(logger)
		}

		tm := tmux.New(sessionName)
		paneID := windowID + ".0"

		var lastContent string
		notified := false

		for {
			if !tm.HasPane(paneID) {
				logging.Debug("Pane %s no longer exists, stopping wait watcher", paneID)
				return nil
			}

			isFinal := false
			windowName, err := getWindowName(tm, windowID)
			if err == nil {
				isFinal = isFinalWindow(windowName)
				if isWaitingWindow(windowName) {
					if !notified {
						notifyWaiting(taskName)
						notified = true
					}
				} else {
					notified = false
				}
			}

			content, err := tm.CapturePane(paneID, waitCaptureLines)
			if err != nil {
				logging.Debug("Failed to capture pane: %v", err)
				time.Sleep(waitPollInterval)
				continue
			}

			if content == lastContent {
				time.Sleep(waitPollInterval)
				continue
			}
			lastContent = content

			waitDetected, reason := detectWaitInContent(content)
			if waitDetected && !isFinal {
				if err := ensureWaitingWindow(tm, windowID, taskName); err != nil {
					logging.Debug("Failed to rename window: %v", err)
				}
				if !notified {
					logging.Log("Wait detected: %s", reason)
					notifyWaiting(taskName)
					notified = true
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

func ensureWaitingWindow(tm tmux.Client, windowID, taskName string) error {
	windowName, err := getWindowName(tm, windowID)
	if err == nil {
		if isWaitingWindow(windowName) || isFinalWindow(windowName) {
			return nil
		}
	}
	return tm.RenameWindow(windowID, waitingWindowName(taskName))
}

func waitingWindowName(taskName string) string {
	name := taskName
	if len(name) > 12 {
		name = name[:12]
	}
	return constants.EmojiWaiting + name
}

func detectWaitInContent(content string) (bool, string) {
	lines := strings.Split(content, "\n")
	lines = trimTrailingEmpty(lines)
	if len(lines) == 0 || !hasInputPrompt(lines) {
		return false, ""
	}

	index, reason := findWaitMarker(lines)
	if index == -1 {
		return false, ""
	}

	linesAfter := len(lines) - index - 1
	maxDistance := waitMarkerMaxDistance
	if reason == "AskUserQuestion" {
		maxDistance = waitAskUserMaxDistance
	}
	if linesAfter <= maxDistance {
		return true, reason
	}

	return false, ""
}

func trimTrailingEmpty(lines []string) []string {
	end := len(lines)
	for end > 0 {
		if strings.TrimSpace(lines[end-1]) != "" {
			break
		}
		end--
	}
	return lines[:end]
}

func hasInputPrompt(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	return strings.HasPrefix(last, ">")
}

func findWaitMarker(lines []string) (int, string) {
	index := -1
	reason := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == waitMarker:
			index = i
			reason = "marker"
		case strings.HasPrefix(trimmed, "AskUserQuestion"):
			index = i
			reason = "AskUserQuestion"
		}
	}
	return index, reason
}

func notifyWaiting(taskName string) {
	title := "TAW: Waiting for input"
	message := fmt.Sprintf("Task %s needs your response.", taskName)
	if err := notify.Send(title, message); err != nil {
		logging.Debug("Failed to send notification: %v", err)
	}
}
