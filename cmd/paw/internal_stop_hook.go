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

const stopHookTimeout = 20 * time.Second
const doneMarker = "PAW_DONE"
const stopHookTraceFile = "/tmp/paw-stop-hook-trace.log"

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
		if sessionName == "" || windowID == "" || taskName == "" {
			stopHookTrace("Skipping: missing env vars (session=%q window=%q task=%q)", sessionName, windowID, taskName)
			return nil
		}

		if pawDir := os.Getenv("PAW_DIR"); pawDir != "" {
			logger, _ := logging.New(filepath.Join(pawDir, constants.LogFileName), os.Getenv("PAW_DEBUG") == "1")
			if logger != nil {
				defer func() { _ = logger.Close() }()
				logger.SetScript("stop-hook")
				logger.SetTask(taskName)
				logging.SetGlobal(logger)
			}
		}

		logging.Trace("stopHookCmd: start session=%s windowID=%s task=%s", sessionName, windowID, taskName)
		defer logging.Trace("stopHookCmd: end")

		tm := tmux.New(sessionName)
		paneID := windowID + ".0"
		if !tm.HasPane(paneID) {
			logging.Debug("stopHookCmd: pane %s not found, skipping", paneID)
			stopHookTrace("Skipping: pane %s not found", paneID)
			return nil
		}

		windowName, err := getWindowName(tm, windowID)
		if err == nil && isFinalWindow(windowName) {
			logging.Debug("stopHookCmd: window already final (%s), skipping", windowName)
			stopHookTrace("Skipping: window already final (%s)", windowName)
			return nil
		}

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

		paneContent = tailString(paneContent, constants.SummaryMaxLen)

		// Check for explicit PAW_DONE marker first (fast path)
		var status task.Status
		if hasDoneMarker(paneContent) {
			logging.Debug("stopHookCmd: PAW_DONE marker detected")
			status = task.StatusDone
			stopHookTrace("PAW_DONE marker detected for task=%s", taskName)
		} else {
			// Fallback to Claude classification
			stopHookTrace("Calling Claude haiku for classification task=%s content_len=%d", taskName, len(paneContent))

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
	prompt := fmt.Sprintf(`You are classifying the current state of a coding task.

Return exactly one label: WORKING, WAITING, DONE, or WARNING.

Definitions:
- WORKING: actively processing, making tool calls, executing commands, in progress.
- WAITING: user input required, question asked, blocked on decision or info.
- DONE: work completed successfully and ready for review/merge.
- WARNING: errors, failing tests, merge conflicts, or incomplete/failed work.

If unsure, return WORKING.

Task: %s
Terminal output (most recent):
%s
`, taskName, paneContent)

	output, err := runClaudePrompt(prompt)
	if err != nil {
		return "", err
	}

	status, ok := parseStopHookDecision(output)
	if !ok {
		return "", fmt.Errorf("unrecognized stop-hook output: %q", output)
	}

	return status, nil
}

func runClaudePrompt(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), stopHookTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", "haiku")
	cmd.Env = append(os.Environ(), "PAW_STOP_HOOK=1")
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude timeout after %s", stopHookTimeout)
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
	case strings.HasPrefix(upper, "WAITING"):
		return task.StatusWaiting, true
	case strings.HasPrefix(upper, "DONE"):
		return task.StatusDone, true
	case strings.HasPrefix(upper, "WARNING"), strings.HasPrefix(upper, "WARN"):
		return task.StatusCorrupted, true
	case strings.Contains(upper, "WORKING"):
		return task.StatusWorking, true
	case strings.Contains(upper, "WAITING"):
		return task.StatusWaiting, true
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

// hasDoneMarker checks if the pane content contains the PAW_DONE marker.
// The marker must appear on its own line (possibly with whitespace).
func hasDoneMarker(content string) bool {
	lines := strings.Split(content, "\n")
	// Check the last N lines for the marker (agent may output text after PAW_DONE)
	// Uses doneMarkerMaxDistance from wait.go for consistency
	start := len(lines) - doneMarkerMaxDistance
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		if strings.TrimSpace(line) == doneMarker {
			return true
		}
	}
	return false
}
