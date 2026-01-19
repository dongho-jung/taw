// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/service"
)

const (
	kanbanMinColumnWidth = 15
	kanbanColumnGap      = 6
	kanbanHeaderLines    = 2
	kanbanTaskIndent     = 2
)

// KanbanView renders a Kanban-style task board.
// The board has 3 columns: Working, Waiting, Done.
type KanbanView struct {
	width   int
	height  int
	isDark  bool
	service *service.TaskDiscoveryService

	// Cached task data (refreshed on tick, not on every render)
	// 3 columns: Working, Waiting, Done (Warning merged into Waiting)
	working []*service.DiscoveredTask
	waiting []*service.DiscoveredTask
	done    []*service.DiscoveredTask

	// Scroll state
	scrollOffset int
	focused      bool
	focusedCol   int // -1 = none, 0-2 = specific column

	// Task selection state (per column)
	// -1 = no task selected, 0+ = index of selected task
	selectedTaskIdx [3]int

	// Text selection state (column-aware)
	selecting     bool
	hasSelection  bool // True if a selection was made (persists until ClearSelection)
	selectColumn  int  // Column being selected (0-2), -1 if none
	selectStartX  int  // Start X position (relative to column)
	selectStartY  int  // Start row (relative to kanban top)
	selectEndX    int  // End X position (relative to column)
	selectEndY    int  // End row (relative to kanban top)
	selectedText  string
	renderedLines []string // Cache of rendered text lines for selection
}

// NewKanbanView creates a new Kanban view.
func NewKanbanView(isDark bool) *KanbanView {
	return &KanbanView{
		isDark:          isDark,
		service:         service.NewTaskDiscoveryService(),
		focusedCol:      -1,                 // No column focused initially
		selectColumn:    -1,                 // No column selected initially
		selectedTaskIdx: [3]int{-1, -1, -1}, // No task selected in any column
	}
}

// SetDarkMode updates the cached theme for adaptive rendering.
func (k *KanbanView) SetDarkMode(isDark bool) {
	k.isDark = isDark
}

// SetSize sets the view dimensions.
func (k *KanbanView) SetSize(width, height int) {
	k.width = width
	k.height = height
}

func (k *KanbanView) baseColumnWidth() int {
	if k.width < kanbanMinColumnWidth*3+kanbanColumnGap {
		return 0
	}
	return (k.width - kanbanColumnGap) / 3
}

func (k *KanbanView) columnContentWidth() int {
	columnWidth := k.baseColumnWidth()
	if columnWidth <= 0 {
		return 0
	}
	// columnWidth includes padding; subtract padding(2) + border(2) + indent(2).
	width := columnWidth - 6
	if width < 0 {
		return 0
	}
	return width
}

func (k *KanbanView) taskAreaHeight() int {
	if k.height <= 0 {
		return 0
	}
	maxHeight := k.height - 2 // Reserve space for border only
	contentHeight := maxHeight - kanbanHeaderLines
	if contentHeight < 0 {
		return 0
	}
	return contentHeight
}

// Refresh updates the cached task data by discovering all tasks.
// This should be called periodically (e.g., on tick) rather than on every render.
func (k *KanbanView) Refresh() {
	k.working, k.waiting, k.done = k.service.DiscoverAll()
}

// Render renders the Kanban board using cached task data.
// Call Refresh() first to update the cache.
func (k *KanbanView) Render() string {
	// Use cached task data (populated by Refresh())
	// 3 columns: Working, Waiting, Done (Warning merged into Waiting)
	working, waiting, done := k.working, k.waiting, k.done

	// Styles (adaptive to light/dark mode)
	lightDark := lipgloss.LightDark(k.isDark)
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))
	// Dim color: medium contrast for non-selected items (readable on various backgrounds)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("243"))

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(normalColor)

	taskNameStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	// Selected task style: inverted colors for visibility
	selectedTaskStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("231")). // White text
		Background(lipgloss.Color("39")).  // Blue background
		Bold(true)

	actionStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Italic(true)

	// Calculate column width (3 columns with gaps)
	// Minimum width per column
	// columnGap accounts for borders only (padding is included in lipgloss Width):
	// - In lipgloss v2, Width(X) sets content+padding width, then border is added
	// - Borders: 3 columns × 2 chars = 6
	// Note: Scrollbar (2 chars) is added separately after columns, not included here
	columnWidth := k.baseColumnWidth()
	if columnWidth == 0 {
		return ""
	}

	// Build each column
	// Colors are chosen to have good contrast on both light and dark backgrounds
	// Using bold text with medium-saturation colors for visibility
	workingColor := lightDark(lipgloss.Color("28"), lipgloss.Color("34"))   // Dark green / Bright green
	waitingColor := lightDark(lipgloss.Color("130"), lipgloss.Color("178")) // Dark orange / Gold
	doneColor := lightDark(lipgloss.Color("240"), lipgloss.Color("250"))    // Dark gray / Light gray

	columns := []struct {
		emoji string
		title string
		tasks []*service.DiscoveredTask
		color color.Color
	}{
		{constants.EmojiWorking, "Working", working, workingColor},
		{constants.EmojiWaiting, "Waiting", waiting, waitingColor},
		{constants.EmojiDone, "Done", done, doneColor},
	}

	var columnViews []string
	maxHeight := k.height - 2 // Reserve space for border only (no title)
	indent := strings.Repeat(" ", kanbanTaskIndent)

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
		colHeaderStyle := headerStyle.Foreground(col.color)
		header := fmt.Sprintf("%s %s (%d)", col.emoji, col.title, len(col.tasks))
		content.WriteString(colHeaderStyle.Render(header))
		content.WriteString("\n")
		content.WriteString(strings.Repeat("─", max(0, columnWidth-4)))
		content.WriteString("\n")

		// Tasks (limited by height, with scroll offset applied)
		// Each task shows: project/name (line 1), action/preview lines (line 2+)
		linesUsed := 2 // header + separator

		// Calculate available width for display
		// columnWidth - 6 accounts for padding and borders
		availableWidth := k.columnContentWidth()

		// Calculate dynamic action lines per task based on available height
		contentHeight := k.taskAreaHeight()
		actionLinesPerTask := calculateActionLinesPerTask(contentHeight, len(col.tasks))

		linesToSkip := k.scrollOffset

		for taskIdx, task := range col.tasks {
			if linesUsed >= maxHeight {
				break
			}

			// Full task display name: session/taskName (using camelCase for task name)
			camelTaskName := constants.ToCamelCase(task.Name)
			fullName := task.Session + "/" + camelTaskName

			// Determine if this task is selected
			isSelected := k.focused && k.focusedCol == colIdx && k.selectedTaskIdx[colIdx] == taskIdx

			// Build the display name (no metadata on name line anymore)
			displayName := fullName
			if task.Status == service.DiscoveredWaiting &&
				(task.StatusEmoji == constants.EmojiReview || task.StatusEmoji == constants.EmojiWarning) {
				displayName = task.StatusEmoji + " " + displayName
			}
			displayName = truncateWithEllipsis(displayName, availableWidth)

			// Build task lines for scrolling (name + detail lines)
			var taskLines []string
			if isSelected {
				taskLines = append(taskLines, selectedTaskStyle.Render(displayName))
			} else {
				taskLines = append(taskLines, taskNameStyle.Render(displayName))
			}

			detailLines := buildTaskDetailLines(task, actionLinesPerTask, availableWidth)
			for _, line := range detailLines {
				taskLines = append(taskLines, actionStyle.Render(indent+line))
			}

			// Apply scroll offset line-by-line
			if linesToSkip >= len(taskLines) {
				linesToSkip -= len(taskLines)
				continue
			}

			startIdx := 0
			if linesToSkip > 0 {
				startIdx = linesToSkip
				linesToSkip = 0
			}

			for i := startIdx; i < len(taskLines) && linesUsed < maxHeight; i++ {
				content.WriteString(taskLines[i])
				content.WriteString("\n")
				linesUsed++
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
	return len(k.working)+len(k.waiting)+len(k.done) > 0
}

// TaskCount returns the total number of cached tasks.
func (k *KanbanView) TaskCount() int {
	return len(k.working) + len(k.waiting) + len(k.done)
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

// SetFocusedColumn sets which column is focused (0-2), or -1 for none.
func (k *KanbanView) SetFocusedColumn(col int) {
	if col >= -1 && col < 3 {
		k.focusedCol = col
	}
}

// FocusedColumn returns the currently focused column index (-1 if none).
func (k *KanbanView) FocusedColumn() int {
	return k.focusedCol
}

// ColumnTaskCount returns the number of tasks in a specific column.
func (k *KanbanView) ColumnTaskCount(col int) int {
	switch col {
	case 0:
		return len(k.working)
	case 1:
		return len(k.waiting)
	case 2:
		return len(k.done)
	default:
		return 0
	}
}

// SelectedTaskIndex returns the selected task index for a column (-1 if none).
func (k *KanbanView) SelectedTaskIndex(col int) int {
	if col < 0 || col > 2 {
		return -1
	}
	return k.selectedTaskIdx[col]
}

// SetSelectedTaskIndex sets the selected task index for a column.
func (k *KanbanView) SetSelectedTaskIndex(col, idx int) {
	if col < 0 || col > 2 {
		return
	}
	taskCount := k.ColumnTaskCount(col)
	if taskCount == 0 {
		k.selectedTaskIdx[col] = -1
		return
	}
	// Clamp index to valid range
	if idx < 0 {
		idx = 0
	} else if idx >= taskCount {
		idx = taskCount - 1
	}
	k.selectedTaskIdx[col] = idx
}

// SelectPreviousTask moves selection up in the focused column.
func (k *KanbanView) SelectPreviousTask() {
	if k.focusedCol < 0 || k.focusedCol > 2 {
		return
	}
	taskCount := k.ColumnTaskCount(k.focusedCol)
	if taskCount == 0 {
		return
	}
	current := k.selectedTaskIdx[k.focusedCol]
	if current <= 0 {
		// Wrap to last task
		k.selectedTaskIdx[k.focusedCol] = taskCount - 1
	} else {
		k.selectedTaskIdx[k.focusedCol] = current - 1
	}
}

// SelectNextTask moves selection down in the focused column.
func (k *KanbanView) SelectNextTask() {
	if k.focusedCol < 0 || k.focusedCol > 2 {
		return
	}
	taskCount := k.ColumnTaskCount(k.focusedCol)
	if taskCount == 0 {
		return
	}
	current := k.selectedTaskIdx[k.focusedCol]
	if current < 0 || current >= taskCount-1 {
		// Wrap to first task
		k.selectedTaskIdx[k.focusedCol] = 0
	} else {
		k.selectedTaskIdx[k.focusedCol] = current + 1
	}
}

// InitializeColumnSelection initializes selection when a column gains focus.
// If the column has tasks and no selection, selects the first task.
func (k *KanbanView) InitializeColumnSelection(col int) {
	if col < 0 || col > 2 {
		return
	}
	taskCount := k.ColumnTaskCount(col)
	if taskCount > 0 && k.selectedTaskIdx[col] < 0 {
		k.selectedTaskIdx[col] = 0
	}
}

// GetSelectedTask returns the currently selected task in the focused column.
// Returns nil if no column is focused or no task is selected.
func (k *KanbanView) GetSelectedTask() *service.DiscoveredTask {
	if k.focusedCol < 0 || k.focusedCol > 2 {
		return nil
	}
	idx := k.selectedTaskIdx[k.focusedCol]
	if idx < 0 {
		return nil
	}

	var tasks []*service.DiscoveredTask
	switch k.focusedCol {
	case 0:
		tasks = k.working
	case 1:
		tasks = k.waiting
	case 2:
		tasks = k.done
	}

	if idx >= len(tasks) {
		return nil
	}
	return tasks[idx]
}

// ColumnWidth returns the width of each column (including border and padding).
func (k *KanbanView) ColumnWidth() int {
	// Must match the calculation in Render()
	columnWidth := k.baseColumnWidth()
	if columnWidth == 0 {
		return 0
	}
	return columnWidth + 2 // +2 for border only (padding is included in Width)
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
	visibleHeight := k.taskAreaHeight()
	if visibleHeight <= 0 || contentHeight <= visibleHeight {
		return 0
	}
	return contentHeight - visibleHeight
}

// maxTaskLinesInAnyColumn returns the max number of lines needed across all columns.
func (k *KanbanView) maxTaskLinesInAnyColumn() int {
	if k.baseColumnWidth() == 0 {
		return 0
	}
	contentHeight := k.taskAreaHeight()
	if contentHeight <= 0 {
		return 0
	}
	availableWidth := k.columnContentWidth()
	columns := [][]*service.DiscoveredTask{k.working, k.waiting, k.done}
	maxLines := 0
	for _, tasks := range columns {
		lines := 0
		actionLinesPerTask := calculateActionLinesPerTask(contentHeight, len(tasks))
		for _, task := range tasks {
			detailLines := buildTaskDetailLines(task, actionLinesPerTask, availableWidth)
			lines += 1 + len(detailLines)
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
	return k.taskAreaHeight()
}

// ContentHeight returns the total content height (max lines across columns).
func (k *KanbanView) ContentHeight() int {
	return k.maxTaskLinesInAnyColumn()
}

// renderScrollbar renders a vertical scrollbar for the kanban view.
func (k *KanbanView) renderScrollbar(visibleHeight int) string {
	return renderVerticalScrollbar(k.ContentHeight(), visibleHeight, k.scrollOffset, k.isDark)
}

// StartSelection starts text selection at the given position within a column.
// col is the column index (0-3), x is the position within the column, y is the row.
func (k *KanbanView) StartSelection(col, x, y int) {
	k.selecting = true
	k.hasSelection = true
	k.selectColumn = col
	k.selectStartX = x
	k.selectStartY = y
	k.selectEndX = x
	k.selectEndY = y
}

// ExtendSelection extends the selection to the given position.
// x is clamped to the same column where selection started.
func (k *KanbanView) ExtendSelection(x, y int) {
	if k.selecting {
		k.selectEndX = x
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
	k.hasSelection = false
	k.selectColumn = -1
	k.selectStartX = 0
	k.selectStartY = 0
	k.selectEndX = 0
	k.selectEndY = 0
	k.selectedText = ""
}

// HasSelection returns true if there's an active selection.
// Returns true during active dragging (selecting) or when a selection was made (hasSelection).
func (k *KanbanView) HasSelection() bool {
	return k.hasSelection
}

// GetSelectionRange returns the selection range (min row, max row).
func (k *KanbanView) GetSelectionRange() (int, int) {
	minY := k.selectStartY
	maxY := k.selectEndY
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return minY, maxY
}

// GetSelectionXRange returns the X selection range for a given row.
// Returns (startX, endX) accounting for selection direction.
func (k *KanbanView) GetSelectionXRange(row int) (int, int) {
	minY, maxY := k.GetSelectionRange()
	if row < minY || row > maxY {
		return 0, 0
	}

	// Determine if selection goes forward or backward
	forward := k.selectStartY < k.selectEndY || (k.selectStartY == k.selectEndY && k.selectStartX <= k.selectEndX)

	if minY == maxY {
		// Single row selection
		startX, endX := k.selectStartX, k.selectEndX
		if startX > endX {
			startX, endX = endX, startX
		}
		return startX, endX
	}

	// Multi-row selection
	if forward {
		switch row {
		case minY:
			return k.selectStartX, 9999 // Start row: from startX to end
		case maxY:
			return 0, k.selectEndX // End row: from start to endX
		}
	} else {
		switch row {
		case minY:
			return 0, k.selectEndX // End row (in reverse): from start to endX
		case maxY:
			return k.selectStartX, 9999 // Start row (in reverse): from startX to end
		}
	}
	return 0, 9999 // Middle rows: full width
}

// SelectionColumn returns the column index where selection is active (-1 if none).
func (k *KanbanView) SelectionColumn() int {
	return k.selectColumn
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
// Only copies text from within the selected column's boundaries.
func (k *KanbanView) CopySelection() error {
	if !k.HasSelection() || len(k.renderedLines) == 0 {
		return nil
	}

	if k.selectColumn < 0 || k.selectColumn > 2 {
		return nil // No valid column selected
	}

	minY, maxY := k.GetSelectionRange()

	// Calculate column boundaries (content area only, excluding border/padding)
	colWidth := k.ColumnWidth()
	if colWidth <= 0 {
		return nil
	}
	colStartX := k.selectColumn * colWidth
	// Content area excludes border(1) and padding(1) on each side
	contentStartX := colStartX + 2
	contentEndX := colStartX + colWidth - 2

	var selectedLines []string
	for i := minY; i <= maxY && i < len(k.renderedLines); i++ {
		if i < 0 {
			continue
		}

		line := k.renderedLines[i]
		if len(line) == 0 {
			selectedLines = append(selectedLines, "")
			continue
		}

		// Get the X range for this row
		selStartX, selEndX := k.GetSelectionXRange(i)

		// Convert to absolute X positions (display columns)
		absStartX := colStartX + selStartX
		absEndX := colStartX + selEndX

		// Get display width of the line for clamping
		lineDisplayWidth := runewidth.StringWidth(line)

		// Clamp positions: must stay within content area (excluding border/padding)
		absEndX = min(absEndX, contentEndX)
		absEndX = min(absEndX, lineDisplayWidth)
		absStartX = max(absStartX, contentStartX)
		absStartX = min(absStartX, lineDisplayWidth-1)

		// Skip if selection is outside valid range
		if absStartX >= absEndX || absStartX >= lineDisplayWidth {
			selectedLines = append(selectedLines, "")
			continue
		}

		// Convert display column positions to byte offsets
		byteStart := displayColToByteOffset(line, absStartX)
		byteEnd := displayColToByteOffset(line, absEndX)

		selectedLines = append(selectedLines, strings.TrimSpace(line[byteStart:byteEnd]))
	}

	k.selectedText = strings.Join(selectedLines, "\n")
	return clipboard.WriteAll(k.selectedText)
}

// displayColToByteOffset converts a display column position to a byte offset in the string.
// This accounts for multi-byte UTF-8 characters (like box-drawing chars) and wide characters.
func displayColToByteOffset(s string, displayCol int) int {
	if displayCol <= 0 {
		return 0
	}
	col := 0
	for i, r := range s {
		if col >= displayCol {
			return i
		}
		col += runewidth.RuneWidth(r)
	}
	return len(s)
}

// applySelectionHighlight applies selection highlight to the rendered board.
// Selection is column-aware: only highlights within the selected column's boundaries.
func (k *KanbanView) applySelectionHighlight(board string) string {
	if k.selectColumn < 0 || k.selectColumn > 2 {
		return board // No valid column selected
	}

	lines := strings.Split(board, "\n")
	minY, maxY := k.GetSelectionRange()

	// Calculate column boundaries (content area only, excluding border/padding)
	colWidth := k.ColumnWidth()
	if colWidth <= 0 {
		return board
	}
	colStartX := k.selectColumn * colWidth
	// Content area excludes border(1) and padding(1) on each side
	// Structure: │ content │  (border + padding + content + padding + border)
	contentStartX := colStartX + 2          // Skip left border and padding
	contentEndX := colStartX + colWidth - 2 // Exclude right padding and border

	// Selection highlight style with background color
	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("39")). // Blue background
		Foreground(lipgloss.Color("231")) // White text for contrast

	for i := range lines {
		if i < minY || i > maxY {
			continue
		}

		// Get the X range for this row
		selStartX, selEndX := k.GetSelectionXRange(i)

		// Convert to absolute X positions within the board (display columns)
		// selStartX/selEndX are relative to column start, so add colStartX
		absStartX := colStartX + selStartX
		absEndX := colStartX + selEndX

		// Strip ANSI codes to work with plain text positions
		plainLine := ansi.Strip(lines[i])
		if len(plainLine) == 0 {
			continue
		}

		// Get display width of the line for clamping
		lineDisplayWidth := runewidth.StringWidth(plainLine)

		// Clamp positions: must stay within content area (excluding border/padding)
		absEndX = min(absEndX, contentEndX)
		absEndX = min(absEndX, lineDisplayWidth)
		absStartX = max(absStartX, contentStartX)
		absStartX = min(absStartX, lineDisplayWidth-1)

		// Skip if selection is outside valid range
		if absStartX >= absEndX || absStartX >= lineDisplayWidth {
			continue
		}

		// Convert display column positions to byte offsets
		byteStart := displayColToByteOffset(plainLine, absStartX)
		byteEnd := displayColToByteOffset(plainLine, absEndX)

		// Build the highlighted line: before + highlighted + after
		before := plainLine[:byteStart]
		selected := plainLine[byteStart:byteEnd]
		after := plainLine[byteEnd:]

		lines[i] = before + highlightStyle.Render(selected) + after
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

// NextNonEmptyColumn finds the next column with tasks, starting from currentCol+1.
// Wraps around to column 0 if no column is found after currentCol.
// Returns -1 if all columns are empty.
func (k *KanbanView) NextNonEmptyColumn(currentCol int) int {
	// Try columns after currentCol first
	for i := 1; i <= 3; i++ {
		col := (currentCol + i) % 3
		if k.ColumnTaskCount(col) > 0 {
			return col
		}
	}
	return -1 // All columns empty
}

// PrevNonEmptyColumn finds the previous column with tasks, starting from currentCol-1.
// Wraps around to column 2 if no column is found before currentCol.
// Returns -1 if all columns are empty.
func (k *KanbanView) PrevNonEmptyColumn(currentCol int) int {
	// Try columns before currentCol first
	for i := 1; i <= 3; i++ {
		col := (currentCol - i + 3) % 3
		if k.ColumnTaskCount(col) > 0 {
			return col
		}
	}
	return -1 // All columns empty
}

// FirstNonEmptyColumn returns the first column (0-2) that has tasks.
// Returns -1 if all columns are empty.
func (k *KanbanView) FirstNonEmptyColumn() int {
	for col := 0; col < 3; col++ {
		if k.ColumnTaskCount(col) > 0 {
			return col
		}
	}
	return -1
}

// GetTaskAtPosition returns the task at the given column and row position.
// col is the column index (0-2: working, waiting, done).
// row is the Y position relative to the kanban area (0-indexed).
// Returns nil if no task is found at that position.
func (k *KanbanView) GetTaskAtPosition(col, row int) *service.DiscoveredTask {
	if col < 0 || col > 2 {
		return nil
	}

	contentHeight := k.taskAreaHeight()
	availableWidth := k.columnContentWidth()

	// Get the task list for the column
	var tasks []*service.DiscoveredTask
	switch col {
	case 0:
		tasks = k.working
	case 1:
		tasks = k.waiting
	case 2:
		tasks = k.done
	}

	if len(tasks) == 0 {
		return nil
	}

	// Account for header (2 lines: title + separator)
	// The border adds 1 line at the top, so content starts at row 1 within the rendered kanban
	// Layout within each column:
	// Row 0: top border
	// Row 1: header (emoji + title + count)
	// Row 2: separator (───)
	// Row 3+: tasks (each task takes 1+ lines)
	taskStartRow := 1 + kanbanHeaderLines

	// Adjust for scroll offset
	adjustedRow := row - taskStartRow + k.scrollOffset

	if adjustedRow < 0 {
		return nil
	}

	// Find which task is at the adjusted row
	// Each task takes 1 line (name) + N detail lines
	actionLinesPerTask := calculateActionLinesPerTask(contentHeight, len(tasks))
	currentLine := 0
	for _, task := range tasks {
		detailLines := buildTaskDetailLines(task, actionLinesPerTask, availableWidth)
		taskLines := 1 + len(detailLines)

		if adjustedRow >= currentLine && adjustedRow < currentLine+taskLines {
			return task
		}
		currentLine += taskLines
	}

	return nil
}

// buildMetadataString builds a display string from duration and tokens.
// Returns a string like "1m 36s · ↓ 5.9k" or just one of them if the other is empty.
func buildMetadataString(duration, tokens string) string {
	if duration == "" && tokens == "" {
		return ""
	}
	if duration == "" {
		return tokens
	}
	if tokens == "" {
		return duration
	}
	return duration + " · " + tokens
}

// extractLastSegment extracts only the last segment from a preview string.
// Segments are separated by ⏺ markers in Claude's output.
// This allows the kanban view to show only the most recent activity.
func extractLastSegment(preview string) string {
	lines := strings.Split(preview, "\n")
	lastSegmentStart := 0

	// Find the last segment start (line starting with ⏺)
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "⏺") {
			lastSegmentStart = i
			break
		}
	}

	// Return everything from the last segment start
	return strings.Join(lines[lastSegmentStart:], "\n")
}

func buildTaskDetailLines(task *service.DiscoveredTask, maxLines, availableWidth int) []string {
	if maxLines <= 0 || availableWidth <= kanbanTaskIndent {
		return nil
	}

	metadata := buildMetadataString(task.Duration, task.Tokens)
	var baseLines []string

	if task.Preview != "" {
		// Extract only the last segment (starting from the last ⏺ marker)
		lastSegment := extractLastSegment(task.Preview)
		for _, line := range strings.Split(lastSegment, "\n") {
			cleaned := normalizePreviewLine(line)
			if cleaned != "" {
				baseLines = append(baseLines, cleaned)
			}
		}
	}

	if task.CurrentAction != "" {
		action := strings.TrimSpace(task.CurrentAction)
		if action != "" {
			found := false
			for _, line := range baseLines {
				if line == action || strings.Contains(line, action) {
					found = true
					break
				}
			}
			if !found {
				baseLines = append(baseLines, action)
			}
		}
	}

	if metadata != "" {
		for _, line := range baseLines {
			if strings.Contains(line, metadata) {
				metadata = ""
				break
			}
		}
	}

	if len(baseLines) == 0 && metadata != "" {
		baseLines = append(baseLines, metadata)
	}

	if len(baseLines) == 0 {
		return nil
	}

	hasOnlyMetadata := len(baseLines) == 1 && baseLines[0] == metadata &&
		task.CurrentAction == "" && task.Preview == ""
	if metadata != "" && !hasOnlyMetadata {
		baseLines[len(baseLines)-1] = baseLines[len(baseLines)-1] + " · " + metadata
	}

	wrapWidth := availableWidth - kanbanTaskIndent
	if wrapWidth <= 0 {
		return nil
	}

	var wrapped []string
	for _, line := range baseLines {
		if line == "" {
			continue
		}
		for _, segment := range wrapByWidth(line, wrapWidth) {
			if segment != "" {
				wrapped = append(wrapped, segment)
			}
		}
	}

	if len(wrapped) == 0 {
		return nil
	}

	if len(wrapped) > maxLines {
		wrapped = wrapped[len(wrapped)-maxLines:]
	}

	return wrapped
}

func normalizePreviewLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "⏺") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "⏺"))
	}
	if strings.HasPrefix(trimmed, "✻") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "✻"))
	}
	// Filter out Claude Code keyboard hints and option toggle lines
	if isKeyboardHintLine(trimmed) {
		return ""
	}
	// Filter out separator lines (lines consisting only of dash-like characters)
	if isSeparatorLine(trimmed) {
		return ""
	}
	// Filter out prompt lines (❯ character)
	if strings.HasPrefix(trimmed, "❯") {
		return ""
	}
	return strings.TrimSpace(trimmed)
}

// isKeyboardHintLine checks if a line is a Claude Code keyboard hint or option toggle.
// These lines typically contain keyboard shortcuts or UI hints that aren't useful in Kanban view.
func isKeyboardHintLine(line string) bool {
	// Option toggle lines start with ⏵ (triangle indicators)
	if strings.HasPrefix(line, "⏵") {
		return true
	}
	// Lines containing keyboard hint patterns
	hintPatterns := []string{
		"(shift+tab",      // Cycle options hint
		"(shift-tab",      // Alternative format
		"(tab to cycle",   // Tab hint
		"(esc to",         // Escape key hint
		"(ctrl+c to",      // Ctrl+C hint
		"(ctrl-c to",      // Alternative format
		"(⌃c to",          // macOS ctrl notation
		"to interrupt)",   // Common suffix
		"to cancel)",      // Common suffix
	}
	lower := strings.ToLower(line)
	for _, pattern := range hintPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// isSeparatorLine checks if a line consists only of separator/dash-like characters.
// These are horizontal rule lines used in Claude's terminal output (e.g., "─────────").
func isSeparatorLine(line string) bool {
	if len(line) < 3 {
		return false // Too short to be a meaningful separator
	}
	for _, r := range line {
		// Check for common separator characters: box drawing horizontal, dashes, underscores
		switch r {
		case '─', '━', '-', '—', '–', '_', '═':
			// These are separator characters, continue checking
		default:
			return false // Found a non-separator character
		}
	}
	return true
}

func wrapByWidth(text string, width int) []string {
	if width <= 0 || text == "" {
		return nil
	}
	textWidth := ansi.StringWidth(text)
	if textWidth <= width {
		return []string{text}
	}

	var lines []string
	for start := 0; start < textWidth; start += width {
		end := start + width
		if end > textWidth {
			end = textWidth
		}
		lines = append(lines, ansi.Cut(text, start, end))
	}
	return lines
}

// calculateActionLinesPerTask calculates how many action lines to show per task
// based on available height and number of tasks in the column.
// This allows dynamic display of more content when there's plenty of space.
func calculateActionLinesPerTask(contentHeight, taskCount int) int {
	if taskCount == 0 {
		return 1
	}

	// Each task needs at least 1 line for the name
	// Remaining lines can be used for action content
	linesPerTask := contentHeight / taskCount
	actionLines := linesPerTask - 1 // Subtract 1 for the task name line

	// Clamp to reasonable bounds: minimum 1, maximum 3 action lines
	if actionLines < 1 {
		actionLines = 1
	}
	if actionLines > 3 {
		actionLines = 3
	}

	return actionLines
}
