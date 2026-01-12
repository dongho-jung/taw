package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/claude"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/notify"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

var handleTaskCmd = &cobra.Command{
	Use:   "handle-task [session] [agent-dir]",
	Short: "Handle a task (create window, start Claude)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Debug("-> handleTaskCmd(session=%s, agentDir=%s)", args[0], args[1])
		defer logging.Debug("<- handleTaskCmd")

		sessionName := args[0]
		agentDir := args[1]

		taskName := filepath.Base(agentDir)

		appCtx, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		_, cleanup := setupLoggerFromApp(appCtx, "handle-task", taskName)
		defer cleanup()

		logging.Debug("New task detected: name=%s, agentDir=%s", taskName, agentDir)

		// Get task
		mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
		t, err := mgr.GetTask(taskName)
		if err != nil {
			logging.Error("Failed to get task: %v", err)
			return err
		}
		logging.Trace("Task loaded: content_length=%d", len(t.Content))

		// Load task options
		taskOpts, err := config.LoadTaskOptions(agentDir)
		if err != nil {
			logging.Warn("Failed to load task options: %v", err)
			taskOpts = config.DefaultTaskOptions()
		}
		logging.Debug("Task options: model=%s, ultrathink=%v", taskOpts.Model, taskOpts.Ultrathink)

		// Create tab-lock atomically
		created, err := t.CreateTabLock()
		if err != nil {
			logging.Error("Failed to create tab-lock: %v", err)
			return err
		}
		if !created {
			logging.Debug("Task already being handled by another process")
			return nil
		}
		logging.Debug("Tab-lock created successfully")

		// Track if this is a reopen case (for session resume)
		isReopen := false

		// Setup worktree if git mode (skip if worktree already exists - reopen case)
		if appCtx.IsWorktreeMode() {
			worktreeDir := t.GetWorktreeDir()
			if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
				// Worktree doesn't exist, create it
				timer := logging.StartTimer("worktree setup")
				if err := mgr.SetupWorktree(t); err != nil {
					timer.StopWithResult(false, err.Error())
					_ = t.RemoveTabLock()
					return fmt.Errorf("failed to setup worktree: %w", err)
				}
				timer.StopWithResult(true, fmt.Sprintf("branch=%s, path=%s", taskName, t.WorktreeDir))
			} else {
				// Worktree already exists (reopen case)
				logging.Debug("Worktree already exists, reusing: %s", worktreeDir)
				t.WorktreeDir = worktreeDir
				// Check if session was started before (has marker)
				if t.HasSessionMarker() {
					isReopen = true
					logging.Log("Session resume: detected previous session for task %s", taskName)
				}
			}
		} else {
			// Non-worktree mode: check session marker for reopen
			if t.HasSessionMarker() {
				isReopen = true
				logging.Log("Session resume: detected previous session for task %s", taskName)
			}
		}

		// Load saved status for reopen (to restore window name with correct emoji)
		if isReopen {
			if status, err := t.LoadStatus(); err == nil && status != "" {
				logging.Debug("Loaded saved status for resume: %s", status)
			}
		}

		// Setup symlinks (error is non-fatal)
		if err := t.SetupSymlinks(appCtx.ProjectDir); err != nil {
			logging.Warn("Failed to setup symlinks: %v", err)
		}

		// Create tmux window
		tm := tmux.New(sessionName)
		workDir := mgr.GetWorkingDirectory(t)
		windowName := t.GetWindowName()
		logging.Trace("handleTaskCmd: creating task window name=%s workDir=%s", windowName, workDir)
		logging.Debug("Creating tmux window: session=%s, workDir=%s", sessionName, workDir)

		windowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     windowName,
			StartDir: workDir,
			Detached: true,
		})
		if err != nil {
			logging.Error("Failed to create tmux window: %v", err)
			_ = t.RemoveTabLock()
			return fmt.Errorf("failed to create window: %w", err)
		}
		logging.Trace("handleTaskCmd: task window created windowID=%s name=%s", windowID, windowName)
		logging.Debug("Tmux window created: windowID=%s, name=%s", windowID, windowName)

		// Save window ID
		if err := t.SaveWindowID(windowID); err != nil {
			logging.Warn("Failed to save window ID: %v", err)
		}
		if _, err := service.UpdateWindowMap(appCtx.PawDir, t.Name); err != nil {
			logging.Warn("Failed to update window map: %v", err)
		}

		if !isReopen {
			prevStatus, valid, err := t.TransitionStatus(task.StatusWorking)
			if err != nil {
				logging.Warn("Failed to persist working status: %v", err)
			} else {
				logging.Debug("Status set to working for new task")
			}
			if !valid {
				logging.Warn("Invalid status transition: %s -> %s", prevStatus, task.StatusWorking)
			}
			historyService := service.NewHistoryService(appCtx.GetHistoryDir())
			if err := historyService.RecordStatusTransition(t.Name, prevStatus, task.StatusWorking, "handle-task", "task started", valid); err != nil {
				logging.Warn("Failed to record status transition: %v", err)
			}
		}

		if !isReopen && appCtx.Config != nil && appCtx.Config.PreTaskHook != "" {
			hookEnv := appCtx.GetEnvVars(taskName, workDir, windowID)
			_, err := service.RunHook(
				"pre-task",
				appCtx.Config.PreTaskHook,
				workDir,
				hookEnv,
				t.GetHookOutputPath("pre-task"),
				t.GetHookMetaPath("pre-task"),
				constants.DefaultHookTimeout,
			)
			if err != nil {
				logging.Warn("Pre-task hook failed: %v", err)
			}
		}

		// Split window for user pane (error is non-fatal)
		taskFilePath := t.GetTaskFilePath()
		userPaneCmd := fmt.Sprintf("sh -c 'cat %s; echo; exec %s'", taskFilePath, getShell())
		if err := tm.SplitWindow(windowID, true, workDir, userPaneCmd); err != nil {
			logging.Warn("Failed to split window: %v", err)
		} else {
			logging.Trace("Window split for user pane: startDir=%s", workDir)
		}

		// Build system prompt
		globalPrompt, _ := embed.GetPrompt(appCtx.IsGitRepo)
		projectPrompt, _ := os.ReadFile(appCtx.GetPromptPath())
		systemPrompt := claude.BuildSystemPrompt(globalPrompt, string(projectPrompt))

		// Get paw binary path for end-task script
		pawBin, _ := os.Executable()
		// Use symlink path for PAW_BIN so running agents can use updated binary
		pawBinSymlink := filepath.Join(appCtx.PawDir, constants.BinSymlinkName)

		// Build user prompt with context
		userPrompt := buildUserPrompt(appCtx, t, taskName, workDir)

		// Save prompts (errors are non-fatal but should be logged)
		if err := os.WriteFile(t.GetSystemPromptPath(), []byte(systemPrompt), 0644); err != nil {
			logging.Warn("Failed to save system prompt: %v", err)
		}
		if err := os.WriteFile(t.GetUserPromptPath(), []byte(userPrompt), 0644); err != nil {
			logging.Warn("Failed to save user prompt: %v", err)
		}

		// Create task-specific end-task script (user-initiated only)
		endTaskScriptPath := filepath.Join(t.AgentDir, "end-task")
		endTaskContent := fmt.Sprintf(`#!/bin/bash
# Auto-generated end-task script for this task
# Finish is user-initiated (Ctrl+F). This script is retained for reference.
# PAW_DIR is set to ensure the correct project is found
export PAW_DIR="%s"
exec "%s" internal end-task "%s" "%s"
`, appCtx.PawDir, pawBin, sessionName, windowID)
		if err := os.WriteFile(endTaskScriptPath, []byte(endTaskContent), 0755); err != nil {
			logging.Warn("Failed to create end-task script: %v", err)
		} else {
			logging.Debug("End-task script created: %s", endTaskScriptPath)
		}

		// Create start-agent script to avoid shell escaping issues with tmux
		startAgentScriptPath := filepath.Join(t.AgentDir, "start-agent")
		startAgentContent := buildStartAgentScript(appCtx, t, taskOpts, windowID, workDir, systemPrompt, pawBin, pawBinSymlink, sessionName, isReopen)

		if err := os.WriteFile(startAgentScriptPath, []byte(startAgentContent), 0755); err != nil {
			logging.Warn("Failed to create start-agent script: %v", err)
		} else {
			logging.Debug("Start-agent script created: %s (resume=%v)", startAgentScriptPath, isReopen)
		}

		agentPane := windowID + ".0"

		if taskOpts.DependsOn != nil && taskOpts.DependsOn.TaskName != "" {
			if proceed := waitForDependency(appCtx, tm, windowID, t, taskOpts.DependsOn); !proceed {
				return nil
			}
		}

		// Start Claude using the start-agent script
		if err := tm.RespawnPane(agentPane, workDir, startAgentScriptPath); err != nil {
			return fmt.Errorf("failed to start Claude: %w", err)
		}

		// Wait for Claude to be ready
		logging.Debug("Waiting for Claude to be ready...")
		claudeClient := claude.New()
		claudeTimer := logging.StartTimer("Claude startup")
		if err := claudeClient.WaitForReady(tm, agentPane); err != nil {
			claudeTimer.StopWithResult(false, err.Error())
			logging.Warn("WaitForReady timed out, continuing anyway...")
		} else {
			claudeTimer.StopWithResult(true, "")
		}

		// Verify Claude is actually running (has content)
		verifyTimer := logging.StartTimer("Verify Claude alive")
		if err := claudeClient.VerifyPaneAlive(tm, agentPane, 10*time.Second); err != nil {
			verifyTimer.StopWithResult(false, err.Error())
			logging.Warn("Claude pane may not be alive: %v", err)
		} else {
			verifyTimer.StopWithResult(true, "")
		}

		// Send trust response if needed (error is non-fatal)
		if err := claudeClient.SendTrustResponse(tm, agentPane); err != nil {
			logging.Trace("Failed to send trust response: %v", err)
		} else {
			logging.Debug("Trust response sent")
		}

		// Wait a bit more for Claude to be fully ready
		time.Sleep(1 * time.Second)

		if isReopen {
			// Resume mode: don't clear history or send task instruction
			logging.Log("Session resumed: task=%s, windowID=%s", taskName, windowID)
		} else {
			// New task: clear screen and send task instruction
			startNewTaskSession(tm, claudeClient, agentPane, t, taskOpts, taskName, windowID)
		}

		// Start wait watcher to handle window status + notifications when user input is needed
		watchCmd := exec.Command(pawBin, "internal", "watch-wait", sessionName, windowID, taskName)
		watchCmd.Dir = appCtx.ProjectDir
		if err := watchCmd.Start(); err != nil {
			logging.Warn("Failed to start wait watcher: %v", err)
		} else {
			logging.Debug("Wait watcher started for windowID=%s", windowID)
		}

		// Notify user
		logging.Trace("handleTaskCmd: playing SoundTaskCreated for task=%s", taskName)
		notify.PlaySound(notify.SoundTaskCreated)
		if isReopen {
			_ = notify.Send("Session resumed", fmt.Sprintf("🔄 %s resumed", taskName))
			logging.Trace("handleTaskCmd: displaying session resumed message for task=%s", taskName)
			if err := tm.DisplayMessage(fmt.Sprintf("🔄 Session resumed: %s", taskName), 2000); err != nil {
				logging.Trace("Failed to display message: %v", err)
			}
		} else {
			_ = notify.Send("Task started", fmt.Sprintf("🤖 %s started", taskName))
			logging.Trace("handleTaskCmd: displaying task started message for task=%s", taskName)
			if err := tm.DisplayMessage(fmt.Sprintf("🤖 Task started: %s", taskName), 2000); err != nil {
				logging.Trace("Failed to display message: %v", err)
			}
		}

		return nil
	},
}

// buildUserPrompt constructs the user prompt for a task.
func buildUserPrompt(appCtx *app.App, t *task.Task, taskName, workDir string) string {
	var userPrompt strings.Builder
	userPrompt.WriteString(fmt.Sprintf("# Task: %s\n\n", taskName))
	if appCtx.IsWorktreeMode() {
		userPrompt.WriteString(fmt.Sprintf("**Worktree**: %s\n", workDir))
	}
	userPrompt.WriteString(fmt.Sprintf("**Project**: %s\n\n", appCtx.ProjectDir))

	// Add ON_COMPLETE setting
	userPrompt.WriteString(fmt.Sprintf("**ON_COMPLETE**: %s\n", appCtx.Config.OnComplete))
	userPrompt.WriteString("**Finish**: User triggers completion with Ctrl+F. Do not call end-task automatically.\n\n")

	// Add Plan Mode instructions (always shown since we start in plan mode)
	userPrompt.WriteString("## 📋 PLAN MODE (Required)\n\n")
	userPrompt.WriteString("You are starting in **Plan Mode**. Before writing any code:\n\n")
	userPrompt.WriteString("1. **Project analysis**: Identify build/test commands.\n")
	userPrompt.WriteString("2. **Write the Plan** including:\n")
	userPrompt.WriteString("   - Implementation steps\n")
	userPrompt.WriteString("   - **✅ How to validate success** (state whether automated verification is possible)\n")
	userPrompt.WriteString("3. Start implementation after the plan is ready.\n\n")

	// Add critical instruction for auto-merge mode
	if appCtx.Config.OnComplete == config.OnCompleteAutoMerge {
		userPrompt.WriteString("## ⚠️ AUTO-MERGE MODE (conditional)\n\n")
		userPrompt.WriteString("**Run auto-merge only after verification succeeds.**\n\n")
		userPrompt.WriteString("✅ **Auto-merge allowed when:**\n")
		userPrompt.WriteString("- The Plan marks the change as automatically verifiable\n")
		userPrompt.WriteString("- Build/tests/lint all pass\n\n")
		userPrompt.WriteString("❌ **Do NOT auto-merge when:**\n")
		userPrompt.WriteString("- Automated verification is not possible (UI/docs/config changes, etc.)\n")
		userPrompt.WriteString("- Tests are missing or not relevant\n")
		userPrompt.WriteString("- Verification fails\n\n")
		userPrompt.WriteString("**If verification succeeds:**\n")
		userPrompt.WriteString("→ Tell the user it's ready and ask them to press Ctrl+F to finish.\n\n")
		userPrompt.WriteString("**If verification is impossible or fails:**\n")
		userPrompt.WriteString("→ Explain the blocker and stop; PAW will set the window status automatically.\n\n")
	}

	userPrompt.WriteString("---\n\n")
	userPrompt.WriteString(t.Content)

	return userPrompt.String()
}

// buildStartAgentScript creates the start-agent script content.
func buildStartAgentScript(appCtx *app.App, t *task.Task, taskOpts *config.TaskOptions, windowID, workDir, systemPrompt, pawBin, pawBinSymlink, sessionName string, isReopen bool) string {
	taskName := t.Name
	worktreeDirExport := ""
	if workDir != "" && workDir != appCtx.ProjectDir {
		worktreeDirExport = fmt.Sprintf("export WORKTREE_DIR='%s'\n", workDir)
	}

	// Build model flag for Claude command
	modelFlag := ""
	if taskOpts.Model != "" && taskOpts.Model != config.DefaultModel {
		modelFlag = fmt.Sprintf(" --model %s", taskOpts.Model)
	}

	if isReopen {
		// Resume mode: use --continue to automatically continue previous session
		return fmt.Sprintf(`#!/bin/bash
# Auto-generated start-agent script for this task (RESUME MODE)
export TASK_NAME='%s'
export PAW_DIR='%s'
export PROJECT_DIR='%s'
%sexport WINDOW_ID='%s'
export ON_COMPLETE='%s'
export PAW_HOME='%s'
export PAW_BIN='%s'
export SESSION_NAME='%s'
export IS_DEMO='1'

# Continue the previous Claude session (--continue auto-selects last session)
exec claude --continue --dangerously-skip-permissions%s
`, taskName, appCtx.PawDir, appCtx.ProjectDir, worktreeDirExport, windowID,
			appCtx.Config.OnComplete, filepath.Dir(filepath.Dir(pawBin)), pawBinSymlink, sessionName, modelFlag)
	}

	// New session: start fresh with system prompt
	encodedPrompt := base64.StdEncoding.EncodeToString([]byte(systemPrompt))
	return fmt.Sprintf(`#!/bin/bash
# Auto-generated start-agent script for this task
export TASK_NAME='%s'
export PAW_DIR='%s'
export PROJECT_DIR='%s'
%sexport WINDOW_ID='%s'
export ON_COMPLETE='%s'
export PAW_HOME='%s'
export PAW_BIN='%s'
export SESSION_NAME='%s'
export IS_DEMO='1'

# System prompt is base64 encoded to avoid shell escaping issues
# Using heredoc with single-quoted delimiter prevents any shell interpretation
exec claude --dangerously-skip-permissions%s --system-prompt "$(base64 -d <<'__PROMPT_END__'
%s
__PROMPT_END__
)"
`, taskName, appCtx.PawDir, appCtx.ProjectDir, worktreeDirExport, windowID,
		appCtx.Config.OnComplete, filepath.Dir(filepath.Dir(pawBin)), pawBinSymlink, sessionName,
		modelFlag, encodedPrompt)
}

// startNewTaskSession handles the setup for a new (non-resumed) task session.
func startNewTaskSession(tm tmux.Client, claudeClient claude.Client, agentPane string, t *task.Task, taskOpts *config.TaskOptions, taskName, windowID string) {
	// Clear scrollback history and screen before sending task instruction
	if err := tm.ClearHistory(agentPane); err != nil {
		logging.Trace("Failed to clear history: %v", err)
	}
	if err := tm.SendKeys(agentPane, "C-l"); err != nil {
		logging.Trace("Failed to clear screen: %v", err)
	}

	// Small delay after clearing screen
	time.Sleep(200 * time.Millisecond)

	// Send task instruction - tell Claude to read from file
	var taskInstruction string
	if taskOpts.Ultrathink {
		taskInstruction = fmt.Sprintf("ultrathink Read and execute the task from '%s'", t.GetUserPromptPath())
	} else {
		taskInstruction = fmt.Sprintf("Read and execute the task from '%s'", t.GetUserPromptPath())
	}
	logging.Trace("Sending task instruction: length=%d, ultrathink=%v", len(taskInstruction), taskOpts.Ultrathink)
	if err := claudeClient.SendInputWithRetry(tm, agentPane, taskInstruction, 5); err != nil {
		logging.Warn("Failed to send task instruction: %v", err)
		// As a last resort, try the basic send
		if err := claudeClient.SendInput(tm, agentPane, taskInstruction); err != nil {
			logging.Error("Final attempt to send task instruction failed: %v", err)
		}
	}

	// Create session marker to track that Claude was started
	if err := t.CreateSessionMarker(); err != nil {
		logging.Warn("Failed to create session marker: %v", err)
	} else {
		logging.Debug("Session marker created for task: %s", taskName)
	}

	logging.Log("Task started successfully: name=%s, windowID=%s", taskName, windowID)

	// Wait for Claude's first output (⏺ spinner) and clear scrollback history
	go func() {
		if err := claudeClient.ScrollToFirstSpinner(tm, agentPane, 30*time.Second); err != nil {
			logging.Trace("Failed to scroll to first spinner: %v", err)
		} else {
			logging.Debug("Scrollback trimmed to first spinner for task: %s", taskName)
		}
	}()
}
