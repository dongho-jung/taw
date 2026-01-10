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
func stopHookTrace(format string, args ...interface{}) {
	f, err := os.OpenFile(stopHookTraceFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// Setup logging if PAW_DIR is available
		if pawDir := os.Getenv("PAW_DIR"); pawDir != "" {
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
		// This allows re-classification when new work is requested after PAW_DONE
		if isFinal && hasDoneMarker(paneContent) {
			logging.Debug("stopHookCmd: window already final (%s) with valid done marker, skipping", windowName)
			stopHookTrace("Skipping: window already final (%s) with valid done marker", windowName)
			return nil
		}

		paneContent = tailString(paneContent, constants.SummaryMaxLen)

		// Check for explicit PAW_DONE marker first (fast path)
		var status task.Status
		if hasDoneMarker(paneContent) {
			logging.Debug("stopHookCmd: PAW_DONE marker detected")
			status = task.StatusDone
			stopHookTrace("PAW_DONE marker detected for task=%s", taskName)
		} else if hasAskUserQuestionInLastSegment(paneContent) {
			// Skip classification if AskUserQuestion is in the last segment
			// The watch-wait watcher will handle WAITING status detection
			logging.Debug("stopHookCmd: AskUserQuestion detected in last segment, skipping classification")
			status = task.StatusWorking
			stopHookTrace("AskUserQuestion detected for task=%s (skipping classification, watch-wait will handle)", taskName)
		} else {
			// Fallback to Claude classification with progressive model escalation
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

		if err := renameWindowCmd.RunE(renameWindowCmd, []string{windowID, newName}); err != nil {
			logging.Warn("stopHookCmd: failed to rename window: %v", err)
			stopHookTrace("Rename FAILED task=%s error=%v", taskName, err)
			return nil
		}

		stopHookTrace("STATUS UPDATE SUCCESS task=%s status=%s newName=%s", taskName, status, newName)
		logging.Debug("stopHookCmd: status updated to %s", status)
		return nil
	},
}

func classifyStopStatus(taskName, paneContent string) (task.Status, error) {
	// NOTE: WAITING is intentionally excluded from stop-hook classification.
	// The watch-wait watcher handles WAITING detection using specific markers
	// (PAW_WAITING, AskUserQuestion tool, UI patterns) which are more reliable
	// than text-based classification that can produce false positives from
	// conversational phrases like "please test" or "please check".
	prompt := fmt.Sprintf(`You are classifying the current state of a coding task.

Return exactly one label: WORKING, DONE, or WARNING.

Definitions:
- WORKING: actively processing, making tool calls, executing commands, in progress, or asking user questions.
- DONE: work completed successfully and ready for review/merge.
- WARNING: errors, failing tests, merge conflicts, or incomplete/failed work.

If unsure, return WORKING.

Task: %s
Terminal output (most recent):
%s
`, taskName, paneContent)

	// Progressive model escalation: haiku -> sonnet -> opus -> opus with thinking
	// This handles cases where the agent forgot to output markers in long responses
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
			lastErr = fmt.Errorf("unrecognized stop-hook output: %q", output)
			logging.Debug("classifyStopStatus: attempt %d parse failed: %v", i+1, lastErr)
			stopHookTrace("Classification attempt %d parse failed: %v", i+1, lastErr)
			continue
		}

		stopHookTrace("Classification SUCCESS at attempt %d: status=%s", i+1, status)
		logging.Debug("classifyStopStatus: success at attempt %d, status=%s", i+1, status)
		return status, nil
	}

	return "", lastErr
}

func runClaudePromptWithModel(prompt, model string, thinking bool, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"-p", "--model", model}
	if thinking {
		args = append(args, "--think")
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
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

	// NOTE: WAITING is mapped to WORKING because the watch-wait watcher
	// handles WAITING detection more accurately using specific markers.
	// This prevents false positives from text-based classification.
	switch {
	case strings.HasPrefix(upper, "WORKING"):
		return task.StatusWorking, true
	case strings.HasPrefix(upper, "WAITING"):
		return task.StatusWorking, true // Map to WORKING - let watch-wait handle it
	case strings.HasPrefix(upper, "DONE"):
		return task.StatusDone, true
	case strings.HasPrefix(upper, "WARNING"), strings.HasPrefix(upper, "WARN"):
		return task.StatusCorrupted, true
	case strings.Contains(upper, "WORKING"):
		return task.StatusWorking, true
	case strings.Contains(upper, "WAITING"):
		return task.StatusWorking, true // Map to WORKING - let watch-wait handle it
	case strings.Contains(upper, "DONE"):
		return task.StatusDone, true
	case strings.Contains(upper, "WARNING"):
		return task.StatusCorrupted, true
	default:
		return "", false
	}
}

func windowNameForStatus(taskName string, status task.Status) string {
	emoji := constants.EmojiWorking
	switch status {
	case task.StatusWaiting:
		emoji = constants.EmojiWaiting
	case task.StatusDone:
		emoji = constants.EmojiDone
	case task.StatusCorrupted:
		emoji = constants.EmojiWarning
	}

	return emoji + constants.TruncateForWindowName(taskName)
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
	segmentStart := findLastSegmentStartStopHook(lines)

	// Check if any line in the last segment contains "AskUserQuestion"
	for _, line := range lines[segmentStart:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "AskUserQuestion") {
			return true
		}
	}
	return false
}

// hasDoneMarker checks if the pane content contains the PAW_DONE marker.
// The marker must appear on its own line (possibly with whitespace)
// AND in the last segment (after the last ⏺ marker, which indicates a new Claude response).
// This prevents a previously completed task from staying "done" when given new work.
func hasDoneMarker(content string) bool {
	lines := strings.Split(content, "\n")
	// Trim trailing empty lines to ensure we check the actual content,
	// not empty lines from terminal scroll regions (consistent with detectDoneInContent in wait.go)
	lines = trimTrailingEmptyLines(lines)
	if len(lines) == 0 {
		return false
	}

	// Find the last segment (after the last ⏺ marker)
	// This ensures we only detect PAW_DONE in the most recent agent response
	segmentStart := findLastSegmentStartStopHook(lines)

	// Check the last N lines from the segment for the marker (agent may output text after PAW_DONE)
	// Uses doneMarkerMaxDistance from wait.go for consistency
	start := len(lines) - doneMarkerMaxDistance
	if start < segmentStart {
		start = segmentStart
	}
	for _, line := range lines[start:] {
		if matchesDoneMarkerStopHook(line) {
			return true
		}
	}
	return false
}

// findLastSegmentStartStopHook finds the index of the last line starting with ⏺.
// Returns 0 if no segment marker is found (search entire content).
func findLastSegmentStartStopHook(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "⏺") {
			return i
		}
	}
	return 0
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
