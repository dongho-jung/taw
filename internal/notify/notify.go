// Package notify provides simple desktop notifications.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/donghojung/taw/internal/logging"
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
func Send(title, message string) error {
	logging.Trace("Send: start title=%q message=%q", title, message)
	defer logging.Trace("Send: end title=%q", title)

	if runtime.GOOS != "darwin" {
		return nil
	}

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
