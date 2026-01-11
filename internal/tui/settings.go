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
	SettingsTabNotifications
)

const settingsTabCount = 2

// SettingsField represents the current field being edited.
type SettingsField int

// General tab fields
const (
	SettingsFieldWorkMode SettingsField = iota
	SettingsFieldOnComplete
	SettingsFieldTheme
	SettingsFieldNonGitWorkspace
	SettingsFieldVerifyRequired
	SettingsFieldSelfImprove
)

const generalFieldCount = 6

// Notifications tab fields
const (
	SettingsFieldSlackWebhook SettingsField = iota
	SettingsFieldNtfyTopic
	SettingsFieldNtfyServer
)

const notificationsFieldCount = 3

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
	theme     config.Theme
	isDark    bool
	isGitRepo bool
	colors    ThemeColors

	// Field indices for dropdown-style fields
	workModeIdx        int
	onCompleteIdx      int
	themeIdx           int
	nonGitWorkspaceIdx int

	// Text input state for editable fields
	slackWebhook string
	ntfyTopic    string
	ntfyServer   string
	editingText  bool
	cursorPos    int
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
	theme := loadThemeFromConfig()
	isDark := detectDarkMode(theme)

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

	// Find current indices for dropdown fields
	workModeIdx := 0
	for i, m := range config.ValidWorkModes() {
		if m == cfg.WorkMode {
			workModeIdx = i
			break
		}
	}

	onCompleteIdx := 0
	for i, c := range config.ValidOnCompletes() {
		if c == cfg.OnComplete {
			onCompleteIdx = i
			break
		}
	}

	themeIdx := 0
	themes := []config.Theme{config.ThemeAuto, config.ThemeLight, config.ThemeDark}
	for i, t := range themes {
		if t == cfg.Theme {
			themeIdx = i
			break
		}
	}

	nonGitWorkspaceIdx := 0
	if cfg.NonGitWorkspace == string(config.NonGitWorkspaceCopy) {
		nonGitWorkspaceIdx = 1
	}

	// Initialize text fields
	slackWebhook := ""
	ntfyTopic := ""
	ntfyServer := ""
	if cfg.Notifications != nil {
		if cfg.Notifications.Slack != nil {
			slackWebhook = cfg.Notifications.Slack.Webhook
		}
		if cfg.Notifications.Ntfy != nil {
			ntfyTopic = cfg.Notifications.Ntfy.Topic
			ntfyServer = cfg.Notifications.Ntfy.Server
		}
	}

	return &SettingsUI{
		scope:              SettingsScopeProject,
		globalConfig:       globalCfg,
		projectConfig:      projectCfg,
		inheritConfig:      inheritCfg,
		config:             cfg,
		tab:                SettingsTabGeneral,
		field:              0,
		theme:              theme,
		isDark:             isDark,
		isGitRepo:          isGitRepo,
		colors:             NewThemeColors(isDark),
		workModeIdx:        workModeIdx,
		onCompleteIdx:      onCompleteIdx,
		themeIdx:           themeIdx,
		nonGitWorkspaceIdx: nonGitWorkspaceIdx,
		slackWebhook:       slackWebhook,
		ntfyTopic:          ntfyTopic,
		ntfyServer:         ntfyServer,
	}
}

// Init initializes the settings UI.
func (m *SettingsUI) Init() tea.Cmd {
	if m.theme == config.ThemeAuto {
		return tea.RequestBackgroundColor
	}
	return nil
}

// Update handles messages and updates the model.
func (m *SettingsUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.BackgroundColorMsg:
		if m.theme == config.ThemeAuto {
			m.isDark = msg.IsDark()
			m.colors = NewThemeColors(m.isDark)
			setCachedDarkMode(m.isDark)
		}
		return m, nil

	case tea.KeyMsg:
		// Handle text editing mode
		if m.editingText {
			return m.handleTextInput(msg)
		}

		switch msg.String() {
		case "esc", "ctrl+c":
			m.cancelled = true
			m.done = true
			return m, tea.Quit

		case "enter":
			// If on a text field, enter editing mode
			if m.isTextField() {
				m.editingText = true
				m.cursorPos = len(m.getCurrentTextValue())
				return m, nil
			}
			// Otherwise save and quit
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

		case " ":
			// Toggle for boolean fields
			if m.tab == SettingsTabGeneral && m.field == SettingsFieldVerifyRequired {
				m.config.VerifyRequired = !m.config.VerifyRequired
				return m, nil
			}

		case "i":
			// Toggle inherit for current field (project scope only)
			if m.scope == SettingsScopeProject && m.tab == SettingsTabGeneral {
				m.toggleInheritForCurrentField()
				return m, nil
			}
		}
	}

	return m, nil
}

// toggleInheritForCurrentField toggles the inherit flag for the current field.
func (m *SettingsUI) toggleInheritForCurrentField() {
	if m.inheritConfig == nil || m.scope != SettingsScopeProject {
		return
	}

	switch m.field {
	case SettingsFieldWorkMode:
		m.inheritConfig.WorkMode = !m.inheritConfig.WorkMode
		if m.inheritConfig.WorkMode {
			// Copy global value to project
			m.config.WorkMode = m.globalConfig.WorkMode
			m.updateFieldIndices()
		}
	case SettingsFieldOnComplete:
		m.inheritConfig.OnComplete = !m.inheritConfig.OnComplete
		if m.inheritConfig.OnComplete {
			m.config.OnComplete = m.globalConfig.OnComplete
			m.updateFieldIndices()
		}
	case SettingsFieldTheme:
		m.inheritConfig.Theme = !m.inheritConfig.Theme
		if m.inheritConfig.Theme {
			m.config.Theme = m.globalConfig.Theme
			m.updateFieldIndices()
		}
	case SettingsFieldNonGitWorkspace:
		m.inheritConfig.NonGitWorkspace = !m.inheritConfig.NonGitWorkspace
		if m.inheritConfig.NonGitWorkspace {
			m.config.NonGitWorkspace = m.globalConfig.NonGitWorkspace
			m.updateFieldIndices()
		}
	case SettingsFieldVerifyRequired:
		m.inheritConfig.VerifyRequired = !m.inheritConfig.VerifyRequired
		if m.inheritConfig.VerifyRequired {
			m.config.VerifyRequired = m.globalConfig.VerifyRequired
		}
	case SettingsFieldSelfImprove:
		m.inheritConfig.SelfImprove = !m.inheritConfig.SelfImprove
		if m.inheritConfig.SelfImprove {
			m.config.SelfImprove = m.globalConfig.SelfImprove
		}
	}
}

func (m *SettingsUI) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editingText = false
		return m, nil
	case "enter":
		m.editingText = false
		m.setCurrentTextValue(m.getCurrentTextValue())
		return m, nil
	case "backspace":
		val := m.getCurrentTextValue()
		if m.cursorPos > 0 {
			val = val[:m.cursorPos-1] + val[m.cursorPos:]
			m.cursorPos--
			m.setCurrentTextValue(val)
		}
		return m, nil
	case "delete":
		val := m.getCurrentTextValue()
		if m.cursorPos < len(val) {
			val = val[:m.cursorPos] + val[m.cursorPos+1:]
			m.setCurrentTextValue(val)
		}
		return m, nil
	case "left":
		if m.cursorPos > 0 {
			m.cursorPos--
		}
		return m, nil
	case "right":
		if m.cursorPos < len(m.getCurrentTextValue()) {
			m.cursorPos++
		}
		return m, nil
	case "home", "ctrl+a":
		m.cursorPos = 0
		return m, nil
	case "end", "ctrl+e":
		m.cursorPos = len(m.getCurrentTextValue())
		return m, nil
	default:
		// Insert character
		if len(msg.String()) == 1 {
			val := m.getCurrentTextValue()
			val = val[:m.cursorPos] + msg.String() + val[m.cursorPos:]
			m.cursorPos++
			m.setCurrentTextValue(val)
		}
		return m, nil
	}
}

func (m *SettingsUI) isTextField() bool {
	return m.tab == SettingsTabNotifications
}

func (m *SettingsUI) getCurrentTextValue() string {
	if m.tab != SettingsTabNotifications {
		return ""
	}
	switch m.field {
	case SettingsFieldSlackWebhook:
		return m.slackWebhook
	case SettingsFieldNtfyTopic:
		return m.ntfyTopic
	case SettingsFieldNtfyServer:
		return m.ntfyServer
	}
	return ""
}

func (m *SettingsUI) setCurrentTextValue(val string) {
	if m.tab != SettingsTabNotifications {
		return
	}
	switch m.field {
	case SettingsFieldSlackWebhook:
		m.slackWebhook = val
	case SettingsFieldNtfyTopic:
		m.ntfyTopic = val
	case SettingsFieldNtfyServer:
		m.ntfyServer = val
	}
}

func (m *SettingsUI) currentFieldCount() SettingsField {
	if m.tab == SettingsTabGeneral {
		return generalFieldCount
	}
	return notificationsFieldCount
}

func (m *SettingsUI) handleLeft() {
	if m.tab == SettingsTabGeneral {
		switch m.field {
		case SettingsFieldWorkMode:
			if m.workModeIdx > 0 {
				m.workModeIdx--
			}
		case SettingsFieldOnComplete:
			if m.onCompleteIdx > 0 {
				m.onCompleteIdx--
			}
		case SettingsFieldTheme:
			if m.themeIdx > 0 {
				m.themeIdx--
			}
		case SettingsFieldNonGitWorkspace:
			if m.nonGitWorkspaceIdx > 0 {
				m.nonGitWorkspaceIdx--
			}
		case SettingsFieldVerifyRequired:
			m.config.VerifyRequired = true
		case SettingsFieldSelfImprove:
			m.config.SelfImprove = true
		}
	}
}

func (m *SettingsUI) handleRight() {
	if m.tab == SettingsTabGeneral {
		switch m.field {
		case SettingsFieldWorkMode:
			modes := config.ValidWorkModes()
			if m.workModeIdx < len(modes)-1 {
				m.workModeIdx++
			}
		case SettingsFieldOnComplete:
			completes := config.ValidOnCompletes()
			if m.onCompleteIdx < len(completes)-1 {
				m.onCompleteIdx++
			}
		case SettingsFieldTheme:
			if m.themeIdx < 2 {
				m.themeIdx++
			}
		case SettingsFieldNonGitWorkspace:
			if m.nonGitWorkspaceIdx < 1 {
				m.nonGitWorkspaceIdx++
			}
		case SettingsFieldVerifyRequired:
			m.config.VerifyRequired = false
		case SettingsFieldSelfImprove:
			m.config.SelfImprove = false
		}
	}
}

func (m *SettingsUI) applyChanges() {
	// Apply general settings
	modes := config.ValidWorkModes()
	if m.workModeIdx < len(modes) {
		m.config.WorkMode = modes[m.workModeIdx]
	}

	completes := config.ValidOnCompletes()
	if m.onCompleteIdx < len(completes) {
		m.config.OnComplete = completes[m.onCompleteIdx]
	}

	themes := []config.Theme{config.ThemeAuto, config.ThemeLight, config.ThemeDark}
	if m.themeIdx < len(themes) {
		m.config.Theme = themes[m.themeIdx]
	}

	if m.nonGitWorkspaceIdx == 0 {
		m.config.NonGitWorkspace = string(config.NonGitWorkspaceShared)
	} else {
		m.config.NonGitWorkspace = string(config.NonGitWorkspaceCopy)
	}

	// Apply notification settings
	if m.slackWebhook != "" || m.ntfyTopic != "" {
		if m.config.Notifications == nil {
			m.config.Notifications = &config.NotificationsConfig{}
		}

		if m.slackWebhook != "" {
			if m.config.Notifications.Slack == nil {
				m.config.Notifications.Slack = &config.SlackConfig{}
			}
			m.config.Notifications.Slack.Webhook = m.slackWebhook
		}

		if m.ntfyTopic != "" {
			if m.config.Notifications.Ntfy == nil {
				m.config.Notifications.Ntfy = &config.NtfyConfig{}
			}
			m.config.Notifications.Ntfy.Topic = m.ntfyTopic
			m.config.Notifications.Ntfy.Server = m.ntfyServer
		}
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
	// Work mode
	m.workModeIdx = 0
	for i, mode := range config.ValidWorkModes() {
		if mode == m.config.WorkMode {
			m.workModeIdx = i
			break
		}
	}

	// On complete
	m.onCompleteIdx = 0
	for i, c := range config.ValidOnCompletes() {
		if c == m.config.OnComplete {
			m.onCompleteIdx = i
			break
		}
	}

	// Theme
	m.themeIdx = 0
	themes := []config.Theme{config.ThemeAuto, config.ThemeLight, config.ThemeDark}
	for i, t := range themes {
		if t == m.config.Theme {
			m.themeIdx = i
			break
		}
	}

	// Non-git workspace
	m.nonGitWorkspaceIdx = 0
	if m.config.NonGitWorkspace == string(config.NonGitWorkspaceCopy) {
		m.nonGitWorkspaceIdx = 1
	}

	// Update text fields
	m.slackWebhook = ""
	m.ntfyTopic = ""
	m.ntfyServer = ""
	if m.config.Notifications != nil {
		if m.config.Notifications.Slack != nil {
			m.slackWebhook = m.config.Notifications.Slack.Webhook
		}
		if m.config.Notifications.Ntfy != nil {
			m.ntfyTopic = m.config.Notifications.Ntfy.Topic
			m.ntfyServer = m.config.Notifications.Ntfy.Server
		}
	}
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
		Width(20)

	selectedLabelStyle := lipgloss.NewStyle().
		Foreground(c.Accent).
		Bold(true).
		Width(20)

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

	textInputStyle := lipgloss.NewStyle().
		Foreground(c.TextNormal).
		Background(c.Background).
		Padding(0, 1)

	editingTextStyle := lipgloss.NewStyle().
		Foreground(c.Accent).
		Background(c.Background).
		Padding(0, 1)

	var sb strings.Builder

	// Title with scope indicator
	scopeText := "[Project]"
	if m.scope == SettingsScopeGlobal {
		scopeText = "[Global]"
	}
	sb.WriteString(titleStyle.Render("⚙ Settings " + scopeText))
	sb.WriteString("\n\n")

	// Tabs
	tabs := []string{"General", "Notifications"}
	for i, tab := range tabs {
		if SettingsTab(i) == m.tab {
			sb.WriteString(activeTabStyle.Render(tab))
		} else {
			sb.WriteString(tabStyle.Render(tab))
		}
	}
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(strings.Repeat("─", 50)))
	sb.WriteString("\n\n")

	// Content based on tab
	switch m.tab {
	case SettingsTabGeneral:
		m.renderGeneralTab(&sb, labelStyle, selectedLabelStyle, valueStyle, selectedValueStyle, dimStyle)
	case SettingsTabNotifications:
		m.renderNotificationsTab(&sb, labelStyle, selectedLabelStyle, textInputStyle, editingTextStyle, dimStyle)
	}

	// Help text
	sb.WriteString("\n")
	if m.editingText {
		sb.WriteString(helpStyle.Render("Enter: Confirm  |  Esc: Cancel  |  ←/→: Move cursor"))
	} else if m.scope == SettingsScopeProject && m.tab == SettingsTabGeneral {
		sb.WriteString(helpStyle.Render("⌥Tab: Global/Project  |  i: Toggle inherit  |  ↑/↓: Navigate  |  ←/→: Change  |  ⌃S: Save"))
	} else {
		sb.WriteString(helpStyle.Render("⌥Tab: Global/Project  |  Tab: Switch tab  |  ↑/↓: Navigate  |  ←/→: Change  |  ⌃S: Save  |  Esc: Cancel"))
	}

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

func (m *SettingsUI) renderGeneralTab(sb *strings.Builder, labelStyle, selectedLabelStyle, valueStyle, selectedValueStyle, dimStyle lipgloss.Style) {
	// Helper to render inherit indicator
	inheritIndicator := func(inherited bool) string {
		if m.scope != SettingsScopeProject {
			return ""
		}
		if inherited {
			return dimStyle.Render(" (inherited)")
		}
		return ""
	}

	// Work Mode
	{
		label := labelStyle.Render("Work Mode:")
		if m.field == SettingsFieldWorkMode {
			label = selectedLabelStyle.Render("Work Mode:")
		}

		inherited := m.scope == SettingsScopeProject && m.inheritConfig != nil && m.inheritConfig.WorkMode
		modes := config.ValidWorkModes()
		var parts []string
		for i, mode := range modes {
			if i == m.workModeIdx {
				if m.field == SettingsFieldWorkMode {
					parts = append(parts, selectedValueStyle.Render("["+string(mode)+"]"))
				} else {
					parts = append(parts, valueStyle.Render("["+string(mode)+"]"))
				}
			} else {
				parts = append(parts, dimStyle.Render(" "+string(mode)+" "))
			}
		}
		sb.WriteString(label + strings.Join(parts, "") + inheritIndicator(inherited))
		sb.WriteString("\n")
	}

	// On Complete
	{
		label := labelStyle.Render("On Complete:")
		if m.field == SettingsFieldOnComplete {
			label = selectedLabelStyle.Render("On Complete:")
		}

		inherited := m.scope == SettingsScopeProject && m.inheritConfig != nil && m.inheritConfig.OnComplete
		completes := config.ValidOnCompletes()
		var parts []string
		for i, c := range completes {
			if i == m.onCompleteIdx {
				if m.field == SettingsFieldOnComplete {
					parts = append(parts, selectedValueStyle.Render("["+string(c)+"]"))
				} else {
					parts = append(parts, valueStyle.Render("["+string(c)+"]"))
				}
			} else {
				parts = append(parts, dimStyle.Render(" "+string(c)+" "))
			}
		}
		sb.WriteString(label + strings.Join(parts, "") + inheritIndicator(inherited))
		sb.WriteString("\n")
	}

	// Theme
	{
		label := labelStyle.Render("Theme:")
		if m.field == SettingsFieldTheme {
			label = selectedLabelStyle.Render("Theme:")
		}

		inherited := m.scope == SettingsScopeProject && m.inheritConfig != nil && m.inheritConfig.Theme
		themes := []config.Theme{config.ThemeAuto, config.ThemeLight, config.ThemeDark}
		var parts []string
		for i, t := range themes {
			if i == m.themeIdx {
				if m.field == SettingsFieldTheme {
					parts = append(parts, selectedValueStyle.Render("["+string(t)+"]"))
				} else {
					parts = append(parts, valueStyle.Render("["+string(t)+"]"))
				}
			} else {
				parts = append(parts, dimStyle.Render(" "+string(t)+" "))
			}
		}
		sb.WriteString(label + strings.Join(parts, "") + inheritIndicator(inherited))
		sb.WriteString("\n")
	}

	// Non-Git Workspace
	{
		label := labelStyle.Render("Non-Git Workspace:")
		if m.field == SettingsFieldNonGitWorkspace {
			label = selectedLabelStyle.Render("Non-Git Workspace:")
		}

		inherited := m.scope == SettingsScopeProject && m.inheritConfig != nil && m.inheritConfig.NonGitWorkspace
		workspaces := []string{"shared", "copy"}
		var parts []string
		for i, w := range workspaces {
			if i == m.nonGitWorkspaceIdx {
				if m.field == SettingsFieldNonGitWorkspace {
					parts = append(parts, selectedValueStyle.Render("["+w+"]"))
				} else {
					parts = append(parts, valueStyle.Render("["+w+"]"))
				}
			} else {
				parts = append(parts, dimStyle.Render(" "+w+" "))
			}
		}
		sb.WriteString(label + strings.Join(parts, "") + inheritIndicator(inherited))
		sb.WriteString("\n")
	}

	// Verify Required
	{
		label := labelStyle.Render("Verify Required:")
		if m.field == SettingsFieldVerifyRequired {
			label = selectedLabelStyle.Render("Verify Required:")
		}

		inherited := m.scope == SettingsScopeProject && m.inheritConfig != nil && m.inheritConfig.VerifyRequired
		var onText, offText string
		if m.config.VerifyRequired {
			if m.field == SettingsFieldVerifyRequired {
				onText = selectedValueStyle.Render("[on]")
			} else {
				onText = valueStyle.Render("[on]")
			}
			offText = dimStyle.Render(" off ")
		} else {
			onText = dimStyle.Render(" on ")
			if m.field == SettingsFieldVerifyRequired {
				offText = selectedValueStyle.Render("[off]")
			} else {
				offText = valueStyle.Render("[off]")
			}
		}
		sb.WriteString(label + onText + " " + offText + inheritIndicator(inherited))
		sb.WriteString("\n")
	}

	// Self Improve
	{
		label := labelStyle.Render("Self Improve:")
		if m.field == SettingsFieldSelfImprove {
			label = selectedLabelStyle.Render("Self Improve:")
		}

		inherited := m.scope == SettingsScopeProject && m.inheritConfig != nil && m.inheritConfig.SelfImprove
		var onText, offText string
		if m.config.SelfImprove {
			if m.field == SettingsFieldSelfImprove {
				onText = selectedValueStyle.Render("[on]")
			} else {
				onText = valueStyle.Render("[on]")
			}
			offText = dimStyle.Render(" off ")
		} else {
			onText = dimStyle.Render(" on ")
			if m.field == SettingsFieldSelfImprove {
				offText = selectedValueStyle.Render("[off]")
			} else {
				offText = valueStyle.Render("[off]")
			}
		}
		sb.WriteString(label + onText + " " + offText + inheritIndicator(inherited))
		sb.WriteString("\n")
	}
}

func (m *SettingsUI) renderNotificationsTab(sb *strings.Builder, labelStyle, selectedLabelStyle, textInputStyle, editingTextStyle, dimStyle lipgloss.Style) {
	// Slack Webhook
	{
		label := labelStyle.Render("Slack Webhook:")
		if m.field == SettingsFieldSlackWebhook {
			label = selectedLabelStyle.Render("Slack Webhook:")
		}

		value := m.slackWebhook
		if value == "" {
			value = "(not set)"
		}

		if m.field == SettingsFieldSlackWebhook && m.editingText {
			// Show cursor in editing mode
			display := m.slackWebhook[:m.cursorPos] + "█" + m.slackWebhook[m.cursorPos:]
			sb.WriteString(label + editingTextStyle.Render(display))
		} else if m.field == SettingsFieldSlackWebhook {
			sb.WriteString(label + textInputStyle.Render(value))
		} else {
			sb.WriteString(label + dimStyle.Render(value))
		}
		sb.WriteString("\n")
	}

	// ntfy Topic
	{
		label := labelStyle.Render("ntfy Topic:")
		if m.field == SettingsFieldNtfyTopic {
			label = selectedLabelStyle.Render("ntfy Topic:")
		}

		value := m.ntfyTopic
		if value == "" {
			value = "(not set)"
		}

		if m.field == SettingsFieldNtfyTopic && m.editingText {
			display := m.ntfyTopic[:m.cursorPos] + "█" + m.ntfyTopic[m.cursorPos:]
			sb.WriteString(label + editingTextStyle.Render(display))
		} else if m.field == SettingsFieldNtfyTopic {
			sb.WriteString(label + textInputStyle.Render(value))
		} else {
			sb.WriteString(label + dimStyle.Render(value))
		}
		sb.WriteString("\n")
	}

	// ntfy Server
	{
		label := labelStyle.Render("ntfy Server:")
		if m.field == SettingsFieldNtfyServer {
			label = selectedLabelStyle.Render("ntfy Server:")
		}

		value := m.ntfyServer
		if value == "" {
			value = "(https://ntfy.sh)"
		}

		if m.field == SettingsFieldNtfyServer && m.editingText {
			display := m.ntfyServer[:m.cursorPos] + "█" + m.ntfyServer[m.cursorPos:]
			sb.WriteString(label + editingTextStyle.Render(display))
		} else if m.field == SettingsFieldNtfyServer {
			sb.WriteString(label + textInputStyle.Render(value))
		} else {
			sb.WriteString(label + dimStyle.Render(value))
		}
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
