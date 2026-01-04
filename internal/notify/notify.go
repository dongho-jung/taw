// Package notify provides simple desktop notifications.
package notify

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/dongho-jung/taw/internal/config"
	"github.com/dongho-jung/taw/internal/logging"
)

const (
	// NotifyAppName is the name of the notification helper app bundle.
	NotifyAppName = "taw-notify.app"
	// NotifyBinaryName is the name of the executable inside the app bundle.
	NotifyBinaryName = "taw-notify"
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

// Send shows a desktop notification when supported.
// It prefers using the taw-notify helper for consistent icon display,
// falling back to AppleScript if the helper is not available.
func Send(title, message string) error {
	logging.Trace("Send: start title=%q message=%q", title, message)
	defer logging.Trace("Send: end title=%q", title)

	if runtime.GOOS != "darwin" {
		return nil
	}

	// Try taw-notify helper first for consistent icon display
	if helperPath := findNotifyHelper(); helperPath != "" {
		if err := sendViaHelper(helperPath, title, message); err == nil {
			logging.Trace("Send: sent via taw-notify helper")
			return nil
		}
		logging.Trace("Send: taw-notify helper failed, falling back to AppleScript")
	}

	// Fall back to AppleScript
	script := fmt.Sprintf(`display notification %q with title %q`, message, title)
	cmd := appleScriptCommand("-e", script)
	if err := cmd.Run(); err != nil {
		fallbackErr := exec.Command("osascript", "-e", script).Run()
		if fallbackErr == nil {
			return nil
		}
		return err
	}
	return nil
}

// sendViaHelper sends a simple notification using the taw-notify helper.
// Note: We don't pass icon as attachment because it would show on the right side
// of the notification banner, taking space from action buttons. The app bundle
// icon automatically shows on the left side.
func sendViaHelper(helperPath, title, message string) error {
	args := []string{
		"--title", title,
		"--body", message,
		"--timeout", "1", // Short timeout since we don't need to wait for response
	}

	// Run the helper via 'open' command to ensure proper bundle ID recognition
	openArgs := []string{
		"-W", // Wait for app to finish
		"-a", helperPath,
		"--args",
	}
	openArgs = append(openArgs, args...)

	cmd := exec.Command("open", openArgs...)
	return cmd.Run()
}

// FindIconPath locates the taw icon for notifications.
func FindIconPath() string {
	candidates := []string{}

	// ~/.local/share/taw/icon.png
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "share", "taw", "icon.png"))
	}

	// Same directory as current executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "icon.png"))
	}

	// Inside the app bundle's Resources
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "share", "taw",
			NotifyAppName, "Contents", "Resources", "icon.png"))
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			logging.Trace("findIconPath: found at %s", path)
			return path
		}
	}

	logging.Trace("findIconPath: not found")
	return ""
}

// PlaySound plays a system sound (macOS only).
// It runs in the background and does not block.
// Uses nohup to ensure the sound plays even if parent process exits.
func PlaySound(soundType SoundType) {
	logging.Trace("PlaySound: start soundType=%s", soundType)
	defer logging.Trace("PlaySound: end soundType=%s", soundType)

	if runtime.GOOS != "darwin" {
		return
	}

	soundPath := fmt.Sprintf("/System/Library/Sounds/%s.aiff", soundType)

	// Check if sound file exists
	if _, err := os.Stat(soundPath); os.IsNotExist(err) {
		logging.Trace("PlaySound: sound file not found path=%s", soundPath)
		return
	}

	// Run afplay via nohup to ensure it survives parent process exit
	// Redirect output to /dev/null to fully detach
	cmd := exec.Command("nohup", "afplay", soundPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		logging.Trace("PlaySound: failed to start afplay err=%v", err)
	}
}

func appleScriptCommand(args ...string) *exec.Cmd {
	if runtime.GOOS != "darwin" {
		return exec.Command("osascript", args...)
	}
	uid := os.Getuid()
	if uid > 0 {
		cmdArgs := append([]string{"asuser", fmt.Sprintf("%d", uid), "osascript"}, args...)
		return exec.Command("launchctl", cmdArgs...)
	}
	return exec.Command("osascript", args...)
}

// SendWithActions shows a notification with action buttons and returns the selected action index.
// Returns the 0-based index of the selected action, or -1 if dismissed/timed out/clicked without action.
// If the notification helper is not available, falls back to a simple notification and returns -1.
func SendWithActions(title, message, iconPath string, actions []string, timeoutSec int) (int, error) {
	logging.Trace("SendWithActions: start title=%q actions=%v timeout=%d", title, actions, timeoutSec)
	defer logging.Trace("SendWithActions: end")

	if runtime.GOOS != "darwin" {
		return -1, nil
	}

	// Find the notification helper
	helperPath := findNotifyHelper()
	if helperPath == "" {
		logging.Debug("SendWithActions: notification helper not found, falling back to simple notification")
		if err := Send(title, message); err != nil {
			return -1, err
		}
		return -1, nil
	}

	logging.Trace("SendWithActions: using helper at %s", helperPath)

	// Build command arguments
	args := []string{
		"--title", title,
		"--body", message,
		"--timeout", strconv.Itoa(timeoutSec),
	}
	if iconPath != "" {
		args = append(args, "--icon", iconPath)
	}
	for _, action := range actions {
		args = append(args, "--action", action)
	}

	// Run the helper via 'open' command to ensure proper bundle ID recognition
	openArgs := []string{
		"--stdout", "/dev/stdout",
		"--stderr", "/dev/stderr",
		"-W", // Wait for app to finish
		"-a", helperPath,
		"--args",
	}
	openArgs = append(openArgs, args...)

	cmd := exec.Command("open", openArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logging.Trace("SendWithActions: running command: open %v", openArgs)
	if err := cmd.Run(); err != nil {
		logging.Debug("SendWithActions: helper failed err=%v stderr=%s", err, stderr.String())
		// Fall back to simple notification
		if sendErr := Send(title, message); sendErr != nil {
			return -1, sendErr
		}
		return -1, nil
	}

	result := strings.TrimSpace(stdout.String())
	logging.Trace("SendWithActions: helper returned %q", result)

	// Parse result
	if strings.HasPrefix(result, "ACTION_") {
		indexStr := strings.TrimPrefix(result, "ACTION_")
		index, err := strconv.Atoi(indexStr)
		if err == nil && index >= 0 && index < len(actions) {
			return index, nil
		}
	}

	return -1, nil
}

// findNotifyHelper locates the taw-notify.app helper.
// It searches in the following order:
// 1. ~/.local/share/taw/taw-notify.app (installed location)
// 2. Same directory as the taw binary
// 3. Current working directory
func findNotifyHelper() string {
	candidates := []string{}

	// 1. ~/.local/share/taw/
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "share", "taw", NotifyAppName))
	}

	// 2. Same directory as taw binary
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, NotifyAppName))
	}

	// 3. Current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, NotifyAppName))
	}

	for _, path := range candidates {
		binaryPath := filepath.Join(path, "Contents", "MacOS", NotifyBinaryName)
		if _, err := os.Stat(binaryPath); err == nil {
			logging.Trace("findNotifyHelper: found at %s", path)
			return path
		}
	}

	logging.Trace("findNotifyHelper: not found, searched %v", candidates)
	return ""
}

// SendAll sends a notification to all configured channels.
// It sends to macOS desktop notification (if on macOS), Slack (if configured),
// and ntfy (if configured). Errors from individual channels are logged but
// do not prevent other channels from being notified.
func SendAll(notifications *config.NotificationsConfig, title, message string) {
	logging.Trace("SendAll: start title=%q", title)
	defer logging.Trace("SendAll: end")

	// Send macOS desktop notification (non-blocking, errors logged)
	if err := Send(title, message); err != nil {
		logging.Trace("SendAll: desktop notification failed: %v", err)
	}

	// Send to configured external channels
	if notifications == nil {
		return
	}

	// Send to Slack (non-blocking, errors logged)
	if notifications.Slack != nil {
		go func() {
			if err := SendSlack(notifications.Slack, title, message); err != nil {
				logging.Trace("SendAll: Slack notification failed: %v", err)
			}
		}()
	}

	// Send to ntfy (non-blocking, errors logged)
	if notifications.Ntfy != nil {
		go func() {
			if err := SendNtfy(notifications.Ntfy, title, message); err != nil {
				logging.Trace("SendAll: ntfy notification failed: %v", err)
			}
		}()
	}
}
