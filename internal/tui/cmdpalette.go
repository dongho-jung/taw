package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/dongho-jung/paw/internal/logging"
)

// Command represents a command in the palette.
type Command struct {
	Name        string
	Description string
	ID          string // Internal identifier for action handling
}

// CommandPaletteAction represents the selected action.
type CommandPaletteAction int

const (
	CommandPaletteCancel CommandPaletteAction = iota
	CommandPaletteExecute
)

// CommandPalette is a fuzzy-searchable command palette.
type CommandPalette struct {
	input            textinput.Model
	inputOffset      int
	inputOffsetRight int
	commands         []Command
	filtered         []Command
	cursor           int
	action           CommandPaletteAction
	selected         *Command
	isDark           bool
	colors           ThemeColors

	// Style cache (reused across renders)
	styleTitle        lipgloss.Style
	styleInput        lipgloss.Style
	styleItem         lipgloss.Style
	styleSelected     lipgloss.Style
	styleDesc         lipgloss.Style
	styleSelectedDesc lipgloss.Style
	styleHelp         lipgloss.Style
	styleDim          lipgloss.Style
	stylesCached      bool
}

// NewCommandPalette creates a new command palette.
func NewCommandPalette(commands []Command) *CommandPalette {
	logging.Debug("-> NewCommandPalette(commands=%d)", len(commands))
	defer logging.Debug("<- NewCommandPalette")

	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Type to search..."
	ti.Focus()
	ti.CharLimit = 100
	ti.SetWidth(40)
	ti.VirtualCursor = false

	return &CommandPalette{
		input:    ti,
		commands: commands,
		filtered: commands,
		cursor:   0,
		isDark:   isDark,
		colors:   NewThemeColors(isDark),
	}
}

// Init initializes the command palette.
func (m *CommandPalette) Init() tea.Cmd {
	// Skip textinput.Blink since we use VirtualCursor = false (real cursor mode)
	// Only request background color if not already cached
	if _, ok := cachedDarkModeValue(); ok {
		return nil
	}
	return tea.RequestBackgroundColor
}

// Update handles messages.
func (m *CommandPalette) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		m.stylesCached = false // Invalidate style cache on theme change
		setCachedDarkMode(m.isDark)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "ctrl+p":
			m.action = CommandPaletteCancel
			return m, tea.Quit

		case "enter":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				m.selected = &m.filtered[m.cursor]
				m.action = CommandPaletteExecute
			} else {
				m.action = CommandPaletteCancel
			}
			return m, tea.Quit

		case "up", "ctrl+k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "ctrl+j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil

		case "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
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

func (m *CommandPalette) syncInputOffset() {
	syncTextInputOffset([]rune(m.input.Value()), m.input.Position(), m.input.Width(), &m.inputOffset, &m.inputOffsetRight)
}

// updateFiltered filters commands based on input.
func (m *CommandPalette) updateFiltered() {
	query := m.input.Value()
	if query == "" {
		m.filtered = m.commands
		m.cursor = 0
		return
	}

	// Create a list of searchable strings (name + description)
	searchables := make([]string, 0, len(m.commands))
	for _, cmd := range m.commands {
		searchables = append(searchables, cmd.Name+" "+cmd.Description)
	}

	// Fuzzy search
	matches := fuzzy.Find(query, searchables)

	// Build filtered list
	m.filtered = make([]Command, 0, len(matches))
	for _, match := range matches {
		m.filtered = append(m.filtered, m.commands[match.Index])
	}

	// Reset cursor if out of bounds
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

// renderInput prepares the text input line and cursor position.
func (m *CommandPalette) renderInput() textInputRender {
	return renderTextInput(m.input.Value(), m.input.Position(), m.input.Width(), m.input.Placeholder, m.inputOffset, m.inputOffsetRight)
}

// View renders the command palette.
func (m *CommandPalette) View() tea.View {
	c := m.colors

	// Update style cache if needed (only on theme change)
	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(c.Accent)
		m.styleInput = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(c.BorderFocused).
			Padding(0, 1).
			Width(46)
		m.styleItem = lipgloss.NewStyle().
			Foreground(c.TextNormal).
			PaddingLeft(2)
		m.styleSelected = lipgloss.NewStyle().
			Foreground(c.Accent).
			Bold(true).
			PaddingLeft(0)
		m.styleDesc = lipgloss.NewStyle().
			Foreground(c.TextDim).
			PaddingLeft(4)
		m.styleSelectedDesc = lipgloss.NewStyle().
			Foreground(c.Accent).
			PaddingLeft(4)
		m.styleHelp = lipgloss.NewStyle().
			Foreground(c.TextDim).
			MarginTop(1)
		m.styleDim = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.stylesCached = true
	}

	var sb strings.Builder
	line := 0

	// Title
	sb.WriteString(m.styleTitle.Render("Command Palette"))
	sb.WriteString("\n\n")
	line += 2

	// Input - use custom rendering for proper Korean/CJK cursor positioning
	inputRender := m.renderInput()
	inputBox := m.styleInput.Render(inputRender.Text)
	inputBoxTopY := line
	sb.WriteString(inputBox)
	sb.WriteString("\n\n")

	// Filtered commands
	if len(m.filtered) == 0 {
		sb.WriteString(m.styleDim.Render("  No matching commands"))
		sb.WriteString("\n")
	} else {
		// Show up to 10 commands
		maxItems := 10
		if len(m.filtered) < maxItems {
			maxItems = len(m.filtered)
		}

		for i := 0; i < maxItems; i++ {
			cmd := m.filtered[i]
			if i == m.cursor {
				sb.WriteString(m.styleSelected.Render("> " + cmd.Name))
				sb.WriteString("\n")
				sb.WriteString(m.styleSelectedDesc.Render(cmd.Description))
			} else {
				sb.WriteString(m.styleItem.Render(cmd.Name))
				sb.WriteString("\n")
				sb.WriteString(m.styleDesc.Render(cmd.Description))
			}
			sb.WriteString("\n")
		}

		if len(m.filtered) > maxItems {
			sb.WriteString(m.styleDim.Render("  ... and more"))
			sb.WriteString("\n")
		}
	}

	// Help
	sb.WriteString(m.styleHelp.Render("↑/↓: Navigate  Enter: Execute  Esc/⌃P: Close"))

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

// Result returns the selected command and action.
func (m *CommandPalette) Result() (CommandPaletteAction, *Command) {
	return m.action, m.selected
}

// RunCommandPalette runs the command palette and returns the selected command.
func RunCommandPalette(commands []Command) (CommandPaletteAction, *Command, error) {
	logging.Debug("-> RunCommandPalette(commands=%d)", len(commands))
	defer logging.Debug("<- RunCommandPalette")

	m := NewCommandPalette(commands)
	logging.Debug("RunCommandPalette: starting tea.Program")
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		logging.Debug("RunCommandPalette: tea.Program.Run failed: %v", err)
		return CommandPaletteCancel, nil, err
	}

	cp := finalModel.(*CommandPalette)
	action, selected := cp.Result()
	if selected != nil {
		logging.Debug("RunCommandPalette: completed, action=%d, selected=%s", action, selected.ID)
	} else {
		logging.Debug("RunCommandPalette: completed, action=%d, selected=nil", action)
	}
	return action, selected, nil
}
