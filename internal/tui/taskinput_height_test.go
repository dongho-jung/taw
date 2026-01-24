package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea/v2"
)

// TestTaskInput_DynamicHeight verifies the textarea height adjustment behavior:
// 1. Default 5 lines
// 2. Auto-expand as content grows (up to 50% of screen)
// 3. Scrollbar appears when content exceeds max height
func TestTaskInput_DynamicHeight(t *testing.T) {
	m := NewTaskInputWithOptions(nil, true)

	// Simulate window size: 100x40 terminal
	// Max height should be 50% of (40 - 4) = 18 lines
	windowMsg := tea.WindowSizeMsg{
		Width:  100,
		Height: 40,
	}

	_, _ = m.Update(windowMsg)

	// Test 1: Initial state - should be 5 lines (default)
	if m.textareaHeight != 5 {
		t.Errorf("Initial height should be 5, got %d", m.textareaHeight)
	}

	// Test 2: Add 3 lines of content - should stay at 5 lines (min height)
	m.textarea.SetValue("line 1\nline 2\nline 3")
	m.updateTextareaHeight()
	if m.textareaHeight != 5 {
		t.Errorf("Height with 3 lines should be 5 (min), got %d", m.textareaHeight)
	}

	// Test 3: Add 10 lines of content - should expand to 10 lines
	m.textarea.SetValue(strings.Repeat("line\n", 9) + "line 10")
	m.updateTextareaHeight()
	if m.textareaHeight != 10 {
		t.Errorf("Height with 10 lines should be 10, got %d", m.textareaHeight)
	}

	// Test 4: Add 15 lines of content - should expand to 15 lines
	m.textarea.SetValue(strings.Repeat("line\n", 14) + "line 15")
	m.updateTextareaHeight()
	if m.textareaHeight != 15 {
		t.Errorf("Height with 15 lines should be 15, got %d", m.textareaHeight)
	}

	// Test 5: Add 20 lines of content - should cap at max height (18)
	// Max height = (40 - 4) * 50% = 18
	m.textarea.SetValue(strings.Repeat("line\n", 19) + "line 20")
	m.updateTextareaHeight()
	expectedMaxHeight := 18
	if m.textareaHeight != expectedMaxHeight {
		t.Errorf("Height with 20 lines should be capped at %d, got %d", expectedMaxHeight, m.textareaHeight)
	}
	if m.textareaMaxHeight != expectedMaxHeight {
		t.Errorf("Max height should be %d, got %d", expectedMaxHeight, m.textareaMaxHeight)
	}

	// Test 6: Verify scrollbar condition when content exceeds visible height
	contentLines := 20
	visibleLines := m.textarea.Height()

	// When content (20 lines) > visible (18 lines), scrollbar should appear
	if visibleLines >= contentLines {
		t.Errorf("Expected scrollbar condition: contentLines (%d) should be > visibleLines (%d)", contentLines, visibleLines)
	}

	// Test 7: Reduce content back to 5 lines - should shrink back to 5
	m.textarea.SetValue("line 1\nline 2\nline 3\nline 4\nline 5")
	m.updateTextareaHeight()
	if m.textareaHeight != 5 {
		t.Errorf("Height should shrink back to 5, got %d", m.textareaHeight)
	}

	// Test 8: Test with different screen size (smaller terminal)
	// 100x20 terminal -> max height should be (20-4) * 50% = 8 lines
	smallWindowMsg := tea.WindowSizeMsg{
		Width:  100,
		Height: 20,
	}
	_, _ = m.Update(smallWindowMsg)

	// Add 15 lines, should cap at 8
	m.textarea.SetValue(strings.Repeat("line\n", 14) + "line 15")
	m.updateTextareaHeight()
	expectedSmallMax := 8
	if m.textareaHeight != expectedSmallMax {
		t.Errorf("Height in small terminal should be capped at %d, got %d", expectedSmallMax, m.textareaHeight)
	}
}

// TestTaskInput_MinHeight verifies the minimum height is always maintained
func TestTaskInput_MinHeight(t *testing.T) {
	m := NewTaskInputWithOptions(nil, true)

	// Simulate window size
	windowMsg := tea.WindowSizeMsg{
		Width:  100,
		Height: 40,
	}
	_, _ = m.Update(windowMsg)

	// Test with 1 line - should maintain min height of 5
	m.textarea.SetValue("single line")
	m.updateTextareaHeight()
	if m.textareaHeight != 5 {
		t.Errorf("Height with 1 line should be 5 (min), got %d", m.textareaHeight)
	}

	// Test with empty content - should maintain min height of 5
	m.textarea.SetValue("")
	m.updateTextareaHeight()
	if m.textareaHeight != 5 {
		t.Errorf("Height with empty content should be 5 (min), got %d", m.textareaHeight)
	}
}

// TestTaskInput_MaxHeightCalculation verifies the 50% screen calculation
func TestTaskInput_MaxHeightCalculation(t *testing.T) {
	testCases := []struct {
		name              string
		screenHeight      int
		expectedMaxHeight int
	}{
		{"Small screen (20)", 20, 8},   // (20-4) * 50% = 8
		{"Medium screen (40)", 40, 18}, // (40-4) * 50% = 18
		{"Large screen (60)", 60, 28},  // (60-4) * 50% = 28
		{"Very small (10)", 10, 5},     // (10-4) * 50% = 3, but min is 5
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewTaskInputWithOptions(nil, true)
			windowMsg := tea.WindowSizeMsg{
				Width:  100,
				Height: tc.screenHeight,
			}
			_, _ = m.Update(windowMsg)

			if m.textareaMaxHeight != tc.expectedMaxHeight {
				t.Errorf("For screen height %d, expected max height %d, got %d",
					tc.screenHeight, tc.expectedMaxHeight, m.textareaMaxHeight)
			}
		})
	}
}
