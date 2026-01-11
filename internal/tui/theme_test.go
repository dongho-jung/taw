package tui

import (
	"testing"

	"github.com/dongho-jung/paw/internal/config"
)

func TestDetectDarkMode_CachesBehavior(t *testing.T) {
	// Reset cache to ensure clean state
	cachedDarkMode.Store(darkModeUnknown)

	// Test explicit light theme
	t.Run("Light theme returns false", func(t *testing.T) {
		result := detectDarkMode(config.ThemeLight)
		if result {
			t.Errorf("expected false for light theme, got true")
		}
	})

	// Test explicit dark theme
	t.Run("Dark theme returns true", func(t *testing.T) {
		result := detectDarkMode(config.ThemeDark)
		if !result {
			t.Errorf("expected true for dark theme, got false")
		}
	})

	// Test cache consistency
	t.Run("Cache is set after explicit theme", func(t *testing.T) {
		// Reset cache
		cachedDarkMode.Store(darkModeUnknown)

		// Set explicit dark mode
		setCachedDarkMode(true)

		// Verify cache is set
		isDark, ok := cachedDarkModeValue()
		if !ok {
			t.Error("expected cache to be set")
		}
		if !isDark {
			t.Errorf("expected isDark=true from cache, got false")
		}
	})

	// Test cache returns correct light value
	t.Run("Cache returns light mode correctly", func(t *testing.T) {
		cachedDarkMode.Store(darkModeUnknown)
		setCachedDarkMode(false)

		isDark, ok := cachedDarkModeValue()
		if !ok {
			t.Error("expected cache to be set")
		}
		if isDark {
			t.Errorf("expected isDark=false from cache, got true")
		}
	})

	// Test cache returns correct dark value
	t.Run("Cache returns dark mode correctly", func(t *testing.T) {
		cachedDarkMode.Store(darkModeUnknown)
		setCachedDarkMode(true)

		isDark, ok := cachedDarkModeValue()
		if !ok {
			t.Error("expected cache to be set")
		}
		if !isDark {
			t.Errorf("expected isDark=true from cache, got false")
		}
	})

	// Test that auto mode uses cache when available
	t.Run("Auto mode uses cached value", func(t *testing.T) {
		// Set cache to light
		cachedDarkMode.Store(darkModeUnknown)
		setCachedDarkMode(false)

		// Auto mode should use cached value
		result := detectDarkMode(config.ThemeAuto)
		if result {
			t.Errorf("expected false (light) from cached auto detection, got true")
		}

		// Set cache to dark
		setCachedDarkMode(true)

		// Auto mode should use updated cached value
		result = detectDarkMode(config.ThemeAuto)
		if !result {
			t.Errorf("expected true (dark) from cached auto detection, got false")
		}
	})

	// Clean up: reset cache
	cachedDarkMode.Store(darkModeUnknown)
}

func TestSetCachedDarkMode(t *testing.T) {
	// Reset to ensure clean state
	cachedDarkMode.Store(darkModeUnknown)

	t.Run("Set to dark", func(t *testing.T) {
		setCachedDarkMode(true)
		val := cachedDarkMode.Load()
		if val != darkModeDark {
			t.Errorf("expected darkModeDark, got %d", val)
		}
	})

	t.Run("Set to light", func(t *testing.T) {
		setCachedDarkMode(false)
		val := cachedDarkMode.Load()
		if val != darkModeLight {
			t.Errorf("expected darkModeLight, got %d", val)
		}
	})

	// Clean up
	cachedDarkMode.Store(darkModeUnknown)
}

func TestCachedDarkModeValue(t *testing.T) {
	t.Run("Unknown returns false, false", func(t *testing.T) {
		cachedDarkMode.Store(darkModeUnknown)
		isDark, ok := cachedDarkModeValue()
		if ok {
			t.Error("expected ok=false for unknown cache")
		}
		if isDark {
			t.Error("expected isDark=false for unknown cache")
		}
	})

	t.Run("Light returns false, true", func(t *testing.T) {
		cachedDarkMode.Store(darkModeLight)
		isDark, ok := cachedDarkModeValue()
		if !ok {
			t.Error("expected ok=true for light cache")
		}
		if isDark {
			t.Error("expected isDark=false for light cache")
		}
	})

	t.Run("Dark returns true, true", func(t *testing.T) {
		cachedDarkMode.Store(darkModeDark)
		isDark, ok := cachedDarkModeValue()
		if !ok {
			t.Error("expected ok=true for dark cache")
		}
		if !isDark {
			t.Error("expected isDark=true for dark cache")
		}
	})

	// Clean up
	cachedDarkMode.Store(darkModeUnknown)
}

func TestResetDarkModeCache(t *testing.T) {
	t.Run("Reset clears dark mode cache", func(t *testing.T) {
		// Set cache to dark
		setCachedDarkMode(true)
		isDark, ok := cachedDarkModeValue()
		if !ok || !isDark {
			t.Error("expected cache to be set to dark before reset")
		}

		// Reset cache
		ResetDarkModeCache()

		// Verify cache is unknown
		_, ok = cachedDarkModeValue()
		if ok {
			t.Error("expected cache to be unknown after reset")
		}
	})

	t.Run("Reset clears light mode cache", func(t *testing.T) {
		// Set cache to light
		setCachedDarkMode(false)
		isDark, ok := cachedDarkModeValue()
		if !ok || isDark {
			t.Error("expected cache to be set to light before reset")
		}

		// Reset cache
		ResetDarkModeCache()

		// Verify cache is unknown
		_, ok = cachedDarkModeValue()
		if ok {
			t.Error("expected cache to be unknown after reset")
		}
	})

	// Clean up
	cachedDarkMode.Store(darkModeUnknown)
}
