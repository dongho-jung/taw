package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/tmux"
	"github.com/dongho-jung/paw/internal/tui"
)

var toggleLogCmd = &cobra.Command{
	Use:   "toggle-log [session]",
	Short: "Toggle log viewer popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleLogCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleLogCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			logging.Debug("toggleLogCmd: getAppFromSession failed: %v", err)
			return err
		}

		logPath := appCtx.GetLogPath()

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run log viewer in popup (closes with q/Esc/Ctrl+L)
		logCmd := fmt.Sprintf("%s internal log-viewer %s", pawBin, logPath)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:  constants.PopupWidthFull,
			Height: constants.PopupHeightFull,
			Title:  " Log Viewer ",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, logCmd)
		return nil
	},
}

var logViewerCmd = &cobra.Command{
	Use:    "log-viewer [logfile]",
	Short:  "Run the log viewer",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logFile := args[0]
		return tui.RunLogViewer(logFile)
	},
}

var toggleHelpCmd = &cobra.Command{
	Use:   "toggle-help [session]",
	Short: "Toggle help popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run help viewer in popup (closes with q/Esc/Ctrl+/)
		helpCmd := fmt.Sprintf("%s internal help-viewer", pawBin)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:  constants.PopupWidthHelp,
			Height: constants.PopupHeightHelp,
			Title:  " Help ",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, helpCmd)
		return nil
	},
}

var helpViewerCmd = &cobra.Command{
	Use:    "help-viewer",
	Short:  "Run the help viewer",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get help content from embedded assets
		helpContent, err := embed.GetHelp()
		if err != nil {
			return fmt.Errorf("failed to get help content: %w", err)
		}

		return tui.RunHelpViewer(helpContent)
	},
}

var gitViewerCmd = &cobra.Command{
	Use:    "git-viewer [work-dir] [main-branch]",
	Short:  "Run the git viewer",
	Args:   cobra.ExactArgs(2),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir := args[0]
		mainBranch := args[1]
		return tui.RunGitViewer(workDir, mainBranch)
	},
}

var toggleGitStatusCmd = &cobra.Command{
	Use:   "toggle-git-status [session]",
	Short: "Show git viewer popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Check if this is a git repo
		if !appCtx.IsGitRepo {
			_ = tm.DisplayMessage("Not a git repository", 2000)
			return nil
		}

		// Get current pane's working directory (for worktree context)
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			panePath = appCtx.ProjectDir
		}
		panePath = strings.TrimSpace(panePath)

		// Get the main branch name dynamically
		gitClient := git.New()
		mainBranch := gitClient.GetMainBranch(panePath)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run git viewer in popup (closes with q/Esc/Ctrl+G)
		gitCmd := fmt.Sprintf("%s internal git-viewer %s %s", pawBin, panePath, mainBranch)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     constants.PopupWidthFull,
			Height:    constants.PopupHeightFull,
			Title:     " Git ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: panePath,
		}, gitCmd)
		return nil
	},
}

var toggleShowDiffCmd = &cobra.Command{
	Use:   "toggle-show-diff [session]",
	Short: "Show diff between task branch and main",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Check if this is a git repo
		if !appCtx.IsGitRepo {
			_ = tm.DisplayMessage("Not a git repository", 2000)
			return nil
		}

		// Get current pane's working directory (for worktree context)
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			panePath = appCtx.ProjectDir
		}
		panePath = strings.TrimSpace(panePath)

		// Get the main branch name dynamically
		gitClient := git.New()
		mainBranch := gitClient.GetMainBranch(panePath)

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run diff viewer in popup (closes with q/Esc/Ctrl+D)
		diffCmd := fmt.Sprintf("%s internal diff-viewer %s %s", pawBin, panePath, mainBranch)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     constants.PopupWidthFull,
			Height:    constants.PopupHeightFull,
			Title:     fmt.Sprintf(" Diff (%s...HEAD) ", mainBranch),
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: panePath,
		}, diffCmd)
		return nil
	},
}

var diffViewerCmd = &cobra.Command{
	Use:    "diff-viewer [work-dir] [main-branch]",
	Short:  "Run the diff viewer",
	Args:   cobra.ExactArgs(2),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir := args[0]
		mainBranch := args[1]
		return tui.RunDiffViewer(workDir, mainBranch)
	},
}

var toggleHistoryCmd = &cobra.Command{
	Use:   "toggle-history [session]",
	Short: "Toggle history picker popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run history picker in popup
		historyCmd := fmt.Sprintf("%s internal history-picker %s", pawBin, sessionName)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     constants.PopupWidthHistory,
			Height:    constants.PopupHeightHistory,
			Title:     " Task History (⌃R) ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: appCtx.ProjectDir,
		}, historyCmd)
		return nil
	},
}

var historyPickerCmd = &cobra.Command{
	Use:    "history-picker [session]",
	Short:  "Run the history picker",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(appCtx.GetLogPath(), appCtx.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("history-picker")
			logging.SetGlobal(logger)
		}

		logging.Debug("-> historyPickerCmd(session=%s)", sessionName)
		defer logging.Debug("<- historyPickerCmd")

		// Initialize input history service
		inputHistorySvc := service.NewInputHistoryService(appCtx.PawDir)

		// Load history
		history, err := inputHistorySvc.GetAllContents()
		if err != nil {
			logging.Warn("Failed to load input history: %v", err)
			fmt.Println("Failed to load history.")
			return nil
		}
		if len(history) == 0 {
			fmt.Println("No task history yet.")
			return nil
		}

		// Run history picker
		action, selected, err := tui.RunInputHistoryPicker(history)
		if err != nil {
			logging.Warn("Failed to run history picker: %v", err)
			return nil
		}

		// If user selected something, write it to the history selection file
		// The TaskInput will read and apply this content on its next update
		if action == tui.InputHistorySelect && selected != "" {
			logging.Trace("historyPickerCmd: selected history item")

			// Write selection to temp file for TaskInput to pick up
			selectionPath := filepath.Join(appCtx.PawDir, constants.HistorySelectionFile)
			if err := os.WriteFile(selectionPath, []byte(selected), 0644); err != nil {
				logging.Warn("Failed to write history selection: %v", err)
			} else {
				logging.Debug("Wrote history selection to %s", selectionPath)
			}
		}

		return nil
	},
}
