package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/mattn/go-runewidth"
)

// handleTextareaMouse handles mouse click in the textarea area.
// Returns the row, column, and whether the click was valid.
func (m *TaskInput) handleTextareaMouse(x, y int) (int, int, bool) {
	if m.focusPanel != FocusPanelLeft {
		return 0, 0, false
	}

	m.textarea.Focus()

	textareaStartY := 2 // Account for help text line (1) + top border (1)
	textareaStartX := 2

	targetRow := y - textareaStartY
	targetCol := x - textareaStartX

	if targetRow < 0 {
		return 0, 0, false
	}

	if targetCol < 0 {
		targetCol = 0
	}

	if cursor := m.textarea.Cursor(); cursor != nil {
		currentRow := cursor.Y

		switch {
		case targetRow > currentRow:
			steps := targetRow - currentRow
			for i := 0; i < steps; i++ {
				prev := m.textarea.Cursor()
				m.textarea.CursorDown()
				next := m.textarea.Cursor()
				if next == nil || (prev != nil && next.Y == prev.Y) {
					break
				}
			}
		case targetRow < currentRow:
			steps := currentRow - targetRow
			for i := 0; i < steps; i++ {
				prev := m.textarea.Cursor()
				m.textarea.CursorUp()
				next := m.textarea.Cursor()
				if next == nil || (prev != nil && next.Y == prev.Y) {
					break
				}
			}
		}
	}

	m.moveCursorToVisualColumn(targetCol)

	row, col := m.textarea.CursorPosition()
	return row, col, true
}

// moveCursorToVisualColumn moves the cursor to the specified visual column.
func (m *TaskInput) moveCursorToVisualColumn(targetCol int) {
	lines := strings.Split(m.textarea.Value(), "\n")
	row := m.textarea.Line()
	if row < 0 || row >= len(lines) {
		return
	}

	lineInfo := m.textarea.LineInfo()
	runes := []rune(lines[row])

	start := min(lineInfo.StartColumn, len(runes))
	col := start
	width := 0

	if targetCol < 0 {
		targetCol = 0
	}
	if lineInfo.CharWidth > 0 {
		targetCol = min(targetCol, lineInfo.CharWidth)
	}

	for idx := start; idx < len(runes); idx++ {
		rw := runewidth.RuneWidth(runes[idx])
		if rw <= 0 {
			rw = 1
		}

		if width+rw > targetCol {
			break
		}

		width += rw
		col = idx + 1
	}

	m.textarea.SetCursorColumn(col)
}

// detectClickedPanel determines which panel was clicked based on mouse position.
func (m *TaskInput) detectClickedPanel(x, y int) FocusPanel {
	// Calculate approximate box boundaries
	// Textarea: starts at Y=1 (after help text), height = textareaHeight + 2 (border)
	// Options panel: same Y range as textarea, but to the right
	// Kanban: starts after top section, takes remaining space

	// Calculate textarea width using same adaptive logic as WindowSizeMsg handler
	const kanbanColumnGap = 8
	const minOptionsPanelWidth = 43
	kanbanColWidth := (m.width - kanbanColumnGap) / 4
	kanbanColDisplayWidth := kanbanColWidth + 2
	textareaHeightWithBorder := m.textareaHeight + 2 // Dynamic height + border

	// Detect narrow terminal and calculate textarea width accordingly
	isNarrow := kanbanColDisplayWidth < minOptionsPanelWidth
	var textareaWidth int
	if isNarrow {
		// Narrow mode: textarea spans 2 columns (Working + Waiting)
		textareaWidth = 2 * kanbanColDisplayWidth
	} else {
		// Normal mode: textarea spans 3 columns (Working + Waiting + Done)
		textareaWidth = 3 * kanbanColDisplayWidth
	}
	if textareaWidth < 30 {
		textareaWidth = 30
	}

	// Account for help text line at Y=0
	topSectionStart := 1
	topSectionEnd := topSectionStart + textareaHeightWithBorder

	// Check if click is in the top section (textarea or options)
	if y >= topSectionStart && y < topSectionEnd {
		// Left side = textarea, right side = options
		if x < textareaWidth+2 { // +2 for border
			return FocusPanelLeft
		}
		return FocusPanelRight
	}

	// Below top section = kanban (if visible)
	// Kanban is visible when height > 20 and width >= 70 (minColumnWidth*4 + columnGap)
	// This must match the condition in KanbanView.Render()
	if m.height > 20 && m.width >= 70 {
		return FocusPanelKanban
	}

	// Default to current focus if no clear match
	return m.focusPanel
}

// detectKanbanColumn determines which Kanban column was clicked based on X position.
// Returns column index (0-3) or -1 if outside column area.
func (m *TaskInput) detectKanbanColumn(x int) int {
	colWidth := m.kanban.ColumnWidth()
	if colWidth <= 0 {
		return -1
	}

	// Kanban columns start at X=0
	// Each column takes colWidth pixels
	col := x / colWidth
	if col >= 0 && col < 4 {
		return col
	}
	return -1
}

// getKanbanRelativeY converts absolute Y coordinate to kanban-relative row.
func (m *TaskInput) getKanbanRelativeY(y int) int {
	// Kanban starts after: help text (1) + textarea (dynamic height + 2 border)
	// Y=0: help, Y=1 to Y=(textareaHeight+2): topSection, Y=(textareaHeight+3): kanban starts
	kanbanStartY := 1 + m.textareaHeight + 2
	relY := y - kanbanStartY
	if relY < 0 {
		relY = 0
	}
	return relY
}

// getKanbanRelativeX converts absolute X coordinate to column-relative position.
// col is the column index (0-3). Returns X position relative to the column's start.
func (m *TaskInput) getKanbanRelativeX(x, col int) int {
	if col < 0 || col > 3 {
		return 0
	}
	colWidth := m.kanban.ColumnWidth()
	if colWidth <= 0 {
		return 0
	}
	// Each column starts at col * colWidth
	colStartX := col * colWidth
	relX := x - colStartX
	if relX < 0 {
		relX = 0
	}
	// Clamp to colWidth-1 (last valid position within column)
	// colWidth is the first position of the NEXT column
	if relX >= colWidth {
		relX = colWidth - 1
	}
	return relX
}

// switchFocusTo switches focus to the specified panel.
func (m *TaskInput) switchFocusTo(panel FocusPanel) {
	// Blur current panel
	switch m.focusPanel {
	case FocusPanelLeft:
		m.textarea.Blur()
	case FocusPanelKanban:
		m.kanban.SetFocused(false)
	}

	m.focusPanel = panel

	// Focus new panel
	switch panel {
	case FocusPanelLeft:
		m.textarea.Focus()
	case FocusPanelKanban:
		m.kanban.SetFocused(true)
	}
}

// updateKanbanPanel handles key events when the kanban panel is focused.
func (m *TaskInput) updateKanbanPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "up", "k":
		m.kanban.ScrollUp(1)
		return m, nil
	case "down", "j":
		m.kanban.ScrollDown(1)
		return m, nil
	case "pgup", "ctrl+u":
		m.kanban.ScrollUp(5)
		return m, nil
	case "pgdown", "ctrl+d":
		m.kanban.ScrollDown(5)
		return m, nil
	}

	return m, nil
}

// handleMouseScroll handles mouse wheel scroll events.
func (m *TaskInput) handleMouseScroll(msg tea.MouseWheelMsg) {
	// Scroll the panel under the mouse cursor
	clickedPanel := m.detectClickedPanel(msg.X, msg.Y)

	switch clickedPanel {
	case FocusPanelLeft:
		// Scroll the textarea by moving the cursor
		switch msg.Button {
		case tea.MouseWheelUp:
			m.textarea.CursorUp()
			m.textarea.EnsureCursorVisible()
		case tea.MouseWheelDown:
			m.textarea.CursorDown()
			m.textarea.EnsureCursorVisible()
		}
	case FocusPanelKanban:
		// Scroll the kanban view
		switch msg.Button {
		case tea.MouseWheelUp:
			m.kanban.ScrollUp(1)
		case tea.MouseWheelDown:
			m.kanban.ScrollDown(1)
		}
	}
}
