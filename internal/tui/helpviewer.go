// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dongho-jung/paw/internal/config"
)

// HelpViewer provides an interactive help viewer with vim-like navigation.
type HelpViewer struct {
	lines         []string
	scrollPos     int
	horizontalPos int
	width         int
	height        int
	theme         config.Theme
	isDark        bool
	colors        ThemeColors

	// Mouse text selection state
	selecting    bool
	hasSelection bool
	selectStartY int // Start row (screen-relative)
	selectStartX int // Start column (screen-relative)
	selectEndY   int // End row (screen-relative)
	selectEndX   int // End column (screen-relative)
}

// NewHelpViewer creates a new help viewer with the given content.
func NewHelpViewer(content string) *HelpViewer {
	lines := strings.Split(content, "\n")
	// Remove last empty line if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Detect dark mode BEFORE bubbletea starts
	theme := loadThemeFromConfig()
	isDark := detectDarkMode(theme)

	return &HelpViewer{
		lines:  lines,
		theme:  theme,
		isDark: isDark,
		colors: NewThemeColors(isDark),
	}
}

// Init initializes the help viewer.
func (m *HelpViewer) Init() tea.Cmd {
	if m.theme == config.ThemeAuto {
		return tea.RequestBackgroundColor
	}
	return nil
}

// Update handles messages and updates the model.
func (m *HelpViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		if m.theme == config.ThemeAuto {
			m.isDark = msg.IsDark()
			m.colors = NewThemeColors(m.isDark)
			setCachedDarkMode(m.isDark)
		}
		return m, nil

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
	}

	return m, nil
}

// handleKey handles keyboard input.
func (m *HelpViewer) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	// Copy selection with Ctrl+C
	case "ctrl+c":
		if m.hasSelection {
			m.copySelection()
		}
		return m, nil

	// Close on q, Esc, or Ctrl+/ (which is ctrl+_)
	case "q", "esc", "ctrl+_", "ctrl+/", "ctrl+shift+/":
		return m, tea.Quit

	case "down", "j":
		m.scrollDown(1)

	case "up", "k":
		m.scrollUp(1)

	case "left", "h":
		if m.horizontalPos > 0 {
			m.horizontalPos -= 10
			if m.horizontalPos < 0 {
				m.horizontalPos = 0
			}
		}

	case "right", "l":
		m.horizontalPos += 10

	case "g", "home":
		m.scrollPos = 0
		m.horizontalPos = 0

	case "G", "end":
		m.scrollToEnd()
		m.horizontalPos = 0

	case "pgup", "ctrl+b":
		m.scrollUp(m.contentHeight())

	case "pgdown", "ctrl+f", " ":
		m.scrollDown(m.contentHeight())

	case "ctrl+u":
		m.scrollUp(m.contentHeight() / 2)

	case "ctrl+d":
		m.scrollDown(m.contentHeight() / 2)
	}

	return m, nil
}

// scrollUp scrolls up by n lines.
func (m *HelpViewer) scrollUp(n int) {
	m.scrollPos -= n
	if m.scrollPos < 0 {
		m.scrollPos = 0
	}
}

// scrollDown scrolls down by n lines.
func (m *HelpViewer) scrollDown(n int) {
	max := len(m.lines) - m.contentHeight()
	if max < 0 {
		max = 0
	}
	m.scrollPos += n
	if m.scrollPos > max {
		m.scrollPos = max
	}
}

// View renders the help viewer.
func (m *HelpViewer) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("Loading...")
	}

	var sb strings.Builder

	// Calculate visible lines
	contentHeight := m.contentHeight()
	endPos := m.scrollPos + contentHeight
	if endPos > len(m.lines) {
		endPos = len(m.lines)
	}

	c := m.colors

	// Selection highlight style
	highlightStyle := lipgloss.NewStyle().
		Background(c.Selection).
		Foreground(c.TextBright)

	// Render visible lines
	for i := m.scrollPos; i < endPos; i++ {
		screenY := i - m.scrollPos // Screen-relative Y position
		line := m.lines[i]
		lineWidth := ansi.StringWidth(line)

		// Apply horizontal scroll (visual width based)
		if m.horizontalPos > 0 {
			if m.horizontalPos < lineWidth {
				line = ansi.Cut(line, m.horizontalPos, lineWidth)
				lineWidth = ansi.StringWidth(line)
			} else {
				line = ""
				lineWidth = 0
			}
		}

		// Truncate to screen width (visual width based)
		if lineWidth > m.width {
			line = ansi.Cut(line, 0, m.width)
			lineWidth = m.width
		}

		// Pad to full width
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
		Background(c.StatusBar).
		Foreground(c.StatusBarText)

	var status string
	if len(m.lines) > 0 {
		status = fmt.Sprintf(" Lines %d-%d of %d ", m.scrollPos+1, endPos, len(m.lines))
	} else {
		status = " (empty) "
	}

	// Keybindings hint
	hint := "↑↓j/k:scroll g/G:top/end ⌃/:close"
	padding := m.width - len(status) - len(hint)
	if padding < 0 {
		hint = "⌃/:close"
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
func (m *HelpViewer) contentHeight() int {
	// Reserve 1 line for status bar
	h := m.height - 1
	if h < 1 {
		h = 1
	}
	return h
}

// scrollToEnd scrolls to the end of the content.
func (m *HelpViewer) scrollToEnd() {
	max := len(m.lines) - m.contentHeight()
	if max < 0 {
		max = 0
	}
	m.scrollPos = max
}

// getSelectionRange returns the normalized selection range (minY, maxY, startX, endX).
// startX/endX are adjusted based on selection direction.
func (m *HelpViewer) getSelectionRange() (minY, maxY, startX, endX int) {
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
func (m *HelpViewer) getSelectionXRange(screenY int) (int, int) {
	minY, maxY, startX, endX := m.getSelectionRange()

	if screenY < minY || screenY > maxY {
		return -1, -1 // Not in selection
	}

	if minY == maxY {
		// Single row selection
		return startX, endX
	}

	// Multi-row selection
	switch screenY {
	case minY:
		// First row: from startX to end of line
		return startX, m.width
	case maxY:
		// Last row: from start to endX
		return 0, endX
	default:
		// Middle rows: full line
		return 0, m.width
	}
}

// applySelectionToLine applies selection highlighting to a line.
func (m *HelpViewer) applySelectionToLine(line string, screenY int, highlightStyle lipgloss.Style) string {
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

	// Split line into before, selected, and after parts (visual width based)
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
func (m *HelpViewer) copySelection() {
	if !m.hasSelection {
		return
	}

	minY, maxY, _, _ := m.getSelectionRange()

	var selectedLines []string
	for screenY := minY; screenY <= maxY; screenY++ {
		lineIdx := m.scrollPos + screenY
		if lineIdx < 0 || lineIdx >= len(m.lines) {
			continue
		}

		line := m.lines[lineIdx]
		lineWidth := ansi.StringWidth(line)

		// Apply horizontal scroll offset for accurate X positions
		if m.horizontalPos > 0 {
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

// RunHelpViewer runs the help viewer with the given content.
func RunHelpViewer(content string) error {
	// Reset theme cache to ensure fresh detection on each TUI start
	ResetDarkModeCache()

	m := NewHelpViewer(content)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
