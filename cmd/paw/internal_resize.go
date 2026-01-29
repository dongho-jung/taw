package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

var resizeFilePickerReason string

var resizeFilePickerCmd = &cobra.Command{
	Use:    "resize-file-picker [session]",
	Short:  "Force file picker pane width (debug)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]
		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		_, cleanup := setupLoggerFromApp(appCtx, "resize-file-picker", "")
		defer cleanup()

		reason := strings.TrimSpace(resizeFilePickerReason)
		if reason == "" {
			reason = "manual"
		}

		tm := tmux.New(sessionName)

		windows, err := tm.ListWindows()
		if err != nil {
			logging.Debug("resize-file-picker: list windows failed: %v", err)
			return nil
		}

		var mainWindowID string
		for _, w := range windows {
			if strings.HasPrefix(w.Name, constants.EmojiNew) {
				mainWindowID = w.ID
				break
			}
		}
		if mainWindowID == "" {
			logging.Debug("resize-file-picker: main window not found")
			return nil
		}

		forceFilePickerWidth(tm, mainWindowID, "resize-file-picker-"+reason)
		return nil
	},
}

func init() {
	resizeFilePickerCmd.Flags().StringVar(&resizeFilePickerReason, "reason", "", "Reason for resizing file picker pane")
}
