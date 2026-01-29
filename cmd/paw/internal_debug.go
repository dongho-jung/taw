package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

var logPaneLayoutReason string

var logPaneLayoutCmd = &cobra.Command{
	Use:    "log-pane-layout [session]",
	Short:  "Log pane layout for debugging",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]
		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		_, cleanup := setupLoggerFromApp(appCtx, "log-pane-layout", "")
		defer cleanup()

		reason := strings.TrimSpace(logPaneLayoutReason)
		if reason == "" {
			reason = "manual"
		}

		tm := tmux.New(sessionName)

		logging.Debug("PaneLayout: reason=%s session=%s", reason, sessionName)

		clientInfo, err := tm.DisplayMultiple("#{client_name}", "#{client_width}", "#{client_height}")
		if err == nil && len(clientInfo) == 3 {
			logging.Debug("PaneLayout: client name=%s size=%sx%s", clientInfo[0], clientInfo[1], clientInfo[2])
		}

		optionVal, err := tm.GetOption(filePickerPaneIDKey)
		if err == nil {
			logging.Debug("PaneLayout: %s=%s", filePickerPaneIDKey, strings.TrimSpace(optionVal))
		} else {
			logging.Debug("PaneLayout: %s read failed: %v", filePickerPaneIDKey, err)
		}

		windows, err := tm.ListWindows()
		if err != nil {
			logging.Debug("PaneLayout: list windows failed: %v", err)
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
			logging.Debug("PaneLayout: main window not found")
			return nil
		}

		windowInfo, err := tm.RunWithOutput("display-message", "-t", mainWindowID, "-p",
			"#{window_id}\t#{window_name}\t#{window_width}\t#{window_height}\t#{window_layout}")
		if err != nil {
			logging.Debug("PaneLayout: window info failed: %v", err)
		} else {
			logging.Debug("PaneLayout: window=%s", windowInfo)
		}

		panesInfo, err := tm.RunWithOutput("list-panes", "-t", mainWindowID, "-F",
			"#{pane_id}\tidx=#{pane_index}\tleft=#{pane_left}\twidth=#{pane_width}\ttop=#{pane_top}\theight=#{pane_height}\tactive=#{pane_active}")
		if err != nil {
			logging.Debug("PaneLayout: list panes failed: %v", err)
			return nil
		}

		for _, line := range strings.Split(strings.TrimSpace(panesInfo), "\n") {
			if strings.TrimSpace(line) != "" {
				logging.Debug("PaneLayout: pane %s", line)
			}
		}

		leftmostID, err := findLeftmostPaneID(tm, mainWindowID)
		if err != nil {
			logging.Debug("PaneLayout: leftmost lookup failed: %v", err)
		} else if leftmostID != "" {
			logging.Debug("PaneLayout: leftmost pane=%s", leftmostID)
		}

		return nil
	},
}

func init() {
	logPaneLayoutCmd.Flags().StringVar(&logPaneLayoutReason, "reason", "", "Reason for logging pane layout")
}
