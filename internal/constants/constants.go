// Package constants defines shared constants used throughout the PAW application.
package constants

import (
	"strings"
	"sync"
	"time"
	"unicode"
)

// camelCaseCache caches ToCamelCase results to avoid repeated conversions.
// Task names are converted frequently in hot paths like kanban rendering.
var camelCaseCache sync.Map

// Window status emojis
const (
	EmojiWorking = "🤖"
	EmojiWaiting = "💬"
	EmojiReview  = "👀"
	EmojiWarning = "⚠️"
	EmojiDone    = "✅"
	EmojiNew     = "⭐️"
)

// TaskEmojis contains all emojis used for task windows.
var TaskEmojis = []string{
	EmojiWorking,
	EmojiWaiting,
	EmojiReview,
	EmojiWarning,
	EmojiDone,
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

// ToCamelCase converts kebab-case or snake_case to camelCase.
// Examples: "cancel-task-twice" → "cancelTaskTwice", "my_task_name" → "myTaskName"
// Results are cached since task names are converted repeatedly in hot paths.
func ToCamelCase(name string) string {
	if name == "" {
		return ""
	}

	// Check cache first
	if cached, ok := camelCaseCache.Load(name); ok {
		return cached.(string)
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

	converted := result.String()
	camelCaseCache.Store(name, converted)
	return converted
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

// MatchesWindowToken returns true if the extracted window token matches the task name.
func MatchesWindowToken(extracted, taskName string) bool {
	return extracted == TruncateForWindowName(taskName)
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

// PR watch interval
const (
	PRWatchInterval = 1 * time.Minute
)

// Hook execution timeout
const (
	DefaultHookTimeout = 5 * time.Minute // Default timeout for hooks
)

// Tmux command timeout
const (
	TmuxCommandTimeout  = 10 * time.Second
	PaneWaitTimeout     = 5 * time.Second  // Timeout for waiting for pane to be ready
	ScrollToSpinnerWait = 30 * time.Second // Timeout for scrolling to first Claude spinner
)

// Default configuration values
const (
	DefaultMainBranch = "main"
	DefaultWorkMode   = "worktree"
)

// End-task action names
const (
	ActionDone       = "done"
	ActionDrop       = "drop"
	ActionKeep       = "keep"
	ActionMerge      = "merge"
	ActionMergePush  = "merge-push"
	ActionPR         = "pr"
	ActionCreateMain = "create-main" // Create main branch and merge
)

// Log format constants
const (
	LogFormatText  = "text"
	LogFormatJSONL = "jsonl"
)

// Global PAW directories (relative to $HOME)
const (
	GlobalConfigDir     = ".config/paw"       // Global config directory ($HOME/.config/paw)
	GlobalDataDir       = ".local/share/paw"  // Base directory for global PAW data
	GlobalWorkspacesDir = "workspaces"        // Subdirectory for project workspaces
)

// Directory and file names
const (
	PawDirName            = ".paw"
	AgentsDirName         = "agents"
	HistoryDirName        = "history"
	WindowMapFileName     = "window-map.json"
	ConfigFileName        = "config"
	LogFileName           = "log"
	PromptFileName        = "PROMPT.md"
	TaskFileName          = "task"
	TaskContextFileName   = ".task-context"
	TabLockDirName        = ".tab-lock"
	WindowIDFileName      = "window_id"
	PRFileName            = ".pr"
	GitRepoMarker         = ".is-git-repo"
	GlobalPromptLink      = ".global-prompt"
	ClaudeLink            = ".claude"
	BinSymlinkName        = "bin"                  // Symlink to current paw binary (updated on attach)
	VersionFileName       = ".version"             // Stores PAW version for upgrade detection
	HistorySelectionFile  = ".history-selection"   // Temp file for Ctrl+R history selection
	TemplateSelectionFile = ".template-selection"  // Temp file for Ctrl+T template selection
	TemplateDraftFile     = ".template-draft"      // Temp file for Ctrl+T template creation
	StatusSignalFileName  = ".status-signal"       // Temp file for Claude to signal status directly
	ProjectSwitchFileName = ".project-switch"      // Temp file for project picker to signal switch target
	ProjectPathFileName   = ".project-path"        // Stores project path for global workspaces
	TaskNameSelectionFile = ".task-name-selection" // Temp file for Alt+Enter task name input

	// Task agent directory file names
	OriginLinkName          = "origin"           // Symlink to project root
	WorktreeDirName         = "worktree"         // Git worktree directory
	StatusFileName          = ".status"          // Task status file (working/waiting/done)
	SessionStartedFile      = ".session-started" // Session marker file
	AgentSystemPromptFile   = ".system-prompt"   // Agent's system prompt file (in agent dir)
	AgentUserPromptFile     = ".user-prompt"     // Agent's user prompt file (in agent dir)
	VerifyLogFile           = ".verify.log"      // Verify log file
	VerifyJSONFile          = ".verify.json"     // Verify JSON result file
	StartAgentScriptName    = "start-agent"      // Agent start script
)

// Prompts directory and file names
const (
	PromptsDirName          = "prompts"           // Directory for custom prompts
	TaskNamePromptFile      = "task-name.md"      // Task name generation rules
	MergeConflictPromptFile = "merge-conflict.md" // Merge conflict resolution prompt
	PRDescriptionPromptFile = "pr-description.md" // PR title/body template
	CommitMessagePromptFile = "commit-message.md" // Commit message template
	SystemPromptFile        = "system.md"         // Custom system prompt (overrides embedded)
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

// Task dependency settings
const (
	DependencyPollInterval = 5 * time.Second // Interval for checking dependency status
)

// Commit message templates
const (
	CommitMessageAutoCommit      = "chore: auto-commit on task end\n\n%s"
	CommitMessageAutoCommitMerge = "chore: auto-commit before merge\n\n%s"
	CommitMessageAutoCommitPush  = "chore: auto-commit before push"
)

// Stash message for merge operations
const (
	MergeStashMessage = "paw-merge-temp"
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

// Popup sizes (percentage-based for consistent terminal sizing)
const (
	// Full-screen size for popups.
	PopupWidthFull  = "100%"
	PopupHeightFull = "100%"

	// Full-screen size for PR popup (shown after PR creation).
	PopupWidthPR  = PopupWidthFull
	PopupHeightPR = PopupHeightFull

	// Compact size for the finish picker popup.
	PopupWidthFinish  = "80%"
	PopupHeightFinish = "15"

	// Compact size for the project picker popup.
	PopupWidthProject  = "80%"
	PopupHeightProject = "20"

	// Compact size for the task name input popup.
	PopupWidthTaskName  = "60%"
	PopupHeightTaskName = "10"
)

// Pane sizes for split panes
const (
	// TopPaneSize is the default height for top/bottom split panes (40% of window).
	TopPaneSize = "40%"
)

// Git commit message limits (conventional commit style)
const (
	CommitSubjectMaxLen       = 72 // Max length for commit subject line
	CommitSubjectTruncatedLen = 69 // Length when truncated (leaves room for "...")
)

// Display message durations (milliseconds) for tmux status bar messages.
const (
	DisplayMsgQuick     = 1500 // Quick feedback messages
	DisplayMsgStandard  = 2000 // Standard informational messages
	DisplayMsgImportant = 3000 // Important messages (errors, warnings, dependency status)
	DisplayMsgCritical  = 4000 // Critical messages requiring user attention
)

// Conflict resolution settings.
const (
	ConflictResolutionTimeout = 10 * time.Minute // Timeout for merge conflict resolution
)
