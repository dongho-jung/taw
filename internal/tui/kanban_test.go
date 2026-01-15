// Package tui provides terminal user interface components for PAW.
package tui

import (
	"testing"
)

func TestCalculateActionLinesPerTask(t *testing.T) {
	tests := []struct {
		name          string
		contentHeight int
		taskCount     int
		expected      int
	}{
		{
			name:          "no tasks returns 1",
			contentHeight: 20,
			taskCount:     0,
			expected:      1,
		},
		{
			name:          "single task with plenty of height",
			contentHeight: 10,
			taskCount:     1,
			expected:      3, // 10/1 - 1 = 9, capped at 3
		},
		{
			name:          "two tasks with moderate height",
			contentHeight: 8,
			taskCount:     2,
			expected:      3, // 8/2 - 1 = 3
		},
		{
			name:          "many tasks with limited height",
			contentHeight: 10,
			taskCount:     5,
			expected:      1, // 10/5 - 1 = 1
		},
		{
			name:          "very limited height returns minimum 1",
			contentHeight: 5,
			taskCount:     10,
			expected:      1, // 5/10 - 1 = -0.5, clamped to 1
		},
		{
			name:          "three tasks normal case",
			contentHeight: 9,
			taskCount:     3,
			expected:      2, // 9/3 - 1 = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateActionLinesPerTask(tt.contentHeight, tt.taskCount)
			if result != tt.expected {
				t.Errorf("calculateActionLinesPerTask(%d, %d) = %d, want %d",
					tt.contentHeight, tt.taskCount, result, tt.expected)
			}
		})
	}
}

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
