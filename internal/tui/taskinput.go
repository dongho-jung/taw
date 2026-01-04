// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/v2/textarea"
	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
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
	OptFieldWorktreeHook
)

const optFieldCount = 5

// TaskInput provides an inline text input for task content.
type TaskInput struct {
	textarea    textarea.Model
	submitted   bool
	cancelled   bool
	width       int
	height      int
	options     *config.TaskOptions
	activeTasks []string // Active task names for dependency selection

	// Inline options editing
	focusPanel   FocusPanel
	optField     OptField
	modelIdx     int
	condIdx      int
	depTaskInput textinput.Model
	hookInput    textinput.Model
}

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
	ta.Styles.Focused.CursorLine = lipgloss.NewStyle()
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

	// Dependency task input
	depTaskInput := textinput.New()
	depTaskInput.Placeholder = "task-name"
	depTaskInput.CharLimit = 64
	depTaskInput.SetWidth(20)

	// Worktree hook input
	hookInput := textinput.New()
	hookInput.Placeholder = "npm install"
	hookInput.CharLimit = 256
	hookInput.SetWidth(20)

	return &TaskInput{
		textarea:     ta,
		width:        80,
		height:       15,
		options:      opts,
		activeTasks:  activeTasks,
		focusPanel:   FocusPanelLeft,
		optField:     OptFieldModel,
		modelIdx:     modelIdx,
		condIdx:      0,
		depTaskInput: depTaskInput,
		hookInput:    hookInput,
	}
}

// Init initializes the task input.
func (m *TaskInput) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles messages and updates the model.
func (m *TaskInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Adjust textarea size (leave space for options panel)
		newWidth := min(msg.Width-50, 80) // Leave room for options panel
		newHeight := min(msg.Height-8, 20)
		if newWidth > 40 {
			m.textarea.SetWidth(newWidth)
		}
		if newHeight > 5 {
			m.textarea.SetHeight(newHeight)
		}

	case tea.KeyMsg:
		keyStr := msg.String()

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
				m.updateOptionFocus()
			} else {
				m.focusPanel = FocusPanelLeft
				m.depTaskInput.Blur()
				m.hookInput.Blur()
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
		// Handle mouse click to position cursor in textarea
		if msg.Button == tea.MouseLeft && m.focusPanel == FocusPanelLeft {
			m.textarea.Focus()

			textareaStartY := 1
			textareaStartX := 2

			targetRow := msg.Y - textareaStartY
			targetCol := msg.X - textareaStartX

			if targetRow >= 0 {
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
	var cmd tea.Cmd

	keyStr := msg.String()

	switch keyStr {
	case "tab", "down", "j":
		m.applyOptionInputValues()
		m.optField = OptField((int(m.optField) + 1) % optFieldCount)
		m.updateOptionFocus()
		return m, nil

	case "shift+tab", "up", "k":
		m.applyOptionInputValues()
		m.optField = OptField((int(m.optField) - 1 + optFieldCount) % optFieldCount)
		m.updateOptionFocus()
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

	// Update text inputs if focused
	switch m.optField {
	case OptFieldDependsOnTask:
		m.depTaskInput, cmd = m.depTaskInput.Update(msg)
		return m, cmd
	case OptFieldWorktreeHook:
		m.hookInput, cmd = m.hookInput.Update(msg)
		return m, cmd
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
		m.options.Ultrathink = false
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
		m.options.Ultrathink = true
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

	if m.condIdx == 0 || m.depTaskInput.Value() == "" {
		m.options.DependsOn = nil
	} else {
		m.options.DependsOn = &config.TaskDependency{
			TaskName:  m.depTaskInput.Value(),
			Condition: conditions[m.condIdx],
		}
	}
}

// applyOptionInputValues applies text input values to options.
func (m *TaskInput) applyOptionInputValues() {
	m.updateDependsOn()
	m.options.WorktreeHook = m.hookInput.Value()
}

// updateOptionFocus updates focus state for option inputs.
func (m *TaskInput) updateOptionFocus() {
	m.depTaskInput.Blur()
	m.hookInput.Blur()

	switch m.optField {
	case OptFieldDependsOnTask:
		m.depTaskInput.Focus()
	case OptFieldWorktreeHook:
		m.hookInput.Focus()
	}
}

// View renders the task input.
func (m *TaskInput) View() tea.View {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)

	// Build left panel (task input) - no title, starts at same height as Options box
	var leftPanel strings.Builder
	leftPanel.WriteString(m.textarea.View())

	// Build right panel (options)
	rightPanel := m.renderOptionsPanel()

	// Join panels horizontally with gap
	gapStyle := lipgloss.NewStyle().Width(4)
	combined := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel.String(),
		gapStyle.Render(""),
		rightPanel,
	)

	// Add help text at bottom
	var sb strings.Builder
	sb.WriteString(combined)
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
func (m *TaskInput) renderOptionsPanel() string {
	isFocused := m.focusPanel == FocusPanelRight

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	titleDimStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("240")).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(12)

	selectedLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Width(12)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	selectedValueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	borderColor := lipgloss.Color("240")
	if isFocused {
		borderColor = lipgloss.Color("39")
	}
	panelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(36)

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

	// Depends on task field
	{
		isSelected := isFocused && m.optField == OptFieldDependsOnTask
		label := labelStyle.Render("Depends on:")
		if isSelected {
			label = selectedLabelStyle.Render("Depends on:")
		}
		content.WriteString(label)
		content.WriteString(m.depTaskInput.View())
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

	// Worktree hook field
	{
		isSelected := isFocused && m.optField == OptFieldWorktreeHook
		label := labelStyle.Render("Hook:")
		if isSelected {
			label = selectedLabelStyle.Render("Hook:")
		}
		content.WriteString(label)
		content.WriteString(m.hookInput.View())
		content.WriteString("\n")
	}

	return panelStyle.Render(content.String())
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
