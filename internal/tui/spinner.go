// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// Spinner provides a loading spinner with message.
type Spinner struct {
	message string
	frame   int
	done    bool
	result  string
	err     error
	width   int
	height  int
}

// spinnerFrames are the animation frames for the spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// tickMsg is sent periodically to animate the spinner.
type spinnerTickMsg time.Time

// SpinnerDoneMsg is sent when the spinner task is complete.
type SpinnerDoneMsg struct {
	Result string
	Err    error
}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
	}
}

// Init initializes the spinner.
func (m *Spinner) Init() tea.Cmd {
	return m.tick()
}

// Update handles messages and updates the model.
func (m *Spinner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case spinnerTickMsg:
		m.frame = (m.frame + 1) % len(spinnerFrames)
		return m, m.tick()

	case SpinnerDoneMsg:
		m.done = true
		m.result = msg.Result
		m.err = msg.Err
		return m, tea.Quit
	}

	return m, nil
}

// View renders the spinner.
func (m *Spinner) View() tea.View {
	// Default dimensions for fallback
	width := m.width
	height := m.height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	// Build the spinner content
	var innerContent string
	if m.done {
		if m.err != nil {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
			innerContent = style.Render(fmt.Sprintf("✗ %s: %v", m.message, m.err))
		} else {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("40"))
			if m.result != "" {
				innerContent = style.Render(fmt.Sprintf("✓ %s: %s", m.message, m.result))
			} else {
				innerContent = style.Render(fmt.Sprintf("✓ %s", m.message))
			}
		}
	} else {
		spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
		frame := spinnerFrames[m.frame]
		innerContent = spinnerStyle.Render(fmt.Sprintf("%s %s", frame, m.message))
	}

	// Create a box around the spinner content
	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(1, 3)

	box := boxStyle.Render(innerContent)

	// Center horizontally and vertically
	centeredStyle := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	content := centeredStyle.Render(box)

	v := tea.NewView(content)
	v.AltScreen = true

	return v
}

// tick returns a command that sends a tick message.
func (m *Spinner) tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

// GetError returns the error if any occurred during the spinner task.
func (m *Spinner) GetError() error {
	return m.err
}

// GetResult returns the result of the spinner task.
func (m *Spinner) GetResult() string {
	return m.result
}

// Done sends a done message with the given result.
func Done(result string) tea.Cmd {
	return func() tea.Msg {
		return SpinnerDoneMsg{Result: result}
	}
}

// Error sends a done message with an error.
func Error(err error) tea.Cmd {
	return func() tea.Msg {
		return SpinnerDoneMsg{Err: err}
	}
}

// RunSpinner runs a spinner while executing a task.
func RunSpinner(message string, task func() (string, error)) (string, error) {
	m := NewSpinner(message)

	p := tea.NewProgram(m)

	// Run task in background
	go func() {
		result, err := task()
		p.Send(SpinnerDoneMsg{Result: result, Err: err})
	}()

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	spinner := finalModel.(*Spinner)
	if spinner.err != nil {
		return "", spinner.err
	}
	return spinner.result, nil
}

// SimpleSpinner provides a non-interactive spinner for use in scripts.
type SimpleSpinner struct {
	message string
	done    chan struct{}
	frame   int
}

// NewSimpleSpinner creates a new simple spinner.
func NewSimpleSpinner(message string) *SimpleSpinner {
	return &SimpleSpinner{
		message: message,
		done:    make(chan struct{}),
	}
}

// Start starts the spinner animation.
func (s *SimpleSpinner) Start() {
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				frame := spinnerFrames[s.frame%len(spinnerFrames)]
				fmt.Printf("\r  %s %s", frame, s.message)
				s.frame++
			}
		}
	}()
}

// Stop stops the spinner and shows the result.
func (s *SimpleSpinner) Stop(success bool, result string) {
	close(s.done)

	if success {
		fmt.Printf("\r  ✓ %s", s.message)
		if result != "" {
			fmt.Printf(": %s", result)
		}
	} else {
		fmt.Printf("\r  ✗ %s", s.message)
		if result != "" {
			fmt.Printf(": %s", result)
		}
	}
	fmt.Println()
}
