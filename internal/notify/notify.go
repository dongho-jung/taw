// Package notify provides simple desktop notifications.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
)

// Send shows a desktop notification when supported.
func Send(title, message string) error {
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
func PlaySound(soundType SoundType) {
	if runtime.GOOS != "darwin" {
		return
	}

	soundPath := fmt.Sprintf("/System/Library/Sounds/%s.aiff", soundType)

	// Check if sound file exists
	if _, err := os.Stat(soundPath); os.IsNotExist(err) {
		return
	}

	// Run afplay in background (non-blocking)
	cmd := exec.Command("afplay", soundPath)
	_ = cmd.Start()
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
