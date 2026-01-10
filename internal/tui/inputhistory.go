package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/dongho-jung/paw/internal/config"
)

// InputHistoryAction represents the selected action.
type InputHistoryAction int

const (
	InputHistoryCancel InputHistoryAction = iota
	InputHistorySelect
)

// InputHistoryPicker is a fuzzy-searchable input history picker.
type InputHistoryPicker struct {
	input    textinput.Model
	history  []string // All history entries
	filtered []int    // Indices into history for filtered results
	cursor   int
	action   InputHistoryAction
	selected string
	theme    config.Theme
	isDark   bool
	width    int
	height   int
}

// NewInputHistoryPicker creates a new input history picker.
func NewInputHistoryPicker(history []string) *InputHistoryPicker {
	// Detect dark mode BEFORE bubbletea starts
	// Uses config theme setting if available, otherwise auto-detects
	theme := loadThemeFromConfig()
	isDark := detectDarkMode(theme)

	ti := textinput.New()
	ti.Placeholder = "Type to search history..."
	ti.Focus()
	ti.CharLimit = 200
	ti.SetWidth(60)

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
		theme:    theme,
		isDark:   isDark,
		width:    80,
		height:   24,
	}
}

// Init initializes the input history picker.
func (m *InputHistoryPicker) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.theme == config.ThemeAuto {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
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
		if m.theme == config.ThemeAuto {
			m.isDark = msg.IsDark()
			setCachedDarkMode(m.isDark)
		}

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

	return m, cmd
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
	lightDark := lipgloss.LightDark(m.isDark)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	inputStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1)

	itemStyle := lipgloss.NewStyle().
		Foreground(normalColor).
		PaddingLeft(2)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		PaddingLeft(0)

	helpStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		MarginTop(1)

	previewBorderStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(dimColor).
		Padding(0, 1)

	previewTitleStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Bold(true)

	var sb strings.Builder

	// Title
	sb.WriteString(titleStyle.Render("Task History"))
	sb.WriteString("\n\n")

	// Input
	sb.WriteString(inputStyle.Render(m.input.View()))
	sb.WriteString("\n\n")

	// Calculate available height for list
	// Reserve: title(1) + gap(1) + input(3) + gap(1) + help(2) + preview area
	reservedLines := 10
	previewHeight := 5
	listHeight := max(3, m.height-reservedLines-previewHeight-2)

	// Filtered history
	if len(m.filtered) == 0 {
		if len(m.history) == 0 {
			sb.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render("  No history yet"))
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render("  No matching entries"))
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
			scrollInfo := lipgloss.NewStyle().Foreground(dimColor).Render(
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

	return tea.NewView(sb.String())
}

// truncateWithEllipsis truncates a string to maxLen and adds ellipsis if needed.
func truncateWithEllipsis(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
