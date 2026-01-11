// Package constants defines shared constants used throughout the PAW application.
package constants

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"
	"unicode"
)

// Window status emojis
const (
	EmojiWorking = "🤖"
	EmojiWaiting = "💬"
	EmojiDone    = "✅"
	EmojiWarning = "⚠️"
	EmojiNew     = "⭐️"
)

// TaskEmojis contains all emojis used for task windows.
// Note: EmojiWarning is kept for backward compatibility with old window names
// but is no longer used for new windows. Warning state now maps to Waiting.
var TaskEmojis = []string{
	EmojiWorking,
	EmojiWaiting,
	EmojiDone,
	EmojiWarning, // Legacy: kept for detecting old windows, new windows use EmojiWaiting
}

// IsTaskWindow returns true if the window name has a task emoji prefix.
func IsTaskWindow(windowName string) bool {
	for _, emoji := range TaskEmojis {
		if strings.HasPrefix(windowName, emoji) {
			return true
		}
	}
	return false
}

// ExtractTaskName extracts the window token from a window name by removing the emoji prefix.
// Returns the token and true if a task emoji was found, or empty string and false otherwise.
func ExtractTaskName(windowName string) (string, bool) {
	for _, emoji := range TaskEmojis {
		if strings.HasPrefix(windowName, emoji) {
			return strings.TrimPrefix(windowName, emoji), true
		}
	}
	return "", false
}

const (
	WindowTokenSep = "~"
	WindowIDLen    = 4
)

// ToCamelCase converts kebab-case or snake_case to camelCase.
// Examples: "cancel-task-twice" → "cancelTaskTwice", "my_task_name" → "myTaskName"
func ToCamelCase(name string) string {
	if name == "" {
		return ""
	}

	var result strings.Builder
	result.Grow(len(name))

	capitalizeNext := false
	firstWritten := false
	for _, r := range name {
		if r == '-' || r == '_' {
			// Only capitalize next if we've already written something
			if firstWritten {
				capitalizeNext = true
			}
			continue
		}
		if capitalizeNext {
			result.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
		} else {
			result.WriteRune(r)
		}
		firstWritten = true
	}

	return result.String()
}

// TruncateForWindowName returns a stable window token for a task name.
// The token includes a short ID suffix to avoid collisions.
// The name is converted to camelCase for display.
func TruncateForWindowName(name string) string {
	return WindowToken(name)
}

// TruncateWithWidth returns a truncated display name for a given width.
// Uses camelCase for display and adds ellipsis if truncated.
// Does NOT include ID suffix - use for display-only purposes.
func TruncateWithWidth(name string, maxLen int) string {
	if maxLen < 1 {
		return ""
	}
	camel := ToCamelCase(name)
	if len(camel) <= maxLen {
		return camel
	}
	if maxLen <= 1 {
		return "…"
	}
	return camel[:maxLen-1] + "…"
}

// LegacyTruncateForWindowName truncates a task name without an ID suffix.
// This preserves backward compatibility with older window names.
func LegacyTruncateForWindowName(name string) string {
	if len(name) > MaxWindowNameLen {
		return name[:MaxWindowNameLen]
	}
	return name
}

// WindowToken builds a window-safe token for a task name.
// The name is converted to camelCase for display and truncated if needed.
func WindowToken(name string) string {
	// Convert to camelCase for display
	base := ToCamelCase(name)
	if len(base) > MaxWindowNameLen {
		base = base[:MaxWindowNameLen]
	}
	return base
}

// ShortTaskID returns a stable short ID for a task name.
func ShortTaskID(name string) string {
	sum := sha1.Sum([]byte(name))
	return hex.EncodeToString(sum[:])[:WindowIDLen]
}

// MatchesWindowToken returns true if the extracted window token matches the task name.
// Supports: new format (camelCase), legacy format (plain truncation), and old hash format (camelCase~xxxx).
func MatchesWindowToken(extracted, taskName string) bool {
	return extracted == TruncateForWindowName(taskName) ||
		extracted == LegacyTruncateForWindowName(taskName) ||
		extracted == oldHashWindowToken(taskName)
}

// oldHashWindowToken builds the old window token format with hash suffix.
// Used for backward compatibility with existing windows.
func oldHashWindowToken(name string) string {
	id := ShortTaskID(name)
	suffix := WindowTokenSep + id
	maxBase := MaxWindowNameLen - len(suffix)
	if maxBase < 1 {
		maxBase = 1
	}
	base := ToCamelCase(name)
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + suffix
}

// Display limits
const (
	MaxDisplayNameLen = 32
	MaxTaskNameLen    = 32
	MinTaskNameLen    = 8
	MaxWindowNameLen  = 20 // Max task name length in tmux window names (increased for better readability)
)

// Claude interaction timeouts
const (
	ClaudeReadyMaxAttempts  = 60
	ClaudeReadyPollInterval = 500 * time.Millisecond
	ClaudeNameGenTimeout1   = 1 * time.Minute // haiku
	ClaudeNameGenTimeout2   = 2 * time.Minute // sonnet
	ClaudeNameGenTimeout3   = 3 * time.Minute // opus
	ClaudeNameGenTimeout4   = 4 * time.Minute // opus with thinking
)

// Git/Worktree timeouts
const (
	WorktreeTimeout       = 30 * time.Second
	WindowCreationTimeout = 30 * time.Second
)

// Tmux command timeout
const (
	TmuxCommandTimeout = 10 * time.Second
)

// Default configuration values
const (
	DefaultMainBranch = "main"
	DefaultWorkMode   = "worktree"
	DefaultOnComplete = "confirm"
)

// Directory and file names
const (
	PawDirName             = ".paw"
	AgentsDirName          = "agents"
	HistoryDirName         = "history"
	WindowMapFileName      = "window-map.json"
	ConfigFileName         = "config"
	LogFileName            = "log"
	MemoryFileName         = "memory"
	PromptFileName         = "PROMPT.md"
	TaskFileName           = "task"
	TabLockDirName         = ".tab-lock"
	WindowIDFileName       = "window_id"
	PRFileName             = ".pr"
	GitRepoMarker          = ".is-git-repo"
	GlobalPromptLink       = ".global-prompt"
	ClaudeLink             = ".claude"
	BinSymlinkName         = "bin"                // Symlink to current paw binary (updated on attach)
	VersionFileName        = ".version"           // Stores PAW version for upgrade detection
	HistorySelectionFile   = ".history-selection" // Temp file for Ctrl+R history selection
)

// Tmux related constants
const (
	TmuxSocketPrefix = "paw-"
	NewWindowName    = EmojiNew + "main"
)

// Pane capture settings
const (
	PaneCaptureLines = 10000 // Number of lines to capture from pane history
	SummaryMaxLen    = 8000  // Max characters to send for summary generation
)

// Merge lock settings
const (
	MergeLockMaxRetries    = 900             // Maximum retries to acquire merge lock (15 minutes)
	MergeLockRetryInterval = 1 * time.Second // Interval between lock retries
)

// Commit message templates
const (
	CommitMessageAutoCommit      = "chore: auto-commit on task end\n\n%s"
	CommitMessageAutoCommitMerge = "chore: auto-commit before merge\n\n%s"
	CommitMessageAutoCommitPush  = "chore: auto-commit before push"
)

// commitTypeMapping defines the mapping from task name patterns to commit types.
type commitTypeMapping struct {
	prefix     string
	commitType string
}

// commitTypePrefixes defines all recognized commit type prefixes.
// Order matters: more specific prefixes should come first.
var commitTypePrefixes = []commitTypeMapping{
	{"fix-", "fix"},
	{"fix/", "fix"},
	{"bugfix-", "fix"},
	{"bugfix/", "fix"},
	{"hotfix-", "fix"},
	{"hotfix/", "fix"},
	{"feat-", "feat"},
	{"feat/", "feat"},
	{"feature-", "feat"},
	{"feature/", "feat"},
	{"add-", "feat"},
	{"add/", "feat"},
	{"refactor-", "refactor"},
	{"refactor/", "refactor"},
	{"docs-", "docs"},
	{"docs/", "docs"},
	{"doc-", "docs"},
	{"doc/", "docs"},
	{"test-", "test"},
	{"test/", "test"},
	{"tests-", "test"},
	{"tests/", "test"},
	{"chore-", "chore"},
	{"chore/", "chore"},
	{"perf-", "perf"},
	{"perf/", "perf"},
	{"style-", "style"},
	{"style/", "style"},
	{"ci-", "ci"},
	{"ci/", "ci"},
	{"build-", "build"},
	{"build/", "build"},
}

// InferCommitType determines the conventional commit type from a task name.
// It looks for common prefixes/keywords to classify the change.
func InferCommitType(taskName string) string {
	lower := strings.ToLower(taskName)

	// Check prefixes first (most specific)
	for _, p := range commitTypePrefixes {
		if strings.HasPrefix(lower, p.prefix) {
			return p.commitType
		}
	}

	// Check for keywords anywhere in the name
	// Order matters: more specific keywords should come first
	keywords := []struct {
		keyword    string
		commitType string
	}{
		// Fix-related keywords first (commonly used)
		{"fix", "fix"},
		{"bug", "fix"},
		{"repair", "fix"},
		{"patch", "fix"},
		{"resolve", "fix"},
		// Refactor keywords
		{"refactor", "refactor"},
		{"cleanup", "refactor"},
		{"clean-up", "refactor"},
		{"restructure", "refactor"},
		{"reorganize", "refactor"},
		{"improve", "refactor"},
		// Docs keywords (before general feat keywords)
		{"doc", "docs"},
		{"readme", "docs"},
		{"comment", "docs"},
		// Test keywords
		{"test", "test"},
		{"spec", "test"},
		// Performance keywords
		{"perf", "perf"},
		{"optim", "perf"},
		{"speed", "perf"},
		{"fast", "perf"},
		// Feature keywords last (most general)
		{"update", "feat"},
		{"add", "feat"},
		{"implement", "feat"},
		{"create", "feat"},
		{"new", "feat"},
		{"introduce", "feat"},
		{"enable", "feat"},
		{"support", "feat"},
	}

	for _, kw := range keywords {
		if strings.Contains(lower, kw.keyword) {
			return kw.commitType
		}
	}

	// Default to feat for general features
	return "feat"
}

// FormatTaskNameForCommit converts a task name into a readable commit subject.
// It removes common prefixes and converts to readable format.
func FormatTaskNameForCommit(taskName string) string {
	lower := strings.ToLower(taskName)

	// Remove common prefixes using the shared mapping
	result := taskName
	for _, p := range commitTypePrefixes {
		if strings.HasPrefix(lower, p.prefix) {
			result = taskName[len(p.prefix):]
			break
		}
	}

	// Replace hyphens with spaces for readability
	result = strings.ReplaceAll(result, "-", " ")
	result = strings.ReplaceAll(result, "_", " ")

	// Trim and ensure first letter is lowercase (conventional commit style)
	result = strings.TrimSpace(result)
	if len(result) > 0 {
		result = strings.ToLower(result[:1]) + result[1:]
	}

	return result
}

// Double-press detection
const (
	DoublePressIntervalSec = 2 // Seconds to wait for second keypress
)

// Task window handling
const (
	WindowIDWaitMaxAttempts = 60                     // Max attempts to wait for window ID file
	WindowIDWaitInterval    = 500 * time.Millisecond // Interval between checks
)

// Popup sizes (percentage-based for compatibility)
const (
	// Full size for content viewers (log, git, diff, template)
	PopupWidthFull  = "94%"
	PopupHeightFull = "90%"

	// Compact size for help viewer
	PopupWidthHelp  = "80"
	PopupHeightHelp = "80%"

	// Small size for command palette (fits 3 items + input + help)
	PopupWidthPalette  = "60"
	PopupHeightPalette = "18"

	// Medium size for settings popup (fits 5 fields + tabs + help)
	PopupWidthSettings  = "60"
	PopupHeightSettings = "22"

	// Medium size for history picker
	PopupWidthHistory  = "80%"
	PopupHeightHistory = "70%"
)
