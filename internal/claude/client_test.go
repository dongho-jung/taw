package claude

import (
	"testing"
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
		"short",        // too short
		"-start-dash",  // starts with dash
		"end-dash-",    // ends with dash
		"has space",    // contains space
		"HAS_UPPER",    // contains uppercase
		"a",            // too short
		"ab",           // too short
		"abc",          // too short
		"",             // empty
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
