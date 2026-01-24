package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var togglePromptPickerCmd = &cobra.Command{
	Use:   "toggle-prompt-picker [session]",
	Short: "Toggle prompt picker top pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> togglePromptPickerCmd(session=%s)", args[0])
		defer logging.Debug("<- togglePromptPickerCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run prompt picker in top pane
		pickerCmd := shellJoin(pawBin, "internal", "prompt-picker-tui", sessionName)

		result, err := displayTopPane(tm, "prompt", pickerCmd, "")
		if err != nil {
			logging.Debug("togglePromptPickerCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("togglePromptPickerCmd: blocked by another top pane")
		}
		return nil
	},
}

var promptPickerTUICmd = &cobra.Command{
	Use:    "prompt-picker-tui [session]",
	Short:  "Run prompt picker TUI (called from popup)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "prompt-picker-tui", "")
		defer cleanup()

		logging.Debug("-> promptPickerTUICmd(session=%s)", sessionName)
		defer logging.Debug("<- promptPickerTUICmd")

		// Build list of prompts
		prompts := buildPromptList(appCtx.PawDir, appCtx.ProjectDir)

		logging.Debug("promptPickerTUICmd: running prompt picker with %d prompts", len(prompts))
		action, selected, err := tui.RunPromptPicker(prompts)
		if err != nil {
			logging.Debug("promptPickerTUICmd: RunPromptPicker failed: %v", err)
			return err
		}

		if action == tui.PromptPickerCancel || selected == nil {
			logging.Debug("promptPickerTUICmd: cancelled or no selection")
			return nil
		}

		logging.Debug("promptPickerTUICmd: selected prompt=%s", selected.ID)

		// Ensure the prompt file exists
		promptPath, err := ensurePromptFile(appCtx.PawDir, appCtx.ProjectDir, selected)
		if err != nil {
			logging.Warn("Failed to ensure prompt file: %v", err)
			return err
		}

		// Open in editor
		editor := getEditor()
		logging.Debug("promptPickerTUICmd: opening %s in %s", promptPath, editor)

		editorCmd := exec.Command(editor, promptPath)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr
		return editorCmd.Run()
	},
}

// buildPromptList creates the list of available prompts.
func buildPromptList(pawDir, projectDir string) []tui.PromptEntry {
	prompts := []tui.PromptEntry{
		{
			ID:          "task-prompt",
			Name:        "Task Prompt",
			Description: "Project-specific instructions for agents",
			Scope:       "workspace",
		},
		{
			ID:          "system-prompt",
			Name:        "System Prompt",
			Description: "Global system prompt for all tasks",
			Scope:       "workspace",
		},
		{
			ID:          "task-name",
			Name:        "Task Name Rules",
			Description: "Rules for generating task names",
			Scope:       "workspace",
		},
		{
			ID:          "merge-conflict",
			Name:        "Merge Conflict Resolution",
			Description: "Prompt for Claude when resolving merge conflicts",
			Scope:       "workspace",
		},
		{
			ID:          "pr-description",
			Name:        "PR Description Template",
			Description: "Template for PR title and body",
			Scope:       "workspace",
		},
		{
			ID:          "commit-message",
			Name:        "Commit Message Template",
			Description: "Template for auto-commit messages",
			Scope:       "workspace",
		},
	}

	// Check which files exist and set paths
	promptsDir := filepath.Join(pawDir, constants.PromptsDirName)
	for i := range prompts {
		var path string
		switch prompts[i].ID {
		case "task-prompt":
			path = filepath.Join(pawDir, constants.PromptFileName)
		case "system-prompt":
			path = filepath.Join(promptsDir, constants.SystemPromptFile)
		case "task-name":
			path = filepath.Join(promptsDir, constants.TaskNamePromptFile)
		case "merge-conflict":
			path = filepath.Join(promptsDir, constants.MergeConflictPromptFile)
		case "pr-description":
			path = filepath.Join(promptsDir, constants.PRDescriptionPromptFile)
		case "commit-message":
			path = filepath.Join(promptsDir, constants.CommitMessagePromptFile)
		}

		if _, err := os.Stat(path); err == nil {
			prompts[i].Path = path
		}
	}

	return prompts
}

// ensurePromptFile ensures the prompt file exists, creating it with defaults if needed.
func ensurePromptFile(pawDir, projectDir string, prompt *tui.PromptEntry) (string, error) {
	promptsDir := filepath.Join(pawDir, constants.PromptsDirName)

	switch prompt.ID {
	case "task-prompt":
		// PROMPT.md in paw dir
		path := filepath.Join(pawDir, constants.PromptFileName)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// Create with default content
			content := `# Project Prompt

Add project-specific instructions here. These will be appended to the system prompt.

## Examples

- Coding style preferences
- Framework-specific guidelines
- Testing requirements
- Documentation standards
`
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return "", err
			}
		}
		return path, nil

	case "system-prompt":
		// Copy embedded system prompt to prompts/system.md
		path := filepath.Join(promptsDir, constants.SystemPromptFile)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.MkdirAll(promptsDir, 0755); err != nil {
				return "", err
			}
			// Get embedded system prompt (git mode by default)
			content, err := embed.GetPrompt(true)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return "", err
			}
		}
		return path, nil

	case "task-name":
		return embed.WriteDefaultPrompt(promptsDir, "task-name")

	case "merge-conflict":
		return embed.WriteDefaultPrompt(promptsDir, "merge-conflict")

	case "pr-description":
		return embed.WriteDefaultPrompt(promptsDir, "pr-description")

	case "commit-message":
		return embed.WriteDefaultPrompt(promptsDir, "commit-message")

	default:
		return "", fmt.Errorf("unknown prompt: %s", prompt.ID)
	}
}

// getEditor returns the user's preferred editor.
func getEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	// Default to vim
	return "vim"
}
