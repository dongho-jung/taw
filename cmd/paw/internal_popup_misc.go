package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea/v2"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var loadingScreenCmd = &cobra.Command{
	Use:    "loading-screen [message]",
	Short:  "Show a loading screen with braille animation",
	Args:   cobra.MaximumNArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> loadingScreenCmd")
		defer logging.Debug("<- loadingScreenCmd")

		message := "Generating task name..."
		if len(args) > 0 {
			message = args[0]
		}

		// Run the spinner TUI
		spinner := tui.NewSpinner(message)
		p := tea.NewProgram(spinner)

		// Block forever until killed (spawn-task will kill the window when done)
		_, err := p.Run()
		return err
	},
}

var toggleSetupCmd = &cobra.Command{
	Use:   "toggle-setup [session]",
	Short: "Toggle setup wizard popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleSetupCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleSetupCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			logging.Debug("toggleSetupCmd: getAppFromSession failed: %v", err)
			return err
		}

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run setup wizard in popup (closes when done)
		// After setup completes, reload-config is called to apply changes
		setupCmd := fmt.Sprintf("%s internal setup-wizard %s", pawBin, sessionName)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     constants.PopupWidthFull,
			Height:    constants.PopupHeightFull,
			Title:     " Setup ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: appCtx.ProjectDir,
		}, setupCmd)
		return nil
	},
}

var setupWizardCmd = &cobra.Command{
	Use:    "setup-wizard [session]",
	Short:  "Run the setup wizard (internal)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> setupWizardCmd(session=%s)", args[0])
		defer logging.Debug("<- setupWizardCmd")

		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			logging.Debug("setupWizardCmd: getAppFromSession failed: %v", err)
			return err
		}

		// Run the setup wizard
		if err := runSetupWizard(appCtx); err != nil {
			logging.Debug("setupWizardCmd: runSetupWizard failed: %v", err)
			return err
		}

		// Reload config and re-apply tmux settings
		if err := appCtx.LoadConfig(); err != nil {
			logging.Debug("setupWizardCmd: LoadConfig failed: %v", err)
			return fmt.Errorf("failed to reload config: %w", err)
		}

		// Re-apply tmux configuration
		tm := tmux.New(sessionName)
		if err := reapplyTmuxConfig(appCtx, tm); err != nil {
			logging.Warn("Failed to re-apply tmux config: %v", err)
		}

		fmt.Println("\n✅ Settings applied!")
		fmt.Println("Press Enter to close...")
		_, _ = fmt.Scanln()

		return nil
	},
}

var toggleCmdPaletteCmd = &cobra.Command{
	Use:   "toggle-cmd-palette [session]",
	Short: "Toggle command palette popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleCmdPaletteCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleCmdPaletteCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run command palette in popup
		paletteCmd := fmt.Sprintf("%s internal cmd-palette-tui %s", pawBin, sessionName)

		// Ignore error - popup returns non-zero when closed with Esc/Ctrl+C
		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:  constants.PopupWidthPalette,
			Height: constants.PopupHeightPalette,
			Title:  "",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, paletteCmd)
		return nil
	},
}

var cmdPaletteTUICmd = &cobra.Command{
	Use:    "cmd-palette-tui [session]",
	Short:  "Run command palette TUI (called from popup)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "cmd-palette-tui", "")
		defer cleanup()

		logging.Debug("-> cmdPaletteTUICmd(session=%s)", sessionName)
		defer logging.Debug("<- cmdPaletteTUICmd")

		// Define available commands
		commands := []tui.Command{
			{
				Name:        "Settings",
				Description: "Configure PAW project settings",
				ID:          "settings",
			},
			{
				Name:        "Restore Panes",
				Description: "Restore missing panes in current task window",
				ID:          "restore-panes",
			},
		}

		logging.Debug("cmdPaletteTUICmd: running command palette")
		action, selected, err := tui.RunCommandPalette(commands)
		if err != nil {
			logging.Debug("cmdPaletteTUICmd: RunCommandPalette failed: %v", err)
			return err
		}

		if action == tui.CommandPaletteCancel || selected == nil {
			logging.Debug("cmdPaletteTUICmd: cancelled or no selection")
			return nil
		}

		logging.Debug("cmdPaletteTUICmd: selected command=%s", selected.ID)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		switch selected.ID {
		case "settings":
			// Queue the settings popup to open after this popup closes
			// Use tmux run-shell -b (background) with a small delay
			logging.Debug("cmdPaletteTUICmd: queuing toggle-settings in background")
			tm := tmux.New(sessionName)
			// Run toggle-settings in background after popup closes
			_ = tm.Run("run-shell", "-b",
				fmt.Sprintf("sleep 0.1 && %s internal toggle-settings %s", pawBin, sessionName))
			// Exit immediately to close this popup
			return nil

		case "restore-panes":
			logging.Debug("cmdPaletteTUICmd: executing restore-panes")
			restoreCmd := exec.Command(pawBin, "internal", "restore-panes", sessionName)
			return restoreCmd.Run()
		}

		return nil
	},
}

var toggleSettingsCmd = &cobra.Command{
	Use:   "toggle-settings [session]",
	Short: "Toggle settings popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "toggle-settings", "")
		defer cleanup()

		logging.Debug("-> toggleSettingsCmd(session=%s)", sessionName)
		defer logging.Debug("<- toggleSettingsCmd")

		tm := tmux.New(sessionName)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run settings in popup
		settingsCmd := fmt.Sprintf("%s internal settings-tui %s", pawBin, sessionName)
		logging.Debug("toggleSettingsCmd: opening popup with cmd=%s", settingsCmd)

		// Ignore error - popup returns non-zero when closed with Esc/Ctrl+C
		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     constants.PopupWidthSettings,
			Height:    constants.PopupHeightSettings,
			Title:     " Settings ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: appCtx.ProjectDir,
		}, settingsCmd)
		return nil
	},
}

var settingsTUICmd = &cobra.Command{
	Use:    "settings-tui [session]",
	Short:  "Run settings TUI (called from popup)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "settings-tui", "")
		defer cleanup()

		logging.Debug("-> settingsTUICmd(session=%s)", sessionName)
		defer logging.Debug("<- settingsTUICmd")

		// Load global config
		globalCfg, err := config.LoadGlobal()
		if err != nil {
			logging.Warn("settingsTUICmd: failed to load global config: %v", err)
			globalCfg = config.DefaultConfig()
		}

		// Run the settings UI with both global and project configs
		logging.Debug("settingsTUICmd: calling tui.RunSettingsUI isGitRepo=%v", appCtx.IsGitRepo)
		result, err := tui.RunSettingsUI(globalCfg, appCtx.Config, appCtx.IsGitRepo)
		if err != nil {
			logging.Debug("settingsTUICmd: RunSettingsUI failed: %v", err)
			return err
		}
		logging.Debug("settingsTUICmd: RunSettingsUI returned cancelled=%v, scope=%d", result.Cancelled, result.Scope)

		if result.Cancelled {
			logging.Debug("settingsTUICmd: cancelled by user")
			return nil
		}

		// Save configs based on scope
		if result.Scope == tui.SettingsScopeGlobal {
			// Ensure global directory exists
			if err := config.EnsureGlobalDir(); err != nil {
				logging.Debug("settingsTUICmd: EnsureGlobalDir failed: %v", err)
				return fmt.Errorf("failed to create global config directory: %w", err)
			}
			// Save global config
			logging.Debug("settingsTUICmd: saving global config to %s", config.GlobalPawDir())
			if err := result.GlobalConfig.Save(config.GlobalPawDir()); err != nil {
				logging.Debug("settingsTUICmd: Save global failed: %v", err)
				return fmt.Errorf("failed to save global config: %w", err)
			}
		} else {
			// Save project config
			logging.Debug("settingsTUICmd: saving project config to %s", appCtx.PawDir)
			if err := result.ProjectConfig.Save(appCtx.PawDir); err != nil {
				logging.Debug("settingsTUICmd: Save project failed: %v", err)
				return fmt.Errorf("failed to save project config: %w", err)
			}
		}

		// Reload config and re-apply tmux settings
		if err := appCtx.LoadConfig(); err != nil {
			logging.Debug("settingsTUICmd: LoadConfig failed: %v", err)
			return fmt.Errorf("failed to reload config: %w", err)
		}

		// Re-apply tmux configuration
		tm := tmux.New(sessionName)
		if err := reapplyTmuxConfig(appCtx, tm); err != nil {
			logging.Warn("Failed to re-apply tmux config: %v", err)
		}

		fmt.Println("\n✅ Settings saved!")
		fmt.Println("Press Enter to close...")
		_, _ = fmt.Scanln()

		return nil
	},
}

var finishPickerTUICmd = &cobra.Command{
	Use:    "finish-picker-tui [session] [window-id]",
	Short:  "Run finish picker TUI (called from popup)",
	Args:   cobra.ExactArgs(2),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "finish-picker-tui", "")
		defer cleanup()

		logging.Debug("-> finishPickerTUICmd(session=%s, windowID=%s)", sessionName, windowID)
		defer logging.Debug("<- finishPickerTUICmd")

		// Run the finish picker
		action, err := tui.RunFinishPicker(appCtx.IsGitRepo)
		if err != nil {
			logging.Debug("finishPickerTUICmd: RunFinishPicker failed: %v", err)
			return err
		}

		logging.Debug("finishPickerTUICmd: selected action=%s", action)

		if action == tui.FinishActionCancel {
			logging.Debug("finishPickerTUICmd: cancelled by user")
			return nil
		}

		// Execute the selected action
		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Map TUI action to end-task action flag
		var endAction string
		switch action {
		case tui.FinishActionMerge:
			endAction = "merge"
		case tui.FinishActionPR:
			endAction = "pr"
		case tui.FinishActionKeep:
			endAction = "keep"
		case tui.FinishActionDrop:
			endAction = "drop"
		default:
			logging.Debug("finishPickerTUICmd: unknown action=%s", action)
			return nil
		}

		// Call end-task-ui with the action flag
		logging.Debug("finishPickerTUICmd: calling end-task-ui with action=%s", endAction)
		endCmd := exec.Command(pawBin, "internal", "end-task-ui", sessionName, windowID, "--action", endAction)
		return endCmd.Run()
	},
}
