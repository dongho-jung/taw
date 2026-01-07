// Package tui provides terminal user interface components for PAW.
package tui

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/dongho-jung/paw/internal/tui/textarea"
	"github.com/mattn/go-runewidth"

	"github.com/dongho-jung/paw/internal/config"
)

// FocusPanel represents which panel is currently focused.
type FocusPanel int

const (
	FocusPanelLeft  FocusPanel = iota // Task input textarea
	FocusPanelRight                   // Options panel
)

// OptField represents which option field is currently selected.
type OptField int

const (
	OptFieldModel OptField = iota
	OptFieldUltrathink
	OptFieldDependsOnTask
	OptFieldDependsOnCondition
)

const optFieldCount = 4

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
	condIdx    int
	depTaskIdx int // Index into activeTasks (0 = none, 1+ = task index)

	mouseSelecting  bool
	selectAnchorRow int
	selectAnchorCol int

	// Kanban view for tasks across all sessions
	kanban *KanbanView
}

// tickMsg is used for periodic Kanban refresh.
type tickMsg time.Time

// TaskInputResult contains the result of the task input.
type TaskInputResult struct {
	Content   string
	Options   *config.TaskOptions
	Cancelled bool
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
	ta.Placeholder = "Describe your task here...\n\nExamples:\n- Add user authentication\n- Fix bug in login form\n- Refactor API handlers"
	ta.Focus()
	ta.CharLimit = 0 // No limit
	ta.ShowLineNumbers = false
	ta.Prompt = "" // Clear prompt to avoid extra characters on the left
	ta.SetWidth(80)
	ta.SetHeight(10)

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
		condIdx:     0,
		depTaskIdx:  0, // 0 = no dependency
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

// tickCmd returns a command that triggers a tick after 3 seconds.
func (m *TaskInput) tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate Kanban height (about 1/3 of the screen, min 8 lines)
		kanbanHeight := max(8, msg.Height/3)
		m.kanban.SetSize(msg.Width, kanbanHeight)

		// Adjust textarea size (leave space for options panel and Kanban)
		// No max height cap - allow flexible scaling with terminal size
		newWidth := min(msg.Width-50, 80) // Leave room for options panel
		newHeight := msg.Height - kanbanHeight - 8 // Reserve space for Kanban, help text and borders
		if newWidth > 40 {
			m.textarea.SetWidth(newWidth)
		}
		if newHeight > 5 {
			m.textarea.SetHeight(newHeight)
		}

	case tea.KeyMsg:
		keyStr := msg.String()

		if keyStr == "ctrl+c" && m.focusPanel == FocusPanelLeft && m.textarea.HasSelection() {
			_ = m.textarea.CopySelection()
			return m, nil
		}

		// Global keys (work in both panels)
		switch keyStr {
		case "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit

		// Submit: Alt+Enter or F5
		case "alt+enter", "f5":
			m.applyOptionInputValues()
			content := strings.TrimSpace(m.textarea.Value())
			if content != "" {
				m.submitted = true
				return m, tea.Quit
			}
			return m, nil

		// Toggle panel: Alt+Tab (cycle between input box and options)
		case "alt+tab":
			m.applyOptionInputValues()
			if m.focusPanel == FocusPanelLeft {
				m.focusPanel = FocusPanelRight
				m.textarea.Blur()
			} else {
				m.focusPanel = FocusPanelLeft
				m.textarea.Focus()
			}
			return m, nil
		}

		// Panel-specific key handling
		if m.focusPanel == FocusPanelRight {
			return m.updateOptionsPanel(msg)
		}

		// Left panel (textarea) - handle mouse clicks below

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			if row, col, ok := m.handleTextareaMouse(msg.X, msg.Y); ok {
				m.mouseSelecting = true
				m.selectAnchorRow = row
				m.selectAnchorCol = col
				m.textarea.SetSelection(row, col, row, col)
			}
		}

	case tea.MouseMotionMsg:
		if msg.Button == tea.MouseLeft && m.mouseSelecting {
			if row, col, ok := m.handleTextareaMouse(msg.X, msg.Y); ok {
				m.textarea.SetSelection(m.selectAnchorRow, m.selectAnchorCol, row, col)
			}
		}

	case tea.MouseReleaseMsg:
		if msg.Button == tea.MouseLeft && m.mouseSelecting {
			m.mouseSelecting = false
			if !m.textarea.HasSelection() {
				m.textarea.ClearSelection()
			}
		}
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
	case OptFieldDependsOnTask:
		// Cycle through active tasks
		if len(m.activeTasks) > 0 && m.depTaskIdx > 0 {
			m.depTaskIdx--
			m.updateDependsOn()
		}
	case OptFieldDependsOnCondition:
		if m.condIdx > 0 {
			m.condIdx--
			m.updateDependsOn()
		}
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
	case OptFieldDependsOnTask:
		// Cycle through active tasks
		if len(m.activeTasks) > 0 && m.depTaskIdx < len(m.activeTasks) {
			m.depTaskIdx++
			m.updateDependsOn()
		}
	case OptFieldDependsOnCondition:
		conditions := []config.DependsOnCondition{
			config.DependsOnNone,
			config.DependsOnSuccess,
			config.DependsOnFailure,
			config.DependsOnAlways,
		}
		if m.condIdx < len(conditions)-1 {
			m.condIdx++
			m.updateDependsOn()
		}
	}
}

// updateDependsOn updates the dependency options based on current state.
func (m *TaskInput) updateDependsOn() {
	conditions := []config.DependsOnCondition{
		config.DependsOnNone,
		config.DependsOnSuccess,
		config.DependsOnFailure,
		config.DependsOnAlways,
	}

	// depTaskIdx: 0 = none, 1+ = index into activeTasks
	if m.condIdx == 0 || m.depTaskIdx == 0 || len(m.activeTasks) == 0 {
		m.options.DependsOn = nil
	} else {
		taskName := m.activeTasks[m.depTaskIdx-1]
		m.options.DependsOn = &config.TaskDependency{
			TaskName:  taskName,
			Condition: conditions[m.condIdx],
		}
	}
}

// applyOptionInputValues applies current selection values to options.
func (m *TaskInput) applyOptionInputValues() {
	m.updateDependsOn()
}

// View renders the task input.
func (m *TaskInput) View() tea.View {
	// Adaptive color for help text (use cached isDark value)
	lightDark := lipgloss.LightDark(m.isDark)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))

	helpStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		MarginTop(1)

	// Build left panel (task input)
	var leftPanel strings.Builder
	leftPanel.WriteString(m.textarea.View())

	// Build right panel (options)
	rightPanel := m.renderOptionsPanel()

	// Join panels horizontally with gap
	gapStyle := lipgloss.NewStyle().Width(4)
	topSection := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel.String(),
		gapStyle.Render(""),
		rightPanel,
	)

	// Build content with Kanban below
	var sb strings.Builder
	sb.WriteString(topSection)
	sb.WriteString("\n")

	// Add Kanban view if there's enough space
	if m.height > 20 {
		kanbanContent := m.kanban.Render()
		if kanbanContent != "" {
			sb.WriteString("\n")
			sb.WriteString(kanbanContent)
		}
	}

	// Add help text at bottom
	sb.WriteString("\n")
	if m.focusPanel == FocusPanelLeft {
		sb.WriteString(helpStyle.Render("Alt+Enter/F5: Submit  |  ⌥Tab: Options  |  Esc: Cancel"))
	} else {
		sb.WriteString(helpStyle.Render("↑/↓: Navigate  |  ←/→: Change  |  ⌥Tab: Task  |  Alt+Enter: Submit"))
	}

	v := tea.NewView(sb.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	// Set real cursor based on focus
	if m.focusPanel == FocusPanelLeft {
		if cursor := m.textarea.Cursor(); cursor != nil {
			cursor.Y += 1 // Only account for top border (no title anymore)
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
		Padding(0, 2). // No vertical padding - Options title provides spacing
		Width(41)      // Wider to accommodate content

	var content strings.Builder

	if isFocused {
		content.WriteString(titleStyle.Render("Options"))
	} else {
		content.WriteString(titleDimStyle.Render("Options"))
	}
	content.WriteString("\n")

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

	// Depends on task field (dropdown selector)
	{
		isSelected := isFocused && m.optField == OptFieldDependsOnTask
		label := labelStyle.Render("Depends on:")
		if isSelected {
			label = selectedLabelStyle.Render("Depends on:")
		}
		content.WriteString(label)

		if len(m.activeTasks) == 0 {
			content.WriteString(dimStyle.Render("(no tasks)"))
		} else {
			// Build options: [none] task1 task2 ...
			options := make([]string, 0, len(m.activeTasks)+1)
			options = append(options, "-") // none option
			options = append(options, m.activeTasks...)

			var parts []string
			for i, opt := range options {
				text := opt
				if len(text) > 10 {
					text = text[:9] + "…" // Truncate long names
				}
				if i == m.depTaskIdx {
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
		}
		content.WriteString("\n")
	}

	// Depends on condition field
	{
		isSelected := isFocused && m.optField == OptFieldDependsOnCondition
		label := labelStyle.Render("Run when:")
		if isSelected {
			label = selectedLabelStyle.Render("Run when:")
		}
		content.WriteString(label)

		conditions := []struct {
			val   config.DependsOnCondition
			label string
		}{
			{config.DependsOnNone, "-"},
			{config.DependsOnSuccess, "ok"},
			{config.DependsOnFailure, "fail"},
			{config.DependsOnAlways, "any"},
		}

		var parts []string
		for i, cond := range conditions {
			text := cond.label
			if i == m.condIdx {
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

// Result returns the task input result.
func (m *TaskInput) Result() TaskInputResult {
	m.applyOptionInputValues()
	return TaskInputResult{
		Content:   strings.TrimSpace(m.textarea.Value()),
		Options:   m.options,
		Cancelled: m.cancelled,
	}
}

// RunTaskInput runs the task input and returns the result.
func RunTaskInput() (*TaskInputResult, error) {
	return RunTaskInputWithTasks(nil)
}

// RunTaskInputWithTasks runs the task input with active task list and returns the result.
func RunTaskInputWithTasks(activeTasks []string) (*TaskInputResult, error) {
	m := NewTaskInputWithTasks(activeTasks)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	input := finalModel.(*TaskInput)
	result := input.Result()
	return &result, nil
}
