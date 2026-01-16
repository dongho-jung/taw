package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

// runCleanAll removes all PAW resources across all projects
func runCleanAll(cmd *cobra.Command, args []string) error {
	// Find all running PAW sessions
	sessions, err := findPawSessions()
	if err != nil {
		return fmt.Errorf("failed to find PAW sessions: %w", err)
	}

	// Find all PAW workspaces in global directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	workspacesDir := filepath.Join(homeDir, ".local", "share", "paw", "workspaces")
	var workspaces []string

	entries, err := os.ReadDir(workspacesDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				workspaces = append(workspaces, filepath.Join(workspacesDir, entry.Name()))
			}
		}
	}

	if len(sessions) == 0 && len(workspaces) == 0 {
		fmt.Println("No PAW sessions or workspaces found.")
		return nil
	}

	// Show what will be cleaned
	if len(sessions) > 0 {
		fmt.Printf("Found %d running PAW session(s):\n", len(sessions))
		for _, s := range sessions {
			fmt.Printf("  - %s\n", s.Name)
		}
	}
	if len(workspaces) > 0 {
		fmt.Printf("Found %d PAW workspace(s):\n", len(workspaces))
		for _, w := range workspaces {
			fmt.Printf("  - %s\n", filepath.Base(w))
		}
	}
	fmt.Println()

	// Confirm before cleaning
	if !confirmPrompt("Clean all PAW resources? [y/N]: ") {
		fmt.Println("Cancelled.")
		return nil
	}

	fmt.Println()

	// Kill all running sessions
	for _, s := range sessions {
		fmt.Printf("Killing session: %s\n", s.Name)
		if err := forceKillSession(s, false); err != nil {
			fmt.Printf("  Warning: failed to kill session: %v\n", err)
		}
	}

	// Clean up each workspace
	for _, wsPath := range workspaces {
		fmt.Printf("Cleaning workspace: %s\n", filepath.Base(wsPath))

		// Try to read project path to clean up git resources
		projectPathFile := filepath.Join(wsPath, ".project-path")
		if data, err := os.ReadFile(projectPathFile); err == nil {
			projectDir := strings.TrimSpace(string(data))
			cleanWorkspaceGitResources(wsPath, projectDir)
		}

		// Remove workspace directory
		if err := os.RemoveAll(wsPath); err != nil {
			fmt.Printf("  Warning: failed to remove workspace: %v\n", err)
		}
	}

	fmt.Println("\nAll PAW resources cleaned successfully.")
	return nil
}

// cleanWorkspaceGitResources cleans up git worktrees and branches for a workspace
func cleanWorkspaceGitResources(wsPath, projectDir string) {
	gitClient := git.New()
	if !gitClient.IsGitRepo(projectDir) {
		return
	}

	agentsDir := filepath.Join(wsPath, "agents")

	// Load config if available
	cfg, err := config.Load(wsPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	mgr := task.NewManager(agentsDir, projectDir, wsPath, true, cfg)

	// Prune stale worktree entries first
	mgr.PruneWorktrees()

	// Clean up tasks
	tasks, _ := mgr.ListTasks()
	for _, t := range tasks {
		fmt.Printf("  Cleaning task: %s\n", t.Name)
		_ = mgr.CleanupTask(t)
	}
}

// runClean removes all PAW resources
func runClean(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// If in git repo, use repo root as project directory
	gitClient := git.New()
	isGitRepo := gitClient.IsGitRepo(cwd)
	projectDir := cwd
	if isGitRepo {
		if repoRoot, err := gitClient.GetRepoRoot(cwd); err == nil {
			projectDir = repoRoot
		}
	}

	// Use NewWithGitInfo to correctly resolve global workspace path
	application, err := app.NewWithGitInfo(projectDir, isGitRepo)
	if err != nil {
		return err
	}

	// Set subdirectory context for correct session name (e.g., finops-treemap)
	if isGitRepo {
		application.SetSubdirectoryContext(cwd, projectDir)
	}

	if err := application.LoadConfig(); err != nil {
		// Config might not exist, continue anyway
		application.Config = config.DefaultConfig()
	}

	tm := tmux.New(application.SessionName)

	fmt.Println("Cleaning up PAW resources...")

	// Kill tmux session if exists
	if tm.HasSession(application.SessionName) {
		fmt.Println("Killing tmux session...")
		_ = tm.KillSession(application.SessionName)
	}

	// Clean up tasks
	if application.IsGitRepo {
		mgr := task.NewManager(application.AgentsDir, application.ProjectDir, application.PawDir, application.IsGitRepo, application.Config)

		// Prune stale worktree entries first to prevent git errors
		mgr.PruneWorktrees()

		tasks, _ := mgr.ListTasks()
		for _, t := range tasks {
			fmt.Printf("Cleaning up task: %s\n", t.Name)
			_ = mgr.CleanupTask(t)
		}
	}

	// Remove .paw directory
	fmt.Println("Removing .paw directory...")
	_ = os.RemoveAll(application.PawDir)

	fmt.Println("Done!")
	return nil
}

// getPawHome returns the PAW installation directory
func getPawHome() (string, error) {
	// Check PAW_HOME env var
	if home := os.Getenv("PAW_HOME"); home != "" {
		return home, nil
	}

	// Get path of current executable
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Resolve symlinks
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}

	// PAW home is the directory containing the binary
	return filepath.Dir(exe), nil
}

// updateGitignore adds .paw gitignore rules if not already present
// Rules: .paw/ (ignore all), !.paw/config (keep config)
func updateGitignore(projectDir string) {
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	// Read existing content
	content, _ := os.ReadFile(gitignorePath)
	contentStr := string(content)

	// Check if proper .paw rules already exist
	// Need: .paw/ + !.paw/config
	lines := strings.Split(contentStr, "\n")
	hasPawIgnore := false
	hasConfigException := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == ".paw" || line == ".paw/" {
			hasPawIgnore = true
		}
		if line == "!.paw/config" {
			hasConfigException = true
		}
	}

	// If all rules exist, nothing to do
	if hasPawIgnore && hasConfigException {
		return
	}

	// Prompt user to add rules (default Y)
	fmt.Print("Add .paw/ gitignore rules (keeps config tracked)? [Y/n]: ")
	var answer string
	_, _ = fmt.Scanln(&answer)
	answer = strings.TrimSpace(strings.ToLower(answer))

	// Default is Y
	if answer != "" && answer != "y" && answer != "yes" {
		return
	}

	// Append missing rules
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	// Ensure there's a newline before our rules
	if len(content) > 0 && content[len(content)-1] != '\n' {
		_, _ = f.WriteString("\n")
	}

	// Add header comment if adding .paw/ for the first time
	if !hasPawIgnore {
		_, _ = f.WriteString("\n# PAW\n")
		_, _ = f.WriteString(".paw/\n")
	}
	if !hasConfigException {
		_, _ = f.WriteString("!.paw/config\n")
	}
}
