// Package config handles PAW configuration parsing and management.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
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

// NonGitWorkspaceMode defines workspace behavior when git is not available.
type NonGitWorkspaceMode string

const (
	NonGitWorkspaceShared NonGitWorkspaceMode = "shared"
	NonGitWorkspaceCopy   NonGitWorkspaceMode = "copy"
)

// Theme defines the UI color theme setting.
type Theme string

const (
	ThemeAuto  Theme = "auto"  // Auto-detect based on terminal background
	ThemeLight Theme = "light" // Force light theme colors
	ThemeDark  Theme = "dark"  // Force dark theme colors
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
	WorkMode        WorkMode             `yaml:"work_mode"`
	OnComplete      OnComplete           `yaml:"on_complete"`
	Theme           Theme                `yaml:"theme"`
	WorktreeHook    string               `yaml:"worktree_hook"`
	PreTaskHook     string               `yaml:"pre_task_hook"`
	PostTaskHook    string               `yaml:"post_task_hook"`
	PreMergeHook    string               `yaml:"pre_merge_hook"`
	PostMergeHook   string               `yaml:"post_merge_hook"`
	VerifyCommand   string               `yaml:"verify_command"`
	VerifyTimeout   int                  `yaml:"verify_timeout_sec"`
	VerifyRequired  bool                 `yaml:"verify_required"`
	NonGitWorkspace string               `yaml:"non_git_workspace"`
	Notifications   *NotificationsConfig `yaml:"notifications"`
	LogFormat       string               `yaml:"log_format"`
	LogMaxSizeMB    int                  `yaml:"log_max_size_mb"`
	LogMaxBackups   int                  `yaml:"log_max_backups"`
}

// Normalize validates configuration values, applying safe defaults when needed.
// It returns warnings for any corrections that were applied.
func (c *Config) Normalize() []string {
	if c == nil {
		return nil
	}

	var warnings []string

	c.WorkMode = WorkMode(strings.TrimSpace(string(c.WorkMode)))
	c.OnComplete = OnComplete(strings.TrimSpace(string(c.OnComplete)))
	c.Theme = Theme(strings.TrimSpace(string(c.Theme)))

	if c.WorkMode == "" {
		c.WorkMode = WorkModeWorktree
	}
	if c.OnComplete == "" {
		c.OnComplete = OnCompleteConfirm
	}
	if c.Theme == "" {
		c.Theme = ThemeAuto
	}

	if !isValidWorkMode(c.WorkMode) {
		warnings = append(warnings, fmt.Sprintf("invalid work_mode %q; defaulting to %q", c.WorkMode, WorkModeWorktree))
		c.WorkMode = WorkModeWorktree
	}

	if !isValidOnComplete(c.OnComplete) {
		warnings = append(warnings, fmt.Sprintf("invalid on_complete %q; defaulting to %q", c.OnComplete, OnCompleteConfirm))
		c.OnComplete = OnCompleteConfirm
	}

	if c.WorkMode == WorkModeMain && (c.OnComplete == OnCompleteAutoMerge || c.OnComplete == OnCompleteAutoPR) {
		warnings = append(warnings, fmt.Sprintf("on_complete %q is not supported in main mode; defaulting to %q", c.OnComplete, OnCompleteConfirm))
		c.OnComplete = OnCompleteConfirm
	}

	if !isValidTheme(c.Theme) {
		warnings = append(warnings, fmt.Sprintf("invalid theme %q; defaulting to %q", c.Theme, ThemeAuto))
		c.Theme = ThemeAuto
	}

	if c.VerifyTimeout <= 0 {
		c.VerifyTimeout = 600
	}

	c.NonGitWorkspace = strings.TrimSpace(c.NonGitWorkspace)
	if c.NonGitWorkspace == "" {
		c.NonGitWorkspace = string(NonGitWorkspaceShared)
	}
	if c.NonGitWorkspace != string(NonGitWorkspaceShared) && c.NonGitWorkspace != string(NonGitWorkspaceCopy) {
		warnings = append(warnings, fmt.Sprintf("invalid non_git_workspace %q; defaulting to %q", c.NonGitWorkspace, NonGitWorkspaceShared))
		c.NonGitWorkspace = string(NonGitWorkspaceShared)
	}

	c.LogFormat = strings.TrimSpace(c.LogFormat)
	if c.LogFormat == "" {
		c.LogFormat = "text"
	}
	if c.LogFormat != "text" && c.LogFormat != "jsonl" {
		warnings = append(warnings, fmt.Sprintf("invalid log_format %q; defaulting to %q", c.LogFormat, "text"))
		c.LogFormat = "text"
	}
	if c.LogMaxSizeMB <= 0 {
		c.LogMaxSizeMB = 10
	}
	if c.LogMaxBackups < 0 {
		c.LogMaxBackups = 3
	}

	return warnings
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		WorkMode:        WorkModeWorktree,
		OnComplete:      OnCompleteConfirm,
		Theme:           ThemeAuto,
		LogFormat:       "text",
		LogMaxSizeMB:    10,
		LogMaxBackups:   3,
		VerifyTimeout:   600,
		NonGitWorkspace: string(NonGitWorkspaceShared),
	}
}

func isValidWorkMode(mode WorkMode) bool {
	return mode == WorkModeWorktree || mode == WorkModeMain
}

func isValidOnComplete(value OnComplete) bool {
	return value == OnCompleteConfirm || value == OnCompleteAutoMerge || value == OnCompleteAutoPR
}

func isValidTheme(theme Theme) bool {
	return theme == ThemeAuto || theme == ThemeLight || theme == ThemeDark
}

// Load reads the configuration from the given paw directory.
func Load(pawDir string) (*Config, error) {
	logging.Debug("-> config.Load(pawDir=%s)", pawDir)
	defer logging.Debug("<- config.Load")

	configPath := filepath.Join(pawDir, constants.ConfigFileName)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logging.Debug("config.Load: config file not found, using defaults")
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		logging.Debug("config.Load: failed to read config: %v", err)
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	cfg, err := parseConfig(string(data))
	if err != nil {
		logging.Debug("config.Load: failed to parse config: %v", err)
		return nil, err
	}

	logging.Debug("config.Load: loaded WorkMode=%s OnComplete=%s", cfg.WorkMode, cfg.OnComplete)
	return cfg, nil
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
		case "theme":
			cfg.Theme = Theme(value)
		case "worktree_hook":
			cfg.WorktreeHook = value
		case "pre_task_hook":
			cfg.PreTaskHook = value
		case "post_task_hook":
			cfg.PostTaskHook = value
		case "pre_merge_hook":
			cfg.PreMergeHook = value
		case "post_merge_hook":
			cfg.PostMergeHook = value
		case "verify_command":
			cfg.VerifyCommand = value
		case "verify_timeout_sec":
			if parsed, err := strconv.Atoi(value); err == nil {
				cfg.VerifyTimeout = parsed
			}
		case "verify_required":
			if parsed, err := strconv.ParseBool(value); err == nil {
				cfg.VerifyRequired = parsed
			}
		case "non_git_workspace":
			cfg.NonGitWorkspace = value
		case "log_format":
			cfg.LogFormat = value
		case "log_max_size_mb":
			if parsed, err := strconv.Atoi(value); err == nil {
				cfg.LogMaxSizeMB = parsed
			}
		case "log_max_backups":
			if parsed, err := strconv.Atoi(value); err == nil {
				cfg.LogMaxBackups = parsed
			}
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
	logging.Debug("-> config.Save(pawDir=%s)", pawDir)
	defer logging.Debug("<- config.Save")

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

# UI color theme: auto, light, or dark
# - auto: Auto-detect based on terminal background (default)
# - light: Force light theme colors (dark text on light background)
# - dark: Force dark theme colors (light text on dark background)
# Use explicit value if auto-detection doesn't work correctly
theme: %s

# Non-git workspace: shared or copy
non_git_workspace: %s

# Hook to run after worktree/workspace creation (optional)
# Single line example: worktree_hook: npm install
# Multi-line example:
#   worktree_hook: |
#     npm install
#     npm run build

# Verification (optional)
# verify_command: npm test
verify_timeout_sec: %d
verify_required: %t

# Hooks (optional)
# pre_task_hook: echo "pre task"
# post_task_hook: echo "post task"
# pre_merge_hook: echo "pre merge"
# post_merge_hook: echo "post merge"

# Log format: text or jsonl
log_format: %s

# Log rotation (size in MB, backups)
log_max_size_mb: %d
log_max_backups: %d
`, c.WorkMode, c.OnComplete, c.Theme, c.NonGitWorkspace, c.VerifyTimeout, c.VerifyRequired, c.LogFormat, c.LogMaxSizeMB, c.LogMaxBackups)

	// Add worktree_hook if set
	if c.WorktreeHook != "" {
		content += formatHook("worktree_hook", c.WorktreeHook)
	}
	if c.VerifyCommand != "" {
		content += formatHook("verify_command", c.VerifyCommand)
	}
	if c.PreTaskHook != "" {
		content += formatHook("pre_task_hook", c.PreTaskHook)
	}
	if c.PostTaskHook != "" {
		content += formatHook("post_task_hook", c.PostTaskHook)
	}
	if c.PreMergeHook != "" {
		content += formatHook("pre_merge_hook", c.PreMergeHook)
	}
	if c.PostMergeHook != "" {
		content += formatHook("post_merge_hook", c.PostMergeHook)
	}

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		logging.Debug("config.Save: failed to write config: %v", err)
		return err
	}
	logging.Debug("config.Save: written to %s", configPath)
	return nil
}

// formatHook formats a hook command for saving.
// Multi-line values use YAML-like '|' syntax.
func formatHook(key, hook string) string {
	if strings.Contains(hook, "\n") {
		// Multi-line: use | syntax with indentation
		lines := strings.Split(hook, "\n")
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s: |\n", key))
		for _, line := range lines {
			sb.WriteString("  ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		return sb.String()
	}
	// Single line
	return fmt.Sprintf("%s: %s\n", key, hook)
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
