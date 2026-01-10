// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// DiffViewer provides an interactive diff viewer with vim-like navigation.
// It displays the git diff between the main branch and HEAD.
type DiffViewer struct {
	workDir       string
	mainBranch    string
	lines         []string
	scrollPos     int
	horizontalPos int
	wordWrap      bool
	width         int
	height        int
	err           error

	// Mouse text selection state
	selecting    bool
	hasSelection bool
	selectStartY int // Start row (screen-relative)
	selectStartX int // Start column (screen-relative)
	selectEndY   int // End row (screen-relative)
	selectEndX   int // End column (screen-relative)
}

// NewDiffViewer creates a new diff viewer for the given working directory.
func NewDiffViewer(workDir, mainBranch string) *DiffViewer {
	return &DiffViewer{
		workDir:    workDir,
		mainBranch: mainBranch,
	}
}

// Init initializes the diff viewer.
func (m *DiffViewer) Init() tea.Cmd {
	return m.loadDiffOutput()
}

// Update handles messages and updates the model.
func (m *DiffViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.scrollUp(3)
		case tea.MouseWheelDown:
			m.scrollDown(3)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case diffOutputMsg:
		m.lines = msg.lines
		m.scrollPos = 0
		m.horizontalPos = 0
		m.clearSelection()
		return m, nil

	case error:
		m.err = msg
		return m, nil
	}

	return m, nil
}

// diffOutputMsg is sent when diff output is loaded.
type diffOutputMsg struct {
	lines []string
}

// handleKey handles keyboard input.
func (m *DiffViewer) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	// Copy selection with Ctrl+C
	case "ctrl+c":
		if m.hasSelection {
			m.copySelection()
		}
		return m, nil

	// Close on q, Esc, or Ctrl+Shift+D
	case "q", "esc", "ctrl+shift+d":
		return m, tea.Quit

	case "down", "j":
		m.scrollDown(1)

	case "up", "k":
		m.scrollUp(1)

	case "left", "h":
		if !m.wordWrap && m.horizontalPos > 0 {
			m.horizontalPos -= 10
			if m.horizontalPos < 0 {
				m.horizontalPos = 0
			}
		}

	case "right", "l":
		if !m.wordWrap {
			// Limit horizontal scroll to max line width minus screen width
			maxScroll := m.maxLineWidth() - m.width
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.horizontalPos += 10
			if m.horizontalPos > maxScroll {
				m.horizontalPos = maxScroll
			}
		}

	case "g":
		// Go to top
		m.scrollPos = 0
		m.horizontalPos = 0

	case "G":
		// Go to bottom
		m.scrollToEnd()
		m.horizontalPos = 0

	case "w":
		// Toggle word wrap
		m.wordWrap = !m.wordWrap
		// Reset scroll positions when toggling wrap mode
		m.scrollPos = 0
		m.horizontalPos = 0

	case "pgup", "ctrl+b":
		m.scrollUp(m.contentHeight())

	case "pgdown", "ctrl+f":
		m.scrollDown(m.contentHeight())

	case "ctrl+u":
		m.scrollUp(m.contentHeight() / 2)

	case "ctrl+d":
		m.scrollDown(m.contentHeight() / 2)
	}

	return m, nil
}

// scrollUp scrolls up by n lines.
func (m *DiffViewer) scrollUp(n int) {
	m.scrollPos -= n
	if m.scrollPos < 0 {
		m.scrollPos = 0
	}
}

// scrollDown scrolls down by n lines.
func (m *DiffViewer) scrollDown(n int) {
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

// scrollToEnd scrolls to the end of the content.
func (m *DiffViewer) scrollToEnd() {
	displayLines := m.getDisplayLines()
	max := len(displayLines) - m.contentHeight()
	if max < 0 {
		max = 0
	}
	m.scrollPos = max
}

// getDisplayLines returns lines to display, handling word wrap if enabled.
func (m *DiffViewer) getDisplayLines() []string {
	if !m.wordWrap || m.width <= 0 {
		return m.lines
	}

	// Word wrap mode: wrap long lines (ANSI-aware)
	var wrapped []string
	for _, line := range m.lines {
		lineWidth := ansi.StringWidth(line)
		if lineWidth <= m.width {
			wrapped = append(wrapped, line)
		} else {
			// Wrap the line using visual positions
			pos := 0
			for pos < lineWidth {
				end := pos + m.width
				if end > lineWidth {
					end = lineWidth
				}
				wrapped = append(wrapped, ansi.Cut(line, pos, end))
				pos = end
			}
		}
	}
	return wrapped
}

// View renders the diff viewer.
func (m *DiffViewer) View() tea.View {
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
		lineWidth := ansi.StringWidth(line)

		if !m.wordWrap {
			// Apply horizontal scroll (ANSI-aware)
			if m.horizontalPos < lineWidth {
				line = ansi.Cut(line, m.horizontalPos, lineWidth)
				lineWidth = ansi.StringWidth(line)
			} else {
				line = ""
				lineWidth = 0
			}
		}

		// Truncate to screen width (ANSI-aware)
		if lineWidth > m.width {
			line = ansi.Cut(line, 0, m.width)
			lineWidth = m.width
		}

		// Pad to full width (accounting for visual width)
		if lineWidth < m.width {
			line = line + strings.Repeat(" ", m.width-lineWidth)
		}

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
	status = fmt.Sprintf(" [DIFF %s...HEAD]", m.mainBranch)
	if m.wordWrap {
		status += " [WRAP]"
	}
	status += " "

	if len(displayLines) > 0 {
		status += fmt.Sprintf("Lines %d-%d of %d ", m.scrollPos+1, endPos, len(displayLines))
	} else {
		status += "(no changes) "
	}

	// Keybindings hint (use ansi.StringWidth for unicode characters like ⌃)
	hint := "⌃D/⌃U:scroll w:wrap g/G:top/end q:close"
	padding := m.width - ansi.StringWidth(status) - ansi.StringWidth(hint)
	if padding < 0 {
		hint = "q:close"
		padding = m.width - ansi.StringWidth(status) - ansi.StringWidth(hint)
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
func (m *DiffViewer) contentHeight() int {
	// Reserve 1 line for status bar
	h := m.height - 1
	if h < 1 {
		h = 1
	}
	return h
}

// maxLineWidth returns the maximum visual width among all lines.
func (m *DiffViewer) maxLineWidth() int {
	maxWidth := 0
	for _, line := range m.lines {
		w := ansi.StringWidth(line)
		if w > maxWidth {
			maxWidth = w
		}
	}
	return maxWidth
}

// clearSelection clears the current selection.
func (m *DiffViewer) clearSelection() {
	m.selecting = false
	m.hasSelection = false
	m.selectStartY = 0
	m.selectStartX = 0
	m.selectEndY = 0
	m.selectEndX = 0
}

// getSelectionRange returns the normalized selection range (minY, maxY, startX, endX).
// startX/endX are adjusted based on selection direction.
func (m *DiffViewer) getSelectionRange() (minY, maxY, startX, endX int) {
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
func (m *DiffViewer) getSelectionXRange(screenY int) (int, int) {
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
func (m *DiffViewer) applySelectionToLine(line string, screenY int, highlightStyle lipgloss.Style) string {
	startX, endX := m.getSelectionXRange(screenY)
	if startX < 0 || startX >= endX {
		return line // Not in selection or invalid range
	}

	lineWidth := ansi.StringWidth(line)
	if startX >= lineWidth {
		return line // Selection starts beyond line
	}
	if endX > lineWidth {
		endX = lineWidth
	}

	// Split line into before, selected, and after parts (ANSI-aware)
	before := ""
	if startX > 0 {
		before = ansi.Cut(line, 0, startX)
	}
	selected := ansi.Cut(line, startX, endX)
	after := ""
	if endX < lineWidth {
		after = ansi.Cut(line, endX, lineWidth)
	}

	// Strip ANSI from selected text and apply highlight
	plainSelected := ansi.Strip(selected)

	return before + highlightStyle.Render(plainSelected) + after
}

// copySelection copies the selected text to clipboard.
func (m *DiffViewer) copySelection() {
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
		lineWidth := ansi.StringWidth(line)

		// Apply horizontal scroll offset for accurate X positions
		if !m.wordWrap && m.horizontalPos > 0 {
			if m.horizontalPos < lineWidth {
				line = ansi.Cut(line, m.horizontalPos, lineWidth)
				lineWidth = ansi.StringWidth(line)
			} else {
				line = ""
				lineWidth = 0
			}
		}

		// Truncate to screen width
		if lineWidth > m.width {
			line = ansi.Cut(line, 0, m.width)
			lineWidth = m.width
		}

		// Pad for consistent width
		if lineWidth < m.width {
			line = line + strings.Repeat(" ", m.width-lineWidth)
		}

		startX, endX := m.getSelectionXRange(screenY)
		if startX < 0 || startX >= endX {
			continue
		}
		if endX > m.width {
			endX = m.width
		}

		// Extract selected portion
		selected := ansi.Cut(line, startX, endX)
		plainSelected := strings.TrimRight(ansi.Strip(selected), " ")
		selectedLines = append(selectedLines, plainSelected)
	}

	if len(selectedLines) > 0 {
		text := strings.Join(selectedLines, "\n")
		_ = clipboard.WriteAll(text)
	}
}

// loadDiffOutput loads git diff output.
func (m *DiffViewer) loadDiffOutput() tea.Cmd {
	return func() tea.Msg {
		// git diff main...HEAD shows changes on the current branch since it diverged from main
		cmd := exec.Command("git", "diff", "--color=always", m.mainBranch+"...HEAD")
		cmd.Dir = m.workDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Check if it's just an empty diff (which is not an error)
			if len(output) == 0 {
				return diffOutputMsg{lines: []string{}}
			}
			return fmt.Errorf("git diff failed: %w\nOutput: %s", err, string(output))
		}

		// Split output into lines
		lines := strings.Split(string(output), "\n")
		// Remove last empty line if present
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		return diffOutputMsg{lines: lines}
	}
}

// RunDiffViewer runs the diff viewer for the given working directory.
func RunDiffViewer(workDir, mainBranch string) error {
	m := NewDiffViewer(workDir, mainBranch)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
