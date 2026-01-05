// Package main provides the entry point for the PAW CLI.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"
	// Commit is the git commit hash, set at build time via ldflags
	Commit = "unknown"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "paw",
	Short: "PAW - Parallel AI Workers",
	Long: `PAW is a Claude Code-based autonomous task execution system.
It manages tasks in tmux sessions with optional git worktree isolation.`,
	RunE:         runMain,
	SilenceUsage: true,
}

var showVersion bool

func init() {
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(versionCmd)

	// Internal commands (hidden, called by tmux keybindings)
	rootCmd.AddCommand(internalCmd)

	// Add -v/--version flag to root command
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Print version information")
}

// checkVersionFlag checks if -v flag was passed and prints version if so
func checkVersionFlag() bool {
	if showVersion {
		printVersion()
		return true
	}
	return false
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

// printVersion prints the version and commit information
func printVersion() {
	fmt.Printf("paw %s (%s)\n", Version, Commit)
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up all PAW resources",
	Long:  "Remove all worktrees, branches, tmux session, and .paw directory",
	RunE:  runClean,
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run the setup wizard",
	Long:  "Configure PAW settings for the current project",
	RunE:  runSetup,
}

// runMain is the main entry point - starts or attaches to a tmux session
func runMain(cmd *cobra.Command, args []string) error {
	// Check for -v flag first
	if checkVersionFlag() {
		return nil
	}

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Detect git repo first - if in a git repo, use repo root as project dir
	// This prevents issues with:
	// 1. Session names containing colons (e.g., "project:src") conflicting with tmux target syntax
	// 2. .paw directory being created in subdirectory instead of repo root
	gitClient := git.New()
	isGitRepo := gitClient.IsGitRepo(cwd)
	projectDir := cwd
	if isGitRepo {
		if repoRoot, err := gitClient.GetRepoRoot(cwd); err == nil {
			projectDir = repoRoot
		}
	}

	// Create app context with appropriate project directory
	application, err := app.New(projectDir)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	// Set PAW home
	pawHome, err := getPawHome()
	if err != nil {
		return fmt.Errorf("failed to get PAW home: %w", err)
	}
	application.SetPawHome(pawHome)

	// Set git repo state
	application.SetGitRepo(isGitRepo)

	// Initialize .paw directory
	if err := application.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	// Setup logging
	logger, err := logging.New(application.GetLogPath(), application.Debug)
	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}
	defer func() { _ = logger.Close() }()
	logger.SetScript("paw")
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
		logging.Debug("Attaching to existing session: %s", application.SessionName)
		return attachToSession(application, tm)
	}

	// Start new session
	logging.Log("=== New session start ===")
	logging.Debug("Project: %s", application.ProjectDir)
	logging.Debug("Session: %s", application.SessionName)
	logging.Debug("Git repo: %v", application.IsGitRepo)
	if application.Config != nil {
		logging.Debug("Config: WorkMode=%s, OnComplete=%s", application.Config.WorkMode, application.Config.OnComplete)
	}
	return startNewSession(application, tm)
}

// startNewSession creates a new tmux session
func startNewSession(app *app.App, tm tmux.Client) error {
	logging.Debug("Starting new tmux session...")

	// Clean up merged tasks before starting new session
	mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)

	// Prune stale worktree entries first to prevent git errors
	mgr.PruneWorktrees()

	merged, err := mgr.FindMergedTasks()
	if err == nil && len(merged) > 0 {
		logging.Log("Found %d merged tasks to clean up", len(merged))
		for _, t := range merged {
			logging.Log("Auto-cleaning merged task: %s", t.Name)
			_ = mgr.CleanupTask(t)
			fmt.Printf("✅ Cleaned up merged task: %s\n", t.Name)
		}
	}

	// Get paw binary path for initial command
	pawBin, err := os.Executable()
	if err != nil {
		pawBin = "paw"
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
		markerPath := filepath.Join(app.PawDir, constants.GitRepoMarker)
		_ = os.WriteFile(markerPath, []byte{}, 0644)
	}

	// Write embedded claude files to .paw/.claude/
	claudeDir := filepath.Join(app.PawDir, constants.ClaudeLink)
	if err := embed.WriteClaudeFiles(claudeDir); err != nil {
		logging.Warn("Failed to write claude files: %v", err)
	}

	// Update .gitignore
	if app.IsGitRepo {
		updateGitignore(app.ProjectDir)
	}

	// Reopen incomplete tasks (tasks with worktree but no window)
	mgr.SetTmuxClient(tm)
	incomplete, err := mgr.FindIncompleteTasks(app.SessionName)
	if err == nil && len(incomplete) > 0 {
		logging.Log("Found %d incomplete tasks to reopen", len(incomplete))
		for _, t := range incomplete {
			logging.Log("Reopening incomplete task: %s", t.Name)
			// Remove old tab-lock so handle-task can create a new one
			_ = t.RemoveTabLock()
			// Re-run handle-task to create window and restart Claude
			handleCmd := exec.Command(pawBin, "internal", "handle-task", app.SessionName, t.AgentDir)
			if err := handleCmd.Start(); err != nil {
				logging.Warn("Failed to reopen task %s: %v", t.Name, err)
			} else {
				fmt.Printf("🔄 Reopening task: %s\n", t.Name)
			}
		}
	}

	// Wait for shell to be ready before sending keys
	// This prevents the race condition where keys are lost if sent before shell initializes
	paneTarget := app.SessionName + ":" + constants.NewWindowName + ".0"
	if err := tm.WaitForPane(paneTarget, 5*time.Second, 1); err != nil {
		logging.Warn("WaitForPane timed out, continuing anyway: %v", err)
	}

	// Send new-task command to the _ window
	// Use SendKeysLiteral for the command and SendKeys for Enter
	newTaskCmd := fmt.Sprintf("%s internal new-task %s", pawBin, app.SessionName)
	_ = tm.SendKeysLiteral(app.SessionName+":"+constants.NewWindowName, newTaskCmd)
	_ = tm.SendKeys(app.SessionName+":"+constants.NewWindowName, "Enter")

	// Attach to session
	return tm.AttachSession(app.SessionName)
}

// attachToSession attaches to an existing session
func attachToSession(app *app.App, tm tmux.Client) error {
	logging.Debug("Running pre-attach cleanup and recovery...")

	// Run cleanup and recovery before attaching
	mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
	mgr.SetTmuxClient(tm)

	// Prune stale worktree entries first to prevent git errors
	mgr.PruneWorktrees()

	// Auto cleanup merged tasks
	merged, err := mgr.FindMergedTasks()
	if err == nil && len(merged) > 0 {
		logging.Log("Found %d merged tasks to clean up", len(merged))

		// Build a map of task name -> window ID for quick lookup
		taskWindowMap := make(map[string]string)
		if windows, err := tm.ListWindows(); err == nil {
			for _, w := range windows {
				if taskName, ok := constants.ExtractTaskName(w.Name); ok {
					taskWindowMap[taskName] = w.ID
				}
			}
		}

		for _, t := range merged {
			logging.Log("Auto-cleaning merged task: %s", t.Name)
			// Close window first if exists (by tab-lock or by matching window name)
			windowKilled := false
			if t.HasTabLock() {
				if windowID, err := t.LoadWindowID(); err == nil && windowID != "" {
					logging.Debug("Killing window %s for merged task %s", windowID, t.Name)
					_ = tm.KillWindow(windowID)
					windowKilled = true
				}
			}
			// If no tab-lock or tab-lock didn't have window ID, try to find by window name
			if !windowKilled {
				if windowID, ok := taskWindowMap[t.Name]; ok {
					logging.Debug("Killing window %s (by name) for merged task %s", windowID, t.Name)
					_ = tm.KillWindow(windowID)
				}
			}
			_ = mgr.CleanupTask(t)
			fmt.Printf("✅ Cleaned up merged task: %s\n", t.Name)
		}
	}

	// Clean up orphaned windows (windows without agent directory)
	orphanedWindows, err := mgr.FindOrphanedWindows()
	if err == nil && len(orphanedWindows) > 0 {
		logging.Log("Found %d orphaned windows to close", len(orphanedWindows))
		for _, windowID := range orphanedWindows {
			logging.Debug("Closing orphaned window: %s", windowID)
			_ = tm.KillWindow(windowID)
			fmt.Printf("✅ Closed orphaned window: %s\n", windowID)
		}
	}

	// Reopen incomplete tasks (tasks with tab-lock but no window)
	incomplete, err := mgr.FindIncompleteTasks(app.SessionName)
	if err == nil && len(incomplete) > 0 {
		logging.Log("Found %d incomplete tasks to reopen", len(incomplete))
		pawBin, _ := os.Executable()
		for _, t := range incomplete {
			logging.Log("Reopening incomplete task: %s (reason: window not found)", t.Name)
			// Remove old tab-lock so handle-task can create a new one
			_ = t.RemoveTabLock()
			// Re-run handle-task to create window and restart Claude
			handleCmd := exec.Command(pawBin, "internal", "handle-task", app.SessionName, t.AgentDir)
			if err := handleCmd.Start(); err != nil {
				logging.Warn("Failed to reopen task %s: %v", t.Name, err)
			} else {
				fmt.Printf("🔄 Reopening task: %s\n", t.Name)
			}
		}
	}

	// Resume stopped agents (windows exist but Claude has exited)
	stopped, err := mgr.FindStoppedTasks()
	if err == nil && len(stopped) > 0 {
		logging.Log("Found %d stopped agents to resume", len(stopped))
		pawBin, _ := os.Executable()
		for _, info := range stopped {
			logging.Log("Resuming stopped agent: %s (window=%s)", info.Task.Name, info.WindowID)
			// Run resume-agent to restart Claude with --continue flag
			resumeCmd := exec.Command(pawBin, "internal", "resume-agent", app.SessionName, info.WindowID, info.Task.AgentDir)
			if err := resumeCmd.Start(); err != nil {
				logging.Warn("Failed to resume agent %s: %v", info.Task.Name, err)
			} else {
				fmt.Printf("🔄 Resuming agent: %s\n", info.Task.Name)
			}
		}
	}

	logging.Debug("Attaching to session: %s", app.SessionName)
	// Attach to session
	return tm.AttachSession(app.SessionName)
}

// reapplyTmuxConfig re-applies tmux configuration after config reload.
// This is a subset of setupTmuxConfig that updates settings that depend on config.
func reapplyTmuxConfig(app *app.App, tm tmux.Client) error {
	// Get path to paw binary
	pawBin, err := os.Executable()
	if err != nil {
		pawBin = "paw"
	}

	// Re-apply keybindings (in case session name changed or for consistency)
	bindings := buildKeybindings(pawBin, app.SessionName)
	for _, b := range bindings {
		if err := tm.Bind(b); err != nil {
			logging.Debug("Failed to bind %s: %v", b.Key, err)
		}
	}

	return nil
}

// setupTmuxConfig configures tmux keybindings and options
func setupTmuxConfig(app *app.App, tm tmux.Client) error {
	// Get path to paw binary
	pawBin, err := os.Executable()
	if err != nil {
		pawBin = "paw"
	}

	// Change prefix to an unused key (M-F12) so C-b is available for toggle-bottom
	// Note: "None" is not a valid tmux key, so we use an obscure key instead
	_ = tm.SetOption("prefix", "M-F12", true)
	_ = tm.SetOption("prefix2", "M-F12", true)

	// Setup status bar
	_ = tm.SetOption("status", "on", true)
	_ = tm.SetOption("status-position", "bottom", true)
	_ = tm.SetOption("status-left", " "+app.SessionName+" ", true)
	_ = tm.SetOption("status-left-length", "30", true)
	_ = tm.SetOption("status-right", " ⌃T:tasks ⌃O:logs ⌃B:shell ⌃/:help ", true)
	_ = tm.SetOption("status-right-length", "100", true)

	// Window status format - removes index numbers (0:, 1:, 2:) and asterisk (*)
	// Current window uses blue background and bold for visual distinction
	_ = tm.SetOption("window-status-format", "#[fg=colour231] #W ", true)
	_ = tm.SetOption("window-status-current-format", "#[fg=colour231,bg=colour24,bold] #W ", true)
	_ = tm.SetOption("window-status-separator", "", true)

	// Pane border styling for visual distinction
	_ = tm.SetOption("pane-border-style", "fg=colour238", true)            // Dim border for inactive panes
	_ = tm.SetOption("pane-active-border-style", "fg=colour39,bold", true) // Bright cyan border for active pane

	// Popup styling
	_ = tm.SetOption("popup-style", "fg=terminal,bg=terminal", true)
	_ = tm.SetOption("popup-border-style", "fg=colour244", true)

	// Enable mouse mode
	_ = tm.SetOption("mouse", "on", true)

	// Enable vi-style copy mode and clipboard integration
	_ = tm.SetOption("mode-keys", "vi", true)
	_ = tm.SetOption("set-clipboard", "on", true)

	// Allow escape sequences to pass through to the terminal (tmux 3.3+)
	// Enables OSC 52 clipboard, terminal images, hyperlinks, etc.
	_ = tm.SetOption("allow-passthrough", "all", true)

	// Auto-copy to system clipboard when mouse selection ends
	// In copy-mode, commands must use "send-keys -X" format
	_ = tm.Bind(tmux.BindOpts{
		Key:     "MouseDragEnd1Pane",
		Command: "send-keys -X copy-pipe-and-cancel 'pbcopy'",
		Table:   "copy-mode-vi",
	})

	// Unbind C-b from root table before setting up keybindings
	// This ensures C-b doesn't act as prefix even if tmux.conf wasn't reloaded
	// (tmux.conf is only loaded when server starts, not on reconnect)
	_ = tm.Run("unbind-key", "-T", "root", "C-b")

	// Setup keybindings (English + Korean layouts)
	bindings := buildKeybindings(pawBin, app.SessionName)
	for _, b := range bindings {
		if err := tm.Bind(b); err != nil {
			logging.Debug("Failed to bind %s: %v", b.Key, err)
		}
	}

	return nil
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
func runSetupWizard(app *app.App) error {
	cfg := config.DefaultConfig()

	fmt.Println("\n🚀 PAW Setup Wizard")

	// Work mode (only for git repos)
	if app.IsGitRepo {
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

	// On complete
	fmt.Println("\nOn Complete Action:")
	fmt.Println("  1. confirm (Recommended) - Ask before each action")
	fmt.Println("  2. auto-commit - Automatically commit changes")

	// Show merge/PR options only in worktree mode
	if cfg.WorkMode == config.WorkModeWorktree {
		fmt.Println("  3. auto-merge - Auto commit + merge + cleanup")
		fmt.Println("  4. auto-pr - Auto commit + create pull request")
		fmt.Print("\nSelect [1-4, default: 1]: ")
	} else {
		fmt.Print("\nSelect [1-2, default: 1]: ")
	}

	var choice string
	_, _ = fmt.Scanln(&choice)

	switch choice {
	case "2":
		cfg.OnComplete = config.OnCompleteAutoCommit
	case "3":
		if cfg.WorkMode == config.WorkModeWorktree {
			cfg.OnComplete = config.OnCompleteAutoMerge
		} else {
			cfg.OnComplete = config.OnCompleteConfirm // Invalid in main mode, default to confirm
		}
	case "4":
		if cfg.WorkMode == config.WorkModeWorktree {
			cfg.OnComplete = config.OnCompleteAutoPR
		} else {
			cfg.OnComplete = config.OnCompleteConfirm // Invalid in main mode, default to confirm
		}
	default:
		cfg.OnComplete = config.OnCompleteConfirm
	}

	// Save configuration
	if err := cfg.Save(app.PawDir); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("\n✅ Configuration saved!")
	fmt.Printf("   Work mode: %s\n", cfg.WorkMode)
	fmt.Printf("   On complete: %s\n", cfg.OnComplete)

	return nil
}
