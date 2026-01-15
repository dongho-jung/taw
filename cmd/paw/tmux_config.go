package main

import (
	"os"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

// reapplyTmuxConfig re-applies tmux configuration after config reload.
// This is a subset of setupTmuxConfig that updates settings that depend on config.
func reapplyTmuxConfig(appCtx *app.App, tm tmux.Client) error {
	// Get path to paw binary
	pawBin, err := os.Executable()
	if err != nil {
		pawBin = "paw"
	}

	// Re-apply keybindings (in case session name changed or for consistency)
	bindings := buildKeybindings(pawBin, appCtx.SessionName)
	for _, b := range bindings {
		if err := tm.Bind(b); err != nil {
			logging.Debug("Failed to bind %s: %v", b.Key, err)
		}
	}

	return nil
}

// setupTmuxConfig configures tmux keybindings and options
func setupTmuxConfig(appCtx *app.App, tm tmux.Client) error {
	// Get path to paw binary
	pawBin, err := os.Executable()
	if err != nil {
		pawBin = "paw"
	}

	// Detect terminal theme and apply theme-aware colors (auto only)
	resolved := resolveThemePreset(ThemeAuto)
	applyTmuxTheme(tm, resolved)

	// Change prefix to an unused key (M-F12) so C-b is available for toggle-bottom
	// Note: "None" is not a valid tmux key, so we use an obscure key instead
	_ = tm.SetOption("prefix", "M-F12", true)
	_ = tm.SetOption("prefix2", "M-F12", true)

	// Setup terminal title (for iTerm2 tab naming)
	// This makes tmux set the terminal title, which works better than OSC sequences
	// because iTerm otherwise shows the running command (tmux -f /var/...)
	_ = tm.SetOption("set-titles", "on", true)
	_ = tm.SetOption("set-titles-string", "[paw] "+appCtx.SessionName, true)

	// Setup status bar
	_ = tm.SetOption("status", "on", true)
	_ = tm.SetOption("status-position", "bottom", true)
	_ = tm.SetOption("status-left", " "+appCtx.SessionName+" ", true)
	_ = tm.SetOption("status-left-length", "30", true)
	_ = tm.SetOption("status-right", " ⌥←→:windows ⌥Tab:panes ^/:help ", true)
	_ = tm.SetOption("status-right-length", "100", true)

	// Window status separator (no separator between windows)
	_ = tm.SetOption("window-status-separator", "", true)

	// Popup styling (use terminal colors for content, border is set by applyTmuxTheme)
	_ = tm.SetOption("popup-style", "fg=terminal,bg=terminal", true)

	// Enable mouse mode
	_ = tm.SetOption("mouse", "on", true)

	// Enable focus events (required for tea.FocusMsg to work)
	// This is required for auto-focusing the input textarea when switching windows
	_ = tm.SetOption("focus-events", "on", true)

	// Conditional mouse drag handling: TUI windows vs normal windows
	// - In main window (⭐️main) pane 0: Forward drag events to bubbletea TUI for cell-level selection
	// - In other panes (shell pane via Ctrl+B) or task windows: Use normal tmux copy-mode
	// The #{m:pattern,string} format checks if string matches pattern (⭐* = starts with ⭐).
	// The #{&&:...} format combines multiple conditions with AND.
	// Note: ⭐️ is multi-byte UTF-8, so we use the prefix check pattern.
	_ = tm.Run("bind", "-n", "MouseDrag1Pane",
		"if-shell", "-F", "#{&&:#{m:⭐*,#{window_name}},#{==:#{pane_index},0}}",
		"send-keys -M", // Forward mouse event to pane (TUI handles it)
		"copy-mode -M") // Enter copy-mode with mouse selection

	// Enable vi-style copy mode and clipboard integration
	_ = tm.SetOption("mode-keys", "vi", true)
	_ = tm.SetOption("set-clipboard", "on", true)

	// Allow escape sequences to pass through to the terminal (tmux 3.3+)
	// Enables OSC 52 clipboard, terminal images, hyperlinks, etc.
	_ = tm.SetOption("allow-passthrough", "all", true)

	// Auto-copy to system clipboard when mouse selection ends
	// In copy-mode, commands must use "send-keys -X" format
	_ = tm.Bind(tmux.BindOpts{
		Key:     "MouseDragEnd1Pane",
		Command: "send-keys -X copy-pipe-and-cancel 'pbcopy'",
		Table:   "copy-mode-vi",
	})

	// Unbind C-b from root table before setting up keybindings
	// This ensures C-b doesn't act as prefix even if tmux.conf wasn't reloaded
	// (tmux.conf is only loaded when server starts, not on reconnect)
	_ = tm.Run("unbind-key", "-T", "root", "C-b")

	// Setup keybindings (English + Korean layouts)
	bindings := buildKeybindings(pawBin, appCtx.SessionName)
	for _, b := range bindings {
		if err := tm.Bind(b); err != nil {
			logging.Debug("Failed to bind %s: %v", b.Key, err)
		}
	}

	return nil
}
