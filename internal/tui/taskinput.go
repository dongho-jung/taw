// Package tui provides terminal user interface components for PAW.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/tmux"
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
	OptFieldBranchName
)

// optFieldCount returns the number of option fields based on git mode.
// In non-git mode, the Branch field is hidden.
func optFieldCount(isGitRepo bool) int {
	if isGitRepo {
		return 2 // Model + Branch
	}
	return 1 // Model only
}

// cancelDoublePressTimeout is the time window for double-press cancel detection.
const cancelDoublePressTimeout = 2 * time.Second

// Duration for the transient template jump tip.
const templateTipDuration = 3 * time.Second

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
	isDark      bool     // Cached dark mode detection (must be detected before bubbletea starts)
	isGitRepo   bool     // Whether the project is a git repository
	pawDir      string

	// Dynamic textarea height
	textareaHeight    int // Current textarea height (visible lines)
	textareaMaxHeight int // Maximum textarea height (50% of screen)

	// Dynamic panel widths for alignment with Kanban columns
	optionsPanelWidth int // Options panel display width (dynamic for alignment)

	// Inline options editing
	focusPanel FocusPanel
	optField   OptField
	modelIdx   int
	branchName string // Custom branch name input (empty = auto)

	mouseSelecting  bool
	selectAnchorRow int
	selectAnchorCol int

	// Kanban mouse selection (column-aware)
	kanbanSelecting  bool
	kanbanSelectCol  int // Column being selected (0-3)
	kanbanSelectX    int // X position relative to column
	kanbanSelectY    int // Y position relative to kanban area
	kanbanClickStart struct {
		x, y int // Initial click position (absolute) for detecting single click vs drag
	}

	// Kanban view for tasks across all sessions
	kanban *KanbanView

	// Double-press cancel detection
	cancelPressTime time.Time
	cancelKey       string // Track which key was pressed for cancel ("esc" or "ctrl+c")

	// Tip caching - only changes every minute
	currentTip     string
	lastTipRefresh time.Time

	// Template draft tracking for Ctrl+T
	lastTemplateDraft string
	templateTipUntil  time.Time

	// Cross-project jump target (set when user requests to jump to external project)
	jumpTarget *JumpTarget
}

// tickMsg is used for periodic Kanban refresh.
type tickMsg time.Time

// cancelClearMsg is used to clear the cancel pending state after timeout.
type cancelClearMsg struct{}

// templateTipClearMsg triggers a redraw after the transient template tip expires.
type templateTipClearMsg struct{}

// JumpTarget contains information for jumping to a task in an external project.
type JumpTarget struct {
	Session  string // Target session name (project name)
	WindowID string // Target window ID
}

// jumpToTaskMsg is the result of attempting to jump to a task.
type jumpToTaskMsg struct {
	err        error
	jumpTarget *JumpTarget // Non-nil for cross-project jumps
}

// TaskInputResult contains the result of the task input.
type TaskInputResult struct {
	Content    string
	Options    *config.TaskOptions
	Cancelled  bool
	JumpTarget *JumpTarget // Non-nil if user requested to jump to an external project task
}

// NewTaskInput creates a new task input model.
// Deprecated: Use NewTaskInputWithOptions for explicit git mode control.
func NewTaskInput() *TaskInput {
	return NewTaskInputWithOptions(nil, true) // Default to git mode for backward compatibility
}

// NewTaskInputWithTasks creates a new task input model with active task list.
// Deprecated: Use NewTaskInputWithOptions for explicit git mode control.
func NewTaskInputWithTasks(activeTasks []string) *TaskInput {
	return NewTaskInputWithOptions(activeTasks, true) // Default to git mode for backward compatibility
}

// NewTaskInputWithOptions creates a new task input model with active task list and git mode flag.
func NewTaskInputWithOptions(activeTasks []string, isGitRepo bool) *TaskInput {
	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

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
	ta.HighlightToken = templatePlaceholderToken

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
		isDark:            isDark,
		isGitRepo:         isGitRepo,
		pawDir:            findPawDir(),
		textareaHeight:    textareaDefaultHeight,
		textareaMaxHeight: 15, // Will be updated on WindowSizeMsg
		optionsPanelWidth: 43, // Default, will be updated on WindowSizeMsg for alignment
		focusPanel:        FocusPanelLeft,
		optField:          OptFieldModel,
		modelIdx:          modelIdx,
		branchName:        opts.BranchName,
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
	return tea.Batch(textarea.Blink, m.tickCmd(), tea.RequestBackgroundColor)
}

// tickCmd returns a command that triggers a tick after 1 second.
// The short interval ensures responsive kanban updates for working tasks.
func (m *TaskInput) tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// jumpToTask returns a command that navigates to the given task.
// For same-project tasks, it directly selects the window.
// For different-project tasks, it returns a jump target for the parent to handle
// (since cross-socket jumps require replacing the current process).
func jumpToTask(task *service.DiscoveredTask) tea.Cmd {
	return func() tea.Msg {
		if task == nil {
			return jumpToTaskMsg{}
		}

		// Check if we need to jump to a different project
		// Use SessionName (tmux session name) for comparison, not ProjectName (display name)
		// In subdirectory context: SessionName="repo-subdir", ProjectName="repo/subdir"
		if task.Session != SessionName {
			// Different project - return jump target for parent to handle
			// Cross-socket jumps require replacing the current process with tmux attach
			return jumpToTaskMsg{
				jumpTarget: &JumpTarget{
					Session:  task.Session,
					WindowID: task.WindowID,
				},
			}
		}

		// Same project - just select the window directly
		tm := tmux.New(task.Session)
		if err := tm.SelectWindow(task.WindowID); err != nil {
			return jumpToTaskMsg{err: err}
		}

		return jumpToTaskMsg{}
	}
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

	// Always adjust viewport position based on visual lines vs visible height
	// Use TotalVisualLines() which accounts for soft-wrapped lines, not just newlines
	// This prevents cursor from going outside the box when long lines are wrapped
	visualLines := m.textarea.TotalVisualLines()
	if visualLines <= m.textareaHeight {
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
		// Persist template draft on tick (debounced, not on every keystroke)
		m.persistTemplateDraft()
		// Check for history/template selection on tick (not on every keystroke)
		// This avoids file I/O on every keystroke which causes stuttering
		m.checkHistorySelection()
		if m.checkTemplateSelection() {
			cmds = append(cmds, tea.Tick(templateTipDuration, func(t time.Time) tea.Msg {
				return templateTipClearMsg{}
			}))
		}
		// Refresh tip every minute
		if time.Since(m.lastTipRefresh) >= time.Minute {
			m.currentTip = GetTip()
			m.lastTipRefresh = time.Now()
		}
		// Schedule next tick
		return m, tea.Batch(append(cmds, m.tickCmd())...)

	case cancelClearMsg:
		// Clear the cancel pending state after timeout
		m.cancelPressTime = time.Time{}
		m.cancelKey = ""
		return m, nil

	case templateTipClearMsg:
		// No-op; triggers a redraw after the transient tip window elapses.
		return m, nil

	case jumpToTaskMsg:
		if msg.jumpTarget != nil {
			// Cross-project jump - store the target and exit TUI
			// The parent command will handle the actual jump via syscall.Exec
			m.jumpTarget = msg.jumpTarget
			return m, tea.Quit
		}
		// Same-project jump completed - nothing to do on success
		return m, nil

	case tea.BackgroundColorMsg:
		isDark := msg.IsDark()
		setCachedDarkMode(isDark)
		m.applyTheme(isDark)
		return m, nil

	case tea.FocusMsg:
		// When terminal gains focus (user switches to this window),
		// automatically focus the task input textarea
		m.switchFocusTo(FocusPanelLeft)
		// Check for history/template selection when window regains focus
		// (e.g., returning from Ctrl+R history picker or Ctrl+T template picker)
		m.checkHistorySelection()
		if m.checkTemplateSelection() {
			return m, tea.Tick(templateTipDuration, func(t time.Time) tea.Msg {
				return templateTipClearMsg{}
			})
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
		// 3-column Kanban layout: Working, Waiting, Done
		//
		// Alignment goals:
		// - Textarea right border aligns with Waiting column (2nd column)
		// - Options right border aligns with Done column (3rd column)
		//
		// On narrow terminals where column width < minOptionsPanelWidth,
		// we prioritize Options minimum width over perfect alignment.
		const kanbanColumnGap = 6 // Same as kanban.go: 3 columns × 2 chars border
		const minOptionsInnerWidth = 37
		const optionsPaddingBorder = 6                                           // padding(4) + border(2)
		const minOptionsPanelWidth = minOptionsInnerWidth + optionsPaddingBorder // 43
		const minTextareaContentWidth = 30

		// Calculate kanban column display width (must match kanban.go calculation)
		kanbanColWidth := (msg.Width - kanbanColumnGap) / 3
		kanbanColDisplayWidth := kanbanColWidth + 2 // +2 for border

		var textareaDisplayWidth int
		if kanbanColDisplayWidth >= minOptionsPanelWidth {
			// Wide enough: use exact column-aligned widths
			// This ensures perfect border alignment with Kanban columns
			textareaDisplayWidth = 2 * kanbanColDisplayWidth // Aligns with Waiting right border
			m.optionsPanelWidth = kanbanColDisplayWidth      // Aligns with Done right border
		} else {
			// Narrow terminal: prioritize Options minimum width over alignment
			m.optionsPanelWidth = minOptionsPanelWidth
			textareaDisplayWidth = msg.Width - m.optionsPanelWidth
		}

		textareaContentWidth := textareaDisplayWidth - 2 // -2 for border
		if textareaContentWidth < minTextareaContentWidth {
			textareaContentWidth = minTextareaContentWidth
		}
		m.textarea.SetWidth(textareaContentWidth)

	case tea.KeyMsg:
		keyStr := msg.String()

		// Handle Ctrl+C or Cmd+C for copying selection (textarea or kanban)
		// Cmd+C works on terminals that support the Kitty keyboard protocol
		key := msg.Key()
		isCopyKey := keyStr == "ctrl+c" || (key.Code == 'c' && key.Mod&tea.ModSuper != 0)
		if isCopyKey {
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

		case "tab":
			if m.focusPanel == FocusPanelLeft {
				if m.jumpToNextTemplatePlaceholder() {
					return m, nil
				}
			}

		// Toggle panel: Alt+Tab (cycle through input box, options, and non-empty kanban columns)
		// Cycle order: Left → Right → Kanban(non-empty cols only) → Left
		// Empty kanban columns are skipped during cycling
		case "alt+tab":
			m.applyOptionInputValues()
			switch m.focusPanel {
			case FocusPanelLeft:
				m.switchFocusTo(FocusPanelRight)
			case FocusPanelRight:
				// Move to first non-empty Kanban column, or skip to Left if all empty
				firstCol := m.kanban.FirstNonEmptyColumn()
				if firstCol >= 0 {
					m.switchFocusToKanbanColumn(firstCol)
				} else {
					m.switchFocusTo(FocusPanelLeft)
				}
			case FocusPanelKanban:
				// Find next non-empty column, or go back to Left if none
				currentCol := m.kanban.FocusedColumn()
				nextCol := m.kanban.NextNonEmptyColumn(currentCol)
				// If next non-empty column would wrap back to or before current, go to Left
				if nextCol < 0 || nextCol <= currentCol {
					m.switchFocusTo(FocusPanelLeft)
				} else {
					m.switchFocusToKanbanColumn(nextCol)
				}
			}
			return m, nil

		// Toggle panel backward: Alt+Shift+Tab (cycle backward through panels)
		// Cycle order: Left → Kanban(col 2) → Kanban(col 1) → Kanban(col 0) → Right → Left
		case "alt+shift+tab":
			m.applyOptionInputValues()
			switch m.focusPanel {
			case FocusPanelLeft:
				// Move to last Kanban column (Done = column 2)
				m.switchFocusToKanbanColumn(2)
			case FocusPanelRight:
				m.switchFocusTo(FocusPanelLeft)
			case FocusPanelKanban:
				// Cycle backward through Kanban columns, then to Right
				currentCol := m.kanban.FocusedColumn()
				if currentCol > 0 {
					m.switchFocusToKanbanColumn(currentCol - 1)
				} else {
					m.switchFocusTo(FocusPanelRight)
				}
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

				// Track initial click position for single-click vs drag detection
				m.kanbanClickStart.x = msg.X
				m.kanbanClickStart.y = msg.Y

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

				// Check if this was a single click (minimal movement from click start)
				// If so, jump to the clicked task instead of selecting text
				dx := msg.X - m.kanbanClickStart.x
				dy := msg.Y - m.kanbanClickStart.y
				if dx >= -2 && dx <= 2 && dy >= -2 && dy <= 2 {
					// Single click detected - try to jump to task
					kanbanY := m.getKanbanRelativeY(msg.Y)
					col := m.detectKanbanColumn(msg.X)
					if task := m.kanban.GetTaskAtPosition(col, kanbanY); task != nil {
						// Clear selection since we're jumping, not selecting
						m.kanban.ClearSelection()
						return m, jumpToTask(task)
					}
				}
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
		// Note: persistTemplateDraft() is called on tick (every 1s) for performance
		// instead of on every keystroke to avoid disk I/O stuttering
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

	// Show warning if terminal is smaller than 72x22
	isNarrow := m.width < 72 || m.height < 22

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
		if time.Now().Before(m.templateTipUntil) {
			templateTipStyle := lipgloss.NewStyle().
				Foreground(lightDark(lipgloss.Color("24"), lipgloss.Color("214"))).
				Bold(true)
			tipText = templateTipStyle.Render("  Tip: Tab to jump to next ___ placeholder")
		} else {
			tipText = tipStyle.Render("  Tip: " + m.currentTip)
		}
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
	v.ReportFocus = true
	// Use AllMotion for better tmux mouse passthrough compatibility
	// CellMotion was causing tmux to intercept mouse events and use its own copy-mode
	// (line-by-line selection at window level instead of cell-based TUI selection)
	v.MouseMode = tea.MouseModeAllMotion

	// Set real cursor based on focus
	if m.focusPanel == FocusPanelLeft {
		if cursor := m.textarea.Cursor(); cursor != nil {
			cursor.Y += 2 // Account for help text line + top border
			cursor.X += 1

			// Clamp cursor Y to textarea visible bounds as a safety measure
			// This prevents cursor from appearing outside the box in edge cases
			// (e.g., during rapid scrolling or when soft-wrapping changes)
			minY := 2                        // help text (1) + top border (1)
			maxY := 2 + m.textareaHeight - 1 // last visible content line
			if cursor.Y < minY {
				cursor.Y = minY
			} else if cursor.Y > maxY {
				cursor.Y = maxY
			}

			v.Cursor = cursor
		}
	}

	return v
}
