package claude

import (
	"fmt"
	"testing"
	"time"

	"github.com/dongho-jung/paw/internal/tmux"
)

func TestSanitizeTaskName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "already valid",
			input: "add-login-feature",
			want:  "add-login-feature",
		},
		{
			name:  "uppercase to lowercase",
			input: "ADD-LOGIN-FEATURE",
			want:  "add-login-feature",
		},
		{
			name:  "spaces to hyphens",
			input: "add login feature",
			want:  "add-login-feature",
		},
		{
			name:  "underscores to hyphens",
			input: "add_login_feature",
			want:  "add-login-feature",
		},
		{
			name:  "special characters removed",
			input: "add@login#feature!",
			want:  "addloginfeature",
		},
		{
			name:  "remove quotes",
			input: `"add-login-feature"`,
			want:  "add-login-feature",
		},
		{
			name:  "remove backticks",
			input: "`add-login-feature`",
			want:  "add-login-feature",
		},
		{
			name:  "collapse multiple hyphens",
			input: "add--login---feature",
			want:  "add-login-feature",
		},
		{
			name:  "trim leading/trailing hyphens",
			input: "-add-login-feature-",
			want:  "add-login-feature",
		},
		{
			name:  "truncate long name",
			input: "this-is-a-very-long-task-name-that-exceeds-the-maximum-length-limit",
			want:  "this-is-a-very-long-task-name-th",
		},
		{
			name:  "whitespace trimmed",
			input: "  add-login-feature  ",
			want:  "add-login-feature",
		},
		{
			name:  "newlines removed",
			input: "add-login-feature\n",
			want:  "add-login-feature",
		},
		{
			name:  "numbers allowed",
			input: "fix-bug-123",
			want:  "fix-bug-123",
		},
		{
			name:  "mixed case and special chars",
			input: "Fix User@Email.com Validation!",
			want:  "fix-useremailcom-validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTaskName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTaskName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTaskNamePattern(t *testing.T) {
	validNames := []string{
		"add-login-feature",
		"fix-bug-123",
		"12345678",
		"a1b2c3d4",
		"feature-a",
	}

	invalidNames := []string{
		"short",       // too short
		"-start-dash", // starts with dash
		"end-dash-",   // ends with dash
		"has space",   // contains space
		"HAS_UPPER",   // contains uppercase
		"a",           // too short
		"ab",          // too short
		"abc",         // too short
		"",            // empty
	}

	for _, name := range validNames {
		if !TaskNamePattern.MatchString(name) {
			t.Errorf("TaskNamePattern should match %q", name)
		}
	}

	for _, name := range invalidNames {
		if TaskNamePattern.MatchString(name) {
			t.Errorf("TaskNamePattern should not match %q", name)
		}
	}
}

func TestReadyPatterns(t *testing.T) {
	shouldMatch := []string{
		"Trust this project",
		"bypass permissions",
		"╭─ Claude is ready",
		">",
		"> ",
		"claude-code",
		"Cost: $0.02",
		"Cost $0.01",
	}

	shouldNotMatch := []string{
		"Loading...",
		"Processing",
		"Running tests",
	}

	for _, s := range shouldMatch {
		if !ReadyPatterns.MatchString(s) {
			t.Errorf("ReadyPatterns should match %q", s)
		}
	}

	for _, s := range shouldNotMatch {
		if ReadyPatterns.MatchString(s) {
			t.Errorf("ReadyPatterns should not match %q", s)
		}
	}
}

func TestTrustPattern(t *testing.T) {
	shouldMatch := []string{
		"Trust this project",
		"Do you trust this",
		"trust",
		"TRUST",
	}

	shouldNotMatch := []string{
		"Loading...",
		"Processing",
		"Running tests",
	}

	for _, s := range shouldMatch {
		if !TrustPattern.MatchString(s) {
			t.Errorf("TrustPattern should match %q", s)
		}
	}

	for _, s := range shouldNotMatch {
		if TrustPattern.MatchString(s) {
			t.Errorf("TrustPattern should not match %q", s)
		}
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	tests := []struct {
		name          string
		globalPrompt  string
		projectPrompt string
		want          string
	}{
		{
			name:          "both prompts",
			globalPrompt:  "Global instructions",
			projectPrompt: "Project instructions",
			want:          "Global instructions\n\n---\n\nProject instructions",
		},
		{
			name:          "only global",
			globalPrompt:  "Global instructions",
			projectPrompt: "",
			want:          "Global instructions",
		},
		{
			name:          "only project",
			globalPrompt:  "",
			projectPrompt: "Project instructions",
			want:          "Project instructions",
		},
		{
			name:          "neither",
			globalPrompt:  "",
			projectPrompt: "",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSystemPrompt(tt.globalPrompt, tt.projectPrompt)
			if got != tt.want {
				t.Errorf("BuildSystemPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildClaudeCommand(t *testing.T) {
	tests := []struct {
		name                       string
		systemPrompt               string
		dangerouslySkipPermissions bool
		wantArgs                   []string
	}{
		{
			name:                       "no options",
			systemPrompt:               "",
			dangerouslySkipPermissions: false,
			wantArgs:                   []string{"claude"},
		},
		{
			name:                       "with system prompt",
			systemPrompt:               "My prompt",
			dangerouslySkipPermissions: false,
			wantArgs:                   []string{"claude", "--system-prompt", "My prompt"},
		},
		{
			name:                       "with skip permissions",
			systemPrompt:               "",
			dangerouslySkipPermissions: true,
			wantArgs:                   []string{"claude", "--dangerously-skip-permissions"},
		},
		{
			name:                       "with both",
			systemPrompt:               "My prompt",
			dangerouslySkipPermissions: true,
			wantArgs:                   []string{"claude", "--system-prompt", "My prompt", "--dangerously-skip-permissions"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildClaudeCommand(tt.systemPrompt, tt.dangerouslySkipPermissions)
			if len(got) != len(tt.wantArgs) {
				t.Errorf("BuildClaudeCommand() = %v, want %v", got, tt.wantArgs)
				return
			}
			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("BuildClaudeCommand()[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestNew(t *testing.T) {
	client := New()
	if client == nil {
		t.Fatal("New() returned nil")
	}
}

// mockTmuxClient implements tmux.Client for testing
type mockTmuxClient struct {
	hasPaneResult      bool
	capturePaneContent string
	capturePaneErr     error
	sendKeysErr        error
	paneCommand        string
	paneCommandErr     error
	captureCallCount   int
}

func (m *mockTmuxClient) HasPane(target string) bool {
	return m.hasPaneResult
}

func (m *mockTmuxClient) CapturePane(target string, lines int) (string, error) {
	m.captureCallCount++
	return m.capturePaneContent, m.capturePaneErr
}

func (m *mockTmuxClient) SendKeys(target string, keys ...string) error {
	return m.sendKeysErr
}

func (m *mockTmuxClient) SendKeysLiteral(target, text string) error {
	return m.sendKeysErr
}

func (m *mockTmuxClient) GetPaneCommand(target string) (string, error) {
	return m.paneCommand, m.paneCommandErr
}

// Implement remaining interface methods as no-ops
func (m *mockTmuxClient) HasSession(name string) bool                          { return false }
func (m *mockTmuxClient) NewSession(opts tmux.SessionOpts) error               { return nil }
func (m *mockTmuxClient) AttachSession(name string) error                      { return nil }
func (m *mockTmuxClient) SwitchClient(target string) error                     { return nil }
func (m *mockTmuxClient) KillSession(name string) error                        { return nil }
func (m *mockTmuxClient) KillServer() error                                    { return nil }
func (m *mockTmuxClient) NewWindow(opts tmux.WindowOpts) (string, error)       { return "", nil }
func (m *mockTmuxClient) KillWindow(target string) error                       { return nil }
func (m *mockTmuxClient) RenameWindow(target, name string) error               { return nil }
func (m *mockTmuxClient) ListWindows() ([]tmux.Window, error)                  { return nil, nil }
func (m *mockTmuxClient) SelectWindow(target string) error                     { return nil }
func (m *mockTmuxClient) MoveWindow(source, target string) error               { return nil }
func (m *mockTmuxClient) SplitWindow(target string, h bool, d, c string) error { return nil }
func (m *mockTmuxClient) SplitWindowPane(opts tmux.SplitOpts) (string, error)  { return "", nil }
func (m *mockTmuxClient) SelectPane(target string) error                       { return nil }
func (m *mockTmuxClient) KillPane(target string) error                         { return nil }
func (m *mockTmuxClient) ClearHistory(target string) error                     { return nil }
func (m *mockTmuxClient) RespawnPane(target, startDir, command string) error   { return nil }
func (m *mockTmuxClient) WaitForPane(target string, maxWait time.Duration, minLen int) error {
	return nil
}
func (m *mockTmuxClient) DisplayPopup(opts tmux.PopupOpts, command string) error { return nil }
func (m *mockTmuxClient) SetOption(key, value string, global bool) error      { return nil }
func (m *mockTmuxClient) GetOption(key string) (string, error)                { return "", nil }
func (m *mockTmuxClient) SetMultipleOptions(options map[string]string) error  { return nil }
func (m *mockTmuxClient) SetEnv(key, value string) error                      { return nil }
func (m *mockTmuxClient) Bind(opts tmux.BindOpts) error                       { return nil }
func (m *mockTmuxClient) Unbind(key string) error                             { return nil }
func (m *mockTmuxClient) Run(args ...string) error                            { return nil }
func (m *mockTmuxClient) RunWithOutput(args ...string) (string, error)        { return "", nil }
func (m *mockTmuxClient) Display(format string) (string, error)               { return "", nil }
func (m *mockTmuxClient) DisplayMultiple(formats ...string) ([]string, error) { return nil, nil }
func (m *mockTmuxClient) DisplayMessage(message string, durationMs int) error { return nil }
func (m *mockTmuxClient) JoinPane(source, target string, opts tmux.JoinOpts) error {
	return nil
}
func (m *mockTmuxClient) BreakPane(source string, opts tmux.BreakOpts) (string, error) {
	return "", nil
}

func TestIsClaudeRunning(t *testing.T) {
	tests := []struct {
		name        string
		hasPaneRes  bool
		paneCommand string
		paneErr     error
		expected    bool
	}{
		{
			name:        "pane does not exist",
			hasPaneRes:  false,
			paneCommand: "",
			paneErr:     nil,
			expected:    false,
		},
		{
			name:        "pane shows bash shell",
			hasPaneRes:  true,
			paneCommand: "bash",
			paneErr:     nil,
			expected:    false,
		},
		{
			name:        "pane shows zsh shell",
			hasPaneRes:  true,
			paneCommand: "zsh",
			paneErr:     nil,
			expected:    false,
		},
		{
			name:        "pane shows login shell",
			hasPaneRes:  true,
			paneCommand: "-zsh",
			paneErr:     nil,
			expected:    false,
		},
		{
			name:        "pane shows claude command",
			hasPaneRes:  true,
			paneCommand: "claude",
			paneErr:     nil,
			expected:    true,
		},
		{
			name:        "pane shows start-agent script",
			hasPaneRes:  true,
			paneCommand: "start-agent",
			paneErr:     nil,
			expected:    true,
		},
		{
			name:        "pane shows other command",
			hasPaneRes:  true,
			paneCommand: "node",
			paneErr:     nil,
			expected:    true, // assume subprocess
		},
		{
			name:        "pane command error",
			hasPaneRes:  true,
			paneCommand: "",
			paneErr:     fmt.Errorf("error"),
			expected:    false,
		},
	}

	client := New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTmuxClient{
				hasPaneResult:  tt.hasPaneRes,
				paneCommand:    tt.paneCommand,
				paneCommandErr: tt.paneErr,
			}
			// Cast to the internal type to access the method with our mock
			result := client.IsClaudeRunning(mock, "@0")
			if result != tt.expected {
				t.Errorf("IsClaudeRunning() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSendTrustResponse(t *testing.T) {
	tests := []struct {
		name           string
		hasPaneRes     bool
		paneContent    string
		capturePaneErr error
		sendKeysErr    error
		expectErr      bool
	}{
		{
			name:        "trust prompt detected, sends y",
			hasPaneRes:  true,
			paneContent: "Do you trust this project?",
			expectErr:   false,
		},
		{
			name:        "no trust prompt, no action",
			hasPaneRes:  true,
			paneContent: "Claude is ready",
			expectErr:   false,
		},
		{
			name:           "capture pane error",
			hasPaneRes:     true,
			paneContent:    "",
			capturePaneErr: fmt.Errorf("capture error"),
			expectErr:      true,
		},
	}

	client := New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTmuxClient{
				hasPaneResult:      tt.hasPaneRes,
				capturePaneContent: tt.paneContent,
				capturePaneErr:     tt.capturePaneErr,
				sendKeysErr:        tt.sendKeysErr,
			}
			err := client.SendTrustResponse(mock, "@0")
			if (err != nil) != tt.expectErr {
				t.Errorf("SendTrustResponse() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestVerifyPaneAlive(t *testing.T) {
	tests := []struct {
		name        string
		hasPaneRes  bool
		paneContent string
		expectErr   bool
	}{
		{
			name:        "pane alive with content",
			hasPaneRes:  true,
			paneContent: "Some content here",
			expectErr:   false,
		},
		{
			name:        "pane does not exist",
			hasPaneRes:  false,
			paneContent: "",
			expectErr:   true,
		},
		{
			name:        "pane exists but empty",
			hasPaneRes:  true,
			paneContent: "",
			expectErr:   true,
		},
		{
			name:        "pane exists with whitespace only",
			hasPaneRes:  true,
			paneContent: "   \n\t  ",
			expectErr:   true,
		},
	}

	client := New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTmuxClient{
				hasPaneResult:      tt.hasPaneRes,
				capturePaneContent: tt.paneContent,
			}
			// Use a very short timeout for testing
			err := client.VerifyPaneAlive(mock, "@0", 100*time.Millisecond)
			if (err != nil) != tt.expectErr {
				t.Errorf("VerifyPaneAlive() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestShellCommands(t *testing.T) {
	// Test that shellCommands contains expected shells
	expectedShells := []string{"bash", "zsh", "sh", "fish"}
	for _, shell := range expectedShells {
		found := false
		for _, s := range shellCommands {
			if s == shell {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("shellCommands should contain %q", shell)
		}
	}
}

func TestSummaryTimeout(t *testing.T) {
	// Verify SummaryTimeout is reasonable
	if SummaryTimeout < 5*time.Second {
		t.Errorf("SummaryTimeout too short: %v", SummaryTimeout)
	}
	if SummaryTimeout > 60*time.Second {
		t.Errorf("SummaryTimeout too long: %v", SummaryTimeout)
	}
}
