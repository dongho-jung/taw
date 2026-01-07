package runeutil

import (
	"testing"
	"unicode/utf8"
)

func TestNewSanitizer_DefaultOptions(t *testing.T) {
	s := NewSanitizer()
	if s == nil {
		t.Fatal("expected non-nil sanitizer")
	}
}

func TestSanitizer_PassthroughNormalText(t *testing.T) {
	s := NewSanitizer()

	input := []rune("Hello, World!")
	result := s.Sanitize(input)

	if string(result) != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", string(result))
	}
}

func TestSanitizer_ReplaceNewlines(t *testing.T) {
	s := NewSanitizer()

	// Test \n replacement (default replaces with \n, so it should pass through)
	input := []rune("line1\nline2")
	result := s.Sanitize(input)
	if string(result) != "line1\nline2" {
		t.Errorf("expected 'line1\\nline2', got %q", string(result))
	}

	// Test \r replacement
	input = []rune("line1\rline2")
	result = s.Sanitize(input)
	if string(result) != "line1\nline2" {
		t.Errorf("expected '\\r' to be replaced with '\\n', got %q", string(result))
	}
}

func TestSanitizer_ReplaceNewlinesCustom(t *testing.T) {
	s := NewSanitizer(ReplaceNewlines(" "))

	input := []rune("line1\nline2\nline3")
	result := s.Sanitize(input)

	if string(result) != "line1 line2 line3" {
		t.Errorf("expected 'line1 line2 line3', got %q", string(result))
	}
}

func TestSanitizer_ReplaceTabs(t *testing.T) {
	s := NewSanitizer()

	// Default replaces tab with 4 spaces
	input := []rune("hello\tworld")
	result := s.Sanitize(input)

	if string(result) != "hello    world" {
		t.Errorf("expected 'hello    world' (4 spaces), got %q", string(result))
	}
}

func TestSanitizer_ReplaceTabsCustom(t *testing.T) {
	s := NewSanitizer(ReplaceTabs("  "))

	input := []rune("hello\tworld")
	result := s.Sanitize(input)

	if string(result) != "hello  world" {
		t.Errorf("expected 'hello  world' (2 spaces), got %q", string(result))
	}
}

func TestSanitizer_RemoveControlCharacters(t *testing.T) {
	s := NewSanitizer()

	// Control characters other than \r, \n, \t should be removed
	input := []rune("hello\x00\x01\x02world")
	result := s.Sanitize(input)

	if string(result) != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", string(result))
	}
}

func TestSanitizer_SkipRuneError(t *testing.T) {
	s := NewSanitizer()

	// RuneError should be skipped
	input := []rune{'h', 'e', utf8.RuneError, 'l', 'l', 'o'}
	result := s.Sanitize(input)

	if string(result) != "hello" {
		t.Errorf("expected 'hello', got %q", string(result))
	}
}

func TestSanitizer_EmptyInput(t *testing.T) {
	s := NewSanitizer()

	input := []rune{}
	result := s.Sanitize(input)

	if len(result) != 0 {
		t.Errorf("expected empty result, got %q", string(result))
	}
}

func TestSanitizer_UnicodeCharacters(t *testing.T) {
	s := NewSanitizer()

	// Unicode characters should be preserved
	input := []rune("한글테스트 🎉 日本語")
	result := s.Sanitize(input)

	if string(result) != "한글테스트 🎉 日本語" {
		t.Errorf("expected unicode to be preserved, got %q", string(result))
	}
}

func TestSanitizer_MixedContent(t *testing.T) {
	s := NewSanitizer(ReplaceNewlines("|"), ReplaceTabs("->"))

	input := []rune("hello\tworld\nfoo\rbar\x00end")
	result := s.Sanitize(input)

	if string(result) != "hello->world|foo|barend" {
		t.Errorf("expected 'hello->world|foo|barend', got %q", string(result))
	}
}

func TestReplaceTabs_Option(t *testing.T) {
	opt := ReplaceTabs("XX")
	s := sanitizer{
		replaceNewLine: []rune("\n"),
		replaceTab:     []rune("    "),
	}
	s = opt(s)

	if string(s.replaceTab) != "XX" {
		t.Errorf("expected replaceTab to be 'XX', got %q", string(s.replaceTab))
	}
}

func TestReplaceNewlines_Option(t *testing.T) {
	opt := ReplaceNewlines(" ")
	s := sanitizer{
		replaceNewLine: []rune("\n"),
		replaceTab:     []rune("    "),
	}
	s = opt(s)

	if string(s.replaceNewLine) != " " {
		t.Errorf("expected replaceNewLine to be ' ', got %q", string(s.replaceNewLine))
	}
}
