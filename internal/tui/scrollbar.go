package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
)

// scrollbarStyles caches scrollbar styles to avoid repeated allocations.
// Key is isDark (0=light, 1=dark).
var scrollbarStyles [2]struct {
	track lipgloss.Style
	thumb lipgloss.Style
	init  bool
}

// getScrollbarStyles returns cached styles for the given theme.
func getScrollbarStyles(isDark bool) (track, thumb lipgloss.Style) {
	idx := 0
	if isDark {
		idx = 1
	}

	if !scrollbarStyles[idx].init {
		lightDark := lipgloss.LightDark(isDark)
		trackColor := lightDark(lipgloss.Color("250"), lipgloss.Color("238"))
		thumbColor := lightDark(lipgloss.Color("245"), lipgloss.Color("245"))

		scrollbarStyles[idx].track = lipgloss.NewStyle().Foreground(trackColor)
		scrollbarStyles[idx].thumb = lipgloss.NewStyle().Foreground(thumbColor)
		scrollbarStyles[idx].init = true
	}

	return scrollbarStyles[idx].track, scrollbarStyles[idx].thumb
}

func renderVerticalScrollbar(contentHeight, visibleHeight, scrollOffset int, isDark bool) string {
	if contentHeight <= visibleHeight || visibleHeight <= 0 {
		return ""
	}

	trackStyle, thumbStyle := getScrollbarStyles(isDark)

	thumbSize := max(1, visibleHeight*visibleHeight/contentHeight)
	maxOffset := contentHeight - visibleHeight
	thumbPosition := 0
	if maxOffset > 0 {
		thumbPosition = scrollOffset * (visibleHeight - thumbSize) / maxOffset
	}
	thumbPosition = max(0, min(thumbPosition, visibleHeight-thumbSize))

	var sb strings.Builder
	sb.Grow(visibleHeight * 20) // Pre-allocate for styled characters + newlines
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
