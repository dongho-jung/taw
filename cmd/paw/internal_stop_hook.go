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

var stopHookCmd = &cobra.Command{
	Use:   "stop-hook",
	Short: "Handle Claude stop hook for task status updates",
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("PAW_STOP_HOOK") != "" {
			return nil
		}

		sessionName := os.Getenv("SESSION_NAME")
		windowID := os.Getenv("WINDOW_ID")
		taskName := os.Getenv("TASK_NAME")
		if sessionName == "" || windowID == "" || taskName == "" {
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
			return nil
		}

		windowName, err := getWindowName(tm, windowID)
		if err == nil && isFinalWindow(windowName) {
			logging.Debug("stopHookCmd: window already final (%s), skipping", windowName)
			return nil
		}

		paneContent, err := tm.CapturePane(paneID, constants.PaneCaptureLines)
		if err != nil {
			logging.Warn("stopHookCmd: failed to capture pane: %v", err)
			return nil
		}

		paneContent = strings.TrimSpace(paneContent)
		if paneContent == "" {
			logging.Warn("stopHookCmd: empty pane capture, skipping")
			return nil
		}

		paneContent = tailString(paneContent, constants.SummaryMaxLen)

		// Check for explicit PAW_DONE marker first (fast path)
		var status task.Status
		if hasDoneMarker(paneContent) {
			logging.Debug("stopHookCmd: PAW_DONE marker detected")
			status = task.StatusDone
		} else {
			// Fallback to Claude classification
			var err error
			status, err = classifyStopStatus(taskName, paneContent)
			if err != nil {
				logging.Warn("stopHookCmd: classification failed: %v", err)
				status = task.StatusWaiting
			}
		}

		newName := windowNameForStatus(taskName, status)
		if err := renameWindowCmd.RunE(renameWindowCmd, []string{windowID, newName}); err != nil {
			logging.Warn("stopHookCmd: failed to rename window: %v", err)
			return nil
		}

		logging.Debug("stopHookCmd: status updated to %s", status)
		return nil
	},
}

func classifyStopStatus(taskName, paneContent string) (task.Status, error) {
	prompt := fmt.Sprintf(`You are classifying the final state of a coding task.

Return exactly one label: WAITING, DONE, or WARNING.

Definitions:
- WAITING: user input required, question asked, blocked on decision or info.
- DONE: work completed successfully and ready for review/merge.
- WARNING: errors, failing tests, merge conflicts, or incomplete/failed work.

If unsure, return WAITING.

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
	case strings.HasPrefix(upper, "WAITING"):
		return task.StatusWaiting, true
	case strings.HasPrefix(upper, "DONE"):
		return task.StatusDone, true
	case strings.HasPrefix(upper, "WARNING"), strings.HasPrefix(upper, "WARN"):
		return task.StatusCorrupted, true
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
	// Check the last N lines for the marker (it should be near the end)
	maxLines := 20
	start := len(lines) - maxLines
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
