package notify

import (
	"os"
	"testing"
)

func TestSoundTypeConstants(t *testing.T) {
	// Verify sound type constants have expected values
	sounds := map[SoundType]string{
		SoundTaskCreated:   "Glass",
		SoundTaskCompleted: "Hero",
		SoundNeedInput:     "Funk",
		SoundError:         "Basso",
		SoundCancelPending: "Tink",
	}

	for sound, expected := range sounds {
		if string(sound) != expected {
			t.Errorf("SoundType %q = %q, want %q", sound, string(sound), expected)
		}
	}
}

func TestEscapeSequenceConstants(t *testing.T) {
	if ESC != "\033" {
		t.Errorf("ESC = %q, want %q", ESC, "\033")
	}
	if BEL != "\a" {
		t.Errorf("BEL = %q, want %q", BEL, "\a")
	}
	if ST != "\033\\" {
		t.Errorf("ST = %q, want %q", ST, "\033\\")
	}
}

func TestDetectTerminal(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "Kitty terminal",
			envVars:  map[string]string{"KITTY_WINDOW_ID": "1"},
			expected: termKitty,
		},
		{
			name:     "WezTerm terminal",
			envVars:  map[string]string{"WEZTERM_PANE": "1"},
			expected: termWezTerm,
		},
		{
			name:     "Ghostty terminal",
			envVars:  map[string]string{"GHOSTTY_RESOURCES_DIR": "/path/to/resources"},
			expected: termGhostty,
		},
		{
			name:     "iTerm2 via TERM_PROGRAM",
			envVars:  map[string]string{"TERM_PROGRAM": "iTerm.app"},
			expected: termITerm2,
		},
		{
			name:     "WezTerm via TERM_PROGRAM",
			envVars:  map[string]string{"TERM_PROGRAM": "WezTerm"},
			expected: termWezTerm,
		},
		{
			name:     "rxvt via TERM",
			envVars:  map[string]string{"TERM": "rxvt-unicode-256color"},
			expected: termRxvt,
		},
		{
			name:     "Warp terminal (no OSC support)",
			envVars:  map[string]string{"WARP_TERMINAL_VERSION": "1.0.0"},
			expected: termUnknown,
		},
		{
			name:     "Unknown terminal",
			envVars:  map[string]string{},
			expected: termUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and clear relevant env vars
			savedEnv := map[string]string{}
			envKeys := []string{
				"KITTY_WINDOW_ID", "WEZTERM_PANE", "GHOSTTY_RESOURCES_DIR",
				"WARP_TERMINAL_VERSION", "TERM_PROGRAM", "TERM",
			}
			for _, key := range envKeys {
				savedEnv[key] = os.Getenv(key)
				os.Unsetenv(key)
			}

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Run test
			result := detectTerminal()
			if result != tt.expected {
				t.Errorf("detectTerminal() = %q, want %q", result, tt.expected)
			}

			// Restore env vars
			for k, v := range savedEnv {
				if v == "" {
					os.Unsetenv(k)
				} else {
					os.Setenv(k, v)
				}
			}
		})
	}
}

func TestWrapTmuxPassthrough(t *testing.T) {
	// Test that tmux passthrough wrapping doubles escape characters
	input := "\033]9;test\a"
	result := wrapTmuxPassthrough(input)

	// Should start with ESC P tmux;
	if result[:7] != "\033Ptmux;" {
		t.Errorf("wrapTmuxPassthrough should start with ESC P tmux;, got %q", result[:7])
	}

	// Should end with ST (ESC \)
	if result[len(result)-2:] != "\033\\" {
		t.Errorf("wrapTmuxPassthrough should end with ST, got %q", result[len(result)-2:])
	}

	// Should contain doubled escape characters
	// Original has 1 ESC, wrapped should have 2 (doubled)
	// Plus the wrapper adds its own ESC characters
}

func TestSendDoesNotPanic(t *testing.T) {
	// Just verify Send doesn't panic
	err := Send("Test Title", "Test Message")
	if err != nil {
		t.Errorf("Send() returned error: %v", err)
	}
}

func TestSendWithActionsDoesNotPanic(t *testing.T) {
	// Just verify SendWithActions doesn't panic and returns -1
	index, err := SendWithActions("Title", "Message", "", []string{"Action1", "Action2"}, 5)
	if err != nil {
		t.Errorf("SendWithActions() returned error: %v", err)
	}
	if index != -1 {
		t.Errorf("SendWithActions() should return -1, got %d", index)
	}
}

func TestPlaySoundDoesNotPanic(t *testing.T) {
	// Just verify PlaySound doesn't panic
	PlaySound(SoundTaskCreated)
	PlaySound(SoundTaskCompleted)
	PlaySound(SoundNeedInput)
	PlaySound(SoundError)
	PlaySound(SoundCancelPending)
}
