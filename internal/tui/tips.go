// Package tui provides terminal user interface components for PAW.
package tui

import (
	"math/rand/v2"
)

// Version is the PAW version string, set from main package.
var Version = "dev"

// tips contains usage tips shown to users.
var tips = []string{
	"Press ⌃R to search task history",
	"Press ⌥Tab to switch between panels",
	"Press ⌃T to open task list",
	"Press ⌃O to view logs",
	"Press ⌃B to toggle bottom shell",
	"Press ⌃/ for help",
	"Press ⌃F to finish a completed task",
	"Press ⌃K to cancel a running task",
	"Press ⌃S to sync with main branch",
	"Use mouse to select and copy text",
	"Drag to select text, then ⌃C to copy",
	"Scroll with mouse wheel in kanban",
}

// selectedTip is cached on init to show the same tip during the session.
var selectedTip string

func init() {
	// Select a random tip at startup (Go 1.20+ auto-seeds math/rand)
	selectedTip = tips[rand.IntN(len(tips))]
}

// GetTip returns a random usage tip.
func GetTip() string {
	return selectedTip
}

// SetVersion sets the PAW version string.
func SetVersion(v string) {
	Version = v
}
