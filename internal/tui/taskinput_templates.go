package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
)

const (
	templatePlaceholderInput = "???"
	templatePlaceholderToken = "___"
)

// Pre-computed rune slice for template placeholder token (avoids repeated conversions)
var templatePlaceholderTokenRunes = []rune(templatePlaceholderToken)

func (m *TaskInput) pawDirPath() string {
	if pawDir := os.Getenv("PAW_DIR"); pawDir != "" {
		info, err := os.Stat(pawDir)
		if err == nil && info.IsDir() {
			if m.pawDir != pawDir {
				if m.pawDir != "" {
					logging.Debug("pawDirPath: overriding cached pawDir (old=%s new=%s)", m.pawDir, pawDir)
				} else {
					logging.Debug("pawDirPath: using PAW_DIR=%s", pawDir)
				}
				m.pawDir = pawDir
			}
			return m.pawDir
		}
		logging.Debug("pawDirPath: PAW_DIR invalid (path=%s, err=%v)", pawDir, err)
	}
	if m.pawDir != "" {
		return m.pawDir
	}
	m.pawDir = findPawDir()
	if m.pawDir == "" {
		logging.Debug("pawDirPath: no .paw directory found")
	} else {
		logging.Debug("pawDirPath: found .paw at %s", m.pawDir)
	}
	return m.pawDir
}

func (m *TaskInput) persistTemplateDraft() {
	pawDir := m.pawDirPath()
	if pawDir == "" {
		logging.Debug("persistTemplateDraft: pawDir empty")
		return
	}

	content := m.textarea.Value()
	if content == m.lastTemplateDraft {
		return
	}

	draftPath := filepath.Join(pawDir, constants.TemplateDraftFile)
	if err := os.WriteFile(draftPath, []byte(content), 0644); err != nil {
		logging.Warn("persistTemplateDraft: failed to write draft (path=%s, err=%v)", draftPath, err)
		return
	}
	m.lastTemplateDraft = content
	logging.Debug("persistTemplateDraft: wrote draft (path=%s, bytes=%d)", draftPath, len(content))
}

func (m *TaskInput) checkTemplateSelection() bool {
	pawDir := m.pawDirPath()
	if pawDir == "" {
		logging.Debug("checkTemplateSelection: pawDir empty")
		return false
	}

	selectionPath := filepath.Join(pawDir, constants.TemplateSelectionFile)
	data, err := os.ReadFile(selectionPath)
	if err != nil {
		return false
	}
	logging.Debug("checkTemplateSelection: loaded selection (path=%s, bytes=%d)", selectionPath, len(data))

	content := string(data)
	applied := false
	if content != "" {
		content = strings.ReplaceAll(content, templatePlaceholderInput, templatePlaceholderToken)
		m.textarea.SetValue(content)
		m.templateTipUntil = time.Now().Add(templateTipDuration)
		if !m.jumpToFirstTemplatePlaceholder() {
			m.textarea.CursorEnd()
			m.updateTextareaHeight()
			m.persistTemplateDraft()
		}
		applied = true
	}

	_ = os.Remove(selectionPath)
	return applied
}

func (m *TaskInput) jumpToNextTemplatePlaceholder() bool {
	content := m.textarea.Value()
	if !strings.Contains(content, templatePlaceholderToken) {
		return false
	}

	lines := strings.Split(content, "\n")
	row, col := m.textarea.CursorPosition()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
		if row < 0 {
			return false
		}
		col = len(lines[row])
	}

	targetRow, targetCol, ok := findNextPlaceholder(lines, row, col)
	if !ok {
		targetRow, targetCol, ok = findNextPlaceholder(lines, 0, 0)
	}
	if !ok {
		return false
	}

	line := lines[targetRow]
	lineRunes := []rune(line)
	tokenLen := len(templatePlaceholderTokenRunes)
	if targetCol+tokenLen > len(lineRunes) {
		return false
	}

	lines[targetRow] = string(append(lineRunes[:targetCol], lineRunes[targetCol+tokenLen:]...))
	m.textarea.SetValue(strings.Join(lines, "\n"))
	m.moveCursorTo(targetRow, targetCol)
	m.updateTextareaHeight()
	m.persistTemplateDraft()
	return true
}

func (m *TaskInput) jumpToFirstTemplatePlaceholder() bool {
	content := m.textarea.Value()
	if !strings.Contains(content, templatePlaceholderToken) {
		return false
	}

	lines := strings.Split(content, "\n")
	targetRow, targetCol, ok := findNextPlaceholder(lines, 0, 0)
	if !ok {
		return false
	}

	line := lines[targetRow]
	lineRunes := []rune(line)
	tokenLen := len(templatePlaceholderTokenRunes)
	if targetCol+tokenLen > len(lineRunes) {
		return false
	}

	lines[targetRow] = string(append(lineRunes[:targetCol], lineRunes[targetCol+tokenLen:]...))
	m.textarea.SetValue(strings.Join(lines, "\n"))
	m.moveCursorTo(targetRow, targetCol)
	m.updateTextareaHeight()
	m.persistTemplateDraft()
	return true
}

func (m *TaskInput) moveCursorTo(row, col int) {
	currentRow, _ := m.textarea.CursorPosition()
	if row < 0 {
		row = 0
	}

	for currentRow < row {
		m.textarea.CursorDown()
		currentRow++
	}
	for currentRow > row {
		m.textarea.CursorUp()
		currentRow--
	}
	m.textarea.SetCursorColumn(col)
}

func findNextPlaceholder(lines []string, startRow, startCol int) (int, int, bool) {
	if startRow < 0 {
		startRow = 0
	}
	if startRow >= len(lines) {
		return 0, 0, false
	}

	for row := startRow; row < len(lines); row++ {
		lineRunes := []rune(lines[row])
		colStart := 0
		if row == startRow {
			colStart = clampInt(startCol, 0, len(lineRunes))
		}

		if col, ok := findPlaceholderInRunes(lineRunes, colStart); ok {
			return row, col, true
		}
	}
	return 0, 0, false
}

func findPlaceholderInRunes(line []rune, startCol int) (int, bool) {
	if startCol < 0 {
		startCol = 0
	}
	if startCol > len(line) {
		startCol = len(line)
	}

	token := templatePlaceholderTokenRunes
	tokenLen := len(token)
	if tokenLen == 0 || len(line) < tokenLen {
		return 0, false
	}

	for idx := 0; idx <= len(line)-tokenLen; idx++ {
		match := true
		for j := 0; j < tokenLen; j++ {
			if line[idx+j] != token[j] {
				match = false
				break
			}
		}
		if !match {
			continue
		}

		pos := idx
		if pos >= startCol || (startCol > pos && startCol < pos+tokenLen) {
			return pos, true
		}
	}
	return 0, false
}

func clampInt(v, minVal, maxVal int) int {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}
