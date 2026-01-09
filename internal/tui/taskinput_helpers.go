package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// isCancelPending returns true if we're waiting for the second ESC/Ctrl+C press.
func (m *TaskInput) isCancelPending() bool {
	return !m.cancelPressTime.IsZero()
}

// padToWidth pads a styled string to a target visible width.
// It uses lipgloss.Width() to calculate the visible width (excluding ANSI codes).
func padToWidth(s string, targetWidth int) string {
	currentWidth := lipgloss.Width(s)
	if currentWidth >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-currentWidth)
}

// embedScrollbarInTextarea embeds a scrollbar into a textarea view.
func embedScrollbarInTextarea(view string, scrollbar string, visibleLines int) string {
	if visibleLines <= 0 || scrollbar == "" {
		return view
	}

	lines := strings.Split(view, "\n")
	if len(lines) < visibleLines+2 {
		return view
	}

	scrollLines := strings.Split(scrollbar, "\n")
	if len(scrollLines) < visibleLines {
		return view
	}

	for i := 0; i < visibleLines; i++ {
		lineIdx := i + 1 // Skip top border
		line := lines[lineIdx]
		width := ansi.StringWidth(line)
		if width < 3 {
			continue
		}

		// Replace the right padding cell so the scrollbar sits inside the border.
		targetCol := width - 2
		left := ansi.Cut(line, 0, targetCol)
		right := ansi.Cut(line, targetCol+1, width)
		lines[lineIdx] = left + scrollLines[i] + right
	}

	return strings.Join(lines, "\n")
}

// Result returns the task input result.
func (m *TaskInput) Result() TaskInputResult {
	m.applyOptionInputValues()
	return TaskInputResult{
		Content:          strings.TrimSpace(m.textarea.Value()),
		Options:          m.options,
		Cancelled:        m.cancelled,
		HistoryRequested: m.historyRequested,
	}
}

// SetContent sets the textarea content (for pre-filling from history).
func (m *TaskInput) SetContent(content string) {
	m.textarea.SetValue(content)
	// Move cursor to end
	m.textarea.CursorEnd()
}

// RunTaskInput runs the task input and returns the result.
func RunTaskInput() (*TaskInputResult, error) {
	return RunTaskInputWithTasks(nil)
}

// RunTaskInputWithTasks runs the task input with active task list and returns the result.
func RunTaskInputWithTasks(activeTasks []string) (*TaskInputResult, error) {
	return RunTaskInputWithTasksAndContent(activeTasks, "")
}

// RunTaskInputWithTasksAndContent runs the task input with active task list and initial content.
func RunTaskInputWithTasksAndContent(activeTasks []string, initialContent string) (*TaskInputResult, error) {
	m := NewTaskInputWithTasks(activeTasks)
	if initialContent != "" {
		m.SetContent(initialContent)
	}
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	input := finalModel.(*TaskInput)
	result := input.Result()
	return &result, nil
}
