// Package tui provides terminal user interface components for PAW.
package tui

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dongho-jung/paw/internal/tui/textarea"
	"github.com/mattn/go-runewidth"

	"github.com/dongho-jung/paw/internal/config"
)

// FocusPanel represents which panel is currently focused.
type FocusPanel int

const (
	FocusPanelLeft   FocusPanel = iota // Task input textarea
	FocusPanelRight                    // Options panel
	FocusPanelKanban                   // Kanban view
)

// OptField represents which option field is currently selected.
type OptField int

const (
	OptFieldModel OptField = iota
	OptFieldUltrathink
)

const optFieldCount = 2

// cancelDoublePressTimeout is the time window for double-press cancel detection.
const cancelDoublePressTimeout = 2 * time.Second

// TaskInput provides an inline text input for task content.
type TaskInput struct {
	textarea    textarea.Model
	submitted   bool
	cancelled   bool
	width       int
	height      int
	options     *config.TaskOptions
	activeTasks []string // Active task names for dependency selection
	isDark      bool     // Cached dark mode detection (must be detected before bubbletea starts)

	// Inline options editing
	focusPanel FocusPanel
	optField   OptField
	modelIdx   int

	mouseSelecting  bool
	selectAnchorRow int
	selectAnchorCol int

	// Kanban mouse selection
	kanbanSelecting  bool
	kanbanSelectY    int // Y position relative to kanban area

	// Kanban view for tasks across all sessions
	kanban *KanbanView

	// Double-press cancel detection
	cancelPressTime time.Time

	// History search request
	historyRequested bool
}

// tickMsg is used for periodic Kanban refresh.
type tickMsg time.Time

// cancelClearMsg is used to clear the cancel pending state after timeout.
type cancelClearMsg struct{}

// TaskInputResult contains the result of the task input.
type TaskInputResult struct {
	Content          string
	Options          *config.TaskOptions
	Cancelled        bool
	HistoryRequested bool // True when Ctrl+R was pressed to show history
}

// NewTaskInput creates a new task input model.
func NewTaskInput() *TaskInput {
	return NewTaskInputWithTasks(nil)
}

// NewTaskInputWithTasks creates a new task input model with active task list.
func NewTaskInputWithTasks(activeTasks []string) *TaskInput {
	// Detect dark mode BEFORE bubbletea starts (HasDarkBackground reads from stdin)
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	ta := textarea.New()
	ta.Placeholder = "Describe your task here...\n\nExamples:\n- Add user authentication\n- Fix bug in login form"
	ta.Focus()
	ta.CharLimit = 0 // No limit
	ta.ShowLineNumbers = false
	ta.Prompt = "" // Clear prompt to avoid extra characters on the left
	ta.MaxHeight = 15 // Max 15 lines
	ta.SetWidth(80)
	ta.SetHeight(8) // Default 8 lines (min 5, max 15)

	// Enable real cursor for proper IME support (Korean input)
	ta.VirtualCursor = false

	// Custom styling using v2 API - assign directly to Styles field
	ta.Styles = textarea.DefaultStyles(true) // dark mode
	ta.Styles.Focused.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1)
	ta.Styles.Blurred.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)
	// Keep text and placeholder fully readable when blurred (border color already indicates focus)
	// Copy all text-related styles from focused to blurred to prevent dimming
	ta.Styles.Blurred.Text = ta.Styles.Focused.Text
	ta.Styles.Blurred.Placeholder = ta.Styles.Focused.Placeholder
	ta.Styles.Blurred.CursorLine = ta.Styles.Focused.CursorLine
	ta.Styles.Focused.CursorLine = lipgloss.NewStyle()
	ta.Styles.Blurred.CursorLine = lipgloss.NewStyle()
	ta.Styles.Focused.Prompt = lipgloss.NewStyle()
	ta.Styles.Blurred.Prompt = lipgloss.NewStyle()

	opts := config.DefaultTaskOptions()

	// Find model index
	modelIdx := 0
	for i, m := range config.ValidModels() {
		if m == opts.Model {
			modelIdx = i
			break
		}
	}

	return &TaskInput{
		textarea:    ta,
		width:       80,
		height:      15,
		options:     opts,
		activeTasks: activeTasks,
		isDark:      isDark,
		focusPanel:  FocusPanelLeft,
		optField:    OptFieldModel,
		modelIdx:    modelIdx,
		kanban:      NewKanbanView(isDark),
	}
}

// Init initializes the task input.
func (m *TaskInput) Init() tea.Cmd {
	// Refresh Kanban data on init
	m.kanban.Refresh()

	return tea.Batch(
		textarea.Blink,
		m.tickCmd(),
	)
}

// tickCmd returns a command that triggers a tick after 5 seconds.
func (m *TaskInput) tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update handles messages and updates the model.
func (m *TaskInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		// Refresh Kanban data on tick (expensive I/O is done here, not in View)
		m.kanban.Refresh()
		// Schedule next tick
		return m, m.tickCmd()

	case cancelClearMsg:
		// Clear the cancel pending state after timeout
		m.cancelPressTime = time.Time{}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Textarea height: default 8, min 5, max 15 (already set via MaxHeight)
		// Options panel height: set to 8 (inner) + 2 (border) = 10 total to match textarea
		const optionsPanelHeight = 10
		const textareaDefaultHeight = 8
		const textareaMinHeight = 5

		// Calculate textarea height based on available space
		// With border: internal + 2
		topSectionHeight := max(textareaDefaultHeight, optionsPanelHeight-2) + 2
		kanbanHeight := max(8, msg.Height-topSectionHeight-3) // -3 for help text + gap + statusline
		m.kanban.SetSize(msg.Width, kanbanHeight)

		// Options panel needs ~47 chars width (content 43 + padding 4 + border ~2)
		const optionsPanelWidth = 47
		// Textarea gets remaining width with minimal gap (1 char)
		newWidth := msg.Width - optionsPanelWidth - 1
		if newWidth > 30 {
			m.textarea.SetWidth(newWidth)
		}
		// Set textarea height (respecting min 5, max 15)
		textareaHeight := max(textareaMinHeight, textareaDefaultHeight)
		m.textarea.SetHeight(textareaHeight)

	case tea.KeyMsg:
		keyStr := msg.String()

		// Handle Ctrl+C for copying selection (textarea or kanban)
		if keyStr == "ctrl+c" {
			if m.focusPanel == FocusPanelLeft && m.textarea.HasSelection() {
				_ = m.textarea.CopySelection()
				return m, nil
			}
			if m.focusPanel == FocusPanelKanban && m.kanban.HasSelection() {
				_ = m.kanban.CopySelection()
				return m, nil
			}
		}

		// Global keys (work in both panels)
		switch keyStr {
		case "esc", "ctrl+c":
			// Double-press detection: require pressing twice within cancelDoublePressTimeout
			now := time.Now()
			if !m.cancelPressTime.IsZero() && now.Sub(m.cancelPressTime) <= cancelDoublePressTimeout {
				// Second press within timeout - cancel
				m.cancelled = true
				return m, tea.Quit
			}
			// First press or timeout - record time and wait for second press
			// Return a tick command to clear the pending state after timeout
			m.cancelPressTime = now
			return m, tea.Tick(cancelDoublePressTimeout, func(t time.Time) tea.Msg {
				return cancelClearMsg{}
			})

		// Submit: Alt+Enter or F5
		case "alt+enter", "f5":
			m.applyOptionInputValues()
			content := strings.TrimSpace(m.textarea.Value())
			if content != "" {
				m.submitted = true
				return m, tea.Quit
			}
			return m, nil

		// Toggle panel: Alt+Tab (cycle between input box, options, and kanban)
		case "alt+tab":
			m.applyOptionInputValues()
			switch m.focusPanel {
			case FocusPanelLeft:
				m.switchFocusTo(FocusPanelRight)
			case FocusPanelRight:
				// Switch to Kanban if visible, otherwise back to Left
				if m.height > 20 && m.kanban.HasTasks() {
					m.switchFocusTo(FocusPanelKanban)
				} else {
					m.switchFocusTo(FocusPanelLeft)
				}
			case FocusPanelKanban:
				m.switchFocusTo(FocusPanelLeft)
			}
			return m, nil

		// History search: Ctrl+R
		case "ctrl+r":
			m.historyRequested = true
			return m, tea.Quit
		}

		// Panel-specific key handling
		if m.focusPanel == FocusPanelRight {
			return m.updateOptionsPanel(msg)
		}

		// Kanban panel key handling
		if m.focusPanel == FocusPanelKanban {
			return m.updateKanbanPanel(msg)
		}

		// Left panel (textarea) - handle mouse clicks below

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			// Determine which panel was clicked and switch focus
			clickedPanel := m.detectClickedPanel(msg.X, msg.Y)
			if clickedPanel != m.focusPanel {
				m.applyOptionInputValues()
				m.switchFocusTo(clickedPanel)
			}

			// Handle textarea mouse selection if clicking in textarea
			if clickedPanel == FocusPanelLeft {
				// Clear Kanban selection when clicking textarea
				m.kanban.ClearSelection()
				m.kanbanSelecting = false

				if row, col, ok := m.handleTextareaMouse(msg.X, msg.Y); ok {
					m.mouseSelecting = true
					m.selectAnchorRow = row
					m.selectAnchorCol = col
					m.textarea.SetSelection(row, col, row, col)
				}
			}

			// Handle Kanban mouse selection
			if clickedPanel == FocusPanelKanban {
				// Clear textarea selection when clicking kanban
				m.textarea.ClearSelection()
				m.mouseSelecting = false

				col := m.detectKanbanColumn(msg.X, msg.Y)
				m.kanban.SetFocusedColumn(col)

				// Start Kanban text selection
				kanbanY := m.getKanbanRelativeY(msg.Y)
				m.kanbanSelecting = true
				m.kanbanSelectY = kanbanY
				m.kanban.StartSelection(kanbanY)
			}
		}

	case tea.MouseMotionMsg:
		if msg.Button == tea.MouseLeft {
			if m.mouseSelecting {
				if row, col, ok := m.handleTextareaMouse(msg.X, msg.Y); ok {
					m.textarea.SetSelection(m.selectAnchorRow, m.selectAnchorCol, row, col)
				}
			}
			if m.kanbanSelecting {
				kanbanY := m.getKanbanRelativeY(msg.Y)
				m.kanban.ExtendSelection(kanbanY)
			}
		}

	case tea.MouseReleaseMsg:
		if msg.Button == tea.MouseLeft {
			if m.mouseSelecting {
				m.mouseSelecting = false
				if !m.textarea.HasSelection() {
					m.textarea.ClearSelection()
				}
			}
			if m.kanbanSelecting {
				m.kanbanSelecting = false
				m.kanban.EndSelection()
			}
		}

	case tea.MouseWheelMsg:
		// Handle mouse scroll on the focused panel
		m.handleMouseScroll(msg)
	}

	// Update textarea if left panel is focused
	if m.focusPanel == FocusPanelLeft {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// updateOptionsPanel handles key events when the options panel is focused.
func (m *TaskInput) updateOptionsPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "tab", "down", "j":
		m.applyOptionInputValues()
		m.optField = OptField((int(m.optField) + 1) % optFieldCount)
		return m, nil

	case "shift+tab", "up", "k":
		m.applyOptionInputValues()
		m.optField = OptField((int(m.optField) - 1 + optFieldCount) % optFieldCount)
		return m, nil

	case "left", "h":
		m.handleOptionLeft()
		return m, nil

	case "right", "l":
		m.handleOptionRight()
		return m, nil

	case " ":
		// Space toggles for ultrathink
		if m.optField == OptFieldUltrathink {
			m.options.Ultrathink = !m.options.Ultrathink
			return m, nil
		}
	}

	return m, nil
}

// handleOptionLeft handles left arrow key in options panel.
func (m *TaskInput) handleOptionLeft() {
	switch m.optField {
	case OptFieldModel:
		if m.modelIdx > 0 {
			m.modelIdx--
			m.options.Model = config.ValidModels()[m.modelIdx]
		}
	case OptFieldUltrathink:
		// Left moves to [on] which is visually on the left
		m.options.Ultrathink = true
	}
}

// handleOptionRight handles right arrow key in options panel.
func (m *TaskInput) handleOptionRight() {
	switch m.optField {
	case OptFieldModel:
		models := config.ValidModels()
		if m.modelIdx < len(models)-1 {
			m.modelIdx++
			m.options.Model = models[m.modelIdx]
		}
	case OptFieldUltrathink:
		// Right moves to [off] which is visually on the right
		m.options.Ultrathink = false
	}
}

// applyOptionInputValues applies current selection values to options.
// Currently a no-op since Model and Ultrathink are applied immediately.
func (m *TaskInput) applyOptionInputValues() {
	// No-op: Model and Ultrathink are applied directly when changed
}

// isCancelPending returns true if we're waiting for the second ESC/Ctrl+C press.
func (m *TaskInput) isCancelPending() bool {
	return !m.cancelPressTime.IsZero()
}

// View renders the task input.
func (m *TaskInput) View() tea.View {
	// Adaptive color for help text (use cached isDark value)
	lightDark := lipgloss.LightDark(m.isDark)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))

	helpStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	// Build left panel (task input) with scrollbar if needed
	textareaView := m.textarea.View()

	contentLines := strings.Count(m.textarea.Value(), "\n") + 1
	visibleLines := m.textarea.Height()
	if contentLines > visibleLines && visibleLines > 0 {
		scrollOffset := m.textarea.Line() // Use cursor line as scroll indicator
		scrollbar := renderVerticalScrollbar(contentLines, visibleLines, scrollOffset, m.isDark)
		textareaView = embedScrollbarInTextarea(textareaView, scrollbar, visibleLines)
	}

	// Build right panel (options)
	rightPanel := m.renderOptionsPanel()

	// Join panels horizontally with minimal gap
	gapStyle := lipgloss.NewStyle().Width(1)
	topSection := lipgloss.JoinHorizontal(
		lipgloss.Top,
		textareaView,
		gapStyle.Render(""),
		rightPanel,
	)

	// Build content with version+tip at top-left and help text at top-right
	var sb strings.Builder

	// Version and tip style (same dim color as help text)
	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)
	tipStyle := helpStyle

	// Left side: PAW {version} Tip: {tip}
	versionText := versionStyle.Render("PAW " + Version)
	tipText := tipStyle.Render(" Tip: " + GetTip())
	leftContent := versionText + tipText
	leftWidth := lipgloss.Width(leftContent)

	// Show cancel pending hint if waiting for second press, otherwise show normal help text
	if m.isCancelPending() {
		// Cancel pending state - show prominent hint on the right
		cancelHintStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")). // Orange/yellow for visibility
			Bold(true)
		cancelHint := cancelHintStyle.Render("Press Esc again to cancel")
		hintWidth := lipgloss.Width(cancelHint)

		sb.WriteString(leftContent)
		gap := m.width - leftWidth - hintWidth
		if gap > 0 {
			sb.WriteString(strings.Repeat(" ", gap))
		}
		sb.WriteString(cancelHint)
	} else {
		// Determine help text based on focus panel
		var helpText string
		switch m.focusPanel {
		case FocusPanelLeft:
			helpText = "⌃R: History  |  Alt+Enter/F5: Submit  |  ⌥Tab: Options  |  Esc×2: Cancel"
		case FocusPanelRight:
			helpText = "↑/↓: Navigate  |  ←/→: Change  |  ⌥Tab: Tasks  |  Alt+Enter: Submit"
		case FocusPanelKanban:
			helpText = "↑/↓: Scroll  |  ⌥Tab: Input  |  Alt+Enter: Submit  |  Esc×2: Cancel"
		}

		// Add version+tip on left, help text on right
		helpRendered := helpStyle.Render(helpText)
		helpWidth := lipgloss.Width(helpRendered)

		sb.WriteString(leftContent)
		gap := m.width - leftWidth - helpWidth
		if gap > 0 {
			sb.WriteString(strings.Repeat(" ", gap))
		}
		sb.WriteString(helpRendered)
	}
	sb.WriteString("\n")

	sb.WriteString(topSection)
	sb.WriteString("\n")

	// Add Kanban view if there's enough space (no extra gap)
	if m.height > 20 {
		kanbanContent := m.kanban.Render()
		if kanbanContent != "" {
			sb.WriteString(kanbanContent)
		}
	}

	v := tea.NewView(sb.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	// Set real cursor based on focus
	if m.focusPanel == FocusPanelLeft {
		if cursor := m.textarea.Cursor(); cursor != nil {
			cursor.Y += 2 // Account for help text line + top border
			cursor.X += 1
			v.Cursor = cursor
		}
	}

	return v
}

// renderOptionsPanel renders the options panel for the right side.
// The panel auto-sizes to fit its content.
func (m *TaskInput) renderOptionsPanel() string {
	isFocused := m.focusPanel == FocusPanelRight

	// Adaptive colors for light/dark terminal themes (use cached isDark value)
	// Light theme: use darker colors for visibility on white background
	// Dark theme: use lighter colors for visibility on dark background
	lightDark := lipgloss.LightDark(m.isDark)
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	titleDimStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(dimColor).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(normalColor).
		Width(12)

	selectedLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Width(12)

	valueStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	selectedValueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	borderColor := dimColor
	if isFocused {
		borderColor = lipgloss.Color("39")
	}
	panelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 2).  // No vertical padding - Options title provides spacing
		Width(43).      // Wider to accommodate Model row without line wrapping
		Height(8)       // Match textarea internal height (8 rows)

	var content strings.Builder

	if isFocused {
		content.WriteString(titleStyle.Render("Options"))
	} else {
		content.WriteString(titleDimStyle.Render("Options"))
	}
	// Note: MarginBottom(1) on titleStyle already provides spacing, no explicit \n needed

	// Model field
	{
		isSelected := isFocused && m.optField == OptFieldModel
		label := labelStyle.Render("Model:")
		if isSelected {
			label = selectedLabelStyle.Render("Model:")
		}
		content.WriteString(label)

		models := config.ValidModels()
		var parts []string
		for i, model := range models {
			text := string(model)
			if i == m.modelIdx {
				if isSelected {
					text = selectedValueStyle.Render("[" + text + "]")
				} else {
					text = valueStyle.Render("[" + text + "]")
				}
			} else {
				text = dimStyle.Render(" " + text + " ")
			}
			parts = append(parts, text)
		}
		content.WriteString(strings.Join(parts, ""))
		content.WriteString("\n")
	}

	// Ultrathink field
	{
		isSelected := isFocused && m.optField == OptFieldUltrathink
		label := labelStyle.Render("Ultrathink:")
		if isSelected {
			label = selectedLabelStyle.Render("Ultrathink:")
		}
		content.WriteString(label)

		var onText, offText string
		if m.options.Ultrathink {
			if isSelected {
				onText = selectedValueStyle.Render("[on]")
			} else {
				onText = valueStyle.Render("[on]")
			}
			offText = dimStyle.Render(" off ")
		} else {
			onText = dimStyle.Render(" on ")
			if isSelected {
				offText = selectedValueStyle.Render("[off]")
			} else {
				offText = valueStyle.Render("[off]")
			}
		}
		content.WriteString(onText + offText)
		content.WriteString("\n")
	}

	return panelStyle.Render(content.String())
}

func (m *TaskInput) handleTextareaMouse(x, y int) (int, int, bool) {
	if m.focusPanel != FocusPanelLeft {
		return 0, 0, false
	}

	m.textarea.Focus()

	textareaStartY := 1
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
	// Textarea: starts at Y=1 (after help text), height = 8 + 2 (border) = 10
	// Options panel: same Y range as textarea, but to the right
	// Kanban: starts after top section, takes remaining space

	const optionsPanelWidth = 47
	textareaHeight := 10 // 8 rows + 2 for border
	textareaWidth := m.width - optionsPanelWidth - 1
	if textareaWidth < 30 {
		textareaWidth = 30
	}

	// Account for help text line at Y=0
	topSectionStart := 1
	topSectionEnd := topSectionStart + textareaHeight

	// Check if click is in the top section (textarea or options)
	if y >= topSectionStart && y < topSectionEnd {
		// Left side = textarea, right side = options
		if x < textareaWidth+2 { // +2 for border
			return FocusPanelLeft
		}
		return FocusPanelRight
	}

	// Below top section = kanban (if visible)
	// Kanban is visible when height > 20 and width >= 40 (minimum width for Kanban to render)
	if m.height > 20 && m.width >= 40 {
		return FocusPanelKanban
	}

	// Default to current focus if no clear match
	return m.focusPanel
}

// detectKanbanColumn determines which Kanban column was clicked based on X position.
// Returns column index (0-3) or -1 if outside column area.
func (m *TaskInput) detectKanbanColumn(x, y int) int {
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
	// Kanban starts after: help text (1) + textarea (10 with border) + gap (1)
	const kanbanStartY = 12
	relY := y - kanbanStartY
	if relY < 0 {
		relY = 0
	}
	return relY
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

func embedScrollbarInTextarea(view string, scrollbar string, visibleLines int) string {
	if visibleLines <= 0 || scrollbar == "" {
		return view
	}

	lines := strings.Split(view, "\n")
	if len(lines) < visibleLines+2 {
		return view
	}

	scrollLines := strings.Split(scrollbar, "\n")
	if len(scrollLines) < visibleLines {
		return view
	}

	for i := 0; i < visibleLines; i++ {
		lineIdx := i + 1 // Skip top border
		line := lines[lineIdx]
		width := ansi.StringWidth(line)
		if width < 3 {
			continue
		}

		// Replace the right padding cell so the scrollbar sits inside the border.
		targetCol := width - 2
		left := ansi.Cut(line, 0, targetCol)
		right := ansi.Cut(line, targetCol+1, width)
		lines[lineIdx] = left + scrollLines[i] + right
	}

	return strings.Join(lines, "\n")
}

// Result returns the task input result.
func (m *TaskInput) Result() TaskInputResult {
	m.applyOptionInputValues()
	return TaskInputResult{
		Content:          strings.TrimSpace(m.textarea.Value()),
		Options:          m.options,
		Cancelled:        m.cancelled,
		HistoryRequested: m.historyRequested,
	}
}

// SetContent sets the textarea content (for pre-filling from history).
func (m *TaskInput) SetContent(content string) {
	m.textarea.SetValue(content)
	// Move cursor to end
	m.textarea.CursorEnd()
}

// RunTaskInput runs the task input and returns the result.
func RunTaskInput() (*TaskInputResult, error) {
	return RunTaskInputWithTasks(nil)
}

// RunTaskInputWithTasks runs the task input with active task list and returns the result.
func RunTaskInputWithTasks(activeTasks []string) (*TaskInputResult, error) {
	return RunTaskInputWithTasksAndContent(activeTasks, "")
}

// RunTaskInputWithTasksAndContent runs the task input with active task list and initial content.
func RunTaskInputWithTasksAndContent(activeTasks []string, initialContent string) (*TaskInputResult, error) {
	m := NewTaskInputWithTasks(activeTasks)
	if initialContent != "" {
		m.SetContent(initialContent)
	}
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	input := finalModel.(*TaskInput)
	result := input.Result()
	return &result, nil
}
