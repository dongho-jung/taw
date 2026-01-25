package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

// updateWindowStatus is a helper that handles common hook setup and window status updates.
// It reads env vars, validates them, sets up logging, checks pane existence, and renames the window.
func updateWindowStatus(hookName string, emoji string) error {
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
		_, cleanup := setupLogger(filepath.Join(pawDir, constants.LogFileName), os.Getenv("PAW_DEBUG") == "1", hookName, taskName)
		defer cleanup()
	}

	logging.Debug("-> %s(session=%s, windowID=%s, task=%s)", hookName, sessionName, windowID, taskName)
	defer logging.Debug("<- %s", hookName)

	tm := tmux.New(sessionName)
	paneID := windowID + ".0"
	if !tm.HasPane(paneID) {
		logging.Debug("%s: pane %s not found, skipping", hookName, paneID)
		return nil
	}

	// Update window status
	newName := emoji + constants.TruncateForWindowName(taskName)
	if err := renameWindowCmd.RunE(renameWindowCmd, []string{windowID, newName}); err != nil {
		logging.Warn("%s: failed to rename window: %v", hookName, err)
		return nil
	}

	logging.Debug("%s: status updated", hookName)
	return nil
}

var userPromptSubmitHookCmd = &cobra.Command{
	Use:   "user-prompt-submit-hook",
	Short: "Handle Claude UserPromptSubmit hook to set working status",
	RunE: func(_ *cobra.Command, _ []string) error {
		return updateWindowStatus("userPromptSubmitHookCmd", constants.EmojiWorking)
	},
}

// askUserQuestionPreHookCmd handles the PreToolUse hook for AskUserQuestion.
// This is triggered when Claude calls AskUserQuestion (before showing UI to user).
// Sets status to WAITING immediately when Claude asks a question.
var askUserQuestionPreHookCmd = &cobra.Command{
	Use:   "ask-user-question-pre-hook",
	Short: "Handle Claude PreToolUse hook for AskUserQuestion to set waiting status",
	RunE: func(_ *cobra.Command, _ []string) error {
		return updateWindowStatus("askUserQuestionPreHookCmd", constants.EmojiWaiting)
	},
}

// askUserQuestionHookCmd handles the PostToolUse hook for AskUserQuestion.
// This is triggered when the user responds to an AskUserQuestion tool call
// (e.g., by selecting an option from the UI), which doesn't trigger UserPromptSubmit.
var askUserQuestionHookCmd = &cobra.Command{
	Use:   "ask-user-question-hook",
	Short: "Handle Claude PostToolUse hook for AskUserQuestion to set working status",
	RunE: func(_ *cobra.Command, _ []string) error {
		return updateWindowStatus("askUserQuestionHookCmd", constants.EmojiWorking)
	},
}
