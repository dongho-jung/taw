package main

import (
	"fmt"

	"github.com/donghojung/taw/internal/tmux"
)

// ModifierPrefix is the tmux key modifier for all TAW hotkeys.
// Change this to customize the modifier (e.g., "M-" for Alt, "C-S-" for Ctrl+Shift).
const ModifierPrefix = "C-S-"

// hotkeyDef defines a hotkey action
type hotkeyDef struct {
	key     string // Key (e.g., "n", "t", "/")
	command string
}

// buildKeybindings creates tmux keybindings with the configured modifier
func buildKeybindings(tawBin, sessionName string) []tmux.BindOpts {
	// Command templates
	cmdToggleNew := fmt.Sprintf("run-shell '%s internal toggle-new %s'", tawBin, sessionName)
	cmdToggleTaskList := fmt.Sprintf("run-shell '%s internal toggle-task-list %s'", tawBin, sessionName)
	cmdEndTaskUI := fmt.Sprintf("run-shell '%s internal end-task-ui %s #{window_id}'", tawBin, sessionName)
	cmdMergeCompleted := fmt.Sprintf("run-shell '%s internal merge-completed %s'", tawBin, sessionName)
	cmdPopupShell := fmt.Sprintf("run-shell '%s internal popup-shell %s'", tawBin, sessionName)
	cmdQuickTask := fmt.Sprintf("run-shell '%s internal quick-task %s'", tawBin, sessionName)
	cmdToggleLog := fmt.Sprintf("run-shell '%s internal toggle-log %s'", tawBin, sessionName)
	cmdToggleHelp := fmt.Sprintf("run-shell '%s internal toggle-help %s'", tawBin, sessionName)

	// Hotkey definitions (key -> command)
	hotkeys := []hotkeyDef{
		{"n", cmdToggleNew},
		{"t", cmdToggleTaskList},
		{"e", cmdEndTaskUI},
		{"m", cmdMergeCompleted},
		{"p", cmdPopupShell},
		{"u", cmdQuickTask},
		{"l", cmdToggleLog},
		{"/", cmdToggleHelp},
		{"q", "detach"},
	}

	// Build bindings
	var bindings []tmux.BindOpts

	// Navigation keys
	bindings = append(bindings,
		tmux.BindOpts{Key: ModifierPrefix + "Tab", Command: "select-pane -t :.+", NoPrefix: true},
		tmux.BindOpts{Key: ModifierPrefix + "Left", Command: "previous-window", NoPrefix: true},
		tmux.BindOpts{Key: ModifierPrefix + "Right", Command: "next-window", NoPrefix: true},
	)

	// Add bindings for each hotkey
	for _, hk := range hotkeys {
		bindings = append(bindings, tmux.BindOpts{
			Key:      ModifierPrefix + hk.key,
			Command:  hk.command,
			NoPrefix: true,
		})
	}

	return bindings
}
