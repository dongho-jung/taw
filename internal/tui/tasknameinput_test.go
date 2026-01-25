package tui

import "testing"

func TestValidateTaskName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		// Valid names
		{"valid simple", "my-task", ""},
		{"valid with numbers", "task-123", ""},
		{"valid all lowercase", "mytaskname", ""},
		{"valid min length", "abc", ""},
		{"valid max length", "abcdefghijklmnopqrstuvwxyz123456", ""}, // 32 chars

		// Too short
		{"too short empty", "", "Name must be at least 3 characters"},
		{"too short 1 char", "a", "Name must be at least 3 characters"},
		{"too short 2 chars", "ab", "Name must be at least 3 characters"},
		{"too short with spaces", "  a  ", "Name must be at least 3 characters"},

		// Too long
		{"too long", "abcdefghijklmnopqrstuvwxyz1234567", "Name must be at most 32 characters"}, // 33 chars

		// Invalid characters
		{"uppercase letters", "MyTask", "Use only lowercase letters, numbers, and hyphens"},
		{"spaces", "my task", "Use only lowercase letters, numbers, and hyphens"},
		{"underscores", "my_task", "Use only lowercase letters, numbers, and hyphens"},
		{"special chars", "my@task", "Use only lowercase letters, numbers, and hyphens"},

		// Hyphen issues
		{"leading hyphen", "-my-task", "Name cannot start or end with a hyphen"},
		{"trailing hyphen", "my-task-", "Name cannot start or end with a hyphen"},
		{"consecutive hyphens", "my--task", "Name cannot contain consecutive hyphens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateTaskName(tt.input)
			if got != tt.wantErr {
				t.Errorf("validateTaskName(%q) = %q, want %q", tt.input, got, tt.wantErr)
			}
		})
	}
}
