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

	application, err := app.New(projectDir)
	if err != nil {
		return err
	}

	application.SetGitRepo(isGitRepo)

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

// runSetup runs the setup wizard
func runSetup(cmd *cobra.Command, args []string) error {
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

	application, err := app.New(projectDir)
	if err != nil {
		return err
	}

	application.SetGitRepo(isGitRepo)

	// Initialize .paw directory
	if err := application.Initialize(); err != nil {
		return err
	}

	return runSetupWizard(application)
}

// runSetupWizard runs the interactive setup wizard
func runSetupWizard(appCtx *app.App) error {
	cfg := config.DefaultConfig()

	fmt.Println("\n🚀 PAW Setup Wizard")

	// Work mode (only for git repos)
	if appCtx.IsGitRepo {
		fmt.Println("Work Mode:")
		fmt.Println("  1. worktree (Recommended) - Each task gets its own git worktree")
		fmt.Println("  2. main - All tasks work on current branch")
		fmt.Print("\nSelect [1-2, default: 1]: ")

		var choice string
		_, _ = fmt.Scanln(&choice)

		switch choice {
		case "2":
			cfg.WorkMode = config.WorkModeMain
		default:
			cfg.WorkMode = config.WorkModeWorktree
		}
	}

	// When task completes
	fmt.Println("\nWhen Task Completes:")
	fmt.Println("  1. confirm (Recommended) - Commit only (no push/PR/merge)")

	// Show merge/PR options only in worktree mode
	if cfg.WorkMode == config.WorkModeWorktree {
		fmt.Println("  2. auto-pr - Auto commit + push + create pull request")
		fmt.Println("  3. auto-merge - Auto commit + push + merge + cleanup")
		fmt.Print("\nSelect [1-3, default: 1]: ")
	} else {
		fmt.Print("\nSelect [1, default: 1]: ")
	}

	var choice string
	_, _ = fmt.Scanln(&choice)

	switch choice {
	case "2":
		if cfg.WorkMode == config.WorkModeWorktree {
			cfg.OnComplete = config.OnCompleteAutoPR
		} else {
			cfg.OnComplete = config.OnCompleteConfirm // Invalid in main mode, default to confirm
		}
	case "3":
		if cfg.WorkMode == config.WorkModeWorktree {
			cfg.OnComplete = config.OnCompleteAutoMerge
		} else {
			cfg.OnComplete = config.OnCompleteConfirm // Invalid in main mode, default to confirm
		}
	default:
		cfg.OnComplete = config.OnCompleteConfirm
	}

	// Save configuration
	if err := cfg.Save(appCtx.PawDir); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("\n✅ Configuration saved!")
	fmt.Printf("   Work mode: %s\n", cfg.WorkMode)
	fmt.Printf("   On complete: %s\n", cfg.OnComplete)
	fmt.Printf("   Workspace: %s\n", appCtx.PawDir)

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
// Rules: .paw/ (ignore all), !.paw/config (keep config), !.paw/memory (keep memory)
func updateGitignore(projectDir string) {
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	// Read existing content
	content, _ := os.ReadFile(gitignorePath)
	contentStr := string(content)

	// Check if proper .paw rules already exist
	// Need: .paw/ + !.paw/config + !.paw/memory
	lines := strings.Split(contentStr, "\n")
	hasPawIgnore := false
	hasConfigException := false
	hasMemoryException := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == ".paw" || line == ".paw/" {
			hasPawIgnore = true
		}
		if line == "!.paw/config" {
			hasConfigException = true
		}
		if line == "!.paw/memory" {
			hasMemoryException = true
		}
	}

	// If all rules exist, nothing to do
	if hasPawIgnore && hasConfigException && hasMemoryException {
		return
	}

	// Prompt user to add rules (default Y)
	fmt.Print("Add .paw/ gitignore rules (keeps config and memory tracked)? [Y/n]: ")
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
	if !hasMemoryException {
		_, _ = f.WriteString("!.paw/memory\n")
	}
}
