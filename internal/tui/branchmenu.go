package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// BranchAction represents the selected action.
type BranchAction int

// Branch action options.
const (
	BranchActionCancel BranchAction = iota
	BranchActionMerge               // ↑ Merge to main (default ← task)
	BranchActionSync                // ↓ Sync from main (default → task)
)

// BranchMenu is a simple menu for branch operations.
type BranchMenu struct {
	action BranchAction
	isDark bool
	colors ThemeColors

	// Style cache (reused across renders)
	styleTitle   lipgloss.Style
	styleItem    lipgloss.Style
	styleKey     lipgloss.Style
	styleDim     lipgloss.Style
	stylesCached bool
}

// NewBranchMenu creates a new branch menu.
func NewBranchMenu() *BranchMenu {
	isDark := DetectDarkMode()
	return &BranchMenu{
		isDark: isDark,
		colors: NewThemeColors(isDark),
	}
}

// Init initializes the menu.
func (m *BranchMenu) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update handles key events.
func (m *BranchMenu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		m.stylesCached = false // Invalidate style cache on theme change
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.action = BranchActionMerge
			return m, tea.Quit
		case "down", "j":
			m.action = BranchActionSync
			return m, tea.Quit
		default:
			// Any other key cancels
			m.action = BranchActionCancel
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the menu.
func (m *BranchMenu) View() tea.View {
	c := m.colors

	// Update style cache if needed (only on theme change)
	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().Bold(true).Foreground(c.Accent)
		m.styleItem = lipgloss.NewStyle().Foreground(c.TextNormal)
		m.styleKey = lipgloss.NewStyle().Foreground(c.WarningColor)
		m.styleDim = lipgloss.NewStyle().Foreground(c.TextDim)
		m.stylesCached = true
	}

	content := fmt.Sprintf(
		"%s\n\n  %s  %s\n  %s  %s\n\n%s",
		m.styleTitle.Render("Branch Actions"),
		m.styleKey.Render("↑"),
		m.styleItem.Render("Merge to main (default ← task)"),
		m.styleKey.Render("↓"),
		m.styleItem.Render("Sync from main (default → task)"),
		m.styleDim.Render("Press any other key to cancel"),
	)

	return tea.NewView(content)
}

// Action returns the selected action.
func (m *BranchMenu) Action() BranchAction {
	return m.action
}

// RunBranchMenu runs the branch menu and returns the selected action.
func RunBranchMenu() (BranchAction, error) {
	m := NewBranchMenu()
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return BranchActionCancel, err
	}

	menu := finalModel.(*BranchMenu)
	return menu.Action(), nil
}
