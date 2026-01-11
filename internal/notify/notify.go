// Package notify provides cross-platform desktop notifications using terminal escape sequences.
//
// Supported terminals and protocols:
//   - iTerm2: OSC 9 (basic notifications)
//   - Kitty: OSC 99 (rich notifications with urgency, icons, sounds)
//   - WezTerm: OSC 777 (title + body)
//   - Ghostty: OSC 777 (title + body), OSC 9
//   - Windows Terminal: OSC 9 (basic notifications)
//   - VSCode Terminal: OSC 9 (forwarded through SSH)
//   - foot: OSC 777, OSC 99
//   - Contour: OSC 99, OSC 777
//   - rxvt-unicode: OSC 777
//   - Linux: notify-send fallback when available
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

// Urgency represents notification urgency levels.
// Supported by Kitty (OSC 99) and Linux notify-send.
type Urgency int

const (
	// UrgencyLow for non-critical notifications.
	UrgencyLow Urgency = 0
	// UrgencyNormal for standard notifications (default).
	UrgencyNormal Urgency = 1
	// UrgencyCritical for important notifications that should not be missed.
	UrgencyCritical Urgency = 2
)

// Icon represents standard notification icon names.
// Supported by terminals implementing OSC 99 with icon support (Kitty, foot).
type Icon string

const (
	// IconNone indicates no icon should be shown.
	IconNone Icon = ""
	// IconInfo for informational notifications.
	IconInfo Icon = "info"
	// IconWarning for warning notifications.
	IconWarning Icon = "warning"
	// IconError for error notifications.
	IconError Icon = "error"
	// IconQuestion for prompts requiring user input.
	IconQuestion Icon = "question"
	// IconHelp for help-related notifications.
	IconHelp Icon = "help"
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
	termITerm2          = "iterm2"
	termKitty           = "kitty"
	termWezTerm         = "wezterm"
	termGhostty         = "ghostty"
	termRxvt            = "rxvt"
	termWindowsTerminal = "windows-terminal"
	termVSCode          = "vscode"
	termFoot            = "foot"
	termContour         = "contour"
	termUnknown         = "unknown"
)

// NotifyOptions contains optional parameters for notifications.
type NotifyOptions struct {
	Urgency Urgency // Notification urgency level (default: UrgencyNormal)
	Icon    Icon    // Standard icon name (default: none)
}

// Send shows a desktop notification using terminal escape sequences.
// Supports multiple terminals (iTerm2, Kitty, WezTerm, Ghostty, rxvt, Windows Terminal,
// VSCode, foot, Contour) with fallback to Linux notify-send and terminal bell.
func Send(title, message string) error {
	return SendWithOptions(title, message, NotifyOptions{Urgency: UrgencyNormal})
}

// SendWithUrgency shows a desktop notification with the specified urgency level.
func SendWithUrgency(title, message string, urgency Urgency) error {
	return SendWithOptions(title, message, NotifyOptions{Urgency: urgency})
}

// SendWithOptions shows a desktop notification with custom options.
func SendWithOptions(title, message string, opts NotifyOptions) error {
	logging.Debug("-> SendWithOptions(title=%q, message=%q, urgency=%d, icon=%q)", title, message, opts.Urgency, opts.Icon)
	defer logging.Debug("<- SendWithOptions")

	sendTerminalNotification(title, message, opts)
	return nil
}

// sendTerminalNotification sends notification using appropriate terminal protocol.
func sendTerminalNotification(title, message string, opts NotifyOptions) {
	term := detectTerminal()
	inTmux := os.Getenv("TMUX") != ""

	logging.Trace("sendTerminalNotification: term=%s, inTmux=%v, urgency=%d", term, inTmux, opts.Urgency)

	// Send appropriate OSC based on terminal
	switch term {
	case termKitty:
		// Kitty supports OSC 99 with rich features (urgency, icons, occasion control)
		sendOSC99Enhanced(title, message, opts, inTmux)
	case termFoot, termContour:
		// foot and Contour support OSC 99 with icon support
		sendOSC99Enhanced(title, message, opts, inTmux)
	case termITerm2:
		// iTerm2 pioneered OSC 9
		sendOSC9(message, inTmux)
	case termWezTerm, termGhostty:
		// WezTerm and Ghostty support both OSC 9 and OSC 777
		// Use OSC 777 for title+body support
		sendOSC777(title, message, inTmux)
	case termWindowsTerminal, termVSCode:
		// Windows Terminal and VSCode support OSC 9
		sendOSC9(message, inTmux)
	case termRxvt:
		// rxvt-unicode uses OSC 777
		sendOSC777(title, message, inTmux)
	default:
		// Try notify-send on Linux first, then fall back to OSC 9
		if runtime.GOOS == "linux" && tryNotifySend(title, message, opts) {
			// notify-send succeeded, still send bell for terminal alert
		} else {
			// Try OSC 9 (most widely supported) for unknown terminals
			sendOSC9(message, inTmux)
		}
	}

	// Always send bell as additional alert
	// Terminals can be configured to show notification on bell
	fmt.Fprint(os.Stderr, BEL)
}

// detectTerminal returns the terminal emulator type.
func detectTerminal() string {
	// Check terminal-specific environment variables (most reliable)
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return termKitty
	}
	if os.Getenv("WEZTERM_PANE") != "" {
		return termWezTerm
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return termGhostty
	}
	// Windows Terminal sets WT_SESSION
	if os.Getenv("WT_SESSION") != "" {
		return termWindowsTerminal
	}
	if os.Getenv("WARP_TERMINAL_VERSION") != "" {
		// Warp doesn't support OSC notifications yet
		return termUnknown
	}

	// Check TERM_PROGRAM for macOS and cross-platform terminals
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	switch {
	case strings.Contains(termProgram, "iterm"):
		return termITerm2
	case strings.Contains(termProgram, "wezterm"):
		return termWezTerm
	case strings.Contains(termProgram, "ghostty"):
		return termGhostty
	case termProgram == "vscode":
		return termVSCode
	}

	// Check TERM for terminal-specific identifiers
	term := strings.ToLower(os.Getenv("TERM"))
	switch {
	case strings.Contains(term, "rxvt"):
		return termRxvt
	case strings.HasPrefix(term, "foot"):
		return termFoot
	case strings.HasPrefix(term, "contour"):
		return termContour
	}

	// Check for VSCode via alternative detection
	// VSCode sets VSCODE_INJECTION when running in integrated terminal
	if os.Getenv("VSCODE_INJECTION") != "" || os.Getenv("VSCODE_GIT_IPC_HANDLE") != "" {
		return termVSCode
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

// sendOSC99 sends basic Kitty-style notification (legacy function for compatibility).
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

// sendOSC99Enhanced sends Kitty-style notification with full options.
// Format: ESC ] 99 ; metadata ; payload <terminator>
// Metadata keys:
//   - i: notification id for updates/close
//   - d: done flag (0=incomplete, 1=complete)
//   - u: urgency (0=low, 1=normal, 2=critical)
//   - o: occasion (always, unfocused, invisible)
//   - a: activation action (focus, report)
//   - c: close event notification (0 or 1)
//   - n: icon name (base64 encoded for symbolic names)
//
// Supported by: Kitty, foot, Contour
// See: https://sw.kovidgoyal.net/kitty/desktop-notifications/
func sendOSC99Enhanced(title, message string, opts NotifyOptions, inTmux bool) {
	// Build metadata string with all supported options
	// i=paw: unique notification id (allows updating/closing)
	// d=0: notification is not done (show it immediately)
	// u=N: urgency level
	// o=unfocused: only show when terminal is not focused (reduces noise)
	// a=focus: clicking the notification focuses the terminal window
	metadata := fmt.Sprintf("i=paw:d=0:u=%d:o=unfocused:a=focus", opts.Urgency)

	// Add icon if specified (use standard icon names)
	if opts.Icon != IconNone {
		metadata += ":n=" + string(opts.Icon)
	}

	// First, send title payload
	titleOSC := fmt.Sprintf("%s]99;%s:p=title;%s%s", ESC, metadata, title, BEL)
	writeOSC(titleOSC, inTmux)

	// Then send body payload if message differs from title
	if message != "" && message != title {
		bodyOSC := fmt.Sprintf("%s]99;i=paw:p=body;%s%s", ESC, message, BEL)
		writeOSC(bodyOSC, inTmux)
	}
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

// notifySendPath caches the path to notify-send binary.
// Empty string means not checked, "-" means not found.
var notifySendPath string

// tryNotifySend attempts to send notification via Linux notify-send.
// Returns true if notification was sent successfully.
// This is used as a fallback for Linux systems when terminal doesn't support OSC notifications.
func tryNotifySend(title, message string, opts NotifyOptions) bool {
	// Check if notify-send is available (cached)
	if notifySendPath == "" {
		path, err := exec.LookPath("notify-send")
		if err != nil {
			notifySendPath = "-" // Mark as not found
		} else {
			notifySendPath = path
		}
	}

	if notifySendPath == "-" {
		return false
	}

	// Build notify-send command with options
	args := []string{}

	// Add urgency level
	switch opts.Urgency {
	case UrgencyLow:
		args = append(args, "-u", "low")
	case UrgencyNormal:
		args = append(args, "-u", "normal")
	case UrgencyCritical:
		args = append(args, "-u", "critical")
	}

	// Add icon if specified
	if opts.Icon != IconNone {
		// notify-send accepts standard icon names directly
		args = append(args, "-i", string(opts.Icon))
	}

	// Add app name for identification
	args = append(args, "-a", "PAW")

	// Add title and message
	args = append(args, title)
	if message != "" && message != title {
		args = append(args, message)
	}

	// Run notify-send in background (don't wait)
	cmd := exec.Command(notifySendPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		logging.Debug("tryNotifySend: failed to start notify-send err=%v", err)
		return false
	}

	logging.Trace("tryNotifySend: sent notification via notify-send")
	return true
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
