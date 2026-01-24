// Package claude provides an interface for interacting with Claude CLI.
package claude

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

// bufferPool reuses bytes.Buffer instances to reduce allocations.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// Client defines the interface for Claude CLI operations.
type Client interface {
	// GenerateTaskName generates a task name from the given content.
	GenerateTaskName(content string) (string, error)

	// GenerateSummary generates a brief summary of the task work from pane content.
	GenerateSummary(paneContent string) (string, error)

	// WaitForReady waits for Claude to be ready in a tmux pane.
	WaitForReady(tm tmux.Client, target string) error

	// SendInput sends input to Claude in a tmux pane.
	SendInput(tm tmux.Client, target, input string) error

	// SendInputWithRetry sends input with retry logic for reliability.
	SendInputWithRetry(tm tmux.Client, target, input string, maxRetries int) error

	// SendTrustResponse sends 'y' if trust prompt is detected.
	SendTrustResponse(tm tmux.Client, target string) error

	// VerifyPaneAlive checks that the pane has content (Claude is running).
	VerifyPaneAlive(tm tmux.Client, target string, timeout time.Duration) error

	// IsClaudeRunning checks if Claude is running in the specified pane.
	// Returns true if Claude is running, false if the pane shows a shell.
	IsClaudeRunning(tm tmux.Client, target string) bool

	// ScrollToFirstSpinner waits for the first ⏺ spinner line to appear and
	// clears the scrollback history. This effectively hides the Claude banner
	// and initial command from the scrollback.
	ScrollToFirstSpinner(tm tmux.Client, target string, timeout time.Duration) error
}

// claudeClient implements the Client interface.
type claudeClient struct {
	maxAttempts  int
	pollInterval time.Duration
}

// New creates a new Claude client.
func New() Client {
	return &claudeClient{
		maxAttempts:  constants.ClaudeReadyMaxAttempts,
		pollInterval: constants.ClaudeReadyPollInterval,
	}
}

// TaskNamePattern validates task name format.
var TaskNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{6,30}[a-z0-9]$`)

// ReadyPatterns matches Claude ready prompts.
// Added more patterns to improve detection reliability:
// - Trust/bypass: initial trust prompt
// - ╭─: Claude's box drawing UI
// - >: input prompt
// - claude: Claude branding in output
// - Cost: token/cost display
var ReadyPatterns = regexp.MustCompile(`(?i)(Trust|bypass permissions|╭─|^>\s*$|claude|Cost:?\s*\$)`)

// TrustPattern matches trust confirmation prompt.
var TrustPattern = regexp.MustCompile(`(?i)trust`)

// SummaryTimeout is the timeout for summary generation.
const SummaryTimeout = 15 * time.Second

// GenerateSummary generates a brief summary of task work from the pane content.
func (c *claudeClient) GenerateSummary(paneContent string) (string, error) {
	// Truncate pane content if too long (keep last 8000 chars for summary)
	maxLen := 8000
	if len(paneContent) > maxLen {
		paneContent = paneContent[len(paneContent)-maxLen:]
	}

	prompt := fmt.Sprintf(`Below is the terminal output from a development task. Please provide a brief summary of what was done (3-5 lines).

- What changes/modifications were made
- Key files or features affected
- Result (success/failure)

Terminal output:
%s

Write a concise summary only. Do NOT include any header like "Summary:" or "**Summary:**" - just the content.`, paneContent)

	logging.Trace("GenerateSummary: starting with content length=%d", len(paneContent))

	summary, err := c.runClaude(prompt, SummaryTimeout)
	if err != nil {
		logging.Debug("GenerateSummary: failed: %v", err)
		return "", err
	}

	logging.Debug("GenerateSummary: success, length=%d", len(summary))
	return summary, nil
}

// modelAttempt defines a model escalation attempt configuration.
type modelAttempt struct {
	model    string
	thinking bool
	timeout  time.Duration
}

// GenerateTaskName generates a task name using Claude CLI with progressive model escalation.
// Starts with haiku and escalates to sonnet, opus, then opus with thinking on failure.
func (c *claudeClient) GenerateTaskName(content string) (string, error) {
	prompt := fmt.Sprintf(`Create a short task name for this task (8-32 lowercase chars, hyphens only, verb-noun format like "add-login-feature"):
%s

Respond with ONLY the task name, nothing else.`, content)

	// Progressive model escalation: haiku -> sonnet -> opus -> opus with thinking
	attempts := []modelAttempt{
		{model: "haiku", thinking: false, timeout: constants.ClaudeNameGenTimeout1},
		{model: "sonnet", thinking: false, timeout: constants.ClaudeNameGenTimeout2},
		{model: "opus", thinking: false, timeout: constants.ClaudeNameGenTimeout3},
		{model: "opus", thinking: true, timeout: constants.ClaudeNameGenTimeout4},
	}

	logging.Trace("GenerateTaskName: starting with %d model attempts", len(attempts))

	var lastErr error
	for i, attempt := range attempts {
		modelDesc := attempt.model
		if attempt.thinking {
			modelDesc = attempt.model + " (thinking)"
		}
		logging.Debug("GenerateTaskName: attempt %d/%d with model=%s, timeout=%v", i+1, len(attempts), modelDesc, attempt.timeout)
		name, err := c.runClaudeWithModel(prompt, attempt.model, attempt.thinking, attempt.timeout)
		if err != nil {
			logging.Debug("GenerateTaskName: attempt %d failed: %v", i+1, err)
			lastErr = err
			continue
		}

		logging.Trace("GenerateTaskName: raw response=%q", name)

		// Validate the name
		sanitized := sanitizeTaskName(name)
		logging.Trace("GenerateTaskName: sanitized=%q", sanitized)

		if TaskNamePattern.MatchString(sanitized) {
			logging.Debug("GenerateTaskName: valid name generated: %s", sanitized)
			return sanitized, nil
		}

		lastErr = fmt.Errorf("invalid task name format: raw=%q, sanitized=%q", name, sanitized)
		logging.Debug("GenerateTaskName: invalid format: %v", lastErr)
	}

	// Return error - let caller decide fallback
	logging.Debug("GenerateTaskName: all attempts failed: %v", lastErr)
	return "", lastErr
}

func (c *claudeClient) runClaude(prompt string, timeout time.Duration) (string, error) {
	return c.runClaudeWithModel(prompt, "haiku", false, timeout)
}

func (c *claudeClient) runClaudeWithModel(prompt, model string, thinking bool, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"-p", "--model", model}
	if thinking {
		args = append(args, "--think")
	}

	modelDesc := model
	if thinking {
		modelDesc = model + " (thinking)"
	}
	logging.Trace("runClaudeWithModel: executing claude %v with timeout=%v", args, timeout)

	cmd := exec.CommandContext(ctx, "claude", args...)
	env := append(os.Environ(), "PAW_STOP_HOOK=1")
	if os.Getenv("PAW_BIN") == "" {
		if exe, err := os.Executable(); err == nil {
			env = append(env, "PAW_BIN="+exe)
		}
	}
	cmd.Env = env
	cmd.Stdin = strings.NewReader(prompt)

	// Reuse buffers from pool to reduce allocations
	stdout := bufferPool.Get().(*bytes.Buffer)
	stderr := bufferPool.Get().(*bytes.Buffer)
	stdout.Reset()
	stderr.Reset()
	defer bufferPool.Put(stdout)
	defer bufferPool.Put(stderr)

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		logging.Trace("runClaudeWithModel: command failed (model=%s): err=%v, stderr=%q", modelDesc, err, errMsg)
		return "", fmt.Errorf("claude command failed: %w: %s", err, errMsg)
	}

	result := strings.TrimSpace(stdout.String())
	logging.Trace("runClaudeWithModel: success (model=%s), output=%q", modelDesc, result)
	return result, nil
}

// sanitizeTaskName cleans up a task name to match the required format.
func sanitizeTaskName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Remove any quotes or extra whitespace
	name = strings.Trim(name, "\"'`\n\r\t ")

	// Replace spaces and underscores with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove any characters that aren't lowercase letters, numbers, or hyphens
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	name = result.String()

	// Remove leading/trailing hyphens
	name = strings.Trim(name, "-")

	// Collapse multiple hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	// Truncate if too long
	if len(name) > constants.MaxTaskNameLen {
		name = name[:constants.MaxTaskNameLen]
		// Remove trailing hyphen if we cut in the middle of a word
		name = strings.TrimSuffix(name, "-")
	}

	return name
}

// WaitForReady waits for Claude to be ready in the specified tmux pane.
// Uses exponential backoff starting from pollInterval, capped at 2 seconds.
func (c *claudeClient) WaitForReady(tm tmux.Client, target string) error {
	currentInterval := c.pollInterval
	maxInterval := 2 * time.Second
	emptyCount := 0
	maxEmptyBeforeWarn := 10

	for i := 0; i < c.maxAttempts; i++ {
		// First verify the pane exists
		if !tm.HasPane(target) {
			logging.Debug("WaitForReady: pane %s does not exist (attempt %d/%d)", target, i+1, c.maxAttempts)
			time.Sleep(currentInterval)
			continue
		}

		content, err := tm.CapturePane(target, 50)
		if err != nil {
			logging.Debug("WaitForReady: failed to capture pane (attempt %d/%d): %v", i+1, c.maxAttempts, err)
			time.Sleep(currentInterval)
			continue
		}

		trimmedContent := strings.TrimSpace(content)
		if len(trimmedContent) == 0 {
			emptyCount++
			if emptyCount == maxEmptyBeforeWarn {
				logging.Debug("WaitForReady: pane still empty after %d attempts, continuing to wait...", emptyCount)
			}
			time.Sleep(currentInterval)
			// Exponential backoff for empty panes
			if currentInterval < maxInterval {
				currentInterval = time.Duration(float64(currentInterval) * 1.5)
				if currentInterval > maxInterval {
					currentInterval = maxInterval
				}
			}
			continue
		}

		// Reset interval and empty count when we get content
		emptyCount = 0
		currentInterval = c.pollInterval

		if ReadyPatterns.MatchString(content) {
			logging.Debug("WaitForReady: Claude ready detected (attempt %d/%d)", i+1, c.maxAttempts)
			return nil
		}

		// Log what we're seeing for debugging (only first 100 chars)
		preview := trimmedContent
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		logging.Trace("WaitForReady: content preview (attempt %d/%d): %q", i+1, c.maxAttempts, preview)

		time.Sleep(currentInterval)
	}

	return fmt.Errorf("timeout waiting for Claude to be ready after %d attempts", c.maxAttempts)
}

// SendInput sends input to Claude in the specified tmux pane.
// Uses Escape followed by CR to properly submit multi-line input.
func (c *claudeClient) SendInput(tm tmux.Client, target, input string) error {
	// First send the text literally
	if err := tm.SendKeysLiteral(target, input); err != nil {
		return fmt.Errorf("failed to send input: %w", err)
	}

	// Then send Escape followed by Enter to submit
	// This is how Claude Code handles multi-line input submission
	time.Sleep(100 * time.Millisecond)

	if err := tm.SendKeys(target, "Escape"); err != nil {
		return fmt.Errorf("failed to send Escape: %w", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := tm.SendKeys(target, "Enter"); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	return nil
}

// SendTrustResponse sends 'y' if a trust prompt is detected.
func (c *claudeClient) SendTrustResponse(tm tmux.Client, target string) error {
	content, err := tm.CapturePane(target, 20)
	if err != nil {
		return fmt.Errorf("failed to capture pane: %w", err)
	}

	if TrustPattern.MatchString(content) {
		if err := tm.SendKeys(target, "y", "Enter"); err != nil {
			return fmt.Errorf("failed to send trust response: %w", err)
		}
	}

	return nil
}

// SendInputWithRetry sends input with retry logic for reliability.
// It verifies the pane is responsive before each attempt and waits for content change.
func (c *claudeClient) SendInputWithRetry(tm tmux.Client, target, input string, maxRetries int) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Verify pane exists and is responsive
		if !tm.HasPane(target) {
			lastErr = fmt.Errorf("pane %s does not exist", target)
			logging.Debug("SendInputWithRetry: pane not found (attempt %d/%d)", attempt, maxRetries)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Get content before sending
		contentBefore, err := tm.CapturePane(target, 10)
		if err != nil {
			lastErr = fmt.Errorf("failed to capture pane before send: %w", err)
			logging.Debug("SendInputWithRetry: capture failed (attempt %d/%d): %v", attempt, maxRetries, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Send input
		if err := c.SendInput(tm, target, input); err != nil {
			lastErr = err
			logging.Debug("SendInputWithRetry: send failed (attempt %d/%d): %v", attempt, maxRetries, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Wait a bit and verify content changed (input was accepted)
		time.Sleep(300 * time.Millisecond)
		contentAfter, err := tm.CapturePane(target, 10)
		if err != nil {
			// Can't verify, but send succeeded - consider it successful
			logging.Debug("SendInputWithRetry: can't verify after send, assuming success")
			return nil
		}

		// If content changed, input was likely accepted
		if contentBefore != contentAfter {
			logging.Debug("SendInputWithRetry: input accepted (attempt %d/%d)", attempt, maxRetries)
			return nil
		}

		// Content didn't change - might need retry
		logging.Debug("SendInputWithRetry: content unchanged after send (attempt %d/%d)", attempt, maxRetries)
		lastErr = fmt.Errorf("input may not have been accepted")
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		return fmt.Errorf("failed to send input after %d attempts: %w", maxRetries, lastErr)
	}
	return fmt.Errorf("failed to send input after %d attempts", maxRetries)
}

// VerifyPaneAlive checks that the pane exists and has content (Claude is running).
// This should be called after RespawnPane to ensure Claude actually started.
func (c *claudeClient) VerifyPaneAlive(tm tmux.Client, target string, timeout time.Duration) error {
	pollInterval := 200 * time.Millisecond
	maxAttempts := int(timeout / pollInterval)
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for i := 0; i < maxAttempts; i++ {
		if !tm.HasPane(target) {
			logging.Trace("VerifyPaneAlive: pane %s not found (attempt %d/%d)", target, i+1, maxAttempts)
			time.Sleep(pollInterval)
			continue
		}

		content, err := tm.CapturePane(target, 5)
		if err != nil {
			logging.Trace("VerifyPaneAlive: pane %s capture failed (attempt %d/%d): %v", target, i+1, maxAttempts, err)
			time.Sleep(pollInterval)
			continue
		}

		// Any non-empty content suggests the pane is alive
		if len(strings.TrimSpace(content)) > 0 {
			logging.Debug("VerifyPaneAlive: pane %s is alive (attempt %d/%d)", target, i+1, maxAttempts)
			return nil
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("pane %s did not become alive within %v", target, timeout)
}

// shellCommands are common shell command names that indicate Claude has exited.
var shellCommands = []string{"bash", "zsh", "sh", "fish", "tcsh", "csh", "ksh", "dash"}

// IsClaudeRunning checks if Claude is running in the specified pane.
// Returns true if Claude (or its start-agent script) is running.
// Returns false if a shell is running (meaning Claude has exited).
func (c *claudeClient) IsClaudeRunning(tm tmux.Client, target string) bool {
	if !tm.HasPane(target) {
		logging.Trace("IsClaudeRunning: pane %s does not exist", target)
		return false
	}

	cmd, err := tm.GetPaneCommand(target)
	if err != nil {
		logging.Trace("IsClaudeRunning: failed to get pane command: %v", err)
		return false
	}

	cmd = strings.ToLower(strings.TrimSpace(cmd))
	logging.Trace("IsClaudeRunning: pane %s command=%q", target, cmd)

	// Check if it's a shell command (Claude exited)
	for _, shell := range shellCommands {
		if cmd == shell {
			logging.Debug("IsClaudeRunning: pane %s shows shell %q, Claude not running", target, cmd)
			return false
		}
	}

	// Check for common shell patterns
	if strings.HasPrefix(cmd, "-") {
		// Login shell like "-zsh" or "-bash"
		shellName := strings.TrimPrefix(cmd, "-")
		for _, shell := range shellCommands {
			if shellName == shell {
				logging.Debug("IsClaudeRunning: pane %s shows login shell %q, Claude not running", target, cmd)
				return false
			}
		}
	}

	// If the command contains "claude" or "start-agent", it's running
	if strings.Contains(cmd, "claude") || strings.Contains(cmd, "start-agent") {
		logging.Debug("IsClaudeRunning: pane %s shows Claude-related command %q", target, cmd)
		return true
	}

	// For other commands (like node, python which Claude might spawn), assume Claude is running
	// This is a conservative approach - better to assume running than to restart unnecessarily
	logging.Debug("IsClaudeRunning: pane %s shows command %q, assuming Claude subprocess", target, cmd)
	return true
}

// ScrollToFirstSpinner waits for the first ⏺ spinner line to appear in the pane content,
// waits for the banner to scroll into scrollback, then clears the scrollback.
// This creates a cleaner view where the pane content starts from Claude's first output.
func (c *claudeClient) ScrollToFirstSpinner(tm tmux.Client, target string, timeout time.Duration) error {
	logging.Trace("ScrollToFirstSpinner: waiting for spinner in pane %s", target)

	pollInterval := 500 * time.Millisecond
	deadline := time.Now().Add(timeout)

	// The banner (logo + version + path) is typically 4-5 lines, plus the command prompt
	// is another 2-3 lines. Total ~8 lines before the spinner.
	// We wait until the spinner moves to line 2 or lower in the VISIBLE pane,
	// which means the banner has scrolled into the scrollback buffer.
	const maxSpinnerLineFromTop = 2

	for time.Now().Before(deadline) {
		// Capture only the visible pane content (no scrollback)
		// By passing 0 lines, we get just what's visible on screen
		content, err := tm.CapturePane(target, 0)
		if err != nil {
			logging.Trace("ScrollToFirstSpinner: capture failed: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		// Look for the first ⏺ character (spinner indicator)
		lines := strings.Split(content, "\n")
		spinnerLine := -1
		for i, line := range lines {
			if strings.Contains(line, "⏺") {
				spinnerLine = i
				break
			}
		}

		if spinnerLine >= 0 {
			logging.Trace("ScrollToFirstSpinner: spinner at visible line %d", spinnerLine)

			// When the spinner is at or near the top of the visible pane,
			// the banner has scrolled into the scrollback buffer
			if spinnerLine <= maxSpinnerLineFromTop {
				logging.Debug("ScrollToFirstSpinner: spinner at top (line %d), clearing history", spinnerLine)

				// Clear scrollback history - this removes the banner and initial command
				// from the scrollable history, so users can't scroll back to see them
				if err := tm.ClearHistory(target); err != nil {
					logging.Trace("ScrollToFirstSpinner: failed to clear history: %v", err)
				}

				return nil
			}

			// Spinner not at top yet, continue waiting for more output
			logging.Trace("ScrollToFirstSpinner: waiting for spinner to reach top (currently at line %d)",
				spinnerLine)
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for spinner in pane %s", target)
}

// BuildSystemPrompt builds the system prompt from global and project prompts.
func BuildSystemPrompt(globalPrompt, projectPrompt string) string {
	var sb strings.Builder

	if globalPrompt != "" {
		sb.WriteString(globalPrompt)
	}

	if projectPrompt != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString(projectPrompt)
	}

	return sb.String()
}

// BuildClaudeCommand builds the claude command with the given options.
func BuildClaudeCommand(systemPrompt string, dangerouslySkipPermissions bool) []string {
	args := []string{"claude"}

	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	if dangerouslySkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	return args
}
