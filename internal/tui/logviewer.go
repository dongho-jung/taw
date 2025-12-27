// Package tui provides terminal user interface components for TAW.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogViewer provides an interactive log viewer with vim-like navigation.
type LogViewer struct {
	logFile      string
	lines        []string
	scrollPos    int
	horizontalPos int
	tailMode     bool
	wordWrap     bool
	width        int
	height       int
	lastModTime  time.Time
	err          error
}

// logUpdateMsg is sent when the log file is updated.
type logUpdateMsg struct {
	lines   []string
	modTime time.Time
}

// tickMsg is sent periodically to check for file updates.
type tickMsg time.Time

// NewLogViewer creates a new log viewer for the given log file.
func NewLogViewer(logFile string) *LogViewer {
	return &LogViewer{
		logFile:  logFile,
		tailMode: true,
	}
}

// Init initializes the log viewer.
func (m *LogViewer) Init() tea.Cmd {
	return tea.Batch(
		m.loadFile(),
		m.tick(),
	)
}

// Update handles messages and updates the model.
func (m *LogViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.tailMode {
			m.scrollToEnd()
		}
		return m, nil

	case logUpdateMsg:
		m.lines = msg.lines
		m.lastModTime = msg.modTime
		if m.tailMode {
			m.scrollToEnd()
		}
		return m, m.tick()

	case tickMsg:
		return m, m.checkFileUpdate()

	case error:
		m.err = msg
		return m, nil
	}

	return m, nil
}

// handleKey handles keyboard input.
func (m *LogViewer) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "alt+l":
		return m, tea.Quit

	case "down":
		m.tailMode = false
		m.scrollDown(1)

	case "up":
		m.tailMode = false
		m.scrollUp(1)

	case "left":
		if !m.wordWrap && m.horizontalPos > 0 {
			m.horizontalPos -= 10
			if m.horizontalPos < 0 {
				m.horizontalPos = 0
			}
		}

	case "right":
		if !m.wordWrap {
			m.horizontalPos += 10
		}

	case "g":
		m.tailMode = false
		m.scrollPos = 0
		m.horizontalPos = 0

	case "G":
		m.scrollToEnd()
		m.horizontalPos = 0

	case "s":
		m.tailMode = !m.tailMode
		if m.tailMode {
			m.scrollToEnd()
		}

	case "w":
		m.wordWrap = !m.wordWrap
		if m.wordWrap {
			m.horizontalPos = 0
		}

	case "pgup":
		m.tailMode = false
		m.scrollUp(m.contentHeight())

	case "pgdown":
		m.scrollDown(m.contentHeight())
	}

	return m, nil
}

// scrollUp scrolls up by n lines.
func (m *LogViewer) scrollUp(n int) {
	m.scrollPos -= n
	if m.scrollPos < 0 {
		m.scrollPos = 0
	}
}

// scrollDown scrolls down by n lines.
func (m *LogViewer) scrollDown(n int) {
	displayLines := m.getDisplayLines()
	max := len(displayLines) - m.contentHeight()
	if max < 0 {
		max = 0
	}
	m.scrollPos += n
	if m.scrollPos > max {
		m.scrollPos = max
	}
}

// getDisplayLines returns lines to display, handling word wrap if enabled.
func (m *LogViewer) getDisplayLines() []string {
	if !m.wordWrap || m.width <= 0 {
		return m.lines
	}

	// Word wrap mode: wrap long lines
	var wrapped []string
	for _, line := range m.lines {
		if len(line) <= m.width {
			wrapped = append(wrapped, line)
		} else {
			// Wrap the line
			for len(line) > 0 {
				end := m.width
				if end > len(line) {
					end = len(line)
				}
				wrapped = append(wrapped, line[:end])
				line = line[end:]
			}
		}
	}
	return wrapped
}

// View renders the log viewer.
func (m *LogViewer) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q or Esc to close.", m.err)
	}

	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var sb strings.Builder

	displayLines := m.getDisplayLines()

	// Calculate visible lines
	contentHeight := m.contentHeight()
	endPos := m.scrollPos + contentHeight
	if endPos > len(displayLines) {
		endPos = len(displayLines)
	}

	// Render visible lines
	for i := m.scrollPos; i < endPos; i++ {
		line := displayLines[i]

		if !m.wordWrap {
			// Apply horizontal scroll
			if m.horizontalPos < len(line) {
				line = line[m.horizontalPos:]
			} else {
				line = ""
			}
		}

		// Truncate to screen width
		if len(line) > m.width {
			line = line[:m.width]
		}

		// Pad to full width
		line = fmt.Sprintf("%-*s", m.width, line)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Pad remaining lines
	for i := endPos - m.scrollPos; i < contentHeight; i++ {
		sb.WriteString(strings.Repeat(" ", m.width))
		sb.WriteString("\n")
	}

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("240")).
		Foreground(lipgloss.Color("252"))

	var status string
	if m.tailMode {
		status = " [TAIL]"
	}
	if m.wordWrap {
		status += " [WRAP]"
	}
	if status == "" {
		status = " "
	} else {
		status += " "
	}

	if len(displayLines) > 0 {
		status += fmt.Sprintf("Lines %d-%d of %d ", m.scrollPos+1, endPos, len(displayLines))
	} else {
		status += "(empty) "
	}

	// Keybindings hint
	hint := "↑↓←→:scroll s:tail w:wrap g/G:top/end q:close"
	padding := m.width - len(status) - len(hint)
	if padding < 0 {
		padding = 0
		hint = "q:close"
		padding = m.width - len(status) - len(hint)
		if padding < 0 {
			padding = 0
		}
	}

	statusLine := statusStyle.Render(
		status + strings.Repeat(" ", padding) + hint,
	)

	sb.WriteString(statusLine)

	return sb.String()
}

// contentHeight returns the height available for content.
func (m *LogViewer) contentHeight() int {
	// Reserve 1 line for status bar
	h := m.height - 1
	if h < 1 {
		h = 1
	}
	return h
}

// scrollToEnd scrolls to the end of the log.
func (m *LogViewer) scrollToEnd() {
	displayLines := m.getDisplayLines()
	max := len(displayLines) - m.contentHeight()
	if max < 0 {
		max = 0
	}
	m.scrollPos = max
}

// loadFile loads the log file contents.
func (m *LogViewer) loadFile() tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(m.logFile)
		if err != nil {
			return err
		}

		info, err := os.Stat(m.logFile)
		if err != nil {
			return err
		}

		lines := strings.Split(string(data), "\n")
		// Remove last empty line if present
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		return logUpdateMsg{
			lines:   lines,
			modTime: info.ModTime(),
		}
	}
}

// checkFileUpdate checks if the file has been updated.
func (m *LogViewer) checkFileUpdate() tea.Cmd {
	return func() tea.Msg {
		info, err := os.Stat(m.logFile)
		if err != nil {
			return err
		}

		if info.ModTime().After(m.lastModTime) {
			data, err := os.ReadFile(m.logFile)
			if err != nil {
				return err
			}

			lines := strings.Split(string(data), "\n")
			if len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}

			return logUpdateMsg{
				lines:   lines,
				modTime: info.ModTime(),
			}
		}

		return tickMsg(time.Now())
	}
}

// tick returns a command that sends a tick message after a delay.
func (m *LogViewer) tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// RunLogViewer runs the log viewer for the given log file.
func RunLogViewer(logFile string) error {
	m := NewLogViewer(logFile)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
