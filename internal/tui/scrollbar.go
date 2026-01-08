package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
)

func renderVerticalScrollbar(contentHeight, visibleHeight, scrollOffset int, isDark bool) string {
	if contentHeight <= visibleHeight || visibleHeight <= 0 {
		return ""
	}

	lightDark := lipgloss.LightDark(isDark)
	trackColor := lightDark(lipgloss.Color("250"), lipgloss.Color("238"))
	thumbColor := lightDark(lipgloss.Color("245"), lipgloss.Color("245"))

	trackStyle := lipgloss.NewStyle().Foreground(trackColor)
	thumbStyle := lipgloss.NewStyle().Foreground(thumbColor)

	thumbSize := max(1, visibleHeight*visibleHeight/contentHeight)
	maxOffset := contentHeight - visibleHeight
	thumbPosition := 0
	if maxOffset > 0 {
		thumbPosition = scrollOffset * (visibleHeight - thumbSize) / maxOffset
	}
	thumbPosition = max(0, min(thumbPosition, visibleHeight-thumbSize))

	var sb strings.Builder
	for i := 0; i < visibleHeight; i++ {
		if i >= thumbPosition && i < thumbPosition+thumbSize {
			sb.WriteString(thumbStyle.Render("┃"))
		} else {
			sb.WriteString(trackStyle.Render("│"))
		}
		if i < visibleHeight-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
