package main

import (
	"fmt"

	"github.com/donghojung/taw/internal/tmux"
)

// buildKeybindings creates tmux keybindings for TAW.
// Hotkey scheme:
//   - Alt+Tab: Cycle panes
//   - Alt+Left/Right: Navigate windows
//   - Ctrl+R: Command palette (fzf-based fuzzy search)
//
// Note: Ctrl+C/D are NOT bound to preserve normal terminal behavior.
// To exit the session, use Ctrl+R → detach.
func buildKeybindings(tawBin, sessionName string) []tmux.BindOpts {
	// Command palette command
	cmdPalette := fmt.Sprintf("run-shell '%s internal command-palette %s'", tawBin, sessionName)

	return []tmux.BindOpts{
		// Navigation (Alt-based)
		{Key: "M-Tab", Command: "select-pane -t :.+", NoPrefix: true},
		{Key: "M-Left", Command: "previous-window", NoPrefix: true},
		{Key: "M-Right", Command: "next-window", NoPrefix: true},

		// Command palette (Ctrl+R)
		{Key: "C-r", Command: cmdPalette, NoPrefix: true},
	}
}
