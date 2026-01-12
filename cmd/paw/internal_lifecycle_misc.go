package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/notify"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

var doneTaskCmd = &cobra.Command{
	Use:   "done-task [session]",
	Short: "Show finish action picker for current task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "done-task", "")
		defer cleanup()

		logging.Debug("-> doneTaskCmd(session=%s)", sessionName)
		defer logging.Debug("<- doneTaskCmd")

		tm := tmux.New(sessionName)

		// Get current window ID
		windowID, err := tm.Display("#{window_id}")
		if err != nil {
			return fmt.Errorf("failed to get window ID: %w", err)
		}
		windowID = strings.TrimSpace(windowID)

		// Get current window name
		windowName, _ := tm.Display("#{window_name}")
		windowName = strings.TrimSpace(windowName)

		// Check if this is a task window (has emoji prefix)
		if !constants.IsTaskWindow(windowName) {
			_ = tm.DisplayMessage("Not a task window", 1500)
			return nil
		}

		// Show finish picker popup
		pawBin, _ := os.Executable()
		finishCmd := fmt.Sprintf("%s internal finish-picker-tui %s %s", pawBin, sessionName, windowID)

		// Display popup - if user presses Ctrl+F again, popup closes automatically
		_ = tm.DisplayPopup(tmux.PopupOpts{
			Width:  constants.PopupWidthFinishPicker,
			Height: constants.PopupHeightFinishPicker,
			Title:  " Finish Task ",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, finishCmd)

		return nil
	},
}

var recoverTaskCmd = &cobra.Command{
	Use:   "recover-task [session] [task-name]",
	Short: "Recover a corrupted task",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		taskName := args[1]

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "recover-task", taskName)
		defer cleanup()

		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		t, err := mgr.GetTask(taskName)
		if err != nil {
			return err
		}

		recoveryMgr := task.NewRecoveryManager(appCtx.ProjectDir)
		if err := recoveryMgr.RecoverTask(t); err != nil {
			return fmt.Errorf("failed to recover task: %w", err)
		}

		fmt.Printf("Task %s recovered successfully\n", taskName)
		return nil
	},
}

var resumeAgentCmd = &cobra.Command{
	Use:   "resume-agent [session] [window-id] [agent-dir]",
	Short: "Resume a stopped Claude agent in an existing window",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]
		agentDir := args[2]

		taskName := filepath.Base(agentDir)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "resume-agent", taskName)
		defer cleanup()

		logging.Log("=== Resuming agent: %s ===", taskName)

		tm := tmux.New(sessionName)

		// Get task
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		t, err := mgr.GetTask(taskName)
		if err != nil {
			return fmt.Errorf("failed to get task: %w", err)
		}

		// Determine work directory
		workDir := mgr.GetWorkingDirectory(t)

		// Get paw binary path
		pawBin, _ := os.Executable()
		// Use symlink path for PAW_BIN so running agents can use updated binary
		pawBinSymlink := filepath.Join(appCtx.PawDir, constants.BinSymlinkName)

		// Build start-agent script with --continue flag
		worktreeDirExport := ""
		if appCtx.IsWorktreeMode() {
			worktreeDirExport = fmt.Sprintf("export WORKTREE_DIR='%s'\n", workDir)
		}

		startAgentContent := fmt.Sprintf(`#!/bin/bash
# Auto-generated start-agent script for this task (RESUME MODE)
export TASK_NAME='%s'
export PAW_DIR='%s'
export PROJECT_DIR='%s'
%sexport WINDOW_ID='%s'
export PAW_HOME='%s'
export PAW_BIN='%s'
export SESSION_NAME='%s'

# Continue the previous Claude session (--continue auto-selects last session)
exec claude --continue --dangerously-skip-permissions
`, taskName, appCtx.PawDir, appCtx.ProjectDir, worktreeDirExport, windowID,
			filepath.Dir(filepath.Dir(pawBin)), pawBinSymlink, sessionName)

		startAgentScriptPath := filepath.Join(t.AgentDir, "start-agent")
		if err := os.WriteFile(startAgentScriptPath, []byte(startAgentContent), 0755); err != nil {
			return fmt.Errorf("failed to write start-agent script: %w", err)
		}

		agentPane := windowID + ".0"

		// Respawn the agent pane with the resume script
		if err := tm.RespawnPane(agentPane, workDir, startAgentScriptPath); err != nil {
			return fmt.Errorf("failed to respawn agent pane: %w", err)
		}

		logging.Log("Agent resumed: task=%s, windowID=%s", taskName, windowID)

		// Start wait watcher
		watchCmd := exec.Command(pawBin, "internal", "watch-wait", sessionName, windowID, taskName)
		watchCmd.Dir = appCtx.ProjectDir
		if err := watchCmd.Start(); err != nil {
			logging.Warn("Failed to start wait watcher: %v", err)
		} else {
			logging.Debug("Wait watcher started for windowID=%s", windowID)
		}

		// Notify user
		notify.PlaySound(notify.SoundTaskCreated)
		_ = notify.Send("Session resumed", fmt.Sprintf("🔄 %s resumed", taskName))
		if err := tm.DisplayMessage(fmt.Sprintf("🔄 Session resumed: %s", taskName), 2000); err != nil {
			logging.Trace("Failed to display message: %v", err)
		}

		return nil
	},
}
