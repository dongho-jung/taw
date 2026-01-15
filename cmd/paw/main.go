// Package main provides the entry point for the PAW CLI.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
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
var forceLocal bool

func init() {
	// Set version for TUI display
	tui.SetVersion(Version)

	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(cleanAllCmd)
	rootCmd.AddCommand(killCmd)
	rootCmd.AddCommand(killAllCmd)
	rootCmd.AddCommand(locationCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(windowMapCmd)
	rootCmd.AddCommand(versionCmd)

	// Internal commands (hidden, called by tmux keybindings)
	rootCmd.AddCommand(internalCmd)

	// Add -v/--version flag to root command
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Print version information")
	rootCmd.Flags().BoolVar(&forceLocal, "local", false, "Force local .paw workspace for git repositories")
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

var cleanAllCmd = &cobra.Command{
	Use:   "clean-all",
	Short: "Clean up all PAW resources across all projects",
	Long: `Clean up all PAW sessions and workspaces across all projects.

Prompts for confirmation before cleaning.

This command:
1. Kills all running PAW tmux sessions
2. Removes all PAW workspaces from ~/.local/share/paw/workspaces/
3. Cleans up git worktrees and branches for each workspace`,
	RunE: runCleanAll,
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

	workspaceMode := config.PawInProjectAuto
	if forceLocal {
		workspaceMode = config.PawInProjectLocal
	}

	// Create app context with appropriate project directory
	// Pass isGitRepo to ensure correct workspace location in auto mode
	application, err := app.NewWithGitInfoWithWorkspace(projectDir, isGitRepo, workspaceMode)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	// Set PAW home
	pawHome, err := getPawHome()
	if err != nil {
		return fmt.Errorf("failed to get PAW home: %w", err)
	}
	application.SetPawHome(pawHome)

	// Initialize .paw directory
	if err := application.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	// Bootstrap logger (stdout) until config loads
	logging.SetGlobal(logging.NewStdout(application.Debug))

	// Ensure config exists (create defaults on first run)
	if !application.HasConfig() {
		if err := config.DefaultConfig().Save(application.PawDir); err != nil {
			return fmt.Errorf("failed to write default config: %w", err)
		}
	}

	// Load configuration
	if err := application.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if application.Config != nil {
		_ = os.Setenv("PAW_LOG_FORMAT", application.Config.LogFormat)
		_ = os.Setenv("PAW_LOG_MAX_SIZE_MB", fmt.Sprintf("%d", application.Config.LogMaxSizeMB))
		_ = os.Setenv("PAW_LOG_MAX_BACKUPS", fmt.Sprintf("%d", application.Config.LogMaxBackups))
	}

	// Setup logging (file) with configured options
	logger, err := logging.New(application.GetLogPath(), application.Debug)
	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}
	defer func() { _ = logger.Close() }()
	logger.SetScript("paw")
	logging.SetGlobal(logger)

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
	return startNewSession(application, tm)
}
