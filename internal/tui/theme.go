// Package tui provides terminal user interface components for PAW.
package tui

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/dongho-jung/paw/internal/constants"
)

// paddingCache stores pre-computed padding strings to avoid repeated allocations.
// Common terminal widths (80, 120, 160, 200) are pre-computed.
var paddingCache struct {
	sync.RWMutex
	cache map[int]string
}

func init() {
	paddingCache.cache = make(map[int]string, 16)
	// Pre-compute common widths
	for _, w := range []int{80, 100, 120, 140, 160, 180, 200} {
		paddingCache.cache[w] = strings.Repeat(" ", w)
	}
}

// getPadding returns a string of n spaces, using cache for common widths.
func getPadding(n int) string {
	if n <= 0 {
		return ""
	}

	// Fast path: check cache without lock
	paddingCache.RLock()
	if s, ok := paddingCache.cache[n]; ok {
		paddingCache.RUnlock()
		return s
	}
	paddingCache.RUnlock()

	// Slow path: generate and cache
	s := strings.Repeat(" ", n)

	// Only cache reasonable widths to avoid memory bloat
	if n <= 300 {
		paddingCache.Lock()
		paddingCache.cache[n] = s
		paddingCache.Unlock()
	}

	return s
}

// buildSearchBar creates a search bar string with proper padding.
// This is more efficient than fmt.Sprintf("/%-*s", width, input) for hot paths.
func buildSearchBar(input string, width int) string {
	prefix := "/" + input
	prefixLen := len(prefix)

	if prefixLen >= width {
		return prefix[:width]
	}

	return prefix + getPadding(width-prefixLen)
}

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

	// Log level colors
	LogTrace color.Color // L0 - Trace level (least important)
	LogDebug color.Color // L1 - Debug level
	LogInfo  color.Color // L2 - Info level
	LogWarn  color.Color // L3 - Warning level
	LogError color.Color // L4 - Error level
	LogFatal color.Color // L5 - Fatal level (most critical)
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
			TextDim:      lipgloss.Color("243"), // Medium gray (better contrast on dark backgrounds)
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

			// Log level colors - distinct colors for dark backgrounds
			LogTrace: lipgloss.Color("243"), // Gray (dim, least important)
			LogDebug: lipgloss.Color("75"),  // Light blue
			LogInfo:  lipgloss.Color("114"), // Green
			LogWarn:  lipgloss.Color("214"), // Orange/yellow
			LogError: lipgloss.Color("196"), // Bright red
			LogFatal: lipgloss.Color("201"), // Magenta (most critical)
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
		TextDim:      lipgloss.Color("245"), // Medium gray (good contrast on light backgrounds)
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

		// Log level colors - distinct colors for light backgrounds
		LogTrace: lipgloss.Color("247"), // Gray (dim, least important)
		LogDebug: lipgloss.Color("25"),  // Dark blue
		LogInfo:  lipgloss.Color("28"),  // Dark green
		LogWarn:  lipgloss.Color("166"), // Dark orange
		LogError: lipgloss.Color("160"), // Dark red
		LogFatal: lipgloss.Color("125"), // Dark magenta (most critical)
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

// LogLevelColor returns the color for a given log level (0-5).
// Returns nil for invalid levels.
func (tc ThemeColors) LogLevelColor(level int) color.Color {
	switch level {
	case 0:
		return tc.LogTrace
	case 1:
		return tc.LogDebug
	case 2:
		return tc.LogInfo
	case 3:
		return tc.LogWarn
	case 4:
		return tc.LogError
	case 5:
		return tc.LogFatal
	default:
		return nil
	}
}

const (
	darkModeUnknown int32 = iota
	darkModeLight
	darkModeDark
)

var cachedDarkMode atomic.Int32

// DetectDarkMode returns whether the terminal is in dark mode.
// It auto-detects based on terminal background color using lipgloss.
//
// This function should be called BEFORE bubbletea starts, as
// lipgloss.HasDarkBackground() reads from stdin.
func DetectDarkMode() bool {
	if isDark, ok := cachedDarkModeValue(); ok {
		return isDark
	}
	// Auto-detect with improved reliability
	return detectDarkModeWithRetry()
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

// detectDarkModeWithRetry performs dark mode detection with multiple methods
// for improved reliability.
func detectDarkModeWithRetry() bool {
	// Method 1: Check COLORFGBG environment variable (fast, no I/O)
	// Format: "fg;bg" where bg > 6 typically means light background
	if colorfgbg := os.Getenv("COLORFGBG"); colorfgbg != "" {
		parts := strings.Split(colorfgbg, ";")
		if len(parts) >= 2 {
			bg := 0
			for _, ch := range parts[len(parts)-1] {
				if ch >= '0' && ch <= '9' {
					bg = bg*10 + int(ch-'0')
				} else {
					break
				}
			}
			if parts[len(parts)-1] != "" {
				// Background colors: 0-6 are dark, 7-15 are mixed, higher depends
				isDark := bg <= 6 || (bg >= 8 && bg <= 14)
				setCachedDarkMode(isDark)
				return isDark
			}
		}
	}

	// Method 2: Try OSC 11 query via lipgloss with optimized handling
	// Reduced attempts and timeouts for faster response
	const attempts = 2
	darkCount := 0
	validCount := 0

	for i := 0; i < attempts; i++ {
		// Use a goroutine with timeout to avoid hanging
		resultCh := make(chan bool, 1)
		go func() {
			resultCh <- lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
		}()

		select {
		case result := <-resultCh:
			validCount++
			if result {
				darkCount++
			}
		case <-time.After(25 * time.Millisecond):
			// Timeout - detection failed for this attempt
		}
	}

	// If we got at least one valid response, use it
	if validCount >= 1 {
		isDark := darkCount > validCount/2
		setCachedDarkMode(isDark)
		return isDark
	}

	// Method 3: Check if we're in a tmux session
	if os.Getenv("TMUX") != "" {
		// Inside tmux, OSC queries might not work reliably
		// Check terminal type
		term := os.Getenv("TERM_PROGRAM")
		if strings.Contains(strings.ToLower(term), "apple_terminal") {
			// Apple Terminal default is light
			setCachedDarkMode(false)
			return false
		}
	}

	// Default fallback: assume dark mode (most common for terminal users)
	setCachedDarkMode(true)
	return true
}

// findPawDir searches for the .paw directory starting from the current
// working directory and going up to parent directories.
// Returns the path to .paw directory or empty string if not found.
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
			// Reached root
			break
		}
		dir = parent
	}

	return ""
}
