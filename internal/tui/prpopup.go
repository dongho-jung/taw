// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type linkBoxBounds struct {
	top    int
	bottom int
	left   int
	right  int
}

// PRPopup provides a popup UI for PR actions.
type PRPopup struct {
	url        string
	displayURL string

	cursor int // 0 = No, 1 = Yes
	open   bool

	copied bool

	width  int
	height int
	isDark bool
	colors ThemeColors

	linkBox linkBoxBounds

	// Style cache (reused across renders)
	styleTitle    lipgloss.Style
	styleLabel    lipgloss.Style
	styleBox      lipgloss.Style
	styleChoice   lipgloss.Style
	styleSelected lipgloss.Style
	styleHelp     lipgloss.Style
	stylesCached  bool
}

// NewPRPopup creates a new PR popup model.
func NewPRPopup(url string) *PRPopup {
	isDark := DetectDarkMode()
	return &PRPopup{
		url:    url,
		cursor: 0,
		isDark: isDark,
		colors: NewThemeColors(isDark),
		linkBox: linkBoxBounds{
			top:    -1,
			bottom: -1,
			left:   -1,
			right:  -1,
		},
	}
}

// Init initializes the popup.
func (m *PRPopup) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update handles messages.
func (m *PRPopup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		m.stylesCached = false // Invalidate style cache on theme change
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			if m.inLinkBox(msg.X, msg.Y) {
				_ = clipboard.WriteAll(m.url)
				m.copied = true
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h", "up", "k":
			m.cursor = 0
			return m, nil
		case "right", "l", "down", "j":
			m.cursor = 1
			return m, nil
		case "y", "Y":
			m.cursor = 1
			m.open = true
			return m, tea.Quit
		case "n", "N":
			m.cursor = 0
			m.open = false
			return m, tea.Quit
		case "enter", " ":
			m.open = m.cursor == 1
			return m, tea.Quit
		case "esc", "ctrl+c", "q":
			m.open = false
			return m, tea.Quit
		}
	}

	return m, nil
}

// View renders the popup.
func (m *PRPopup) View() tea.View {
	c := m.colors

	maxURLWidth := 70
	if m.width > 0 {
		maxURLWidth = min(maxURLWidth, m.width-10)
	}
	if maxURLWidth < 10 {
		maxURLWidth = 10
	}
	m.displayURL = truncateWithEllipsis(m.url, maxURLWidth)

	// Update style cache if needed (only on theme change)
	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(c.Accent)
		m.styleLabel = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(c.Border).
			Padding(0, 1)
		m.styleChoice = lipgloss.NewStyle().
			Foreground(c.TextNormal)
		m.styleSelected = lipgloss.NewStyle().
			Foreground(c.TextInverted).
			Background(c.Accent).
			Bold(true)
		m.styleHelp = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.stylesCached = true
	}

	var noLabel, yesLabel string
	if m.cursor == 0 {
		noLabel = m.styleSelected.Render("No")
		yesLabel = m.styleChoice.Render("Yes")
	} else {
		noLabel = m.styleChoice.Render("No")
		yesLabel = m.styleSelected.Render("Yes")
	}

	copyHint := "Click link to copy"
	if m.copied {
		copyHint = "Copied to clipboard"
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styleTitle.Render("PR Created"),
		"",
		m.styleLabel.Render("Link"),
		m.styleBox.Render(m.displayURL),
		m.styleHelp.Render(copyHint),
		"",
		m.styleLabel.Render("Open in browser?  ")+noLabel+"  "+yesLabel,
		m.styleHelp.Render("Left/Right: Select  Enter: Confirm  Esc: Cancel"),
	)

	view := content
	if m.width > 0 && m.height > 0 {
		view = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	m.updateLinkBoxBounds(view)
	return tea.NewView(view)
}

// Result returns whether the user chose to open the PR in the browser.
func (m *PRPopup) Result() bool {
	return m.open
}

// RunPRPopup runs the PR popup UI and returns whether to open the PR in browser.
func RunPRPopup(url string) (bool, error) {
	m := NewPRPopup(url)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}

	popup := finalModel.(*PRPopup)
	return popup.Result(), nil
}

func (m *PRPopup) inLinkBox(x, y int) bool {
	if m.linkBox.top < 0 {
		return false
	}
	if y < m.linkBox.top || y > m.linkBox.bottom {
		return false
	}
	if x < m.linkBox.left || x > m.linkBox.right {
		return false
	}
	return true
}

func (m *PRPopup) updateLinkBoxBounds(view string) {
	m.linkBox = linkBoxBounds{top: -1, bottom: -1, left: -1, right: -1}
	if m.displayURL == "" {
		return
	}

	lines := strings.Split(ansi.Strip(view), "\n")
	for i, line := range lines {
		if !strings.Contains(line, m.displayURL) {
			continue
		}
		top := i - 1
		bottom := i + 1
		if top < 0 || bottom >= len(lines) {
			return
		}

		left := firstRuneIndex(line, '│')
		right := lastRuneIndex(line, '│')
		if left == -1 || right == -1 || right <= left {
			return
		}

		m.linkBox = linkBoxBounds{
			top:    top,
			bottom: bottom,
			left:   left,
			right:  right,
		}
		return
	}
}

func firstRuneIndex(line string, target rune) int {
	runes := []rune(line)
	for i, r := range runes {
		if r == target {
			return i
		}
	}
	return -1
}

func lastRuneIndex(line string, target rune) int {
	runes := []rune(line)
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == target {
			return i
		}
	}
	return -1
}
