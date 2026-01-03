package main

import (
	"fmt"

	"github.com/dongho-jung/taw/internal/tmux"
)

// buildKeybindings creates tmux keybindings for TAW.
// Keyboard shortcuts:
//   - Ctrl+N: New task
//   - Ctrl+K: Send Ctrl+C (double-press to cancel task)
//   - Ctrl+F: Finish task
//   - Ctrl+Q: Quit taw
//   - Ctrl+T: Toggle tasks
//   - Ctrl+O: Toggle logs
//   - Ctrl+G: Toggle git status
//   - Ctrl+B: Toggle bottom (shell)
//   - Ctrl+/: Toggle help
//   - Alt+Left/Right: Move window
//   - Alt+Tab: Cycle pane
func buildKeybindings(tawBin, sessionName string) []tmux.BindOpts {
	// Command shortcuts
	cmdNewTask := fmt.Sprintf("run-shell '%s internal toggle-new %s'", tawBin, sessionName)
	cmdCtrlC := fmt.Sprintf("run-shell '%s internal ctrl-c %s'", tawBin, sessionName)
	cmdDoneTask := fmt.Sprintf("run-shell '%s internal done-task %s'", tawBin, sessionName)
	cmdQuit := "detach-client"
	cmdToggleTasks := fmt.Sprintf("run-shell '%s internal toggle-task-list %s'", tawBin, sessionName)
	cmdToggleLogs := fmt.Sprintf("run-shell '%s internal toggle-log %s'", tawBin, sessionName)
	cmdToggleGitStatus := fmt.Sprintf("run-shell '%s internal toggle-git-status %s'", tawBin, sessionName)
	cmdToggleBottom := fmt.Sprintf("run-shell '%s internal popup-shell %s'", tawBin, sessionName)
	cmdToggleHelp := fmt.Sprintf("run-shell '%s internal toggle-help %s'", tawBin, sessionName)

	return []tmux.BindOpts{
		// Navigation (Alt-based)
		{Key: "M-Tab", Command: "select-pane -t :.+", NoPrefix: true},
		{Key: "M-Left", Command: "previous-window", NoPrefix: true},
		{Key: "M-Right", Command: "next-window", NoPrefix: true},

		// Task commands (Ctrl-based)
		{Key: "C-n", Command: cmdNewTask, NoPrefix: true},
		{Key: "C-k", Command: cmdCtrlC, NoPrefix: true},
		{Key: "C-f", Command: cmdDoneTask, NoPrefix: true},
		{Key: "C-q", Command: cmdQuit, NoPrefix: true},

		// Toggle commands (Ctrl-based)
		{Key: "C-t", Command: cmdToggleTasks, NoPrefix: true},
		{Key: "C-o", Command: cmdToggleLogs, NoPrefix: true},
		{Key: "C-g", Command: cmdToggleGitStatus, NoPrefix: true},
		{Key: "C-b", Command: cmdToggleBottom, NoPrefix: true},
		{Key: "C-_", Command: cmdToggleHelp, NoPrefix: true}, // Ctrl+/ sends C-_
	}
}
