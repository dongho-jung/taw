package tui

import (
	"strings"

	rw "github.com/mattn/go-runewidth"
)

type textInputRender struct {
	Text    string
	CursorX int
}

func syncTextInputOffset(value []rune, pos, width int, offset, offsetRight *int) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(value) {
		pos = len(value)
	}

	if width <= 0 || rw.StringWidth(string(value)) <= width {
		*offset = 0
		*offsetRight = len(value)
		return
	}

	if *offsetRight > len(value) {
		*offsetRight = len(value)
	}
	if *offset < 0 {
		*offset = 0
	}

	if pos < *offset {
		*offset = pos
		w := 0
		i := 0
		runes := value[*offset:]
		for i < len(runes) && w <= width {
			w += rw.RuneWidth(runes[i])
			if w <= width+1 {
				i++
			}
		}
		*offsetRight = *offset + i
		return
	}

	if pos >= *offsetRight {
		*offsetRight = pos
		w := 0
		runes := value[:*offsetRight]
		i := len(runes) - 1
		for i > 0 && w < width {
			w += rw.RuneWidth(runes[i])
			if w <= width {
				i--
			}
		}
		*offset = *offsetRight - (len(runes) - 1 - i)
	}
}

func renderTextInput(value string, pos, width int, placeholder string, offset, offsetRight int) textInputRender {
	runes := []rune(value)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	if len(runes) == 0 {
		text := trimToWidth(placeholder, width)
		return textInputRender{
			Text:    padTextInputWidth(text, width),
			CursorX: 0,
		}
	}

	if offset < 0 {
		offset = 0
	}
	if offset > len(runes) {
		offset = len(runes)
	}
	if offsetRight < offset {
		offsetRight = offset
	}
	if offsetRight > len(runes) {
		offsetRight = len(runes)
	}

	visible := runes[offset:offsetRight]
	cursorPos := pos - offset
	if cursorPos < 0 {
		cursorPos = 0
	}
	if cursorPos > len(visible) {
		cursorPos = len(visible)
	}

	text := padTextInputWidth(string(visible), width)
	cursorX := rw.StringWidth(string(visible[:cursorPos]))
	if width > 0 && cursorX > width {
		cursorX = width
	}

	return textInputRender{
		Text:    text,
		CursorX: cursorX,
	}
}

func trimToWidth(s string, width int) string {
	if width <= 0 || rw.StringWidth(s) <= width {
		return s
	}

	runes := []rune(s)
	w := 0
	for i, r := range runes {
		w += rw.RuneWidth(r)
		if w > width {
			return string(runes[:i])
		}
	}
	return s
}

func padTextInputWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	padding := width - rw.StringWidth(s)
	if padding <= 0 {
		return s
	}
	return s + strings.Repeat(" ", padding)
}
