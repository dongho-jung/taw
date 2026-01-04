package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/notify"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check dependencies and system requirements",
	Long:  "Verify that all required and optional dependencies are installed and configured correctly.",
	RunE:  runCheck,
}

// checkResult holds the result of a single dependency check.
type checkResult struct {
	name     string
	ok       bool
	message  string
	required bool
}

// runCheck runs all dependency checks and prints the results.
func runCheck(cmd *cobra.Command, args []string) error {
	fmt.Println("PAW Dependency Check")
	fmt.Println("====================")
	fmt.Println()

	results := []checkResult{
		checkTmux(),
		checkClaude(),
		checkGit(),
		checkGh(),
	}

	// macOS-specific checks
	if runtime.GOOS == "darwin" {
		results = append(results, checkNotifyApp())
		results = append(results, checkNotificationPermission())
		results = append(results, checkSounds())
	}

	// Print results
	hasErrors := false
	for _, r := range results {
		printResult(r)
		if r.required && !r.ok {
			hasErrors = true
		}
	}

	fmt.Println()
	if hasErrors {
		fmt.Println("❌ Some required dependencies are missing. Please install them before using PAW.")
		return fmt.Errorf("required dependencies missing")
	}
	fmt.Println("✅ All required dependencies are available.")
	return nil
}

// printResult prints a single check result with appropriate formatting.
func printResult(r checkResult) {
	var icon string
	if r.ok {
		icon = "✅"
	} else if r.required {
		icon = "❌"
	} else {
		icon = "⚠️ "
	}

	optionalSuffix := ""
	if !r.required && !r.ok {
		optionalSuffix = " (optional)"
	}

	fmt.Printf("%s %s: %s%s\n", icon, r.name, r.message, optionalSuffix)
}

// checkTmux verifies tmux is installed and returns its version.
func checkTmux() checkResult {
	result := checkResult{name: "tmux", required: true}

	output, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		result.ok = false
		result.message = "not installed"
		return result
	}

	version := strings.TrimSpace(string(output))
	result.ok = true
	result.message = fmt.Sprintf("installed (%s)", version)
	return result
}

// checkClaude verifies the Claude CLI is installed.
func checkClaude() checkResult {
	result := checkResult{name: "claude", required: true}

	output, err := exec.Command("claude", "--version").Output()
	if err != nil {
		result.ok = false
		result.message = "not installed - install from https://claude.ai/claude-code"
		return result
	}

	version := strings.TrimSpace(string(output))
	// Claude version output might be multi-line, take first line
	if idx := strings.Index(version, "\n"); idx != -1 {
		version = version[:idx]
	}
	result.ok = true
	result.message = fmt.Sprintf("installed (%s)", version)
	return result
}

// checkGit verifies git is installed and returns its version.
func checkGit() checkResult {
	result := checkResult{name: "git", required: false}

	output, err := exec.Command("git", "--version").Output()
	if err != nil {
		result.ok = false
		result.message = "not installed - needed for worktree mode"
		return result
	}

	version := strings.TrimSpace(string(output))
	// Extract just the version number from "git version X.Y.Z"
	version = strings.TrimPrefix(version, "git version ")
	result.ok = true
	result.message = fmt.Sprintf("installed (v%s)", version)
	return result
}

// checkGh verifies the GitHub CLI is installed.
func checkGh() checkResult {
	result := checkResult{name: "gh", required: false}

	output, err := exec.Command("gh", "--version").Output()
	if err != nil {
		result.ok = false
		result.message = "not installed - needed for PR creation"
		return result
	}

	version := strings.TrimSpace(string(output))
	// gh --version output is "gh version X.Y.Z (date)"
	if idx := strings.Index(version, "\n"); idx != -1 {
		version = version[:idx]
	}
	version = strings.TrimPrefix(version, "gh version ")
	if idx := strings.Index(version, " "); idx != -1 {
		version = version[:idx]
	}
	result.ok = true
	result.message = fmt.Sprintf("installed (v%s)", version)
	return result
}

// checkNotifyApp verifies the paw-notify.app is installed.
func checkNotifyApp() checkResult {
	result := checkResult{name: "paw-notify.app", required: false}

	// Check in ~/.local/share/paw/
	home, err := os.UserHomeDir()
	if err != nil {
		result.ok = false
		result.message = "could not determine home directory"
		return result
	}

	appPath := filepath.Join(home, ".local", "share", "paw", notify.NotifyAppName)
	binaryPath := filepath.Join(appPath, "Contents", "MacOS", notify.NotifyBinaryName)

	if _, err := os.Stat(binaryPath); err != nil {
		result.ok = false
		result.message = "not installed - run 'make install' to install"
		return result
	}

	result.ok = true
	result.message = "installed"
	return result
}

// checkNotificationPermission verifies notification permissions are granted.
func checkNotificationPermission() checkResult {
	result := checkResult{name: "notifications", required: false}

	// Check if we can find the notification helper
	home, err := os.UserHomeDir()
	if err != nil {
		result.ok = false
		result.message = "could not determine home directory"
		return result
	}

	appPath := filepath.Join(home, ".local", "share", "paw", notify.NotifyAppName)
	if _, err := os.Stat(appPath); err != nil {
		result.ok = false
		result.message = "paw-notify.app not installed (install it first)"
		return result
	}

	// Since we can't directly query notification permissions without running the app,
	// we provide guidance on how to check permissions.
	// User should verify via System Settings > Notifications > PAW Notify
	result.ok = true
	result.message = "verify in System Settings > Notifications > PAW Notify"
	return result
}

// checkSounds verifies system sounds are available.
func checkSounds() checkResult {
	result := checkResult{name: "sounds", required: false}

	sounds := []string{
		string(notify.SoundTaskCreated),
		string(notify.SoundTaskCompleted),
		string(notify.SoundNeedInput),
		string(notify.SoundError),
		string(notify.SoundCancelPending),
	}

	soundDir := "/System/Library/Sounds"
	missing := []string{}

	for _, sound := range sounds {
		soundPath := filepath.Join(soundDir, sound+".aiff")
		if _, err := os.Stat(soundPath); err != nil {
			missing = append(missing, sound)
		}
	}

	if len(missing) > 0 {
		result.ok = false
		result.message = fmt.Sprintf("missing: %s", strings.Join(missing, ", "))
		return result
	}

	result.ok = true
	result.message = "all available"
	return result
}
