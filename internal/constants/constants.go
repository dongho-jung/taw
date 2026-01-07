// Package constants defines shared constants used throughout the PAW application.
package constants

import (
	"strings"
	"time"
)

// Window status emojis
const (
	EmojiWorking = "🤖"
	EmojiWaiting = "💬"
	EmojiDone    = "✅"
	EmojiWarning = "⚠️"
	EmojiNew     = "⭐️"
	EmojiIdea    = "💡"
)

// TaskEmojis contains all emojis used for task windows.
var TaskEmojis = []string{
	EmojiWorking,
	EmojiWaiting,
	EmojiDone,
	EmojiWarning,
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

// ExtractTaskName extracts the task name from a window name by removing the emoji prefix.
// Returns the task name and true if a task emoji was found, or empty string and false otherwise.
// Note: The returned name may be truncated to MaxWindowNameLen characters.
func ExtractTaskName(windowName string) (string, bool) {
	for _, emoji := range TaskEmojis {
		if strings.HasPrefix(windowName, emoji) {
			return strings.TrimPrefix(windowName, emoji), true
		}
	}
	return "", false
}

// TruncateForWindowName truncates a task name to fit in window name display.
// This should be used when comparing task names to window names since window
// names are truncated to MaxWindowNameLen characters.
func TruncateForWindowName(name string) string {
	if len(name) > MaxWindowNameLen {
		return name[:MaxWindowNameLen]
	}
	return name
}

// Display limits
const (
	MaxDisplayNameLen = 32
	MaxTaskNameLen    = 32
	MinTaskNameLen    = 8
	MaxWindowNameLen  = 12 // Max task name length in tmux window names
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
	PawDirName       = ".paw"
	AgentsDirName    = "agents"
	HistoryDirName   = "history"
	ConfigFileName   = "config"
	LogFileName      = "log"
	MemoryFileName   = "memory"
	PromptFileName   = "PROMPT.md"
	TaskFileName     = "task"
	TabLockDirName   = ".tab-lock"
	WindowIDFileName = "window_id"
	PRFileName       = ".pr"
	GitRepoMarker    = ".is-git-repo"
	GlobalPromptLink = ".global-prompt"
	ClaudeLink       = ".claude"
)

// Tmux related constants
const (
	TmuxSocketPrefix = "paw-"
	NewWindowName    = EmojiNew + "main"
	IdeaWindowName   = EmojiIdea + "idea"
)

// Pane capture settings
const (
	PaneCaptureLines = 10000 // Number of lines to capture from pane history
	SummaryMaxLen    = 8000  // Max characters to send for summary generation
)

// Merge lock settings
const (
	MergeLockMaxRetries    = 30              // Maximum retries to acquire merge lock
	MergeLockRetryInterval = 1 * time.Second // Interval between lock retries
)

// Double-press detection
const (
	DoublePressIntervalSec = 2 // Seconds to wait for second keypress
)

// Task window handling
const (
	WindowIDWaitMaxAttempts = 60                     // Max attempts to wait for window ID file
	WindowIDWaitInterval    = 500 * time.Millisecond // Interval between checks
)
