// Package tui provides terminal user interface components for PAW.
package tui

import (
	"math/rand/v2"
	"regexp"
)

// Version is the PAW version string, set from main package.
var Version = "dev"

// ProjectName is the display name for UI (may include subdir like "repo/subdir").
var ProjectName = ""

// SessionName is the tmux session name (safe chars, no slashes like "repo-subdir").
var SessionName = ""

// tips contains usage tips shown to users.
var tips = []string{
	// Keyboard shortcuts - Task commands
	"Press ⌃N to create a new task",
	"Press ⌃R to search task history",
	"Press ⌃F to finish task (action picker)",
	"Press Alt+Enter or F5 to submit task",
	"Press Esc twice quickly to cancel input",

	// Keyboard shortcuts - Navigation
	"Press ⌥Tab to switch between panels",
	"Use ⌥←/→ to move between windows",
	"Press ⌃↑ to toggle between task and main",
	"Press ⌃↓ to sync with main branch",

	// Keyboard shortcuts - Toggle panels
	"Press ⌃P to open command palette",
	"Press ⌃G to view git status/log",
	"Press ⌃O to view logs",
	"Press ⌃B to toggle bottom shell",
	"Press ⌃/ for help",

	// Mouse interactions
	"Use mouse to select and copy text",
	"Click on a task in kanban to jump to it",
	"Scroll with mouse wheel in kanban",

	// Workflow tips
	"Each task runs in its own git worktree",
	"Tasks can run in parallel without conflicts",
	"Worktree mode keeps main branch clean",
	"Task history is saved for easy reuse",

	// Configuration
	"Configure notifications in .paw/config",
	"Run 'paw check' to verify dependencies",
	"Run 'paw attach' to reconnect sessions",
	"Run 'paw history show' to review past work",
}

// versionHashRegex matches the git hash suffix in version strings.
// Pattern: -g[0-9a-f]+ or -g[0-9a-f]+-dirty at the end
var versionHashRegex = regexp.MustCompile(`-g[0-9a-f]+(-dirty)?$`)

// GetTip returns a random usage tip.
// Each call returns a different random tip.
func GetTip() string {
	return tips[rand.IntN(len(tips))]
}

// SetVersion sets the PAW version string, stripping the git hash suffix.
// e.g., "v0.3.0-32-gabcdef" -> "v0.3.0-32"
func SetVersion(v string) {
	Version = versionHashRegex.ReplaceAllString(v, "")
}

// SetProjectName sets the display name for UI.
func SetProjectName(name string) {
	ProjectName = name
}

// SetSessionName sets the tmux session name.
func SetSessionName(name string) {
	SessionName = name
}
