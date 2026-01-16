package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

// ProjectPickerAction represents the selected action.
type ProjectPickerAction int

const (
	ProjectPickerCancel ProjectPickerAction = iota
	ProjectPickerSelect
)

// ProjectPickerItem represents a PAW project session.
type ProjectPickerItem struct {
	Name       string // Session name (project name)
	SocketPath string // Tmux socket path
}

// ProjectPicker is a fuzzy-searchable project picker.
type ProjectPicker struct {
	input            textinput.Model
	inputOffset      int
	inputOffsetRight int
	projects         []ProjectPickerItem // All projects
	filtered         []int               // Indices into projects for filtered results
	cursor           int
	action           ProjectPickerAction
	selected         *ProjectPickerItem
	isDark           bool
	colors           ThemeColors
	width            int
	height           int
}

// NewProjectPicker creates a new project picker.
func NewProjectPicker(projects []ProjectPickerItem) *ProjectPicker {
	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Type to search projects..."
	ti.Focus()
	ti.CharLimit = 100
	ti.SetWidth(50)
	ti.VirtualCursor = false

	// Initialize filtered to all indices
	filtered := make([]int, len(projects))
	for i := range projects {
		filtered[i] = i
	}

	return &ProjectPicker{
		input:    ti,
		projects: projects,
		filtered: filtered,
		cursor:   0,
		isDark:   isDark,
		colors:   NewThemeColors(isDark),
		width:    70,
		height:   20,
	}
}

// Init initializes the project picker.
func (m *ProjectPicker) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tea.RequestBackgroundColor)
}

// Update handles messages.
func (m *ProjectPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Adjust input width
		inputWidth := min(50, m.width-10)
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
		case "ctrl+c", "esc", "ctrl+j":
			m.action = ProjectPickerCancel
			return m, tea.Quit

		case "enter", " ":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				m.selected = &m.projects[m.filtered[m.cursor]]
				m.action = ProjectPickerSelect
			} else {
				m.action = ProjectPickerCancel
			}
			return m, tea.Quit

		case "up", "ctrl+k", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil

		case "pgup", "ctrl+b":
			m.cursor -= 5
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil

		case "pgdown", "ctrl+f":
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

func (m *ProjectPicker) syncInputOffset() {
	syncTextInputOffset([]rune(m.input.Value()), m.input.Position(), m.input.Width(), &m.inputOffset, &m.inputOffsetRight)
}

// updateFiltered filters projects based on input.
func (m *ProjectPicker) updateFiltered() {
	query := m.input.Value()
	if query == "" {
		// Show all
		m.filtered = make([]int, len(m.projects))
		for i := range m.projects {
			m.filtered[i] = i
		}
		m.cursor = 0
		return
	}

	// Create searchable strings (project names)
	var searchables []string
	for _, p := range m.projects {
		searchables = append(searchables, p.Name)
	}

	// Fuzzy search
	matches := fuzzy.Find(query, searchables)

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

// renderInput prepares the text input line and cursor position.
func (m *ProjectPicker) renderInput() textInputRender {
	return renderTextInput(m.input.Value(), m.input.Position(), m.input.Width(), m.input.Placeholder, m.inputOffset, m.inputOffsetRight)
}

// View renders the project picker.
func (m *ProjectPicker) View() tea.View {
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

	var sb strings.Builder
	line := 0

	// Title
	sb.WriteString(titleStyle.Render("Switch Project"))
	sb.WriteString("\n\n")
	line += 2

	// Input - use custom rendering for proper Korean/CJK cursor positioning
	inputRender := m.renderInput()
	inputBox := inputStyle.Render(inputRender.Text)
	inputBoxTopY := line
	sb.WriteString(inputBox)
	sb.WriteString("\n\n")

	// Calculate available height for list
	// Reserve: title(1) + gap(1) + input(3) + gap(1) + help(2)
	reservedLines := 8
	listHeight := max(3, m.height-reservedLines)

	// Filtered projects
	if len(m.filtered) == 0 {
		if len(m.projects) == 0 {
			sb.WriteString(lipgloss.NewStyle().Foreground(c.TextDim).Render("  No other projects running"))
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(c.TextDim).Render("  No matching projects"))
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
			project := m.projects[idx]

			if i == m.cursor {
				sb.WriteString(selectedStyle.Render("> " + project.Name))
			} else {
				sb.WriteString(itemStyle.Render(project.Name))
			}
			sb.WriteString("\n")
		}

		// Show scroll indicator if needed
		if len(m.filtered) > listHeight {
			scrollInfo := lipgloss.NewStyle().Foreground(c.TextDim).Render(
				"  ... " + formatNumber(m.cursor+1) + "/" + formatNumber(len(m.filtered)))
			sb.WriteString(scrollInfo)
			sb.WriteString("\n")
		}
	}

	// Help
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("↑/↓: Navigate  Enter/Space: Switch  Esc/⌃J: Cancel"))

	v := tea.NewView(sb.String())
	if m.input.Focused() {
		cursor := tea.NewCursor(2+inputRender.CursorX, inputBoxTopY+1)
		cursor.Blink = m.input.Styles.Cursor.Blink
		cursor.Color = m.input.Styles.Cursor.Color
		cursor.Shape = m.input.Styles.Cursor.Shape
		v.Cursor = cursor
	}
	return v
}

// formatNumber formats a number for display (simple implementation).
func formatNumber(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// Result returns the selected project and action.
func (m *ProjectPicker) Result() (ProjectPickerAction, *ProjectPickerItem) {
	return m.action, m.selected
}

// RunProjectPicker runs the project picker and returns the selected project.
func RunProjectPicker(projects []ProjectPickerItem) (ProjectPickerAction, *ProjectPickerItem, error) {
	if len(projects) == 0 {
		return ProjectPickerCancel, nil, nil
	}

	// Reset theme cache to ensure fresh detection on each TUI start
	ResetDarkModeCache()

	m := NewProjectPicker(projects)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return ProjectPickerCancel, nil, err
	}

	picker := finalModel.(*ProjectPicker)
	action, selected := picker.Result()
	return action, selected, nil
}
