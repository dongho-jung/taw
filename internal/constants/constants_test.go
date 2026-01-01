package constants

import "testing"

func TestExtractTaskName(t *testing.T) {
	tests := []struct {
		name         string
		windowName   string
		wantTaskName string
		wantFound    bool
	}{
		{
			name:         "working emoji prefix",
			windowName:   "🤖my-task",
			wantTaskName: "my-task",
			wantFound:    true,
		},
		{
			name:         "waiting emoji prefix",
			windowName:   "💬my-task",
			wantTaskName: "my-task",
			wantFound:    true,
		},
		{
			name:         "done emoji prefix",
			windowName:   "✅my-task",
			wantTaskName: "my-task",
			wantFound:    true,
		},
		{
			name:         "warning emoji prefix",
			windowName:   "⚠️my-task",
			wantTaskName: "my-task",
			wantFound:    true,
		},
		{
			name:         "no emoji prefix",
			windowName:   "my-task",
			wantTaskName: "",
			wantFound:    false,
		},
		{
			name:         "different emoji",
			windowName:   "🚀my-task",
			wantTaskName: "",
			wantFound:    false,
		},
		{
			name:         "empty string",
			windowName:   "",
			wantTaskName: "",
			wantFound:    false,
		},
		{
			name:         "emoji only",
			windowName:   "🤖",
			wantTaskName: "",
			wantFound:    true,
		},
		{
			name:         "task with spaces",
			windowName:   "🤖task with spaces",
			wantTaskName: "task with spaces",
			wantFound:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTaskName, gotFound := ExtractTaskName(tt.windowName)
			if gotTaskName != tt.wantTaskName {
				t.Errorf("ExtractTaskName(%q) taskName = %q, want %q", tt.windowName, gotTaskName, tt.wantTaskName)
			}
			if gotFound != tt.wantFound {
				t.Errorf("ExtractTaskName(%q) found = %v, want %v", tt.windowName, gotFound, tt.wantFound)
			}
		})
	}
}

func TestTaskEmojis(t *testing.T) {
	expectedEmojis := []string{
		EmojiWorking,
		EmojiWaiting,
		EmojiDone,
		EmojiWarning,
	}

	if len(TaskEmojis) != len(expectedEmojis) {
		t.Errorf("TaskEmojis length = %d, want %d", len(TaskEmojis), len(expectedEmojis))
	}

	for i, emoji := range expectedEmojis {
		if TaskEmojis[i] != emoji {
			t.Errorf("TaskEmojis[%d] = %q, want %q", i, TaskEmojis[i], emoji)
		}
	}
}

func TestConstants(t *testing.T) {
	// Test that constants have expected values
	if TawDirName != ".taw" {
		t.Errorf("TawDirName = %q, want %q", TawDirName, ".taw")
	}
	if AgentsDirName != "agents" {
		t.Errorf("AgentsDirName = %q, want %q", AgentsDirName, "agents")
	}
	if DefaultMainBranch != "main" {
		t.Errorf("DefaultMainBranch = %q, want %q", DefaultMainBranch, "main")
	}
	if NewWindowName != EmojiNew+"new" {
		t.Errorf("NewWindowName = %q, want %q", NewWindowName, EmojiNew+"new")
	}
}
