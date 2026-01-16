package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	rw "github.com/mattn/go-runewidth"
	"github.com/sahilm/fuzzy"
)

// InputHistoryAction represents the selected action.
type InputHistoryAction int

const (
	InputHistoryCancel InputHistoryAction = iota
	InputHistorySelect
)

// InputHistoryPicker is a fuzzy-searchable input history picker.
type InputHistoryPicker struct {
	input            textinput.Model
	inputOffset      int
	inputOffsetRight int
	history          []string // All history entries
	filtered         []int    // Indices into history for filtered results
	cursor           int
	action           InputHistoryAction
	selected         string
	isDark           bool
	colors           ThemeColors
	width            int
	height           int
}

// NewInputHistoryPicker creates a new input history picker.
func NewInputHistoryPicker(history []string) *InputHistoryPicker {
	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Type to search history..."
	ti.Focus()
	ti.CharLimit = 200
	ti.SetWidth(60)
	ti.VirtualCursor = false

	// Initialize filtered to all indices
	filtered := make([]int, len(history))
	for i := range history {
		filtered[i] = i
	}

	return &InputHistoryPicker{
		input:    ti,
		history:  history,
		filtered: filtered,
		cursor:   0,
		isDark:   isDark,
		colors:   NewThemeColors(isDark),
		width:    80,
		height:   24,
	}
}

// Init initializes the input history picker.
func (m *InputHistoryPicker) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tea.RequestBackgroundColor)
}

// Update handles messages.
func (m *InputHistoryPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Adjust input width
		inputWidth := min(60, m.width-10)
		if inputWidth > 20 {
			m.input.SetWidth(inputWidth)
		}
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "ctrl+r":
			m.action = InputHistoryCancel
			return m, tea.Quit

		case "enter":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				m.selected = m.history[m.filtered[m.cursor]]
				m.action = InputHistorySelect
			} else {
				m.action = InputHistoryCancel
			}
			return m, tea.Quit

		case "up", "ctrl+k", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "ctrl+j", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil

		case "pgup", "ctrl+b":
			m.cursor -= 10
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil

		case "pgdown", "ctrl+f":
			m.cursor += 10
			if m.cursor >= len(m.filtered) {
				m.cursor = max(0, len(m.filtered)-1)
			}
			return m, nil

		case "ctrl+u":
			m.cursor -= 5
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil

		case "ctrl+d":
			m.cursor += 5
			if m.cursor >= len(m.filtered) {
				m.cursor = max(0, len(m.filtered)-1)
			}
			return m, nil
		}
	}

	// Update text input
	m.input, cmd = m.input.Update(msg)

	// Update filtered list based on input
	m.updateFiltered()
	m.syncInputOffset()

	return m, cmd
}

func (m *InputHistoryPicker) syncInputOffset() {
	syncTextInputOffset([]rune(m.input.Value()), m.input.Position(), m.input.Width(), &m.inputOffset, &m.inputOffsetRight)
}

// updateFiltered filters history based on input.
func (m *InputHistoryPicker) updateFiltered() {
	query := m.input.Value()
	if query == "" {
		// Show all
		m.filtered = make([]int, len(m.history))
		for i := range m.history {
			m.filtered[i] = i
		}
		m.cursor = 0
		return
	}

	// Fuzzy search
	matches := fuzzy.Find(query, m.history)

	// Build filtered list
	m.filtered = make([]int, len(matches))
	for i, match := range matches {
		m.filtered[i] = match.Index
	}

	// Reset cursor if out of bounds
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

// View renders the input history picker.
func (m *InputHistoryPicker) View() tea.View {
	c := m.colors

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(c.Accent)

	inputStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.BorderFocused).
		Padding(0, 1)

	itemStyle := lipgloss.NewStyle().
		Foreground(c.TextNormal).
		PaddingLeft(2)

	selectedStyle := lipgloss.NewStyle().
		Foreground(c.Accent).
		Bold(true).
		PaddingLeft(0)

	helpStyle := lipgloss.NewStyle().
		Foreground(c.TextDim).
		MarginTop(1)

	previewBorderStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Border).
		Padding(0, 1)

	previewTitleStyle := lipgloss.NewStyle().
		Foreground(c.TextDim).
		Bold(true)

	var sb strings.Builder
	line := 0

	// Title
	sb.WriteString(titleStyle.Render("Task History"))
	sb.WriteString("\n\n")
	line += 2

	// Input - use custom rendering for proper Korean/CJK cursor positioning
	inputRender := m.renderInput()
	inputBox := inputStyle.Render(inputRender.Text)
	inputBoxTopY := line
	sb.WriteString(inputBox)
	sb.WriteString("\n\n")

	// Calculate available height for list
	// Reserve: title(1) + gap(1) + input(3) + gap(1) + help(2) + preview area
	reservedLines := 10
	previewHeight := 5
	listHeight := max(3, m.height-reservedLines-previewHeight-2)

	// Filtered history
	if len(m.filtered) == 0 {
		if len(m.history) == 0 {
			sb.WriteString(lipgloss.NewStyle().Foreground(c.TextDim).Render("  No history yet"))
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(c.TextDim).Render("  No matching entries"))
		}
		sb.WriteString("\n")
	} else {
		// Calculate visible range (show items around cursor)
		start := 0
		if m.cursor >= listHeight {
			start = m.cursor - listHeight + 1
		}
		end := min(start+listHeight, len(m.filtered))

		for i := start; i < end; i++ {
			idx := m.filtered[i]
			content := m.history[idx]

			// Truncate long content for display
			displayContent := truncateWithEllipsis(content, m.width-10)
			// Replace newlines with space for single-line display
			displayContent = strings.ReplaceAll(displayContent, "\n", " ")

			if i == m.cursor {
				sb.WriteString(selectedStyle.Render("> " + displayContent))
			} else {
				sb.WriteString(itemStyle.Render(displayContent))
			}
			sb.WriteString("\n")
		}

		// Show scroll indicator if needed
		if len(m.filtered) > listHeight {
			scrollInfo := lipgloss.NewStyle().Foreground(c.TextDim).Render(
				strings.Repeat(" ", 2) + "... " + string(rune('0'+(m.cursor+1)/10)) + string(rune('0'+(m.cursor+1)%10)) + "/" + string(rune('0'+len(m.filtered)/10)) + string(rune('0'+len(m.filtered)%10)))
			sb.WriteString(scrollInfo)
			sb.WriteString("\n")
		}
	}

	// Preview of selected item (if any)
	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		idx := m.filtered[m.cursor]
		content := m.history[idx]

		sb.WriteString("\n")
		sb.WriteString(previewTitleStyle.Render("Preview:"))
		sb.WriteString("\n")

		// Show preview with limited height
		lines := strings.Split(content, "\n")
		previewLines := min(previewHeight, len(lines))
		previewContent := strings.Join(lines[:previewLines], "\n")
		if len(lines) > previewHeight {
			previewContent += "\n..."
		}

		// Truncate each line
		previewWidth := min(m.width-6, 70)
		truncatedLines := make([]string, 0)
		for _, line := range strings.Split(previewContent, "\n") {
			truncatedLines = append(truncatedLines, truncateWithEllipsis(line, previewWidth))
		}
		previewContent = strings.Join(truncatedLines, "\n")

		sb.WriteString(previewBorderStyle.Width(previewWidth).Render(previewContent))
	}

	// Help
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("↑/↓: Navigate  Enter: Select  Esc/⌃R: Cancel"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	if m.input.Focused() {
		cursor := tea.NewCursor(2+inputRender.CursorX, inputBoxTopY+1)
		cursor.Blink = m.input.Styles.Cursor.Blink
		cursor.Color = m.input.Styles.Cursor.Color
		cursor.Shape = m.input.Styles.Cursor.Shape
		v.Cursor = cursor
	}
	return v
}

func (m *InputHistoryPicker) renderInput() textInputRender {
	return renderTextInput(m.input.Value(), m.input.Position(), m.input.Width(), m.input.Placeholder, m.inputOffset, m.inputOffsetRight)
}

// truncateWithEllipsis truncates a string to maxLen display width and adds ellipsis if needed.
// It uses runewidth to correctly handle CJK characters (which are 2 cells wide).
func truncateWithEllipsis(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	if rw.StringWidth(s) <= maxLen {
		return s
	}

	// Truncate to fit maxLen-3 (leaving room for "...")
	runes := []rune(s)
	width := 0
	for i, r := range runes {
		charWidth := rw.RuneWidth(r)
		if width+charWidth > maxLen-3 {
			return string(runes[:i]) + "..."
		}
		width += charWidth
	}
	return s
}

// Result returns the selected content and action.
func (m *InputHistoryPicker) Result() (InputHistoryAction, string) {
	return m.action, m.selected
}

// RunInputHistoryPicker runs the input history picker and returns the selected content.
func RunInputHistoryPicker(history []string) (InputHistoryAction, string, error) {
	if len(history) == 0 {
		return InputHistoryCancel, "", nil
	}

	// Reset theme cache to ensure fresh detection on each TUI start
	ResetDarkModeCache()

	m := NewInputHistoryPicker(history)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return InputHistoryCancel, "", err
	}

	picker := finalModel.(*InputHistoryPicker)
	action, selected := picker.Result()
	return action, selected, nil
}
