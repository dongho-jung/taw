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

var toggleProjectPickerCmd = &cobra.Command{
	Use:   "toggle-project-picker [session]",
	Short: "Toggle project picker popup to switch between PAW sessions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			logging.Debug("toggleProjectPickerCmd: getAppFromSession failed: %v", err)
			return err
		}

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Clean up any stale switch target file before showing popup
		switchPath := filepath.Join(appCtx.PawDir, constants.ProjectSwitchFileName)
		_ = os.Remove(switchPath)

		// Run project picker in popup
		pickerCmd := fmt.Sprintf("%s internal project-picker %s", pawBin, sessionName)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:  constants.PopupWidthProjectPicker,
			Height: constants.PopupHeightProjectPicker,
			Title:  " Switch Project (⌃J) ",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, pickerCmd)

		// Check if a switch target was written by the project picker
		targetData, err := os.ReadFile(switchPath)
		if err != nil {
			// No switch target - user cancelled or closed popup
			return nil
		}

		// Clean up the switch file
		_ = os.Remove(switchPath)

		targetSession := strings.TrimSpace(string(targetData))
		if targetSession == "" {
			return nil
		}

		logging.Debug("toggleProjectPickerCmd: switching to session %s", targetSession)

		// Use detach-client -E to replace the current client with a new attachment
		// to the target session. This works across different tmux sockets.
		targetSocket := constants.TmuxSocketPrefix + targetSession
		switchCmd := fmt.Sprintf("tmux -L %s attach-session -t %s", targetSocket, targetSession)
		return tm.Run("detach-client", "-E", switchCmd)
	},
}

var projectPickerCmd = &cobra.Command{
	Use:    "project-picker [session]",
	Short:  "Run the project picker",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		currentSession := args[0]

		// Find all PAW sessions
		sessions, err := findPawSessions()
		if err != nil {
			fmt.Println("Failed to find PAW sessions.")
			return nil
		}

		// Filter out current session
		var projects []tui.ProjectPickerItem
		for _, s := range sessions {
			if s.Name != currentSession {
				projects = append(projects, tui.ProjectPickerItem{
					Name:       s.Name,
					SocketPath: s.SocketPath,
				})
			}
		}

		if len(projects) == 0 {
			fmt.Println("No other PAW projects running.")
			return nil
		}

		// Run project picker
		action, selected, err := tui.RunProjectPicker(projects)
		if err != nil {
			fmt.Printf("Failed to run project picker: %v\n", err)
			return nil
		}

		// If user selected a project, write the selection to a file
		// The toggle-project-picker command will read this and execute the switch
		if action == tui.ProjectPickerSelect && selected != nil {
			appCtx, err := getAppFromSession(currentSession)
			if err != nil {
				logging.Warn("projectPickerCmd: failed to get app context: %v", err)
				return nil
			}

			// Write the target session name to a file
			switchPath := filepath.Join(appCtx.PawDir, constants.ProjectSwitchFileName)
			if err := os.WriteFile(switchPath, []byte(selected.Name), 0644); err != nil {
				logging.Warn("projectPickerCmd: failed to write switch target: %v", err)
				return nil
			}
			logging.Debug("projectPickerCmd: wrote switch target to %s: %s", switchPath, selected.Name)
		}

		return nil
	},
}

