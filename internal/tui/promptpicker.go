package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/logging"
)

// PromptEntry represents an editable prompt.
type PromptEntry struct {
	ID          string // Internal identifier (e.g., "task-name", "merge-conflict")
	Name        string // Display name (e.g., "Task Name Rules")
	Description string // Short description
	Path        string // File path (empty if not yet created)
	Scope       string // "workspace" or "global"
}

// PromptPickerAction represents the selected action.
type PromptPickerAction int

const (
	PromptPickerCancel PromptPickerAction = iota
	PromptPickerSelect
)

// PromptPicker is a picker for editable prompts.
type PromptPicker struct {
	prompts  []PromptEntry
	cursor   int
	action   PromptPickerAction
	selected *PromptEntry

	isDark bool
	colors ThemeColors
	width  int
	height int

	// Style cache (reused across renders)
	styleTitle        lipgloss.Style
	styleSubtitle     lipgloss.Style
	styleItem         lipgloss.Style
	styleSelected     lipgloss.Style
	styleDesc         lipgloss.Style
	styleSelectedDesc lipgloss.Style
	styleScope        lipgloss.Style
	stylePath         lipgloss.Style
	styleHelp         lipgloss.Style
	styleDim          lipgloss.Style
	stylesCached      bool
}

// NewPromptPicker creates a new prompt picker.
func NewPromptPicker(prompts []PromptEntry) *PromptPicker {
	logging.Debug("-> NewPromptPicker(prompts=%d)", len(prompts))
	defer logging.Debug("<- NewPromptPicker")

	isDark := DetectDarkMode()

	return &PromptPicker{
		prompts: prompts,
		cursor:  0,
		isDark:  isDark,
		colors:  NewThemeColors(isDark),
		width:   80,
		height:  24,
	}
}

// Init initializes the prompt picker.
func (m *PromptPicker) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update handles messages.
func (m *PromptPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		m.stylesCached = false // Invalidate style cache on theme change
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "ctrl+y", "q":
			m.action = PromptPickerCancel
			return m, tea.Quit

		case "enter":
			if len(m.prompts) > 0 && m.cursor < len(m.prompts) {
				m.selected = &m.prompts[m.cursor]
				m.action = PromptPickerSelect
			} else {
				m.action = PromptPickerCancel
			}
			return m, tea.Quit

		case "up", "k", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j", "ctrl+n":
			if m.cursor < len(m.prompts)-1 {
				m.cursor++
			}
			return m, nil

		case "home", "g":
			m.cursor = 0
			return m, nil

		case "end", "G":
			m.cursor = len(m.prompts) - 1
			return m, nil
		}
	}

	return m, nil
}

// View renders the prompt picker.
func (m *PromptPicker) View() tea.View {
	c := m.colors

	// Update style cache if needed (only on theme change)
	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(c.Accent)
		m.styleSubtitle = lipgloss.NewStyle().
			Foreground(c.TextDim)
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
		m.styleScope = lipgloss.NewStyle().
			Foreground(c.TextDim).
			Italic(true)
		m.stylePath = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.styleHelp = lipgloss.NewStyle().
			Foreground(c.TextDim).
			MarginTop(1)
		m.styleDim = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.stylesCached = true
	}

	var sb strings.Builder

	// Title
	sb.WriteString(m.styleTitle.Render("Edit Prompts"))
	sb.WriteString("\n")
	sb.WriteString(m.styleSubtitle.Render("Select a prompt to edit with $EDITOR"))
	sb.WriteString("\n\n")

	// Prompt list
	if len(m.prompts) == 0 {
		sb.WriteString(m.styleDim.Render("  No prompts available"))
		sb.WriteString("\n")
	} else {
		for i, prompt := range m.prompts {
			// Name line
			scopeLabel := m.styleScope.Render("[" + prompt.Scope + "]")
			if i == m.cursor {
				sb.WriteString(m.styleSelected.Render("> " + prompt.Name + " " + scopeLabel))
				sb.WriteString("\n")
				sb.WriteString(m.styleSelectedDesc.Render(prompt.Description))
			} else {
				sb.WriteString(m.styleItem.Render(prompt.Name + " " + scopeLabel))
				sb.WriteString("\n")
				sb.WriteString(m.styleDesc.Render(prompt.Description))
			}
			sb.WriteString("\n")

			// Path line (if exists)
			if prompt.Path != "" {
				displayPath := truncateWithEllipsis(prompt.Path, m.width-8)
				sb.WriteString(m.stylePath.Render("      " + displayPath))
				sb.WriteString("\n")
			}
		}
	}

	// Help
	sb.WriteString("\n")
	sb.WriteString(m.styleHelp.Render("↑/↓: Navigate  Enter: Edit  Esc/⌃Y: Close"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

// Result returns the selected prompt and action.
func (m *PromptPicker) Result() (PromptPickerAction, *PromptEntry) {
	return m.action, m.selected
}

// RunPromptPicker runs the prompt picker and returns the selected prompt.
func RunPromptPicker(prompts []PromptEntry) (PromptPickerAction, *PromptEntry, error) {
	logging.Debug("-> RunPromptPicker(prompts=%d)", len(prompts))
	defer logging.Debug("<- RunPromptPicker")

	m := NewPromptPicker(prompts)
	logging.Debug("RunPromptPicker: starting tea.Program")
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		logging.Debug("RunPromptPicker: tea.Program.Run failed: %v", err)
		return PromptPickerCancel, nil, err
	}

	pp := finalModel.(*PromptPicker)
	action, selected := pp.Result()
	if selected != nil {
		logging.Debug("RunPromptPicker: completed, action=%d, selected=%s", action, selected.ID)
	} else {
		logging.Debug("RunPromptPicker: completed, action=%d, selected=nil", action)
	}
	return action, selected, nil
}
