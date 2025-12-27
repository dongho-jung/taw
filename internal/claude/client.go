// Package claude provides an interface for interacting with Claude CLI.
package claude

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/donghojung/taw/internal/constants"
	"github.com/donghojung/taw/internal/logging"
	"github.com/donghojung/taw/internal/tmux"
)

// Client defines the interface for Claude CLI operations.
type Client interface {
	// GenerateTaskName generates a task name from the given content.
	GenerateTaskName(content string) (string, error)

	// WaitForReady waits for Claude to be ready in a tmux pane.
	WaitForReady(tm tmux.Client, target string) error

	// SendInput sends input to Claude in a tmux pane.
	SendInput(tm tmux.Client, target, input string) error

	// SendTrustResponse sends 'y' if trust prompt is detected.
	SendTrustResponse(tm tmux.Client, target string) error
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
var ReadyPatterns = regexp.MustCompile(`(?i)(Trust|trust|bypass permissions|╭─|^> $)`)

// TrustPattern matches trust confirmation prompt.
var TrustPattern = regexp.MustCompile(`(?i)trust`)

// GenerateTaskName generates a task name using Claude CLI (Haiku model).
func (c *claudeClient) GenerateTaskName(content string) (string, error) {
	prompt := fmt.Sprintf(`Create a short task name for this task (8-32 lowercase chars, hyphens only, verb-noun format like "add-login-feature"):
%s

Respond with ONLY the task name, nothing else.`, content)

	// Try with increasing timeouts
	timeouts := []time.Duration{
		constants.ClaudeNameGenTimeout1,
		constants.ClaudeNameGenTimeout2,
		constants.ClaudeNameGenTimeout3,
	}

	logging.Debug("GenerateTaskName: starting with %d timeout attempts", len(timeouts))

	var lastErr error
	for i, timeout := range timeouts {
		logging.Debug("GenerateTaskName: attempt %d with timeout=%v", i+1, timeout)
		name, err := c.runClaude(prompt, timeout)
		if err != nil {
			logging.Debug("GenerateTaskName: attempt %d failed: %v", i+1, err)
			lastErr = err
			continue
		}

		logging.Debug("GenerateTaskName: raw response=%q", name)

		// Validate the name
		sanitized := sanitizeTaskName(name)
		logging.Debug("GenerateTaskName: sanitized=%q", sanitized)

		if TaskNamePattern.MatchString(sanitized) {
			logging.Debug("GenerateTaskName: valid name generated: %s", sanitized)
			return sanitized, nil
		}

		lastErr = fmt.Errorf("invalid task name format: raw=%q, sanitized=%q", name, sanitized)
		logging.Debug("GenerateTaskName: invalid format: %v", lastErr)
	}

	// Return error - let caller decide fallback
	logging.Debug("GenerateTaskName: all attempts failed, returning error: %v", lastErr)
	return "", lastErr
}

func (c *claudeClient) runClaude(prompt string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	logging.Debug("runClaude: executing claude -p --model haiku with timeout=%v", timeout)

	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", "haiku")
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		logging.Debug("runClaude: command failed: err=%v, stderr=%q", err, errMsg)
		return "", fmt.Errorf("claude command failed: %w: %s", err, errMsg)
	}

	result := strings.TrimSpace(stdout.String())
	logging.Debug("runClaude: success, output=%q", result)
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
func (c *claudeClient) WaitForReady(tm tmux.Client, target string) error {
	for i := 0; i < c.maxAttempts; i++ {
		content, err := tm.CapturePane(target, 50)
		if err != nil {
			return fmt.Errorf("failed to capture pane: %w", err)
		}

		if ReadyPatterns.MatchString(content) {
			return nil
		}

		time.Sleep(c.pollInterval)
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
