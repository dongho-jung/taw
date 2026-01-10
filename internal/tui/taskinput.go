// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/tui/textarea"
)

// FocusPanel represents which panel is currently focused.
type FocusPanel int

const (
	FocusPanelLeft   FocusPanel = iota // Task input textarea
	FocusPanelRight                    // Options panel
	FocusPanelKanban                   // Kanban view
)

// OptField represents which option field is currently selected.
type OptField int

const (
	OptFieldModel OptField = iota
	OptFieldUltrathink
)

const optFieldCount = 2

// cancelDoublePressTimeout is the time window for double-press cancel detection.
const cancelDoublePressTimeout = 2 * time.Second

// Textarea height constants
const (
	textareaMinHeight     = 5  // Minimum textarea height in lines
	textareaDefaultHeight = 5  // Default starting height (will expand as needed)
	textareaMaxHeightPct  = 50 // Maximum height as percentage of screen height
)

// TaskInput provides an inline text input for task content.
type TaskInput struct {
	textarea    textarea.Model
	submitted   bool
	cancelled   bool
	width       int
	height      int
	options     *config.TaskOptions
	activeTasks []string // Active task names for dependency selection
	theme       config.Theme
	isDark      bool // Cached dark mode detection (must be detected before bubbletea starts)

	// Dynamic textarea height
	textareaHeight    int // Current textarea height (visible lines)
	textareaMaxHeight int // Maximum textarea height (50% of screen)

	// Dynamic panel widths for alignment with Kanban columns
	optionsPanelWidth int // Options panel display width (dynamic for alignment)

	// Inline options editing
	focusPanel FocusPanel
	optField   OptField
	modelIdx   int

	mouseSelecting  bool
	selectAnchorRow int
	selectAnchorCol int

	// Kanban mouse selection (column-aware)
	kanbanSelecting bool
	kanbanSelectCol int // Column being selected (0-3)
	kanbanSelectX   int // X position relative to column
	kanbanSelectY   int // Y position relative to kanban area

	// Kanban view for tasks across all sessions
	kanban *KanbanView

	// Double-press cancel detection
	cancelPressTime time.Time
	cancelKey       string // Track which key was pressed for cancel ("esc" or "ctrl+c")

	// Tip caching - only changes every minute
	currentTip     string
	lastTipRefresh time.Time
}

// tickMsg is used for periodic Kanban refresh.
type tickMsg time.Time

// cancelClearMsg is used to clear the cancel pending state after timeout.
type cancelClearMsg struct{}

// TaskInputResult contains the result of the task input.
type TaskInputResult struct {
	Content   string
	Options   *config.TaskOptions
	Cancelled bool
}

// NewTaskInput creates a new task input model.
func NewTaskInput() *TaskInput {
	return NewTaskInputWithTasks(nil)
}

// NewTaskInputWithTasks creates a new task input model with active task list.
func NewTaskInputWithTasks(activeTasks []string) *TaskInput {
	// Detect dark mode BEFORE bubbletea starts
	// Uses config theme setting if available, otherwise auto-detects
	theme := loadThemeFromConfig()
	isDark := detectDarkMode(theme)

	ta := textarea.New()
	ta.Placeholder = "Describe your task here... and press Alt+Enter\n\nExamples:\n- Add user authentication\n- Fix bug in login form"
	ta.Focus()
	ta.CharLimit = 0 // No limit
	ta.ShowLineNumbers = false
	ta.Prompt = "" // Clear prompt to avoid extra characters on the left
	// MaxHeight will be set dynamically based on screen size (50% of screen)
	// Start with a reasonable default that will be updated on WindowSizeMsg
	ta.MaxHeight = 99
	ta.SetWidth(80)
	ta.SetHeight(textareaDefaultHeight)

	// Enable real cursor for proper IME support (Korean input)
	ta.VirtualCursor = false

	// Custom styling using v2 API - assign directly to Styles field
	applyTaskInputTextareaTheme(&ta, isDark)

	opts := config.DefaultTaskOptions()

	// Find model index
	modelIdx := 0
	for i, m := range config.ValidModels() {
		if m == opts.Model {
			modelIdx = i
			break
		}
	}

	return &TaskInput{
		textarea:          ta,
		width:             80,
		height:            15,
		options:           opts,
		activeTasks:       activeTasks,
		theme:             theme,
		isDark:            isDark,
		textareaHeight:    textareaDefaultHeight,
		textareaMaxHeight: 15, // Will be updated on WindowSizeMsg
		optionsPanelWidth: 43, // Default, will be updated on WindowSizeMsg for alignment
		focusPanel:        FocusPanelLeft,
		optField:          OptFieldModel,
		modelIdx:          modelIdx,
		kanban:            NewKanbanView(isDark),
		currentTip:        GetTip(),
		lastTipRefresh:    time.Now(),
	}
}

func applyTaskInputTextareaTheme(ta *textarea.Model, isDark bool) {
	ta.Styles = textarea.DefaultStyles(isDark)
	// Accent color: darker blue for light bg (good contrast), bright cyan for dark bg
	lightDark := lipgloss.LightDark(isDark)
	accentColor := lightDark(lipgloss.Color("25"), lipgloss.Color("39"))
	dimColor := lightDark(lipgloss.Color("250"), lipgloss.Color("240"))
	ta.Styles.Focused.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(0, 1)
	ta.Styles.Blurred.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(dimColor).
		Padding(0, 1)
	// Keep text and placeholder fully readable when blurred (border color already indicates focus)
	// Copy all text-related styles from focused to blurred to prevent dimming
	ta.Styles.Blurred.Text = ta.Styles.Focused.Text
	ta.Styles.Blurred.Placeholder = ta.Styles.Focused.Placeholder
	ta.Styles.Blurred.CursorLine = ta.Styles.Focused.CursorLine
	ta.Styles.Focused.CursorLine = lipgloss.NewStyle()
	ta.Styles.Blurred.CursorLine = lipgloss.NewStyle()
	ta.Styles.Focused.Prompt = lipgloss.NewStyle()
	ta.Styles.Blurred.Prompt = lipgloss.NewStyle()
}

func (m *TaskInput) applyTheme(isDark bool) {
	if m.isDark == isDark {
		return
	}
	m.isDark = isDark
	m.kanban.SetDarkMode(isDark)
	applyTaskInputTextareaTheme(&m.textarea, isDark)
}

// Init initializes the task input.
func (m *TaskInput) Init() tea.Cmd {
	// Refresh Kanban data on init
	m.kanban.Refresh()
	cmds := []tea.Cmd{textarea.Blink, m.tickCmd()}
	if m.theme == config.ThemeAuto {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

// tickCmd returns a command that triggers a tick after 5 seconds.
func (m *TaskInput) tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// updateTextareaHeight calculates and sets the appropriate textarea height based on content.
// The height expands automatically as content grows, up to textareaMaxHeight (50% of screen).
func (m *TaskInput) updateTextareaHeight() {
	// Count content lines (newlines + 1)
	content := m.textarea.Value()
	contentLines := strings.Count(content, "\n") + 1

	// Calculate required height: content lines, but within min/max bounds
	requiredHeight := contentLines
	if requiredHeight < textareaMinHeight {
		requiredHeight = textareaMinHeight
	}
	if requiredHeight > m.textareaMaxHeight {
		requiredHeight = m.textareaMaxHeight
	}

	// Only update if height changed
	if requiredHeight != m.textareaHeight {
		m.textareaHeight = requiredHeight
		m.textarea.SetHeight(requiredHeight)
	}

	// Always adjust viewport position based on content vs visible height
	// This prevents the "first line cut off" issue when expanding height
	if contentLines <= m.textareaHeight {
		// Content fits in viewport - always show from top (no scrolling needed)
		m.textarea.GotoTop()
	} else {
		// Content exceeds viewport - ensure cursor is visible with proper scrolling
		// This allows scrolling but prevents last line from reaching the top
		m.textarea.EnsureCursorVisible()
	}
}

// Update handles messages and updates the model.
func (m *TaskInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		// Refresh Kanban data on tick (expensive I/O is done here, not in View)
		m.kanban.Refresh()
		// Refresh tip every minute
		if time.Since(m.lastTipRefresh) >= time.Minute {
			m.currentTip = GetTip()
			m.lastTipRefresh = time.Now()
		}
		// Schedule next tick
		return m, m.tickCmd()

	case cancelClearMsg:
		// Clear the cancel pending state after timeout
		m.cancelPressTime = time.Time{}
		m.cancelKey = ""
		return m, nil

	case tea.BackgroundColorMsg:
		if m.theme == config.ThemeAuto {
			isDark := msg.IsDark()
			setCachedDarkMode(isDark)
			m.applyTheme(isDark)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate max textarea height as 50% of screen height (minus overhead for help text, border, etc.)
		// Overhead: help text (1) + textarea border (2) + kanban gap (1) = 4 lines
		const uiOverhead = 4
		maxAvailableHeight := (msg.Height - uiOverhead) * textareaMaxHeightPct / 100
		m.textareaMaxHeight = max(textareaMinHeight, maxAvailableHeight)

		// Update textarea MaxHeight setting
		m.textarea.MaxHeight = m.textareaMaxHeight

		// Calculate required height based on current content
		m.updateTextareaHeight()

		// Calculate kanban height based on current textarea height
		topSectionHeight := m.textareaHeight + 2              // +2 for border
		kanbanHeight := max(8, msg.Height-topSectionHeight-3) // -3 for help text + gap + statusline
		m.kanban.SetSize(msg.Width, kanbanHeight)

		// Calculate widths for alignment with Kanban columns.
		// Adaptive layout: switches between normal and narrow mode based on terminal width.
		//
		// Normal mode (wide terminal):
		// - Textarea right border aligns with Done column right border (3rd column)
		// - Options right border aligns with Warning column right border (4th column)
		//
		// Narrow mode (narrow terminal, when single column < minOptionsPanelWidth):
		// - Textarea right border aligns with Waiting column right border (2nd column)
		// - Options left border aligns with Done column left border (3rd column)
		// - Options right border aligns with Warning column right border (4th column)
		const kanbanColumnGap = 8 // Same as kanban.go: 4 columns × 2 chars border
		const minOptionsInnerWidth = 37
		const optionsPaddingBorder = 6                                           // padding(4) + border(2)
		const minOptionsPanelWidth = minOptionsInnerWidth + optionsPaddingBorder // 43

		// Calculate kanban column display width (must match kanban.go calculation)
		kanbanColWidth := (msg.Width - kanbanColumnGap) / 4
		kanbanColDisplayWidth := kanbanColWidth + 2 // +2 for border

		// Detect narrow terminal: switch layout when single column is too narrow for options
		isNarrow := kanbanColDisplayWidth < minOptionsPanelWidth

		var textareaDisplayWidth int
		if isNarrow {
			// Narrow mode: textarea spans 2 columns (Working + Waiting)
			// Waiting right = 2 * kanbanColDisplayWidth
			textareaDisplayWidth = 2 * kanbanColDisplayWidth
			// Options spans 2 columns (Done + Warning)
			m.optionsPanelWidth = 2 * kanbanColDisplayWidth
		} else {
			// Normal mode: textarea spans 3 columns (Working + Waiting + Done)
			// Done right = 3 * kanbanColDisplayWidth
			textareaDisplayWidth = 3 * kanbanColDisplayWidth
			// Options spans 1 column (Warning)
			// But ensure minimum width for content
			m.optionsPanelWidth = max(minOptionsPanelWidth, kanbanColDisplayWidth)
		}

		// Textarea display width = calculated above based on mode
		// SetWidth sets content+padding width, border adds 2 more
		textareaContentWidth := textareaDisplayWidth - 2 // -2 for border
		if textareaContentWidth > 30 {
			m.textarea.SetWidth(textareaContentWidth)
		}

	case tea.KeyMsg:
		keyStr := msg.String()

		// Handle Ctrl+C for copying selection (textarea or kanban)
		if keyStr == "ctrl+c" {
			if m.focusPanel == FocusPanelLeft && m.textarea.HasSelection() {
				_ = m.textarea.CopySelection()
				return m, nil
			}
			if m.focusPanel == FocusPanelKanban && m.kanban.HasSelection() {
				_ = m.kanban.CopySelection()
				return m, nil
			}
		}

		// Global keys (work in both panels)
		switch keyStr {
		case "esc", "ctrl+c":
			// Double-press detection: require pressing twice within cancelDoublePressTimeout
			now := time.Now()
			if !m.cancelPressTime.IsZero() && now.Sub(m.cancelPressTime) <= cancelDoublePressTimeout {
				// Second press within timeout - cancel
				m.cancelled = true
				return m, tea.Quit
			}
			// First press or timeout - record time and key, then wait for second press
			// Return a tick command to clear the pending state after timeout
			m.cancelPressTime = now
			m.cancelKey = keyStr // Store which key was pressed ("esc" or "ctrl+c")
			return m, tea.Tick(cancelDoublePressTimeout, func(t time.Time) tea.Msg {
				return cancelClearMsg{}
			})

		// Submit: Alt+Enter or F5
		case "alt+enter", "f5":
			m.applyOptionInputValues()
			content := strings.TrimSpace(m.textarea.Value())
			if content != "" {
				m.submitted = true
				return m, tea.Quit
			}
			return m, nil

		// Toggle panel: Alt+Tab (cycle between input box and options only)
		// Kanban is accessible via mouse click but not included in keyboard cycle
		case "alt+tab":
			m.applyOptionInputValues()
			switch m.focusPanel {
			case FocusPanelLeft:
				m.switchFocusTo(FocusPanelRight)
			case FocusPanelRight, FocusPanelKanban:
				// Always return to input box (Left panel)
				m.switchFocusTo(FocusPanelLeft)
			}
			return m, nil
		}

		// Panel-specific key handling
		if m.focusPanel == FocusPanelRight {
			return m.updateOptionsPanel(msg)
		}

		// Kanban panel key handling
		if m.focusPanel == FocusPanelKanban {
			return m.updateKanbanPanel(msg)
		}

		// Left panel (textarea) - handle mouse clicks below

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			// Determine which panel was clicked and switch focus
			clickedPanel := m.detectClickedPanel(msg.X, msg.Y)
			if clickedPanel != m.focusPanel {
				m.applyOptionInputValues()
				m.switchFocusTo(clickedPanel)
			}

			// Handle textarea mouse selection if clicking in textarea
			if clickedPanel == FocusPanelLeft {
				// Clear Kanban selection when clicking textarea
				m.kanban.ClearSelection()
				m.kanbanSelecting = false

				if row, col, ok := m.handleTextareaMouse(msg.X, msg.Y); ok {
					m.mouseSelecting = true
					m.selectAnchorRow = row
					m.selectAnchorCol = col
					m.textarea.SetSelection(row, col, row, col)
				}
			}

			// Handle Kanban mouse selection
			if clickedPanel == FocusPanelKanban {
				// Clear textarea selection when clicking kanban
				m.textarea.ClearSelection()
				m.mouseSelecting = false

				col := m.detectKanbanColumn(msg.X)
				m.kanban.SetFocusedColumn(col)

				// Start Kanban text selection (column-aware)
				kanbanY := m.getKanbanRelativeY(msg.Y)
				kanbanX := m.getKanbanRelativeX(msg.X, col)
				m.kanbanSelecting = true
				m.kanbanSelectCol = col
				m.kanbanSelectX = kanbanX
				m.kanbanSelectY = kanbanY
				m.kanban.StartSelection(col, kanbanX, kanbanY)
			}
		}

	case tea.MouseMotionMsg:
		// Process drag motion based on state tracking, not button field.
		// In AllMotion mode, MouseMotionMsg.Button may not reflect the held button.
		// The selecting state is set in MouseClickMsg and cleared in MouseReleaseMsg.
		if m.mouseSelecting {
			if row, col, ok := m.handleTextareaMouse(msg.X, msg.Y); ok {
				m.textarea.SetSelection(m.selectAnchorRow, m.selectAnchorCol, row, col)
			}
		}
		if m.kanbanSelecting {
			kanbanY := m.getKanbanRelativeY(msg.Y)
			// Use the same column as when selection started (don't allow cross-column selection)
			kanbanX := m.getKanbanRelativeX(msg.X, m.kanbanSelectCol)
			m.kanban.ExtendSelection(kanbanX, kanbanY)
		}

	case tea.MouseReleaseMsg:
		if msg.Button == tea.MouseLeft {
			if m.mouseSelecting {
				m.mouseSelecting = false
				if !m.textarea.HasSelection() {
					m.textarea.ClearSelection()
				}
			}
			if m.kanbanSelecting {
				m.kanbanSelecting = false
				m.kanban.EndSelection()
			}
		}

	case tea.MouseWheelMsg:
		// Handle mouse scroll on the focused panel
		m.handleMouseScroll(msg)
	}

	// Update textarea if left panel is focused
	if m.focusPanel == FocusPanelLeft {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)

		// Update textarea height dynamically based on content
		m.updateTextareaHeight()
	}

	return m, tea.Batch(cmds...)
}


// View renders the task input.
func (m *TaskInput) View() tea.View {
	// Adaptive color for help text (use cached isDark value)
	lightDark := lipgloss.LightDark(m.isDark)
	dimColor := lightDark(lipgloss.Color("245"), lipgloss.Color("240"))

	helpStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	// Build left panel (task input) with scrollbar if needed
	textareaView := m.textarea.View()

	contentLines := strings.Count(m.textarea.Value(), "\n") + 1
	visibleLines := m.textarea.Height()
	if contentLines > visibleLines && visibleLines > 0 {
		scrollOffset := m.textarea.Line() // Use cursor line as scroll indicator
		scrollbar := renderVerticalScrollbar(contentLines, visibleLines, scrollOffset, m.isDark)
		textareaView = embedScrollbarInTextarea(textareaView, scrollbar, visibleLines)
	}

	// Build right panel (options)
	rightPanel := m.renderOptionsPanel()

	// Join panels horizontally (no gap for proper alignment with Kanban columns)
	topSection := lipgloss.JoinHorizontal(
		lipgloss.Top,
		textareaView,
		rightPanel,
	)

	// Build content with version+tip at top-left and help text at top-right
	var sb strings.Builder

	// Version and tip style (same dim color as help text)
	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)
	tipStyle := helpStyle

	// Show warning if terminal is smaller than 85x22
	isNarrow := m.width < 85 || m.height < 22

	// Left side: PAW {version} - {projectName}  Tip: {tip} or Warning
	versionText := versionStyle.Render("PAW " + Version)
	projectText := ""
	if ProjectName != "" {
		projectText = versionStyle.Render(" - " + ProjectName)
	}

	// Show warning in bright red if terminal is too small, otherwise show tip
	var tipText string
	if isNarrow {
		warningStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")). // Bright red
			Bold(true)
		tipText = warningStyle.Render("  ⚠️  Terminal too small - content may be truncated")
	} else {
		tipText = tipStyle.Render("  Tip: " + m.currentTip)
	}

	leftContent := versionText + projectText + tipText
	leftWidth := lipgloss.Width(leftContent)

	// Show cancel pending hint if waiting for second press, otherwise show normal help text
	if m.isCancelPending() {
		// Cancel pending state - show prominent hint on the right
		cancelHintStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")). // Orange/yellow for visibility
			Bold(true)
		// Display the appropriate key based on what was pressed
		keyName := "Esc"
		if m.cancelKey == "ctrl+c" {
			keyName = "Ctrl+C"
		}
		cancelHint := cancelHintStyle.Render(fmt.Sprintf("Press %s again to cancel", keyName))
		hintWidth := lipgloss.Width(cancelHint)

		sb.WriteString(leftContent)
		gap := m.width - leftWidth - hintWidth
		if gap > 0 {
			sb.WriteString(strings.Repeat(" ", gap))
		}
		sb.WriteString(cancelHint)
	} else {
		// Determine help text based on focus panel
		var helpText string
		switch m.focusPanel {
		case FocusPanelLeft:
			helpText = "Alt+Enter: Submit  |  Esc×2: Cancel"
		case FocusPanelRight:
			helpText = "Alt+Enter: Submit  |  Esc×2: Cancel"
		case FocusPanelKanban:
			helpText = "Alt+Enter: Submit  |  Esc×2: Cancel"
		}

		// Add version+tip on left, help text on right
		helpRendered := helpStyle.Render(helpText)
		helpWidth := lipgloss.Width(helpRendered)

		sb.WriteString(leftContent)
		gap := m.width - leftWidth - helpWidth
		if gap > 0 {
			sb.WriteString(strings.Repeat(" ", gap))
		}
		sb.WriteString(helpRendered)
	}
	sb.WriteString("\n")

	sb.WriteString(topSection)
	sb.WriteString("\n")

	// Add Kanban view if there's enough space (no extra gap)
	if m.height > 20 {
		kanbanContent := m.kanban.Render()
		if kanbanContent != "" {
			sb.WriteString(kanbanContent)
		}
	}

	v := tea.NewView(sb.String())
	v.AltScreen = true
	// Use AllMotion for better tmux mouse passthrough compatibility
	// CellMotion was causing tmux to intercept mouse events and use its own copy-mode
	// (line-by-line selection at window level instead of cell-based TUI selection)
	v.MouseMode = tea.MouseModeAllMotion

	// Set real cursor based on focus
	if m.focusPanel == FocusPanelLeft {
		if cursor := m.textarea.Cursor(); cursor != nil {
			cursor.Y += 2 // Account for help text line + top border
			cursor.X += 1
			v.Cursor = cursor
		}
	}

	return v
}


