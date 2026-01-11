// Package notify provides cross-platform desktop notifications using terminal escape sequences.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/dongho-jung/paw/internal/logging"
)

// Terminal escape sequences
const (
	ESC = "\033"
	BEL = "\a"
	ST  = ESC + "\\" // String Terminator
)

// SoundType represents different notification sounds.
type SoundType string

const (
	// SoundTaskCreated is played when a task window is created.
	SoundTaskCreated SoundType = "Glass"
	// SoundTaskCompleted is played when a task is completed successfully.
	SoundTaskCompleted SoundType = "Hero"
	// SoundNeedInput is played when user intervention is needed.
	SoundNeedInput SoundType = "Funk"
	// SoundError is played when an error or problem occurs.
	SoundError SoundType = "Basso"
	// SoundCancelPending is played when waiting for second Ctrl+C to cancel.
	SoundCancelPending SoundType = "Tink"
)

// Terminal types
const (
	termITerm2  = "iterm2"
	termKitty   = "kitty"
	termWezTerm = "wezterm"
	termGhostty = "ghostty"
	termRxvt    = "rxvt"
	termUnknown = "unknown"
)

// Send shows a desktop notification using terminal escape sequences.
// Supports multiple terminals (iTerm2, Kitty, WezTerm, Ghostty, rxvt) with fallback to terminal bell.
func Send(title, message string) error {
	logging.Debug("-> Send(title=%q, message=%q)", title, message)
	defer logging.Debug("<- Send")

	sendTerminalNotification(title, message)
	return nil
}

// sendTerminalNotification sends notification using appropriate terminal protocol.
func sendTerminalNotification(title, message string) {
	term := detectTerminal()
	inTmux := os.Getenv("TMUX") != ""

	logging.Trace("sendTerminalNotification: term=%s, inTmux=%v", term, inTmux)

	// Send appropriate OSC based on terminal
	switch term {
	case termKitty:
		// Kitty supports both OSC 9 and OSC 99, prefer OSC 99 for richer notifications
		sendOSC99(title, message, inTmux)
	case termITerm2:
		// iTerm2 pioneered OSC 9
		sendOSC9(message, inTmux)
	case termWezTerm, termGhostty:
		// WezTerm and Ghostty support both OSC 9 and OSC 777
		// Use OSC 777 for title+body support
		sendOSC777(title, message, inTmux)
	case termRxvt:
		// rxvt-unicode uses OSC 777
		sendOSC777(title, message, inTmux)
	default:
		// Try OSC 9 (most widely supported) for unknown terminals
		sendOSC9(message, inTmux)
	}

	// Always send bell as additional alert
	// Terminals can be configured to show notification on bell
	fmt.Fprint(os.Stderr, BEL)
}

// detectTerminal returns the terminal emulator type.
func detectTerminal() string {
	// Check terminal-specific environment variables
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return termKitty
	}
	if os.Getenv("WEZTERM_PANE") != "" {
		return termWezTerm
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return termGhostty
	}
	if os.Getenv("WARP_TERMINAL_VERSION") != "" {
		// Warp doesn't support OSC notifications yet
		return termUnknown
	}

	// Check TERM_PROGRAM
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	if strings.Contains(termProgram, "iterm") {
		return termITerm2
	}
	if strings.Contains(termProgram, "wezterm") {
		return termWezTerm
	}
	if strings.Contains(termProgram, "ghostty") {
		return termGhostty
	}

	// Check TERM for rxvt
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "rxvt") {
		return termRxvt
	}

	return termUnknown
}

// sendOSC9 sends iTerm2-style notification.
// Format: ESC ] 9 ; message BEL
// Supported by: iTerm2, Kitty, WezTerm, Ghostty
func sendOSC9(message string, inTmux bool) {
	osc := fmt.Sprintf("%s]9;%s%s", ESC, message, BEL)
	writeOSC(osc, inTmux)
}

// sendOSC99 sends Kitty-style notification.
// Format: ESC ] 99 ; i=id:d=done:p=urgency ; body BEL
// Supported by: Kitty
// See: https://sw.kovidgoyal.net/kitty/desktop-notifications/
func sendOSC99(title, message string, inTmux bool) {
	body := title
	if message != "" && message != title {
		body = title + ": " + message
	}
	// i=1: notification id (for updates/close)
	// d=0: notification is not done (show it)
	// p=2: high urgency
	osc := fmt.Sprintf("%s]99;i=1:d=0:p=2;%s%s", ESC, body, BEL)
	writeOSC(osc, inTmux)
}

// sendOSC777 sends rxvt-unicode style notification.
// Format: ESC ] 777 ; notify ; title ; body BEL
// Supported by: rxvt-unicode, WezTerm, Ghostty
func sendOSC777(title, message string, inTmux bool) {
	osc := fmt.Sprintf("%s]777;notify;%s;%s%s", ESC, title, message, BEL)
	writeOSC(osc, inTmux)
}

// writeOSC writes an OSC sequence, wrapping for tmux passthrough if needed.
func writeOSC(osc string, inTmux bool) {
	if inTmux {
		osc = wrapTmuxPassthrough(osc)
	}
	fmt.Fprint(os.Stderr, osc)
}

// wrapTmuxPassthrough wraps an OSC sequence for tmux passthrough.
// Format: ESC P tmux; ESC <escaped-content> ESC \
// The escape characters inside need to be doubled.
func wrapTmuxPassthrough(osc string) string {
	// Double all escape characters for tmux passthrough
	escaped := strings.ReplaceAll(osc, ESC, ESC+ESC)
	return fmt.Sprintf("%sPtmux;%s%s", ESC, escaped, ST)
}

// PlaySound plays an alert sound.
// On macOS, uses system sounds via afplay. On other platforms, uses terminal bell.
func PlaySound(soundType SoundType) {
	logging.Debug("-> PlaySound(soundType=%s)", soundType)
	defer logging.Debug("<- PlaySound")

	if runtime.GOOS == "darwin" {
		soundPath := fmt.Sprintf("/System/Library/Sounds/%s.aiff", soundType)
		if _, err := os.Stat(soundPath); err == nil {
			// Run afplay in background (don't wait)
			cmd := exec.Command("afplay", soundPath)
			cmd.Stdout = nil
			cmd.Stderr = nil
			cmd.Stdin = nil
			if err := cmd.Start(); err != nil {
				logging.Warn("PlaySound: failed to start afplay err=%v", err)
			}
			return
		}
	}

	// Fallback: terminal bell
	fmt.Fprint(os.Stderr, BEL)
}

// SendWithActions shows a notification. Action buttons are not supported
// in terminal-based notifications, so this just sends a simple notification.
// Always returns -1 (no action selected).
func SendWithActions(title, message, iconPath string, actions []string, timeoutSec int) (int, error) {
	logging.Debug("-> SendWithActions(title=%q, actions=%v)", title, actions)
	defer logging.Debug("<- SendWithActions")

	if err := Send(title, message); err != nil {
		return -1, err
	}
	return -1, nil
}
