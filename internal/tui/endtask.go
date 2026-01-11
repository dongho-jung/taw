// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
)

// StepStatus represents the status of a step.
type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepOK      StepStatus = "ok"
	StepSkip    StepStatus = "skip"
	StepFail    StepStatus = "fail"
)

// Step represents a step in the finish task process.
type Step struct {
	Name    string
	Status  StepStatus
	Message string
}

// EndTaskUI provides UI for the finish task process.
type EndTaskUI struct {
	taskName    string
	steps       []Step
	currentStep int
	done        bool
	err         error
	width       int
	height      int
	theme       config.Theme
	isDark      bool
	colors      ThemeColors
}

// stepCompleteMsg is sent when a step completes.
type stepCompleteMsg struct {
	index   int
	status  StepStatus
	message string
}

// NewEndTaskUI creates a new finish task UI.
func NewEndTaskUI(taskName string, isGitRepo bool) *EndTaskUI {
	// Detect dark mode BEFORE bubbletea starts
	theme := loadThemeFromConfig()
	isDark := detectDarkMode(theme)

	steps := []Step{}

	if isGitRepo {
		steps = append(steps,
			Step{Name: "Check uncommitted changes", Status: StepPending},
			Step{Name: "Commit changes", Status: StepPending},
			Step{Name: "Push to remote", Status: StepPending},
			Step{Name: "Check merge status", Status: StepPending},
		)
	}

	steps = append(steps,
		Step{Name: "Cleanup task", Status: StepPending},
		Step{Name: "Close window", Status: StepPending},
	)

	return &EndTaskUI{
		taskName: taskName,
		steps:    steps,
		theme:    theme,
		isDark:   isDark,
		colors:   NewThemeColors(isDark),
	}
}

// Init initializes the finish task UI.
func (m *EndTaskUI) Init() tea.Cmd {
	cmds := []tea.Cmd{m.runNextStep()}
	if m.theme == config.ThemeAuto {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

// Update handles messages and updates the model.
func (m *EndTaskUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		if m.theme == config.ThemeAuto {
			m.isDark = msg.IsDark()
			m.colors = NewThemeColors(m.isDark)
			setCachedDarkMode(m.isDark)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case stepCompleteMsg:
		m.steps[msg.index].Status = msg.status
		m.steps[msg.index].Message = msg.message

		if msg.status == StepFail {
			m.done = true
			return m, nil
		}

		m.currentStep++
		if m.currentStep >= len(m.steps) {
			m.done = true
			return m, tea.Quit
		}

		return m, m.runNextStep()

	case error:
		m.err = msg
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// View renders the finish task UI.
func (m *EndTaskUI) View() tea.View {
	c := m.colors
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(c.Accent)

	okStyle := lipgloss.NewStyle().
		Foreground(c.SuccessColor)

	skipStyle := lipgloss.NewStyle().
		Foreground(c.TextDim)

	failStyle := lipgloss.NewStyle().
		Foreground(c.ErrorColor)

	runningStyle := lipgloss.NewStyle().
		Foreground(c.WarningColor)

	pendingStyle := lipgloss.NewStyle().
		Foreground(c.TextDim)

	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render(fmt.Sprintf("Finishing task: %s", m.taskName)))
	sb.WriteString("\n\n")

	for i, step := range m.steps {
		var icon string
		var style lipgloss.Style

		switch step.Status {
		case StepOK:
			icon = "✓"
			style = okStyle
		case StepSkip:
			icon = "○"
			style = skipStyle
		case StepFail:
			icon = "✗"
			style = failStyle
		case StepRunning:
			icon = "●"
			style = runningStyle
		default:
			icon = "○"
			style = pendingStyle
		}

		line := fmt.Sprintf(" %s %s", icon, step.Name)
		sb.WriteString(style.Render(line))

		if step.Message != "" {
			sb.WriteString(skipStyle.Render(fmt.Sprintf(" (%s)", step.Message)))
		}

		sb.WriteString("\n")

		_ = i
	}

	if m.done {
		sb.WriteString("\n")
		if m.err != nil {
			sb.WriteString(failStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		} else {
			allOK := true
			for _, step := range m.steps {
				if step.Status == StepFail {
					allOK = false
					break
				}
			}
			if allOK {
				sb.WriteString(okStyle.Render("Done!"))
			}
		}
		sb.WriteString("\n")
	}

	return tea.NewView(sb.String())
}

// runNextStep runs the next step.
func (m *EndTaskUI) runNextStep() tea.Cmd {
	if m.currentStep >= len(m.steps) {
		return nil
	}

	m.steps[m.currentStep].Status = StepRunning

	return func() tea.Msg {
		// Simulate step execution
		// In real implementation, this would execute actual tasks
		time.Sleep(500 * time.Millisecond)

		return stepCompleteMsg{
			index:   m.currentStep,
			status:  StepOK,
			message: "",
		}
	}
}

// RunEndTaskUI runs the finish task UI.
func RunEndTaskUI(taskName string, isGitRepo bool) error {
	// Reset theme cache to ensure fresh detection on each TUI start
	ResetDarkModeCache()

	m := NewEndTaskUI(taskName, isGitRepo)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
