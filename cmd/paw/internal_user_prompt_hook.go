package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

var userPromptSubmitHookCmd = &cobra.Command{
	Use:   "user-prompt-submit-hook",
	Short: "Handle Claude UserPromptSubmit hook to set working status",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := os.Getenv("SESSION_NAME")
		windowID := os.Getenv("WINDOW_ID")
		taskName := os.Getenv("TASK_NAME")

		// Validate required environment variables
		if err := validateRequiredParams(map[string]string{
			"SESSION_NAME": sessionName,
			"WINDOW_ID":    windowID,
			"TASK_NAME":    taskName,
		}); err != nil {
			return nil
		}

		// Setup logging if PAW_DIR is available
		if pawDir := os.Getenv("PAW_DIR"); pawDir != "" {
			_, cleanup := setupLogger(filepath.Join(pawDir, constants.LogFileName), os.Getenv("PAW_DEBUG") == "1", "user-prompt-submit-hook", taskName)
			defer cleanup()
		}

		logging.Trace("userPromptSubmitHookCmd: start session=%s windowID=%s task=%s", sessionName, windowID, taskName)
		defer logging.Trace("userPromptSubmitHookCmd: end")

		tm := tmux.New(sessionName)
		paneID := windowID + ".0"
		if !tm.HasPane(paneID) {
			logging.Debug("userPromptSubmitHookCmd: pane %s not found, skipping", paneID)
			return nil
		}

		// Set window to working state (user submitted a prompt, agent is now working)
		newName := constants.EmojiWorking + constants.TruncateForWindowName(taskName)
		if err := renameWindowCmd.RunE(renameWindowCmd, []string{windowID, newName}); err != nil {
			logging.Warn("userPromptSubmitHookCmd: failed to rename window: %v", err)
			return nil
		}

		logging.Debug("userPromptSubmitHookCmd: status updated to working")
		return nil
	},
}

// askUserQuestionHookCmd handles the PostToolUse hook for AskUserQuestion.
// This is triggered when the user responds to an AskUserQuestion tool call
// (e.g., by selecting an option from the UI), which doesn't trigger UserPromptSubmit.
var askUserQuestionHookCmd = &cobra.Command{
	Use:   "ask-user-question-hook",
	Short: "Handle Claude PostToolUse hook for AskUserQuestion to set working status",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := os.Getenv("SESSION_NAME")
		windowID := os.Getenv("WINDOW_ID")
		taskName := os.Getenv("TASK_NAME")

		// Validate required environment variables
		if err := validateRequiredParams(map[string]string{
			"SESSION_NAME": sessionName,
			"WINDOW_ID":    windowID,
			"TASK_NAME":    taskName,
		}); err != nil {
			return nil
		}

		// Setup logging if PAW_DIR is available
		if pawDir := os.Getenv("PAW_DIR"); pawDir != "" {
			_, cleanup := setupLogger(filepath.Join(pawDir, constants.LogFileName), os.Getenv("PAW_DEBUG") == "1", "ask-user-question-hook", taskName)
			defer cleanup()
		}

		logging.Trace("askUserQuestionHookCmd: start session=%s windowID=%s task=%s", sessionName, windowID, taskName)
		defer logging.Trace("askUserQuestionHookCmd: end")

		tm := tmux.New(sessionName)
		paneID := windowID + ".0"
		if !tm.HasPane(paneID) {
			logging.Debug("askUserQuestionHookCmd: pane %s not found, skipping", paneID)
			return nil
		}

		// Set window to working state (user answered AskUserQuestion, agent is now working)
		newName := constants.EmojiWorking + constants.TruncateForWindowName(taskName)
		if err := renameWindowCmd.RunE(renameWindowCmd, []string{windowID, newName}); err != nil {
			logging.Warn("askUserQuestionHookCmd: failed to rename window: %v", err)
			return nil
		}

		logging.Debug("askUserQuestionHookCmd: status updated to working")
		return nil
	},
}
