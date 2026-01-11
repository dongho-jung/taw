// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
)

// TemplateAction represents the action to take on a template.
type TemplateAction int

const (
	TemplateActionNone TemplateAction = iota
	TemplateActionSelect
	TemplateActionCreate
	TemplateActionEdit
	TemplateActionDelete
)

// TemplateUI provides an interactive template selector with fuzzy search.
type TemplateUI struct {
	templates     *config.Templates
	filtered      []config.Template
	cursor        int
	width         int
	height        int
	done          bool
	action        TemplateAction
	selected      *config.Template
	pawDir        string
	searchQuery   string
	previewScroll int
	previewLines  []string
	previewIndex  int
	theme         config.Theme
	isDark        bool
	colors        ThemeColors
}

// NewTemplateUI creates a new template UI.
func NewTemplateUI(pawDir string) *TemplateUI {
	// Detect dark mode BEFORE bubbletea starts
	theme := loadThemeFromConfig()
	isDark := detectDarkMode(theme)

	return &TemplateUI{
		pawDir:       pawDir,
		previewIndex: -1,
		theme:        theme,
		isDark:       isDark,
		colors:       NewThemeColors(isDark),
	}
}

// Init initializes the template UI.
func (m *TemplateUI) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadTemplates()}
	if m.theme == config.ThemeAuto {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

type templatesLoadedMsg struct {
	templates *config.Templates
}

// loadTemplates loads templates from storage.
func (m *TemplateUI) loadTemplates() tea.Cmd {
	return func() tea.Msg {
		templates, err := config.LoadTemplates(m.pawDir)
		if err != nil {
			templates = &config.Templates{Items: []config.Template{}}
		}
		return templatesLoadedMsg{templates: templates}
	}
}

func (m *TemplateUI) updateFilter() {
	m.filtered = m.templates.Filter(m.searchQuery)
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.previewIndex = -1
	m.previewScroll = 0
	m.updatePreviewCache()
}

func (m *TemplateUI) updatePreviewCache() {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		m.previewLines = nil
		m.previewIndex = -1
		return
	}

	if m.previewIndex == m.cursor {
		return
	}

	tmpl := m.filtered[m.cursor]
	content := tmpl.Content
	if content == "" {
		content = "(no content)"
	}

	m.previewLines = strings.Split(content, "\n")
	m.previewIndex = m.cursor
}

// Update handles messages and updates the model.
func (m *TemplateUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case templatesLoadedMsg:
		m.templates = msg.templates
		m.updateFilter()
		return m, nil

	case tea.BackgroundColorMsg:
		if m.theme == config.ThemeAuto {
			m.isDark = msg.IsDark()
			m.colors = NewThemeColors(m.isDark)
			setCachedDarkMode(m.isDark)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	return m, nil
}

// handleKey handles keyboard input.
func (m *TemplateUI) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "esc", "ctrl+t":
		m.done = true
		return m, tea.Quit

	case "ctrl+n":
		// Create new template
		m.action = TemplateActionCreate
		m.done = true
		return m, tea.Quit

	case "ctrl+e":
		// Edit selected template
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			m.action = TemplateActionEdit
			m.selected = &m.filtered[m.cursor]
			m.done = true
			return m, tea.Quit
		}

	case "ctrl+d":
		// Delete selected template
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			m.action = TemplateActionDelete
			m.selected = &m.filtered[m.cursor]
			m.done = true
			return m, tea.Quit
		}

	case "up", "ctrl+k":
		if m.cursor > 0 {
			m.cursor--
			m.previewScroll = 0
			m.updatePreviewCache()
		}

	case "down", "ctrl+j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.previewScroll = 0
			m.updatePreviewCache()
		}

	case "enter":
		// Select template
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			m.action = TemplateActionSelect
			m.selected = &m.filtered[m.cursor]
			m.done = true
			return m, tea.Quit
		}

	case "pgup", "ctrl+u":
		m.previewScroll -= 10
		if m.previewScroll < 0 {
			m.previewScroll = 0
		}

	case "pgdown":
		m.previewScroll += 10

	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.updateFilter()
		}

	default:
		// Add to search query (single printable characters)
		if len(keyStr) == 1 && keyStr[0] >= 32 && keyStr[0] < 127 {
			m.searchQuery += keyStr
			m.updateFilter()
		}
	}

	return m, nil
}

// View renders the template UI.
func (m *TemplateUI) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("Loading...")
	}
	if len(m.filtered) > 0 && m.previewIndex != m.cursor {
		m.updatePreviewCache()
	}

	c := m.colors

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(c.Accent)

	selectedStyle := lipgloss.NewStyle().
		Foreground(c.Accent).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(c.TextNormal)

	dimStyle := lipgloss.NewStyle().
		Foreground(c.TextDim)

	searchStyle := lipgloss.NewStyle().
		Foreground(c.Accent)

	previewStyle := lipgloss.NewStyle().
		Foreground(c.TextNormal)

	// Layout: left panel (template list) + right panel (preview)
	listWidth := m.width * 35 / 100
	if listWidth < 20 {
		if m.width < 20 {
			listWidth = m.width
		} else {
			listWidth = 20
		}
	}
	if listWidth < 1 {
		listWidth = 1
	}
	previewWidth := m.width - listWidth - 3
	if previewWidth < 0 {
		previewWidth = 0
	}

	var listBuilder strings.Builder
	var previewBuilder strings.Builder

	// Title and search
	listBuilder.WriteString(titleStyle.Render("Templates"))
	listBuilder.WriteString("\n")
	listBuilder.WriteString(strings.Repeat("─", max(0, listWidth-1)))
	listBuilder.WriteString("\n")

	// Search input
	searchPrompt := "🔍 "
	if m.searchQuery != "" {
		searchPrompt += searchStyle.Render(m.searchQuery)
	} else {
		searchPrompt += dimStyle.Render("type to search...")
	}
	listBuilder.WriteString(searchPrompt)
	listBuilder.WriteString("\n")
	listBuilder.WriteString("\n")

	// Template list
	listHeight := m.height - 7 // Reserve space for title, search, and status bar
	if listHeight < 1 {
		listHeight = 1
	}

	if len(m.filtered) == 0 {
		if m.searchQuery != "" {
			listBuilder.WriteString(dimStyle.Render("No templates match"))
		} else {
			listBuilder.WriteString(dimStyle.Render("No templates yet"))
			listBuilder.WriteString("\n")
			listBuilder.WriteString(dimStyle.Render("Press Ctrl+N to create"))
		}
	} else {
		// Calculate visible range
		start := 0
		if m.cursor >= listHeight {
			start = m.cursor - listHeight + 1
		}
		end := start + listHeight
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		linesWritten := 0
		for i := start; i < end; i++ {
			tmpl := m.filtered[i]

			// Cursor
			cursor := "  "
			style := normalStyle
			if i == m.cursor {
				cursor = "▸ "
				style = selectedStyle
			}

			// Truncate name if needed
			maxNameLen := listWidth - 4
			if maxNameLen < 1 {
				maxNameLen = 1
			}
			name := tmpl.Name
			if len(name) > maxNameLen {
				name = name[:maxNameLen-1] + "…"
			}

			line := cursor + style.Render(name)
			listBuilder.WriteString(line)
			listBuilder.WriteString("\n")
			linesWritten++
		}

		// Pad remaining lines
		for i := linesWritten; i < listHeight; i++ {
			listBuilder.WriteString("\n")
		}
	}

	// Preview panel
	previewBuilder.WriteString(titleStyle.Render("Preview"))
	previewBuilder.WriteString("\n")
	previewBuilder.WriteString(strings.Repeat("─", max(0, previewWidth-1)))
	previewBuilder.WriteString("\n")

	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		previewHeight := m.height - 5
		if previewHeight < 1 {
			previewHeight = 1
		}

		previewLines := m.previewLines
		if len(previewLines) == 0 {
			previewLines = []string{"(no content)"}
		}

		// Apply scroll
		if m.previewScroll >= len(previewLines) {
			m.previewScroll = len(previewLines) - 1
		}
		if m.previewScroll < 0 {
			m.previewScroll = 0
		}

		visibleStart := m.previewScroll
		visibleEnd := visibleStart + previewHeight
		if visibleEnd > len(previewLines) {
			visibleEnd = len(previewLines)
		}

		for i := visibleStart; i < visibleEnd; i++ {
			line := previewLines[i]
			maxPreviewLen := previewWidth - 2
			if maxPreviewLen < 1 {
				line = ""
			} else if len(line) > maxPreviewLen {
				if maxPreviewLen > 1 {
					line = line[:maxPreviewLen-1] + "…"
				} else {
					line = "…"
				}
			}
			previewBuilder.WriteString(previewStyle.Render(line))
			previewBuilder.WriteString("\n")
		}
	}

	// Combine panels with border
	listContent := listBuilder.String()
	previewContent := previewBuilder.String()

	listLines := strings.Split(listContent, "\n")
	previewLines := strings.Split(previewContent, "\n")

	var combined strings.Builder
	maxLines := listHeight + 5
	if len(listLines) > maxLines {
		listLines = listLines[:maxLines]
	}
	if len(previewLines) > maxLines {
		previewLines = previewLines[:maxLines]
	}

	for i := 0; i < maxLines; i++ {
		listLine := ""
		if i < len(listLines) {
			listLine = listLines[i]
		}
		previewLine := ""
		if i < len(previewLines) {
			previewLine = previewLines[i]
		}

		// Pad list line
		listLinePadded := listLine + strings.Repeat(" ", max(0, listWidth-lipgloss.Width(listLine)))
		combined.WriteString(listLinePadded)
		combined.WriteString(" │ ")
		combined.WriteString(previewLine)
		combined.WriteString("\n")
	}

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Background(c.StatusBar).
		Foreground(c.StatusBarText)

	statusHints := []string{"↑↓:nav", "⏎:select", "⌃N:new", "⌃E:edit", "⌃D:del", "Esc:close"}

	statusText := " " + strings.Join(statusHints, "  ") + " "
	if len(m.filtered) > 0 {
		statusText += "  │  " + dimStyle.Render(m.filtered[m.cursor].Name)
	}

	padding := m.width - lipgloss.Width(statusText)
	if padding < 0 {
		padding = 0
	}

	combined.WriteString(statusStyle.Render(statusText + strings.Repeat(" ", padding)))

	v := tea.NewView(combined.String())
	v.AltScreen = true
	return v
}

// Result returns the action and selected template.
func (m *TemplateUI) Result() (TemplateAction, *config.Template) {
	return m.action, m.selected
}

// RunTemplateUI runs the template UI and returns the action and selected template.
func RunTemplateUI(pawDir string) (TemplateAction, *config.Template, error) {
	// Reset theme cache to ensure fresh detection on each TUI start
	ResetDarkModeCache()

	m := NewTemplateUI(pawDir)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return TemplateActionNone, nil, err
	}

	ui := finalModel.(*TemplateUI)
	action, selected := ui.Result()
	return action, selected, nil
}
