package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

const doneMarker = "PAW_DONE"
const stopHookTraceFile = "/tmp/paw-stop-hook-trace.log"

// modelAttempt defines a model escalation attempt configuration for classification.
type modelAttempt struct {
	model    string
	thinking bool
	timeout  time.Duration
}

// stopHookTrace writes a debug trace to help diagnose stop hook issues.
// This is written to a separate file for debugging since stop hook runs
// outside the normal logging context and may fail before logging is initialized.
func stopHookTrace(format string, args ...any) {
	f, err := os.OpenFile(stopHookTraceFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // G302: trace file needs to be readable by users for debugging
	if err != nil {
		return
	}
	defer func() {
		_ = f.Close()
	}()
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	_, err = fmt.Fprintf(f, "[%s] %s\n", timestamp, msg)
	if err != nil {
		return
	}
}

var stopHookCmd = &cobra.Command{
	Use:   "stop-hook",
	Short: "Handle Claude stop hook for task status updates",
	RunE: func(_ *cobra.Command, _ []string) error {
		// Trace hook invocation with environment variables for debugging
		stopHookTrace("stop-hook called: PAW_STOP_HOOK=%q SESSION_NAME=%q WINDOW_ID=%q TASK_NAME=%q PAW_BIN=%q",
			os.Getenv("PAW_STOP_HOOK"),
			os.Getenv("SESSION_NAME"),
			os.Getenv("WINDOW_ID"),
			os.Getenv("TASK_NAME"),
			os.Getenv("PAW_BIN"),
		)

		// Prevent recursive hook execution (when calling claude for classification)
		if os.Getenv("PAW_STOP_HOOK") != "" {
			stopHookTrace("Skipping: PAW_STOP_HOOK is set (recursive call)")
			return nil
		}

		sessionName := os.Getenv("SESSION_NAME")
		windowID := os.Getenv("WINDOW_ID")
		taskName := os.Getenv("TASK_NAME")

		// Validate required environment variables
		if err := validateRequiredParams(map[string]string{
			"SESSION_NAME": sessionName,
			"WINDOW_ID":    windowID,
			"TASK_NAME":    taskName,
		}); err != nil {
			stopHookTrace("Skipping: %v", err)
			return nil
		}

		pawDir := os.Getenv("PAW_DIR")

		// Setup logging if PAW_DIR is available
		if pawDir != "" {
			_, cleanup := setupLogger(filepath.Join(pawDir, constants.LogFileName), os.Getenv("PAW_DEBUG") == "1", "stop-hook", taskName)
			defer cleanup()
		}

		logging.Debug("-> stopHookCmd(session=%s, windowID=%s, task=%s)", sessionName, windowID, taskName)
		defer logging.Debug("<- stopHookCmd")

		tm := tmux.New(sessionName)
		paneID := windowID + ".0"
		if !tm.HasPane(paneID) {
			logging.Debug("stopHookCmd: pane %s not found, skipping", paneID)
			stopHookTrace("Skipping: pane %s not found", paneID)
			return nil
		}

		windowName, err := getWindowName(tm, windowID)
		isFinal := err == nil && isFinalWindow(windowName)

		paneContent, err := tm.CapturePane(paneID, constants.PaneCaptureLines)
		if err != nil {
			logging.Warn("stopHookCmd: failed to capture pane: %v", err)
			stopHookTrace("Error: failed to capture pane: %v", err)
			return nil
		}

		paneContent = strings.TrimSpace(paneContent)
		if paneContent == "" {
			logging.Warn("stopHookCmd: empty pane capture, skipping")
			stopHookTrace("Skipping: empty pane capture")
			return nil
		}

		// If window is already final and done marker is still valid in last segment, skip
		// UNLESS there's a waiting marker (which indicates new work started)
		// This allows re-classification when new work is requested after PAW_DONE
		if isFinal && hasDoneMarker(paneContent) && !hasWaitingMarker(paneContent) && !hasAskUserQuestionInLastSegment(paneContent) {
			logging.Debug("stopHookCmd: window already final (%s) with valid done marker, skipping", windowName)
			stopHookTrace("Skipping: window already final (%s) with valid done marker", windowName)
			return nil
		}

		paneContent = tailString(paneContent, constants.SummaryMaxLen)

		// Check status signal file first (highest priority - Claude writes directly)
		// This is more reliable than terminal parsing since it's a direct file write.
		if pawDir != "" {
			signalPath := filepath.Join(pawDir, constants.AgentsDirName, taskName, constants.StatusSignalFileName)
			if signalStatus := readAndClearStatusSignal(signalPath); signalStatus != "" {
				logging.Info("stopHookCmd: task=%s detected=status_signal status=%s", taskName, signalStatus)
				stopHookTrace("Status signal file detected: %s", signalStatus)

				newName := windowNameForStatus(taskName, signalStatus)
				stopHookTrace("Renaming window from status signal task=%s windowID=%s to status=%s", taskName, windowID, signalStatus)

				if err := renameWindowWithStatus(tm, windowID, newName, pawDir, taskName, "stop-hook", signalStatus); err != nil {
					logging.Warn("stopHookCmd: failed to rename window: %v", err)
					stopHookTrace("Rename FAILED task=%s error=%v", taskName, err)
					return nil
				}

				logging.Info("stopHookCmd: task=%s status_changed_to=%s window=%s (via signal file)", taskName, signalStatus, newName)
				stopHookTrace("STATUS UPDATE SUCCESS (signal) task=%s status=%s", taskName, signalStatus)
				return nil
			}
		}

		// Check for explicit markers in terminal (fast path)
		// IMPORTANT: Waiting markers take priority over done markers because they indicate
		// user action is needed NOW. This handles the case where:
		// - Task outputs PAW_DONE (Done state)
		// - User asks new question
		// - Agent outputs PAW_WAITING
		// - Old PAW_DONE is still in terminal, but PAW_WAITING should win
		var status task.Status
		if hasWaitingMarker(paneContent) {
			// Detect PAW_WAITING marker directly in stop hook
			// This is more reliable than watch-wait's distance-limited detection
			logging.Info("stopHookCmd: task=%s detected=PAW_WAITING_marker", taskName)
			status = task.StatusWaiting
			stopHookTrace("PAW_WAITING marker detected for task=%s", taskName)
		} else if hasAskUserQuestionInLastSegment(paneContent) {
			// AskUserQuestion without PAW_WAITING marker
			// Set to WAITING directly since watch-wait may not detect the UI
			logging.Info("stopHookCmd: task=%s detected=AskUserQuestion", taskName)
			status = task.StatusWaiting
			stopHookTrace("AskUserQuestion detected for task=%s", taskName)
		} else if hasDoneMarker(paneContent) {
			logging.Info("stopHookCmd: task=%s detected=PAW_DONE_marker", taskName)
			status = task.StatusDone
			stopHookTrace("PAW_DONE marker detected for task=%s", taskName)
		} else {
			// Fallback to Claude classification with progressive model escalation
			logging.Info("stopHookCmd: task=%s no_marker_found, starting_classification", taskName)
			stopHookTrace("Calling Claude for classification task=%s content_len=%d (will try haiku→sonnet→opus→opus+thinking)", taskName, len(paneContent))

			var err error
			status, err = classifyStopStatus(taskName, paneContent)
			if err != nil {
				logging.Warn("stopHookCmd: classification failed: %v", err)
				status = task.StatusWorking
				stopHookTrace("Classification FAILED task=%s error=%v (defaulting to WORKING)", taskName, err)
			} else {
				stopHookTrace("Classification SUCCESS task=%s status=%s", taskName, status)
			}
		}

		newName := windowNameForStatus(taskName, status)
		stopHookTrace("Renaming window task=%s windowID=%s to status=%s newName=%s", taskName, windowID, status, newName)

		if err := renameWindowWithStatus(tm, windowID, newName, pawDir, taskName, "stop-hook", status); err != nil {
			logging.Warn("stopHookCmd: failed to rename window: %v", err)
			stopHookTrace("Rename FAILED task=%s error=%v", taskName, err)
			return nil
		}

		// L2 logging for all status changes
		logging.Info("stopHookCmd: task=%s status_changed_to=%s window=%s", taskName, status, newName)
		stopHookTrace("STATUS UPDATE SUCCESS task=%s status=%s newName=%s", taskName, status, newName)
		return nil
	},
}

func classifyStopStatus(taskName, paneContent string) (task.Status, error) {
	// NOTE: WAITING is intentionally excluded from stop-hook classification.
	// The watch-wait watcher handles WAITING detection using specific markers
	// (PAW_WAITING, AskUserQuestion tool, UI patterns) which are more reliable
	// than text-based classification that can produce false positives from
	// conversational phrases like "please test" or "please check".

	// Fast path: Check for obvious idle prompt patterns before calling Claude
	if isIdlePromptPattern(paneContent) {
		logging.Info("classifyStopStatus: idle prompt pattern detected, status=DONE")
		stopHookTrace("Idle prompt pattern detected - skipping Claude classification")
		return task.StatusDone, nil
	}

	prompt := fmt.Sprintf(`You are classifying the current state of a Claude Code CLI session.

Return exactly one label: WORKING or DONE

Definitions:
- WORKING: Claude is actively processing - making tool calls, executing commands, writing code, running builds/tests, or in the middle of a response.
- DONE: Claude has finished responding and is idle. The terminal shows an empty input prompt (❯) waiting for the next user input.

Key indicators of DONE state:
- Empty input prompt line (❯ or >) at the end
- Status line showing "bypass permissions" or permission settings
- "Brewed for Xs" or "Cogitated for Xs" followed by prompt (thinking completed)
- Horizontal separator lines (─────) before empty prompt
- Response text followed by waiting for next input

Key indicators of WORKING state:
- Active tool calls (⏺ Read, ⏺ Edit, ⏺ Bash, etc.)
- Build/test output in progress
- Partial response being streamed
- Error messages being handled

IMPORTANT: Do NOT return "unsure" or any other value. You MUST choose either WORKING or DONE.
If the terminal shows an idle prompt waiting for input, return DONE.

Task: %s
Terminal output (most recent):
%s
`, taskName, paneContent)

	// Progressive model escalation: haiku -> sonnet -> opus -> opus with thinking
	// This handles cases where the agent forgot to output markers in long responses
	// If a model returns unparseable output, escalate to the next model instead of giving up
	attempts := []modelAttempt{
		{model: "haiku", thinking: false, timeout: constants.ClaudeNameGenTimeout1},
		{model: "sonnet", thinking: false, timeout: constants.ClaudeNameGenTimeout2},
		{model: "opus", thinking: false, timeout: constants.ClaudeNameGenTimeout3},
		{model: "opus", thinking: true, timeout: constants.ClaudeNameGenTimeout4},
	}

	var lastErr error
	for i, attempt := range attempts {
		modelDesc := attempt.model
		if attempt.thinking {
			modelDesc = attempt.model + " (thinking)"
		}
		stopHookTrace("Classification attempt %d/%d: model=%s timeout=%v", i+1, len(attempts), modelDesc, attempt.timeout)
		logging.Debug("classifyStopStatus: attempt %d/%d with model=%s, timeout=%v", i+1, len(attempts), modelDesc, attempt.timeout)

		output, err := runClaudePromptWithModel(prompt, attempt.model, attempt.thinking, attempt.timeout)
		if err != nil {
			logging.Debug("classifyStopStatus: attempt %d failed: %v", i+1, err)
			stopHookTrace("Classification attempt %d failed: %v", i+1, err)
			lastErr = err
			continue
		}

		status, ok := parseStopHookDecision(output)
		if !ok {
			// Instead of giving up, escalate to next model for unparseable output
			lastErr = fmt.Errorf("unrecognized stop-hook output: %q (escalating to next model)", output)
			logging.Info("classifyStopStatus: attempt %d unparseable output=%q, escalating", i+1, output)
			stopHookTrace("Classification attempt %d unparseable: %q (escalating)", i+1, output)
			continue
		}

		logging.Info("classifyStopStatus: SUCCESS model=%s status=%s", modelDesc, status)
		stopHookTrace("Classification SUCCESS at attempt %d: status=%s", i+1, status)
		return status, nil
	}

	return "", lastErr
}

// isIdlePromptPattern checks for obvious terminal patterns indicating Claude is idle.
// This provides a fast path before expensive Claude classification.
func isIdlePromptPattern(content string) bool {
	lines := strings.Split(content, "\n")
	lines = trimTrailingEmptyLines(lines)
	if len(lines) < 2 {
		return false
	}

	// Check the last few lines for idle prompt patterns
	lastLines := lines
	if len(lastLines) > 10 {
		lastLines = lastLines[len(lastLines)-10:]
	}

	hasEmptyPrompt := false
	hasStatusLine := false
	hasThinkingComplete := false

	for _, line := range lastLines {
		trimmed := strings.TrimSpace(line)

		// Empty prompt line (❯ or > alone, or with just whitespace)
		if trimmed == "❯" || trimmed == ">" {
			hasEmptyPrompt = true
		}

		// Status line patterns
		if strings.Contains(trimmed, "bypass permissions") ||
			strings.Contains(trimmed, "shift+tab to cycle") ||
			strings.HasPrefix(trimmed, "⏵⏵") {
			hasStatusLine = true
		}

		// Thinking completed patterns
		if strings.HasPrefix(trimmed, "✻ Brewed for") ||
			strings.HasPrefix(trimmed, "✻ Cogitated for") {
			hasThinkingComplete = true
		}
	}

	// Pattern: thinking completed + empty prompt indicates done
	if hasThinkingComplete && hasEmptyPrompt {
		return true
	}

	// Pattern: empty prompt + status line indicates done
	if hasEmptyPrompt && hasStatusLine {
		return true
	}

	return false
}

func runClaudePromptWithModel(prompt, model string, thinking bool, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"-p", "--model", model}
	if thinking {
		args = append(args, "--think")
	}

	cmd := exec.CommandContext(ctx, "claude", args...) //nolint:gosec // G204: args are controlled internally
	cmd.Env = append(os.Environ(), "PAW_STOP_HOOK=1")
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			modelDesc := model
			if thinking {
				modelDesc = model + " (thinking)"
			}
			return "", fmt.Errorf("claude %s timeout after %s", modelDesc, timeout)
		}
		return "", fmt.Errorf("claude failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

func parseStopHookDecision(output string) (task.Status, bool) {
	cleaned := strings.TrimSpace(output)
	cleaned = strings.Trim(cleaned, "`\"' \t\r\n")
	upper := strings.ToUpper(cleaned)

	switch {
	case strings.HasPrefix(upper, "WORKING"):
		return task.StatusWorking, true
	case strings.HasPrefix(upper, "DONE"):
		return task.StatusDone, true
	case strings.Contains(upper, "WORKING"):
		return task.StatusWorking, true
	case strings.Contains(upper, "DONE"):
		return task.StatusDone, true
	default:
		return "", false
	}
}

func windowNameForStatus(taskName string, status task.Status) string {
	emoji := constants.EmojiWorking
	switch status { //nolint:exhaustive // StatusPending/StatusWorking use default (Working emoji)
	case task.StatusWaiting, task.StatusCorrupted:
		// Corrupted status displays as Waiting.
		emoji = constants.EmojiWaiting
	case task.StatusDone:
		emoji = constants.EmojiDone
	}

	return emoji + constants.TruncateForWindowName(taskName)
}

// readAndClearStatusSignal reads the status signal file and deletes it.
// This allows Claude to directly signal its status by writing to a file,
// which is more reliable than parsing terminal output.
// Returns empty string if file doesn't exist or is invalid.
func readAndClearStatusSignal(signalPath string) task.Status {
	data, err := os.ReadFile(signalPath) //nolint:gosec // G304: signalPath is from task agent directory
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Debug("readAndClearStatusSignal: failed to read %s: %v", signalPath, err)
		}
		return ""
	}

	// Delete the signal file immediately to prevent reuse
	if err := os.Remove(signalPath); err != nil {
		logging.Debug("readAndClearStatusSignal: failed to delete %s: %v", signalPath, err)
	}

	statusStr := strings.TrimSpace(string(data))
	status := task.Status(statusStr)

	// Validate the status (only accept valid runtime states)
	switch status { //nolint:exhaustive // StatusPending/StatusCorrupted are not valid signal values
	case task.StatusWorking, task.StatusWaiting, task.StatusDone:
		logging.Debug("readAndClearStatusSignal: valid status=%s", status)
		return status
	default:
		logging.Debug("readAndClearStatusSignal: invalid status=%q", statusStr)
		return ""
	}
}

func tailString(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[len(value)-maxLen:]
}

// hasAskUserQuestionInLastSegment checks if the last segment contains AskUserQuestion.
// This is used to skip AI classification since watch-wait watcher handles WAITING status.
func hasAskUserQuestionInLastSegment(content string) bool {
	lines := strings.Split(content, "\n")
	lines = trimTrailingEmptyLines(lines)
	if len(lines) == 0 {
		return false
	}

	// Find the last segment (after the last ⏺ marker)
	segmentStart, ok := findLastSegmentStartStopHookWithFlag(lines)
	if !ok {
		return false
	}

	// Check if any line in the last segment contains "AskUserQuestion"
	for _, line := range lines[segmentStart:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "AskUserQuestion") {
			return true
		}
	}
	return false
}

// hasWaitingMarker checks if the pane content contains the PAW_WAITING marker.
// Similar to hasDoneMarker, the marker must appear in the last segment.
// This allows the stop hook to directly set WAITING status when the agent
// outputs PAW_WAITING, instead of relying on watch-wait's distance-limited detection.
func hasWaitingMarker(content string) bool {
	lines := strings.Split(content, "\n")
	lines = trimTrailingEmptyLines(lines)
	if len(lines) == 0 {
		return false
	}

	// Find the last segment (after the last ⏺ marker)
	segmentStart, ok := findLastSegmentStartStopHookWithFlag(lines)
	if !ok {
		return false
	}

	// Check the last N lines from the segment for the marker
	// Use a larger distance than doneMarker since UI may render after PAW_WAITING
	const waitingMarkerMaxDistance = 100
	start := len(lines) - waitingMarkerMaxDistance
	if start < segmentStart {
		start = segmentStart
	}
	for _, line := range lines[start:] {
		if matchesWaitingMarkerStopHook(line) {
			return true
		}
	}
	return false
}

// matchesWaitingMarkerStopHook checks if a line contains the PAW_WAITING marker.
// Allows prefix (like "⏺ " from Claude Code) but requires marker at end of line.
func matchesWaitingMarkerStopHook(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Exact match
	if trimmed == waitMarker {
		return true
	}
	// Allow prefix (e.g., "⏺ PAW_WAITING") but marker must be at end
	if strings.HasSuffix(trimmed, " "+waitMarker) {
		return true
	}
	return false
}

// hasDoneMarker checks if the pane content contains the PAW_DONE marker.
// The marker must appear on its own line (possibly with whitespace)
// AND in the last segment (after the last ⏺ marker, which indicates a new Claude response).
// This prevents a previously completed task from staying "done" when given new work.
// If no segment marker is present, this returns false.
//
// Additionally, if user input is detected after PAW_DONE (indicating a new request was
// submitted but Claude hasn't responded yet), the marker is considered stale and false is returned.
func hasDoneMarker(content string) bool {
	lines := strings.Split(content, "\n")
	// Trim trailing empty lines to ensure we check the actual content,
	// not empty lines from terminal scroll regions (consistent with detectDoneInContent in wait_detect.go).
	lines = trimTrailingEmptyLines(lines)
	if len(lines) == 0 {
		return false
	}

	// Find the last segment (after the last ⏺ marker)
	// This ensures we only detect PAW_DONE in the most recent agent response
	segmentStart, hasSegment := findLastSegmentStartStopHookWithFlag(lines)
	if !hasSegment {
		return false
	}
	// Use generous distance (agent may output text after PAW_DONE).
	maxDistance := doneMarkerMaxDistance

	start := len(lines) - maxDistance
	if start < segmentStart {
		start = segmentStart
	}

	// Find PAW_DONE marker and its line index
	doneLineIdx := -1
	for i := start; i < len(lines); i++ {
		if matchesDoneMarkerStopHook(lines[i]) {
			doneLineIdx = i
			break
		}
	}

	if doneLineIdx < 0 {
		return false // No PAW_DONE found
	}

	// Check if there's user input after PAW_DONE
	// This happens when user sends a new request but Claude hasn't responded yet (no new ⏺ segment)
	// In this case, PAW_DONE is from a previous session and should be considered stale
	if hasUserInputAfterIndex(lines, doneLineIdx) {
		return false
	}

	return true
}

// hasUserInputAfterIndex checks if there appears to be user input after the given line index.
// User input is detected by looking for:
// - Text on the same line as ">" prompt (e.g., "> user input here")
// - Non-empty text after a ">" prompt line (before any new ⏺ segment)
// Returns true only if user input is found AND no new ⏺ segment follows it.
// This ensures we don't flag stale markers when Claude has already processed the input.
func hasUserInputAfterIndex(lines []string, startIdx int) bool {
	sawPrompt := false
	foundUserInput := false

	for i := startIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])

		if trimmed == "" {
			continue
		}

		// New Claude segment (⏺) means any previous input was already processed
		if strings.HasPrefix(trimmed, "⏺") {
			// If we found user input before this, Claude has processed it (not stale)
			// If no user input was found, there's nothing stale
			return false
		}

		// Claude's prompt line (just ">")
		if trimmed == ">" {
			sawPrompt = true
			continue
		}

		// User input on same line as prompt ("> text")
		if strings.HasPrefix(trimmed, "> ") {
			foundUserInput = true
			continue
		}

		// If we saw a prompt and now see other text, check if it's user input
		// (skip UI decorations like box-drawing characters)
		if sawPrompt {
			if isUIDecoration(trimmed) {
				continue
			}
			foundUserInput = true
			sawPrompt = false
			continue
		}
	}

	// Return true if we found user input and no ⏺ came after it
	return foundUserInput
}

// isUIDecoration checks if a line is a UI decoration (box-drawing, etc.)
// that should not be considered as user input.
func isUIDecoration(line string) bool {
	if len(line) == 0 {
		return false
	}
	// Check for common UI decoration patterns (box-drawing characters)
	for _, r := range line {
		switch r {
		case '╭', '╰', '│', '─', '├', '┤', '┬', '┴', '┼', '╮', '╯', '┌', '┐', '└', '┘':
			return true
		default:
			return false
		}
	}
	return false
}

// findLastSegmentStartStopHookWithFlag finds the index of the last line starting with ⏺.
// Returns (index, true) if found, or (-1, false) if no segment marker is found.
func findLastSegmentStartStopHookWithFlag(lines []string) (int, bool) {
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "⏺") {
			return i, true
		}
	}
	return -1, false
}

// matchesDoneMarkerStopHook checks if a line contains the PAW_DONE marker.
// Allows prefix (like "⏺ " from Claude Code) but requires marker at end of line.
func matchesDoneMarkerStopHook(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Exact match
	if trimmed == doneMarker {
		return true
	}
	// Allow prefix (e.g., "⏺ PAW_DONE") but marker must be at end
	if strings.HasSuffix(trimmed, " "+doneMarker) {
		return true
	}
	return false
}

// trimTrailingEmptyLines removes empty/whitespace-only lines from the end of the slice.
func trimTrailingEmptyLines(lines []string) []string {
	end := len(lines)
	for end > 0 {
		if strings.TrimSpace(lines[end-1]) != "" {
			break
		}
		end--
	}
	return lines[:end]
}
