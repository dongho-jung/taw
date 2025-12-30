package main

import (
	"fmt"

	"github.com/donghojung/taw/internal/tmux"
)

// buildKeybindings creates tmux keybindings for TAW.
// New simplified hotkey scheme:
//   - Alt+Tab: Cycle panes
//   - Alt+Left/Right: Navigate windows
//   - Ctrl+R: Command palette (fzf-based fuzzy search)
//   - Ctrl+C/D twice: Exit session
func buildKeybindings(tawBin, sessionName string) []tmux.BindOpts {
	// Command palette command
	cmdPalette := fmt.Sprintf("run-shell '%s internal command-palette %s'", tawBin, sessionName)

	// Double quit command (Ctrl+C/D twice to exit)
	// First send the key to pane, then check for double quit in background
	cmdDoubleQuitC := fmt.Sprintf("send-keys C-c \\; run-shell -b '%s internal double-quit %s'", tawBin, sessionName)
	cmdDoubleQuitD := fmt.Sprintf("send-keys C-d \\; run-shell -b '%s internal double-quit %s'", tawBin, sessionName)

	return []tmux.BindOpts{
		// Navigation (Alt-based)
		{Key: "M-Tab", Command: "select-pane -t :.+", NoPrefix: true},
		{Key: "M-Left", Command: "previous-window", NoPrefix: true},
		{Key: "M-Right", Command: "next-window", NoPrefix: true},

		// Command palette (Ctrl+R)
		{Key: "C-r", Command: cmdPalette, NoPrefix: true},

		// Double quit (Ctrl+C/D twice to exit)
		{Key: "C-c", Command: cmdDoubleQuitC, NoPrefix: true},
		{Key: "C-d", Command: cmdDoubleQuitD, NoPrefix: true},
	}
}
