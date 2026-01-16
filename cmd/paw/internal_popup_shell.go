package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/claude"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

var popupShellCmd = &cobra.Command{
	Use:   "popup-shell [session]",
	Short: "Toggle shell pane at bottom 40%",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Check if shell pane exists - if so, close it (toggle off)
		paneID, _ := tm.GetOption("@paw_shell_pane_id")
		if paneID != "" && tm.HasPane(paneID) {
			_ = tm.KillPane(paneID)
			_ = tm.SetOption("@paw_shell_pane_id", "", true)
			return nil
		}

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

		// Create shell pane at bottom 40%
		newPaneID, err := tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false, // vertical split (top/bottom)
			Size:       "40%",
			StartDir:   panePath,
			Full:       true, // span entire window width
		})
		if err != nil {
			return fmt.Errorf("failed to create shell pane: %w", err)
		}

		// Store pane ID for toggle
		_ = tm.SetOption("@paw_shell_pane_id", strings.TrimSpace(newPaneID), true)

		return nil
	},
}

var restorePanesCmd = &cobra.Command{
	Use:    "restore-panes [session]",
	Short:  "Restore missing panes in current task window",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
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
		windowName, err := tm.Display("#{window_name}")
		if err != nil {
			return fmt.Errorf("failed to get window name: %w", err)
		}
		windowName = strings.TrimSpace(windowName)

		windowID, err := tm.Display("#{window_id}")
		if err != nil {
			return fmt.Errorf("failed to get window ID: %w", err)
		}
		windowID = strings.TrimSpace(windowID)

		logging.Debug("Current window: name=%s, id=%s", windowName, windowID)

		// Check if this is a task window (has task emoji prefix)
		taskName, isTaskWindow := constants.ExtractTaskName(windowName)
		if !isTaskWindow {
			_ = tm.DisplayMessage("Not a task window", 2000)
			return nil
		}

		logging.Debug("Task name (may be truncated): %s", taskName)

		// Find task using truncated name (window names are limited to MaxWindowNameLen chars)
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		t, err := mgr.FindTaskByTruncatedName(taskName)
		if err != nil {
			_ = tm.DisplayMessage(fmt.Sprintf("Task not found: %s", taskName), 2000)
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
			_ = tm.DisplayMessage("All panes are present", 2000)
			return nil
		case "0":
			// Both panes missing - respawn the window
			logging.Log("Both panes missing, respawning agent pane")

			// Start agent pane
			startAgentScript := filepath.Join(agentDir, "start-agent")
			if _, err := os.Stat(startAgentScript); os.IsNotExist(err) {
				_ = tm.DisplayMessage("start-agent script not found", 2000)
				return nil
			}

			if err := tm.RespawnPane(windowID+".0", workDir, startAgentScript); err != nil {
				return fmt.Errorf("failed to respawn agent pane: %w", err)
			}

			// Create user pane
			taskFilePath := t.GetTaskFilePath()
			userPaneCmd := fmt.Sprintf("sh -c 'cat %s; echo; exec %s'", taskFilePath, getShell())
			if err := tm.SplitWindow(windowID, true, workDir, userPaneCmd); err != nil {
				logging.Warn("Failed to create user pane: %v", err)
			}

			_ = tm.DisplayMessage("Restored both panes", 2000)
		case "1":
			// One pane exists - need to determine which one is missing
			// Check if the existing pane is running claude (agent) or shell (user)
			paneCmd, err := tm.GetPaneCommand(windowID + ".0")
			if err != nil {
				paneCmd = ""
			}
			paneCmd = strings.TrimSpace(paneCmd)

			logging.Debug("Existing pane command: %s", paneCmd)

			if paneCmd == "claude" || strings.Contains(paneCmd, "start-agent") {
				// Agent pane exists, user pane is missing
				logging.Log("User pane missing, creating it")
				taskFilePath := t.GetTaskFilePath()
				userPaneCmd := fmt.Sprintf("sh -c 'cat %s; echo; exec %s'", taskFilePath, getShell())
				if err := tm.SplitWindow(windowID, true, workDir, userPaneCmd); err != nil {
					return fmt.Errorf("failed to create user pane: %w", err)
				}
				_ = tm.DisplayMessage("Restored user pane", 2000)
			} else {
				// User pane exists (or unknown), agent pane is missing
				logging.Log("Agent pane missing, creating it")

				// Need to create agent pane before the user pane
				startAgentScript := filepath.Join(agentDir, "start-agent")
				if _, err := os.Stat(startAgentScript); os.IsNotExist(err) {
					_ = tm.DisplayMessage("start-agent script not found", 2000)
					return nil
				}

				// Split before the current pane to create agent pane at position 0
				_, err := tm.SplitWindowPane(tmux.SplitOpts{
					Target:     windowID + ".0",
					Horizontal: true,
					Before:     true,
					StartDir:   workDir,
					Command:    startAgentScript,
				})
				if err != nil {
					return fmt.Errorf("failed to create agent pane: %w", err)
				}
				_ = tm.DisplayMessage("Restored agent pane", 2000)
			}
		default:
			logging.Warn("Unexpected pane count (%s), skipping restore", paneCount)
			_ = tm.DisplayMessage(fmt.Sprintf("Unexpected pane count: %s", paneCount), 2000)
			return nil
		}

		logging.Log("Panes restored for task: %s", t.Name)

		// Check for stdin injection failure: if agent pane exists but session marker doesn't,
		// it means the task instruction was never sent
		if err := checkAndRecoverStdinInjection(tm, t, windowID, agentDir); err != nil {
			logging.Warn("Failed to recover stdin injection: %v", err)
		}

		return nil
	},
}

// checkAndRecoverStdinInjection detects and recovers from failed stdin injection.
// If Claude is running but the session marker doesn't exist, it sends the task instruction.
func checkAndRecoverStdinInjection(tm tmux.Client, t *task.Task, windowID, agentDir string) error {
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

	// Load task options to get ultrathink setting
	taskOpts, err := config.LoadTaskOptions(agentDir)
	if err != nil {
		logging.Warn("Failed to load task options: %v", err)
		taskOpts = config.DefaultTaskOptions()
	}

	// Build and send task instruction
	taskInstruction := buildTaskInstruction(userPromptPath, taskOpts.Ultrathink)

	logging.Debug("Sending task instruction: ultrathink=%v", taskOpts.Ultrathink)

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

	_ = tm.DisplayMessage(fmt.Sprintf("Recovered task instruction for: %s", t.Name), 2000)
	logging.Log("Successfully recovered stdin injection for task: %s", t.Name)

	return nil
}
