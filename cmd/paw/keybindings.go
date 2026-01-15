package main

import (
	"fmt"

	"github.com/dongho-jung/paw/internal/tmux"
)

// buildKeybindings creates tmux keybindings for PAW.
// Keyboard shortcuts:
//   - Ctrl+N: New task
//   - Ctrl+F: Finish task (shows action picker)
//   - Ctrl+P: Command palette
//   - Ctrl+Q: Quit paw
//   - Ctrl+O: Toggle logs
//   - Ctrl+G: Toggle git viewer (status/log/graph modes)
//   - Ctrl+B: Toggle bottom (shell)
//   - Ctrl+/: Toggle help
//   - Ctrl+R: Toggle history search (in new task window only)
//   - Ctrl+T: Toggle template picker (in new task window only)
//   - Ctrl+J: Toggle project picker (switch between PAW sessions)
//   - Alt+Left/Right: Move window
//   - Alt+Tab: Cycle pane forward (in task windows) / Cycle options (in new task window)
//   - Alt+Shift+Tab: Cycle pane backward (in task windows) / Cycle options backward (in new task window)
func buildKeybindings(pawBin, sessionName string) []tmux.BindOpts {
	// Command shortcuts
	cmdNewTask := fmt.Sprintf("run-shell '%s internal toggle-new %s'", pawBin, sessionName)
	cmdDoneTask := fmt.Sprintf("run-shell '%s internal done-task %s'", pawBin, sessionName)
	cmdQuit := "detach-client"
	cmdToggleLogs := fmt.Sprintf("run-shell '%s internal toggle-log %s'", pawBin, sessionName)
	cmdToggleGitStatus := fmt.Sprintf("run-shell '%s internal toggle-git-status %s'", pawBin, sessionName)
	cmdToggleBottom := fmt.Sprintf("run-shell '%s internal popup-shell %s'", pawBin, sessionName)
	cmdToggleHelp := fmt.Sprintf("run-shell '%s internal toggle-help %s'", pawBin, sessionName)
	cmdToggleCmdPalette := fmt.Sprintf("run-shell '%s internal toggle-cmd-palette %s'", pawBin, sessionName)
	cmdToggleProjectPicker := fmt.Sprintf("run-shell '%s internal toggle-project-picker %s'", pawBin, sessionName)

	// Alt+Tab: context-aware - pass through to TUI in new task window, cycle panes otherwise
	// #{m:pattern,string} checks if string matches pattern (⭐️* = starts with ⭐️)
	// Use "Escape Tab" instead of "M-Tab" because send-keys M-Tab may not produce
	// the correct escape sequence (\x1b\x09) that bubbletea expects for "alt+tab"
	// -F flag is required so tmux evaluates the format as a boolean, not as a shell command
	cmdAltTab := `if -F "#{m:⭐️*,#{window_name}}" "send-keys Escape Tab" "select-pane -t :.+"`
	cmdAltShiftTab := `if -F "#{m:⭐️*,#{window_name}}" "send-keys Escape BTab" "select-pane -t :.-"`

	// Ctrl+R: context-aware - show history picker only in new task window (⭐️)
	// In other windows, pass through Ctrl+R for normal reverse search
	cmdCtrlR := fmt.Sprintf(`if -F "#{m:⭐️*,#{window_name}}" "run-shell '%s internal toggle-history %s'" "send-keys C-r"`, pawBin, sessionName)
	// Ctrl+T: context-aware - show template picker only in new task window (⭐️)
	cmdCtrlT := fmt.Sprintf(`if -F "#{m:⭐️*,#{window_name}}" "run-shell '%s internal toggle-template %s'" "send-keys C-t"`, pawBin, sessionName)

	return []tmux.BindOpts{
		// Navigation (Alt-based)
		{Key: "M-Tab", Command: cmdAltTab, NoPrefix: true},
		{Key: "M-BTab", Command: cmdAltShiftTab, NoPrefix: true},
		{Key: "M-Left", Command: "previous-window", NoPrefix: true},
		{Key: "M-Right", Command: "next-window", NoPrefix: true},

		// Task commands (Ctrl-based)
		{Key: "C-n", Command: cmdNewTask, NoPrefix: true},
		{Key: "C-f", Command: cmdDoneTask, NoPrefix: true},
		{Key: "C-p", Command: cmdToggleCmdPalette, NoPrefix: true},
		{Key: "C-q", Command: cmdQuit, NoPrefix: true},

		// Toggle commands (Ctrl-based)
		{Key: "C-o", Command: cmdToggleLogs, NoPrefix: true},
		{Key: "C-g", Command: cmdToggleGitStatus, NoPrefix: true},
		{Key: "C-b", Command: cmdToggleBottom, NoPrefix: true},
		{Key: "C-_", Command: cmdToggleHelp, NoPrefix: true},          // Ctrl+/ sends C-_
		{Key: "C-r", Command: cmdCtrlR, NoPrefix: true},               // History search in new task window
		{Key: "C-t", Command: cmdCtrlT, NoPrefix: true},               // Template picker in new task window
		{Key: "C-j", Command: cmdToggleProjectPicker, NoPrefix: true}, // Project picker
	}
}
