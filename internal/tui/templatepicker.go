package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/service"
)

// TemplatePickerAction represents the selected action.
type TemplatePickerAction int

// Template picker action options.
const (
	TemplatePickerCancel TemplatePickerAction = iota
	TemplatePickerSelect
)

// TemplatePicker is a fuzzy-searchable template picker.
type TemplatePicker struct {
	input            textinput.Model
	inputOffset      int
	inputOffsetRight int

	nameInput       textinput.Model
	nameOffset      int
	nameOffsetRight int
	prompting       bool

	templates    []service.TemplateEntry
	filtered     []int
	cursor       int
	action       TemplatePickerAction
	selected     *service.TemplateEntry
	draftContent string
	dirty        bool

	deleting    bool
	deletingIdx int

	isDark bool
	colors ThemeColors
	width  int
	height int

	// Style cache (reused across renders)
	styleTitle         lipgloss.Style
	styleInput         lipgloss.Style
	styleItem          lipgloss.Style
	styleSelected      lipgloss.Style
	styleHelp          lipgloss.Style
	styleDim           lipgloss.Style
	stylePreviewBorder lipgloss.Style
	stylePreviewTitle  lipgloss.Style
	styleError         lipgloss.Style
	styleName          lipgloss.Style
	stylesCached       bool
}

// NewTemplatePicker creates a new template picker.
func NewTemplatePicker(templates []service.TemplateEntry, draftContent string) *TemplatePicker {
	logging.Debug("NewTemplatePicker: templates=%d draftBytes=%d", len(templates), len(draftContent))

	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Type to search templates..."
	ti.Focus()
	ti.CharLimit = 100
	ti.SetWidth(60)
	ti.VirtualCursor = false

	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "Template name..."
	ni.CharLimit = 100
	ni.SetWidth(40)
	ni.VirtualCursor = false

	filtered := make([]int, len(templates))
	for i := range templates {
		filtered[i] = i
	}

	return &TemplatePicker{
		input:        ti,
		nameInput:    ni,
		templates:    templates,
		filtered:     filtered,
		cursor:       0,
		action:       TemplatePickerCancel,
		draftContent: draftContent,
		isDark:       isDark,
		colors:       NewThemeColors(isDark),
		width:        80,
		height:       24,
	}
}

// Init initializes the template picker.
func (m *TemplatePicker) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tea.RequestBackgroundColor)
}

// Update handles messages.
func (m *TemplatePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		inputWidth := min(60, m.width-10)
		if inputWidth > 20 {
			m.input.SetWidth(inputWidth)
		}

		nameWidth := min(50, m.width-10)
		if nameWidth > 20 {
			m.nameInput.SetWidth(nameWidth)
		}

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		m.stylesCached = false // Invalidate style cache on theme change
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.KeyMsg:
		if m.deleting {
			return m.handleDeleteConfirmKey(msg)
		}

		if m.prompting {
			return m.handleNamePromptKey(msg)
		}

		switch msg.String() {
		case "ctrl+c", "esc", "ctrl+t":
			m.action = TemplatePickerCancel
			return m, tea.Quit

		case "enter":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				selectedIdx := m.filtered[m.cursor]
				m.selected = &m.templates[selectedIdx]
				logging.Debug("templatePicker: selected name=%s contentBytes=%d", m.selected.Name, len(m.selected.Content))
				m.action = TemplatePickerSelect
			} else {
				logging.Debug("templatePicker: enter with no selection")
				m.action = TemplatePickerCancel
			}
			return m, tea.Quit

		case "up", "ctrl+k", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "ctrl+j":
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

		case "ctrl+n":
			logging.Debug("templatePicker: start new template prompt (draftBytes=%d)", len(m.draftContent))
			m.startNamePrompt()
			return m, nil

		case "ctrl+d":
			m.startDeleteConfirm()
			return m, nil

		case "ctrl+u":
			logging.Debug("templatePicker: update template request (draftBytes=%d)", len(m.draftContent))
			m.updateSelectedTemplate()
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

func (m *TemplatePicker) handleNamePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc", "ctrl+t":
		m.endNamePrompt()
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		if name != "" {
			m.addOrUpdateTemplate(name)
		}
		m.endNamePrompt()
		return m, nil
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	m.syncNameOffset()
	return m, cmd
}

func (m *TemplatePicker) handleDeleteConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc", "ctrl+t", "n":
		m.endDeleteConfirm()
		return m, nil
	case "enter", "y":
		m.confirmDelete()
		return m, nil
	}
	return m, nil
}

func (m *TemplatePicker) startDeleteConfirm() {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return
	}
	idx := m.filtered[m.cursor]
	if idx < 0 || idx >= len(m.templates) {
		return
	}

	logging.Debug("templatePicker: start delete confirm name=%s", m.templates[idx].Name)
	m.deleting = true
	m.deletingIdx = idx
	m.input.Blur()
}

func (m *TemplatePicker) endDeleteConfirm() {
	logging.Debug("templatePicker: cancel delete confirm")
	m.deleting = false
	m.deletingIdx = -1
	m.input.Focus()
}

func (m *TemplatePicker) confirmDelete() {
	if m.deletingIdx < 0 || m.deletingIdx >= len(m.templates) {
		m.endDeleteConfirm()
		return
	}

	logging.Debug("templatePicker: confirm delete name=%s", m.templates[m.deletingIdx].Name)
	m.templates = append(m.templates[:m.deletingIdx], m.templates[m.deletingIdx+1:]...)
	m.dirty = true
	m.deleting = false
	m.deletingIdx = -1
	m.updateFiltered()
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.input.Focus()
}

func (m *TemplatePicker) startNamePrompt() {
	m.prompting = true
	m.nameInput.SetValue("")
	m.nameInput.Focus()
	m.input.Blur()
	m.syncNameOffset()
}

func (m *TemplatePicker) endNamePrompt() {
	m.prompting = false
	m.nameInput.SetValue("")
	m.nameInput.Blur()
	m.input.Focus()
	m.syncInputOffset()
}

func (m *TemplatePicker) addOrUpdateTemplate(name string) {
	now := time.Now()
	for i := range m.templates {
		if m.templates[i].Name == name {
			logging.Debug("templatePicker: update existing template name=%s draftBytes=%d", name, len(m.draftContent))
			m.templates[i].Content = m.draftContent
			m.templates[i].UpdatedAt = now
			m.dirty = true
			m.updateFiltered()
			return
		}
	}

	entry := service.TemplateEntry{
		Name:      name,
		Content:   m.draftContent,
		UpdatedAt: now,
	}
	logging.Debug("templatePicker: add new template name=%s draftBytes=%d", name, len(m.draftContent))
	m.templates = append([]service.TemplateEntry{entry}, m.templates...)
	m.dirty = true
	m.updateFiltered()
}

func (m *TemplatePicker) updateSelectedTemplate() {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return
	}
	idx := m.filtered[m.cursor]
	if idx < 0 || idx >= len(m.templates) {
		return
	}

	logging.Debug("templatePicker: update selected name=%s draftBytes=%d", m.templates[idx].Name, len(m.draftContent))
	m.templates[idx].Content = m.draftContent
	m.templates[idx].UpdatedAt = time.Now()
	m.dirty = true
	m.updateFiltered()
}

func (m *TemplatePicker) syncInputOffset() {
	syncTextInputOffset([]rune(m.input.Value()), m.input.Position(), m.input.Width(), &m.inputOffset, &m.inputOffsetRight)
}

func (m *TemplatePicker) syncNameOffset() {
	syncTextInputOffset([]rune(m.nameInput.Value()), m.nameInput.Position(), m.nameInput.Width(), &m.nameOffset, &m.nameOffsetRight)
}

// updateFiltered filters templates based on input.
func (m *TemplatePicker) updateFiltered() {
	query := m.input.Value()
	if query == "" {
		m.filtered = make([]int, len(m.templates))
		for i := range m.templates {
			m.filtered[i] = i
		}
		if m.cursor >= len(m.filtered) {
			m.cursor = max(0, len(m.filtered)-1)
		}
		return
	}

	searchables := make([]string, len(m.templates))
	for i, t := range m.templates {
		searchables[i] = t.Name + " " + t.Content
	}

	matches := fuzzy.Find(query, searchables)
	m.filtered = make([]int, len(matches))
	for i, match := range matches {
		m.filtered[i] = match.Index
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

func (m *TemplatePicker) renderInput() textInputRender {
	return renderTextInput(m.input.Value(), m.input.Position(), m.input.Width(), m.input.Placeholder, m.inputOffset, m.inputOffsetRight)
}

func (m *TemplatePicker) renderNameInput() textInputRender {
	return renderTextInput(m.nameInput.Value(), m.nameInput.Position(), m.nameInput.Width(), m.nameInput.Placeholder, m.nameOffset, m.nameOffsetRight)
}

// ensureStylesCached initializes styles if needed.
func (m *TemplatePicker) ensureStylesCached() {
	if m.stylesCached {
		return
	}
	c := m.colors
	m.styleTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(c.Accent)
	m.styleInput = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.BorderFocused).
		Padding(0, 1)
	m.styleItem = lipgloss.NewStyle().
		Foreground(c.TextNormal).
		PaddingLeft(2)
	m.styleSelected = lipgloss.NewStyle().
		Foreground(c.Accent).
		Bold(true).
		PaddingLeft(0)
	m.styleHelp = lipgloss.NewStyle().
		Foreground(c.TextDim).
		MarginTop(1)
	m.styleDim = lipgloss.NewStyle().
		Foreground(c.TextDim)
	m.stylePreviewBorder = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Border).
		Padding(0, 1)
	m.stylePreviewTitle = lipgloss.NewStyle().
		Foreground(c.TextDim).
		Bold(true)
	m.styleError = lipgloss.NewStyle().
		Bold(true).
		Foreground(c.ErrorColor)
	m.styleName = lipgloss.NewStyle().
		Bold(true).
		Foreground(c.Accent)
	m.stylesCached = true
}

// View renders the template picker.
func (m *TemplatePicker) View() tea.View {
	if m.deleting {
		return m.deleteConfirmView()
	}

	if m.prompting {
		return m.namePromptView()
	}

	m.ensureStylesCached()

	var sb strings.Builder
	line := 0

	sb.WriteString(m.styleTitle.Render("Templates"))
	sb.WriteString("\n\n")
	line += 2

	inputRender := m.renderInput()
	inputBox := m.styleInput.Render(inputRender.Text)
	inputBoxTopY := line
	sb.WriteString(inputBox)
	sb.WriteString("\n\n")

	reservedLines := 10
	previewHeight := 5
	listHeight := max(3, m.height-reservedLines-previewHeight-2)

	if len(m.filtered) == 0 {
		if len(m.templates) == 0 {
			sb.WriteString(m.styleDim.Render("  No templates yet"))
		} else {
			sb.WriteString(m.styleDim.Render("  No matching templates"))
		}
		sb.WriteString("\n")
	} else {
		start := 0
		if m.cursor >= listHeight {
			start = m.cursor - listHeight + 1
		}
		end := min(start+listHeight, len(m.filtered))

		for i := start; i < end; i++ {
			idx := m.filtered[i]
			name := m.templates[idx].Name
			displayName := truncateWithEllipsis(name, m.width-10)

			if i == m.cursor {
				sb.WriteString(m.styleSelected.Render("> " + displayName))
			} else {
				sb.WriteString(m.styleItem.Render(displayName))
			}
			sb.WriteString("\n")
		}
	}

	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		idx := m.filtered[m.cursor]
		content := m.templates[idx].Content

		sb.WriteString("\n")
		sb.WriteString(m.stylePreviewTitle.Render("Preview:"))
		sb.WriteString("\n")

		// Split once, reuse for truncation
		lines := strings.Split(content, "\n")
		previewLines := min(previewHeight, len(lines))

		previewWidth := min(m.width-6, 70)
		truncatedLines := make([]string, 0, previewLines+1)
		for i := 0; i < previewLines; i++ {
			truncatedLines = append(truncatedLines, truncateWithEllipsis(lines[i], previewWidth))
		}
		if len(lines) > previewHeight {
			truncatedLines = append(truncatedLines, "...")
		}
		previewContent := strings.Join(truncatedLines, "\n")

		sb.WriteString(m.stylePreviewBorder.Width(previewWidth).Render(previewContent))
	}

	sb.WriteString("\n")
	sb.WriteString(m.styleHelp.Render("↑/↓: Navigate  Enter: Apply  Ctrl+N: New  Ctrl+U: Update  Ctrl+D: Delete  Esc/⌃T: Cancel"))

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

func (m *TemplatePicker) namePromptView() tea.View {
	m.ensureStylesCached()

	var sb strings.Builder
	line := 0

	sb.WriteString(m.styleTitle.Render("New Template"))
	sb.WriteString("\n\n")
	line += 2

	inputRender := m.renderNameInput()
	inputBox := m.styleInput.Render(inputRender.Text)
	inputBoxTopY := line
	sb.WriteString(inputBox)
	sb.WriteString("\n")

	sb.WriteString(m.styleHelp.Render("Enter: Save  Esc: Cancel"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	if m.nameInput.Focused() {
		cursor := tea.NewCursor(2+inputRender.CursorX, inputBoxTopY+1)
		cursor.Blink = m.nameInput.Styles.Cursor.Blink
		cursor.Color = m.nameInput.Styles.Cursor.Color
		cursor.Shape = m.nameInput.Styles.Cursor.Shape
		v.Cursor = cursor
	}
	return v
}

func (m *TemplatePicker) deleteConfirmView() tea.View {
	m.ensureStylesCached()

	var sb strings.Builder

	sb.WriteString(m.styleError.Render("Delete Template"))
	sb.WriteString("\n\n")

	templateName := ""
	if m.deletingIdx >= 0 && m.deletingIdx < len(m.templates) {
		templateName = m.templates[m.deletingIdx].Name
	}

	sb.WriteString("Are you sure you want to delete ")
	sb.WriteString(m.styleName.Render(templateName))
	sb.WriteString("?\n\n")

	sb.WriteString(m.styleHelp.Render("Y/Enter: Delete  N/Esc: Cancel"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

// Result returns the selected content and action.
func (m *TemplatePicker) Result() (TemplatePickerAction, *service.TemplateEntry, []service.TemplateEntry, bool) {
	return m.action, m.selected, m.templates, m.dirty
}

// RunTemplatePicker runs the template picker and returns the selected content.
func RunTemplatePicker(templates []service.TemplateEntry, draftContent string) (TemplatePickerAction, *service.TemplateEntry, []service.TemplateEntry, bool, error) {
	m := NewTemplatePicker(templates, draftContent)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return TemplatePickerCancel, nil, templates, false, err
	}

	picker := finalModel.(*TemplatePicker)
	action, selected, updated, dirty := picker.Result()
	return action, selected, updated, dirty, nil
}
