// Package tui provides terminal user interface components for TAW.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/donghojung/taw/internal/constants"
	"github.com/donghojung/taw/internal/task"
)

// TaskItemStatus represents the status of a task item.
type TaskItemStatus string

const (
	TaskItemWorking TaskItemStatus = "working"
	TaskItemWaiting TaskItemStatus = "waiting"
	TaskItemDone    TaskItemStatus = "done"
	TaskItemHistory TaskItemStatus = "history" // Completed and cleaned up (from history dir)
)

// TaskItem represents a task in the list view.
type TaskItem struct {
	Name       string
	Status     TaskItemStatus
	Content    string         // Task content (from task file)
	Summary    string         // Work summary (for done/history tasks)
	UpdatedAt  time.Time      // Last state change time
	WindowID   string         // Tmux window ID (for active tasks)
	AgentDir   string         // Agent directory path
	HistoryFile string        // History file path (for history tasks)
	Task       *task.Task     // Original task (nil for history items)
}

// TaskListAction represents the action to take on a task.
type TaskListAction int

const (
	TaskListActionNone TaskListAction = iota
	TaskListActionCancel
	TaskListActionMerge
	TaskListActionPush
	TaskListActionResume
	TaskListActionSelect // Select/focus the task window
)

// TaskListUI provides an interactive task list viewer.
type TaskListUI struct {
	items         []TaskItem
	cursor        int
	width         int
	height        int
	done          bool
	action        TaskListAction
	selectedItem  *TaskItem
	agentsDir     string
	historyDir    string
	projectDir    string
	sessionName   string
	tawDir        string
	isGitRepo     bool
	previewScroll int
}

// NewTaskListUI creates a new task list UI.
func NewTaskListUI(agentsDir, historyDir, projectDir, sessionName, tawDir string, isGitRepo bool) *TaskListUI {
	return &TaskListUI{
		agentsDir:   agentsDir,
		historyDir:  historyDir,
		projectDir:  projectDir,
		sessionName: sessionName,
		tawDir:      tawDir,
		isGitRepo:   isGitRepo,
	}
}

// Init initializes the task list UI.
func (m *TaskListUI) Init() tea.Cmd {
	return m.loadTasks()
}

type tasksLoadedMsg struct {
	items []TaskItem
}

// loadTasks loads all tasks (active + history).
func (m *TaskListUI) loadTasks() tea.Cmd {
	return func() tea.Msg {
		var items []TaskItem

		// Load active tasks from agents directory
		if entries, err := os.ReadDir(m.agentsDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}

				taskName := entry.Name()
				agentDir := filepath.Join(m.agentsDir, taskName)
				item := TaskItem{
					Name:     taskName,
					AgentDir: agentDir,
				}

				// Determine status from window name or presence
				tabLockDir := filepath.Join(agentDir, constants.TabLockDirName)
				windowIDFile := filepath.Join(tabLockDir, constants.WindowIDFileName)

				if windowIDData, err := os.ReadFile(windowIDFile); err == nil {
					item.WindowID = strings.TrimSpace(string(windowIDData))
				}

				// Load task content
				taskFile := filepath.Join(agentDir, constants.TaskFileName)
				if content, err := os.ReadFile(taskFile); err == nil {
					item.Content = string(content)
				}

				// Get updated time from directory mod time
				if info, err := entry.Info(); err == nil {
					item.UpdatedAt = info.ModTime()
				}

				// Check for summary file
				summaryFile := filepath.Join(agentDir, ".summary")
				if summary, err := os.ReadFile(summaryFile); err == nil {
					item.Summary = string(summary)
				}

				// Determine status: check worktree existence or window status
				worktreeDir := filepath.Join(agentDir, "worktree")
				if _, err := os.Stat(worktreeDir); err == nil {
					// Has worktree - active task
					if item.WindowID != "" {
						// Try to determine if working or waiting from any signals
						item.Status = TaskItemWorking
					} else {
						item.Status = TaskItemWaiting
					}
				} else if item.WindowID != "" {
					item.Status = TaskItemWorking
				} else {
					item.Status = TaskItemDone
				}

				items = append(items, item)
			}
		}

		// Load completed tasks from history directory
		if entries, err := os.ReadDir(m.historyDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				fileName := entry.Name()
				historyFile := filepath.Join(m.historyDir, fileName)

				// Parse filename: YYMMDD_HHMMSS_taskname
				parts := strings.SplitN(fileName, "_", 3)
				if len(parts) < 3 {
					continue
				}

				taskName := parts[2]

				// Skip if this task still exists in agents (not yet cleaned up)
				agentDir := filepath.Join(m.agentsDir, taskName)
				if _, err := os.Stat(agentDir); err == nil {
					continue
				}

				item := TaskItem{
					Name:        taskName,
					Status:      TaskItemHistory,
					HistoryFile: historyFile,
				}

				// Parse timestamp from filename
				timestampStr := parts[0] + "_" + parts[1]
				if t, err := time.ParseInLocation("060102_150405", timestampStr, time.Local); err == nil {
					item.UpdatedAt = t
				} else if info, err := entry.Info(); err == nil {
					item.UpdatedAt = info.ModTime()
				}

				// Load history content (contains task + separator + summary)
				if content, err := os.ReadFile(historyFile); err == nil {
					contentStr := string(content)
					// Split by separator
					if idx := strings.Index(contentStr, "\n---------\n"); idx != -1 {
						item.Content = contentStr[:idx]
						item.Summary = contentStr[idx+11:] // Skip separator
					} else {
						item.Content = contentStr
					}
				}

				items = append(items, item)
			}
		}

		// Sort by UpdatedAt descending (newest first)
		sort.Slice(items, func(i, j int) bool {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		})

		return tasksLoadedMsg{items: items}
	}
}

// Update handles messages and updates the model.
func (m *TaskListUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tasksLoadedMsg:
		m.items = msg.items
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
func (m *TaskListUI) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "alt+t":
		m.done = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.previewScroll = 0
		}

	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
			m.previewScroll = 0
		}

	case "c":
		// Cancel - only for active tasks (working/waiting)
		if len(m.items) > 0 {
			item := m.items[m.cursor]
			if item.Status == TaskItemWorking || item.Status == TaskItemWaiting {
				m.action = TaskListActionCancel
				m.selectedItem = &item
				m.done = true
				return m, tea.Quit
			}
		}

	case "m":
		// Merge - only for active tasks with worktree
		if len(m.items) > 0 {
			item := m.items[m.cursor]
			if item.Status == TaskItemWorking || item.Status == TaskItemWaiting || item.Status == TaskItemDone {
				m.action = TaskListActionMerge
				m.selectedItem = &item
				m.done = true
				return m, tea.Quit
			}
		}

	case "p":
		// Push - only for active tasks
		if len(m.items) > 0 {
			item := m.items[m.cursor]
			if item.Status == TaskItemWorking || item.Status == TaskItemWaiting || item.Status == TaskItemDone {
				m.action = TaskListActionPush
				m.selectedItem = &item
				m.done = true
				return m, tea.Quit
			}
		}

	case "r":
		// Resume - only for history tasks
		if len(m.items) > 0 {
			item := m.items[m.cursor]
			if item.Status == TaskItemHistory {
				m.action = TaskListActionResume
				m.selectedItem = &item
				m.done = true
				return m, tea.Quit
			}
		}

	case "enter", " ":
		// Select - focus the task window (for active tasks)
		if len(m.items) > 0 {
			item := m.items[m.cursor]
			if item.Status == TaskItemWorking || item.Status == TaskItemWaiting {
				m.action = TaskListActionSelect
				m.selectedItem = &item
				m.done = true
				return m, tea.Quit
			}
		}

	case "pgup", "ctrl+u":
		m.previewScroll -= 10
		if m.previewScroll < 0 {
			m.previewScroll = 0
		}

	case "pgdown", "ctrl+d":
		m.previewScroll += 10
	}

	return m, nil
}

// View renders the task list UI.
func (m *TaskListUI) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	statusWorkingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("40"))

	statusWaitingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220"))

	statusDoneStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	statusHistoryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	previewStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250"))

	// Layout: left panel (task list) + right panel (preview)
	listWidth := m.width * 35 / 100 // 35% for task list
	previewWidth := m.width - listWidth - 3 // Rest for preview (minus border)

	var listBuilder strings.Builder
	var previewBuilder strings.Builder

	// Title
	listBuilder.WriteString(titleStyle.Render("Tasks"))
	listBuilder.WriteString("\n")
	listBuilder.WriteString(strings.Repeat("─", listWidth-1))
	listBuilder.WriteString("\n")

	// Task list
	listHeight := m.height - 5 // Reserve space for title and status bar
	if listHeight < 1 {
		listHeight = 1
	}

	// Calculate visible range
	start := 0
	if m.cursor >= listHeight {
		start = m.cursor - listHeight + 1
	}
	end := start + listHeight
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := start; i < end; i++ {
		item := m.items[i]

		// Cursor
		cursor := "  "
		style := normalStyle
		if i == m.cursor {
			cursor = "▸ "
			style = selectedStyle
		}

		// Status emoji
		var statusEmoji string
		var statusStyle lipgloss.Style
		switch item.Status {
		case TaskItemWorking:
			statusEmoji = "🤖"
			statusStyle = statusWorkingStyle
		case TaskItemWaiting:
			statusEmoji = "💬"
			statusStyle = statusWaitingStyle
		case TaskItemDone:
			statusEmoji = "✅"
			statusStyle = statusDoneStyle
		case TaskItemHistory:
			statusEmoji = "📁"
			statusStyle = statusHistoryStyle
		}

		// Truncate name if needed
		name := item.Name
		maxNameLen := listWidth - 6 // Account for cursor and emoji
		if len(name) > maxNameLen {
			name = name[:maxNameLen-1] + "…"
		}

		line := fmt.Sprintf("%s%s %s", cursor, statusEmoji, style.Render(name))
		listBuilder.WriteString(line)
		listBuilder.WriteString("\n")

		// Show timestamp on next line if selected
		if i == m.cursor {
			timeStr := item.UpdatedAt.Format("01/02 15:04")
			listBuilder.WriteString("    " + statusStyle.Render(timeStr) + "\n")
		}
	}

	// Pad remaining lines
	renderedLines := (end - start) * 2 // Each item takes 2 lines when selected
	for i := renderedLines; i < listHeight; i++ {
		listBuilder.WriteString("\n")
	}

	// Preview panel
	previewBuilder.WriteString(titleStyle.Render("Preview"))
	previewBuilder.WriteString("\n")
	previewBuilder.WriteString(strings.Repeat("─", previewWidth-1))
	previewBuilder.WriteString("\n")

	if len(m.items) > 0 && m.cursor < len(m.items) {
		item := m.items[m.cursor]

		// Show task content
		content := item.Content
		if content == "" {
			content = "(no content)"
		}

		// Wrap and truncate preview content
		previewHeight := m.height - 5
		previewLines := strings.Split(content, "\n")

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
			if len(line) > previewWidth-2 {
				line = line[:previewWidth-3] + "…"
			}
			previewBuilder.WriteString(previewStyle.Render(line))
			previewBuilder.WriteString("\n")
		}

		// Show summary separator and summary for done/history tasks
		if (item.Status == TaskItemHistory || item.Status == TaskItemDone) && item.Summary != "" {
			if visibleEnd == len(previewLines) {
				previewBuilder.WriteString("\n")
				previewBuilder.WriteString(dimStyle.Render("─── Summary ───"))
				previewBuilder.WriteString("\n")
				summaryLines := strings.Split(item.Summary, "\n")
				remainingHeight := previewHeight - (visibleEnd - visibleStart) - 2
				for i := 0; i < remainingHeight && i < len(summaryLines); i++ {
					line := summaryLines[i]
					if len(line) > previewWidth-2 {
						line = line[:previewWidth-3] + "…"
					}
					previewBuilder.WriteString(dimStyle.Render(line))
					previewBuilder.WriteString("\n")
				}
			}
		}
	}

	// Combine panels with border
	listContent := listBuilder.String()
	previewContent := previewBuilder.String()

	listLines := strings.Split(listContent, "\n")
	previewLines := strings.Split(previewContent, "\n")

	var combined strings.Builder
	maxLines := listHeight + 3
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
		listLinePadded := listLine + strings.Repeat(" ", listWidth-lipgloss.Width(listLine))
		combined.WriteString(listLinePadded)
		combined.WriteString(" │ ")
		combined.WriteString(previewLine)
		combined.WriteString("\n")
	}

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("240")).
		Foreground(lipgloss.Color("252"))

	var statusHints []string
	statusHints = append(statusHints, "↑↓:nav")

	if len(m.items) > 0 && m.cursor < len(m.items) {
		item := m.items[m.cursor]
		switch item.Status {
		case TaskItemWorking, TaskItemWaiting:
			statusHints = append(statusHints, "c:cancel", "m:merge", "p:push", "⏎:focus")
		case TaskItemDone:
			statusHints = append(statusHints, "m:merge", "p:push")
		case TaskItemHistory:
			statusHints = append(statusHints, "r:resume")
		}
	}
	statusHints = append(statusHints, "q:close")

	statusText := " " + strings.Join(statusHints, "  ") + " "
	if len(m.items) > 0 {
		statusText += fmt.Sprintf("  (%d/%d)", m.cursor+1, len(m.items))
	}

	padding := m.width - len(statusText)
	if padding < 0 {
		padding = 0
	}

	combined.WriteString(statusStyle.Render(statusText + strings.Repeat(" ", padding)))

	return combined.String()
}

// Result returns the action and selected item.
func (m *TaskListUI) Result() (TaskListAction, *TaskItem) {
	return m.action, m.selectedItem
}

// RunTaskListUI runs the task list UI and returns the action and selected item.
func RunTaskListUI(agentsDir, historyDir, projectDir, sessionName, tawDir string, isGitRepo bool) (TaskListAction, *TaskItem, error) {
	m := NewTaskListUI(agentsDir, historyDir, projectDir, sessionName, tawDir, isGitRepo)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return TaskListActionNone, nil, err
	}

	ui := finalModel.(*TaskListUI)
	action, item := ui.Result()
	return action, item, nil
}
