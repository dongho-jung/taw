// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/service"
)

// KanbanView renders a Kanban-style task board.
type KanbanView struct {
	width    int
	height   int
	isDark   bool
	service  *service.TaskDiscoveryService
}

// NewKanbanView creates a new Kanban view.
func NewKanbanView(isDark bool) *KanbanView {
	return &KanbanView{
		isDark:  isDark,
		service: service.NewTaskDiscoveryService(),
	}
}

// SetSize sets the view dimensions.
func (k *KanbanView) SetSize(width, height int) {
	k.width = width
	k.height = height
}

// Render renders the Kanban board.
func (k *KanbanView) Render() string {
	// Discover all tasks
	working, waiting, done, warning := k.service.DiscoverAll()

	// Styles (adaptive to light/dark mode)
	lightDark := lipgloss.LightDark(k.isDark)
	normalColor := lightDark(lipgloss.Color("236"), lipgloss.Color("252"))
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(normalColor)

	taskNameStyle := lipgloss.NewStyle().
		Foreground(normalColor)

	borderColor := dimColor
	panelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	// Calculate column width (4 columns with gaps)
	// Minimum width per column
	if k.width < 40 {
		return ""
	}
	columnWidth := (k.width - 6) / 4 // -6 for borders and gaps
	if columnWidth < 15 {
		columnWidth = 15
	}

	// Build each column
	columns := []struct {
		emoji  string
		title  string
		tasks  []*service.DiscoveredTask
		color  string
	}{
		{constants.EmojiWorking, "Working", working, "40"},   // Green
		{constants.EmojiWaiting, "Waiting", waiting, "220"},  // Yellow
		{constants.EmojiDone, "Done", done, "245"},           // Gray
		{constants.EmojiWarning, "Warning", warning, "203"},  // Red
	}

	var columnViews []string
	maxHeight := k.height - 4 // Reserve space for title and border

	for _, col := range columns {
		var content strings.Builder

		// Column header
		colHeaderStyle := headerStyle.Foreground(lipgloss.Color(col.color))
		header := fmt.Sprintf("%s %s (%d)", col.emoji, col.title, len(col.tasks))
		content.WriteString(colHeaderStyle.Render(header))
		content.WriteString("\n")
		content.WriteString(strings.Repeat("─", columnWidth-4))
		content.WriteString("\n")

		// Tasks (limited by height) - show only task name
		linesUsed := 2 // header + separator
		for _, task := range col.tasks {
			if linesUsed >= maxHeight {
				break
			}

			// Task name (truncated)
			name := task.Name
			if len(name) > columnWidth-6 {
				name = name[:columnWidth-7] + "…"
			}
			content.WriteString(taskNameStyle.Render(name))
			content.WriteString("\n")
			linesUsed++
		}

		// Pad to consistent height
		for linesUsed < maxHeight {
			content.WriteString("\n")
			linesUsed++
		}

		colStyle := panelStyle.Width(columnWidth)
		columnViews = append(columnViews, colStyle.Render(content.String()))
	}

	// Combine columns horizontally
	board := lipgloss.JoinHorizontal(lipgloss.Top, columnViews...)

	// Add title
	var result strings.Builder
	result.WriteString(titleStyle.Render("Tasks"))
	result.WriteString("\n")
	result.WriteString(board)

	return result.String()
}

// HasTasks returns true if there are any tasks to display.
func (k *KanbanView) HasTasks() bool {
	working, waiting, done, warning := k.service.DiscoverAll()
	return len(working)+len(waiting)+len(done)+len(warning) > 0
}

// TaskCount returns the total number of tasks.
func (k *KanbanView) TaskCount() int {
	working, waiting, done, warning := k.service.DiscoverAll()
	return len(working) + len(waiting) + len(done) + len(warning)
}
