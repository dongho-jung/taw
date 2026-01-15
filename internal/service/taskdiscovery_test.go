// Package service provides business logic services for PAW.
package service

import (
	"testing"
)

func TestExtractCurrentAction(t *testing.T) {
	tests := []struct {
		name     string
		capture  string
		expected string
	}{
		{
			name:     "empty capture",
			capture:  "",
			expected: "",
		},
		{
			name:     "no spinner indicator",
			capture:  "Some text without spinner\nAnother line",
			expected: "",
		},
		{
			name:     "single spinner line",
			capture:  "⏺ Reading file...",
			expected: "Reading file...",
		},
		{
			name:     "spinner line with prefix text",
			capture:  "Some output\n⏺ Running tests\nMore output",
			expected: "Running tests",
		},
		{
			name:     "multiple spinner lines - returns last one",
			capture:  "⏺ First action\nSome text\n⏺ Second action\n⏺ Third action",
			expected: "Third action",
		},
		{
			name:     "spinner line with whitespace",
			capture:  "  ⏺  Analyzing code...  \n",
			expected: "Analyzing code...",
		},
		{
			name:     "long action text truncated",
			capture:  "⏺ This is a very long action description that should be truncated because it exceeds the maximum allowed length for display",
			expected: "This is a very long action description that should be tru...",
		},
		{
			name:     "spinner with multiline context",
			capture:  "Output line 1\nOutput line 2\n⏺ Writing tests for authentication module\nMore output",
			expected: "Writing tests for authentication module",
		},
		{
			name:     "realistic claude output with trailing chars",
			capture:  "╭───────────────────────────────────╮\n│ Claude Code ⏺ Reading file...    │\n╰───────────────────────────────────╯",
			expected: "Reading file...    │",
		},
		{
			name:     "spinner on its own line",
			capture:  "Some output\n⏺ Reading file...\nMore output",
			expected: "Reading file...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCurrentAction(tt.capture)
			if result != tt.expected {
				t.Errorf("extractCurrentAction() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTrimPreview(t *testing.T) {
	tests := []struct {
		name     string
		preview  string
		expected string
	}{
		{
			name:     "empty preview",
			preview:  "",
			expected: "",
		},
		{
			name:     "single line",
			preview:  "Hello world",
			expected: "Hello world",
		},
		{
			name:     "three lines",
			preview:  "Line 1\nLine 2\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "more than three lines - returns last 3",
			preview:  "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
			expected: "Line 3\nLine 4\nLine 5",
		},
		{
			name:     "empty lines filtered",
			preview:  "Line 1\n\n\nLine 2\n\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "whitespace trimmed",
			preview:  "  Line 1  \n  Line 2  \n  Line 3  ",
			expected: "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimPreview(tt.preview)
			if result != tt.expected {
				t.Errorf("trimPreview() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractDurationAndTokens(t *testing.T) {
	tests := []struct {
		name             string
		capture          string
		expectedDuration string
		expectedTokens   string
	}{
		{
			name:             "empty capture",
			capture:          "",
			expectedDuration: "",
			expectedTokens:   "",
		},
		{
			name:             "no status line",
			capture:          "Some text without status line\nAnother line",
			expectedDuration: "",
			expectedTokens:   "",
		},
		{
			name:             "full status line with duration and tokens",
			capture:          "✻ Whirring… (ctrl+c to interrupt · 54s · ↓ 2.7k tokens)",
			expectedDuration: "54s",
			expectedTokens:   "↓ 2.7k",
		},
		{
			name:             "status line with minutes and seconds",
			capture:          "⏺ Reading file… (ctrl+c to interrupt · 1m 36s · ↓ 5.9k tokens)",
			expectedDuration: "1m 36s",
			expectedTokens:   "↓ 5.9k",
		},
		{
			name:             "status line with thinking time",
			capture:          "✻ Whirring… (ctrl+c to interrupt · 2m 15s · ↓ 10.2k tokens · thought for 5s)",
			expectedDuration: "2m 15s",
			expectedTokens:   "↓ 10.2k",
		},
		{
			name:             "status line in context of other output",
			capture:          "Some output\n⏺ Working on task...\n✻ Processing… (ctrl+c to interrupt · 30s · ↓ 1.5k tokens)\nMore output",
			expectedDuration: "30s",
			expectedTokens:   "↓ 1.5k",
		},
		{
			name:             "multiple status lines - returns last one",
			capture:          "✻ First… (ctrl+c to interrupt · 10s · ↓ 500 tokens)\n✻ Second… (ctrl+c to interrupt · 45s · ↓ 3.2k tokens)",
			expectedDuration: "45s",
			expectedTokens:   "↓ 3.2k",
		},
		{
			name:             "status line with hours",
			capture:          "⏺ Long task… (ctrl+c to interrupt · 2h 5m · ↓ 100k tokens)",
			expectedDuration: "2h 5m",
			expectedTokens:   "↓ 100k",
		},
		{
			name:             "status line without tokens",
			capture:          "⏺ Starting… (ctrl+c to interrupt · 5s)",
			expectedDuration: "5s",
			expectedTokens:   "",
		},
		{
			name:             "upload tokens (↑)",
			capture:          "✻ Whirring… (ctrl+c to interrupt · 1m · ↑ 2.5k tokens)",
			expectedDuration: "1m",
			expectedTokens:   "↑ 2.5k",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration, tokens := extractDurationAndTokens(tt.capture)
			if duration != tt.expectedDuration {
				t.Errorf("extractDurationAndTokens() duration = %q, want %q", duration, tt.expectedDuration)
			}
			if tokens != tt.expectedTokens {
				t.Errorf("extractDurationAndTokens() tokens = %q, want %q", tokens, tt.expectedTokens)
			}
		})
	}
}

func TestIsDurationString(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"54s", true},
		{"1m 36s", true},
		{"2h 5m", true},
		{"10m", true},
		{"1h", true},
		{"", false},
		{"ctrl+c to interrupt", false},
		{"thought for 2s", true}, // This technically passes but is filtered by caller
		{"↓ 2.7k", false},
		{"tokens", false},
		{"5", false}, // Just number, no unit
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isDurationString(tt.input)
			if result != tt.expected {
				t.Errorf("isDurationString(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseWindowName(t *testing.T) {
	tests := []struct {
		name           string
		windowName     string
		expectedTask   string
		expectedStatus DiscoveredStatus
	}{
		{
			name:           "working task",
			windowName:     "🤖my-task",
			expectedTask:   "my-task",
			expectedStatus: DiscoveredWorking,
		},
		{
			name:           "waiting task",
			windowName:     "💬my-task",
			expectedTask:   "my-task",
			expectedStatus: DiscoveredWaiting,
		},
		{
			name:           "done task",
			windowName:     "✅my-task",
			expectedTask:   "my-task",
			expectedStatus: DiscoveredDone,
		},
		{
			name:           "warning task",
			windowName:     "⚠️my-task",
			expectedTask:   "my-task",
			expectedStatus: DiscoveredWarning,
		},
		{
			name:           "non-task window",
			windowName:     "regular-window",
			expectedTask:   "",
			expectedStatus: "",
		},
		{
			name:           "empty window name",
			windowName:     "",
			expectedTask:   "",
			expectedStatus: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, status := parseWindowName(tt.windowName)
			if task != tt.expectedTask {
				t.Errorf("parseWindowName() task = %q, want %q", task, tt.expectedTask)
			}
			if status != tt.expectedStatus {
				t.Errorf("parseWindowName() status = %q, want %q", status, tt.expectedStatus)
			}
		})
	}
}
