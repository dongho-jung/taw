// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/service"
)

// KanbanView renders a Kanban-style task board.
type KanbanView struct {
	width   int
	height  int
	isDark  bool
	service *service.TaskDiscoveryService

	// Cached task data (refreshed on tick, not on every render)
	working []*service.DiscoveredTask
	waiting []*service.DiscoveredTask
	done    []*service.DiscoveredTask
	warning []*service.DiscoveredTask

	// Scroll state
	scrollOffset int
	focused      bool
	focusedCol   int // -1 = none, 0-3 = specific column

	// Text selection state
	selecting     bool
	selectStartY  int // Start row (relative to kanban top)
	selectEndY    int // End row (relative to kanban top)
	selectedText  string
	renderedLines []string // Cache of rendered text lines for selection
}

// NewKanbanView creates a new Kanban view.
func NewKanbanView(isDark bool) *KanbanView {
	return &KanbanView{
		isDark:     isDark,
		service:    service.NewTaskDiscoveryService(),
		focusedCol: -1, // No column focused initially
	}
}

// SetSize sets the view dimensions.
func (k *KanbanView) SetSize(width, height int) {
	k.width = width
	k.height = height
}

// Refresh updates the cached task data by discovering all tasks.
// This should be called periodically (e.g., on tick) rather than on every render.
func (k *KanbanView) Refresh() {
	k.working, k.waiting, k.done, k.warning = k.service.DiscoverAll()
}

// Render renders the Kanban board using cached task data.
// Call Refresh() first to update the cache.
func (k *KanbanView) Render() string {
	// Use cached task data (populated by Refresh())
	working, waiting, done, warning := k.working, k.waiting, k.done, k.warning

	// Styles (adaptive to light/dark mode)
	lightDark := lipgloss.LightDark(k.isDark)
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(normalColor)

	taskNameStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	actionStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Italic(true)

	// Calculate column width (4 columns with gaps)
	// Minimum width per column
	const minColumnWidth = 15
	const columnGap = 6
	if k.width < minColumnWidth*4+columnGap {
		return ""
	}
	columnWidth := (k.width - columnGap) / 4 // -6 for borders and gaps

	// Build each column
	columns := []struct {
		emoji string
		title string
		tasks []*service.DiscoveredTask
		color string
	}{
		{constants.EmojiWorking, "Working", working, "40"},  // Green
		{constants.EmojiWaiting, "Waiting", waiting, "220"}, // Yellow
		{constants.EmojiDone, "Done", done, "245"},          // Gray
		{constants.EmojiWarning, "Warning", warning, "203"}, // Red
	}

	var columnViews []string
	maxHeight := k.height - 2 // Reserve space for border only (no title)

	for colIdx, col := range columns {
		// Determine border color for this column
		borderColor := dimColor
		if k.focused && k.focusedCol == colIdx {
			borderColor = lipgloss.Color("39")
		}
		panelStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)
		var content strings.Builder

		// Column header
		colHeaderStyle := headerStyle.Foreground(lipgloss.Color(col.color))
		header := fmt.Sprintf("%s %s (%d)", col.emoji, col.title, len(col.tasks))
		content.WriteString(colHeaderStyle.Render(header))
		content.WriteString("\n")
		content.WriteString(strings.Repeat("─", max(0, columnWidth-4)))
		content.WriteString("\n")

		// Tasks (limited by height, with scroll offset applied)
		// Each task shows: project/name (line 1), current action if any (line 2)
		linesUsed := 2    // header + separator
		linesSkipped := 0 // Track lines skipped for scroll
		for _, task := range col.tasks {
			if linesUsed >= maxHeight {
				break
			}

			// Full task display name: session/taskName (using camelCase for task name)
			camelTaskName := constants.ToCamelCase(task.Name)
			fullName := task.Session + "/" + camelTaskName
			displayName := fullName
			if len(displayName) > columnWidth-6 {
				displayName = displayName[:columnWidth-7] + "…"
			}

			// Apply scroll offset - skip lines until we've scrolled past them
			if linesSkipped < k.scrollOffset {
				linesSkipped++
				// Also skip the action line if present
				if task.CurrentAction != "" {
					linesSkipped++
				}
				continue
			}

			content.WriteString(taskNameStyle.Render(displayName))
			content.WriteString("\n")
			linesUsed++

			// Show current action if available and space permits
			if task.CurrentAction != "" && linesUsed < maxHeight {
				action := task.CurrentAction
				// Leave room for indent and truncation
				maxActionLen := columnWidth - 8
				if maxActionLen > 0 {
					if len(action) > maxActionLen {
						action = action[:maxActionLen-1] + "…"
					}
					content.WriteString("  " + actionStyle.Render(action))
					content.WriteString("\n")
					linesUsed++
				}
			}
		}

		// Pad to consistent height
		for linesUsed < maxHeight {
			content.WriteString("\n")
			linesUsed++
		}

		colStyle := panelStyle.Width(columnWidth)
		columnViews = append(columnViews, colStyle.Render(content.String()))
	}

	// Combine columns horizontally
	board := lipgloss.JoinHorizontal(lipgloss.Top, columnViews...)

	// Add scrollbar if content overflows
	if k.NeedsScrollbar() {
		scrollbar := k.renderScrollbar(maxHeight - 2) // -2 for header lines
		board = lipgloss.JoinHorizontal(lipgloss.Top, board, " ", scrollbar)
	}

	// Apply selection highlighting and cache text for copying
	if k.HasSelection() {
		board = k.applySelectionHighlight(board)
	}

	// Cache rendered text for copy functionality (strip ANSI codes for plain text)
	k.cacheTextForCopy(board)

	return board
}

// HasTasks returns true if there are any cached tasks to display.
func (k *KanbanView) HasTasks() bool {
	return len(k.working)+len(k.waiting)+len(k.done)+len(k.warning) > 0
}

// TaskCount returns the total number of cached tasks.
func (k *KanbanView) TaskCount() int {
	return len(k.working) + len(k.waiting) + len(k.done) + len(k.warning)
}

// SetFocused sets the focus state of the kanban view.
func (k *KanbanView) SetFocused(focused bool) {
	k.focused = focused
	if !focused {
		k.focusedCol = -1 // Clear column focus when unfocusing
	}
}

// IsFocused returns whether the kanban view is focused.
func (k *KanbanView) IsFocused() bool {
	return k.focused
}

// SetFocusedColumn sets which column is focused (0-3), or -1 for none.
func (k *KanbanView) SetFocusedColumn(col int) {
	if col >= -1 && col < 4 {
		k.focusedCol = col
	}
}

// FocusedColumn returns the currently focused column index (-1 if none).
func (k *KanbanView) FocusedColumn() int {
	return k.focusedCol
}

// ColumnWidth returns the width of each column (including border).
func (k *KanbanView) ColumnWidth() int {
	if k.width < 40 {
		return 0
	}
	columnWidth := (k.width - 6) / 4
	if columnWidth < 15 {
		columnWidth = 15
	}
	return columnWidth + 2 // +2 for left and right border
}

// ScrollUp scrolls the kanban view up by n lines.
func (k *KanbanView) ScrollUp(n int) {
	k.scrollOffset -= n
	if k.scrollOffset < 0 {
		k.scrollOffset = 0
	}
}

// ScrollDown scrolls the kanban view down by n lines.
func (k *KanbanView) ScrollDown(n int) {
	maxOffset := k.maxScrollOffset()
	k.scrollOffset += n
	if k.scrollOffset > maxOffset {
		k.scrollOffset = maxOffset
	}
}

// ScrollOffset returns the current scroll offset.
func (k *KanbanView) ScrollOffset() int {
	return k.scrollOffset
}

// maxScrollOffset returns the maximum scroll offset.
func (k *KanbanView) maxScrollOffset() int {
	contentHeight := k.maxTaskLinesInAnyColumn()
	visibleHeight := k.height - 2 // Reserve for borders only (no title)
	if contentHeight <= visibleHeight {
		return 0
	}
	return contentHeight - visibleHeight
}

// maxTaskLinesInAnyColumn returns the max number of lines needed across all columns.
func (k *KanbanView) maxTaskLinesInAnyColumn() int {
	columns := [][]*service.DiscoveredTask{k.working, k.waiting, k.done, k.warning}
	maxLines := 0
	for _, tasks := range columns {
		lines := 0
		for _, task := range tasks {
			lines++ // Task name
			if task.CurrentAction != "" {
				lines++ // Current action
			}
		}
		if lines > maxLines {
			maxLines = lines
		}
	}
	return maxLines
}

// NeedsScrollbar returns true if the kanban view needs a scrollbar.
func (k *KanbanView) NeedsScrollbar() bool {
	return k.maxScrollOffset() > 0
}

// VisibleHeight returns the visible height of the kanban content area.
func (k *KanbanView) VisibleHeight() int {
	return k.height - 2
}

// ContentHeight returns the total content height (max lines across columns).
func (k *KanbanView) ContentHeight() int {
	return k.maxTaskLinesInAnyColumn()
}

// renderScrollbar renders a vertical scrollbar for the kanban view.
func (k *KanbanView) renderScrollbar(visibleHeight int) string {
	return renderVerticalScrollbar(k.ContentHeight(), visibleHeight, k.scrollOffset, k.isDark)
}

// StartSelection starts text selection at the given row.
func (k *KanbanView) StartSelection(y int) {
	k.selecting = true
	k.selectStartY = y
	k.selectEndY = y
}

// ExtendSelection extends the selection to the given row.
func (k *KanbanView) ExtendSelection(y int) {
	if k.selecting {
		k.selectEndY = y
	}
}

// EndSelection finalizes the selection.
func (k *KanbanView) EndSelection() {
	k.selecting = false
}

// ClearSelection clears the current selection.
func (k *KanbanView) ClearSelection() {
	k.selecting = false
	k.selectStartY = 0
	k.selectEndY = 0
	k.selectedText = ""
}

// HasSelection returns true if there's an active selection.
func (k *KanbanView) HasSelection() bool {
	return k.selectStartY != k.selectEndY || k.selecting
}

// GetSelectionRange returns the selection range (min, max row).
func (k *KanbanView) GetSelectionRange() (int, int) {
	minY := k.selectStartY
	maxY := k.selectEndY
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return minY, maxY
}

// IsRowSelected returns true if the given row is within the selection.
func (k *KanbanView) IsRowSelected(row int) bool {
	minY, maxY := k.GetSelectionRange()
	return row >= minY && row <= maxY
}

// CacheRenderedText stores the plain text version of each row for copying.
func (k *KanbanView) CacheRenderedText(lines []string) {
	k.renderedLines = lines
}

// CopySelection copies the selected text to clipboard and returns it.
func (k *KanbanView) CopySelection() error {
	if !k.HasSelection() || len(k.renderedLines) == 0 {
		return nil
	}

	minY, maxY := k.GetSelectionRange()
	var selectedLines []string
	for i := minY; i <= maxY && i < len(k.renderedLines); i++ {
		if i >= 0 {
			selectedLines = append(selectedLines, k.renderedLines[i])
		}
	}

	k.selectedText = strings.Join(selectedLines, "\n")
	return clipboard.WriteAll(k.selectedText)
}

// applySelectionHighlight applies selection highlight to the rendered board.
func (k *KanbanView) applySelectionHighlight(board string) string {
	lines := strings.Split(board, "\n")
	minY, maxY := k.GetSelectionRange()

	// Selection highlight style (inverted colors for visibility)
	highlightStyle := lipgloss.NewStyle().
		Reverse(true)

	for i := range lines {
		if i >= minY && i <= maxY {
			// Apply highlight to selected lines
			lines[i] = highlightStyle.Render(lines[i])
		}
	}

	return strings.Join(lines, "\n")
}

// cacheTextForCopy caches plain text version of rendered lines for copy functionality.
func (k *KanbanView) cacheTextForCopy(board string) {
	lines := strings.Split(board, "\n")
	k.renderedLines = make([]string, len(lines))
	for i, line := range lines {
		// Strip ANSI codes to get plain text
		k.renderedLines[i] = ansi.Strip(line)
	}
}
