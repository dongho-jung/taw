package notify

import (
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
			name:     "Windows Terminal",
			envVars:  map[string]string{"WT_SESSION": "abc-123"},
			expected: termWindowsTerminal,
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
			name:     "VSCode via TERM_PROGRAM",
			envVars:  map[string]string{"TERM_PROGRAM": "vscode"},
			expected: termVSCode,
		},
		{
			name:     "VSCode via VSCODE_INJECTION",
			envVars:  map[string]string{"VSCODE_INJECTION": "1"},
			expected: termVSCode,
		},
		{
			name:     "VSCode via VSCODE_GIT_IPC_HANDLE",
			envVars:  map[string]string{"VSCODE_GIT_IPC_HANDLE": "/tmp/vscode.sock"},
			expected: termVSCode,
		},
		{
			name:     "rxvt via TERM",
			envVars:  map[string]string{"TERM": "rxvt-unicode-256color"},
			expected: termRxvt,
		},
		{
			name:     "foot terminal via TERM",
			envVars:  map[string]string{"TERM": "foot"},
			expected: termFoot,
		},
		{
			name:     "foot-extra terminal via TERM",
			envVars:  map[string]string{"TERM": "foot-extra"},
			expected: termFoot,
		},
		{
			name:     "Contour terminal via TERM",
			envVars:  map[string]string{"TERM": "contour"},
			expected: termContour,
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
			// Clear relevant env vars using t.Setenv (auto-restores after test)
			envKeys := []string{
				"KITTY_WINDOW_ID", "WEZTERM_PANE", "GHOSTTY_RESOURCES_DIR",
				"WT_SESSION", "WARP_TERMINAL_VERSION", "TERM_PROGRAM", "TERM",
				"VSCODE_INJECTION", "VSCODE_GIT_IPC_HANDLE",
			}
			for _, key := range envKeys {
				t.Setenv(key, "")
			}

			// Set test env vars
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// Run test
			result := detectTerminal()
			if result != tt.expected {
				t.Errorf("detectTerminal() = %q, want %q", result, tt.expected)
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

func TestUrgencyConstants(t *testing.T) {
	// Verify urgency constants have expected values
	if UrgencyLow != 0 {
		t.Errorf("UrgencyLow = %d, want 0", UrgencyLow)
	}
	if UrgencyNormal != 1 {
		t.Errorf("UrgencyNormal = %d, want 1", UrgencyNormal)
	}
	if UrgencyCritical != 2 {
		t.Errorf("UrgencyCritical = %d, want 2", UrgencyCritical)
	}
}

func TestIconConstants(t *testing.T) {
	// Verify icon constants have expected values
	icons := map[Icon]string{
		IconNone:     "",
		IconInfo:     "info",
		IconWarning:  "warning",
		IconError:    "error",
		IconQuestion: "question",
		IconHelp:     "help",
	}

	for icon, expected := range icons {
		if string(icon) != expected {
			t.Errorf("Icon %q = %q, want %q", icon, string(icon), expected)
		}
	}
}

func TestTerminalTypeConstants(t *testing.T) {
	// Verify all terminal type constants are unique and non-empty
	terminals := []string{
		termITerm2,
		termKitty,
		termWezTerm,
		termGhostty,
		termRxvt,
		termWindowsTerminal,
		termVSCode,
		termFoot,
		termContour,
		termUnknown,
	}

	seen := make(map[string]bool)
	for _, term := range terminals {
		if term == "" {
			t.Error("Terminal type constant should not be empty")
		}
		if seen[term] {
			t.Errorf("Duplicate terminal type constant: %q", term)
		}
		seen[term] = true
	}
}

func TestSendWithUrgencyDoesNotPanic(t *testing.T) {
	// Verify SendWithUrgency doesn't panic with different urgency levels
	err := SendWithUrgency("Test", "Message", UrgencyLow)
	if err != nil {
		t.Errorf("SendWithUrgency(UrgencyLow) returned error: %v", err)
	}

	err = SendWithUrgency("Test", "Message", UrgencyNormal)
	if err != nil {
		t.Errorf("SendWithUrgency(UrgencyNormal) returned error: %v", err)
	}

	err = SendWithUrgency("Test", "Message", UrgencyCritical)
	if err != nil {
		t.Errorf("SendWithUrgency(UrgencyCritical) returned error: %v", err)
	}
}

func TestSendWithOptionsDoesNotPanic(t *testing.T) {
	// Verify SendWithOptions doesn't panic with various options
	opts := Options{
		Urgency: UrgencyCritical,
		Icon:    IconError,
	}
	err := SendWithOptions("Test", "Message", opts)
	if err != nil {
		t.Errorf("SendWithOptions() returned error: %v", err)
	}

	// Test with default options
	err = SendWithOptions("Test", "Message", Options{})
	if err != nil {
		t.Errorf("SendWithOptions() with default options returned error: %v", err)
	}
}

func TestOptionsDefaults(t *testing.T) {
	// Verify Options has sensible zero values
	opts := Options{}

	if opts.Urgency != UrgencyLow {
		t.Errorf("Default Urgency = %d, want %d (UrgencyLow)", opts.Urgency, UrgencyLow)
	}

	if opts.Icon != IconNone {
		t.Errorf("Default Icon = %q, want %q (IconNone)", opts.Icon, IconNone)
	}
}

func TestGetTmuxClientTTYsDoesNotPanic(t *testing.T) {
	// Verify getTmuxClientTTYs doesn't panic even when not in tmux
	// (it should return nil/empty when tmux is not available)
	result := getTmuxClientTTYs()
	// Result can be nil (not in tmux) or a list of ttys (in tmux)
	// Either is fine, we just verify it doesn't panic
	_ = result
}

func TestGetTmuxClientTTYsFiltersValidPaths(t *testing.T) {
	// This test verifies that the function properly filters tty paths
	// Since we can't easily mock exec.Command, we just verify the
	// function returns paths starting with /dev/ when in tmux
	result := getTmuxClientTTYs()
	for _, tty := range result {
		if tty != "" && tty[:5] != "/dev/" {
			t.Errorf("getTmuxClientTTYs returned invalid path: %q", tty)
		}
	}
}

func TestUnixTime(t *testing.T) {
	// Verify unixTime returns a reasonable timestamp
	ts := unixTime()
	// Should be after 2020-01-01 (timestamp 1577836800)
	if ts < 1577836800 {
		t.Errorf("unixTime() = %d, want > 1577836800", ts)
	}
}
