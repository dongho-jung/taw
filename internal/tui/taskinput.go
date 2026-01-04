// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/v2/textarea"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/dongho-jung/paw/internal/config"
)

// TaskInput provides an inline text input for task content.
type TaskInput struct {
	textarea    textarea.Model
	submitted   bool
	cancelled   bool
	width       int
	height      int
	options     *config.TaskOptions
	showOptions bool // When true, options panel is shown
	optsUI      *TaskOptsUI
	activeTasks []string // Active task names for dependency selection
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

	return &TaskInput{
		textarea:    ta,
		width:       80,
		height:      15,
		options:     config.DefaultTaskOptions(),
		activeTasks: activeTasks,
	}
}

// Init initializes the task input.
func (m *TaskInput) Init() tea.Cmd {
	return textarea.Blink
}

// taskOptsResultMsg is sent when the options panel is closed.
type taskOptsResultMsg struct {
	options   *config.TaskOptions
	cancelled bool
}

// Update handles messages and updates the model.
func (m *TaskInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Handle options result message
	if resultMsg, ok := msg.(taskOptsResultMsg); ok {
		m.showOptions = false
		m.optsUI = nil
		if !resultMsg.cancelled {
			m.options = resultMsg.options
		}
		m.textarea.Focus()
		return m, nil
	}

	// If options panel is showing, delegate to it
	if m.showOptions && m.optsUI != nil {
		var optsModel tea.Model
		optsModel, cmd = m.optsUI.Update(msg)
		m.optsUI = optsModel.(*TaskOptsUI)
		cmds = append(cmds, cmd)

		// Check if options UI is done
		if m.optsUI.done {
			result := m.optsUI.Result()
			m.showOptions = false
			m.optsUI = nil
			if !result.Cancelled {
				m.options = result.Options
			}
			m.textarea.Focus()
		}
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Adjust textarea size
		newWidth := min(msg.Width-4, 100)
		newHeight := min(msg.Height-8, 20)
		if newWidth > 40 {
			m.textarea.SetWidth(newWidth)
		}
		if newHeight > 5 {
			m.textarea.SetHeight(newHeight)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit

		// Submit: Alt+Enter or F5
		case "alt+enter", "f5":
			content := strings.TrimSpace(m.textarea.Value())
			if content != "" {
				m.submitted = true
				return m, tea.Quit
			}
			// If empty, don't submit - just continue
			return m, nil

		// Open options panel: F2 (Ctrl+. may not work in all terminals)
		case "f2":
			m.showOptions = true
			m.optsUI = NewTaskOptsUI(m.options, m.activeTasks)
			m.textarea.Blur()
			return m, nil
		}

	case tea.MouseClickMsg:
		// Handle mouse click to position cursor in textarea
		// MouseClickMsg embeds Mouse directly, access fields directly
		if msg.Button == tea.MouseLeft {
			m.textarea.Focus()

			// Calculate textarea position from screen coordinates
			// Screen layout (based on user testing Y offset = +4):
			// Y=0: "New Task" title
			// Y=1: empty (MarginBottom)
			// Y=2: empty (\n)
			// Y=3: ╭── border top
			// Y=4+: textarea content lines
			textareaStartY := 4 // First content line
			textareaStartX := 2 // Border + padding offset

			targetRow := msg.Y - textareaStartY
			targetCol := msg.X - textareaStartX

			// Only reposition if click is within textarea content area
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

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the task input.
func (m *TaskInput) View() tea.View {
	// If options panel is showing, render it instead
	if m.showOptions && m.optsUI != nil {
		return m.optsUI.View()
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)

	optsIndicatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	optsValueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39"))

	var sb strings.Builder

	sb.WriteString(titleStyle.Render("New Task"))
	sb.WriteString("\n\n")
	sb.WriteString(m.textarea.View())
	sb.WriteString("\n")

	// Show current options summary
	optsStr := m.formatOptionsIndicator()
	if optsStr != "" {
		sb.WriteString(optsIndicatorStyle.Render("Options: "))
		sb.WriteString(optsValueStyle.Render(optsStr))
		sb.WriteString("\n")
	}

	sb.WriteString(helpStyle.Render("Alt+Enter/F5: Submit  |  F2: Options  |  Esc: Cancel"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	// Set real cursor from textarea for proper IME support
	if cursor := m.textarea.Cursor(); cursor != nil {
		// Offset cursor position (based on user testing):
		// Y=0: "New Task" title
		// Y=1: empty (MarginBottom)
		// Y=2: empty (\n)
		// Y=3: ╭── border top
		// Y=4+: textarea content (cursor Y=0 maps to screen Y=4)
		cursor.Y += 4
		cursor.X += 1 // Border only; padding already included in textarea cursor
		v.Cursor = cursor
	}

	return v
}

// formatOptionsIndicator returns a compact representation of current options.
func (m *TaskInput) formatOptionsIndicator() string {
	if m.options == nil {
		return ""
	}

	var parts []string

	// Model (only show if not default)
	if m.options.Model != config.DefaultModel {
		parts = append(parts, string(m.options.Model))
	}

	// Ultrathink (show if disabled, since it's on by default)
	if !m.options.Ultrathink {
		parts = append(parts, "no-ultrathink")
	}

	// Dependency
	if m.options.DependsOn != nil && m.options.DependsOn.TaskName != "" {
		parts = append(parts, fmt.Sprintf("after:%s(%s)",
			m.options.DependsOn.TaskName,
			m.options.DependsOn.Condition))
	}

	// Worktree hook
	if m.options.WorktreeHook != "" {
		hookPreview := m.options.WorktreeHook
		if len(hookPreview) > 20 {
			hookPreview = hookPreview[:17] + "..."
		}
		parts = append(parts, fmt.Sprintf("hook:%s", hookPreview))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, ", ")
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
