// Package embed provides embedded assets for PAW.
package embed

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed assets/*

// Assets contains all embedded files for PAW.
var Assets embed.FS

// GetHelp returns the help content.
func GetHelp() (string, error) {
	data, err := Assets.ReadFile("assets/HELP.md")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetPrompt returns the system prompt content based on git mode.
func GetPrompt(isGitRepo bool) (string, error) {
	filename := "assets/PROMPT-nogit.md"
	if isGitRepo {
		filename = "assets/PROMPT.md"
	}
	data, err := Assets.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetTmuxConfig returns the PAW-specific tmux configuration content.
func GetTmuxConfig() (string, error) {
	data, err := Assets.ReadFile("assets/tmux.conf")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetPawHelp returns the PAW help content for agents.
func GetPawHelp() (string, error) {
	data, err := Assets.ReadFile("assets/HELP-FOR-PAW.md")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WritePawHelpFile writes the HELP-FOR-PAW.md file to the target paw directory.
// This provides agents with guidance on PAW-specific operations.
func WritePawHelpFile(pawDir string) error {
	content, err := GetPawHelp()
	if err != nil {
		return err
	}
	targetPath := filepath.Join(pawDir, "HELP-FOR-PAW.md")
	return os.WriteFile(targetPath, []byte(content), 0644) //nolint:gosec // G306: help file needs to be readable
}

// WriteClaudeFiles writes the embedded claude directory to the target path.
// This copies Claude settings to .paw/.claude/
func WriteClaudeFiles(targetDir string) error {
	// Create target directory
	if err := os.MkdirAll(targetDir, 0755); err != nil { //nolint:gosec // G301: standard directory permissions
		return err
	}

	// Walk through embedded claude directory and copy files
	return fs.WalkDir(Assets, "assets/claude", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path from assets/claude
		relPath, err := filepath.Rel("assets/claude", path)
		if err != nil {
			return err
		}

		// Skip root directory
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755) //nolint:gosec // G301: standard directory permissions
		}

		// Read embedded file
		data, err := Assets.ReadFile(path)
		if err != nil {
			return err
		}

		// Write to target
		return os.WriteFile(targetPath, data, 0644) //nolint:gosec // G306: config files need to be readable
	})
}

// GetDefaultPrompt returns the default prompt content by name.
// Available prompts: task-name, merge-conflict, pr-description, commit-message
func GetDefaultPrompt(name string) (string, error) {
	filename := "assets/prompts/" + name + ".md"
	data, err := Assets.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetTaskNamePrompt returns the default task name generation prompt.
func GetTaskNamePrompt() (string, error) {
	return GetDefaultPrompt("task-name")
}

// GetMergeConflictPrompt returns the default merge conflict resolution prompt.
func GetMergeConflictPrompt() (string, error) {
	return GetDefaultPrompt("merge-conflict")
}

// GetPRDescriptionPrompt returns the default PR description template.
func GetPRDescriptionPrompt() (string, error) {
	return GetDefaultPrompt("pr-description")
}

// GetCommitMessagePrompt returns the default commit message template.
func GetCommitMessagePrompt() (string, error) {
	return GetDefaultPrompt("commit-message")
}

// WriteDefaultPrompt writes a default prompt to the target directory if it doesn't exist.
// Returns the path to the prompt file.
func WriteDefaultPrompt(promptsDir, name string) (string, error) {
	targetPath := filepath.Join(promptsDir, name+".md")

	// Don't overwrite existing file
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	}

	// Create prompts directory if needed
	if err := os.MkdirAll(promptsDir, 0755); err != nil { //nolint:gosec // G301: standard directory permissions
		return "", err
	}

	// Get default content
	content, err := GetDefaultPrompt(name)
	if err != nil {
		return "", err
	}

	// Write file
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil { //nolint:gosec // G306: prompt files need to be readable
		return "", err
	}

	return targetPath, nil
}

// GetPreCommitHook returns the pre-commit hook content.
// This hook prevents .claude symlink from being committed.
func GetPreCommitHook() ([]byte, error) {
	return Assets.ReadFile("assets/hooks/pre-commit")
}

// InstallPreCommitHook installs the pre-commit hook to the target git hooks directory.
// If a pre-commit hook already exists, it appends the PAW hook content.
func InstallPreCommitHook(hooksDir string) error {
	hookPath := filepath.Join(hooksDir, "pre-commit")

	// Get PAW hook content
	pawHook, err := GetPreCommitHook()
	if err != nil {
		return err
	}

	// Create hooks directory if needed
	if err := os.MkdirAll(hooksDir, 0755); err != nil { //nolint:gosec // G301: standard directory permissions
		return err
	}

	// Check if pre-commit hook already exists
	existingContent, err := os.ReadFile(hookPath) //nolint:gosec // G304: hookPath is constructed from gitDir
	if err == nil {
		// Check if PAW hook is already installed
		if containsPawHook(existingContent) {
			return nil // Already installed
		}
		// Append PAW hook to existing hook
		existingContent = append(existingContent, '\n')
		existingContent = append(existingContent, pawHook...)
		newContent := existingContent
		return os.WriteFile(hookPath, newContent, 0755) //nolint:gosec // G306: hook needs to be executable
	}

	// No existing hook, write PAW hook directly
	return os.WriteFile(hookPath, pawHook, 0755) //nolint:gosec // G306: hook needs to be executable
}

// containsPawHook checks if the PAW pre-commit hook is already in the content.
func containsPawHook(content []byte) bool {
	return bytes.Contains(content, []byte("PAW pre-commit hook")) ||
		bytes.Contains(content, []byte("PAW: Automatically unstaged .claude"))
}
