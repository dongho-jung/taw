// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
)

// TaskOptsField represents the current field being edited.
type TaskOptsField int

const (
	TaskOptsFieldModel TaskOptsField = iota
	TaskOptsFieldUltrathink
	TaskOptsFieldDependsOnTask
	TaskOptsFieldDependsOnCondition
	TaskOptsFieldWorktreeHook
)

const taskOptsFieldCount = 5

// TaskOptsUI provides an interactive task options form.
type TaskOptsUI struct {
	options     *config.TaskOptions
	field       TaskOptsField
	modelIdx    int
	condIdx     int
	taskInput   textinput.Model
	hookInput   textinput.Model
	width       int
	height      int
	done        bool
	cancelled   bool
	activeTasks []string // List of active task names for dependency selection
}

// TaskOptsResult contains the result of the task options UI.
type TaskOptsResult struct {
	Options   *config.TaskOptions
	Cancelled bool
}

// NewTaskOptsUI creates a new task options UI.
func NewTaskOptsUI(currentOpts *config.TaskOptions, activeTasks []string) *TaskOptsUI {
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

	// Find current condition index
	condIdx := 0
	conditions := []config.DependsOnCondition{
		config.DependsOnNone,
		config.DependsOnSuccess,
		config.DependsOnFailure,
		config.DependsOnAlways,
	}
	if opts.DependsOn != nil {
		for i, c := range conditions {
			if c == opts.DependsOn.Condition {
				condIdx = i
				break
			}
		}
	}

	// Task name input
	taskInput := textinput.New()
	taskInput.Placeholder = "task-name"
	taskInput.CharLimit = 64
	taskInput.SetWidth(30)
	if opts.DependsOn != nil {
		taskInput.SetValue(opts.DependsOn.TaskName)
	}

	// Worktree hook input
	hookInput := textinput.New()
	hookInput.Placeholder = "npm install && npm run build"
	hookInput.CharLimit = 256
	hookInput.SetWidth(50)
	hookInput.SetValue(opts.WorktreeHook)

	return &TaskOptsUI{
		options:     opts,
		field:       TaskOptsFieldModel,
		modelIdx:    modelIdx,
		condIdx:     condIdx,
		taskInput:   taskInput,
		hookInput:   hookInput,
		activeTasks: activeTasks,
	}
}

// Init initializes the task options UI.
func (m *TaskOptsUI) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model.
func (m *TaskOptsUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

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
			// If in text input fields, submit saves
			if m.field == TaskOptsFieldDependsOnTask || m.field == TaskOptsFieldWorktreeHook {
				m.applyInputValues()
			}
			m.done = true
			return m, tea.Quit

		case "tab", "down", "j":
			m.applyInputValues()
			m.field = (m.field + 1) % taskOptsFieldCount
			m.updateFocus()
			return m, nil

		case "shift+tab", "up", "k":
			m.applyInputValues()
			m.field = (m.field - 1 + taskOptsFieldCount) % taskOptsFieldCount
			m.updateFocus()
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

	// Update focused text input
	var cmd tea.Cmd
	switch m.field {
	case TaskOptsFieldDependsOnTask:
		m.taskInput, cmd = m.taskInput.Update(msg)
		cmds = append(cmds, cmd)
	case TaskOptsFieldWorktreeHook:
		m.hookInput, cmd = m.hookInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *TaskOptsUI) handleLeft() {
	switch m.field {
	case TaskOptsFieldModel:
		if m.modelIdx > 0 {
			m.modelIdx--
			m.options.Model = config.ValidModels()[m.modelIdx]
		}
	case TaskOptsFieldUltrathink:
		m.options.Ultrathink = false
	case TaskOptsFieldDependsOnCondition:
		if m.condIdx > 0 {
			m.condIdx--
			m.updateDependsOn()
		}
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
		m.options.Ultrathink = true
	case TaskOptsFieldDependsOnCondition:
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

func (m *TaskOptsUI) updateDependsOn() {
	conditions := []config.DependsOnCondition{
		config.DependsOnNone,
		config.DependsOnSuccess,
		config.DependsOnFailure,
		config.DependsOnAlways,
	}

	if m.condIdx == 0 || m.taskInput.Value() == "" {
		m.options.DependsOn = nil
	} else {
		m.options.DependsOn = &config.TaskDependency{
			TaskName:  m.taskInput.Value(),
			Condition: conditions[m.condIdx],
		}
	}
}

func (m *TaskOptsUI) applyInputValues() {
	// Apply text input values to options
	m.updateDependsOn()
	m.options.WorktreeHook = m.hookInput.Value()
}

func (m *TaskOptsUI) updateFocus() {
	m.taskInput.Blur()
	m.hookInput.Blur()

	switch m.field {
	case TaskOptsFieldDependsOnTask:
		m.taskInput.Focus()
	case TaskOptsFieldWorktreeHook:
		m.hookInput.Focus()
	}
}

// View renders the task options UI.
func (m *TaskOptsUI) View() tea.View {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(20)

	selectedLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Width(20)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	selectedValueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
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

	// Depends on task field
	{
		label := labelStyle.Render("Depends on task:")
		if m.field == TaskOptsFieldDependsOnTask {
			label = selectedLabelStyle.Render("Depends on task:")
		}
		sb.WriteString(label)
		sb.WriteString(m.taskInput.View())
		sb.WriteString("\n")
	}

	// Depends on condition field
	{
		label := labelStyle.Render("Run when:")
		if m.field == TaskOptsFieldDependsOnCondition {
			label = selectedLabelStyle.Render("Run when:")
		}
		sb.WriteString(label)

		conditions := []struct {
			val   config.DependsOnCondition
			label string
		}{
			{config.DependsOnNone, "none"},
			{config.DependsOnSuccess, "success"},
			{config.DependsOnFailure, "failure"},
			{config.DependsOnAlways, "always"},
		}

		var condParts []string
		for i, cond := range conditions {
			text := cond.label
			if i == m.condIdx {
				if m.field == TaskOptsFieldDependsOnCondition {
					text = selectedValueStyle.Render("[" + text + "]")
				} else {
					text = valueStyle.Render("[" + text + "]")
				}
			} else {
				text = dimStyle.Render(" " + text + " ")
			}
			condParts = append(condParts, text)
		}
		sb.WriteString(strings.Join(condParts, " "))
		sb.WriteString("\n")
	}

	// Worktree hook field
	{
		label := labelStyle.Render("Worktree hook:")
		if m.field == TaskOptsFieldWorktreeHook {
			label = selectedLabelStyle.Render("Worktree hook:")
		}
		sb.WriteString(label)
		sb.WriteString(m.hookInput.View())
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
	m.applyInputValues()
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
