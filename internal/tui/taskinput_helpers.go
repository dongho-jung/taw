package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dongho-jung/paw/internal/constants"
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
	return s + getPadding(targetWidth-currentWidth)
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
		Content:    strings.TrimSpace(m.textarea.Value()),
		Options:    m.options,
		Cancelled:  m.cancelled,
		JumpTarget: m.jumpTarget,
	}
}

// SetContent sets the textarea content (for pre-filling from history).
func (m *TaskInput) SetContent(content string) {
	m.textarea.SetValue(content)
	// Move cursor to end
	m.textarea.CursorEnd()
}

// checkHistorySelection checks for a history selection file from Ctrl+R picker.
// If found, it replaces the current content with the selected history item and deletes the file.
func (m *TaskInput) checkHistorySelection() {
	pawDir := m.pawDirPath()
	if pawDir == "" {
		return
	}

	selectionPath := filepath.Join(pawDir, constants.HistorySelectionFile)
	data, err := os.ReadFile(selectionPath)
	if err != nil {
		// File doesn't exist or can't be read - this is normal (no pending selection)
		return
	}

	// Got a selection - replace content and delete the file
	content := string(data)
	if content != "" {
		m.textarea.SetValue(content)
		m.textarea.CursorEnd()
		m.updateTextareaHeight()
		m.persistTemplateDraft()
	}

	// Delete the file to prevent re-loading on next update
	_ = os.Remove(selectionPath)
}

// RunTaskInputWithOptions runs the task input with active task list, git mode flag, and optional initial content.
func RunTaskInputWithOptions(activeTasks []string, isGitRepo bool, initialContent string) (*TaskInputResult, error) {
	m := NewTaskInputWithOptions(activeTasks, isGitRepo)
	if initialContent != "" {
		m.SetContent(initialContent)
	}
	// Note: Cmd+C support depends on terminal supporting Kitty keyboard protocol
	// (e.g., Kitty, WezTerm, iTerm2 with protocol enabled)
	// Ctrl+C works in all terminals
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	input := finalModel.(*TaskInput)
	result := input.Result()
	return &result, nil
}
