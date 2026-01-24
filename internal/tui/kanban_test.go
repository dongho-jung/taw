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

func TestIsKeyboardHintLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		// Lines that should be filtered
		{
			name:     "option toggle with triangle",
			line:     "⏵⏵ bypass permissions on (shift+tab to cycle)",
			expected: true,
		},
		{
			name:     "single triangle option",
			line:     "⏵ auto-accept edits",
			expected: true,
		},
		{
			name:     "shift+tab hint",
			line:     "some option (shift+tab to cycle)",
			expected: true,
		},
		{
			name:     "shift-tab hint",
			line:     "some option (shift-tab to cycle)",
			expected: true,
		},
		{
			name:     "tab to cycle hint",
			line:     "option (tab to cycle)",
			expected: true,
		},
		{
			name:     "ctrl+c to interrupt",
			line:     "processing (ctrl+c to interrupt)",
			expected: true,
		},
		{
			name:     "esc to cancel",
			line:     "doing stuff (esc to cancel)",
			expected: true,
		},
		{
			name:     "to interrupt suffix",
			line:     "waiting (press to interrupt)",
			expected: true,
		},
		{
			name:     "to cancel suffix",
			line:     "running (press to cancel)",
			expected: true,
		},
		// Lines that should NOT be filtered
		{
			name:     "normal action text",
			line:     "Reading file /path/to/file.go",
			expected: false,
		},
		{
			name:     "normal status",
			line:     "Running tests...",
			expected: false,
		},
		{
			name:     "metadata line",
			line:     "1m 36s · ↓ 5.9k",
			expected: false,
		},
		{
			name:     "empty line",
			line:     "",
			expected: false,
		},
		{
			name:     "regular text with parentheses",
			line:     "Analyzing (main.go)",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKeyboardHintLine(tt.line)
			if result != tt.expected {
				t.Errorf("isKeyboardHintLine(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestNormalizePreviewLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "normal line",
			line:     "Reading file.go",
			expected: "Reading file.go",
		},
		{
			name:     "empty line",
			line:     "",
			expected: "",
		},
		{
			name:     "whitespace only",
			line:     "   ",
			expected: "",
		},
		{
			name:     "line with spinner prefix",
			line:     "⏺ Running tests",
			expected: "Running tests",
		},
		{
			name:     "line with alternative spinner",
			line:     "✻ Processing",
			expected: "Processing",
		},
		{
			name:     "keyboard hint line filtered",
			line:     "⏵⏵ bypass permissions on (shift+tab to cycle)",
			expected: "",
		},
		{
			name:     "option toggle filtered",
			line:     "⏵ auto-accept edits",
			expected: "",
		},
		{
			name:     "ctrl+c hint filtered",
			line:     "waiting (ctrl+c to interrupt)",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePreviewLine(tt.line)
			if result != tt.expected {
				t.Errorf("normalizePreviewLine(%q) = %q, want %q", tt.line, result, tt.expected)
			}
		})
	}
}

// Benchmark tests for performance-critical functions

func BenchmarkIsKeyboardHintLine(b *testing.B) {
	lines := []string{
		"⏵⏵ bypass permissions on (shift+tab to cycle)",
		"Reading file /path/to/file.go",
		"Running tests...",
		"1m 36s · ↓ 5.9k",
		"processing (ctrl+c to interrupt)",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			_ = isKeyboardHintLine(line)
		}
	}
}

func BenchmarkWrapByWidth(b *testing.B) {
	text := "This is a longer line of text that will need to be wrapped across multiple lines when displayed in a narrow column"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = wrapByWidth(text, 30)
	}
}

func BenchmarkNormalizePreviewLine(b *testing.B) {
	lines := []string{
		"⏺ Running tests",
		"Reading file.go",
		"⏵ auto-accept edits",
		"waiting (ctrl+c to interrupt)",
		"Processing something important here",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			_ = normalizePreviewLine(line)
		}
	}
}
