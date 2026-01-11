// Package config handles PAW configuration parsing and management.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
)

// InheritConfig defines which fields inherit from global config.
// Only used in project-level config.
type InheritConfig struct {
	WorkMode      bool `yaml:"work_mode"`
	OnComplete    bool `yaml:"on_complete"`
	Theme         bool `yaml:"theme"`
	LogFormat     bool `yaml:"log_format"`
	LogMaxSizeMB  bool `yaml:"log_max_size_mb"`
	LogMaxBackups bool `yaml:"log_max_backups"`
	Notifications bool `yaml:"notifications"`
	SelfImprove   bool `yaml:"self_improve"`
}

// DefaultInheritConfig returns the default inherit configuration.
// By default, all settings are inherited from global config.
func DefaultInheritConfig() *InheritConfig {
	return &InheritConfig{
		WorkMode:      true,
		OnComplete:    true,
		Theme:         true,
		LogFormat:     true,
		LogMaxSizeMB:  true,
		LogMaxBackups: true,
		Notifications: true,
		SelfImprove:   true,
	}
}

// GlobalPawDir returns the global PAW directory path ($HOME/.paw).
func GlobalPawDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, constants.PawDirName)
}

// LoadGlobal reads the global configuration from $HOME/.paw/config.
func LoadGlobal() (*Config, error) {
	logging.Debug("-> config.LoadGlobal()")
	defer logging.Debug("<- config.LoadGlobal")

	globalDir := GlobalPawDir()
	if globalDir == "" {
		logging.Debug("config.LoadGlobal: could not determine home directory")
		return DefaultConfig(), nil
	}

	return Load(globalDir)
}

// EnsureGlobalDir ensures the global PAW directory exists.
func EnsureGlobalDir() error {
	globalDir := GlobalPawDir()
	if globalDir == "" {
		return fmt.Errorf("could not determine home directory")
	}
	return os.MkdirAll(globalDir, 0755)
}

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
	Theme         string               `yaml:"theme"` // Theme preset: auto, dark, dark-blue, light, light-blue, etc.
	WorktreeHook  string               `yaml:"worktree_hook"`
	PreTaskHook   string               `yaml:"pre_task_hook"`
	PostTaskHook  string               `yaml:"post_task_hook"`
	PreMergeHook  string               `yaml:"pre_merge_hook"`
	PostMergeHook string               `yaml:"post_merge_hook"`
	Notifications *NotificationsConfig `yaml:"notifications"`
	LogFormat     string               `yaml:"log_format"`
	LogMaxSizeMB  int                  `yaml:"log_max_size_mb"`
	LogMaxBackups int                  `yaml:"log_max_backups"`
	SelfImprove   bool                 `yaml:"self_improve"`

	// Inherit specifies which fields inherit from global config.
	// Only used in project-level config files.
	Inherit *InheritConfig `yaml:"inherit"`
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

	if c.WorkMode == "" {
		c.WorkMode = WorkModeWorktree
	}
	if c.OnComplete == "" {
		c.OnComplete = OnCompleteConfirm
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
		WorkMode:      WorkModeWorktree,
		OnComplete:    OnCompleteConfirm,
		LogFormat:     "text",
		LogMaxSizeMB:  10,
		LogMaxBackups: 3,
	}
}

// MergeWithGlobal applies inherited values from global config.
// Fields marked for inheritance in cfg.Inherit will be copied from global.
func (c *Config) MergeWithGlobal(global *Config) {
	if c == nil || global == nil {
		return
	}
	if c.Inherit == nil {
		return
	}

	if c.Inherit.WorkMode {
		c.WorkMode = global.WorkMode
	}
	if c.Inherit.OnComplete {
		c.OnComplete = global.OnComplete
	}
	if c.Inherit.Theme {
		c.Theme = global.Theme
	}
	if c.Inherit.LogFormat {
		c.LogFormat = global.LogFormat
	}
	if c.Inherit.LogMaxSizeMB {
		c.LogMaxSizeMB = global.LogMaxSizeMB
	}
	if c.Inherit.LogMaxBackups {
		c.LogMaxBackups = global.LogMaxBackups
	}
	if c.Inherit.Notifications && global.Notifications != nil {
		c.Notifications = global.Notifications
	}
	if c.Inherit.SelfImprove {
		c.SelfImprove = global.SelfImprove
	}
}

// Clone creates a deep copy of the config.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	clone := *c
	if c.Notifications != nil {
		notif := *c.Notifications
		if c.Notifications.Slack != nil {
			slack := *c.Notifications.Slack
			notif.Slack = &slack
		}
		if c.Notifications.Ntfy != nil {
			ntfy := *c.Notifications.Ntfy
			notif.Ntfy = &ntfy
		}
		clone.Notifications = &notif
	}
	if c.Inherit != nil {
		inherit := *c.Inherit
		clone.Inherit = &inherit
	}
	return &clone
}

func isValidWorkMode(mode WorkMode) bool {
	return mode == WorkModeWorktree || mode == WorkModeMain
}

func isValidOnComplete(value OnComplete) bool {
	return value == OnCompleteConfirm || value == OnCompleteAutoMerge || value == OnCompleteAutoPR
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

# Hook to run after worktree/workspace creation (optional)
# Single line example: worktree_hook: npm install
# Multi-line example:
#   worktree_hook: |
#     npm install
#     npm run build

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

# Self-improve: on or off (default: off)
# When enabled, the agent reflects on mistakes at task finish and
# appends learnings to CLAUDE.md, then merges to the default branch.
self_improve: %t
`, c.WorkMode, c.OnComplete, c.LogFormat, c.LogMaxSizeMB, c.LogMaxBackups, c.SelfImprove)

	// Add worktree_hook if set
	if c.WorktreeHook != "" {
		content += formatHook("worktree_hook", c.WorktreeHook)
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

	// Add notifications block if set
	if c.Notifications != nil {
		content += formatNotificationsBlock(c.Notifications)
	}

	// Add inherit block if set (project config only)
	if c.Inherit != nil {
		content += formatInheritBlock(c.Inherit)
	}

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		logging.Debug("config.Save: failed to write config: %v", err)
		return err
	}
	logging.Debug("config.Save: written to %s", configPath)
	return nil
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
