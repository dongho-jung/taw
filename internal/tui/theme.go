// Package tui provides terminal user interface components for PAW.
package tui

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
)

// DetectDarkMode returns whether the terminal is in dark mode.
// It checks the theme config setting first:
//   - "light": always returns false (dark mode = off)
//   - "dark": always returns true (dark mode = on)
//   - "auto" or empty: uses lipgloss.HasDarkBackground() to auto-detect
//
// This function should be called BEFORE bubbletea starts, as
// lipgloss.HasDarkBackground() reads from stdin.
func DetectDarkMode() bool {
	// Try to load theme from config
	theme := loadThemeFromConfig()

	switch theme {
	case config.ThemeLight:
		return false
	case config.ThemeDark:
		return true
	default:
		// Auto-detect with improved reliability
		return detectDarkModeWithRetry()
	}
}

// detectDarkModeWithRetry performs dark mode detection with multiple attempts
// to improve reliability. The OSC query to detect background color can be
// unreliable if called too early or if there's buffered input.
func detectDarkModeWithRetry() bool {
	// Flush stdout to ensure terminal is in a clean state
	os.Stdout.Sync()

	// Small delay to let terminal settle after any previous output
	time.Sleep(5 * time.Millisecond)

	// Try detection multiple times and use majority vote
	// This helps with intermittent detection failures
	const attempts = 3
	darkCount := 0

	for i := range attempts {
		if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
			darkCount++
		}
		// Small delay between attempts (except after last)
		if i < attempts-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Use majority vote: if 2+ out of 3 say dark, return dark
	return darkCount >= 2
}

// loadThemeFromConfig attempts to load the theme setting from .paw/config.
// Returns ThemeAuto if the config cannot be loaded.
func loadThemeFromConfig() config.Theme {
	// Find .paw directory
	pawDir := findPawDir()
	if pawDir == "" {
		return config.ThemeAuto
	}

	cfg, err := config.Load(pawDir)
	if err != nil {
		return config.ThemeAuto
	}

	if cfg.Theme == "" {
		return config.ThemeAuto
	}

	return cfg.Theme
}

// findPawDir looks for .paw directory starting from current dir up to root.
func findPawDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		pawDir := filepath.Join(dir, constants.PawDirName)
		if info, err := os.Stat(pawDir); err == nil && info.IsDir() {
			return pawDir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}
