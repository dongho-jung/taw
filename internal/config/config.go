// Package config handles PAW configuration parsing and management.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dongho-jung/paw/internal/constants"
)

// WorkMode defines how tasks work with git.
type WorkMode string

const (
	WorkModeWorktree WorkMode = "worktree" // Each task gets its own worktree
	WorkModeMain     WorkMode = "main"     // All tasks work on current branch
)

// OnComplete defines what happens when a task completes.
type OnComplete string

const (
	OnCompleteConfirm   OnComplete = "confirm"    // Commit only (no push/PR/merge)
	OnCompleteAutoMerge OnComplete = "auto-merge" // Auto commit + merge + cleanup
	OnCompleteAutoPR    OnComplete = "auto-pr"    // Auto commit + create PR
)

// SlackConfig holds Slack notification settings.
type SlackConfig struct {
	Webhook string `yaml:"webhook"` // Slack incoming webhook URL
}

// NtfyConfig holds ntfy.sh notification settings.
type NtfyConfig struct {
	Topic  string `yaml:"topic"`  // ntfy topic name
	Server string `yaml:"server"` // ntfy server URL (default: https://ntfy.sh)
}

// NotificationsConfig holds all notification channel settings.
type NotificationsConfig struct {
	Slack *SlackConfig `yaml:"slack"`
	Ntfy  *NtfyConfig  `yaml:"ntfy"`
}

// Config represents the PAW project configuration.
type Config struct {
	WorkMode      WorkMode             `yaml:"work_mode"`
	OnComplete    OnComplete           `yaml:"on_complete"`
	WorktreeHook  string               `yaml:"worktree_hook"`
	Notifications *NotificationsConfig `yaml:"notifications"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		WorkMode:   WorkModeWorktree,
		OnComplete: OnCompleteConfirm,
	}
}

// Load reads the configuration from the given paw directory.
func Load(pawDir string) (*Config, error) {
	configPath := filepath.Join(pawDir, constants.ConfigFileName)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	return parseConfig(string(data))
}

// parseConfig parses the configuration from a string.
// Supports multi-line values using YAML-like '|' syntax.
// Supports nested configuration blocks for notifications.
func parseConfig(content string) (*Config, error) {
	cfg := DefaultConfig()
	lines := strings.Split(content, "\n")

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			i++
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Check for nested block (notifications:)
		if key == "notifications" && value == "" {
			cfg.Notifications = parseNotificationsBlock(lines, &i)
			continue
		}

		// Check for multi-line value (starts with '|')
		if value == "|" {
			// Read subsequent indented lines
			var multiLines []string
			i++
			for i < len(lines) {
				nextLine := lines[i]
				// Empty line within multi-line block
				if strings.TrimSpace(nextLine) == "" {
					multiLines = append(multiLines, "")
					i++
					continue
				}
				// Check if line is indented (part of multi-line block)
				if len(nextLine) > 0 && (nextLine[0] == ' ' || nextLine[0] == '\t') {
					// Remove the leading indentation (first 2 spaces or 1 tab)
					trimmedLine := nextLine
					if strings.HasPrefix(trimmedLine, "  ") {
						trimmedLine = trimmedLine[2:]
					} else if strings.HasPrefix(trimmedLine, "\t") {
						trimmedLine = trimmedLine[1:]
					}
					multiLines = append(multiLines, trimmedLine)
					i++
				} else {
					// Non-indented line, end of multi-line block
					break
				}
			}
			value = strings.Join(multiLines, "\n")
			// Trim trailing newlines from multi-line value
			value = strings.TrimRight(value, "\n")
		} else {
			i++
		}

		switch key {
		case "work_mode":
			cfg.WorkMode = WorkMode(value)
		case "on_complete":
			cfg.OnComplete = OnComplete(value)
		case "worktree_hook":
			cfg.WorktreeHook = value
		}
	}

	return cfg, nil
}

// parseNotificationsBlock parses the notifications configuration block.
func parseNotificationsBlock(lines []string, i *int) *NotificationsConfig {
	*i++ // Move past "notifications:" line
	notifications := &NotificationsConfig{}

	for *i < len(lines) {
		line := lines[*i]

		// Check if we're still in the notifications block (indented)
		if len(line) == 0 {
			*i++
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			// Non-indented line, end of notifications block
			break
		}

		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			*i++
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			*i++
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "slack":
			if value == "" {
				notifications.Slack = parseSlackBlock(lines, i)
			}
		case "ntfy":
			if value == "" {
				notifications.Ntfy = parseNtfyBlock(lines, i)
			}
		default:
			*i++
		}
	}

	return notifications
}

// parseSlackBlock parses the slack configuration block.
func parseSlackBlock(lines []string, i *int) *SlackConfig {
	*i++ // Move past "slack:" line
	slack := &SlackConfig{}
	baseIndent := getIndentLevel(lines, *i-1)

	for *i < len(lines) {
		line := lines[*i]

		// Check if we're still in the slack block (more indented than parent)
		if len(line) == 0 {
			*i++
			continue
		}

		currentIndent := countLeadingSpaces(line)
		if currentIndent <= baseIndent && strings.TrimSpace(line) != "" {
			// Less or equal indent, end of slack block
			break
		}

		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			*i++
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			*i++
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "webhook":
			slack.Webhook = value
		}

		*i++
	}

	return slack
}

// parseNtfyBlock parses the ntfy configuration block.
func parseNtfyBlock(lines []string, i *int) *NtfyConfig {
	*i++ // Move past "ntfy:" line
	ntfy := &NtfyConfig{}
	baseIndent := getIndentLevel(lines, *i-1)

	for *i < len(lines) {
		line := lines[*i]

		// Check if we're still in the ntfy block (more indented than parent)
		if len(line) == 0 {
			*i++
			continue
		}

		currentIndent := countLeadingSpaces(line)
		if currentIndent <= baseIndent && strings.TrimSpace(line) != "" {
			// Less or equal indent, end of ntfy block
			break
		}

		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			*i++
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			*i++
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "topic":
			ntfy.Topic = value
		case "server":
			ntfy.Server = value
		}

		*i++
	}

	return ntfy
}

// getIndentLevel returns the indentation level of a line at the given index.
func getIndentLevel(lines []string, index int) int {
	if index < 0 || index >= len(lines) {
		return 0
	}
	return countLeadingSpaces(lines[index])
}

// countLeadingSpaces counts the number of leading spaces/tabs in a string.
// Tabs are counted as 2 spaces.
func countLeadingSpaces(s string) int {
	count := 0
	for _, ch := range s {
		switch ch {
		case ' ':
			count++
		case '\t':
			count += 2
		default:
			return count
		}
	}
	return count
}

// Save writes the configuration to the given paw directory.
func (c *Config) Save(pawDir string) error {
	configPath := filepath.Join(pawDir, constants.ConfigFileName)

	content := fmt.Sprintf(`# PAW Configuration
# Generated by paw setup

# Work mode: worktree or main
# - worktree: Each task gets its own git worktree (recommended)
# - main: All tasks work on the current branch
work_mode: %s

# When task completes: confirm, auto-merge, or auto-pr
# - confirm: Commit only (no push/PR/merge)
# - auto-merge: Auto commit + push + merge + cleanup + close window
# - auto-pr: Auto commit + push + create pull request
on_complete: %s

# Hook to run after worktree creation (optional)
# Single line example: worktree_hook: npm install
# Multi-line example:
#   worktree_hook: |
#     npm install
#     npm run build
`, c.WorkMode, c.OnComplete)

	// Add worktree_hook if set
	if c.WorktreeHook != "" {
		content += formatWorktreeHook(c.WorktreeHook)
	}

	return os.WriteFile(configPath, []byte(content), 0644)
}

// formatWorktreeHook formats the worktree hook for saving.
// Multi-line values use YAML-like '|' syntax.
func formatWorktreeHook(hook string) string {
	if strings.Contains(hook, "\n") {
		// Multi-line: use | syntax with indentation
		lines := strings.Split(hook, "\n")
		var sb strings.Builder
		sb.WriteString("worktree_hook: |\n")
		for _, line := range lines {
			sb.WriteString("  ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		return sb.String()
	}
	// Single line
	return fmt.Sprintf("worktree_hook: %s\n", hook)
}

// Exists checks if a configuration file exists in the given paw directory.
func Exists(pawDir string) bool {
	configPath := filepath.Join(pawDir, constants.ConfigFileName)
	_, err := os.Stat(configPath)
	return err == nil
}

// ValidWorkModes returns all valid work modes.
func ValidWorkModes() []WorkMode {
	return []WorkMode{WorkModeWorktree, WorkModeMain}
}

// ValidOnCompletes returns all valid on_complete values.
func ValidOnCompletes() []OnComplete {
	return []OnComplete{
		OnCompleteConfirm,
		OnCompleteAutoMerge,
		OnCompleteAutoPR,
	}
}
