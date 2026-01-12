// Package config handles PAW configuration parsing and management.
package config

import (
	"crypto/sha256"
	"encoding/hex"
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
	Theme         bool `yaml:"theme"`
	LogFormat     bool `yaml:"log_format"`
	LogMaxSizeMB  bool `yaml:"log_max_size_mb"`
	LogMaxBackups bool `yaml:"log_max_backups"`
	SelfImprove   bool `yaml:"self_improve"`
}

// DefaultInheritConfig returns the default inherit configuration.
// By default, all settings are inherited from global config.
func DefaultInheritConfig() *InheritConfig {
	return &InheritConfig{
		WorkMode:      true,
		Theme:         true,
		LogFormat:     true,
		LogMaxSizeMB:  true,
		LogMaxBackups: true,
		SelfImprove:   true,
	}
}

// GlobalPawDir returns the global PAW directory path ($HOME/.config/paw).
func GlobalPawDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "paw")
}

// GlobalWorkspacesDir returns the global workspaces directory path ($HOME/.local/share/paw/workspaces).
// This is used when paw_in_project is false (default).
func GlobalWorkspacesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "paw", "workspaces")
}

// ProjectWorkspaceID generates a unique workspace ID for a project directory.
// Uses a hash of the absolute path to ensure uniqueness while keeping names reasonable.
func ProjectWorkspaceID(projectDir string) string {
	// Use the last path component + short hash for readability
	base := filepath.Base(projectDir)
	absPath, err := filepath.Abs(projectDir)
	if err != nil {
		absPath = projectDir
	}

	// Create a short hash of the full path for uniqueness
	h := sha256.Sum256([]byte(absPath))
	shortHash := hex.EncodeToString(h[:])[:8]

	// Clean the base name (remove special characters)
	cleanBase := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, base)

	// Limit base name length
	if len(cleanBase) > 32 {
		cleanBase = cleanBase[:32]
	}

	return cleanBase + "-" + shortHash
}

// GetWorkspaceDir returns the workspace directory for a project.
// If localPawDir exists, it takes priority (for backward compatibility).
// Otherwise, uses global workspace location based on pawInProject setting.
func GetWorkspaceDir(projectDir string, pawInProject bool) string {
	// Local .paw takes priority if it exists
	localPawDir := filepath.Join(projectDir, constants.PawDirName)
	if _, err := os.Stat(localPawDir); err == nil {
		return localPawDir
	}

	// Use global or local based on setting
	if pawInProject {
		return localPawDir
	}

	// Use global workspace
	globalDir := GlobalWorkspacesDir()
	if globalDir == "" {
		// Fallback to local if we can't determine home directory
		return localPawDir
	}

	return filepath.Join(globalDir, ProjectWorkspaceID(projectDir))
}

// LoadGlobal reads the global configuration from $HOME/.config/paw/config.
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

// Config represents the PAW project configuration.
type Config struct {
	WorkMode        WorkMode `yaml:"work_mode"`
	Theme           string   `yaml:"theme"` // Theme preset: auto, dark, dark-blue, light, light-blue, etc.
	PawInProject    bool     `yaml:"paw_in_project"` // If true, store .paw in project dir; if false, use global workspace
	PreWorktreeHook string   `yaml:"pre_worktree_hook"`
	PreTaskHook     string   `yaml:"pre_task_hook"`
	PostTaskHook    string   `yaml:"post_task_hook"`
	PreMergeHook    string   `yaml:"pre_merge_hook"`
	PostMergeHook   string   `yaml:"post_merge_hook"`
	LogFormat       string   `yaml:"log_format"`
	LogMaxSizeMB    int      `yaml:"log_max_size_mb"`
	LogMaxBackups   int      `yaml:"log_max_backups"`
	SelfImprove     bool     `yaml:"self_improve"`

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

	if c.WorkMode == "" {
		c.WorkMode = WorkModeWorktree
	}

	if !isValidWorkMode(c.WorkMode) {
		warnings = append(warnings, fmt.Sprintf("invalid work_mode %q; defaulting to %q", c.WorkMode, WorkModeWorktree))
		c.WorkMode = WorkModeWorktree
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
	if c.Inherit != nil {
		inherit := *c.Inherit
		clone.Inherit = &inherit
	}
	return &clone
}

func isValidWorkMode(mode WorkMode) bool {
	return mode == WorkModeWorktree || mode == WorkModeMain
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

	logging.Debug("config.Load: loaded WorkMode=%s", cfg.WorkMode)
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

# Workspace location: true or false (default: false)
# - true: Store .paw directory inside the project (requires .gitignore)
# - false: Store workspace in $HOME/.local/share/paw/workspaces/ (no .gitignore needed)
paw_in_project: %t

# Log format: text or jsonl
log_format: %s

# Log rotation (size in MB, backups)
log_max_size_mb: %d
log_max_backups: %d

# Self-improve: on or off (default: off)
# When enabled, the agent reflects on mistakes at task finish and
# appends learnings to CLAUDE.md, then merges to the default branch.
self_improve: %t

# Hooks (optional) (supports multi-line command with ': |')
# pre_worktree_hook: echo "pre worktree"
# pre_task_hook: echo "pre task"
# post_task_hook: echo "post task"
# pre_merge_hook: echo "pre merge"
# post_merge_hook: echo "post merge"
`, c.WorkMode, c.PawInProject, c.LogFormat, c.LogMaxSizeMB, c.LogMaxBackups, c.SelfImprove)

	// Add hooks if set
	if c.PreWorktreeHook != "" {
		content += formatHook("pre_worktree_hook", c.PreWorktreeHook)
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
