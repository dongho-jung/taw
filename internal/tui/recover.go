// Package tui provides terminal user interface components for PAW.
package tui

import (
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
	isDark    bool
	colors    ThemeColors

	// Style cache (reused across renders)
	styleTitle    lipgloss.Style
	styleWarning  lipgloss.Style
	styleDesc     lipgloss.Style
	styleSelected lipgloss.Style
	styleNormal   lipgloss.Style
	stylesCached  bool
}

// NewRecoverUI creates a new recovery UI.
func NewRecoverUI(t *task.Task) *RecoverUI {
	isDark := DetectDarkMode()
	return &RecoverUI{
		task:   t,
		isDark: isDark,
		colors: NewThemeColors(isDark),
	}
}

// Init initializes the recovery UI.
func (m *RecoverUI) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update handles messages and updates the model.
func (m *RecoverUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		m.stylesCached = false // Invalidate style cache on theme change
		setCachedDarkMode(m.isDark)
		return m, nil

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
	c := m.colors

	// Update style cache if needed (only on theme change)
	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(c.WarningColor)
		m.styleWarning = lipgloss.NewStyle().
			Foreground(c.ErrorColor)
		m.styleDesc = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.styleSelected = lipgloss.NewStyle().
			Foreground(c.Accent).
			Bold(true)
		m.styleNormal = lipgloss.NewStyle().
			Foreground(c.TextNormal)
		m.stylesCached = true
	}

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(m.styleTitle.Render("⚠️  Task Recovery: " + m.task.Name))
	sb.WriteString("\n\n")

	// Show corruption details
	sb.WriteString(m.styleWarning.Render("Problem: "))
	sb.WriteString(task.GetRecoveryDescription(m.task.CorruptedReason))
	sb.WriteString("\n\n")

	sb.WriteString(m.styleDesc.Render("Recommended action: "))
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
		style := m.styleNormal
		if i == m.cursor {
			cursor = "▸ "
			style = m.styleSelected
		}
		sb.WriteString(cursor + style.Render(opt.name) + "\n")
		sb.WriteString("    " + m.styleDesc.Render(opt.desc) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(m.styleDesc.Render("↑/↓: Navigate  Enter: Select  q: Cancel"))

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
