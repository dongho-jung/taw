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

	"github.com/donghojung/taw/internal/claude"
	"github.com/donghojung/taw/internal/config"
	"github.com/donghojung/taw/internal/constants"
	"github.com/donghojung/taw/internal/embed"
	"github.com/donghojung/taw/internal/logging"
	"github.com/donghojung/taw/internal/notify"
	"github.com/donghojung/taw/internal/task"
	"github.com/donghojung/taw/internal/tmux"
	"github.com/donghojung/taw/internal/tui"
)

var toggleNewCmd = &cobra.Command{
	Use:   "toggle-new [session]",
	Short: "Toggle the new task window",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer logger.Close()
			logger.SetScript("toggle-new")
			logging.SetGlobal(logger)
		}

		logging.Trace("toggleNewCmd: start session=%s", sessionName)
		defer logging.Trace("toggleNewCmd: end")

		tm := tmux.New(sessionName)

		// Check if _ window exists
		windows, err := tm.ListWindows()
		if err != nil {
			return err
		}

		for _, w := range windows {
			if strings.HasPrefix(w.Name, constants.EmojiNew) {
				// Window exists, just select it (don't send command again to avoid pasting into vim/editor)
				logging.Trace("toggleNewCmd: new task window already exists, selecting windowID=%s", w.ID)
				return tm.SelectWindow(w.ID)
			}
		}

		// Create new window without command (keeps shell open)
		logging.Trace("toggleNewCmd: creating new task window name=%s", constants.NewWindowName)
		windowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     constants.NewWindowName,
			StartDir: app.ProjectDir,
		})
		if err != nil {
			return err
		}
		logging.Trace("toggleNewCmd: new task window created windowID=%s", windowID)

		// Send new-task command to the new window
		tawBin, _ := os.Executable()
		newTaskCmd := fmt.Sprintf("%s internal new-task %s", tawBin, sessionName)
		if err := tm.SendKeysLiteral(windowID, newTaskCmd); err != nil {
			return fmt.Errorf("failed to send keys: %w", err)
		}
		if err := tm.SendKeys(windowID, "Enter"); err != nil {
			return fmt.Errorf("failed to send Enter: %w", err)
		}

		return nil
	},
}

var newTaskCmd = &cobra.Command{
	Use:   "new-task [session]",
	Short: "Create a new task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer logger.Close()
			logger.SetScript("new-task")
			logging.SetGlobal(logger)
		}

		// Loop continuously for task creation
		for {
			// Use inline task input TUI
			result, err := tui.RunTaskInput()
			if err != nil {
				fmt.Printf("Failed to get task input: %v\n", err)
				continue
			}

			if result.Cancelled {
				fmt.Println("Task creation cancelled.")
				continue
			}

			if result.Content == "" {
				fmt.Println("Task content is empty, try again.")
				continue
			}

			content := result.Content

			// Save content to temp file for spawn-task to read
			tmpFile, err := os.CreateTemp("", "taw-task-content-*.txt")
			if err != nil {
				fmt.Printf("Failed to create temp file: %v\n", err)
				continue
			}
			if _, err := tmpFile.WriteString(content); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				fmt.Printf("Failed to write task content: %v\n", err)
				continue
			}
			tmpFile.Close()

			// Spawn task creation in a separate window (non-blocking)
			tawBin, _ := os.Executable()
			spawnCmd := exec.Command(tawBin, "internal", "spawn-task", sessionName, tmpFile.Name())
			if err := spawnCmd.Start(); err != nil {
				os.Remove(tmpFile.Name())
				logging.Warn("Failed to start spawn-task: %v", err)
				fmt.Printf("Failed to start task: %v\n", err)
				continue
			}

			logging.Debug("Task spawned in background, content file: %s", tmpFile.Name())

			// Immediately loop back to create another task
			// The spawn-task process will handle everything in a separate window
		}
	},
}

var spawnTaskCmd = &cobra.Command{
	Use:   "spawn-task [session] [content-file]",
	Short: "Spawn a task in a separate window (shows progress)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Trace("spawnTaskCmd: start session=%s contentFile=%s", args[0], args[1])
		defer logging.Trace("spawnTaskCmd: end")

		sessionName := args[0]
		contentFile := args[1]

		// Read content from temp file
		contentBytes, err := os.ReadFile(contentFile)
		if err != nil {
			return fmt.Errorf("failed to read content file: %w", err)
		}
		content := string(contentBytes)

		// Clean up temp file
		os.Remove(contentFile)

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer logger.Close()
			logger.SetScript("spawn-task")
			logging.SetGlobal(logger)
		}

		tm := tmux.New(sessionName)
		tawBin, _ := os.Executable()

		// Create a temporary "⏳" window for progress display
		progressWindowName := "⏳..."
		logging.Trace("spawnTaskCmd: creating progress window name=%s", progressWindowName)
		progressWindowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     progressWindowName,
			StartDir: app.ProjectDir,
			Detached: true,
		})
		if err != nil {
			return fmt.Errorf("failed to create progress window: %w", err)
		}
		logging.Trace("spawnTaskCmd: progress window created windowID=%s", progressWindowID)

		logging.Debug("Created progress window: %s", progressWindowID)

		// Clean up progress window on exit (success or failure)
		defer func() {
			// Kill the progress window (it will be replaced by the actual task window)
			if err := tm.KillWindow(progressWindowID); err != nil {
				logging.Trace("Failed to kill progress window (may already be closed): %v", err)
			}
		}()

		// Run loading screen inside the progress window
		loadingCmd := fmt.Sprintf("sh -c %q", fmt.Sprintf("%s internal loading-screen 'Generating task name...'", tawBin))
		if err := tm.RespawnPane(progressWindowID+".0", app.ProjectDir, loadingCmd); err != nil {
			logging.Warn("Failed to run loading screen: %v", err)
		}

		// Create task (loading screen shows while this runs)
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
		newTask, err := mgr.CreateTask(content)
		if err != nil {
			logging.Error("Failed to create task: %v", err)
			return fmt.Errorf("failed to create task: %w", err)
		}

		logging.Log("Task created: %s", newTask.Name)

		// Handle task (creates actual window, starts Claude)
		handleCmd := exec.Command(tawBin, "internal", "handle-task", sessionName, newTask.AgentDir)
		if err := handleCmd.Start(); err != nil {
			logging.Warn("Failed to start handle-task: %v", err)
			return fmt.Errorf("failed to start task handler: %w", err)
		}

		// Wait for handle-task to create the window
		windowIDFile := filepath.Join(newTask.AgentDir, ".tab-lock", "window_id")
		for i := 0; i < 60; i++ { // 30 seconds max (60 * 500ms)
			if _, err := os.Stat(windowIDFile); err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		logging.Debug("Task window created for: %s", newTask.Name)

		return nil
	},
}

var handleTaskCmd = &cobra.Command{
	Use:   "handle-task [session] [agent-dir]",
	Short: "Handle a task (create window, start Claude)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Trace("handleTaskCmd: start session=%s agentDir=%s", args[0], args[1])
		defer logging.Trace("handleTaskCmd: end")

		sessionName := args[0]
		agentDir := args[1]

		taskName := filepath.Base(agentDir)

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer logger.Close()
			logger.SetScript("handle-task")
			logger.SetTask(taskName)
			logging.SetGlobal(logger)
		}

		logging.Debug("New task detected: name=%s, agentDir=%s", taskName, agentDir)

		// Get task
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
		t, err := mgr.GetTask(taskName)
		if err != nil {
			logging.Error("Failed to get task: %v", err)
			return err
		}
		logging.Trace("Task loaded: content_length=%d", len(t.Content))

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
		if app.IsGitRepo && app.Config.WorkMode == config.WorkModeWorktree {
			worktreeDir := t.GetWorktreeDir()
			if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
				// Worktree doesn't exist, create it
				timer := logging.StartTimer("worktree setup")
				if err := mgr.SetupWorktree(t); err != nil {
					timer.StopWithResult(false, err.Error())
					t.RemoveTabLock()
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
		} else if !app.IsGitRepo || app.Config.WorkMode == config.WorkModeMain {
			// Non-git mode or main mode: check session marker for reopen
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
		if err := t.SetupSymlinks(app.ProjectDir); err != nil {
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
			t.RemoveTabLock()
			return fmt.Errorf("failed to create window: %w", err)
		}
		logging.Trace("handleTaskCmd: task window created windowID=%s name=%s", windowID, windowName)
		logging.Debug("Tmux window created: windowID=%s, name=%s", windowID, windowName)

		// Save window ID
		if err := t.SaveWindowID(windowID); err != nil {
			logging.Warn("Failed to save window ID: %v", err)
		}

		// Split window for user pane (error is non-fatal)
		// Pass workDir so user pane starts in the worktree (if git mode) or project dir
		// Show task content first, then start shell
		taskFilePath := t.GetTaskFilePath()
		userPaneCmd := fmt.Sprintf("sh -c 'cat %s; echo; exec %s'", taskFilePath, getShell())
		if err := tm.SplitWindow(windowID, true, workDir, userPaneCmd); err != nil {
			logging.Warn("Failed to split window: %v", err)
		} else {
			logging.Trace("Window split for user pane: startDir=%s", workDir)
		}

		// Build system prompt
		globalPrompt, _ := embed.GetPrompt(app.IsGitRepo)
		projectPrompt, _ := os.ReadFile(app.GetPromptPath())
		systemPrompt := claude.BuildSystemPrompt(globalPrompt, string(projectPrompt))

		// Get taw binary path for end-task (needed for user prompt)
		tawBin, _ := os.Executable()

		// Build user prompt with context
		var userPrompt strings.Builder
		userPrompt.WriteString(fmt.Sprintf("# Task: %s\n\n", taskName))
		if app.IsGitRepo && app.Config.WorkMode == config.WorkModeWorktree {
			userPrompt.WriteString(fmt.Sprintf("**Worktree**: %s\n", workDir))
		}
		userPrompt.WriteString(fmt.Sprintf("**Project**: %s\n\n", app.ProjectDir))

		// Add ON_COMPLETE setting and end-task path for auto-merge
		userPrompt.WriteString(fmt.Sprintf("**ON_COMPLETE**: %s\n", app.Config.OnComplete))
		endTaskScriptPath := filepath.Join(t.AgentDir, "end-task")
		userPrompt.WriteString(fmt.Sprintf("**End-Task Script**: %s\n\n", endTaskScriptPath))

		// Add Plan Mode instructions (always shown since we start in plan mode)
		userPrompt.WriteString("## 📋 PLAN MODE (Required)\n\n")
		userPrompt.WriteString("You are starting in **Plan Mode**. Before writing any code:\n\n")
		userPrompt.WriteString("1. **Project analysis**: Identify build/test commands.\n")
		userPrompt.WriteString("2. **Write the Plan** including:\n")
		userPrompt.WriteString("   - Implementation steps\n")
		userPrompt.WriteString("   - **✅ How to validate success** (state whether automated verification is possible)\n")
		userPrompt.WriteString("3. Start implementation after the plan is ready.\n\n")

		// Add critical instruction for auto-merge mode
		if app.Config.OnComplete == config.OnCompleteAutoMerge {
			userPrompt.WriteString("## ⚠️ AUTO-MERGE MODE (conditional)\n\n")
			userPrompt.WriteString("**Run auto-merge only after verification succeeds.**\n\n")
			userPrompt.WriteString("✅ **Auto-merge allowed when:**\n")
			userPrompt.WriteString("- The Plan marks the change as automatically verifiable\n")
			userPrompt.WriteString("- Build/tests/lint all pass\n\n")
			userPrompt.WriteString("❌ **Do NOT auto-merge → switch to 💬 when:**\n")
			userPrompt.WriteString("- Automated verification is not possible (UI/docs/config changes, etc.)\n")
			userPrompt.WriteString("- Tests are missing or not relevant\n")
			userPrompt.WriteString("- Verification fails\n\n")
			userPrompt.WriteString("**If verification succeeds:**\n")
			userPrompt.WriteString(fmt.Sprintf("→ Run `%s`\n\n", endTaskScriptPath))
			userPrompt.WriteString("**If verification is impossible or fails:**\n")
			userPrompt.WriteString("→ `tmux rename-window \"💬...\"` and wait for user review\n\n")
		}

		userPrompt.WriteString("---\n\n")
		userPrompt.WriteString(t.Content)

		// Save prompts (errors are non-fatal but should be logged)
		if err := os.WriteFile(t.GetSystemPromptPath(), []byte(systemPrompt), 0644); err != nil {
			logging.Warn("Failed to save system prompt: %v", err)
		}
		if err := os.WriteFile(t.GetUserPromptPath(), []byte(userPrompt.String()), 0644); err != nil {
			logging.Warn("Failed to save user prompt: %v", err)
		}

		// Create task-specific end-task script
		// This allows Claude to call end-task without needing environment variables
		endTaskContent := fmt.Sprintf(`#!/bin/bash
# Auto-generated end-task script for this task
# Claude can call this directly without environment variables
exec "%s" internal end-task "%s" "%s"
`, tawBin, sessionName, windowID)
		if err := os.WriteFile(endTaskScriptPath, []byte(endTaskContent), 0755); err != nil {
			logging.Warn("Failed to create end-task script: %v", err)
		} else {
			logging.Debug("End-task script created: %s", endTaskScriptPath)
		}

		// Create start-agent script to avoid shell escaping issues with tmux
		// This script sets environment variables and starts Claude with the system prompt
		// The system prompt is base64 encoded to avoid issues with $, backticks, quotes, etc.
		startAgentScriptPath := filepath.Join(t.AgentDir, "start-agent")
		worktreeDirExport := ""
		if app.IsGitRepo && app.Config.WorkMode == config.WorkModeWorktree {
			worktreeDirExport = fmt.Sprintf("export WORKTREE_DIR='%s'\n", workDir)
		}

		var startAgentContent string
		if isReopen {
			// Resume mode: use --continue to automatically continue previous session
			startAgentContent = fmt.Sprintf(`#!/bin/bash
# Auto-generated start-agent script for this task (RESUME MODE)
export TASK_NAME='%s'
export TAW_DIR='%s'
export PROJECT_DIR='%s'
%sexport WINDOW_ID='%s'
export ON_COMPLETE='%s'
export TAW_HOME='%s'
export TAW_BIN='%s'
export SESSION_NAME='%s'

# Continue the previous Claude session (--continue auto-selects last session)
exec claude --continue --dangerously-skip-permissions
`, taskName, app.TawDir, app.ProjectDir, worktreeDirExport, windowID,
				app.Config.OnComplete, filepath.Dir(filepath.Dir(tawBin)), tawBin, sessionName)
			logging.Log("Session resume: using --continue flag for task %s", taskName)
		} else {
			// New session: start fresh with system prompt
			encodedPrompt := base64.StdEncoding.EncodeToString([]byte(systemPrompt))
			startAgentContent = fmt.Sprintf(`#!/bin/bash
# Auto-generated start-agent script for this task
export TASK_NAME='%s'
export TAW_DIR='%s'
export PROJECT_DIR='%s'
%sexport WINDOW_ID='%s'
export ON_COMPLETE='%s'
export TAW_HOME='%s'
export TAW_BIN='%s'
export SESSION_NAME='%s'

# System prompt is base64 encoded to avoid shell escaping issues
# Using heredoc with single-quoted delimiter prevents any shell interpretation
exec claude --dangerously-skip-permissions --system-prompt "$(base64 -d <<'__PROMPT_END__'
%s
__PROMPT_END__
)"
`, taskName, app.TawDir, app.ProjectDir, worktreeDirExport, windowID,
				app.Config.OnComplete, filepath.Dir(filepath.Dir(tawBin)), tawBin, sessionName,
				encodedPrompt)
		}

		if err := os.WriteFile(startAgentScriptPath, []byte(startAgentContent), 0755); err != nil {
			logging.Warn("Failed to create start-agent script: %v", err)
		} else {
			logging.Debug("Start-agent script created: %s (resume=%v)", startAgentScriptPath, isReopen)
		}

		agentPane := windowID + ".0"

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
			// Don't fail here - Claude might still work, continue and try to send input
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
			// Claude will resume the previous conversation with full context
			logging.Log("Session resumed: task=%s, windowID=%s", taskName, windowID)
		} else {
			// New task: clear screen and send task instruction
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
			taskInstruction := fmt.Sprintf("ultrathink Read and execute the task from '%s'", t.GetUserPromptPath())
			logging.Trace("Sending task instruction: length=%d", len(taskInstruction))
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
		}

		// Start wait watcher to handle window status + notifications when user input is needed
		watchCmd := exec.Command(tawBin, "internal", "watch-wait", sessionName, windowID, taskName)
		watchCmd.Dir = app.ProjectDir
		if err := watchCmd.Start(); err != nil {
			logging.Warn("Failed to start wait watcher: %v", err)
		} else {
			logging.Debug("Wait watcher started for windowID=%s", windowID)
		}

		// Notify user
		logging.Trace("handleTaskCmd: playing SoundTaskCreated for task=%s", taskName)
		notify.PlaySound(notify.SoundTaskCreated)
		if isReopen {
			logging.Trace("handleTaskCmd: displaying session resumed message for task=%s", taskName)
			if err := tm.DisplayMessage(fmt.Sprintf("🔄 Session resumed: %s", taskName), 2000); err != nil {
				logging.Trace("Failed to display message: %v", err)
			}
		} else {
			logging.Trace("handleTaskCmd: displaying task started message for task=%s", taskName)
			if err := tm.DisplayMessage(fmt.Sprintf("🤖 Task started: %s", taskName), 2000); err != nil {
				logging.Trace("Failed to display message: %v", err)
			}
		}

		return nil
	},
}
