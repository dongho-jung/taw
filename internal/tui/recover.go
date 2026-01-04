// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/task"
)

// RecoverUI provides UI for recovering corrupted tasks.
type RecoverUI struct {
	task      *task.Task
	cursor    int
	done      bool
	cancelled bool
	action    task.RecoveryAction
}

// NewRecoverUI creates a new recovery UI.
func NewRecoverUI(t *task.Task) *RecoverUI {
	return &RecoverUI{
		task: t,
	}
}

// Init initializes the recovery UI.
func (m *RecoverUI) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model.
func (m *RecoverUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.cancelled = true
			m.done = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < 2 {
				m.cursor++
			}

		case "enter", " ":
			m.done = true
			switch m.cursor {
			case 0:
				m.action = task.RecoveryRecover
			case 1:
				m.action = task.RecoveryCleanup
			case 2:
				m.action = task.RecoveryCancel
				m.cancelled = true
			}
			return m, tea.Quit
		}
	}

	return m, nil
}

// View renders the recovery UI.
func (m *RecoverUI) View() tea.View {
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220"))

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render(fmt.Sprintf("⚠️  Task Recovery: %s", m.task.Name)))
	sb.WriteString("\n\n")

	// Show corruption details
	sb.WriteString(warningStyle.Render("Problem: "))
	sb.WriteString(task.GetRecoveryDescription(m.task.CorruptedReason))
	sb.WriteString("\n\n")

	sb.WriteString(descStyle.Render("Recommended action: "))
	sb.WriteString(task.GetRecoveryAction(m.task.CorruptedReason))
	sb.WriteString("\n\n")

	// Options
	sb.WriteString("Choose an action:\n\n")

	options := []struct {
		name string
		desc string
	}{
		{"Recover", "Attempt to fix the issue and continue the task"},
		{"Cleanup", "Remove the corrupted task completely"},
		{"Cancel", "Do nothing and exit"},
	}

	for i, opt := range options {
		cursor := "  "
		style := normalStyle
		if i == m.cursor {
			cursor = "▸ "
			style = selectedStyle
		}
		sb.WriteString(cursor + style.Render(opt.name) + "\n")
		sb.WriteString("    " + descStyle.Render(opt.desc) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(descStyle.Render("↑/↓: Navigate  Enter: Select  q: Cancel"))

	return tea.NewView(sb.String())
}

// Result returns the chosen action.
func (m *RecoverUI) Result() task.RecoveryAction {
	if m.cancelled {
		return task.RecoveryCancel
	}
	return m.action
}

// RunRecoverUI runs the recovery UI and returns the chosen action.
func RunRecoverUI(t *task.Task) (task.RecoveryAction, error) {
	m := NewRecoverUI(t)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return task.RecoveryCancel, err
	}

	ui := finalModel.(*RecoverUI)
	return ui.Result(), nil
}
