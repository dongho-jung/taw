// Package constants defines shared constants used throughout the TAW application.
package constants

import "time"

// Window status emojis
const (
	EmojiWorking = "🤖"
	EmojiWaiting = "💬"
	EmojiDone    = "✅"
	EmojiWarning = "⚠️"
	EmojiNew     = "⭐️"
)

// Display limits
const (
	MaxDisplayNameLen = 32
	MaxTaskNameLen    = 32
	MinTaskNameLen    = 8
)

// Claude interaction timeouts
const (
	ClaudeReadyMaxAttempts  = 60
	ClaudeReadyPollInterval = 500 * time.Millisecond
	ClaudeNameGenTimeout1   = 3 * time.Second
	ClaudeNameGenTimeout2   = 5 * time.Second
	ClaudeNameGenTimeout3   = 10 * time.Second
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
	DefaultMainBranch  = "main"
	DefaultWorkMode    = "worktree"
	DefaultOnComplete  = "confirm"
)

// Directory and file names
const (
	TawDirName       = ".taw"
	AgentsDirName    = "agents"
	QueueDirName     = ".queue"
	ConfigFileName   = "config"
	LogFileName      = "log"
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
	TmuxSocketPrefix = "taw-"
	NewWindowName    = EmojiNew + "new"
)
