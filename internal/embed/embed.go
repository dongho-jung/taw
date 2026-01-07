// Package embed provides embedded assets for PAW.
package embed

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed assets/*
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

// WriteClaudeFiles writes the embedded claude directory to the target path.
// This copies Claude settings to .paw/.claude/
func WriteClaudeFiles(targetDir string) error {
	// Create target directory
	if err := os.MkdirAll(targetDir, 0755); err != nil {
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
			return os.MkdirAll(targetPath, 0755)
		}

		// Read embedded file
		data, err := Assets.ReadFile(path)
		if err != nil {
			return err
		}

		// Write to target
		return os.WriteFile(targetPath, data, 0644)
	})
}
