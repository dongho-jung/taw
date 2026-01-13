// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// SetupWizard provides an interactive setup wizard.
type SetupWizard struct {
	isGitRepo bool
	done      bool
	cancelled bool
}

// SetupResult contains the result of the setup wizard.
type SetupResult struct {
	Cancelled bool
}

// NewSetupWizard creates a new setup wizard.
func NewSetupWizard(isGitRepo bool) *SetupWizard {
	return &SetupWizard{
		isGitRepo: isGitRepo,
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

		case "enter", " ":
			m.done = true
			return m, tea.Quit
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

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render("PAW Setup Wizard"))
	sb.WriteString("\n\n")

	if m.isGitRepo {
		sb.WriteString("Git repository detected.\n")
		sb.WriteString(descStyle.Render("PAW will use worktree mode - each task gets its own git worktree.\n\n"))
	} else {
		sb.WriteString("This is not a git repository.\n")
		sb.WriteString(descStyle.Render("Tasks will work in the project directory directly.\n\n"))
	}

	sb.WriteString("Press Enter to continue...")

	sb.WriteString("\n\n")
	sb.WriteString(descStyle.Render("Enter: Continue  q: Cancel"))

	return tea.NewView(sb.String())
}

// Result returns the setup result.
func (m *SetupWizard) Result() SetupResult {
	return SetupResult{
		Cancelled: m.cancelled,
	}
}

// RunSetupWizard runs the setup wizard and returns the result.
func RunSetupWizard(isGitRepo bool) (*SetupResult, error) {
	// Reset theme cache to ensure fresh detection on each TUI start
	ResetDarkModeCache()

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
