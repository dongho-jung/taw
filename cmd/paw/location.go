package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/git"
)

var locationCmd = &cobra.Command{
	Use:   "location",
	Short: "Show workspace location for the current project",
	Long: `Show where PAW stores workspace data for the current project.

By default, PAW stores git project workspaces in ~/.local/share/paw/workspaces/{project-id}/
to avoid modifying project .gitignore files, and uses .paw/ for non-git projects.

Use 'paw --local' to force a local .paw workspace for git projects.`,
	RunE: runLocation,
}

func runLocation(_ *cobra.Command, _ []string) error {
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Detect git repo - if in a git repo, use repo root as project dir
	gitClient := git.New()
	isGitRepo := gitClient.IsGitRepo(cwd)
	projectDir := cwd
	if isGitRepo {
		if repoRoot, err := gitClient.GetRepoRoot(cwd); err == nil {
			projectDir = repoRoot
		}
	}

	// Create app context to get workspace path
	application, err := app.NewWithGitInfo(projectDir, isGitRepo)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	// Print workspace location
	fmt.Println(application.PawDir)

	// Show additional info if verbose mode or workspace not initialized
	if !application.IsInitialized() {
		fmt.Fprintf(os.Stderr, "(workspace not initialized - run 'paw' to initialize)\n")
	}

	if application.IsGlobalWorkspace() {
		fmt.Fprintf(os.Stderr, "(global workspace; auto mode for git projects)\n")
		if application.IsGitRepo {
			fmt.Fprintf(os.Stderr, "Tip: run `paw --local` to force a local .paw workspace.\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "(local workspace in project directory)\n")
	}

	return nil
}
