package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/github"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tui"
)

var prPopupTUICmd = &cobra.Command{
	Use:    "pr-popup-tui [session] [pr-number] [pr-url]",
	Short:  "Run PR popup TUI (called from popup)",
	Args:   cobra.ExactArgs(3),
	Hidden: true,
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]
		prNumberStr := args[1]
		prURL := args[2]

		prNumber, err := strconv.Atoi(prNumberStr)
		if err != nil {
			return fmt.Errorf("invalid PR number: %s", prNumberStr)
		}

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		_, cleanup := setupLoggerFromApp(appCtx, "pr-popup-tui", "")
		defer cleanup()

		logging.Debug("-> prPopupTUICmd(session=%s, pr=%d)", sessionName, prNumber)
		defer logging.Debug("<- prPopupTUICmd")

		openInBrowser, err := tui.RunPRPopup(prURL)
		if err != nil {
			logging.Warn("RunPRPopup failed: %v", err)
			return err
		}

		if !openInBrowser {
			return nil
		}

		ghClient := github.New()
		if !ghClient.IsInstalled() {
			logging.Warn("gh CLI not installed; cannot open PR")
			return nil
		}

		return ghClient.ViewPRWeb(appCtx.ProjectDir, prNumber)
	},
}
