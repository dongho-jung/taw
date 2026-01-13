// Package tui provides terminal user interface components for PAW.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/logging"
)

// SettingsScope represents whether we're editing global or project settings.
type SettingsScope int

const (
	SettingsScopeProject SettingsScope = iota
	SettingsScopeGlobal
)

// SettingsTab represents which tab is active.
type SettingsTab int

const (
	SettingsTabGeneral SettingsTab = iota
)

const settingsTabCount = 1

// SettingsField represents the current field being edited.
type SettingsField int

// General tab fields
const (
	SettingsFieldTheme SettingsField = iota
	SettingsFieldPawInProject
	SettingsFieldSelfImprove
)

const generalFieldCount = 3

// SettingsUI provides an interactive settings configuration form.
type SettingsUI struct {
	// Current scope (global or project)
	scope         SettingsScope
	globalConfig  *config.Config
	projectConfig *config.Config
	inheritConfig *config.InheritConfig

	// Active config being edited (points to global or project)
	config *config.Config

	tab       SettingsTab
	field     SettingsField
	width     int
	height    int
	done      bool
	cancelled bool
	isDark    bool
	isGitRepo bool
	colors    ThemeColors

	// Field indices for dropdown-style fields
	// For fields with inherit option, index 0 = "inherit"
	themeIdx        int // 0=auto, 1...N=presets (no inherit option)
	pawInProjectIdx int // 0=auto, 1=global, 2=local (global scope only, no inherit)
	selfImproveIdx  int // 0=inherit, 1=on, 2=off (project scope) or 0=on, 1=off (global scope)
}

// SettingsResult contains the result of the settings UI.
type SettingsResult struct {
	GlobalConfig  *config.Config
	ProjectConfig *config.Config
	InheritConfig *config.InheritConfig
	Scope         SettingsScope
	Cancelled     bool
}

// NewSettingsUI creates a new settings UI.
// globalCfg is the global config from $HOME/.paw/config.
// projectCfg is the project config from .paw/config.
func NewSettingsUI(globalCfg, projectCfg *config.Config, isGitRepo bool) *SettingsUI {
	logging.Debug("-> NewSettingsUI(isGitRepo=%v)", isGitRepo)
	defer logging.Debug("<- NewSettingsUI")

	// Detect dark mode BEFORE bubbletea starts
	isDark := DetectDarkMode()

	if globalCfg == nil {
		globalCfg = config.DefaultConfig()
	}
	if projectCfg == nil {
		projectCfg = config.DefaultConfig()
	}

	// Clone configs to avoid modifying originals
	globalCfg = globalCfg.Clone()
	projectCfg = projectCfg.Clone()

	// Get or create inherit config
	inheritCfg := projectCfg.Inherit
	if inheritCfg == nil {
		inheritCfg = config.DefaultInheritConfig()
	}

	// Start with project scope, using project config
	cfg := projectCfg

	ui := &SettingsUI{
		scope:         SettingsScopeProject,
		globalConfig:  globalCfg,
		projectConfig: projectCfg,
		inheritConfig: inheritCfg,
		config:        cfg,
		tab:           SettingsTabGeneral,
		field:         0,
		isDark:        isDark,
		isGitRepo:     isGitRepo,
		colors:        NewThemeColors(isDark),
	}

	// Initialize field indices based on current config
	ui.initFieldIndices()

	return ui
}

// initFieldIndices initializes dropdown indices based on current config.
func (m *SettingsUI) initFieldIndices() {
	cfg := m.config

	// Theme: no inherit option (0=auto, 1+=presets)
	m.themeIdx = 0 // default to auto
	themeOptions := getThemeOptions()
	for i, t := range themeOptions {
		if t == cfg.Theme {
			m.themeIdx = i
			break
		}
	}

	// PawInProject: global scope only, no inherit option (0=auto, 1=global, 2=local)
	modes := config.ValidPawInProjectModes()
	m.pawInProjectIdx = 0 // default to auto
	for i, mode := range modes {
		if mode == cfg.PawInProject {
			m.pawInProjectIdx = i
			break
		}
	}

	// SelfImprove: For project scope, index 0 = inherit
	if m.scope == SettingsScopeProject && m.inheritConfig != nil && m.inheritConfig.SelfImprove {
		m.selfImproveIdx = 0 // inherit
	} else {
		offset := 0
		if m.scope == SettingsScopeProject {
			offset = 1
		}
		if cfg.SelfImprove {
			m.selfImproveIdx = 0 + offset // on
		} else {
			m.selfImproveIdx = 1 + offset // off
		}
	}
}

// getThemeOptions returns the list of available theme options.
func getThemeOptions() []string {
	return []string{
		"auto",
		"dark", "dark-blue", "dark-green", "dark-purple", "dark-warm", "dark-mono",
		"light", "light-blue", "light-green", "light-purple", "light-warm", "light-mono",
	}
}

// Init initializes the settings UI.
func (m *SettingsUI) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update handles messages and updates the model.
func (m *SettingsUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.cancelled = true
			m.done = true
			return m, tea.Quit

		case "enter":
			// Save and quit
			m.applyChanges()
			m.done = true
			return m, tea.Quit

		case "ctrl+s":
			// Save shortcut
			m.applyChanges()
			m.done = true
			return m, tea.Quit

		case "tab":
			// Switch tabs
			m.tab = (m.tab + 1) % settingsTabCount
			m.field = 0
			return m, nil

		case "shift+tab":
			m.tab = (m.tab - 1 + settingsTabCount) % settingsTabCount
			m.field = 0
			return m, nil

		case "alt+tab", "alt+shift+tab":
			// Switch between Global and Project scope
			m.switchScope()
			return m, nil

		case "down", "j":
			m.field = (m.field + 1) % m.currentFieldCount()
			return m, nil

		case "up", "k":
			m.field = (m.field - 1 + m.currentFieldCount()) % m.currentFieldCount()
			return m, nil

		case "left", "h":
			m.handleLeft()
			return m, nil

		case "right", "l":
			m.handleRight()
			return m, nil

		}
	}

	return m, nil
}

func (m *SettingsUI) currentFieldCount() SettingsField {
	return generalFieldCount
}

func (m *SettingsUI) handleLeft() {
	switch m.field {
	case SettingsFieldTheme:
		if m.themeIdx > 0 {
			m.themeIdx--
		}
	case SettingsFieldPawInProject:
		// Only editable in global scope
		if m.scope == SettingsScopeGlobal && m.pawInProjectIdx > 0 {
			m.pawInProjectIdx--
		}
	case SettingsFieldSelfImprove:
		if m.selfImproveIdx > 0 {
			m.selfImproveIdx--
		}
	}
}

func (m *SettingsUI) handleRight() {
	switch m.field {
	case SettingsFieldTheme:
		themeOptions := getThemeOptions()
		if m.themeIdx < len(themeOptions)-1 {
			m.themeIdx++
		}
	case SettingsFieldPawInProject:
		// Only editable in global scope (0=auto, 1=global, 2=local)
		maxIdx := len(config.ValidPawInProjectModes()) - 1
		if m.scope == SettingsScopeGlobal && m.pawInProjectIdx < maxIdx {
			m.pawInProjectIdx++
		}
	case SettingsFieldSelfImprove:
		maxIdx := 1 // on=0, off=1
		if m.scope == SettingsScopeProject {
			maxIdx = 2 // inherit=0, on=1, off=2
		}
		if m.selfImproveIdx < maxIdx {
			m.selfImproveIdx++
		}
	}
}

func (m *SettingsUI) applyChanges() {
	// Ensure inherit config exists for project scope
	if m.scope == SettingsScopeProject && m.inheritConfig == nil {
		m.inheritConfig = config.DefaultInheritConfig()
	}

	// Apply Theme (no inherit option)
	themeOptions := getThemeOptions()
	if m.themeIdx < len(themeOptions) {
		m.config.Theme = themeOptions[m.themeIdx]
	}

	// Apply PawInProject (global scope only, no inherit option)
	if m.scope == SettingsScopeGlobal {
		modes := config.ValidPawInProjectModes()
		if m.pawInProjectIdx < len(modes) {
			m.config.PawInProject = modes[m.pawInProjectIdx]
		}
	}

	// Apply SelfImprove
	if m.scope == SettingsScopeProject {
		if m.selfImproveIdx == 0 {
			// inherit selected
			m.inheritConfig.SelfImprove = true
			m.config.SelfImprove = m.globalConfig.SelfImprove
		} else {
			m.inheritConfig.SelfImprove = false
			m.config.SelfImprove = (m.selfImproveIdx == 1) // 1=on, 2=off
		}
	} else {
		// Global scope - no inherit option
		m.config.SelfImprove = (m.selfImproveIdx == 0) // 0=on, 1=off
	}
}

// switchScope toggles between Global and Project settings scope.
func (m *SettingsUI) switchScope() {
	// Apply current changes before switching
	m.applyChanges()

	// Toggle scope
	if m.scope == SettingsScopeProject {
		m.scope = SettingsScopeGlobal
		m.config = m.globalConfig
	} else {
		m.scope = SettingsScopeProject
		m.config = m.projectConfig
	}

	// Update field indices for new config
	m.updateFieldIndices()

	// Reset to first tab/field
	m.tab = SettingsTabGeneral
	m.field = 0
}

// updateFieldIndices updates dropdown indices based on current config.
func (m *SettingsUI) updateFieldIndices() {
	// Reinitialize all field indices for the current scope
	m.initFieldIndices()
}

// View renders the settings UI.
func (m *SettingsUI) View() tea.View {
	c := m.colors

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(c.Accent).
		MarginBottom(1)

	tabStyle := lipgloss.NewStyle().
		Foreground(c.TextDim).
		Padding(0, 2)

	activeTabStyle := lipgloss.NewStyle().
		Foreground(c.Accent).
		Bold(true).
		Padding(0, 2).
		Underline(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(c.TextNormal).
		Width(18)

	selectedLabelStyle := lipgloss.NewStyle().
		Foreground(c.Accent).
		Bold(true).
		Width(18)

	valueStyle := lipgloss.NewStyle().
		Foreground(c.TextNormal)

	selectedValueStyle := lipgloss.NewStyle().
		Foreground(c.Accent).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(c.TextDim)

	helpStyle := lipgloss.NewStyle().
		Foreground(c.TextDim).
		MarginTop(1)

	// Use _ to suppress unused warnings
	_ = tabStyle
	_ = activeTabStyle

	var sb strings.Builder

	// Title with scope indicator
	scopeText := "[Project]"
	if m.scope == SettingsScopeGlobal {
		scopeText = "[Global]"
	}
	sb.WriteString(titleStyle.Render("Settings " + scopeText))
	sb.WriteString("\n\n")

	separatorWidth := m.width - 4 // account for margins
	if separatorWidth < 40 {
		separatorWidth = 40
	}
	sb.WriteString(dimStyle.Render(strings.Repeat("-", separatorWidth)))
	sb.WriteString("\n\n")

	// Content
	m.renderGeneralTab(&sb, labelStyle, selectedLabelStyle, valueStyle, selectedValueStyle, dimStyle)

	// Help text
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("Alt+Tab: Global/Project  |  Up/Down: Navigate  |  Left/Right: Change  |  Ctrl+S: Save  |  Esc: Cancel"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

func (m *SettingsUI) renderGeneralTab(sb *strings.Builder, labelStyle, selectedLabelStyle, valueStyle, selectedValueStyle, dimStyle lipgloss.Style) {
	// Helper to render a selector-style field: < value >
	renderSelector := func(value string, focused bool, hint string) string {
		var display string
		if focused {
			display = selectedValueStyle.Render("< " + value + " >")
		} else {
			display = valueStyle.Render("[ " + value + " ]")
		}
		if hint != "" {
			display += dimStyle.Render(" " + hint)
		}
		return display
	}

	// Theme (no inherit option)
	{
		label := labelStyle.Render("Theme:")
		if m.field == SettingsFieldTheme {
			label = selectedLabelStyle.Render("Theme:")
		}
		focused := m.field == SettingsFieldTheme

		themeOptions := getThemeOptions()
		currentTheme := "auto"
		if m.themeIdx < len(themeOptions) {
			currentTheme = themeOptions[m.themeIdx]
		}
		sb.WriteString(label + renderSelector(currentTheme, focused, ""))
		sb.WriteString("\n")
	}

	// Paw In Project (global scope only, no inherit)
	{
		label := labelStyle.Render("Workspace:")
		if m.field == SettingsFieldPawInProject {
			label = selectedLabelStyle.Render("Workspace:")
		}
		focused := m.field == SettingsFieldPawInProject

		var currentValue string
		var hint string

		if m.scope == SettingsScopeGlobal {
			modes := config.ValidPawInProjectModes()
			if m.pawInProjectIdx < len(modes) {
				switch modes[m.pawInProjectIdx] {
				case config.PawInProjectAuto:
					currentValue = "auto"
					hint = "(git→global, non-git→local)"
				case config.PawInProjectGlobal:
					currentValue = "global"
					hint = "(~/.local/share/paw/workspaces)"
				case config.PawInProjectLocal:
					currentValue = "local"
					hint = "(.paw in project dir)"
				}
			}
		} else {
			// Project scope - show read-only inherited value
			switch m.globalConfig.PawInProject {
			case config.PawInProjectAuto:
				currentValue = "auto"
				hint = "(set in global config)"
			case config.PawInProjectGlobal:
				currentValue = "global"
				hint = "(set in global config)"
			case config.PawInProjectLocal:
				currentValue = "local"
				hint = "(set in global config)"
			default:
				currentValue = "auto"
				hint = "(set in global config)"
			}
		}
		sb.WriteString(label + renderSelector(currentValue, focused, hint))
		sb.WriteString("\n")
	}

	// Self Improve (with inherit option in project scope)
	{
		label := labelStyle.Render("Self Improve:")
		if m.field == SettingsFieldSelfImprove {
			label = selectedLabelStyle.Render("Self Improve:")
		}
		focused := m.field == SettingsFieldSelfImprove

		var currentValue string
		var hint string

		if m.scope == SettingsScopeProject {
			// inherit=0, on=1, off=2
			switch m.selfImproveIdx {
			case 0:
				currentValue = "inherit"
				if m.globalConfig.SelfImprove {
					hint = "(on)"
				} else {
					hint = "(off)"
				}
			case 1:
				currentValue = "on"
			case 2:
				currentValue = "off"
			}
		} else {
			// on=0, off=1
			if m.selfImproveIdx == 0 {
				currentValue = "on"
			} else {
				currentValue = "off"
			}
		}
		sb.WriteString(label + renderSelector(currentValue, focused, hint))
		sb.WriteString("\n")
	}
}

// Result returns the settings result.
func (m *SettingsUI) Result() SettingsResult {
	// Apply changes to ensure config is up-to-date
	m.applyChanges()

	// Attach inherit config to project config
	m.projectConfig.Inherit = m.inheritConfig

	return SettingsResult{
		GlobalConfig:  m.globalConfig,
		ProjectConfig: m.projectConfig,
		InheritConfig: m.inheritConfig,
		Scope:         m.scope,
		Cancelled:     m.cancelled,
	}
}

// RunSettingsUI runs the settings UI and returns the result.
// globalCfg is the global config from $HOME/.paw/config.
// projectCfg is the project config from .paw/config.
func RunSettingsUI(globalCfg, projectCfg *config.Config, isGitRepo bool) (*SettingsResult, error) {
	logging.Debug("-> RunSettingsUI(isGitRepo=%v)", isGitRepo)
	defer logging.Debug("<- RunSettingsUI")

	// Reset theme cache to ensure fresh detection on each TUI start
	ResetDarkModeCache()

	m := NewSettingsUI(globalCfg, projectCfg, isGitRepo)
	logging.Debug("RunSettingsUI: starting tea.Program")
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		logging.Debug("RunSettingsUI: tea.Program.Run failed: %v", err)
		return nil, err
	}

	ui := finalModel.(*SettingsUI)
	result := ui.Result()
	logging.Debug("RunSettingsUI: completed, cancelled=%v, scope=%d", result.Cancelled, result.Scope)
	return &result, nil
}
