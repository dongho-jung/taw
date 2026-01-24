package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dongho-jung/paw/internal/config"
)

// Pre-computed padded labels for options panel (avoids fmt.Sprintf per render)
const (
	optionLabelModel  = "Model:      " // 12 chars, left-aligned
	optionLabelBranch = "Branch:     " // 12 chars, left-aligned
)

// updateOptionsPanel handles key events when the options panel is focused.
func (m *TaskInput) updateOptionsPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()
	fieldCount := optFieldCount(m.isGitRepo)

	// Handle text input for branch name field (only in git mode)
	if m.isGitRepo && m.optField == OptFieldBranchName {
		switch keyStr {
		case "tab", "down":
			m.applyOptionInputValues()
			m.optField = OptField((int(m.optField) + 1) % fieldCount)
			return m, nil
		case "shift+tab", "up":
			m.applyOptionInputValues()
			m.optField = OptField((int(m.optField) - 1 + fieldCount) % fieldCount)
			return m, nil
		case "backspace":
			if len(m.branchName) > 0 {
				m.branchName = m.branchName[:len(m.branchName)-1]
			}
			return m, nil
		case "delete", "ctrl+u":
			m.branchName = ""
			return m, nil
		default:
			// Accept printable characters for branch name
			// Only allow valid branch name characters: a-z, 0-9, -, _
			key := msg.Key()
			if len(keyStr) == 1 && key.Mod == 0 {
				r := rune(keyStr[0])
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
					// Convert to lowercase for branch names
					if r >= 'A' && r <= 'Z' {
						r = r + 32
					}
					if len(m.branchName) >= 32 {
						return m, nil
					}
					m.branchName += string(r)
					return m, nil
				}
			}
		}
		return m, nil
	}

	switch keyStr {
	case "tab", "down", "j":
		m.applyOptionInputValues()
		m.optField = OptField((int(m.optField) + 1) % fieldCount)
		return m, nil

	case "shift+tab", "up", "k":
		m.applyOptionInputValues()
		m.optField = OptField((int(m.optField) - 1 + fieldCount) % fieldCount)
		return m, nil

	case "left", "h":
		m.handleOptionLeft()
		return m, nil

	case "right", "l":
		m.handleOptionRight()
		return m, nil

	}

	return m, nil
}

// handleOptionLeft handles left arrow key in options panel.
func (m *TaskInput) handleOptionLeft() {
	switch m.optField {
	case OptFieldModel:
		if m.modelIdx > 0 {
			m.modelIdx--
			m.options.Model = config.ValidModels()[m.modelIdx]
		}
	}
}

// handleOptionRight handles right arrow key in options panel.
func (m *TaskInput) handleOptionRight() {
	switch m.optField {
	case OptFieldModel:
		models := config.ValidModels()
		if m.modelIdx < len(models)-1 {
			m.modelIdx++
			m.options.Model = models[m.modelIdx]
		}
	}
}

// applyOptionInputValues applies current input values to options.
func (m *TaskInput) applyOptionInputValues() {
	if m.options == nil {
		return
	}
	m.options.BranchName = strings.TrimSpace(m.branchName)
}

// renderOptionsPanel renders the options panel for the right side.
// The panel width is dynamic to align with Kanban Done column.
func (m *TaskInput) renderOptionsPanel() string {
	isFocused := m.focusPanel == FocusPanelRight

	// Update style cache if needed (only on theme change)
	if !m.optStylesCached {
		// Adaptive colors for light/dark terminal themes
		lightDark := lipgloss.LightDark(m.isDark)
		normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))
		dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("243"))
		accentColor := lightDark(lipgloss.Color("25"), lipgloss.Color("39"))

		m.optStyleTitle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
		m.optStyleTitleDim = lipgloss.NewStyle().Bold(true).Foreground(dimColor)
		m.optStyleLabel = lipgloss.NewStyle().Foreground(normalColor)
		m.optStyleSelectedLabel = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
		m.optStyleValue = lipgloss.NewStyle().Foreground(normalColor)
		m.optStyleSelectedValue = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
		m.optStyleDim = lipgloss.NewStyle().Foreground(dimColor)
		m.optStylesCached = true
	}

	// Get border color based on focus (uses cached dim/accent colors via styles)
	borderColor := m.optStyleDim.GetForeground()
	if isFocused {
		borderColor = m.optStyleTitle.GetForeground()
	}

	// Build content lines with consistent visible width
	// Using explicit line-by-line approach to avoid Width() ANSI code issues
	// Inner width = panel width - padding(4) - border(2)
	// Use a low minimum to allow narrow terminals while ensuring content displays
	innerWidth := m.optionsPanelWidth - 6 // -6 for padding(4) + border(2)
	if innerWidth < 20 {
		innerWidth = 20 // Minimum to display labels
	}
	// Pre-allocate lines slice: title + empty + model + branch(if git) + fill lines
	lines := make([]string, 0, m.textareaHeight)

	// Title line (use cached styles)
	if isFocused {
		lines = append(lines, padToWidth(m.optStyleTitle.Render("Options"), innerWidth))
	} else {
		lines = append(lines, padToWidth(m.optStyleTitleDim.Render("Options"), innerWidth))
	}

	// Empty line (from MarginBottom effect)
	emptyLine := getPadding(innerWidth)
	lines = append(lines, emptyLine)

	// Model field (use cached styles)
	{
		isSelected := isFocused && m.optField == OptFieldModel
		label := m.optStyleLabel.Render(optionLabelModel)
		if isSelected {
			label = m.optStyleSelectedLabel.Render(optionLabelModel)
		}

		models := config.ValidModels()
		parts := make([]string, 0, len(models))
		for i, model := range models {
			if i == m.modelIdx {
				if isSelected {
					parts = append(parts, m.optStyleSelectedValue.Render("["+string(model)+"]"))
				} else {
					parts = append(parts, m.optStyleValue.Render("["+string(model)+"]"))
				}
			} else {
				parts = append(parts, m.optStyleDim.Render(" "+string(model)+" "))
			}
		}
		modelLine := label + strings.Join(parts, "")
		lines = append(lines, padToWidth(modelLine, innerWidth))
	}

	// Branch name field (only in git mode, use cached styles)
	if m.isGitRepo {
		isSelected := isFocused && m.optField == OptFieldBranchName
		label := m.optStyleLabel.Render(optionLabelBranch)
		if isSelected {
			label = m.optStyleSelectedLabel.Render(optionLabelBranch)
		}

		branchValue := m.branchName
		branchStyle := m.optStyleValue
		if branchValue == "" {
			branchValue = "auto"
			branchStyle = m.optStyleDim
		}
		if isSelected {
			branchStyle = m.optStyleSelectedValue
		}

		availableWidth := innerWidth - len(optionLabelBranch)
		if availableWidth < 0 {
			availableWidth = 0
		}
		if availableWidth > 0 && lipgloss.Width(branchValue) > availableWidth {
			branchValue = ansi.Truncate(branchValue, availableWidth, "")
		}

		branchLine := label + branchStyle.Render(branchValue)
		lines = append(lines, padToWidth(branchLine, innerWidth))
	}

	// Fill remaining height with empty lines (reuse cached padding)
	for len(lines) < m.textareaHeight {
		lines = append(lines, emptyLine)
	}

	// Apply border and padding to pre-formatted content
	panelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 2)

	return panelStyle.Render(strings.Join(lines, "\n"))
}
