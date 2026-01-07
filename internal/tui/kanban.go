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

	// Cached task data (refreshed on tick, not on every render)
	working  []*service.DiscoveredTask
	waiting  []*service.DiscoveredTask
	done     []*service.DiscoveredTask
	warning  []*service.DiscoveredTask
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

// Refresh updates the cached task data by discovering all tasks.
// This should be called periodically (e.g., on tick) rather than on every render.
func (k *KanbanView) Refresh() {
	k.working, k.waiting, k.done, k.warning = k.service.DiscoverAll()
}

// Render renders the Kanban board using cached task data.
// Call Refresh() first to update the cache.
func (k *KanbanView) Render() string {
	// Use cached task data (populated by Refresh())
	working, waiting, done, warning := k.working, k.waiting, k.done, k.warning

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

	actionStyle := lipgloss.NewStyle().
		Foreground(dimColor).
		Italic(true)

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

		// Tasks (limited by height)
		// Each task shows: project/name (line 1), current action if any (line 2)
		linesUsed := 2 // header + separator
		for _, task := range col.tasks {
			if linesUsed >= maxHeight {
				break
			}

			// Full task display name: session/taskName
			fullName := task.Session + "/" + task.Name
			displayName := fullName
			if len(displayName) > columnWidth-6 {
				displayName = displayName[:columnWidth-7] + "…"
			}
			content.WriteString(taskNameStyle.Render(displayName))
			content.WriteString("\n")
			linesUsed++

			// Show current action if available and space permits
			if task.CurrentAction != "" && linesUsed < maxHeight {
				action := task.CurrentAction
				// Leave room for indent and truncation
				maxActionLen := columnWidth - 8
				if maxActionLen > 0 {
					if len(action) > maxActionLen {
						action = action[:maxActionLen-1] + "…"
					}
					content.WriteString("  " + actionStyle.Render(action))
					content.WriteString("\n")
					linesUsed++
				}
			}
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

// HasTasks returns true if there are any cached tasks to display.
func (k *KanbanView) HasTasks() bool {
	return len(k.working)+len(k.waiting)+len(k.done)+len(k.warning) > 0
}

// TaskCount returns the total number of cached tasks.
func (k *KanbanView) TaskCount() int {
	return len(k.working) + len(k.waiting) + len(k.done) + len(k.warning)
}
