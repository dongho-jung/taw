package tui

import (
	"testing"
)

func TestHandleTextareaMouse_CoordinateCalculation(t *testing.T) {
	m := NewTaskInputWithTasks(nil)
	m.width = 100
	m.height = 30
	m.textareaHeight = 5
	m.focusPanel = FocusPanelLeft
	m.textarea.SetWidth(80)
	m.textarea.SetHeight(5)

	// Set some content in textarea
	m.textarea.SetValue("Line 1\nLine 2\nLine 3")

	// Debug: Check initial cursor position
	initialRow, initialCol := m.textarea.CursorPosition()
	t.Logf("Initial cursor position: row=%d, col=%d", initialRow, initialCol)
	if cursor := m.textarea.Cursor(); cursor != nil {
		t.Logf("Initial cursor.Y=%d", cursor.Y)
	}

	// Debug: Check what border size the style reports
	focusedBase := m.textarea.Styles.Focused.Base
	blurredBase := m.textarea.Styles.Blurred.Base
	t.Logf("Focused base border top size: %d", focusedBase.GetBorderTopSize())
	t.Logf("Blurred base border top size: %d", blurredBase.GetBorderTopSize())
	t.Logf("Textarea focused: %v", m.textarea.Focused())

	// Simulate clicking on first content line (screen Y=2)
	// Screen layout: Y=0 help, Y=1 border, Y=2 first content
	testCases := []struct {
		name      string
		clickX    int
		clickY    int
		expectRow int
		expectOK  bool
	}{
		{
			name:      "Click on first content line",
			clickX:    5,  // Should be within content
			clickY:    2,  // First content line (after help Y=0, border Y=1)
			expectRow: 0,  // Logical row 0
			expectOK:  true,
		},
		{
			name:      "Click on second content line",
			clickX:    5,
			clickY:    3,  // Second content line
			expectRow: 1,  // Logical row 1
			expectOK:  true,
		},
		{
			name:      "Click on third content line",
			clickX:    5,
			clickY:    4,  // Third content line
			expectRow: 2,  // Logical row 2
			expectOK:  true,
		},
		{
			name:      "Click on border (should fail)",
			clickX:    5,
			clickY:    1,  // Border line
			expectRow: 0,
			expectOK:  false,
		},
		{
			name:      "Click on help text (should fail via detectClickedPanel)",
			clickX:    5,
			clickY:    0,  // Help text line
			expectRow: 0,
			expectOK:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset cursor position
			m.textarea.SetValue("Line 1\nLine 2\nLine 3")

			row, col, ok := m.handleTextareaMouse(tc.clickX, tc.clickY)

			t.Logf("Click (%d, %d) -> row=%d, col=%d, ok=%v", tc.clickX, tc.clickY, row, col, ok)

			if ok != tc.expectOK {
				t.Errorf("Expected ok=%v, got ok=%v", tc.expectOK, ok)
			}

			if ok && row != tc.expectRow {
				t.Errorf("Expected row=%d, got row=%d", tc.expectRow, row)
			}
		})
	}
}

func TestGetKanbanRelativeY(t *testing.T) {
	m := NewTaskInputWithTasks(nil)
	m.textareaHeight = 5

	// Layout:
	// Y=0: Help text
	// Y=1 to Y=7: Textarea with borders (5 content + 2 border = 7 lines, indices 1-7)
	// Y=8: First kanban line (= 1 + 5 + 2 = 8)
	// Y=9: Second kanban line

	testCases := []struct {
		name     string
		clickY   int
		expected int
	}{
		{"First kanban line", 8, 0},
		{"Second kanban line", 9, 1},
		{"Third kanban line", 10, 2},
		{"Click above kanban (clamped to 0)", 5, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := m.getKanbanRelativeY(tc.clickY)
			t.Logf("getKanbanRelativeY(%d) = %d (expected %d)", tc.clickY, result, tc.expected)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestTextareaSelection_DragSelection(t *testing.T) {
	m := NewTaskInputWithTasks(nil)
	m.width = 100
	m.height = 30
	m.textareaHeight = 5
	m.focusPanel = FocusPanelLeft

	// Set content
	m.textarea.SetValue("Hello World\nSecond Line\nThird Line")

	// Simulate click to start selection on first line
	row1, col1, ok1 := m.handleTextareaMouse(4, 2) // Click on 'l' of "Hello" (approx)
	if !ok1 {
		t.Fatal("First click should succeed")
	}
	t.Logf("Start selection at row=%d, col=%d", row1, col1)

	// Set anchor and start selection
	m.mouseSelecting = true
	m.selectAnchorRow = row1
	m.selectAnchorCol = col1
	m.textarea.SetSelection(row1, col1, row1, col1)

	// Simulate drag to different position
	row2, col2, ok2 := m.handleTextareaMouse(10, 2) // Drag to later position
	if !ok2 {
		t.Fatal("Drag should succeed")
	}
	t.Logf("End selection at row=%d, col=%d", row2, col2)

	// Extend selection
	m.textarea.SetSelection(m.selectAnchorRow, m.selectAnchorCol, row2, col2)

	// Verify selection exists
	if !m.textarea.HasSelection() {
		t.Error("Selection should exist after drag")
	}

	selectedText := m.textarea.SelectedText()
	t.Logf("Selected text: %q", selectedText)

	if len(selectedText) == 0 {
		t.Error("Selected text should not be empty")
	}
}

func TestKanbanSelection_PersistsAfterEndSelection(t *testing.T) {
	kv := NewKanbanView(true)
	kv.SetSize(300, 20) // Set size for column width calculation

	// Test 1: Single-row selection should persist after EndSelection
	t.Run("Single row selection persists", func(t *testing.T) {
		kv.ClearSelection()

		// Simulate click on column 0, row 2, x position 5
		kv.StartSelection(0, 5, 2)
		if !kv.HasSelection() {
			t.Error("Should have selection during active drag")
		}

		// Simulate release without moving (single row selection)
		kv.EndSelection()
		if !kv.HasSelection() {
			t.Error("Single-row selection should persist after EndSelection")
		}

		minY, maxY := kv.GetSelectionRange()
		if minY != 2 || maxY != 2 {
			t.Errorf("Expected selection range [2, 2], got [%d, %d]", minY, maxY)
		}
	})

	// Test 2: Multi-row selection should persist after EndSelection
	t.Run("Multi row selection persists", func(t *testing.T) {
		kv.ClearSelection()

		// Simulate click on column 1, row 2, drag to row 5
		kv.StartSelection(1, 5, 2)
		kv.ExtendSelection(10, 5)
		kv.EndSelection()

		if !kv.HasSelection() {
			t.Error("Multi-row selection should persist after EndSelection")
		}

		minY, maxY := kv.GetSelectionRange()
		if minY != 2 || maxY != 5 {
			t.Errorf("Expected selection range [2, 5], got [%d, %d]", minY, maxY)
		}
	})

	// Test 3: Row 0 selection should work correctly
	t.Run("Row 0 selection works", func(t *testing.T) {
		kv.ClearSelection()

		// Select row 0 (first row) in column 2
		kv.StartSelection(2, 0, 0)
		kv.EndSelection()

		if !kv.HasSelection() {
			t.Error("Row 0 selection should persist after EndSelection")
		}

		minY, maxY := kv.GetSelectionRange()
		if minY != 0 || maxY != 0 {
			t.Errorf("Expected selection range [0, 0], got [%d, %d]", minY, maxY)
		}
	})

	// Test 4: ClearSelection should clear the selection
	t.Run("ClearSelection clears selection", func(t *testing.T) {
		kv.StartSelection(0, 5, 3)
		kv.EndSelection()

		if !kv.HasSelection() {
			t.Error("Should have selection before clear")
		}

		kv.ClearSelection()
		if kv.HasSelection() {
			t.Error("Should not have selection after ClearSelection")
		}
	})

	// Test 5: Reverse selection (drag upward)
	t.Run("Reverse selection works", func(t *testing.T) {
		kv.ClearSelection()

		// Simulate click on row 5, drag to row 2 (upward)
		kv.StartSelection(0, 10, 5)
		kv.ExtendSelection(5, 2)
		kv.EndSelection()

		if !kv.HasSelection() {
			t.Error("Reverse selection should persist")
		}

		minY, maxY := kv.GetSelectionRange()
		if minY != 2 || maxY != 5 {
			t.Errorf("Expected selection range [2, 5], got [%d, %d]", minY, maxY)
		}
	})

	// Test 6: Column tracking
	t.Run("Column tracking works", func(t *testing.T) {
		kv.ClearSelection()

		// Select in column 2
		kv.StartSelection(2, 5, 3)
		if kv.SelectionColumn() != 2 {
			t.Errorf("Expected selection column 2, got %d", kv.SelectionColumn())
		}

		kv.ClearSelection()
		if kv.SelectionColumn() != -1 {
			t.Errorf("Expected selection column -1 after clear, got %d", kv.SelectionColumn())
		}
	})
}
