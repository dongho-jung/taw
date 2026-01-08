// Package tui provides terminal user interface components for PAW.
package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/tui/textarea"
)

// TemplateEditorMode represents the editor mode.
type TemplateEditorMode int

const (
	TemplateEditorModeCreate TemplateEditorMode = iota
	TemplateEditorModeEdit
)

// TemplateEditorField represents the focused field.
type TemplateEditorField int

const (
	TemplateEditorFieldName TemplateEditorField = iota
	TemplateEditorFieldContent
)

// TemplateEditorResult contains the result of template editing.
type TemplateEditorResult struct {
	Name      string
	Content   string
	Saved     bool
	Cancelled bool
}

// TemplateEditor provides a form for creating/editing templates.
type TemplateEditor struct {
	mode          TemplateEditorMode
	originalName  string // For edit mode - the original name
	nameInput     string
	contentArea   textarea.Model
	focusedField  TemplateEditorField
	width         int
	height        int
	done          bool
	saved         bool
	cancelled     bool
	isDark        bool
	nameCursor    int
}

// NewTemplateEditor creates a new template editor.
func NewTemplateEditor(mode TemplateEditorMode, name, content string) *TemplateEditor {
	// Detect dark mode before bubbletea starts
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	ta := textarea.New()
	ta.Placeholder = "Enter template content..."
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetWidth(60)
	ta.SetHeight(10)
	ta.VirtualCursor = false

	// Custom styling
	ta.Styles = textarea.DefaultStyles(true)
	ta.Styles.Focused.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1)
	ta.Styles.Blurred.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)
	ta.Styles.Blurred.Text = ta.Styles.Focused.Text
	ta.Styles.Blurred.Placeholder = ta.Styles.Focused.Placeholder
	ta.Styles.Focused.CursorLine = lipgloss.NewStyle()
	ta.Styles.Blurred.CursorLine = lipgloss.NewStyle()
	ta.Styles.Focused.Prompt = lipgloss.NewStyle()
	ta.Styles.Blurred.Prompt = lipgloss.NewStyle()

	// Set initial content
	if content != "" {
		ta.SetValue(content)
	}

	// Start focused on name field, so blur textarea
	ta.Blur()

	return &TemplateEditor{
		mode:         mode,
		originalName: name,
		nameInput:    name,
		contentArea:  ta,
		focusedField: TemplateEditorFieldName,
		isDark:       isDark,
		nameCursor:   len(name),
	}
}

// Init initializes the template editor.
func (m *TemplateEditor) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles messages and updates the model.
func (m *TemplateEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Adjust textarea size
		contentWidth := m.width - 4
		if contentWidth < 20 {
			contentWidth = 20
		}
		contentHeight := m.height - 10
		if contentHeight < 5 {
			contentHeight = 5
		}
		m.contentArea.SetWidth(contentWidth)
		m.contentArea.SetHeight(contentHeight)

	case tea.KeyMsg:
		keyStr := msg.String()

		switch keyStr {
		case "esc":
			m.cancelled = true
			m.done = true
			return m, tea.Quit

		case "ctrl+s", "f5":
			// Save
			name := strings.TrimSpace(m.nameInput)
			content := strings.TrimSpace(m.contentArea.Value())
			if name != "" && content != "" {
				m.saved = true
				m.done = true
				return m, tea.Quit
			}

		case "tab", "shift+tab":
			// Switch between fields
			if m.focusedField == TemplateEditorFieldName {
				m.focusedField = TemplateEditorFieldContent
				m.contentArea.Focus()
			} else {
				m.focusedField = TemplateEditorFieldName
				m.contentArea.Blur()
			}
			return m, nil
		}

		// Handle field-specific input
		if m.focusedField == TemplateEditorFieldName {
			switch keyStr {
			case "backspace":
				if m.nameCursor > 0 && len(m.nameInput) > 0 {
					// Delete character before cursor
					m.nameInput = m.nameInput[:m.nameCursor-1] + m.nameInput[m.nameCursor:]
					m.nameCursor--
				}
			case "delete":
				if m.nameCursor < len(m.nameInput) {
					// Delete character at cursor
					m.nameInput = m.nameInput[:m.nameCursor] + m.nameInput[m.nameCursor+1:]
				}
			case "left":
				if m.nameCursor > 0 {
					m.nameCursor--
				}
			case "right":
				if m.nameCursor < len(m.nameInput) {
					m.nameCursor++
				}
			case "home", "ctrl+a":
				m.nameCursor = 0
			case "end", "ctrl+e":
				m.nameCursor = len(m.nameInput)
			default:
				// Add printable characters
				if len(keyStr) == 1 && keyStr[0] >= 32 && keyStr[0] < 127 {
					// Insert at cursor position
					m.nameInput = m.nameInput[:m.nameCursor] + keyStr + m.nameInput[m.nameCursor:]
					m.nameCursor++
				}
			}
			return m, nil
		}
	}

	// Update textarea if focused
	if m.focusedField == TemplateEditorFieldContent {
		m.contentArea, cmd = m.contentArea.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the template editor.
func (m *TemplateEditor) View() tea.View {
	lightDark := lipgloss.LightDark(m.isDark)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	labelStyle := lipgloss.NewStyle().
		Foreground(normalColor).
		Width(10)

	focusedLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Width(10)

	helpStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	nameInputStyle := lipgloss.NewStyle().
		Foreground(normalColor).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(max(20, m.width-16))

	nameInputFocusedStyle := lipgloss.NewStyle().
		Foreground(normalColor).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(0, 1).
		Width(max(20, m.width-16))

	var sb strings.Builder

	// Title
	title := "Create Template"
	if m.mode == TemplateEditorModeEdit {
		title = "Edit Template"
	}
	sb.WriteString(titleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", max(0, m.width-2)))
	sb.WriteString("\n\n")

	// Name field
	nameLabel := labelStyle.Render("Name:")
	if m.focusedField == TemplateEditorFieldName {
		nameLabel = focusedLabelStyle.Render("Name:")
	}

	// Render name input with cursor
	var nameContent string
	if m.focusedField == TemplateEditorFieldName {
		// Show cursor in name input
		if m.nameCursor >= len(m.nameInput) {
			nameContent = m.nameInput + "▏"
		} else {
			nameContent = m.nameInput[:m.nameCursor] + "▏" + m.nameInput[m.nameCursor:]
		}
		nameContent = nameInputFocusedStyle.Render(nameContent)
	} else {
		if m.nameInput == "" {
			nameContent = nameInputStyle.Render(helpStyle.Render("Enter template name..."))
		} else {
			nameContent = nameInputStyle.Render(m.nameInput)
		}
	}

	sb.WriteString(nameLabel)
	sb.WriteString(nameContent)
	sb.WriteString("\n\n")

	// Content field
	contentLabel := labelStyle.Render("Content:")
	if m.focusedField == TemplateEditorFieldContent {
		contentLabel = focusedLabelStyle.Render("Content:")
	}
	sb.WriteString(contentLabel)
	sb.WriteString("\n")
	sb.WriteString(m.contentArea.View())
	sb.WriteString("\n")

	// Help text
	helpText := "Tab: Switch field  |  Ctrl+S/F5: Save  |  Esc: Cancel"
	sb.WriteString(helpStyle.Render(helpText))

	// Validation message
	name := strings.TrimSpace(m.nameInput)
	content := strings.TrimSpace(m.contentArea.Value())
	if name == "" || content == "" {
		sb.WriteString("\n")
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		if name == "" {
			sb.WriteString(warnStyle.Render("⚠ Name is required"))
		} else if content == "" {
			sb.WriteString(warnStyle.Render("⚠ Content is required"))
		}
	}

	v := tea.NewView(sb.String())
	v.AltScreen = true

	// Set cursor position based on focused field
	switch m.focusedField {
	case TemplateEditorFieldName:
		// Position cursor in name input
		// Account for: label (10) + border (1) + padding (1) + cursor position
		v.Cursor = tea.NewCursor(10+2+m.nameCursor, 3) // Y=3 after title and separator
	case TemplateEditorFieldContent:
		if cursor := m.contentArea.Cursor(); cursor != nil {
			cursor.Y += 6 // Account for title, name field, label
			cursor.X += 1
			v.Cursor = cursor
		}
	}

	return v
}

// Result returns the editor result.
func (m *TemplateEditor) Result() TemplateEditorResult {
	return TemplateEditorResult{
		Name:      strings.TrimSpace(m.nameInput),
		Content:   strings.TrimSpace(m.contentArea.Value()),
		Saved:     m.saved,
		Cancelled: m.cancelled,
	}
}

// RunTemplateEditor runs the template editor and returns the result.
func RunTemplateEditor(mode TemplateEditorMode, name, content string) (*TemplateEditorResult, error) {
	m := NewTemplateEditor(mode, name, content)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	editor := finalModel.(*TemplateEditor)
	result := editor.Result()
	return &result, nil
}
