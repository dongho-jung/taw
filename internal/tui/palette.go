// Package tui provides terminal user interface components for TAW.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// PaletteCommand represents a command in the palette.
type PaletteCommand struct {
	Name        string
	Description string
}

// PaletteUI provides an interactive command palette with fuzzy search.
type PaletteUI struct {
	commands  []PaletteCommand
	filtered  []PaletteCommand
	query     string
	cursor    int
	width     int
	height    int
	done      bool
	cancelled bool
	selected  string
}

// NewPaletteUI creates a new palette UI with the given commands.
func NewPaletteUI(commands []PaletteCommand) *PaletteUI {
	return &PaletteUI{
		commands: commands,
		filtered: commands,
	}
}

// Init initializes the palette UI.
func (m *PaletteUI) Init() tea.Cmd {
	return nil
}

// Update handles input events.
func (m *PaletteUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c", "ctrl+d", "ctrl+r":
			m.cancelled = true
			m.done = true
			return m, tea.Quit

		case "enter":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				m.selected = m.filtered[m.cursor].Name
			}
			m.done = true
			return m, tea.Quit

		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil

		case "backspace":
			if len(m.query) > 0 {
				m.query = m.query[:len(m.query)-1]
				m.filterCommands()
			}
			return m, nil

		default:
			// Handle printable characters
			if len(msg.String()) == 1 {
				m.query += msg.String()
				m.filterCommands()
			}
			return m, nil
		}
	}

	return m, nil
}

// filterCommands filters commands based on query using fuzzy matching.
func (m *PaletteUI) filterCommands() {
	if m.query == "" {
		m.filtered = m.commands
		m.cursor = 0
		return
	}

	query := strings.ToLower(m.query)
	m.filtered = nil

	for _, cmd := range m.commands {
		// Check if query matches name or description
		name := strings.ToLower(cmd.Name)
		desc := strings.ToLower(cmd.Description)

		if fuzzyMatch(name, query) || fuzzyMatch(desc, query) {
			m.filtered = append(m.filtered, cmd)
		}
	}

	// Reset cursor if out of bounds
	if m.cursor >= len(m.filtered) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		} else {
			m.cursor = 0
		}
	}
}

// fuzzyMatch checks if text contains all characters of query in order.
func fuzzyMatch(text, query string) bool {
	if query == "" {
		return true
	}
	if len(query) > len(text) {
		return false
	}

	queryIdx := 0
	for i := 0; i < len(text) && queryIdx < len(query); i++ {
		if text[i] == query[queryIdx] {
			queryIdx++
		}
	}
	return queryIdx == len(query)
}

// View renders the palette UI.
func (m *PaletteUI) View() tea.View {
	var b strings.Builder

	// Styles
	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	queryStyle := lipgloss.NewStyle().Bold(true)
	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("4")).
		Foreground(lipgloss.Color("15")).
		Bold(true)
	normalStyle := lipgloss.NewStyle()
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Prompt line
	prompt := promptStyle.Render("> ")
	query := queryStyle.Render(m.query)
	cursor := "█"
	b.WriteString(prompt + query + cursor + "\n\n")

	// Command list
	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("  No matching commands"))
	} else {
		// Calculate visible range
		maxVisible := m.height - 4 // Account for prompt and margins
		if maxVisible < 1 {
			maxVisible = 5
		}

		start := 0
		if m.cursor >= maxVisible {
			start = m.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		for i := start; i < end; i++ {
			cmd := m.filtered[i]
			prefix := "  "

			var line string
			if i == m.cursor {
				prefix = "> "
				// Format: name (description)
				line = selectedStyle.Render(prefix + cmd.Name + " " + dimStyle.Render(cmd.Description))
			} else {
				line = normalStyle.Render(prefix+cmd.Name) + " " + dimStyle.Render(cmd.Description)
			}
			b.WriteString(line + "\n")
		}
	}

	return tea.NewView(b.String())
}

// Selected returns the selected command name, or empty string if cancelled.
func (m *PaletteUI) Selected() string {
	if m.cancelled {
		return ""
	}
	return m.selected
}

// IsCancelled returns true if the user cancelled the palette.
func (m *PaletteUI) IsCancelled() bool {
	return m.cancelled
}

// RunPalette runs the palette UI and returns the selected command name.
func RunPalette(commands []PaletteCommand) (string, error) {
	ui := NewPaletteUI(commands)
	p := tea.NewProgram(ui)

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	m := finalModel.(*PaletteUI)
	return m.Selected(), nil
}
