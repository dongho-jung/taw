// Package tui provides terminal user interface components for TAW.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// LogViewer provides an interactive log viewer with vim-like navigation.
type LogViewer struct {
	logFile       string
	lines         []string
	scrollPos     int
	horizontalPos int
	tailMode      bool
	wordWrap      bool
	minLevel      int // 0-5: minimum level to display (0=all, 1=L1+, ..., 5=L5 only)
	width         int
	height        int
	lastModTime   time.Time
	err           error
}

// logUpdateMsg is sent when the log file is updated.
type logUpdateMsg struct {
	lines   []string
	modTime time.Time
}

// tickMsg is sent periodically to check for file updates.
type logTickMsg time.Time

// NewLogViewer creates a new log viewer for the given log file.
func NewLogViewer(logFile string) *LogViewer {
	return &LogViewer{
		logFile:  logFile,
		tailMode: true,
		minLevel: 2, // Show L2+ (Info and above) by default
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

	case logTickMsg:
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
	case "q", "esc", "ctrl+l", "ctrl+shift+l":
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

	case "l":
		// Cycle through log levels: 0 -> 1 -> 2 -> 3 -> 4 -> 5 -> 0
		m.minLevel++
		if m.minLevel > 5 {
			m.minLevel = 0
		}
		m.scrollPos = 0
		if m.tailMode {
			m.scrollToEnd()
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

// getLogLevel extracts the log level (0-5) from a log line.
// Returns -1 if no level is found (line will always be shown).
func getLogLevel(line string) int {
	// Look for [L0], [L1], [L2], [L3], [L4], [L5] pattern
	// Format: [timestamp] [LN] [context] [caller] message
	for i := 0; i < len(line)-3; i++ {
		if line[i] == '[' && line[i+1] == 'L' && line[i+3] == ']' {
			level := line[i+2]
			if level >= '0' && level <= '5' {
				return int(level - '0')
			}
		}
	}
	return -1
}

// getFilteredLines returns lines filtered by minimum log level.
func (m *LogViewer) getFilteredLines() []string {
	if m.minLevel <= 0 {
		return m.lines
	}

	var filtered []string
	for _, line := range m.lines {
		level := getLogLevel(line)
		// Show line if level is -1 (no level found) or >= minLevel
		if level == -1 || level >= m.minLevel {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

// getDisplayLines returns lines to display, handling word wrap if enabled.
func (m *LogViewer) getDisplayLines() []string {
	lines := m.getFilteredLines()

	if !m.wordWrap || m.width <= 0 {
		return lines
	}

	// Word wrap mode: wrap long lines
	var wrapped []string
	for _, line := range lines {
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
func (m *LogViewer) View() tea.View {
	if m.err != nil {
		return tea.NewView(fmt.Sprintf("Error: %v\n\nPress q or Esc to close.", m.err))
	}

	if m.width == 0 || m.height == 0 {
		return tea.NewView("Loading...")
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
	if m.minLevel > 0 {
		status += fmt.Sprintf(" [L%d+]", m.minLevel)
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
	hint := "↑↓←→:scroll s:tail w:wrap l:level g/G:top/end q:close"
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

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
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

		return logTickMsg(time.Now())
	}
}

// tick returns a command that sends a tick message after a delay.
func (m *LogViewer) tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return logTickMsg(t)
	})
}

// RunLogViewer runs the log viewer for the given log file.
func RunLogViewer(logFile string) error {
	m := NewLogViewer(logFile)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
