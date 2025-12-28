// Package notify provides simple desktop notifications.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
