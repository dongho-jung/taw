package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/claude"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

// Shell pane tmux option keys
const (
	shellPaneIDKey        = "@paw_shell_pane_id"
	stashedShellPaneIDKey = "@paw_stashed_shell_pane_id"
	stashWindowIDKey      = "@paw_stash_window_id"
	stashWindowName       = "_paw_stash" // name of the hidden window used to stash the shell pane
)

var popupShellCmd = &cobra.Command{
	Use:   "popup-shell [session]",
	Short: "Toggle shell pane at bottom",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		paneID, _ := tm.GetOption(shellPaneIDKey)
		hasShellPane := paneID != "" && tm.HasPane(paneID)
		if paneID != "" && !hasShellPane {
			_ = tm.SetOption(shellPaneIDKey, "", true)
			paneID = ""
		}

		stashedPaneID, _ := tm.GetOption(stashedShellPaneIDKey)
		hasStashedPane := stashedPaneID != "" && tm.HasPane(stashedPaneID)
		if stashedPaneID != "" && !hasStashedPane {
			_ = tm.SetOption(stashedShellPaneIDKey, "", true)
			stashedPaneID = ""
		}

		// Check if shell pane is currently visible - if so, hide it (toggle off)
		if hasShellPane {
			// Get or create stash window
			stashWindowID, err := getOrCreateStashWindow(tm, sessionName)
			if err != nil {
				// Fallback: kill the pane if we can't stash it
				_ = tm.KillPane(paneID)
				_ = tm.SetOption(shellPaneIDKey, "", true)
				return nil
			}

			// Move shell pane to stash window
			if err := tm.JoinPane(paneID, stashWindowID, tmux.JoinOpts{Detached: true}); err != nil {
				// JoinPane failed - the pane is likely dead (killed by Ctrl+D or similar)
				// Clean up the invalid state
				_ = tm.KillPane(paneID)
				_ = tm.SetOption(shellPaneIDKey, "", true)
				_ = tm.SetOption(stashedShellPaneIDKey, "", true)
				// Since the pane was dead, the user actually wanted to SHOW a shell
				// (they thought there was no shell). Create a new one.
				return createNewShellPane(tm, sessionName)
			}

			// Store pane ID in stash and clear visible pane ID
			_ = tm.SetOption(stashedShellPaneIDKey, paneID, true)
			_ = tm.SetOption(shellPaneIDKey, "", true)
			return nil
		}

		// Toggle ON: Check if there's a stashed shell pane we can restore
		if hasStashedPane {
			// Get current window ID
			currentWindowID, err := tm.Display("#{window_id}")
			if err != nil {
				return fmt.Errorf("failed to get current window: %w", err)
			}
			currentWindowID = strings.TrimSpace(currentWindowID)

			// Bring back the stashed pane
			if err := tm.JoinPane(stashedPaneID, currentWindowID, tmux.JoinOpts{
				Size: constants.TopPaneSize,
				Full: true,
			}); err != nil {
				_ = tm.SetOption(stashedShellPaneIDKey, "", true)
				// Stash is corrupted, create new pane
				return createNewShellPane(tm, sessionName)
			}

			// Update state: pane is now visible
			_ = tm.SetOption(shellPaneIDKey, stashedPaneID, true)
			_ = tm.SetOption(stashedShellPaneIDKey, "", true)

			// Select the restored shell pane
			_ = tm.SelectPane(stashedPaneID)
			return nil
		}

		// No stashed pane, create a new one
		return createNewShellPane(tm, sessionName)
	},
}

// getOrCreateStashWindow returns the stash window ID, creating it if necessary.
func getOrCreateStashWindow(tm tmux.Client, sessionName string) (string, error) {
	// Check if stash window already exists
	stashWindowID, _ := tm.GetOption(stashWindowIDKey)
	if stashWindowID != "" {
		windows, err := tm.ListWindows()
		if err == nil {
			for _, w := range windows {
				if w.ID == stashWindowID {
					hideStashWindow(tm, stashWindowID)
					return stashWindowID, nil
				}
			}
		}
		_ = tm.SetOption(stashWindowIDKey, "", true)
	}

	// Create new stash window (detached, so it doesn't become active)
	windowID, err := tm.NewWindow(tmux.WindowOpts{
		Target:   sessionName,
		Name:     stashWindowName,
		Detached: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create stash window: %w", err)
	}

	windowID = strings.TrimSpace(windowID)
	_ = tm.SetOption(stashWindowIDKey, windowID, true)
	hideStashWindow(tm, windowID)
	return windowID, nil
}

func hideStashWindow(tm tmux.Client, windowID string) {
	if windowID == "" {
		return
	}
	_ = tm.Run("set-window-option", "-t", windowID, "window-status-format", "")
	_ = tm.Run("set-window-option", "-t", windowID, "window-status-current-format", "")
}

// createNewShellPane creates a new shell pane at the bottom of the current window.
func createNewShellPane(tm tmux.Client, sessionName string) error {
	// Get current pane's working directory
	panePath, err := tm.Display("#{pane_current_path}")
	if err != nil || panePath == "" {
		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}
		panePath = appCtx.ProjectDir
	}
	panePath = strings.TrimSpace(panePath)

	// Create shell pane at bottom
	newPaneID, err := tm.SplitWindowPane(tmux.SplitOpts{
		Horizontal: false, // vertical split (top/bottom)
		Size:       constants.TopPaneSize,
		StartDir:   panePath,
		Full:       true, // span entire window width
	})
	if err != nil {
		return fmt.Errorf("failed to create shell pane: %w", err)
	}

	newPaneID = strings.TrimSpace(newPaneID)

	// Store pane ID for toggle
	_ = tm.SetOption(shellPaneIDKey, newPaneID, true)

	// Explicitly select the new pane to ensure it's visible
	// This is needed because after Ctrl+D kills a pane, tmux may leave
	// the focus in an unexpected state
	_ = tm.SelectPane(newPaneID)

	return nil
}

var showCurrentTaskCmd = &cobra.Command{
	Use:    "show-current-task [session]",
	Short:  "Display current task content in a popup",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "show-current-task", "")
		defer cleanup()

		logging.Debug("-> showCurrentTaskCmd(session=%s)", sessionName)
		defer logging.Debug("<- showCurrentTaskCmd")

		tm := tmux.New(sessionName)

		// Get current window info
		windowID, windowName, err := getCurrentWindowInfo(tm)
		if err != nil {
			return fmt.Errorf("failed to get window info: %w", err)
		}

		logging.Debug("Current window: name=%s, id=%s", windowName, windowID)

		// Check if this is a task window (has task emoji prefix)
		taskName, isTaskWindow := constants.ExtractTaskName(windowName)
		if !isTaskWindow {
			_ = tm.DisplayMessage("Not a task window", constants.DisplayMsgStandard)
			return nil
		}

		logging.Debug("Task name (may be truncated): %s", taskName)

		// Find task using truncated name (window names are limited to MaxWindowNameLen chars)
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		t, err := mgr.FindTaskByTruncatedName(taskName)
		if err != nil {
			_ = tm.DisplayMessage("Task not found: "+taskName, constants.DisplayMsgStandard)
			logging.Debug("Task not found for truncated name: %s", taskName)
			return nil
		}

		logging.Debug("Found task: name=%s, agentDir=%s", t.Name, t.AgentDir)

		// Get task file path
		taskFilePath := t.GetTaskFilePath()
		if _, err := os.Stat(taskFilePath); os.IsNotExist(err) {
			_ = tm.DisplayMessage("Task file not found", constants.DisplayMsgStandard)
			return nil
		}

		// Run task viewer in top pane (closes with q/Esc)
		taskViewerCmd := shellJoin(getPawBin(), "internal", "task-viewer", taskFilePath)

		result, err := displayTopPane(tm, "task", taskViewerCmd, "")
		if err != nil {
			logging.Debug("showCurrentTaskCmd: displayTopPane failed: %v", err)
			return err
		}
		if result == TopPaneBlocked {
			logging.Debug("showCurrentTaskCmd: blocked by another top pane")
		}

		logging.Log("Displayed task content for: %s", t.Name)
		return nil
	},
}

var restorePanesCmd = &cobra.Command{
	Use:    "restore-panes [session]",
	Short:  "Restore missing panes in current task window",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(_ *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "restore-panes", "")
		defer cleanup()

		logging.Debug("-> restorePanesCmd(session=%s)", sessionName)
		defer logging.Debug("<- restorePanesCmd")

		tm := tmux.New(sessionName)

		// Get current window info
		windowID, windowName, err := getCurrentWindowInfo(tm)
		if err != nil {
			return fmt.Errorf("failed to get window info: %w", err)
		}

		logging.Debug("Current window: name=%s, id=%s", windowName, windowID)

		// Check if this is a task window (has task emoji prefix)
		taskName, isTaskWindow := constants.ExtractTaskName(windowName)
		if !isTaskWindow {
			_ = tm.DisplayMessage("Not a task window", constants.DisplayMsgStandard)
			return nil
		}

		logging.Debug("Task name (may be truncated): %s", taskName)

		// Find task using truncated name (window names are limited to MaxWindowNameLen chars)
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		t, err := mgr.FindTaskByTruncatedName(taskName)
		if err != nil {
			_ = tm.DisplayMessage("Task not found: "+taskName, constants.DisplayMsgStandard)
			logging.Debug("Task not found for truncated name: %s", taskName)
			return nil
		}
		agentDir := t.AgentDir
		logging.Debug("Found task: name=%s, agentDir=%s", t.Name, agentDir)

		// Get current pane count
		paneCount, err := tm.Display("#{window_panes}")
		if err != nil {
			return fmt.Errorf("failed to get pane count: %w", err)
		}
		paneCount = strings.TrimSpace(paneCount)

		logging.Debug("Current pane count: %s", paneCount)

		// Task window should have 2 panes: agent (0) and user (1)
		// Get working directory (t and mgr already set above)
		workDir := mgr.GetWorkingDirectory(t)

		// Check which pane is missing and restore
		switch paneCount {
		case "2":
			_ = tm.DisplayMessage("All panes are present", constants.DisplayMsgStandard)
			return nil
		case "0":
			// Both panes missing - respawn the window
			logging.Log("Both panes missing, respawning agent pane")

			// Start agent pane
			startAgentScript := filepath.Join(agentDir, constants.StartAgentScriptName)
			if _, err := os.Stat(startAgentScript); os.IsNotExist(err) {
				_ = tm.DisplayMessage(constants.StartAgentScriptName+" script not found", constants.DisplayMsgStandard)
				return nil
			}

			if err := tm.RespawnPane(windowID+".0", workDir, shellQuote(startAgentScript)); err != nil {
				return fmt.Errorf("failed to respawn agent pane: %w", err)
			}

			// Create user pane
			taskFilePath := t.GetTaskFilePath()
			userPaneCmd := shellCommand(fmt.Sprintf("cat %s; echo; exec %s", shellQuote(taskFilePath), shellQuote(getShell())))
			if err := tm.SplitWindow(windowID, true, workDir, userPaneCmd); err != nil {
				logging.Warn("Failed to create user pane: %v", err)
			}

			_ = tm.DisplayMessage("Restored both panes", constants.DisplayMsgStandard)
		case "1":
			// One pane exists - need to determine which one is missing
			// Check if the existing pane is running claude (agent) or shell (user)
			paneCmd, err := tm.GetPaneCommand(windowID + ".0")
			if err != nil {
				paneCmd = ""
			}
			paneCmd = strings.TrimSpace(paneCmd)

			logging.Debug("Existing pane command: %s", paneCmd)

			if paneCmd == "claude" || strings.Contains(paneCmd, constants.StartAgentScriptName) {
				// Agent pane exists, user pane is missing
				logging.Log("User pane missing, creating it")
				taskFilePath := t.GetTaskFilePath()
				userPaneCmd := shellCommand(fmt.Sprintf("cat %s; echo; exec %s", shellQuote(taskFilePath), shellQuote(getShell())))
				if err := tm.SplitWindow(windowID, true, workDir, userPaneCmd); err != nil {
					return fmt.Errorf("failed to create user pane: %w", err)
				}
				_ = tm.DisplayMessage("Restored user pane", constants.DisplayMsgStandard)
			} else {
				// User pane exists (or unknown), agent pane is missing
				logging.Log("Agent pane missing, creating it")

				// Need to create agent pane before the user pane
				startAgentScript := filepath.Join(agentDir, constants.StartAgentScriptName)
				if _, err := os.Stat(startAgentScript); os.IsNotExist(err) {
					_ = tm.DisplayMessage(constants.StartAgentScriptName+" script not found", constants.DisplayMsgStandard)
					return nil
				}

				// Split before the current pane to create agent pane at position 0
				_, err := tm.SplitWindowPane(tmux.SplitOpts{
					Target:     windowID + ".0",
					Horizontal: true,
					Before:     true,
					StartDir:   workDir,
					Command:    shellQuote(startAgentScript),
				})
				if err != nil {
					return fmt.Errorf("failed to create agent pane: %w", err)
				}
				_ = tm.DisplayMessage("Restored agent pane", constants.DisplayMsgStandard)
			}
		default:
			logging.Warn("Unexpected pane count (%s), skipping restore", paneCount)
			_ = tm.DisplayMessage("Unexpected pane count: "+paneCount, constants.DisplayMsgStandard)
			return nil
		}

		logging.Log("Panes restored for task: %s", t.Name)

		// Check for stdin injection failure: if agent pane exists but session marker doesn't,
		// it means the task instruction was never sent
		if err := checkAndRecoverStdinInjection(tm, t, windowID); err != nil {
			logging.Warn("Failed to recover stdin injection: %v", err)
		}

		return nil
	},
}

// checkAndRecoverStdinInjection detects and recovers from failed stdin injection.
// If Claude is running but the session marker doesn't exist, it sends the task instruction.
func checkAndRecoverStdinInjection(tm tmux.Client, t *task.Task, windowID string) error {
	agentPane := windowID + ".0"

	// Check if session marker exists (indicates task instruction was sent successfully)
	if t.HasSessionMarker() {
		logging.Trace("checkAndRecoverStdinInjection: session marker exists, skipping")
		return nil
	}

	// Check if user prompt file exists (required to send task instruction)
	userPromptPath := t.GetUserPromptPath()
	if _, err := os.Stat(userPromptPath); os.IsNotExist(err) {
		logging.Debug("checkAndRecoverStdinInjection: user prompt not found, skipping")
		return nil
	}

	// Check if Claude is running in the agent pane
	claudeClient := claude.New()
	if !claudeClient.IsClaudeRunning(tm, agentPane) {
		logging.Debug("checkAndRecoverStdinInjection: Claude not running, skipping")
		return nil
	}

	// Claude is running but session marker doesn't exist - stdin injection likely failed
	logging.Log("Detected failed stdin injection for task %s, recovering...", t.Name)

	// Build and send task instruction
	taskInstruction, err := buildTaskInstruction(userPromptPath)
	if err != nil {
		return fmt.Errorf("failed to build task instruction: %w", err)
	}

	logging.Debug("Sending task instruction")

	if err := claudeClient.SendInputWithRetry(tm, agentPane, taskInstruction, 5); err != nil {
		// Try basic send as last resort
		logging.Warn("SendInputWithRetry failed, trying basic send: %v", err)
		if err := claudeClient.SendInput(tm, agentPane, taskInstruction); err != nil {
			return fmt.Errorf("failed to send task instruction: %w", err)
		}
	}

	// Create session marker to prevent re-sending on next restore
	if err := t.CreateSessionMarker(); err != nil {
		logging.Warn("Failed to create session marker: %v", err)
	} else {
		logging.Debug("Session marker created after stdin recovery")
	}

	_ = tm.DisplayMessage("Recovered task instruction for: "+t.Name, constants.DisplayMsgStandard)
	logging.Log("Successfully recovered stdin injection for task: %s", t.Name)

	return nil
}
