package tui

import (
	"testing"
)

func TestDetectDarkMode_CachesBehavior(t *testing.T) {
	// Reset cache to ensure clean state
	cachedDarkMode.Store(darkModeUnknown)

	// Test cache consistency
	t.Run("Cache is set after explicit set", func(t *testing.T) {
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

	// Test that DetectDarkMode uses cache when available
	t.Run("DetectDarkMode uses cached value", func(t *testing.T) {
		// Set cache to light
		cachedDarkMode.Store(darkModeUnknown)
		setCachedDarkMode(false)

		// DetectDarkMode should use cached value
		result := DetectDarkMode()
		if result {
			t.Errorf("expected false (light) from cached detection, got true")
		}

		// Set cache to dark
		setCachedDarkMode(true)

		// DetectDarkMode should use updated cached value
		result = DetectDarkMode()
		if !result {
			t.Errorf("expected true (dark) from cached detection, got false")
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

func TestIsMatchLine(t *testing.T) {
	tests := []struct {
		name     string
		matchSet map[int]struct{}
		idx      int
		want     bool
	}{
		{"nil set returns false", nil, 5, false},
		{"empty set returns false", map[int]struct{}{}, 5, false},
		{"index in set returns true", map[int]struct{}{3: {}, 5: {}, 7: {}}, 5, true},
		{"index not in set returns false", map[int]struct{}{3: {}, 7: {}}, 5, false},
		{"zero index in set", map[int]struct{}{0: {}}, 0, true},
		{"negative index not in set", map[int]struct{}{1: {}}, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMatchLine(tt.matchSet, tt.idx)
			if got != tt.want {
				t.Errorf("isMatchLine(%v, %d) = %v, want %v", tt.matchSet, tt.idx, got, tt.want)
			}
		})
	}
}

func TestIsCurrentMatchLine(t *testing.T) {
	tests := []struct {
		name            string
		searchMatches   []int
		currentMatchIdx int
		idx             int
		want            bool
	}{
		{"empty matches returns false", []int{}, 0, 5, false},
		{"nil matches returns false", nil, 0, 5, false},
		{"currentMatchIdx out of bounds returns false", []int{3, 5, 7}, 5, 5, false},
		{"matching index returns true", []int{3, 5, 7}, 1, 5, true},
		{"non-matching index returns false", []int{3, 5, 7}, 1, 3, false},
		{"first match", []int{3, 5, 7}, 0, 3, true},
		{"last match", []int{3, 5, 7}, 2, 7, true},
		{"negative currentMatchIdx returns false", []int{3, 5, 7}, -1, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCurrentMatchLine(tt.searchMatches, tt.currentMatchIdx, tt.idx)
			if got != tt.want {
				t.Errorf("isCurrentMatchLine(%v, %d, %d) = %v, want %v",
					tt.searchMatches, tt.currentMatchIdx, tt.idx, got, tt.want)
			}
		})
	}
}

func TestBuildSearchBar(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{"empty input", "", 10, "/         "},
		{"short input", "test", 10, "/test     "},
		{"exact width", "test", 5, "/test"},
		{"input exceeds width", "testing", 5, "/test"},
		{"zero width", "test", 0, ""},
		{"negative width", "test", -1, ""},
		{"single char width", "test", 1, "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSearchBar(tt.input, tt.width)
			if got != tt.want {
				t.Errorf("buildSearchBar(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
			}
			// Verify length when width > 0
			if tt.width > 0 && len(got) != tt.width {
				t.Errorf("buildSearchBar(%q, %d) length = %d, want %d", tt.input, tt.width, len(got), tt.width)
			}
		})
	}
}

func TestGetPadding(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{"zero", 0, ""},
		{"negative", -5, ""},
		{"small", 3, "   "},
		{"common width 80", 80, "                                                                                "},
		{"large", 100, "                                                                                                    "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPadding(tt.n)
			if got != tt.want {
				t.Errorf("getPadding(%d) = %q (len=%d), want %q (len=%d)", tt.n, got, len(got), tt.want, len(tt.want))
			}
		})
	}

	// Test caching behavior - same width should return same reference
	t.Run("caching", func(t *testing.T) {
		// First call creates the entry
		first := getPadding(42)
		// Second call should use cache
		second := getPadding(42)
		if first != second {
			t.Error("expected cached padding to be identical")
		}
	})
}
