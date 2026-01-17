package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dongho-jung/paw/internal/config"
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

	// Adaptive colors for light/dark terminal themes (use cached isDark value)
	// Light theme: use darker colors for visibility on light backgrounds
	// Dark theme: use lighter colors for visibility on dark backgrounds
	lightDark := lipgloss.LightDark(m.isDark)
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))
	// Dim color: medium contrast for non-selected items (readable but clearly dimmed)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("243"))
	// Accent color: darker blue for light bg, bright cyan for dark bg
	accentColor := lightDark(lipgloss.Color("25"), lipgloss.Color("39"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor)

	titleDimStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(dimColor)

	labelStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	selectedLabelStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)

	valueStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	selectedValueStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	borderColor := dimColor
	if isFocused {
		borderColor = accentColor
	}

	// Build content lines with consistent visible width
	// Using explicit line-by-line approach to avoid Width() ANSI code issues
	// Inner width = panel width - padding(4) - border(2)
	// Use a low minimum to allow narrow terminals while ensuring content displays
	innerWidth := m.optionsPanelWidth - 6 // -6 for padding(4) + border(2)
	if innerWidth < 20 {
		innerWidth = 20 // Minimum to display labels
	}
	var lines []string

	// Title line
	if isFocused {
		lines = append(lines, padToWidth(titleStyle.Render("Options"), innerWidth))
	} else {
		lines = append(lines, padToWidth(titleDimStyle.Render("Options"), innerWidth))
	}

	// Empty line (from MarginBottom effect)
	lines = append(lines, strings.Repeat(" ", innerWidth))

	// Model field
	{
		isSelected := isFocused && m.optField == OptFieldModel
		paddedLabel := fmt.Sprintf("%-12s", "Model:")
		label := labelStyle.Render(paddedLabel)
		if isSelected {
			label = selectedLabelStyle.Render(paddedLabel)
		}

		models := config.ValidModels()
		var parts []string
		for i, model := range models {
			if i == m.modelIdx {
				if isSelected {
					parts = append(parts, selectedValueStyle.Render("["+string(model)+"]"))
				} else {
					parts = append(parts, valueStyle.Render("["+string(model)+"]"))
				}
			} else {
				parts = append(parts, dimStyle.Render(" "+string(model)+" "))
			}
		}
		modelLine := label + strings.Join(parts, "")
		lines = append(lines, padToWidth(modelLine, innerWidth))
	}

	// Branch name field (only in git mode)
	if m.isGitRepo {
		isSelected := isFocused && m.optField == OptFieldBranchName
		paddedLabel := fmt.Sprintf("%-12s", "Branch:")
		label := labelStyle.Render(paddedLabel)
		if isSelected {
			label = selectedLabelStyle.Render(paddedLabel)
		}

		branchValue := m.branchName
		branchStyle := valueStyle
		if branchValue == "" {
			branchValue = "auto"
			branchStyle = dimStyle
		}
		if isSelected {
			branchStyle = selectedValueStyle
		}

		availableWidth := innerWidth - lipgloss.Width(paddedLabel)
		if availableWidth < 0 {
			availableWidth = 0
		}
		if availableWidth > 0 && lipgloss.Width(branchValue) > availableWidth {
			branchValue = ansi.Truncate(branchValue, availableWidth, "")
		}

		branchLine := label + branchStyle.Render(branchValue)
		lines = append(lines, padToWidth(branchLine, innerWidth))
	}

	// Fill remaining height with empty lines
	for len(lines) < m.textareaHeight {
		lines = append(lines, strings.Repeat(" ", innerWidth))
	}

	// Apply border and padding to pre-formatted content
	panelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 2)

	return panelStyle.Render(strings.Join(lines, "\n"))
}
