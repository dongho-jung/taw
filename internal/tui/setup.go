// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
)

// SetupWizard provides an interactive setup wizard.
type SetupWizard struct {
	step       int
	workMode   config.WorkMode
	onComplete config.OnComplete
	isGitRepo  bool
	cursor     int
	done       bool
	cancelled  bool
}

// SetupResult contains the result of the setup wizard.
type SetupResult struct {
	WorkMode   config.WorkMode
	OnComplete config.OnComplete
	Cancelled  bool
}

// NewSetupWizard creates a new setup wizard.
func NewSetupWizard(isGitRepo bool) *SetupWizard {
	return &SetupWizard{
		step:       0,
		workMode:   config.WorkModeWorktree,
		onComplete: config.OnCompleteConfirm,
		isGitRepo:  isGitRepo,
		cursor:     0,
	}
}

// Init initializes the setup wizard.
func (m *SetupWizard) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model.
func (m *SetupWizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			max := m.maxCursor()
			if m.cursor < max {
				m.cursor++
			}

		case "enter", " ":
			return m.selectOption()
		}
	}

	return m, nil
}

// View renders the setup wizard.
func (m *SetupWizard) View() tea.View {
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render("🚀 PAW Setup Wizard"))
	sb.WriteString("\n\n")

	switch m.step {
	case 0:
		if m.isGitRepo {
			sb.WriteString("Work Mode:\n")
			sb.WriteString(descStyle.Render("Choose how tasks work with git\n\n"))

			options := []struct {
				name string
				desc string
			}{
				{"worktree (Recommended)", "Each task gets its own git worktree"},
				{"main", "All tasks work on the current branch"},
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
		} else {
			// Skip work mode for non-git repos
			m.step = 1
			return m.View()
		}

	case 1:
		sb.WriteString("On Complete Action:\n")
		sb.WriteString(descStyle.Render("What happens when a task is completed\n\n"))

		options := []struct {
			name string
			desc string
		}{
			{"confirm (Recommended)", "Ask before each action"},
			{"auto-commit", "Automatically commit changes"},
		}

		// In worktree mode, add merge and PR options
		if m.workMode == config.WorkModeWorktree {
			options = append(options,
				struct{ name, desc string }{"auto-merge", "Auto commit + merge + cleanup"},
				struct{ name, desc string }{"auto-pr", "Auto commit + create pull request"},
			)
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
	}

	sb.WriteString("\n")
	sb.WriteString(descStyle.Render("↑/↓: Navigate  Enter: Select  q: Cancel"))

	return tea.NewView(sb.String())
}

// maxCursor returns the maximum cursor position for the current step.
func (m *SetupWizard) maxCursor() int {
	switch m.step {
	case 0:
		return 1 // worktree, main
	case 1:
		if m.workMode == config.WorkModeWorktree {
			return 3 // confirm, auto-commit, auto-merge, auto-pr
		}
		return 1 // confirm, auto-commit (main mode)
	}
	return 0
}

// selectOption handles option selection.
func (m *SetupWizard) selectOption() (tea.Model, tea.Cmd) {
	switch m.step {
	case 0:
		// Work mode
		switch m.cursor {
		case 0:
			m.workMode = config.WorkModeWorktree
		case 1:
			m.workMode = config.WorkModeMain
		}
		m.step = 1
		m.cursor = 0

	case 1:
		// On complete
		switch m.cursor {
		case 0:
			m.onComplete = config.OnCompleteConfirm
		case 1:
			m.onComplete = config.OnCompleteAutoCommit
		case 2:
			m.onComplete = config.OnCompleteAutoMerge
		case 3:
			m.onComplete = config.OnCompleteAutoPR
		}
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// Result returns the setup result.
func (m *SetupWizard) Result() SetupResult {
	return SetupResult{
		WorkMode:   m.workMode,
		OnComplete: m.onComplete,
		Cancelled:  m.cancelled,
	}
}

// RunSetupWizard runs the setup wizard and returns the result.
func RunSetupWizard(isGitRepo bool) (*SetupResult, error) {
	m := NewSetupWizard(isGitRepo)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	wizard := finalModel.(*SetupWizard)
	result := wizard.Result()
	return &result, nil
}
