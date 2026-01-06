package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sahilm/fuzzy"
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
	input    textinput.Model
	commands []Command
	filtered []Command
	cursor   int
	action   CommandPaletteAction
	selected *Command
	isDark   bool
}

// NewCommandPalette creates a new command palette.
func NewCommandPalette(commands []Command) *CommandPalette {
	// Detect dark mode BEFORE bubbletea starts
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.Focus()
	ti.CharLimit = 100
	ti.SetWidth(40)

	return &CommandPalette{
		input:    ti,
		commands: commands,
		filtered: commands,
		cursor:   0,
		isDark:   isDark,
	}
}

// Init initializes the command palette.
func (m *CommandPalette) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages.
func (m *CommandPalette) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
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

	return m, cmd
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
	var searchables []string
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

// View renders the command palette.
func (m *CommandPalette) View() tea.View {
	lightDark := lipgloss.LightDark(m.isDark)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	inputStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1).
		Width(46)

	itemStyle := lipgloss.NewStyle().
		Foreground(normalColor).
		PaddingLeft(2)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		PaddingLeft(0)

	descStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		PaddingLeft(4)

	selectedDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		PaddingLeft(2)

	helpStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		MarginTop(1)

	var sb strings.Builder

	// Title
	sb.WriteString(titleStyle.Render("Command Palette"))
	sb.WriteString("\n\n")

	// Input
	sb.WriteString(inputStyle.Render(m.input.View()))
	sb.WriteString("\n\n")

	// Filtered commands
	if len(m.filtered) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render("  No matching commands"))
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
				sb.WriteString(selectedStyle.Render("> " + cmd.Name))
				sb.WriteString("\n")
				sb.WriteString(selectedDescStyle.Render(cmd.Description))
			} else {
				sb.WriteString(itemStyle.Render(cmd.Name))
				sb.WriteString("\n")
				sb.WriteString(descStyle.Render(cmd.Description))
			}
			sb.WriteString("\n")
		}

		if len(m.filtered) > maxItems {
			sb.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(
				"  ... and more"))
			sb.WriteString("\n")
		}
	}

	// Help
	sb.WriteString(helpStyle.Render("↑/↓: Navigate  Enter: Execute  Esc/⌃P: Close"))

	return tea.NewView(sb.String())
}

// Result returns the selected command and action.
func (m *CommandPalette) Result() (CommandPaletteAction, *Command) {
	return m.action, m.selected
}

// RunCommandPalette runs the command palette and returns the selected command.
func RunCommandPalette(commands []Command) (CommandPaletteAction, *Command, error) {
	m := NewCommandPalette(commands)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return CommandPaletteCancel, nil, err
	}

	cp := finalModel.(*CommandPalette)
	action, selected := cp.Result()
	return action, selected, nil
}
