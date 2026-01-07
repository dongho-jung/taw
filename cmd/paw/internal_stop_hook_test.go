package main

import (
	"testing"

	"github.com/dongho-jung/paw/internal/task"
)

func TestParseStopHookDecision(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   task.Status
		ok     bool
	}{
		{name: "waiting exact", output: "WAITING", want: task.StatusWaiting, ok: true},
		{name: "done lowercase", output: "done", want: task.StatusDone, ok: true},
		{name: "warning exact", output: "WARNING", want: task.StatusCorrupted, ok: true},
		{name: "warning prefix", output: "warn", want: task.StatusCorrupted, ok: true},
		{name: "contains waiting", output: "Result: WAITING", want: task.StatusWaiting, ok: true},
		{name: "unknown", output: "maybe", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseStopHookDecision(tt.output)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasDoneMarker(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "marker at end",
			content: "Some output\nVerification complete\nPAW_DONE\n",
			want:    true,
		},
		{
			name:    "marker with trailing whitespace",
			content: "Some output\n  PAW_DONE  \n",
			want:    true,
		},
		{
			name:    "marker in middle (within last 20 lines)",
			content: "Line 1\nPAW_DONE\nReady for review\n",
			want:    true,
		},
		{
			name:    "no marker",
			content: "Some output\nReady for review\n",
			want:    false,
		},
		{
			name:    "partial marker",
			content: "PAW_DONE_EXTRA\n",
			want:    false,
		},
		{
			name:    "marker embedded in text",
			content: "Text PAW_DONE text\n",
			want:    false,
		},
		{
			name:    "empty content",
			content: "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasDoneMarker(tt.content)
			if got != tt.want {
				t.Fatalf("hasDoneMarker() = %v, want %v", got, tt.want)
			}
		})
	}
}
