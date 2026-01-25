// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// StepStatus represents the status of a step.
type StepStatus string

// Step status values.
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
	isDark      bool
	colors      ThemeColors

	// Style cache (reused across renders)
	styleTitle   lipgloss.Style
	styleOK      lipgloss.Style
	styleSkip    lipgloss.Style
	styleFail    lipgloss.Style
	styleRunning lipgloss.Style
	stylePending lipgloss.Style
	stylesCached bool
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
	isDark := DetectDarkMode()

	// Pre-allocate: 6 steps for git repos, 2 for non-git
	stepCapacity := 2
	if isGitRepo {
		stepCapacity = 6
	}
	steps := make([]Step, 0, stepCapacity)

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
		isDark:   isDark,
		colors:   NewThemeColors(isDark),
	}
}

// Init initializes the finish task UI.
func (m *EndTaskUI) Init() tea.Cmd {
	return tea.Batch(m.runNextStep(), tea.RequestBackgroundColor)
}

// Update handles messages and updates the model.
func (m *EndTaskUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	// Update style cache if needed (only on theme change)
	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(c.Accent)
		m.styleOK = lipgloss.NewStyle().
			Foreground(c.SuccessColor)
		m.styleSkip = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.styleFail = lipgloss.NewStyle().
			Foreground(c.ErrorColor)
		m.styleRunning = lipgloss.NewStyle().
			Foreground(c.WarningColor)
		m.stylePending = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.stylesCached = true
	}

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(m.styleTitle.Render("Finishing task: " + m.taskName))
	sb.WriteString("\n\n")

	for i, step := range m.steps {
		var icon string
		var style lipgloss.Style

		switch step.Status { //nolint:exhaustive // StepPending uses default case
		case StepOK:
			icon = "✓"
			style = m.styleOK
		case StepSkip:
			icon = "○"
			style = m.styleSkip
		case StepFail:
			icon = "✗"
			style = m.styleFail
		case StepRunning:
			icon = "●"
			style = m.styleRunning
		default:
			icon = "○"
			style = m.stylePending
		}

		// Use string concatenation instead of fmt.Sprintf
		line := " " + icon + " " + step.Name
		sb.WriteString(style.Render(line))

		if step.Message != "" {
			sb.WriteString(m.styleSkip.Render(" (" + step.Message + ")"))
		}

		sb.WriteString("\n")

		_ = i
	}

	if m.done {
		sb.WriteString("\n")
		if m.err != nil {
			sb.WriteString(m.styleFail.Render("Error: " + m.err.Error()))
		} else {
			allOK := true
			for _, step := range m.steps {
				if step.Status == StepFail {
					allOK = false
					break
				}
			}
			if allOK {
				sb.WriteString(m.styleOK.Render("Done!"))
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
	m := NewEndTaskUI(taskName, isGitRepo)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
