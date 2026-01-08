// Package tui provides terminal user interface components for PAW.
package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
)

// TaskOptsField represents the current field being edited.
type TaskOptsField int

const (
	TaskOptsFieldModel TaskOptsField = iota
	TaskOptsFieldUltrathink
)

const taskOptsFieldCount = 2

// TaskOptsUI provides an interactive task options form.
type TaskOptsUI struct {
	options   *config.TaskOptions
	field     TaskOptsField
	modelIdx  int
	width     int
	height    int
	done      bool
	cancelled bool
	isDark    bool // Cached dark mode detection (must be detected before bubbletea starts)
}

// TaskOptsResult contains the result of the task options UI.
type TaskOptsResult struct {
	Options   *config.TaskOptions
	Cancelled bool
}

// NewTaskOptsUI creates a new task options UI.
func NewTaskOptsUI(currentOpts *config.TaskOptions, activeTasks []string) *TaskOptsUI {
	// Detect dark mode BEFORE bubbletea starts (HasDarkBackground reads from stdin)
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	opts := config.DefaultTaskOptions()
	if currentOpts != nil {
		opts.Merge(currentOpts)
	}

	// Find current model index
	modelIdx := 0
	models := config.ValidModels()
	for i, m := range models {
		if m == opts.Model {
			modelIdx = i
			break
		}
	}

	return &TaskOptsUI{
		options:  opts,
		field:    TaskOptsFieldModel,
		modelIdx: modelIdx,
		isDark:   isDark,
	}
}

// Init initializes the task options UI.
func (m *TaskOptsUI) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model.
func (m *TaskOptsUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.cancelled = true
			m.done = true
			return m, tea.Quit

		case "enter":
			m.done = true
			return m, tea.Quit

		case "tab", "down", "j":
			m.field = (m.field + 1) % taskOptsFieldCount
			return m, nil

		case "shift+tab", "up", "k":
			m.field = (m.field - 1 + taskOptsFieldCount) % taskOptsFieldCount
			return m, nil

		case "left", "h":
			m.handleLeft()
			return m, nil

		case "right", "l":
			m.handleRight()
			return m, nil

		case " ":
			// Space toggles for toggle fields
			if m.field == TaskOptsFieldUltrathink {
				m.options.Ultrathink = !m.options.Ultrathink
				return m, nil
			}
		}
	}

	return m, nil
}

func (m *TaskOptsUI) handleLeft() {
	switch m.field {
	case TaskOptsFieldModel:
		if m.modelIdx > 0 {
			m.modelIdx--
			m.options.Model = config.ValidModels()[m.modelIdx]
		}
	case TaskOptsFieldUltrathink:
		// Left moves to [on] which is visually on the left
		m.options.Ultrathink = true
	}
}

func (m *TaskOptsUI) handleRight() {
	switch m.field {
	case TaskOptsFieldModel:
		models := config.ValidModels()
		if m.modelIdx < len(models)-1 {
			m.modelIdx++
			m.options.Model = models[m.modelIdx]
		}
	case TaskOptsFieldUltrathink:
		// Right moves to [off] which is visually on the right
		m.options.Ultrathink = false
	}
}

// View renders the task options UI.
func (m *TaskOptsUI) View() tea.View {
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

	labelStyle := lipgloss.NewStyle().
		Foreground(normalColor).
		Width(20)

	selectedLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Width(20)

	valueStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	selectedValueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	helpStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		MarginTop(1)

	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Task Options"))
	sb.WriteString("\n\n")

	// Model field
	{
		label := labelStyle.Render("Model:")
		if m.field == TaskOptsFieldModel {
			label = selectedLabelStyle.Render("Model:")
		}
		sb.WriteString(label)

		models := config.ValidModels()
		var modelParts []string
		for i, model := range models {
			text := string(model)
			if i == m.modelIdx {
				if m.field == TaskOptsFieldModel {
					text = selectedValueStyle.Render("[" + text + "]")
				} else {
					text = valueStyle.Render("[" + text + "]")
				}
			} else {
				text = dimStyle.Render(" " + text + " ")
			}
			modelParts = append(modelParts, text)
		}
		sb.WriteString(strings.Join(modelParts, " "))
		sb.WriteString("\n")
	}

	// Ultrathink field
	{
		label := labelStyle.Render("Ultrathink:")
		if m.field == TaskOptsFieldUltrathink {
			label = selectedLabelStyle.Render("Ultrathink:")
		}
		sb.WriteString(label)

		var onText, offText string
		if m.options.Ultrathink {
			if m.field == TaskOptsFieldUltrathink {
				onText = selectedValueStyle.Render("[on]")
			} else {
				onText = valueStyle.Render("[on]")
			}
			offText = dimStyle.Render(" off ")
		} else {
			onText = dimStyle.Render(" on ")
			if m.field == TaskOptsFieldUltrathink {
				offText = selectedValueStyle.Render("[off]")
			} else {
				offText = valueStyle.Render("[off]")
			}
		}
		sb.WriteString(onText + " " + offText)
		sb.WriteString("\n")
	}

	// Help text
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("Tab/↓: Next field  |  Shift+Tab/↑: Prev field  |  ←/→: Change value  |  Enter: Save  |  Esc: Cancel"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

// Result returns the task options result.
func (m *TaskOptsUI) Result() TaskOptsResult {
	return TaskOptsResult{
		Options:   m.options,
		Cancelled: m.cancelled,
	}
}

// RunTaskOptsUI runs the task options UI and returns the result.
func RunTaskOptsUI(currentOpts *config.TaskOptions, activeTasks []string) (*TaskOptsResult, error) {
	m := NewTaskOptsUI(currentOpts, activeTasks)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	ui := finalModel.(*TaskOptsUI)
	result := ui.Result()
	return &result, nil
}
