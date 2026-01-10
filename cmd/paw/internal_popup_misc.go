package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea/v2"

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

		return tm.DisplayPopup(tmux.PopupOpts{
			Width:  constants.PopupWidthPalette,
			Height: constants.PopupHeightPalette,
			Title:  "",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, paletteCmd)
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
		logger, _ := logging.New(appCtx.GetLogPath(), appCtx.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("cmd-palette-tui")
			logging.SetGlobal(logger)
		}

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
				Name:        "Show Diff",
				Description: "Show diff between task branch and main",
				ID:          "show-diff",
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
			logging.Debug("cmdPaletteTUICmd: executing toggle-settings")
			settingsCmd := exec.Command(pawBin, "internal", "toggle-settings", sessionName)
			if err := settingsCmd.Run(); err != nil {
				logging.Debug("cmdPaletteTUICmd: toggle-settings failed: %v", err)
				return err
			}
			return nil
		case "show-diff":
			// Queue the diff popup to open after this popup closes
			// Use tmux run-shell -b (background) with a small delay
			if !appCtx.IsGitRepo {
				fmt.Println("Not a git repository")
				return nil
			}

			tm := tmux.New(sessionName)
			// Run toggle-show-diff in background after popup closes
			_ = tm.Run("run-shell", "-b",
				fmt.Sprintf("sleep 0.1 && %s internal toggle-show-diff %s", pawBin, sessionName))
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
		logger, _ := logging.New(appCtx.GetLogPath(), appCtx.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("toggle-settings")
			logging.SetGlobal(logger)
		}

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

		err = tm.DisplayPopup(tmux.PopupOpts{
			Width:     "70%",
			Height:    "60%",
			Title:     " Settings ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: appCtx.ProjectDir,
		}, settingsCmd)
		if err != nil {
			logging.Debug("toggleSettingsCmd: DisplayPopup failed: %v", err)
		}
		return err
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
		logger, _ := logging.New(appCtx.GetLogPath(), appCtx.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("settings-tui")
			logging.SetGlobal(logger)
		}

		logging.Debug("-> settingsTUICmd(session=%s)", sessionName)
		defer logging.Debug("<- settingsTUICmd")

		// Run the settings UI
		logging.Debug("settingsTUICmd: calling tui.RunSettingsUI isGitRepo=%v", appCtx.IsGitRepo)
		result, err := tui.RunSettingsUI(appCtx.Config, appCtx.IsGitRepo)
		if err != nil {
			logging.Debug("settingsTUICmd: RunSettingsUI failed: %v", err)
			return err
		}
		logging.Debug("settingsTUICmd: RunSettingsUI returned cancelled=%v", result.Cancelled)

		if result.Cancelled {
			logging.Debug("settingsTUICmd: cancelled by user")
			return nil
		}

		// Save the config
		logging.Debug("settingsTUICmd: saving config to %s", appCtx.PawDir)
		if err := result.Config.Save(appCtx.PawDir); err != nil {
			logging.Debug("settingsTUICmd: Save failed: %v", err)
			return fmt.Errorf("failed to save config: %w", err)
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
