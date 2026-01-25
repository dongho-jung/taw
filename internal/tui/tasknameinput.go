package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// TaskNameInputAction represents the selected action.
type TaskNameInputAction int

// Task name input action options.
const (
	TaskNameInputCancel TaskNameInputAction = iota
	TaskNameInputSubmit
)

// TaskNameInput is a simple text input for entering a task name.
type TaskNameInput struct {
	input            textinput.Model
	inputOffset      int
	inputOffsetRight int
	action           TaskNameInputAction
	taskName         string
	isDark           bool
	colors           ThemeColors
	width            int
	height           int
	errorMsg         string

	// Style cache (reused across renders)
	styleTitle   lipgloss.Style
	styleInput   lipgloss.Style
	styleHelp    lipgloss.Style
	styleError   lipgloss.Style
	styleHint    lipgloss.Style
	stylesCached bool
}

// NewTaskNameInput creates a new task name input.
func NewTaskNameInput() *TaskNameInput {
	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Enter task name (e.g., add-login-feature)"
	ti.Focus()
	ti.CharLimit = 32
	ti.SetWidth(50)
	ti.VirtualCursor = false

	return &TaskNameInput{
		input:  ti,
		isDark: isDark,
		colors: NewThemeColors(isDark),
		width:  60,
		height: 10,
	}
}

// Init initializes the task name input.
func (m *TaskNameInput) Init() tea.Cmd {
	// Only request background color if not already cached
	if _, ok := cachedDarkModeValue(); ok {
		return nil
	}
	return tea.RequestBackgroundColor
}

// validateTaskName validates the task name and returns an error message if invalid.
func validateTaskName(name string) string {
	name = strings.TrimSpace(name)

	if len(name) < 3 {
		return "Name must be at least 3 characters"
	}
	if len(name) > 32 {
		return "Name must be at most 32 characters"
	}

	// Check for valid characters (lowercase letters, numbers, hyphens)
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return "Use only lowercase letters, numbers, and hyphens"
		}
	}

	// Check for leading/trailing hyphens
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return "Name cannot start or end with a hyphen"
	}

	// Check for consecutive hyphens
	if strings.Contains(name, "--") {
		return "Name cannot contain consecutive hyphens"
	}

	return ""
}

// Update handles messages.
func (m *TaskNameInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.stylesCached = false // Invalidate style cache on theme change
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.action = TaskNameInputCancel
			return m, tea.Quit

		case "enter":
			name := strings.TrimSpace(m.input.Value())
			if errMsg := validateTaskName(name); errMsg != "" {
				m.errorMsg = errMsg
				return m, nil
			}
			m.taskName = name
			m.action = TaskNameInputSubmit
			return m, tea.Quit
		}
	}

	// Update text input
	m.input, cmd = m.input.Update(msg)

	// Clear error on input change
	m.errorMsg = ""

	// Sync offset for proper cursor positioning
	m.syncInputOffset()

	return m, cmd
}

func (m *TaskNameInput) syncInputOffset() {
	syncTextInputOffset([]rune(m.input.Value()), m.input.Position(), m.input.Width(), &m.inputOffset, &m.inputOffsetRight)
}

// renderInput prepares the text input line and cursor position.
func (m *TaskNameInput) renderInput() textInputRender {
	return renderTextInput(m.input.Value(), m.input.Position(), m.input.Width(), m.input.Placeholder, m.inputOffset, m.inputOffsetRight)
}

// View renders the task name input.
func (m *TaskNameInput) View() tea.View {
	c := m.colors

	// Update style cache if needed (only on theme change)
	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(c.Accent)
		m.styleInput = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(c.BorderFocused).
			Padding(0, 1)
		m.styleHelp = lipgloss.NewStyle().
			Foreground(c.TextDim).
			MarginTop(1)
		m.styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		m.styleHint = lipgloss.NewStyle().
			Foreground(c.TextDim).
			Italic(true)
		m.stylesCached = true
	}

	var sb strings.Builder
	line := 0

	// Title
	sb.WriteString(m.styleTitle.Render("Enter Task Name"))
	sb.WriteString("\n\n")
	line += 2

	// Input - use custom rendering for proper Korean/CJK cursor positioning
	inputRender := m.renderInput()
	inputBox := m.styleInput.Render(inputRender.Text)
	inputBoxTopY := line
	sb.WriteString(inputBox)
	sb.WriteString("\n")

	// Error or hint message
	if m.errorMsg != "" {
		sb.WriteString(m.styleError.Render(m.errorMsg))
	} else {
		sb.WriteString(m.styleHint.Render("3-32 chars: lowercase, numbers, hyphens"))
	}
	sb.WriteString("\n")

	// Help
	sb.WriteString(m.styleHelp.Render("Enter: Submit  Esc: Cancel"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	if m.input.Focused() {
		// Cursor position: X = border(1) + padding(1) + cursorX
		// Y = inputBoxTopY + 1 (skip top border row to reach content row)
		cursor := tea.NewCursor(2+inputRender.CursorX, inputBoxTopY+1)
		cursor.Blink = m.input.Styles.Cursor.Blink
		cursor.Color = m.input.Styles.Cursor.Color
		cursor.Shape = m.input.Styles.Cursor.Shape
		v.Cursor = cursor
	}
	return v
}

// Result returns the entered task name and action.
func (m *TaskNameInput) Result() (TaskNameInputAction, string) {
	return m.action, m.taskName
}

// RunTaskNameInput runs the task name input and returns the entered name.
func RunTaskNameInput() (TaskNameInputAction, string, error) {
	m := NewTaskNameInput()
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return TaskNameInputCancel, "", err
	}

	input := finalModel.(*TaskNameInput)
	action, name := input.Result()
	return action, name, nil
}
