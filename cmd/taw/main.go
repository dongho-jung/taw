// Package main provides the entry point for the TAW CLI.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/donghojung/taw/internal/app"
	"github.com/donghojung/taw/internal/config"
	"github.com/donghojung/taw/internal/constants"
	"github.com/donghojung/taw/internal/git"
	"github.com/donghojung/taw/internal/logging"
	"github.com/donghojung/taw/internal/task"
	"github.com/donghojung/taw/internal/tmux"
)

var (
	// Version is set at build time
	Version = "dev"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "taw",
	Short: "TAW - Tmux + Agent + Worktree",
	Long: `TAW is a Claude Code-based autonomous task execution system.
It manages tasks in tmux sessions with optional git worktree isolation.`,
	RunE:         runMain,
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(versionCmd)

	// Internal commands (hidden, called by tmux keybindings)
	rootCmd.AddCommand(internalCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("taw version %s\n", Version)
	},
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up all TAW resources",
	Long:  "Remove all worktrees, branches, tmux session, and .taw directory",
	RunE:  runClean,
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run the setup wizard",
	Long:  "Configure TAW settings for the current project",
	RunE:  runSetup,
}

// runMain is the main entry point - starts or attaches to a tmux session
func runMain(cmd *cobra.Command, args []string) error {
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Create app context
	application, err := app.New(cwd)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	// Set TAW home
	tawHome, err := getTawHome()
	if err != nil {
		return fmt.Errorf("failed to get TAW home: %w", err)
	}
	application.SetTawHome(tawHome)

	// Detect git repo
	gitClient := git.New()
	application.SetGitRepo(gitClient.IsGitRepo(cwd))

	// Initialize .taw directory
	if err := application.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	// Setup logging
	logger, err := logging.New(application.GetLogPath(), application.Debug)
	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}
	defer logger.Close()
	logger.SetScript("taw")
	logging.SetGlobal(logger)

	// Check if config exists, run setup if not
	if !application.HasConfig() {
		fmt.Println("No configuration found. Running setup...")
		if err := runSetupWizard(application); err != nil {
			return err
		}
	}

	// Load configuration
	if err := application.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create tmux client
	tm := tmux.New(application.SessionName)

	// Check if session already exists
	if tm.HasSession(application.SessionName) {
		logging.Log("Attaching to existing session: %s", application.SessionName)
		return attachToSession(application, tm)
	}

	// Start new session
	logging.Log("=== New session start ===")
	logging.Log("Project: %s", application.ProjectDir)
	logging.Log("Session: %s", application.SessionName)
	logging.Log("Git repo: %v", application.IsGitRepo)
	if application.Config != nil {
		logging.Log("Config: WorkMode=%s, OnComplete=%s", application.Config.WorkMode, application.Config.OnComplete)
	}
	return startNewSession(application, tm)
}

// startNewSession creates a new tmux session
func startNewSession(app *app.App, tm tmux.Client) error {
	logging.Log("Starting new tmux session...")

	// Clean up merged tasks before starting new session
	mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
	merged, err := mgr.FindMergedTasks()
	if err == nil && len(merged) > 0 {
		logging.Log("Found %d merged tasks to clean up", len(merged))
		for _, t := range merged {
			logging.Log("Auto-cleaning merged task: %s", t.Name)
			mgr.CleanupTask(t)
			fmt.Printf("✅ Cleaned up merged task: %s\n", t.Name)
		}
	}

	// Get taw binary path for initial command
	tawBin, err := os.Executable()
	if err != nil {
		tawBin = "taw"
	}

	// Create session with a shell (not the new-task command directly)
	// This keeps the _ window open after new-task exits
	if err := tm.NewSession(tmux.SessionOpts{
		Name:       app.SessionName,
		StartDir:   app.ProjectDir,
		WindowName: constants.NewWindowName,
		Detached:   true,
	}); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Setup tmux configuration
	if err := setupTmuxConfig(app, tm); err != nil {
		logging.Warn("Failed to setup tmux config: %v", err)
	}

	// Setup git repo marker if applicable
	if app.IsGitRepo {
		markerPath := filepath.Join(app.TawDir, constants.GitRepoMarker)
		os.WriteFile(markerPath, []byte{}, 0644)
	}

	// Setup global prompt symlink
	if err := setupPromptSymlink(app); err != nil {
		logging.Warn("Failed to setup prompt symlink: %v", err)
	}

	// Setup .claude symlink
	if err := setupClaudeSymlink(app); err != nil {
		logging.Warn("Failed to setup claude symlink: %v", err)
	}

	// Update .gitignore
	if app.IsGitRepo {
		updateGitignore(app.ProjectDir)
	}

	// Send new-task command to the _ window
	// Use SendKeysLiteral for the command and SendKeys for Enter
	newTaskCmd := fmt.Sprintf("%s internal new-task %s", tawBin, app.SessionName)
	tm.SendKeysLiteral(app.SessionName+":"+constants.NewWindowName, newTaskCmd)
	tm.SendKeys(app.SessionName+":"+constants.NewWindowName, "Enter")

	// Attach to session
	return tm.AttachSession(app.SessionName)
}

// attachToSession attaches to an existing session
func attachToSession(app *app.App, tm tmux.Client) error {
	logging.Log("Running pre-attach cleanup and recovery...")

	// Run cleanup and recovery before attaching
	mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
	mgr.SetTmuxClient(tm)

	// Auto cleanup merged tasks
	merged, err := mgr.FindMergedTasks()
	if err == nil && len(merged) > 0 {
		logging.Log("Found %d merged tasks to clean up", len(merged))
		for _, t := range merged {
			logging.Log("Auto-cleaning merged task: %s", t.Name)
			// Close window first if exists
			if t.HasTabLock() {
				if windowID, err := t.LoadWindowID(); err == nil && windowID != "" {
					logging.Log("Killing window %s for merged task %s", windowID, t.Name)
					tm.KillWindow(windowID)
				}
			}
			mgr.CleanupTask(t)
			fmt.Printf("✅ Cleaned up merged task: %s\n", t.Name)
		}
	}

	// Clean up orphaned windows (windows without agent directory)
	orphanedWindows, err := mgr.FindOrphanedWindows()
	if err == nil && len(orphanedWindows) > 0 {
		logging.Log("Found %d orphaned windows to close", len(orphanedWindows))
		for _, windowID := range orphanedWindows {
			logging.Log("Closing orphaned window: %s", windowID)
			tm.KillWindow(windowID)
			fmt.Printf("✅ Closed orphaned window: %s\n", windowID)
		}
	}

	// Reopen incomplete tasks
	incomplete, err := mgr.FindIncompleteTasks(app.SessionName)
	if err == nil && len(incomplete) > 0 {
		logging.Log("Found %d incomplete tasks", len(incomplete))
		for _, t := range incomplete {
			logging.Log("Reopening incomplete task: %s (reason: window not found)", t.Name)
			// TODO: Implement reopen logic
		}
	}

	logging.Log("Attaching to session: %s", app.SessionName)
	// Attach to session
	return tm.AttachSession(app.SessionName)
}

// setupTmuxConfig configures tmux keybindings and options
func setupTmuxConfig(app *app.App, tm tmux.Client) error {
	// Get path to taw binary
	tawBin, err := os.Executable()
	if err != nil {
		tawBin = "taw"
	}

	// Setup status bar
	tm.SetOption("status", "on", true)
	tm.SetOption("status-position", "bottom", true)
	tm.SetOption("status-left", "", true)
	tm.SetOption("status-right", " ⌥n:new ⌥e:end ⌥m:merge ⌥p:shell ⌥l:log ⌥u:queue ⌥/:help ⌥q:quit ", true)
	tm.SetOption("status-right-length", "100", true)

	// Window status format - removes index numbers (0:, 1:, 2:) and asterisk (*)
	// Uses colors for visual distinction between current and other windows
	tm.SetOption("window-status-format", "#[fg=colour244] #W ", true)
	tm.SetOption("window-status-current-format", "#[fg=colour255,bg=colour238,bold] #W ", true)
	tm.SetOption("window-status-separator", "", true)

	// Enable mouse mode
	tm.SetOption("mouse", "on", true)

	// Setup keybindings
	bindings := []tmux.BindOpts{
		{Key: "M-Tab", Command: "select-pane -t :.+", NoPrefix: true},
		{Key: "M-Left", Command: "previous-window", NoPrefix: true},
		{Key: "M-Right", Command: "next-window", NoPrefix: true},
		{Key: "M-n", Command: fmt.Sprintf("run-shell '%s internal toggle-new %s'", tawBin, app.SessionName), NoPrefix: true},
		{Key: "M-e", Command: fmt.Sprintf("run-shell '%s internal end-task-ui %s #{window_id}'", tawBin, app.SessionName), NoPrefix: true},
		{Key: "M-m", Command: fmt.Sprintf("run-shell '%s internal merge-completed %s'", tawBin, app.SessionName), NoPrefix: true},
		{Key: "M-p", Command: fmt.Sprintf("run-shell '%s internal popup-shell %s'", tawBin, app.SessionName), NoPrefix: true},
		{Key: "M-u", Command: fmt.Sprintf("run-shell '%s internal quick-task %s'", tawBin, app.SessionName), NoPrefix: true},
		{Key: "M-l", Command: fmt.Sprintf("run-shell '%s internal toggle-log %s'", tawBin, app.SessionName), NoPrefix: true},
		{Key: "M-/", Command: fmt.Sprintf("run-shell '%s internal toggle-help %s'", tawBin, app.SessionName), NoPrefix: true},
		{Key: "M-q", Command: "detach", NoPrefix: true},
	}

	for _, b := range bindings {
		if err := tm.Bind(b); err != nil {
			logging.Debug("Failed to bind %s: %v", b.Key, err)
		}
	}

	return nil
}

// setupPromptSymlink creates the global prompt symlink
func setupPromptSymlink(app *app.App) error {
	linkPath := app.GetGlobalPromptPath()

	// Remove existing symlink
	os.Remove(linkPath)

	// Determine target based on git mode
	var target string
	if app.IsGitRepo {
		target = filepath.Join(app.TawHome, "_taw", "PROMPT.md")
	} else {
		target = filepath.Join(app.TawHome, "_taw", "PROMPT-nogit.md")
	}

	return os.Symlink(target, linkPath)
}

// setupClaudeSymlink creates the .claude symlink
func setupClaudeSymlink(app *app.App) error {
	linkPath := filepath.Join(app.TawDir, constants.ClaudeLink)

	// Remove existing symlink
	os.Remove(linkPath)

	target := filepath.Join(app.TawHome, "_taw", "claude")
	return os.Symlink(target, linkPath)
}

// updateGitignore adds .taw to .gitignore if not already present
func updateGitignore(projectDir string) {
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	// Read existing content
	content, _ := os.ReadFile(gitignorePath)
	contentStr := string(content)

	// Check if .taw is already in .gitignore
	// Look for ".taw" or ".taw/" on its own line
	lines := strings.Split(contentStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == ".taw" || line == ".taw/" {
			return
		}
	}

	// Append .taw
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	if len(content) > 0 && content[len(content)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(".taw/\n")
}

// getTawHome returns the TAW installation directory
func getTawHome() (string, error) {
	// Check TAW_HOME env var
	if home := os.Getenv("TAW_HOME"); home != "" {
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

	// TAW home is the directory containing the binary
	return filepath.Dir(exe), nil
}

// runClean removes all TAW resources
func runClean(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	application, err := app.New(cwd)
	if err != nil {
		return err
	}

	gitClient := git.New()
	application.SetGitRepo(gitClient.IsGitRepo(cwd))

	if err := application.LoadConfig(); err != nil {
		// Config might not exist, continue anyway
		application.Config = config.DefaultConfig()
	}

	tm := tmux.New(application.SessionName)

	fmt.Println("Cleaning up TAW resources...")

	// Kill tmux session if exists
	if tm.HasSession(application.SessionName) {
		fmt.Println("Killing tmux session...")
		tm.KillSession(application.SessionName)
	}

	// Clean up tasks
	if application.IsGitRepo {
		mgr := task.NewManager(application.AgentsDir, application.ProjectDir, application.TawDir, application.IsGitRepo, application.Config)
		tasks, _ := mgr.ListTasks()
		for _, t := range tasks {
			fmt.Printf("Cleaning up task: %s\n", t.Name)
			mgr.CleanupTask(t)
		}
	}

	// Remove .taw directory
	fmt.Println("Removing .taw directory...")
	os.RemoveAll(application.TawDir)

	fmt.Println("Done!")
	return nil
}

// runSetup runs the setup wizard
func runSetup(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	application, err := app.New(cwd)
	if err != nil {
		return err
	}

	gitClient := git.New()
	application.SetGitRepo(gitClient.IsGitRepo(cwd))

	// Initialize .taw directory
	if err := application.Initialize(); err != nil {
		return err
	}

	return runSetupWizard(application)
}

// runSetupWizard runs the interactive setup wizard
func runSetupWizard(app *app.App) error {
	cfg := config.DefaultConfig()

	fmt.Println("\n🚀 TAW Setup Wizard")

	// Work mode (only for git repos)
	if app.IsGitRepo {
		fmt.Println("Work Mode:")
		fmt.Println("  1. worktree (Recommended) - Each task gets its own git worktree")
		fmt.Println("  2. main - All tasks work on current branch")
		fmt.Print("\nSelect [1-2, default: 1]: ")

		var choice string
		fmt.Scanln(&choice)

		switch choice {
		case "2":
			cfg.WorkMode = config.WorkModeMain
		default:
			cfg.WorkMode = config.WorkModeWorktree
		}
	}

	// On complete
	fmt.Println("\nOn Complete Action:")
	fmt.Println("  1. confirm (Recommended) - Ask before each action")
	fmt.Println("  2. auto-commit - Automatically commit changes")
	fmt.Println("  3. auto-merge - Auto commit + merge + cleanup")
	fmt.Println("  4. auto-pr - Auto commit + create pull request")
	fmt.Print("\nSelect [1-4, default: 1]: ")

	var choice string
	fmt.Scanln(&choice)

	switch choice {
	case "2":
		cfg.OnComplete = config.OnCompleteAutoCommit
	case "3":
		cfg.OnComplete = config.OnCompleteAutoMerge
	case "4":
		cfg.OnComplete = config.OnCompleteAutoPR
	default:
		cfg.OnComplete = config.OnCompleteConfirm
	}

	// Save configuration
	if err := cfg.Save(app.TawDir); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("\n✅ Configuration saved!")
	fmt.Printf("   Work mode: %s\n", cfg.WorkMode)
	fmt.Printf("   On complete: %s\n", cfg.OnComplete)

	return nil
}
