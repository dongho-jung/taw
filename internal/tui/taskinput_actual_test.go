package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss/v2"
)

func TestTaskInput_RenderOptionsPanel_Actual(t *testing.T) {
	// Create actual TaskInput
	m := NewTaskInputWithTasks(nil)
	m.textareaHeight = 5 // Set consistent height
	m.isDark = true

	// Test with different focus states
	testCases := []struct {
		name       string
		focusPanel FocusPanel
		optField   OptField
		modelIdx   int
	}{
		{"Focused on Model, opus selected", FocusPanelRight, OptFieldModel, 0},
		{"Focused on Model, sonnet selected", FocusPanelRight, OptFieldModel, 1},
		{"Focused on Model, haiku selected", FocusPanelRight, OptFieldModel, 2},
		{"Not focused", FocusPanelLeft, OptFieldModel, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m.focusPanel = tc.focusPanel
			m.optField = tc.optField
			m.modelIdx = tc.modelIdx

			result := m.renderOptionsPanel()

			t.Logf("\n=== %s ===\n%s", tc.name, result)

			// Check all lines have same width
			lines := strings.Split(result, "\n")
			if len(lines) < 3 {
				t.Fatal("Not enough lines")
			}

			expectedWidth := lipgloss.Width(lines[0])
			for i, line := range lines {
				w := lipgloss.Width(line)
				if w != expectedWidth {
					t.Errorf("Line %d has different width: %d != %d", i, w, expectedWidth)
				}
			}

			// Check that Model line doesn't wrap (all models on same line)
			modelLineFound := false
			for _, line := range lines {
				if strings.Contains(line, "Model:") {
					modelLineFound = true
					// Should contain all three models
					if !strings.Contains(line, "opus") {
						t.Error("Model line missing 'opus'")
					}
					if !strings.Contains(line, "sonnet") {
						t.Error("Model line missing 'sonnet'")
					}
					if !strings.Contains(line, "haiku") {
						t.Error("Model line missing 'haiku'")
					}
				}
			}
			if !modelLineFound {
				t.Error("Model line not found")
			}
		})
	}
}
