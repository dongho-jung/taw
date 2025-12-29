// Package tui provides terminal user interface components for TAW.
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

const textareaZoneID = "task-textarea"

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
	ta.Prompt = "" // Clear prompt to avoid extra characters on the left
	ta.SetWidth(80)
	ta.SetHeight(10)

	// Custom styling using v1 API
	ta.FocusedStyle.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1)
	ta.BlurredStyle.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Prompt = lipgloss.NewStyle()

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

	case tea.MouseMsg:
		// Handle mouse click in textarea zone using bubblezone
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if zone.Get(textareaZoneID).InBounds(msg) {
				// Click is within textarea - focus it and move cursor to clicked line
				m.textarea.Focus()

				// Calculate relative position within textarea
				// The textarea starts at approximately y=3 (after title + newlines)
				textareaStartY := 3

				if msg.Y >= textareaStartY {
					relativeY := msg.Y - textareaStartY

					// Move cursor to the clicked line
					currentLine := m.textarea.Line()

					// Move to target line
					lineDiff := relativeY - currentLine
					if lineDiff > 0 {
						for i := 0; i < lineDiff; i++ {
							m.textarea.CursorDown()
						}
					} else if lineDiff < 0 {
						for i := 0; i < -lineDiff; i++ {
							m.textarea.CursorUp()
						}
					}
				}
			}
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
	// Wrap textarea with bubblezone for mouse click handling
	sb.WriteString(zone.Mark(textareaZoneID, m.textarea.View()))
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("Alt+Enter: Submit  |  Esc: Cancel  |  Click to position cursor"))

	return zone.Scan(sb.String())
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
	// Initialize bubblezone manager
	zone.NewGlobal()

	m := NewTaskInput()
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	input := finalModel.(*TaskInput)
	result := input.Result()
	return &result, nil
}
