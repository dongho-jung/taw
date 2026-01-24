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

const (
	// TopPaneSize is the default height for top panes (40% of window)
	TopPaneSize = "40%"
	// tmux option keys for tracking top pane state
	topPaneIDKey     = "@paw_top_pane_id"
	topPaneTypeKey   = "@paw_top_pane_type"
	topPaneWindowKey = "@paw_top_pane_window"
)

// topPaneSizes maps pane types to their specific sizes.
// Content-fitting panes (finish, palette) use absolute line counts to ensure
// all content is visible without clipping.
// Scrollable panes (log, git, help, etc.) use percentage for flexibility.
// Note: project picker now uses popup instead of top pane for faster rendering.
var topPaneSizes = map[string]string{
	"finish":  "13", // 13 lines: title(1) + blank(1) + 4 options(8) + margin(1) + help(1) + buffer(1)
	"palette": "13", // 13 lines: title(1) + blank(1) + input(3) + blank(1) + 2 commands(4) + margin(1) + help(1) + buffer(1)
	// Scrollable/AltScreen panes (log, help, git, diff, history, template, prompt) use TopPaneSize ("40%")
}

// topPaneShortcuts maps pane types to their toggle shortcuts for user feedback
// Note: project picker (⌃J) now uses popup instead of top pane
var topPaneShortcuts = map[string]string{
	"log":      "⌃O",
	"help":     "⌃/",
	"git":      "⌃G",
	"diff":     "⌃D",
	"history":  "⌃R",
	"template": "⌃T",
	"finish":   "⌃F",
}

// TopPaneResult represents the result of displayTopPane operation
type TopPaneResult int

const (
	// TopPaneCreated indicates a new top pane was created
	TopPaneCreated TopPaneResult = iota
	// TopPaneClosed indicates the existing top pane was closed (toggle off)
	TopPaneClosed
	// TopPaneBlocked indicates another top pane is already open
	TopPaneBlocked
)

// TopPaneWindowInfo holds pre-fetched window information to avoid redundant tmux queries.
type TopPaneWindowInfo struct {
	WindowID   string
	WindowName string
}

// displayTopPane creates a top pane for TUI display with toggle and blocking behavior.
// - If no top pane exists, creates one and runs the command
// - If the same paneType is already open, closes it (toggle off)
// - If a different paneType is already open, blocks (returns TopPaneBlocked)
//
// paneType should be a unique identifier for the TUI (e.g., "log", "help", "git")
// command is the command to run in the pane
// workDir is the working directory for the pane (can be empty)
func displayTopPane(tm tmux.Client, paneType, command, workDir string) (TopPaneResult, error) {
	return displayTopPaneWithInfo(tm, paneType, command, workDir, nil)
}

// displayTopPaneWithInfo creates a top pane with optional pre-fetched window info.
// If windowInfo is provided, it skips fetching window_id from tmux, improving performance.
func displayTopPaneWithInfo(tm tmux.Client, paneType, command, workDir string, windowInfo *TopPaneWindowInfo) (TopPaneResult, error) {
	logging.Debug("-> displayTopPane(type=%s, cmd=%s)", paneType, command)
	defer logging.Debug("<- displayTopPane")

	var currentWindowID string
	var existingPaneID, existingWindowID, existingType string

	if windowInfo != nil {
		// Use pre-fetched window ID, only fetch pane state options
		currentWindowID = windowInfo.WindowID
		values, err := tm.DisplayMultiple(
			"#{@paw_top_pane_id}",
			"#{@paw_top_pane_window}",
			"#{@paw_top_pane_type}",
		)
		if err != nil || len(values) < 3 {
			logging.Debug("displayTopPane: DisplayMultiple failed: %v", err)
			return createTopPane(tm, paneType, command, workDir, currentWindowID)
		}
		existingPaneID = strings.TrimSpace(values[0])
		existingWindowID = strings.TrimSpace(values[1])
		existingType = strings.TrimSpace(values[2])
	} else {
		// Batch query: Get current window ID and all pane state options in a single tmux call
		values, err := tm.DisplayMultiple(
			"#{window_id}",
			"#{@paw_top_pane_id}",
			"#{@paw_top_pane_window}",
			"#{@paw_top_pane_type}",
		)
		if err != nil || len(values) < 4 {
			logging.Debug("displayTopPane: DisplayMultiple failed: %v", err)
			// Fallback: create pane directly without state check
			return createTopPane(tm, paneType, command, workDir, "")
		}
		currentWindowID = strings.TrimSpace(values[0])
		existingPaneID = strings.TrimSpace(values[1])
		existingWindowID = strings.TrimSpace(values[2])
		existingType = strings.TrimSpace(values[3])
	}

	// Verify the pane exists AND belongs to the current window
	// This prevents false positives from tmux reusing pane IDs
	paneValid := existingPaneID != "" &&
		tm.HasPane(existingPaneID) &&
		existingWindowID == currentWindowID

	if paneValid {
		if existingType == paneType {
			// Same type - toggle off (close the pane)
			logging.Debug("displayTopPane: toggling off existing %s pane", paneType)
			_ = tm.KillPane(existingPaneID)
			// Batch clear: Set all options to empty in a single call
			_ = tm.SetMultipleOptions(map[string]string{
				topPaneIDKey:     "",
				topPaneTypeKey:   "",
				topPaneWindowKey: "",
			})
			return TopPaneClosed, nil
		}

		// Different type - check if existing is "finish" (block) or auto-close
		if existingType == "finish" {
			// Block when finish picker is open - user is in the middle of finishing a task
			logging.Debug("displayTopPane: blocked by finish picker")
			shortcut := topPaneShortcuts[existingType]
			_ = tm.DisplayMessage(fmt.Sprintf("Finish action in progress (%s to close)", shortcut), 2000)
			return TopPaneBlocked, nil
		}

		// Auto-close existing pane and create new one
		logging.Debug("displayTopPane: auto-closing %s pane to open %s", existingType, paneType)
		_ = tm.KillPane(existingPaneID)
		// Options will be set when creating new pane below
	} else if existingPaneID != "" {
		// Clean up stale options if pane doesn't exist or is in a different window
		logging.Debug("displayTopPane: cleaning up stale options (paneID=%s, window=%s, currentWindow=%s)",
			existingPaneID, existingWindowID, currentWindowID)
	}

	return createTopPane(tm, paneType, command, workDir, currentWindowID)
}

// createTopPane creates a new top pane and stores state options.
// Extracted for reuse and clarity.
func createTopPane(tm tmux.Client, paneType, command, workDir, currentWindowID string) (TopPaneResult, error) {
	// Get window ID if not provided (fallback case)
	if currentWindowID == "" {
		var err error
		currentWindowID, err = tm.Display("#{window_id}")
		if err != nil {
			return TopPaneBlocked, fmt.Errorf("failed to get window ID: %w", err)
		}
		currentWindowID = strings.TrimSpace(currentWindowID)
	}

	// Determine pane size: use specific size for content-fitting panes, default for others
	paneSize := TopPaneSize
	if size, ok := topPaneSizes[paneType]; ok {
		paneSize = size
	}

	// Create new top pane
	newPaneID, err := tm.SplitWindowPane(tmux.SplitOpts{
		Horizontal: false, // vertical split (top/bottom)
		Size:       paneSize,
		StartDir:   workDir,
		Command:    command,
		Before:     true, // create pane above (top)
		Full:       true, // span entire window width
	})
	if err != nil {
		return TopPaneBlocked, fmt.Errorf("failed to create top pane: %w", err)
	}

	// Batch set: Store pane info for toggle/blocking in a single call
	newPaneID = strings.TrimSpace(newPaneID)
	_ = tm.SetMultipleOptions(map[string]string{
		topPaneIDKey:     newPaneID,
		topPaneTypeKey:   paneType,
		topPaneWindowKey: currentWindowID,
	})

	logging.Debug("displayTopPane: created pane %s for type %s in window %s", newPaneID, paneType, currentWindowID)
	return TopPaneCreated, nil
}

var toggleLogCmd = &cobra.Command{
	Use:   "toggle-log [session]",
	Short: "Toggle log viewer top pane",
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

		// Run log viewer in top pane (closes with q/Esc/Ctrl+O)
		logCmd := shellJoin(getPawBin(), "internal", "log-viewer", logPath)

		result, err := displayTopPane(tm, "log", logCmd, "")
		if err != nil {
			logging.Debug("toggleLogCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("toggleLogCmd: blocked by another top pane")
		}
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
	Short: "Toggle help top pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleHelpCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleHelpCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Run help viewer in top pane (closes with q/Esc/Ctrl+/)
		helpCmd := shellJoin(getPawBin(), "internal", "help-viewer")

		result, err := displayTopPane(tm, "help", helpCmd, "")
		if err != nil {
			logging.Debug("toggleHelpCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("toggleHelpCmd: blocked by another top pane")
		}
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
	Short: "Toggle git viewer top pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleGitStatusCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleGitStatusCmd")

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

		// Run git viewer in top pane (closes with q/Esc/Ctrl+G)
		gitCmd := shellJoin(getPawBin(), "internal", "git-viewer", panePath, mainBranch)

		result, err := displayTopPane(tm, "git", gitCmd, panePath)
		if err != nil {
			logging.Debug("toggleGitStatusCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("toggleGitStatusCmd: blocked by another top pane")
		}
		return nil
	},
}

var toggleShowDiffCmd = &cobra.Command{
	Use:   "toggle-show-diff [session]",
	Short: "Toggle diff viewer top pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleShowDiffCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleShowDiffCmd")

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

		// Run diff viewer in top pane (closes with q/Esc/Ctrl+D)
		diffCmd := shellJoin(getPawBin(), "internal", "diff-viewer", panePath, mainBranch)

		result, err := displayTopPane(tm, "diff", diffCmd, panePath)
		if err != nil {
			logging.Debug("toggleShowDiffCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("toggleShowDiffCmd: blocked by another top pane")
		}
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
	Short: "Toggle history picker top pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleHistoryCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleHistoryCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Run history picker in top pane
		historyCmd := shellJoin(getPawBin(), "internal", "history-picker", sessionName)

		result, err := displayTopPane(tm, "history", historyCmd, appCtx.ProjectDir)
		if err != nil {
			logging.Debug("toggleHistoryCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("toggleHistoryCmd: blocked by another top pane")
		}
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

var toggleTemplateCmd = &cobra.Command{
	Use:   "toggle-template [session]",
	Short: "Toggle template picker top pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleTemplateCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleTemplateCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		templateCmd := shellJoin(getPawBin(), "internal", "template-picker", sessionName)

		result, err := displayTopPane(tm, "template", templateCmd, appCtx.ProjectDir)
		if err != nil {
			logging.Debug("toggleTemplateCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("toggleTemplateCmd: blocked by another top pane")
		}
		return nil
	},
}

var templatePickerCmd = &cobra.Command{
	Use:    "template-picker [session]",
	Short:  "Run the template picker",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		_, cleanup := setupLoggerFromApp(appCtx, "template-picker", "")
		defer cleanup()

		templateSvc := service.NewTemplateService(appCtx.PawDir)
		templates, err := templateSvc.LoadTemplates()
		if err != nil {
			logging.Warn("Failed to load templates: %v", err)
			fmt.Println("Failed to load templates.")
			return nil
		}

		draftPath := filepath.Join(appCtx.PawDir, constants.TemplateDraftFile)
		draftContent := ""
		if data, err := os.ReadFile(draftPath); err == nil {
			draftContent = string(data)
			logging.Debug("templatePickerCmd: loaded draft (path=%s, bytes=%d)", draftPath, len(data))
		} else {
			logging.Debug("templatePickerCmd: no draft (path=%s, err=%v)", draftPath, err)
		}

		action, selected, updated, dirty, err := tui.RunTemplatePicker(templates, draftContent)
		if err != nil {
			logging.Warn("Failed to run template picker: %v", err)
			return nil
		}
		if selected != nil {
			logging.Debug("templatePickerCmd: result action=%v selected=%s selectedBytes=%d dirty=%v", action, selected.Name, len(selected.Content), dirty)
		} else {
			logging.Debug("templatePickerCmd: result action=%v selected=nil dirty=%v", action, dirty)
		}

		if dirty {
			if err := templateSvc.SaveTemplates(updated); err != nil {
				logging.Warn("Failed to save templates: %v", err)
			} else {
				logging.Debug("templatePickerCmd: saved templates count=%d", len(updated))
			}
		}

		if action == tui.TemplatePickerSelect && selected != nil {
			selectionPath := filepath.Join(appCtx.PawDir, constants.TemplateSelectionFile)
			if err := os.WriteFile(selectionPath, []byte(selected.Content), 0644); err != nil {
				logging.Warn("Failed to write template selection: %v", err)
			} else {
				logging.Debug("templatePickerCmd: wrote selection (path=%s, bytes=%d)", selectionPath, len(selected.Content))
			}
		}

		return nil
	},
}

var toggleProjectPickerCmd = &cobra.Command{
	Use:   "toggle-project-picker [session]",
	Short: "Show project picker popup to switch between PAW sessions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> toggleProjectPickerCmd(session=%s)", args[0])
		defer logging.Debug("<- toggleProjectPickerCmd")

		sessionName := args[0]
		tm := tmux.New(sessionName)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			logging.Debug("toggleProjectPickerCmd: getAppFromSession failed: %v", err)
			return err
		}

		// Run project picker in popup (faster than top pane)
		pickerCmd := shellJoin(getPawBin(), "internal", "project-picker-wrapper", sessionName)

		err = tm.DisplayPopup(tmux.PopupOpts{
			Width:     constants.PopupWidthProject,
			Height:    constants.PopupHeightProject,
			Title:     " Switch Project ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: appCtx.ProjectDir,
			Env: map[string]string{
				"PAW_DIR":     appCtx.PawDir,
				"PROJECT_DIR": appCtx.ProjectDir,
			},
		}, pickerCmd)
		if err != nil {
			logging.Debug("toggleProjectPickerCmd: displayPopup failed: %v", err)
			return err
		}

		return nil
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

var projectPickerWrapperCmd = &cobra.Command{
	Use:    "project-picker-wrapper [session]",
	Short:  "Run project picker and handle session switch",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		currentSession := args[0]

		appCtx, err := getAppFromSession(currentSession)
		if err != nil {
			logging.Debug("projectPickerWrapperCmd: getAppFromSession failed: %v", err)
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "project-picker-wrapper", "")
		defer cleanup()

		logging.Debug("-> projectPickerWrapperCmd(session=%s)", currentSession)
		defer logging.Debug("<- projectPickerWrapperCmd")

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

		// If user selected a project, perform the switch
		if action == tui.ProjectPickerSelect && selected != nil {
			logging.Debug("projectPickerWrapperCmd: switching to session %s", selected.Name)

			tm := tmux.New(currentSession)

			// Use detach-client -E to replace the current client with a new attachment
			// to the target session. This works across different tmux sockets.
			targetSocket := constants.TmuxSocketPrefix + selected.Name
			switchCmd := fmt.Sprintf("tmux -L %s attach-session -t %s", shellQuote(targetSocket), shellQuote(selected.Name))
			return tm.Run("detach-client", "-E", switchCmd)
		}

		return nil
	},
}
