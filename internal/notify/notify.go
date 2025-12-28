// Package notify provides simple desktop notifications.
package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Send shows a desktop notification when supported.
func Send(title, message string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	script := fmt.Sprintf(`display notification %q with title %q`, message, title)
	return exec.Command("osascript", "-e", script).Run()
}
