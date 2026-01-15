// Package tui provides terminal user interface components for PAW.
package tui

import (
	"testing"
)

func TestBuildMetadataString(t *testing.T) {
	tests := []struct {
		name     string
		duration string
		tokens   string
		expected string
	}{
		{
			name:     "both duration and tokens",
			duration: "1m 36s",
			tokens:   "↓ 5.9k",
			expected: "1m 36s · ↓ 5.9k",
		},
		{
			name:     "only duration",
			duration: "54s",
			tokens:   "",
			expected: "54s",
		},
		{
			name:     "only tokens",
			duration: "",
			tokens:   "↓ 2.7k",
			expected: "↓ 2.7k",
		},
		{
			name:     "neither duration nor tokens",
			duration: "",
			tokens:   "",
			expected: "",
		},
		{
			name:     "longer duration format",
			duration: "2h 15m",
			tokens:   "↓ 100k",
			expected: "2h 15m · ↓ 100k",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMetadataString(tt.duration, tt.tokens)
			if result != tt.expected {
				t.Errorf("buildMetadataString(%q, %q) = %q, want %q", tt.duration, tt.tokens, result, tt.expected)
			}
		})
	}
}
