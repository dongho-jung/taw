package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/logging"
)

// FinishAction represents the selected finish action.
type FinishAction string

// Finish action options.
const (
	FinishActionCancel     FinishAction = "cancel"
	FinishActionMergePush  FinishAction = "merge-push"
	FinishActionMerge      FinishAction = "merge"
	FinishActionPR         FinishAction = "pr"
	FinishActionKeep       FinishAction = "keep"
	FinishActionDone       FinishAction = "done"
	FinishActionDrop       FinishAction = "drop"
	FinishActionCreateMain FinishAction = "create-main" // Create main branch and merge
)

// FinishOption represents an option in the finish picker.
type FinishOption struct {
	Action      FinishAction
	Name        string
	Description string
	Warning     bool // If true, requires confirmation
}

// FinishPicker is a TUI for selecting how to finish a task.
type FinishPicker struct {
	options       []FinishOption
	cursor        int
	selected      FinishAction
	confirming    bool // True when showing confirmation for drop action
	confirmCursor int  // 0=No, 1=Yes for confirmation dialog
	isDark        bool
	colors        ThemeColors

	// Style cache (reused across renders)
	styleTitle        lipgloss.Style
	styleItem         lipgloss.Style
	styleSelected     lipgloss.Style
	styleDesc         lipgloss.Style
	styleSelectedDesc lipgloss.Style
	styleWarning      lipgloss.Style
	styleHelp         lipgloss.Style
	styleDim          lipgloss.Style
	stylesCached      bool
}

// gitOptions returns the options for git mode with remote.
func gitOptions() []FinishOption {
	return []FinishOption{
		{Action: FinishActionMergePush, Name: "Merge & Push", Description: "Merge to main, push to remote, and clean up"},
		{Action: FinishActionMerge, Name: "Merge", Description: "Merge branch to main (local only) and clean up"},
		{Action: FinishActionPR, Name: "PR", Description: "Push branch and create a pull request"},
		{Action: FinishActionDrop, Name: "Drop", Description: "Discard all changes and clean up", Warning: true},
	}
}

// gitOptionsNoRemote returns the options for git mode without remote origin.
func gitOptionsNoRemote() []FinishOption {
	return []FinishOption{
		{Action: FinishActionMerge, Name: "Merge", Description: "Merge branch to main (local only) and clean up"},
		{Action: FinishActionDrop, Name: "Drop", Description: "Discard all changes and clean up", Warning: true},
	}
}

// doneOptions returns the options when there's nothing to merge (non-git or no commits).
func doneOptions() []FinishOption {
	return []FinishOption{
		{Action: FinishActionDone, Name: "Done", Description: "Clean up task"},
		{Action: FinishActionDrop, Name: "Drop", Description: "Discard all changes", Warning: true},
	}
}

// noMainOptions returns the options when main branch doesn't exist.
func noMainOptions() []FinishOption {
	return []FinishOption{
		{Action: FinishActionCreateMain, Name: "Create main & Merge", Description: "Create main branch with init commit, then merge"},
		{Action: FinishActionDrop, Name: "Drop", Description: "Discard all changes and clean up", Warning: true},
	}
}

// NewFinishPicker creates a new finish picker.
// isGitRepo: whether the project is a git repository
// hasCommits: whether there are commits to merge (only relevant if isGitRepo is true)
// hasRemote: whether the repository has a remote origin
// hasMainBranch: whether the main branch exists (only relevant if isGitRepo is true)
func NewFinishPicker(isGitRepo, hasCommits, hasRemote, hasMainBranch bool) *FinishPicker {
	logging.Debug("-> NewFinishPicker(isGitRepo=%v, hasCommits=%v, hasRemote=%v, hasMainBranch=%v)", isGitRepo, hasCommits, hasRemote, hasMainBranch)
	defer logging.Debug("<- NewFinishPicker")

	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

	var options []FinishOption
	if isGitRepo && hasCommits {
		if !hasMainBranch {
			// No main branch: offer to create it
			options = noMainOptions()
		} else if hasRemote {
			options = gitOptions()
		} else {
			options = gitOptionsNoRemote()
		}
	} else {
		// Non-git or no commits: just show Done and Drop
		options = doneOptions()
	}

	return &FinishPicker{
		options:  options,
		cursor:   0,
		selected: FinishActionCancel,
		isDark:   isDark,
		colors:   NewThemeColors(isDark),
	}
}

// Init initializes the finish picker.
func (m *FinishPicker) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update handles messages.
func (m *FinishPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		m.stylesCached = false // Invalidate style cache on theme change
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.KeyMsg:
		// Handle confirmation mode
		if m.confirming {
			switch msg.String() {
			case "y", "Y":
				// Direct yes selection
				m.selected = FinishActionDrop
				return m, tea.Quit
			case "n", "N", "esc", "ctrl+c", "ctrl+f":
				// Direct no selection or cancel
				m.confirming = false
				m.confirmCursor = 0
				return m, nil
			case "enter", " ":
				// Select based on cursor position
				if m.confirmCursor == 1 {
					m.selected = FinishActionDrop
					return m, tea.Quit
				}
				m.confirming = false
				m.confirmCursor = 0
				return m, nil
			case "up", "k":
				if m.confirmCursor > 0 {
					m.confirmCursor--
				}
				return m, nil
			case "down", "j":
				if m.confirmCursor < 1 {
					m.confirmCursor++
				}
				return m, nil
			}
			return m, nil
		}

		// Normal mode
		switch msg.String() {
		case "ctrl+c", "esc", "ctrl+f", "q":
			m.selected = FinishActionCancel
			return m, tea.Quit

		case "enter", " ":
			opt := m.options[m.cursor]
			if opt.Warning {
				// Show confirmation for warning actions
				m.confirming = true
				m.confirmCursor = 0 // Default to "No"
				return m, nil
			}
			m.selected = opt.Action
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
			return m, nil

		// Quick selection keys
		case "u", "U":
			for i, opt := range m.options {
				if opt.Action == FinishActionMergePush {
					m.cursor = i
					m.selected = opt.Action
					return m, tea.Quit
				}
			}
		case "m", "M":
			for i, opt := range m.options {
				if opt.Action == FinishActionMerge {
					m.cursor = i
					m.selected = opt.Action
					return m, tea.Quit
				}
			}
		case "p", "P":
			for i, opt := range m.options {
				if opt.Action == FinishActionPR {
					m.cursor = i
					m.selected = opt.Action
					return m, tea.Quit
				}
			}
		case "n", "N":
			for i, opt := range m.options {
				if opt.Action == FinishActionDone {
					m.cursor = i
					m.selected = opt.Action
					return m, tea.Quit
				}
			}
		case "d", "D":
			for i, opt := range m.options {
				if opt.Action == FinishActionDrop {
					m.cursor = i
					m.confirming = true
					m.confirmCursor = 0 // Default to "No"
					return m, nil
				}
			}
		}
	}

	return m, nil
}

// View renders the finish picker.
func (m *FinishPicker) View() tea.View {
	c := m.colors

	// Update style cache if needed (only on theme change)
	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(c.Accent)
		m.styleItem = lipgloss.NewStyle().
			Foreground(c.TextNormal).
			PaddingLeft(2)
		m.styleSelected = lipgloss.NewStyle().
			Foreground(c.Accent).
			Bold(true).
			PaddingLeft(0)
		m.styleDesc = lipgloss.NewStyle().
			Foreground(c.TextDim).
			PaddingLeft(4)
		m.styleSelectedDesc = lipgloss.NewStyle().
			Foreground(c.Accent).
			PaddingLeft(4)
		m.styleWarning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")). // Red
			Bold(true)
		m.styleHelp = lipgloss.NewStyle().
			Foreground(c.TextDim).
			MarginTop(1)
		m.styleDim = lipgloss.NewStyle().
			Foreground(c.TextDim)
		m.stylesCached = true
	}

	var sb strings.Builder

	// Confirmation mode
	if m.confirming {
		sb.WriteString(m.styleWarning.Render("Are you sure you want to drop all changes?"))
		sb.WriteString("\n\n")
		sb.WriteString(m.styleDim.Render("This will discard all uncommitted work."))
		sb.WriteString("\n\n")

		// Render confirmation options (No, Yes order)
		confirmOptions := []struct {
			name string
			key  string
		}{
			{"No, go back", "n"},
			{"Yes, drop", "y"},
		}
		for i, opt := range confirmOptions {
			if i == m.confirmCursor {
				sb.WriteString(m.styleSelected.Render("> " + opt.name))
			} else {
				sb.WriteString(m.styleItem.Render(opt.name))
			}
			sb.WriteString("\n")
		}

		sb.WriteString(m.styleHelp.Render("↑/↓: Navigate  Enter: Select  n/y: Quick select"))
		return tea.NewView(sb.String())
	}

	// Title
	sb.WriteString(m.styleTitle.Render("Finish Task"))
	sb.WriteString("\n\n")

	// Options
	for i, opt := range m.options {
		name := opt.Name
		if opt.Warning {
			name += " (!)"
		}

		if i == m.cursor {
			sb.WriteString(m.styleSelected.Render("> " + name))
			sb.WriteString("\n")
			sb.WriteString(m.styleSelectedDesc.Render(opt.Description))
		} else {
			sb.WriteString(m.styleItem.Render(name))
			sb.WriteString("\n")
			sb.WriteString(m.styleDesc.Render(opt.Description))
		}
		sb.WriteString("\n")
	}

	// Help
	sb.WriteString(m.styleHelp.Render("↑/↓: Navigate  Enter: Select  ⌃F/Esc: Cancel"))

	return tea.NewView(sb.String())
}

// Result returns the selected action.
func (m *FinishPicker) Result() FinishAction {
	return m.selected
}

// RunFinishPicker runs the finish picker and returns the selected action.
func RunFinishPicker(isGitRepo, hasCommits, hasRemote, hasMainBranch bool) (FinishAction, error) {
	logging.Debug("-> RunFinishPicker(isGitRepo=%v, hasCommits=%v, hasRemote=%v, hasMainBranch=%v)", isGitRepo, hasCommits, hasRemote, hasMainBranch)
	defer logging.Debug("<- RunFinishPicker")

	m := NewFinishPicker(isGitRepo, hasCommits, hasRemote, hasMainBranch)
	logging.Debug("RunFinishPicker: starting tea.Program")
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		logging.Debug("RunFinishPicker: tea.Program.Run failed: %v", err)
		return FinishActionCancel, err
	}

	fp := finalModel.(*FinishPicker)
	action := fp.Result()
	logging.Debug("RunFinishPicker: completed, action=%s", action)
	return action, nil
}
