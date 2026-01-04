package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/notify"
	"github.com/dongho-jung/paw/internal/tmux"
)

const (
	waitMarker             = "PAW_WAITING"
	waitCaptureLines       = 200
	waitPollInterval       = 2 * time.Second
	waitMarkerMaxDistance  = 8
	waitAskUserMaxDistance = 32
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

		logging.Trace("watchWaitCmd: start session=%s windowID=%s task=%s", sessionName, windowID, taskName)
		defer logging.Trace("watchWaitCmd: end")

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
				expectedName := taskName
				if len(expectedName) > 12 {
					expectedName = expectedName[:12]
				}
				if extractedName != expectedName {
					// Window was reassigned to a different task, stop this watcher
					logging.Debug("Window %s now belongs to different task (%s vs %s), stopping wait watcher",
						windowID, extractedName, expectedName)
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

				waitDetected, reason := detectWaitInContent(content)

				if waitDetected && !isFinal {
					if err := ensureWaitingWindow(tm, windowID, taskName); err != nil {
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

func ensureWaitingWindow(tm tmux.Client, windowID, taskName string) error {
	logging.Trace("ensureWaitingWindow: start windowID=%s task=%s", windowID, taskName)
	defer logging.Trace("ensureWaitingWindow: end")

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

	// Verify task name matches (accounting for truncation to 12 chars)
	expectedName := taskName
	if len(expectedName) > 12 {
		expectedName = expectedName[:12]
	}
	if extractedName != expectedName {
		// Wrong task window, don't rename (prevents race condition)
		return nil
	}

	// Already in final state, don't change
	if isWaitingWindow(windowName) || isFinalWindow(windowName) {
		return nil
	}

	newName := waitingWindowName(taskName)
	logging.Trace("ensureWaitingWindow: renaming window from=%s to=%s", windowName, newName)
	return tm.RenameWindow(windowID, newName)
}

func waitingWindowName(taskName string) string {
	return constants.EmojiWaiting + constants.TruncateForWindowName(taskName)
}

func detectWaitInContent(content string) (bool, string) {
	lines := strings.Split(content, "\n")
	lines = trimTrailingEmpty(lines)
	if len(lines) == 0 {
		return false, ""
	}

	index, reason := findWaitMarker(lines)
	if index != -1 {
		linesAfter := len(lines) - index - 1
		maxDistance := waitMarkerMaxDistance
		if reason == "AskUserQuestion" {
			maxDistance = waitAskUserMaxDistance
		}
		if linesAfter <= maxDistance {
			return true, reason
		}
	}

	if index := findAskUserQuestionUIIndex(lines); index != -1 {
		linesAfter := len(lines) - index - 1
		if linesAfter <= waitAskUserMaxDistance {
			return true, "AskUserQuestionUI"
		}
	}

	if hasInputPrompt(lines) {
		return true, "prompt"
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

func findAskUserQuestionUIIndex(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		for _, marker := range askUserQuestionUIMarkers {
			if strings.Contains(trimmed, marker) {
				return i
			}
		}
	}
	return -1
}

func notifyWaiting(notifications *config.NotificationsConfig, taskName, reason string) {
	logging.Trace("notifyWaiting: start task=%s reason=%s", taskName, reason)
	defer logging.Trace("notifyWaiting: end")

	title := taskName
	message := "Waiting for your response"
	logging.Trace("notifyWaiting: sending notifications title=%s", title)
	// Send to all configured channels (macOS, Slack, ntfy)
	notify.SendAll(notifications, title, message)
	// Play sound to alert user
	logging.Trace("notifyWaiting: playing SoundNeedInput")
	notify.PlaySound(notify.SoundNeedInput)
}

func notifyWaitingWithDisplay(tm tmux.Client, notifications *config.NotificationsConfig, taskName, reason string) {
	logging.Trace("notifyWaitingWithDisplay: start task=%s reason=%s", taskName, reason)
	defer logging.Trace("notifyWaitingWithDisplay: end")

	notifyWaiting(notifications, taskName, reason)
	// Show message in tmux status bar
	displayMsg := fmt.Sprintf("💬 %s needs input", taskName)
	if reason != "" && reason != "window" && reason != "marker" {
		displayMsg = fmt.Sprintf("💬 %s: %s", taskName, reason)
	}
	logging.Trace("notifyWaitingWithDisplay: displaying message=%s", displayMsg)
	if err := tm.DisplayMessage(displayMsg, 3000); err != nil {
		logging.Trace("Failed to display message: %v", err)
	}
}

type askPrompt struct {
	Question string
	Options  []string
}

func (p askPrompt) key() string {
	if p.Question == "" || len(p.Options) == 0 {
		return ""
	}
	return p.Question + "\n" + strings.Join(p.Options, "\n")
}

func parseAskUserQuestion(content string) (askPrompt, bool) {
	lines := strings.Split(content, "\n")
	index := findAskUserQuestionIndex(lines)
	if index == -1 {
		return askPrompt{}, false
	}

	var prompt askPrompt
	foundQuestion := false
	for _, line := range lines[index+1:] {
		if value, ok := parseAskField(line, "question"); ok {
			if foundQuestion {
				break
			}
			prompt.Question = value
			foundQuestion = true
			continue
		}
		if !foundQuestion {
			continue
		}
		if value, ok := parseAskField(line, "label"); ok {
			if value != "" {
				prompt.Options = append(prompt.Options, value)
			}
		}
	}

	if prompt.Question == "" || len(prompt.Options) == 0 {
		return askPrompt{}, false
	}
	return prompt, true
}

func findAskUserQuestionIndex(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "AskUserQuestion") {
			return i
		}
	}
	return -1
}

func parseAskField(line, field string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	prefixes := []string{field + ":", "- " + field + ":"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			value = strings.Trim(value, "\"'")
			return value, value != ""
		}
	}
	return "", false
}

// parseAskUserQuestionUI parses the rendered AskUserQuestion UI format.
// The rendered format looks like:
//
//	□ 과일 선택                          <- header
//	사과와 오렌지 중 어느 것을 선택하시겠습니까?  <- question
//	> 1. 🍎 사과                         <- selected option
//	      빨간색의 달콤하고 아삭한 과일      <- description
//	  2. 🍊 오렌지                        <- option
//	  3. Type something.                  <- custom input (skip)
func parseAskUserQuestionUI(content string) (askPrompt, bool) {
	lines := strings.Split(content, "\n")
	lines = trimTrailingEmpty(lines)
	if len(lines) == 0 {
		return askPrompt{}, false
	}

	var prompt askPrompt
	firstOptionIndex := -1

	// Scan ALL lines looking for numbered options ("> 1. Option" or "  2. Option")
	for i := 0; i < len(lines); i++ {
		if option, ok := parseUIOption(lines[i]); ok {
			// Skip "Type something." or similar custom input options
			if strings.Contains(strings.ToLower(option), "type something") ||
				strings.Contains(strings.ToLower(option), "other") {
				continue
			}
			prompt.Options = append(prompt.Options, option)
			if firstOptionIndex == -1 {
				firstOptionIndex = i
			}
		}
	}

	// Need at least 2 options to be a valid prompt
	if len(prompt.Options) < 2 || firstOptionIndex < 1 {
		return askPrompt{}, false
	}

	// The question is typically 1-2 lines above the first option
	for i := firstOptionIndex - 1; i >= 0 && i >= firstOptionIndex-5; i-- {
		line := strings.TrimSpace(lines[i])
		// Skip empty lines, header lines (starting with □ or similar), and UI hints
		if line == "" || isUIHeaderLine(line) || isUIHintLine(line) {
			continue
		}
		// Take the first non-header, non-hint line as the question
		prompt.Question = line
		break
	}

	if prompt.Question == "" || len(prompt.Options) == 0 {
		return askPrompt{}, false
	}

	logging.Trace("parseAskUserQuestionUI: parsed question=%q options=%v", prompt.Question, prompt.Options)
	return prompt, true
}

// parseUIOption extracts option text from a numbered option line.
// Matches formats like "> 1. Option text" or "  2. Option text"
func parseUIOption(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	// Remove selection indicator (> or similar)
	trimmed = strings.TrimPrefix(trimmed, ">")
	trimmed = strings.TrimPrefix(trimmed, "❯")
	trimmed = strings.TrimSpace(trimmed)

	// Check if it starts with a number followed by dot
	if len(trimmed) < 3 {
		return "", false
	}

	// Find the number and dot pattern (e.g., "1.", "2.", etc.)
	dotIndex := strings.Index(trimmed, ".")
	if dotIndex < 1 || dotIndex > 2 {
		return "", false
	}

	// Check if characters before dot are digits
	for i := 0; i < dotIndex; i++ {
		if trimmed[i] < '0' || trimmed[i] > '9' {
			return "", false
		}
	}

	// Extract the option text after "N. "
	option := strings.TrimSpace(trimmed[dotIndex+1:])
	if option == "" {
		return "", false
	}

	return option, true
}

// isUIHeaderLine checks if a line is a UI header (checkbox, title decoration, etc.)
func isUIHeaderLine(line string) bool {
	// Common header prefixes/patterns
	headerPrefixes := []string{"□", "■", "☐", "☑", "◯", "◉", "○", "●"}
	for _, prefix := range headerPrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// isUIHintLine checks if a line is a UI hint/instruction
func isUIHintLine(line string) bool {
	hints := []string{
		"Enter to select",
		"Tab/Arrow keys",
		"Esc to cancel",
		"Space to toggle",
		"navigate",
	}
	lower := strings.ToLower(line)
	for _, hint := range hints {
		if strings.Contains(lower, strings.ToLower(hint)) {
			return true
		}
	}
	return false
}

func sendAgentResponse(tm tmux.Client, paneID, response string) error {
	if err := tm.SendKeysLiteral(paneID, response); err != nil {
		return err
	}
	if err := tm.SendKeys(paneID, "Escape"); err != nil {
		return err
	}
	return tm.SendKeys(paneID, "Enter")
}

// tryNotificationAction attempts to show a notification with action buttons
// for simple prompts (2-5 options). Returns the selected option or empty string
// if notification was not shown or user didn't select an action.
func tryNotificationAction(taskName string, prompt askPrompt) string {
	logging.Trace("tryNotificationAction: start task=%s question=%q options=%v",
		taskName, prompt.Question, prompt.Options)
	defer logging.Trace("tryNotificationAction: end")

	// Only use notification for simple prompts with 2-5 options
	if len(prompt.Options) < 2 || len(prompt.Options) > notifyMaxActions {
		logging.Trace("tryNotificationAction: skipped, option count=%d not in range [2,%d]",
			len(prompt.Options), notifyMaxActions)
		return ""
	}

	// Show notification with actions (no icon attachment - use only app icon on left side)
	title := taskName
	message := prompt.Question

	logging.Debug("tryNotificationAction: showing notification with %d actions", len(prompt.Options))
	index, err := notify.SendWithActions(title, message, "", prompt.Options, notifyTimeoutSec)
	if err != nil {
		logging.Trace("tryNotificationAction: notification failed err=%v", err)
		return ""
	}

	if index >= 0 && index < len(prompt.Options) {
		logging.Debug("tryNotificationAction: user selected action %d: %s", index, prompt.Options[index])
		return prompt.Options[index]
	}

	logging.Trace("tryNotificationAction: no action selected (index=%d)", index)
	return ""
}
