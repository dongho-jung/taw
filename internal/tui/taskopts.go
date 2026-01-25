// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
)

// TaskOptsField represents the current field being edited.
type TaskOptsField int

// Task options field values.
const (
	TaskOptsFieldModel TaskOptsField = iota
)

const taskOptsFieldCount = 1

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
// Note: activeTasks is reserved for future dependency selection feature.
func NewTaskOptsUI(currentOpts *config.TaskOptions, _ []string) *TaskOptsUI {
	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

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
	return tea.RequestBackgroundColor
}

// Update handles messages and updates the model.
func (m *TaskOptsUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		setCachedDarkMode(m.isDark)
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
		}
	}

	return m, nil
}

func (m *TaskOptsUI) handleLeft() {
	if m.field == TaskOptsFieldModel {
		if m.modelIdx > 0 {
			m.modelIdx--
			m.options.Model = config.ValidModels()[m.modelIdx]
		}
	}
}

func (m *TaskOptsUI) handleRight() {
	if m.field == TaskOptsFieldModel {
		models := config.ValidModels()
		if m.modelIdx < len(models)-1 {
			m.modelIdx++
			m.options.Model = models[m.modelIdx]
		}
	}
}

// View renders the task options UI.
func (m *TaskOptsUI) View() tea.View {
	// Adaptive colors for light/dark terminal themes (use cached isDark value)
	// Light theme: use darker colors for visibility on light backgrounds
	// Dark theme: use lighter colors for visibility on dark backgrounds
	lightDark := lipgloss.LightDark(m.isDark)
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))
	// Dim color: medium contrast for non-selected items (readable on various backgrounds)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("243"))
	// Accent color: darker blue for light bg, bright cyan for dark bg
	accentColor := lightDark(lipgloss.Color("25"), lipgloss.Color("39"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	selectedLabelStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)

	valueStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	selectedValueStyle := lipgloss.NewStyle().
		Foreground(accentColor).
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
		label := labelStyle.Render("Model: ")
		if m.field == TaskOptsFieldModel {
			label = selectedLabelStyle.Render("Model: ")
		}

		models := config.ValidModels()
		modelParts := make([]string, 0, len(models))
		for i, model := range models {
			if i == m.modelIdx {
				if m.field == TaskOptsFieldModel {
					modelParts = append(modelParts, selectedValueStyle.Render("["+string(model)+"]"))
				} else {
					modelParts = append(modelParts, valueStyle.Render("["+string(model)+"]"))
				}
			} else {
				modelParts = append(modelParts, dimStyle.Render(" "+string(model)+" "))
			}
		}
		sb.WriteString(label + strings.Join(modelParts, ""))
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
