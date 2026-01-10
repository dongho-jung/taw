package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/logging"
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
	Use:    "git-viewer [work-dir]",
	Short:  "Run the git viewer",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir := args[0]
		return tui.RunGitViewer(workDir)
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

		pawBin, err := os.Executable()
		if err != nil {
			pawBin = "paw"
		}

		// Run git viewer in popup (closes with q/Esc/Ctrl+G)
		gitCmd := fmt.Sprintf("%s internal git-viewer %s", pawBin, panePath)

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

var toggleTemplateCmd = &cobra.Command{
	Use:   "toggle-template [session]",
	Short: "Toggle template selector popup",
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

		// Run template viewer in popup
		templateCmd := fmt.Sprintf("%s internal template-viewer %s", pawBin, sessionName)

		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:     constants.PopupWidthFull,
			Height:    constants.PopupHeightFull,
			Title:     " Templates ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: appCtx.ProjectDir,
		}, templateCmd)
		return nil
	},
}

var templateViewerCmd = &cobra.Command{
	Use:    "template-viewer [session]",
	Short:  "Run the template viewer",
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
			logger.SetScript("template-viewer")
			logging.SetGlobal(logger)
		}

		logging.Debug("-> templateViewerCmd(session=%s)", sessionName)
		defer logging.Debug("<- templateViewerCmd")

		return runTemplateLoop(appCtx, sessionName)
	},
}

// runTemplateLoop runs the template UI loop, handling CRUD operations.
func runTemplateLoop(appCtx *app.App, sessionName string) error {
	for {
		action, selected, err := tui.RunTemplateUI(appCtx.PawDir)
		if err != nil {
			return err
		}

		switch action {
		case tui.TemplateActionNone:
			// User closed without action
			return nil

		case tui.TemplateActionSelect:
			// User selected a template - send content to task input via tmux
			if selected != nil && selected.Content != "" {
				logging.Trace("templateViewerCmd: selected template=%s", selected.Name)
				tm := tmux.New(sessionName)

				// Get the current window name to check if we're in the new task window
				windowName, _ := tm.Display("#{window_name}")
				windowName = strings.TrimSpace(windowName)

				// Only send keys if we're in the new task window (⭐️)
				if strings.HasPrefix(windowName, "⭐️") {
					// Send the template content to the input box
					// Use tmux send-keys to type the content
					if err := tm.SendKeys("", selected.Content); err != nil {
						logging.Warn("Failed to send template content: %v", err)
					}
				} else {
					logging.Debug("Not in new task window, skipping template injection")
				}
			}
			return nil

		case tui.TemplateActionCreate:
			// Open editor for new template
			result, err := tui.RunTemplateEditor(tui.TemplateEditorModeCreate, "", "")
			if err != nil {
				return err
			}

			if result.Saved && result.Name != "" && result.Content != "" {
				templates, err := config.LoadTemplates(appCtx.PawDir)
				if err != nil {
					templates = &config.Templates{Items: []config.Template{}}
				}

				if err := templates.Add(result.Name, result.Content); err != nil {
					logging.Warn("Failed to add template: %v", err)
				} else if err := templates.Save(appCtx.PawDir); err != nil {
					logging.Warn("Failed to save templates: %v", err)
				} else {
					logging.Info("Created template: %s", result.Name)
				}
			}
			// Continue loop to show updated list

		case tui.TemplateActionEdit:
			// Open editor for existing template
			if selected != nil {
				result, err := tui.RunTemplateEditor(tui.TemplateEditorModeEdit, selected.Name, selected.Content)
				if err != nil {
					return err
				}

				if result.Saved && result.Name != "" && result.Content != "" {
					templates, err := config.LoadTemplates(appCtx.PawDir)
					if err != nil {
						logging.Warn("Failed to load templates: %v", err)
						continue
					}

					if err := templates.Update(selected.Name, result.Name, result.Content); err != nil {
						logging.Warn("Failed to update template: %v", err)
					} else if err := templates.Save(appCtx.PawDir); err != nil {
						logging.Warn("Failed to save templates: %v", err)
					} else {
						logging.Info("Updated template: %s", result.Name)
					}
				}
			}
			// Continue loop to show updated list

		case tui.TemplateActionDelete:
			// Delete the selected template
			if selected != nil {
				templates, err := config.LoadTemplates(appCtx.PawDir)
				if err != nil {
					logging.Warn("Failed to load templates: %v", err)
					continue
				}

				if err := templates.Delete(selected.Name); err != nil {
					logging.Warn("Failed to delete template: %v", err)
				} else if err := templates.Save(appCtx.PawDir); err != nil {
					logging.Warn("Failed to save templates: %v", err)
				} else {
					logging.Info("Deleted template: %s", selected.Name)
				}
			}
			// Continue loop to show updated list
		}
	}
}

var templateEditorCmd = &cobra.Command{
	Use:    "template-editor [session] [mode] [name]",
	Short:  "Run the template editor",
	Args:   cobra.RangeArgs(2, 3),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		mode := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		var name, content string
		var editorMode tui.TemplateEditorMode

		if mode == "edit" && len(args) > 2 {
			name = args[2]
			editorMode = tui.TemplateEditorModeEdit

			// Load existing template content
			templates, err := config.LoadTemplates(appCtx.PawDir)
			if err == nil {
				if tmpl := templates.Find(name); tmpl != nil {
					content = tmpl.Content
				}
			}
		} else {
			editorMode = tui.TemplateEditorModeCreate
		}

		result, err := tui.RunTemplateEditor(editorMode, name, content)
		if err != nil {
			return err
		}

		if result.Saved && result.Name != "" && result.Content != "" {
			templates, err := config.LoadTemplates(appCtx.PawDir)
			if err != nil {
				templates = &config.Templates{Items: []config.Template{}}
			}

			if editorMode == tui.TemplateEditorModeEdit {
				if err := templates.Update(name, result.Name, result.Content); err != nil {
					return fmt.Errorf("failed to update template: %w", err)
				}
			} else {
				if err := templates.Add(result.Name, result.Content); err != nil {
					return fmt.Errorf("failed to add template: %w", err)
				}
			}

			if err := templates.Save(appCtx.PawDir); err != nil {
				return fmt.Errorf("failed to save templates: %w", err)
			}
		}

		return nil
	},
}
