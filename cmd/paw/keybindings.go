package main

import (
	"fmt"

	"github.com/dongho-jung/paw/internal/tmux"
)

// buildKeybindings creates tmux keybindings for PAW.
// Keyboard shortcuts:
//   - Ctrl+N: New task
//   - Ctrl+K: Send Ctrl+C (double-press to cancel task)
//   - Ctrl+F: Finish task
//   - Ctrl+Q: Quit paw
//   - Ctrl+T: Toggle tasks
//   - Ctrl+O: Toggle logs
//   - Ctrl+G: Toggle git status
//   - Ctrl+B: Toggle bottom (shell)
//   - Ctrl+/: Toggle help
//   - Ctrl+,: Toggle setup (rerun setup wizard)
//   - Alt+Left/Right: Move window
//   - Alt+Tab: Cycle pane
func buildKeybindings(pawBin, sessionName string) []tmux.BindOpts {
	// Command shortcuts
	cmdNewTask := fmt.Sprintf("run-shell '%s internal toggle-new %s'", pawBin, sessionName)
	cmdCtrlC := fmt.Sprintf("run-shell '%s internal ctrl-c %s'", pawBin, sessionName)
	cmdDoneTask := fmt.Sprintf("run-shell '%s internal done-task %s'", pawBin, sessionName)
	cmdQuit := "detach-client"
	cmdToggleTasks := fmt.Sprintf("run-shell '%s internal toggle-task-list %s'", pawBin, sessionName)
	cmdToggleLogs := fmt.Sprintf("run-shell '%s internal toggle-log %s'", pawBin, sessionName)
	cmdToggleGitStatus := fmt.Sprintf("run-shell '%s internal toggle-git-status %s'", pawBin, sessionName)
	cmdToggleBottom := fmt.Sprintf("run-shell '%s internal popup-shell %s'", pawBin, sessionName)
	cmdToggleHelp := fmt.Sprintf("run-shell '%s internal toggle-help %s'", pawBin, sessionName)
	cmdToggleSetup := fmt.Sprintf("run-shell '%s internal toggle-setup %s'", pawBin, sessionName)

	// Ctrl+. sends F2 to open task options (used in new task window)
	cmdTaskOpts := "send-keys F2"

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
		{Key: "C-/", Command: cmdToggleHelp, NoPrefix: true}, // Ctrl+/ (extended-keys mode)

		// Settings
		{Key: "C-,", Command: cmdToggleSetup, NoPrefix: true}, // Ctrl+, for setup
		{Key: "C-.", Command: cmdTaskOpts, NoPrefix: true},    // Ctrl+. for task options (sends F2)
	}
}
