package main

import (
	"os"
	"path/filepath"
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
		{name: "working exact", output: "WORKING", want: task.StatusWorking, ok: true},
		{name: "working lowercase", output: "working", want: task.StatusWorking, ok: true},
		{name: "done lowercase", output: "done", want: task.StatusDone, ok: true},
		{name: "contains working", output: "Status: WORKING", want: task.StatusWorking, ok: true},
		{name: "contains done", output: "Status: DONE", want: task.StatusDone, ok: true},
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
			name:    "marker with Claude Code prefix",
			content: "Some output\n⏺ PAW_DONE\nReady for review\n",
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
		{
			name:    "done marker in last segment",
			content: "⏺ First response\nPAW_DONE\nReady.\n⏺ Second response\nWorking...\nPAW_DONE\nDone again.\n",
			want:    true,
		},
		{
			name:    "done marker only in previous segment (not last)",
			content: "⏺ First response\nAll done!\nPAW_DONE\nReady.\n⏺ New task started\nWorking on the new task...\n",
			want:    false,
		},
		{
			name:    "done marker with multiple segments",
			content: "⏺ First\nPAW_DONE\n⏺ Second\nPAW_DONE\n⏺ Third (new task)\nWorking...\n",
			want:    false,
		},
		{
			name:    "done marker without segment markers (backward compat)",
			content: "Some output\nTask completed.\nPAW_DONE\nReady for review.\n",
			want:    true,
		},
		{
			name:    "done marker without segment marker but within strict distance",
			content: "Line 1\nLine 2\nLine 3\nPAW_DONE\nLine 4\nLine 5\n",
			want:    true, // Within 20 lines from end
		},
		{
			name:    "old done marker without segment marker (beyond strict distance)",
			content: "PAW_DONE\n" + generateLines(25) + "New work started...\n",
			want:    false, // PAW_DONE is more than 20 lines from end, no segment marker
		},
		{
			name:    "old done marker with new segment marker",
			content: "PAW_DONE\n" + generateLines(25) + "⏺ New response\nWorking...\n",
			want:    false, // PAW_DONE is in old segment
		},
		// Stale marker tests (user input after PAW_DONE)
		{
			name:    "stale marker - user input on same line as prompt",
			content: "⏺ Done with task\nPAW_DONE\nReady for review\n> new request from user\n",
			want:    false,
		},
		{
			name:    "stale marker - user input after prompt line",
			content: "⏺ Done with task\nPAW_DONE\n>\nfix the bug in main.go\n",
			want:    false,
		},
		{
			name:    "stale marker - prompt but no input yet (not stale)",
			content: "⏺ Done with task\nPAW_DONE\nReady.\n>\n",
			want:    true,
		},
		{
			name:    "stale marker - user input but new segment started (not stale)",
			content: "⏺ Old task\nPAW_DONE\n> user input\n⏺ New response\nPAW_DONE\nDone again.\n",
			want:    true,
		},
		{
			name:    "stale marker - UI decoration should be ignored",
			content: "⏺ Done\nPAW_DONE\n>\n╭─────────╮\n│ Ready   │\n╰─────────╯\n",
			want:    true,
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

// generateLines creates N lines of filler content for testing distance limits.
func generateLines(n int) string {
	var result string
	for i := 0; i < n; i++ {
		result += "Line filler content...\n"
	}
	return result
}

func TestHasWaitingMarker(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "marker at end",
			content: "Some output\nWorking on it...\nPAW_WAITING\n",
			want:    true,
		},
		{
			name:    "marker with trailing whitespace",
			content: "Some output\n  PAW_WAITING  \n",
			want:    true,
		},
		{
			name:    "marker with UI after (within distance)",
			content: "Some output\nPAW_WAITING\n🔒 Plan\n> 1. Option A\n  Description\n2. Option B\n  Description\nEnter to select\n",
			want:    true,
		},
		{
			name:    "marker with Claude Code prefix",
			content: "Some output\n⏺ PAW_WAITING\nWaiting for input...\n",
			want:    true,
		},
		{
			name:    "no marker",
			content: "Some output\nStill working...\n",
			want:    false,
		},
		{
			name:    "partial marker",
			content: "PAW_WAITING_EXTRA\n",
			want:    false,
		},
		{
			name:    "marker embedded in text",
			content: "Text PAW_WAITING text\n",
			want:    false,
		},
		{
			name:    "empty content",
			content: "",
			want:    false,
		},
		{
			name:    "marker in last segment",
			content: "⏺ First response\nPAW_WAITING\nUI here.\n⏺ Second response\nWorking...\nPAW_WAITING\nMore UI.\n",
			want:    true,
		},
		{
			name:    "marker only in previous segment (not last)",
			content: "⏺ First response\nPAW_WAITING\nUI here.\n⏺ New task started\nWorking on the new task...\n",
			want:    false,
		},
		{
			name:    "waiting after done (new work started)",
			content: "⏺ First response\nPAW_DONE\nReady.\n⏺ New question response\nPAW_WAITING\n> 1. Option\n",
			want:    true,
		},
		{
			name:    "waiting after done without segment marker (real bug scenario)",
			content: "⏺ PAW_DONE\n⏺ New response\n  PAW_WAITING\n☐ Notify\nEnter to select\n",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasWaitingMarker(tt.content)
			if got != tt.want {
				t.Fatalf("hasWaitingMarker() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestWaitingPriorityOverDone tests that PAW_WAITING takes priority over PAW_DONE
// when both markers exist. This is the real bug scenario where:
// 1. Task outputs PAW_DONE (Done state)
// 2. User asks new question
// 3. Agent outputs PAW_WAITING (should become Waiting, not stay Done)
func TestWaitingPriorityOverDone(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		hasWaiting   bool
		hasDone      bool
		hasAskUser   bool
		expectedPrio string // "waiting" or "done" or "classify"
	}{
		{
			name:         "only done marker",
			content:      "⏺ First response\nAll done!\nPAW_DONE\nReady.\n",
			hasWaiting:   false,
			hasDone:      true,
			hasAskUser:   false,
			expectedPrio: "done",
		},
		{
			name:         "done then waiting (new work)",
			content:      "⏺ First response\nPAW_DONE\n⏺ New response\nPAW_WAITING\nEnter to select\n",
			hasWaiting:   true,
			hasDone:      false, // hasDoneMarker correctly ignores previous segment
			hasAskUser:   false,
			expectedPrio: "waiting",
		},
		{
			name:         "done then AskUserQuestion",
			content:      "⏺ First response\nPAW_DONE\n⏺ New response\nAskUserQuestion:\n  - question: Ready?\n",
			hasWaiting:   false,
			hasDone:      false, // hasDoneMarker correctly ignores previous segment
			hasAskUser:   true,
			expectedPrio: "waiting",
		},
		{
			// Real bug scenario: agent outputs ⏺ PAW_DONE then user asks question
			// Without a new ⏺ marker in the new response
			name:         "done with waiting in same segment (no new segment marker)",
			content:      "⏺ PAW_DONE\nReady.\n\n...user input...\n  PAW_WAITING\n☐ Question\n",
			hasWaiting:   true,
			hasDone:      true, // Both in same segment, both detected
			hasAskUser:   false,
			expectedPrio: "waiting", // But waiting should win
		},
		{
			name:         "no markers",
			content:      "⏺ Response\nWorking on task...\n",
			hasWaiting:   false,
			hasDone:      false,
			hasAskUser:   false,
			expectedPrio: "classify",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWaiting := hasWaitingMarker(tt.content)
			gotDone := hasDoneMarker(tt.content)
			gotAskUser := hasAskUserQuestionInLastSegment(tt.content)

			if gotWaiting != tt.hasWaiting {
				t.Errorf("hasWaitingMarker() = %v, want %v", gotWaiting, tt.hasWaiting)
			}
			if gotDone != tt.hasDone {
				t.Errorf("hasDoneMarker() = %v, want %v", gotDone, tt.hasDone)
			}
			if gotAskUser != tt.hasAskUser {
				t.Errorf("hasAskUserQuestionInLastSegment() = %v, want %v", gotAskUser, tt.hasAskUser)
			}

			// Verify priority logic: waiting/ask > done > classify
			var gotPrio string
			if gotWaiting || gotAskUser {
				gotPrio = "waiting"
			} else if gotDone {
				gotPrio = "done"
			} else {
				gotPrio = "classify"
			}

			if gotPrio != tt.expectedPrio {
				t.Errorf("priority = %v, want %v", gotPrio, tt.expectedPrio)
			}
		})
	}
}

func TestHasAskUserQuestionInLastSegment(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "AskUserQuestion at end of last segment",
			content: "⏺ Working on task\nDoing work...\nAskUserQuestion:\n  - question: How?\n",
			want:    true,
		},
		{
			name:    "AskUserQuestion with options",
			content: "⏺ Response\nAskUserQuestion:\n  questions:\n    - question: Which one?\n      options:\n        - Option A\n        - Option B\n",
			want:    true,
		},
		{
			name:    "AskUserQuestion in previous segment only",
			content: "⏺ First response\nAskUserQuestion:\n  - question: Done?\n⏺ New response\nWorking on changes...\n",
			want:    false,
		},
		{
			name:    "no AskUserQuestion",
			content: "⏺ Response\nAll done!\nPAW_DONE\n",
			want:    false,
		},
		{
			name:    "empty content",
			content: "",
			want:    false,
		},
		{
			name:    "AskUserQuestion without segment marker",
			content: "Working on task...\nAskUserQuestion:\n  - question: Ready?\n",
			want:    true,
		},
		{
			name:    "AskUserQuestion mentioned in text (not tool call)",
			content: "⏺ Response\nI will use AskUserQuestion to ask you\n",
			want:    false,
		},
		{
			name:    "AskUserQuestion tool invocation format",
			content: "⏺ Response\n  AskUserQuestion:\n    questions:\n",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAskUserQuestionInLastSegment(tt.content)
			if got != tt.want {
				t.Fatalf("hasAskUserQuestionInLastSegment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasUserInputAfterIndex(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		startIdx int
		want     bool
	}{
		{
			name:     "user input on same line as prompt",
			lines:    []string{"PAW_DONE", "> new request"},
			startIdx: 0,
			want:     true,
		},
		{
			name:     "user input after separate prompt line",
			lines:    []string{"PAW_DONE", ">", "fix the bug"},
			startIdx: 0,
			want:     true,
		},
		{
			name:     "prompt only, no input",
			lines:    []string{"PAW_DONE", ">"},
			startIdx: 0,
			want:     false,
		},
		{
			name:     "prompt with empty lines after",
			lines:    []string{"PAW_DONE", ">", "", ""},
			startIdx: 0,
			want:     false,
		},
		{
			name:     "new segment after input (not stale)",
			lines:    []string{"PAW_DONE", "> input", "⏺ new response"},
			startIdx: 0,
			want:     false,
		},
		{
			name:     "UI decoration after prompt (not user input)",
			lines:    []string{"PAW_DONE", ">", "╭─────────╮", "│ Ready   │", "╰─────────╯"},
			startIdx: 0,
			want:     false,
		},
		{
			name:     "no prompt at all",
			lines:    []string{"PAW_DONE", "Ready for review"},
			startIdx: 0,
			want:     false,
		},
		{
			name:     "multiple prompts, input on last",
			lines:    []string{"PAW_DONE", ">", "", ">", "user text"},
			startIdx: 0,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasUserInputAfterIndex(tt.lines, tt.startIdx)
			if got != tt.want {
				t.Fatalf("hasUserInputAfterIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUIDecoration(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "box top left", line: "╭─────────╮", want: true},
		{name: "box bottom left", line: "╰─────────╯", want: true},
		{name: "box vertical line", line: "│ content │", want: true},
		{name: "horizontal line", line: "─────────────", want: true},
		{name: "ASCII box corner", line: "┌─────────┐", want: true},
		{name: "ASCII box bottom", line: "└─────────┘", want: true},
		{name: "regular text", line: "Hello world", want: false},
		{name: "prompt", line: "> input", want: false},
		{name: "empty string", line: "", want: false},
		{name: "spaces only", line: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUIDecoration(tt.line)
			if got != tt.want {
				t.Fatalf("isUIDecoration(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestReadAndClearStatusSignal(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantStatus string
		wantExists bool // whether file should exist after call
	}{
		{
			name:       "valid done status",
			content:    "done",
			wantStatus: "done",
			wantExists: false,
		},
		{
			name:       "valid waiting status",
			content:    "waiting",
			wantStatus: "waiting",
			wantExists: false,
		},
		{
			name:       "valid working status",
			content:    "working",
			wantStatus: "working",
			wantExists: false,
		},
		{
			name:       "status with whitespace",
			content:    "  done  \n",
			wantStatus: "done",
			wantExists: false,
		},
		{
			name:       "invalid status",
			content:    "invalid",
			wantStatus: "",
			wantExists: false,
		},
		{
			name:       "empty file",
			content:    "",
			wantStatus: "",
			wantExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			signalPath := filepath.Join(tmpDir, ".status-signal")

			if err := os.WriteFile(signalPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			got := readAndClearStatusSignal(signalPath)

			if string(got) != tt.wantStatus {
				t.Errorf("readAndClearStatusSignal() = %q, want %q", got, tt.wantStatus)
			}

			// Check if file was deleted
			_, err := os.Stat(signalPath)
			fileExists := !os.IsNotExist(err)
			if fileExists != tt.wantExists {
				t.Errorf("file exists = %v, want %v", fileExists, tt.wantExists)
			}
		})
	}

	// Test non-existent file
	t.Run("non-existent file", func(t *testing.T) {
		got := readAndClearStatusSignal("/non/existent/path/.status-signal")
		if got != "" {
			t.Errorf("readAndClearStatusSignal() = %q, want empty string", got)
		}
	})
}

func TestIsIdlePromptPattern(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name: "thinking completed with empty prompt",
			content: `⏺ Working on something...
✻ Brewed for 34s
─────────────────────────────────────────
❯
`,
			want: true,
		},
		{
			name: "cogitated with empty prompt",
			content: `⏺ Working on something...
✻ Cogitated for 37s
─────────────────────────────────────────
❯
`,
			want: true,
		},
		{
			name: "empty prompt with status line",
			content: `⏺ Done with response
─────────────────────────────────────────
❯
─────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle)
`,
			want: true,
		},
		{
			name: "bypass permissions status line with prompt",
			content: `Some output
❯
  ⏵⏵ bypass permissions on (shift+tab to cycle)
`,
			want: true,
		},
		{
			name: "shift+tab to cycle status line with prompt",
			content: `Some output
>
  shift+tab to cycle
`,
			want: true,
		},
		{
			name: "active tool call - not idle",
			content: `⏺ Read(file.go)
  Reading 100 lines...
`,
			want: false,
		},
		{
			name: "in progress response - not idle",
			content: `⏺ Working on the task
Let me analyze this...
`,
			want: false,
		},
		{
			name: "thinking in progress - not idle",
			content: `⏺ Working on task
✻ Brewing...
`,
			want: false,
		},
		{
			name:    "empty content",
			content: "",
			want:    false,
		},
		{
			name:    "single line content",
			content: "Just one line",
			want:    false,
		},
		{
			name: "prompt without status line - not enough signal",
			content: `Some output here
❯
`,
			want: false,
		},
		{
			name: "real example from task",
			content: `✻ Brewed for 34s

─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
❯
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle)
`,
			want: true,
		},
		{
			name: "real example without marker - should be detected",
			content: `  8/10 - 이대로 써도 충분히 좋음.

✻ Cogitated for 37s

❯ ultrathink 근데...

⏺ Read(~/projects/paw/Makefile)
  ⎿  Read 176 lines

⏺ 현재 지원되는 설치 방법
  macOS
  brew install dongho-jung/tap/paw

✻ Brewed for 34s

─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
❯
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle)
`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isIdlePromptPattern(tt.content)
			if got != tt.want {
				t.Fatalf("isIdlePromptPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}
