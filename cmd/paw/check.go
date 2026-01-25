package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/notify"
)

// soundsToCheck is the list of sound types to verify for macOS.
var soundsToCheck = []string{
	string(notify.SoundTaskCreated),
	string(notify.SoundTaskCompleted),
	string(notify.SoundNeedInput),
	string(notify.SoundError),
	string(notify.SoundCancelPending),
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check dependencies and project health",
	Long:  "Verify required dependencies and project/session health for the current directory.",
	RunE:  runCheck,
}

var checkFix bool

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "Attempt to install missing dependencies (Homebrew only)")
}

// checkResult holds the result of a single dependency check.
type checkResult struct {
	name     string
	ok       bool
	message  string
	required bool
	fix      func() error
}

// runCheck runs all dependency checks and prints the results.
func runCheck(_ *cobra.Command, _ []string) error {
	fmt.Println("PAW Check")
	fmt.Println("=========")
	fmt.Println()

	depResults := collectCheckResults()
	projectResults := collectProjectResults()

	printCheckSections(depResults, projectResults)
	hasErrors := hasRequiredErrors(depResults, projectResults)

	if checkFix {
		fixResults := applyFixes(append([]checkResult{}, append(depResults, projectResults...)...))
		if len(fixResults) > 0 {
			fmt.Println()
			fmt.Println("Fixes:")
			for _, r := range fixResults {
				printResult(r)
			}
		}

		fmt.Println()
		fmt.Println("Recheck:")
		depResults = collectCheckResults()
		projectResults = collectProjectResults()
		printCheckSections(depResults, projectResults)
		hasErrors = hasRequiredErrors(depResults, projectResults)
	}

	fmt.Println()
	if hasErrors {
		fmt.Println("❌ Some required dependencies are missing. Please install them before using PAW.")
		return errors.New("required dependencies missing")
	}
	fmt.Println("✅ All required dependencies are available.")
	return nil
}

func printCheckSections(depResults, projectResults []checkResult) {
	fmt.Println("Dependencies")
	fmt.Println("------------")
	for _, r := range depResults {
		printResult(r)
	}

	if len(projectResults) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("Project")
	fmt.Println("-------")
	for _, r := range projectResults {
		printResult(r)
	}
}

func hasRequiredErrors(depResults, projectResults []checkResult) bool {
	for _, r := range depResults {
		if r.required && !r.ok {
			return true
		}
	}
	for _, r := range projectResults {
		if r.required && !r.ok {
			return true
		}
	}
	return false
}

func collectCheckResults() []checkResult {
	results := []checkResult{
		checkTmux(),
		checkClaude(),
		checkGit(),
		checkGh(),
	}

	// macOS-specific checks
	if runtime.GOOS == "darwin" {
		results = append(results, checkSounds())
	}

	return results
}

// printResult prints a single check result with appropriate formatting.
func printResult(r checkResult) {
	var icon string
	switch {
	case r.ok:
		icon = "✅"
	case r.required:
		icon = "❌"
	default:
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
		if brewAvailable() {
			result.fix = func() error {
				return brewInstall("tmux")
			}
		}
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
		if brewAvailable() {
			result.fix = func() error {
				return brewInstall("git")
			}
		}
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
		if brewAvailable() {
			result.fix = func() error {
				return brewInstall("gh")
			}
		}
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

// checkSounds verifies system sounds are available.
func checkSounds() checkResult {
	result := checkResult{name: "sounds", required: false}

	soundDir := "/System/Library/Sounds"
	var missing []string

	for _, sound := range soundsToCheck {
		soundPath := filepath.Join(soundDir, sound+".aiff")
		if _, err := os.Stat(soundPath); err != nil {
			missing = append(missing, sound)
		}
	}

	if len(missing) > 0 {
		result.ok = false
		result.message = "missing: " + strings.Join(missing, ", ")
		return result
	}

	result.ok = true
	result.message = "all available"
	return result
}

func applyFixes(results []checkResult) []checkResult {
	fixes := make([]checkResult, 0, len(results))
	for _, r := range results {
		if r.ok || r.fix == nil {
			continue
		}
		fixResult := checkResult{name: r.name + " fix", required: false}
		if err := r.fix(); err != nil {
			fixResult.ok = false
			fixResult.message = err.Error()
		} else {
			fixResult.ok = true
			fixResult.message = "applied"
		}
		fixes = append(fixes, fixResult)
	}
	return fixes
}

func brewAvailable() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

func brewInstall(pkg string) error {
	cmd := exec.Command("brew", "install", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
