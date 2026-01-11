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

	"github.com/dongho-jung/paw/internal/config"
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

	// Search state
	searchMode      bool   // whether search input is active
	searchQuery     string // current search term (confirmed)
	searchInput     string // input buffer while typing
	searchMatches   []int  // display line indices containing matches
	currentMatchIdx int    // current match index for n/N navigation
}

// NewDiffViewer creates a new diff viewer for the given working directory.
func NewDiffViewer(workDir, mainBranch string) *DiffViewer {
	// Detect dark mode BEFORE bubbletea starts
	theme := loadThemeFromConfig()
	isDark := detectDarkMode(theme)

	return &DiffViewer{
		workDir:    workDir,
		mainBranch: mainBranch,
		theme:      theme,
		isDark:     isDark,
		colors:     NewThemeColors(isDark),
	}
}

// Init initializes the diff viewer.
func (m *DiffViewer) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadDiffOutput()}
	if m.theme == config.ThemeAuto {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

// Update handles messages and updates the model.
func (m *DiffViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	// Handle search mode input
	if m.searchMode {
		return m.handleSearchKey(msg)
	}

	switch msg.String() {
	// Copy selection with Ctrl+C
	case "ctrl+c":
		if m.hasSelection {
			m.copySelection()
		}
		return m, nil

	// Close on q or Ctrl+Shift+D
	case "q", "ctrl+shift+d":
		return m, tea.Quit

	case "esc":
		// Clear search if active, otherwise quit
		if m.searchQuery != "" {
			m.clearSearch()
			return m, nil
		}
		return m, tea.Quit

	// Search mode
	case "/":
		m.searchMode = true
		m.searchInput = ""
		return m, nil

	// Next match
	case "n":
		if m.searchQuery != "" && len(m.searchMatches) > 0 {
			m.nextMatch()
		}
		return m, nil

	// Previous match
	case "N":
		if m.searchQuery != "" && len(m.searchMatches) > 0 {
			m.prevMatch()
		}
		return m, nil

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
		// Re-find matches after wrap change
		if m.searchQuery != "" {
			m.findMatches()
		}

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

// handleSearchKey handles keyboard input in search mode.
func (m *DiffViewer) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Confirm search
		m.searchMode = false
		m.searchQuery = m.searchInput
		if m.searchQuery != "" {
			m.findMatches()
			if len(m.searchMatches) > 0 {
				m.currentMatchIdx = 0
				m.scrollToMatch(0)
			}
		}
		return m, nil

	case "esc":
		// Cancel search mode
		m.searchMode = false
		m.searchInput = ""
		return m, nil

	case "backspace":
		if len(m.searchInput) > 0 {
			m.searchInput = m.searchInput[:len(m.searchInput)-1]
		}
		return m, nil

	default:
		// Append typed character
		if len(msg.String()) == 1 {
			m.searchInput += msg.String()
		}
		return m, nil
	}
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

		// Apply search highlighting before padding
		if m.searchQuery != "" && m.isMatchLine(i) {
			isCurrentMatch := m.isCurrentMatchLine(i)
			line = m.highlightSearchMatches(line, isCurrentMatch)
			lineWidth = ansi.StringWidth(line)
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
	c := m.colors
	statusStyle := lipgloss.NewStyle().
		Background(c.StatusBar).
		Foreground(c.StatusBarText)

	// Search input mode: show search bar instead of status
	if m.searchMode {
		searchBar := fmt.Sprintf("/%-*s", m.width-1, m.searchInput)
		if len(searchBar) > m.width {
			searchBar = searchBar[:m.width]
		}
		sb.WriteString(statusStyle.Render(searchBar))
		v := tea.NewView(sb.String())
		v.AltScreen = true
		v.MouseMode = tea.MouseModeAllMotion
		return v
	}

	var status string
	status = fmt.Sprintf(" [DIFF %s...HEAD]", m.mainBranch)
	if m.wordWrap {
		status += " [WRAP]"
	}
	// Show search info
	if m.searchQuery != "" {
		if len(m.searchMatches) > 0 {
			status += fmt.Sprintf(" [/%s %d/%d]", m.searchQuery, m.currentMatchIdx+1, len(m.searchMatches))
		} else {
			status += fmt.Sprintf(" [/%s 0/0]", m.searchQuery)
		}
	}
	status += " "

	if len(displayLines) > 0 {
		status += fmt.Sprintf("Lines %d-%d of %d ", m.scrollPos+1, endPos, len(displayLines))
	} else {
		status += "(no changes) "
	}

	// Keybindings hint (use ansi.StringWidth for unicode characters like ⌃)
	hint := "/:search n/N:match ⌃D/⌃U:scroll w:wrap q:close"
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

// clearSearch clears the current search state.
func (m *DiffViewer) clearSearch() {
	m.searchQuery = ""
	m.searchInput = ""
	m.searchMatches = nil
	m.currentMatchIdx = 0
}

// findMatches finds all lines containing the search query.
func (m *DiffViewer) findMatches() {
	m.searchMatches = nil
	if m.searchQuery == "" {
		return
	}

	displayLines := m.getDisplayLines()
	query := strings.ToLower(m.searchQuery)

	for i, line := range displayLines {
		// Strip ANSI codes for matching
		plainLine := strings.ToLower(ansi.Strip(line))
		if strings.Contains(plainLine, query) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
}

// scrollToMatch scrolls to make the match at the given index visible.
func (m *DiffViewer) scrollToMatch(idx int) {
	if idx < 0 || idx >= len(m.searchMatches) {
		return
	}

	lineIdx := m.searchMatches[idx]
	contentHeight := m.contentHeight()

	// If line is already visible, don't scroll
	if lineIdx >= m.scrollPos && lineIdx < m.scrollPos+contentHeight {
		return
	}

	// Scroll to put the match line in the middle of the screen
	m.scrollPos = lineIdx - contentHeight/2
	if m.scrollPos < 0 {
		m.scrollPos = 0
	}

	// Clamp to max scroll
	displayLines := m.getDisplayLines()
	maxScroll := len(displayLines) - contentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollPos > maxScroll {
		m.scrollPos = maxScroll
	}
}

// nextMatch moves to the next search match.
func (m *DiffViewer) nextMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.currentMatchIdx = (m.currentMatchIdx + 1) % len(m.searchMatches)
	m.scrollToMatch(m.currentMatchIdx)
}

// prevMatch moves to the previous search match.
func (m *DiffViewer) prevMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.currentMatchIdx--
	if m.currentMatchIdx < 0 {
		m.currentMatchIdx = len(m.searchMatches) - 1
	}
	m.scrollToMatch(m.currentMatchIdx)
}

// isMatchLine returns true if the display line at idx is a match line.
func (m *DiffViewer) isMatchLine(idx int) bool {
	for _, matchIdx := range m.searchMatches {
		if matchIdx == idx {
			return true
		}
	}
	return false
}

// isCurrentMatchLine returns true if the display line at idx is the current match.
func (m *DiffViewer) isCurrentMatchLine(idx int) bool {
	if len(m.searchMatches) == 0 || m.currentMatchIdx >= len(m.searchMatches) {
		return false
	}
	return m.searchMatches[m.currentMatchIdx] == idx
}

// highlightSearchMatches highlights search matches in a line (ANSI-aware).
func (m *DiffViewer) highlightSearchMatches(line string, isCurrentMatch bool) string {
	if m.searchQuery == "" {
		return line
	}

	// Strip ANSI for matching, but we'll rebuild with highlighting
	plainLine := ansi.Strip(line)
	lowerPlain := strings.ToLower(plainLine)
	lowerQuery := strings.ToLower(m.searchQuery)

	// Use different colors for current match vs other matches
	var matchStyle lipgloss.Style
	if isCurrentMatch {
		matchStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("208")). // Orange for current match
			Foreground(lipgloss.Color("0"))    // Black text
	} else {
		matchStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("226")). // Yellow for other matches
			Foreground(lipgloss.Color("0"))    // Black text
	}

	// Find all match positions in the plain text
	var matches [][2]int // [start, end] positions
	searchStart := 0
	for {
		idx := strings.Index(lowerPlain[searchStart:], lowerQuery)
		if idx == -1 {
			break
		}
		actualIdx := searchStart + idx
		matches = append(matches, [2]int{actualIdx, actualIdx + len(m.searchQuery)})
		searchStart = actualIdx + len(m.searchQuery)
	}

	if len(matches) == 0 {
		return line
	}

	// Rebuild the line with highlighting (using plain text since ANSI codes complicate highlighting)
	var result strings.Builder
	lastIdx := 0
	for _, match := range matches {
		result.WriteString(plainLine[lastIdx:match[0]])
		result.WriteString(matchStyle.Render(plainLine[match[0]:match[1]]))
		lastIdx = match[1]
	}
	result.WriteString(plainLine[lastIdx:])

	return result.String()
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
	// Reset theme cache to ensure fresh detection on each TUI start
	ResetDarkModeCache()

	m := NewDiffViewer(workDir, mainBranch)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
