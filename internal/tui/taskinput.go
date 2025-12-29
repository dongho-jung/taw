// Package tui provides terminal user interface components for TAW.
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TaskInput provides an inline text input for task content.
type TaskInput struct {
	textarea  textarea.Model
	submitted bool
	cancelled bool
	width     int
	height    int
}

// TaskInputResult contains the result of the task input.
type TaskInputResult struct {
	Content   string
	Cancelled bool
}

// NewTaskInput creates a new task input model.
func NewTaskInput() *TaskInput {
	ta := textarea.New()
	ta.Placeholder = "Describe your task here...\n\nExamples:\n- Add user authentication\n- Fix bug in login form\n- Refactor API handlers"
	ta.Focus()
	ta.CharLimit = 0 // No limit
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(10)

	// Custom styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1)
	ta.BlurredStyle.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	return &TaskInput{
		textarea: ta,
		width:    80,
		height:   15,
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

		// Submit: Alt+Enter, Ctrl+D, or Ctrl+S
		case "alt+enter", "ctrl+d", "ctrl+s":
			content := strings.TrimSpace(m.textarea.Value())
			if content != "" {
				m.submitted = true
				return m, tea.Quit
			}
			// If empty, don't submit - just continue
			return m, nil
		}
	}

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the task input.
func (m *TaskInput) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)

	var sb strings.Builder

	sb.WriteString(titleStyle.Render("New Task"))
	sb.WriteString("\n\n")
	sb.WriteString(m.textarea.View())
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("Alt+Enter: Submit  |  Esc: Cancel"))

	return sb.String()
}

// Result returns the task input result.
func (m *TaskInput) Result() TaskInputResult {
	return TaskInputResult{
		Content:   strings.TrimSpace(m.textarea.Value()),
		Cancelled: m.cancelled,
	}
}

// RunTaskInput runs the task input and returns the result.
func RunTaskInput() (*TaskInputResult, error) {
	m := NewTaskInput()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	input := finalModel.(*TaskInput)
	result := input.Result()
	return &result, nil
}
