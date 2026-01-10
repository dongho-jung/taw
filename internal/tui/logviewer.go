// Package tui provides terminal user interface components for PAW.
package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// LogViewer provides an interactive log viewer with vim-like navigation.
type LogViewer struct {
	logFile              string
	lines                []string
	scrollPos            int
	horizontalPos        int
	tailMode             bool
	wordWrap             bool
	minLevel             int // 0-5: minimum level to display (0=all, 1=L1+, ..., 5=L5 only)
	width                int
	height               int
	lastModTime          time.Time
	lastSize             int64
	lastEndedWithNewline bool
	err                  error

	// Mouse text selection state
	selecting    bool
	hasSelection bool
	selectStartY int // Start row (screen-relative)
	selectStartX int // Start column (screen-relative)
	selectEndY   int // End row (screen-relative)
	selectEndX   int // End column (screen-relative)
}

const maxLogLines = 5000

// logUpdateMsg is sent when the log file is updated.
type logUpdateMsg struct {
	lines           []string
	modTime         time.Time
	size            int64
	endsWithNewline bool
}

// logAppendMsg is sent when new log lines are appended.
type logAppendMsg struct {
	lines           []string
	modTime         time.Time
	size            int64
	endsWithNewline bool
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

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			// Start text selection
			m.selecting = true
			m.hasSelection = true
			m.selectStartY = msg.Y
			m.selectStartX = msg.X
			m.selectEndY = msg.Y
			m.selectEndX = msg.X
		}
		return m, nil

	case tea.MouseMotionMsg:
		// Extend selection while dragging
		if m.selecting {
			m.selectEndY = msg.Y
			m.selectEndX = msg.X
		}
		return m, nil

	case tea.MouseReleaseMsg:
		if msg.Button == tea.MouseLeft && m.selecting {
			m.selecting = false
			m.selectEndY = msg.Y
			m.selectEndX = msg.X
		}
		return m, nil

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.tailMode = false
			m.scrollUp(3)
		case tea.MouseWheelDown:
			m.scrollDown(3)
		}
		return m, nil

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
		m.lastSize = msg.size
		m.lastEndedWithNewline = msg.endsWithNewline
		m.trimLines()
		if m.tailMode {
			m.scrollToEnd()
		} else {
			m.clampScroll()
		}
		return m, m.tick()

	case logAppendMsg:
		if len(msg.lines) > 0 {
			if !m.lastEndedWithNewline {
				if len(m.lines) == 0 {
					m.lines = append(m.lines, msg.lines[0])
				} else {
					m.lines[len(m.lines)-1] += msg.lines[0]
				}
				msg.lines = msg.lines[1:]
			}
			if len(msg.lines) > 0 {
				m.lines = append(m.lines, msg.lines...)
			}
		}
		m.lastModTime = msg.modTime
		m.lastSize = msg.size
		m.lastEndedWithNewline = msg.endsWithNewline
		m.trimLines()
		if m.tailMode {
			m.scrollToEnd()
		} else {
			m.clampScroll()
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
	// Copy selection with Ctrl+C
	case "ctrl+c":
		if m.hasSelection {
			m.copySelection()
		}
		return m, nil

	case "q", "esc", "ctrl+o", "ctrl+l", "ctrl+shift+l", "ctrl+shift+o":
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

	case "pgup", "ctrl+b":
		m.tailMode = false
		m.scrollUp(m.contentHeight())

	case "pgdown", "ctrl+f":
		m.scrollDown(m.contentHeight())

	case "ctrl+u":
		m.tailMode = false
		m.scrollUp(m.contentHeight() / 2)

	case "ctrl+d":
		m.scrollDown(m.contentHeight() / 2)
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

	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Level string `json:"level"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil && len(payload.Level) == 2 && payload.Level[0] == 'L' {
			level := payload.Level[1]
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

	// Selection highlight style
	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("25")).
		Foreground(lipgloss.Color("255"))

	// Render visible lines
	for i := m.scrollPos; i < endPos; i++ {
		screenY := i - m.scrollPos // Screen-relative Y position
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

		// Apply selection highlighting if this line is in selection
		if m.hasSelection {
			line = m.applySelectionToLine(line, screenY, highlightStyle)
		}

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
	hint := "↑↓←→:scroll s:tail w:wrap l:level g/G:top/end ⌃O/q:close"
	padding := m.width - len(status) - len(hint)
	if padding < 0 {
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
	v.MouseMode = tea.MouseModeAllMotion // Enable mouse drag selection
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

func (m *LogViewer) clampScroll() {
	displayLines := m.getDisplayLines()
	max := len(displayLines) - m.contentHeight()
	if max < 0 {
		max = 0
	}
	if m.scrollPos > max {
		m.scrollPos = max
	}
	if m.scrollPos < 0 {
		m.scrollPos = 0
	}
}

func (m *LogViewer) trimLines() {
	if len(m.lines) <= maxLogLines {
		return
	}
	drop := len(m.lines) - maxLogLines
	m.lines = m.lines[drop:]
	if m.scrollPos >= drop {
		m.scrollPos -= drop
	} else {
		m.scrollPos = 0
	}
}

func (m *LogViewer) readFullFile(info os.FileInfo) tea.Msg {
	data, err := os.ReadFile(m.logFile)
	if err != nil {
		return err
	}

	lines, endsWithNewline := splitLinesWithNewlineFlag(string(data))

	return logUpdateMsg{
		lines:           lines,
		modTime:         info.ModTime(),
		size:            info.Size(),
		endsWithNewline: endsWithNewline,
	}
}

func splitLinesWithNewlineFlag(data string) ([]string, bool) {
	endsWithNewline := strings.HasSuffix(data, "\n")
	if endsWithNewline {
		data = strings.TrimSuffix(data, "\n")
	}
	if data == "" {
		if endsWithNewline {
			return []string{""}, true
		}
		return nil, false
	}
	return strings.Split(data, "\n"), endsWithNewline
}

// loadFile loads the log file contents.
func (m *LogViewer) loadFile() tea.Cmd {
	return func() tea.Msg {
		info, err := os.Stat(m.logFile)
		if err != nil {
			return err
		}
		return m.readFullFile(info)
	}
}

// checkFileUpdate checks if the file has been updated.
func (m *LogViewer) checkFileUpdate() tea.Cmd {
	return func() tea.Msg {
		info, err := os.Stat(m.logFile)
		if err != nil {
			return err
		}
		size := info.Size()

		if size < m.lastSize {
			return m.readFullFile(info)
		}

		if size == m.lastSize {
			if info.ModTime().After(m.lastModTime) {
				return m.readFullFile(info)
			}
			return logTickMsg(time.Now())
		}

		file, err := os.Open(m.logFile)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()

		if _, err := file.Seek(m.lastSize, io.SeekStart); err != nil {
			return err
		}

		data, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		lines, endsWithNewline := splitLinesWithNewlineFlag(string(data))

		return logAppendMsg{
			lines:           lines,
			modTime:         info.ModTime(),
			size:            size,
			endsWithNewline: endsWithNewline,
		}
	}
}

// tick returns a command that sends a tick message after a delay.
func (m *LogViewer) tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return logTickMsg(t)
	})
}

// clearSelection clears the current selection.
func (m *LogViewer) clearSelection() {
	m.selecting = false
	m.hasSelection = false
	m.selectStartY = 0
	m.selectStartX = 0
	m.selectEndY = 0
	m.selectEndX = 0
}

// getSelectionRange returns the normalized selection range (minY, maxY, startX, endX).
// startX/endX are adjusted based on selection direction.
func (m *LogViewer) getSelectionRange() (minY, maxY, startX, endX int) {
	if m.selectStartY < m.selectEndY {
		return m.selectStartY, m.selectEndY, m.selectStartX, m.selectEndX
	} else if m.selectStartY > m.selectEndY {
		return m.selectEndY, m.selectStartY, m.selectEndX, m.selectStartX
	}
	// Same row - ensure startX < endX
	if m.selectStartX <= m.selectEndX {
		return m.selectStartY, m.selectEndY, m.selectStartX, m.selectEndX
	}
	return m.selectStartY, m.selectEndY, m.selectEndX, m.selectStartX
}

// getSelectionXRange returns the X selection range for a given screen row.
func (m *LogViewer) getSelectionXRange(screenY int) (int, int) {
	minY, maxY, startX, endX := m.getSelectionRange()

	if screenY < minY || screenY > maxY {
		return -1, -1 // Not in selection
	}

	if minY == maxY {
		// Single row selection
		return startX, endX
	}

	// Multi-row selection
	if screenY == minY {
		// First row: from startX to end of line
		return startX, m.width
	} else if screenY == maxY {
		// Last row: from start to endX
		return 0, endX
	}
	// Middle rows: full line
	return 0, m.width
}

// applySelectionToLine applies selection highlighting to a line.
func (m *LogViewer) applySelectionToLine(line string, screenY int, highlightStyle lipgloss.Style) string {
	startX, endX := m.getSelectionXRange(screenY)
	if startX < 0 || startX >= endX {
		return line // Not in selection or invalid range
	}

	lineLen := len(line)
	if startX >= lineLen {
		return line // Selection starts beyond line
	}
	if endX > lineLen {
		endX = lineLen
	}

	// Split line into before, selected, and after parts
	before := ""
	if startX > 0 {
		before = line[:startX]
	}
	selected := line[startX:endX]
	after := ""
	if endX < lineLen {
		after = line[endX:]
	}

	return before + highlightStyle.Render(selected) + after
}

// copySelection copies the selected text to clipboard.
func (m *LogViewer) copySelection() {
	if !m.hasSelection {
		return
	}

	displayLines := m.getDisplayLines()
	minY, maxY, _, _ := m.getSelectionRange()

	var selectedLines []string
	for screenY := minY; screenY <= maxY; screenY++ {
		lineIdx := m.scrollPos + screenY
		if lineIdx < 0 || lineIdx >= len(displayLines) {
			continue
		}

		line := displayLines[lineIdx]

		if !m.wordWrap {
			// Apply horizontal scroll offset for accurate X positions
			if m.horizontalPos > 0 {
				if m.horizontalPos < len(line) {
					line = line[m.horizontalPos:]
				} else {
					line = ""
				}
			}
		}

		// Truncate to screen width
		if len(line) > m.width {
			line = line[:m.width]
		}

		// Pad for consistent width
		if len(line) < m.width {
			line = line + strings.Repeat(" ", m.width-len(line))
		}

		startX, endX := m.getSelectionXRange(screenY)
		if startX < 0 || startX >= endX {
			continue
		}
		if endX > m.width {
			endX = m.width
		}

		// Extract selected portion
		selected := line[startX:endX]
		plainSelected := strings.TrimRight(selected, " ")
		selectedLines = append(selectedLines, plainSelected)
	}

	if len(selectedLines) > 0 {
		text := strings.Join(selectedLines, "\n")
		_ = clipboard.WriteAll(text)
	}
}

// RunLogViewer runs the log viewer for the given log file.
func RunLogViewer(logFile string) error {
	m := NewLogViewer(logFile)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
