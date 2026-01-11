// Package tui provides terminal user interface components for PAW.
package tui

import (
	"image/color"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
)

// ThemeColors provides a consistent color palette for TUI components.
// Colors are calculated based on whether the terminal is in dark or light mode.
type ThemeColors struct {
	isDark bool

	// Primary colors
	Accent          color.Color // Primary accent color (blue tones)
	AccentSecondary color.Color // Secondary accent (cyan/green tones)

	// Text colors
	TextNormal   color.Color // Normal text
	TextDim      color.Color // Dimmed/secondary text
	TextBright   color.Color // Bright/emphasized text
	TextInverted color.Color // Text on colored backgrounds

	// UI element colors
	Border         color.Color // Border color
	BorderFocused  color.Color // Focused border color
	BorderDim      color.Color // Dimmed border color
	Background     color.Color // Background for input fields etc.
	BackgroundAlt  color.Color // Alternate background
	Selection      color.Color // Selection/highlight background
	StatusBar      color.Color // Status bar background
	StatusBarText  color.Color // Status bar text
	SuccessColor   color.Color // Success indicator
	WarningColor   color.Color // Warning indicator
	ErrorColor     color.Color // Error indicator
	SearchMatch    color.Color // Search match highlight
	SearchCurrent  color.Color // Current search match
}

// NewThemeColors creates a new color palette based on dark mode setting.
func NewThemeColors(isDark bool) ThemeColors {
	if isDark {
		return ThemeColors{
			isDark: true,

			// Primary colors - blue/cyan for dark backgrounds
			Accent:          lipgloss.Color("39"),  // Bright cyan-blue
			AccentSecondary: lipgloss.Color("86"),  // Bright cyan

			// Text colors
			TextNormal:   lipgloss.Color("252"), // Light gray
			TextDim:      lipgloss.Color("240"), // Medium gray
			TextBright:   lipgloss.Color("255"), // White
			TextInverted: lipgloss.Color("232"), // Near black

			// UI element colors
			Border:         lipgloss.Color("240"), // Medium gray
			BorderFocused:  lipgloss.Color("39"),  // Cyan-blue
			BorderDim:      lipgloss.Color("236"), // Dark gray
			Background:     lipgloss.Color("235"), // Dark gray
			BackgroundAlt:  lipgloss.Color("237"), // Slightly lighter
			Selection:      lipgloss.Color("25"),  // Dark blue
			StatusBar:      lipgloss.Color("240"), // Medium gray
			StatusBarText:  lipgloss.Color("252"), // Light gray
			SuccessColor:   lipgloss.Color("42"),  // Green
			WarningColor:   lipgloss.Color("214"), // Orange
			ErrorColor:     lipgloss.Color("196"), // Red
			SearchMatch:    lipgloss.Color("226"), // Yellow
			SearchCurrent:  lipgloss.Color("208"), // Orange
		}
	}

	// Light mode colors
	return ThemeColors{
		isDark: false,

		// Primary colors - darker blue for light backgrounds
		Accent:          lipgloss.Color("25"),  // Dark blue
		AccentSecondary: lipgloss.Color("30"),  // Teal

		// Text colors
		TextNormal:   lipgloss.Color("236"), // Dark gray
		TextDim:      lipgloss.Color("245"), // Medium gray
		TextBright:   lipgloss.Color("232"), // Near black
		TextInverted: lipgloss.Color("255"), // White

		// UI element colors
		Border:         lipgloss.Color("250"), // Light gray
		BorderFocused:  lipgloss.Color("25"),  // Dark blue
		BorderDim:      lipgloss.Color("252"), // Very light gray
		Background:     lipgloss.Color("254"), // Near white
		BackgroundAlt:  lipgloss.Color("253"), // Slightly darker
		Selection:      lipgloss.Color("153"), // Light blue
		StatusBar:      lipgloss.Color("250"), // Light gray
		StatusBarText:  lipgloss.Color("236"), // Dark gray
		SuccessColor:   lipgloss.Color("28"),  // Dark green
		WarningColor:   lipgloss.Color("166"), // Dark orange
		ErrorColor:     lipgloss.Color("160"), // Dark red
		SearchMatch:    lipgloss.Color("220"), // Yellow
		SearchCurrent:  lipgloss.Color("208"), // Orange
	}
}

// IsDark returns whether this is a dark theme.
func (tc ThemeColors) IsDark() bool {
	return tc.isDark
}

// LightDark returns a lipgloss.LightDark function bound to this theme.
func (tc ThemeColors) LightDark() func(color.Color, color.Color) color.Color {
	return lipgloss.LightDark(tc.isDark)
}

const (
	darkModeUnknown int32 = iota
	darkModeLight
	darkModeDark
)

var cachedDarkMode atomic.Int32

// DetectDarkMode returns whether the terminal is in dark mode.
// It checks the theme config setting first:
//   - "light": always returns false (dark mode = off)
//   - "dark": always returns true (dark mode = on)
//   - "auto" or empty: uses lipgloss.HasDarkBackground() to auto-detect
//
// This function should be called BEFORE bubbletea starts, as
// lipgloss.HasDarkBackground() reads from stdin.
func DetectDarkMode() bool {
	theme := loadThemeFromConfig()
	return detectDarkMode(theme)
}

func detectDarkMode(theme config.Theme) bool {
	switch theme {
	case config.ThemeLight:
		return false
	case config.ThemeDark:
		return true
	default:
		if isDark, ok := cachedDarkModeValue(); ok {
			return isDark
		}
		// Auto-detect with improved reliability
		return detectDarkModeWithRetry()
	}
}

func cachedDarkModeValue() (bool, bool) {
	switch cachedDarkMode.Load() {
	case darkModeDark:
		return true, true
	case darkModeLight:
		return false, true
	default:
		return false, false
	}
}

func setCachedDarkMode(isDark bool) {
	if isDark {
		cachedDarkMode.Store(darkModeDark)
		return
	}
	cachedDarkMode.Store(darkModeLight)
}

// ResetDarkModeCache clears the cached dark mode value, forcing re-detection
// on the next call to DetectDarkMode(). This should be called before starting
// a new TUI to ensure the terminal's current theme is detected, especially
// when the user may have attached from a different terminal or switched themes.
func ResetDarkModeCache() {
	cachedDarkMode.Store(darkModeUnknown)
}

// detectDarkModeWithRetry performs dark mode detection with multiple attempts
// to improve reliability. The OSC query to detect background color can be
// unreliable if called too early or if there's buffered input.
func detectDarkModeWithRetry() bool {
	// Flush stdout to ensure terminal is in a clean state
	_ = os.Stdout.Sync()

	// Small delay to let terminal settle after any previous output
	time.Sleep(5 * time.Millisecond)

	// Try detection multiple times and use majority vote
	// This helps with intermittent detection failures
	const attempts = 3
	darkCount := 0
	results := make([]bool, attempts)

	for i := range attempts {
		result := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
		results[i] = result
		if result {
			darkCount++
		}
		// Small delay between attempts (except after last)
		if i < attempts-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Use majority vote: if 2+ out of 3 say dark, return dark
	isDark := darkCount >= 2

	// Theme detection result is cached, no stderr output to avoid TUI interference

	// Cache the result for consistency across all TUI components.
	// This ensures components created before bubbletea starts use the same
	// detection result. bubbletea's BackgroundColorMsg can still override
	// this with a more accurate detection later.
	setCachedDarkMode(isDark)

	return isDark
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
