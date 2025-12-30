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

	"github.com/donghojung/taw/internal/app"
	"github.com/donghojung/taw/internal/claude"
	"github.com/donghojung/taw/internal/config"
	"github.com/donghojung/taw/internal/constants"
	"github.com/donghojung/taw/internal/embed"
	"github.com/donghojung/taw/internal/git"
	"github.com/donghojung/taw/internal/logging"
	"github.com/donghojung/taw/internal/notify"
	"github.com/donghojung/taw/internal/task"
	"github.com/donghojung/taw/internal/tmux"
	"github.com/donghojung/taw/internal/tui"

	tea "github.com/charmbracelet/bubbletea/v2"
)

// internalCmd groups all internal commands
var internalCmd = &cobra.Command{
	Use:    "internal",
	Short:  "Internal commands (used by tmux keybindings)",
	Hidden: true,
}

func init() {
	internalCmd.AddCommand(toggleNewCmd)
	internalCmd.AddCommand(newTaskCmd)
	internalCmd.AddCommand(spawnTaskCmd)
	internalCmd.AddCommand(handleTaskCmd)
	internalCmd.AddCommand(watchWaitCmd)
	internalCmd.AddCommand(endTaskCmd)
	internalCmd.AddCommand(endTaskUICmd)
	internalCmd.AddCommand(processQueueCmd)
	internalCmd.AddCommand(quickTaskCmd)
	internalCmd.AddCommand(mergeCompletedCmd)
	internalCmd.AddCommand(popupShellCmd)
	internalCmd.AddCommand(toggleLogCmd)
	internalCmd.AddCommand(logViewerCmd)
	internalCmd.AddCommand(toggleHelpCmd)
	internalCmd.AddCommand(recoverTaskCmd)
	internalCmd.AddCommand(loadingScreenCmd)
	internalCmd.AddCommand(toggleTaskListCmd)
	internalCmd.AddCommand(taskListViewerCmd)

	// Add flags to end-task command
	endTaskCmd.Flags().StringVar(&paneCaptureFile, "pane-capture-file", "", "Path to pre-captured pane content file")
}

var toggleNewCmd = &cobra.Command{
	Use:   "toggle-new [session]",
	Short: "Toggle the new task window",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Check if _ window exists
		windows, err := tm.ListWindows()
		if err != nil {
			return err
		}

		for _, w := range windows {
			if strings.HasPrefix(w.Name, constants.EmojiNew) {
				// Window exists, just select it (don't send command again to avoid pasting into vim/editor)
				return tm.SelectWindow(w.ID)
			}
		}

		// Create new window without command (keeps shell open)
		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		windowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     constants.NewWindowName,
			StartDir: app.ProjectDir,
		})
		if err != nil {
			return err
		}

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
		progressWindowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     progressWindowName,
			StartDir: app.ProjectDir,
			Detached: true,
		})
		if err != nil {
			return fmt.Errorf("failed to create progress window: %w", err)
		}

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
			}
		}

		// Setup symlinks (error is non-fatal)
		if err := t.SetupSymlinks(app.ProjectDir); err != nil {
			logging.Warn("Failed to setup symlinks: %v", err)
		}

		// Create tmux window
		tm := tmux.New(sessionName)
		workDir := mgr.GetWorkingDirectory(t)
		logging.Debug("Creating tmux window: session=%s, workDir=%s", sessionName, workDir)

		windowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     t.GetWindowName(),
			StartDir: workDir,
			Detached: true,
		})
		if err != nil {
			logging.Error("Failed to create tmux window: %v", err)
			t.RemoveTabLock()
			return fmt.Errorf("failed to create window: %w", err)
		}
		logging.Debug("Tmux window created: windowID=%s, name=%s", windowID, t.GetWindowName())

		// Save window ID
		if err := t.SaveWindowID(windowID); err != nil {
			logging.Warn("Failed to save window ID: %v", err)
		}

		// Split window for user pane (error is non-fatal)
		// Pass workDir so user pane starts in the worktree (if git mode) or project dir
		// Show task content first, then start shell
		userPaneCmd := fmt.Sprintf("sh -c 'cat ../task; echo; exec %s'", getShell())
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

		encodedPrompt := base64.StdEncoding.EncodeToString([]byte(systemPrompt))
		startAgentContent := fmt.Sprintf(`#!/bin/bash
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

		if err := os.WriteFile(startAgentScriptPath, []byte(startAgentContent), 0755); err != nil {
			logging.Warn("Failed to create start-agent script: %v", err)
		} else {
			logging.Debug("Start-agent script created: %s", startAgentScriptPath)
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

		// Start wait watcher to handle window status + notifications when user input is needed
		watchCmd := exec.Command(tawBin, "internal", "watch-wait", sessionName, windowID, taskName)
		watchCmd.Dir = app.ProjectDir
		if err := watchCmd.Start(); err != nil {
			logging.Warn("Failed to start wait watcher: %v", err)
		} else {
			logging.Debug("Wait watcher started for windowID=%s", windowID)
		}

		logging.Log("Task started successfully: name=%s, windowID=%s", taskName, windowID)

		// Notify user that task has started
		notify.PlaySound(notify.SoundTaskCreated)
		if err := tm.DisplayMessage(fmt.Sprintf("🤖 Task started: %s", taskName), 2000); err != nil {
			logging.Trace("Failed to display message: %v", err)
		}

		return nil
	},
}

var paneCaptureFile string

var endTaskCmd = &cobra.Command{
	Use:   "end-task [session] [window-id]",
	Short: "End a task (commit, merge, cleanup)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Find task by window ID
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
		tasks, err := mgr.ListTasks()
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		var targetTask *task.Task
		for _, t := range tasks {
			if id, _ := t.LoadWindowID(); id == windowID {
				targetTask = t
				break
			}
		}

		if targetTask == nil {
			return fmt.Errorf("task not found for window %s", windowID)
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer logger.Close()
			logger.SetScript("end-task")
			logger.SetTask(targetTask.Name)
			logging.SetGlobal(logger)
		}

		logging.Log("=== End task: %s ===", targetTask.Name)

		// Print task header for user feedback
		fmt.Printf("\n  Ending task: %s\n\n", targetTask.Name)
		logging.Debug("Configuration: ON_COMPLETE=%s, WorkMode=%s", app.Config.OnComplete, app.Config.WorkMode)

		tm := tmux.New(sessionName)
		gitClient := git.New()
		workDir := mgr.GetWorkingDirectory(targetTask)
		logging.Trace("Working directory: %s", workDir)

		// Commit changes if git mode
		if app.IsGitRepo {
			hasChanges := gitClient.HasChanges(workDir)
			logging.Trace("Git status: hasChanges=%v", hasChanges)

			if hasChanges {
				spinner := tui.NewSimpleSpinner("Committing changes")
				spinner.Start()

				commitTimer := logging.StartTimer("git commit")
				if err := gitClient.AddAll(workDir); err != nil {
					logging.Warn("Failed to add changes: %v", err)
				}
				diffStat, _ := gitClient.GetDiffStat(workDir)
				logging.Trace("Changes: %s", strings.ReplaceAll(diffStat, "\n", ", "))
				message := fmt.Sprintf("chore: auto-commit on task end\n\n%s", diffStat)
				if err := gitClient.Commit(workDir, message); err != nil {
					commitTimer.StopWithResult(false, err.Error())
					spinner.Stop(false, err.Error())
				} else {
					commitTimer.StopWithResult(true, "")
					spinner.Stop(true, "")
				}
			} else {
				fmt.Println("  ○ No changes to commit")
			}

			// Push changes
			pushSpinner := tui.NewSimpleSpinner("Pushing to remote")
			pushSpinner.Start()

			pushTimer := logging.StartTimer("git push")
			if err := gitClient.Push(workDir, "origin", targetTask.Name, true); err != nil {
				pushTimer.StopWithResult(false, err.Error())
				pushSpinner.Stop(false, err.Error())
			} else {
				pushTimer.StopWithResult(true, fmt.Sprintf("branch=%s", targetTask.Name))
				pushSpinner.Stop(true, targetTask.Name)
			}

			// Handle auto-merge mode
			mergeSuccess := true // Track merge result to decide cleanup
			if app.Config != nil && app.Config.OnComplete == config.OnCompleteAutoMerge {
				logging.Log("auto-merge: starting merge process")
				fmt.Println()
				fmt.Println("  Auto-merge mode:")

				// Get main branch name
				mainBranch := gitClient.GetMainBranch(app.ProjectDir)
				logging.Debug("Main branch: %s", mainBranch)

				mergeTimer := logging.StartTimer("auto-merge")

				// Acquire merge lock to prevent concurrent merges
				// This is necessary because we need to checkout main in project dir
				lockSpinner := tui.NewSimpleSpinner("Acquiring merge lock")
				lockSpinner.Start()

				lockFile := filepath.Join(app.TawDir, "merge.lock")
				lockAcquired := false
				for retries := 0; retries < 30; retries++ {
					f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
					if err == nil {
						_, writeErr := f.WriteString(fmt.Sprintf("%s\n%d", targetTask.Name, os.Getpid()))
						closeErr := f.Close()
						if writeErr != nil || closeErr != nil {
							// Failed to write lock info, remove and retry
							os.Remove(lockFile)
							logging.Warn("Failed to write lock file: write=%v, close=%v", writeErr, closeErr)
							time.Sleep(100 * time.Millisecond)
							continue
						}
						lockAcquired = true
						break
					}
					logging.Trace("Waiting for merge lock (attempt %d/30)...", retries+1)
					time.Sleep(1 * time.Second)
				}

				if !lockAcquired {
					logging.Warn("Failed to acquire merge lock after 30 seconds")
					mergeTimer.StopWithResult(false, "lock timeout")
					lockSpinner.Stop(false, "timeout after 30s")
					mergeSuccess = false
				} else {
					lockSpinner.Stop(true, "")
					// Ensure lock is released on exit
					defer os.Remove(lockFile)

					// Stash any uncommitted changes in project dir
					hasLocalChanges := gitClient.HasChanges(app.ProjectDir)
					if hasLocalChanges {
						logging.Debug("Stashing local changes...")
						if err := gitClient.StashPush(app.ProjectDir, "taw-merge-temp"); err != nil {
							logging.Warn("Failed to stash changes: %v", err)
						}
					}

					// Remember current branch to restore later
					currentBranch, _ := gitClient.GetCurrentBranch(app.ProjectDir)

					// Fetch latest from origin
					fetchSpinner := tui.NewSimpleSpinner("Fetching from origin")
					fetchSpinner.Start()
					logging.Debug("Fetching from origin...")
					if err := gitClient.Fetch(app.ProjectDir, "origin"); err != nil {
						logging.Warn("Failed to fetch: %v", err)
						fetchSpinner.Stop(false, err.Error())
					} else {
						fetchSpinner.Stop(true, "")
					}

					// Checkout main
					checkoutSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Checking out %s", mainBranch))
					checkoutSpinner.Start()
					logging.Debug("Checking out %s...", mainBranch)
					if err := gitClient.Checkout(app.ProjectDir, mainBranch); err != nil {
						logging.Warn("Failed to checkout %s: %v", mainBranch, err)
						mergeTimer.StopWithResult(false, "checkout failed")
						checkoutSpinner.Stop(false, err.Error())
						mergeSuccess = false
					} else {
						checkoutSpinner.Stop(true, "")

						// Pull latest
						pullSpinner := tui.NewSimpleSpinner("Pulling latest changes")
						pullSpinner.Start()
						logging.Debug("Pulling latest changes...")
						if err := gitClient.Pull(app.ProjectDir); err != nil {
							logging.Warn("Failed to pull: %v", err)
							pullSpinner.Stop(false, err.Error())
						} else {
							pullSpinner.Stop(true, "")
						}

						// Merge task branch (squash)
						mergeSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Merging %s into %s", targetTask.Name, mainBranch))
						mergeSpinner.Start()
						logging.Debug("Squash merging branch %s into %s...", targetTask.Name, mainBranch)
						mergeMsg := fmt.Sprintf("feat: %s", targetTask.Name)
						if err := gitClient.MergeSquash(app.ProjectDir, targetTask.Name, mergeMsg); err != nil {
							logging.Warn("Merge failed: %v - may need manual resolution", err)
							// Abort merge on conflict
							if abortErr := gitClient.MergeAbort(app.ProjectDir); abortErr != nil {
								logging.Warn("Failed to abort merge: %v", abortErr)
							}
							mergeTimer.StopWithResult(false, "merge conflict")
							mergeSpinner.Stop(false, "conflict")
							mergeSuccess = false
						} else {
							mergeSpinner.Stop(true, "")

							// Push merged main
							pushMainSpinner := tui.NewSimpleSpinner(fmt.Sprintf("Pushing %s to origin", mainBranch))
							pushMainSpinner.Start()
							logging.Debug("Pushing merged main to origin...")
							if err := gitClient.Push(app.ProjectDir, "origin", mainBranch, false); err != nil {
								logging.Warn("Failed to push merged main: %v", err)
								mergeTimer.StopWithResult(false, "push failed")
								pushMainSpinner.Stop(false, err.Error())
								mergeSuccess = false
							} else {
								mergeTimer.StopWithResult(true, fmt.Sprintf("squash merged %s into %s", targetTask.Name, mainBranch))
								pushMainSpinner.Stop(true, "")
							}
						}

						// Restore original branch if different from main
						if currentBranch != "" && currentBranch != mainBranch {
							logging.Debug("Restoring branch %s...", currentBranch)
							if err := gitClient.Checkout(app.ProjectDir, currentBranch); err != nil {
								logging.Warn("Failed to restore branch: %v", err)
							}
						}
					}

					// Restore stashed changes
					if hasLocalChanges {
						logging.Debug("Restoring stashed changes...")
						if err := gitClient.StashPop(app.ProjectDir); err != nil {
							logging.Warn("Failed to restore stashed changes: %v", err)
						}
					}
				}

				// If merge failed, rename window to warning and skip cleanup
				if !mergeSuccess {
					logging.Warn("Merge failed - keeping task for manual resolution")
					fmt.Println()
					fmt.Println("  ✗ Merge failed - manual resolution needed")
					warningWindowName := constants.EmojiWarning + targetTask.Name
					if err := tm.RenameWindow(windowID, warningWindowName); err != nil {
						logging.Warn("Failed to rename window: %v", err)
					}
					// Notify user of merge failure
					notify.PlaySound(notify.SoundError)
					if err := tm.DisplayMessage(fmt.Sprintf("⚠️ Merge failed: %s - manual resolution needed", targetTask.Name), 3000); err != nil {
						logging.Trace("Failed to display message: %v", err)
					}
					return nil // Exit without cleanup - keep worktree and branch
				}
			}
		}
		fmt.Println()

		// Capture agent pane history before cleanup
		historyDir := app.GetHistoryDir()
		if err := os.MkdirAll(historyDir, 0755); err != nil {
			logging.Warn("Failed to create history directory: %v", err)
		} else {
			// Get pane content: either from pre-captured file or capture now
			var paneContent string
			var captureErr error
			if paneCaptureFile != "" {
				// Use pre-captured content (from end-task-ui)
				content, err := os.ReadFile(paneCaptureFile)
				if err != nil {
					logging.Warn("Failed to read pane capture file: %v", err)
					// Try to capture directly as fallback
					paneContent, captureErr = tm.CapturePane(windowID+".0", 10000)
				} else {
					paneContent = string(content)
					logging.Debug("Using pre-captured pane content from: %s", paneCaptureFile)
				}
				// Clean up temp file
				os.Remove(paneCaptureFile)
			} else {
				// Capture pane content directly (use a large number to get full history)
				paneContent, captureErr = tm.CapturePane(windowID+".0", 10000)
			}

			if captureErr != nil {
				logging.Warn("Failed to capture pane content: %v", captureErr)
			} else if paneContent != "" {
				// Generate summary using Claude
				summarySpinner := tui.NewSimpleSpinner("Generating summary")
				summarySpinner.Start()

				claudeClient := claude.New()
				summary, err := claudeClient.GenerateSummary(paneContent)
				if err != nil {
					logging.Warn("Failed to generate summary: %v", err)
					summarySpinner.Stop(false, "skipped")
					summary = "" // Continue without summary
				} else {
					logging.Debug("Generated summary: %d chars", len(summary))
					summarySpinner.Stop(true, "")
				}

				// Build history content: task + summary + pane capture
				var historyContent strings.Builder
				taskContent, _ := targetTask.LoadContent()
				if taskContent != "" {
					historyContent.WriteString(taskContent)
					historyContent.WriteString("\n---summary---\n")
				}
				if summary != "" {
					historyContent.WriteString(summary)
				}
				historyContent.WriteString("\n---capture---\n")
				historyContent.WriteString(paneContent)

				// Generate filename: YYMMDD_HHMMSS_taskname
				timestamp := time.Now().Format("060102_150405")
				historyFile := filepath.Join(historyDir, fmt.Sprintf("%s_%s", timestamp, targetTask.Name))
				if err := os.WriteFile(historyFile, []byte(historyContent.String()), 0644); err != nil {
					logging.Warn("Failed to write history file: %v", err)
				} else {
					logging.Debug("Agent history saved: %s", historyFile)
				}
			}
		}

		// Notify user that task completed successfully
		notify.PlaySound(notify.SoundTaskCompleted)
		if err := tm.DisplayMessage(fmt.Sprintf("✅ Task completed: %s", targetTask.Name), 2000); err != nil {
			logging.Trace("Failed to display message: %v", err)
		}

		// Cleanup task (only reached if merge succeeded or not in auto-merge mode)
		cleanupSpinner := tui.NewSimpleSpinner("Cleaning up")
		cleanupSpinner.Start()

		cleanupTimer := logging.StartTimer("task cleanup")
		if err := mgr.CleanupTask(targetTask); err != nil {
			cleanupTimer.StopWithResult(false, err.Error())
			cleanupSpinner.Stop(false, err.Error())
		} else {
			cleanupTimer.StopWithResult(true, "")
			cleanupSpinner.Stop(true, "")
		}

		// Kill window
		if err := tm.KillWindow(windowID); err != nil {
			logging.Warn("Failed to kill window: %v", err)
		}

		fmt.Println()
		fmt.Println("  ✓ Done!")

		// Process queue
		tawBin, _ := os.Executable()
		if err := exec.Command(tawBin, "internal", "process-queue", sessionName).Start(); err != nil {
			logging.Trace("Failed to start process-queue: %v", err)
		}

		return nil
	},
}

var endTaskUICmd = &cobra.Command{
	Use:   "end-task-ui [session] [window-id]",
	Short: "End task with UI feedback (creates visible pane)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]

		tm := tmux.New(sessionName)

		// IMPORTANT: Capture the agent pane content BEFORE creating the split pane
		// This is necessary because splitting shifts pane indices, causing windowID+".0"
		// to no longer be the agent pane
		paneContent, err := tm.CapturePane(windowID+".0", 10000)
		if err != nil {
			logging.Warn("Failed to pre-capture agent pane: %v", err)
			paneContent = "" // Continue anyway, end-task will try to capture directly
		}

		// Save captured content to temp file if we got it
		var capturePath string
		if paneContent != "" {
			tmpFile, err := os.CreateTemp("", "taw-pane-capture-*.txt")
			if err != nil {
				logging.Warn("Failed to create temp file for pane capture: %v", err)
			} else {
				if _, err := tmpFile.WriteString(paneContent); err != nil {
					logging.Warn("Failed to write pane capture to temp file: %v", err)
					tmpFile.Close()
					os.Remove(tmpFile.Name())
				} else {
					capturePath = tmpFile.Name()
					tmpFile.Close()
					logging.Debug("Pre-captured agent pane to: %s", capturePath)
				}
			}
		}

		// Get the taw binary path
		tawBin, err := os.Executable()
		if err != nil {
			tawBin = "taw"
		}

		// Get working directory from pane
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			app, err := getAppFromSession(sessionName)
			if err != nil {
				return err
			}
			panePath = app.ProjectDir
		}

		// Build end-task command that runs in the pane
		// Include pane-capture-file flag if we have pre-captured content
		var endTaskCmdStr string
		if capturePath != "" {
			endTaskCmdStr = fmt.Sprintf("%s internal end-task --pane-capture-file=%q %s %s; echo; echo 'Press Enter to close...'; read",
				tawBin, capturePath, sessionName, windowID)
		} else {
			endTaskCmdStr = fmt.Sprintf("%s internal end-task %s %s; echo; echo 'Press Enter to close...'; read",
				tawBin, sessionName, windowID)
		}

		// Create a top pane (40% height) spanning full window width
		_, err = tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false, // vertical split (top/bottom)
			Size:       "40%",
			StartDir:   panePath,
			Command:    endTaskCmdStr,
			Before:     true, // create pane above (top)
			Full:       true, // span entire window width
		})
		if err != nil {
			// Clean up temp file if we created one
			if capturePath != "" {
				os.Remove(capturePath)
			}
			return fmt.Errorf("failed to create end-task pane: %w", err)
		}

		return nil
	},
}

var processQueueCmd = &cobra.Command{
	Use:   "process-queue [session]",
	Short: "Process the next task in queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		queueMgr := task.NewQueueManager(app.QueueDir)
		queuedTask, err := queueMgr.Pop()
		if err != nil {
			return err
		}

		if queuedTask == nil {
			return nil // Queue is empty
		}

		// Create task from queue
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
		newTask, err := mgr.CreateTask(queuedTask.Content)
		if err != nil {
			return err
		}

		// Handle task
		tawBin, _ := os.Executable()
		handleCmd := exec.Command(tawBin, "internal", "handle-task", sessionName, newTask.AgentDir)
		return handleCmd.Start()
	},
}

var quickTaskCmd = &cobra.Command{
	Use:   "quick-task [session]",
	Short: "Add a quick task to the queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		tm := tmux.New(sessionName)

		// Use tmux display-popup to get input
		popupCmd := fmt.Sprintf("read -p 'Quick task: ' task && echo \"$task\" >> %s/.queue/$(date +%%s).task", app.TawDir)

		return tm.DisplayPopup(tmux.PopupOpts{
			Width:  "60",
			Height: "3",
			Title:  "Quick Task",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, fmt.Sprintf("bash -c '%s'", popupCmd))
	},
}

var mergeCompletedCmd = &cobra.Command{
	Use:   "merge-completed [session]",
	Short: "Merge all completed tasks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		if !app.IsGitRepo {
			return fmt.Errorf("merge-completed only works in git repositories")
		}

		tm := tmux.New(sessionName)
		gitClient := git.New()

		// Find windows with ✅ emoji
		windows, err := tm.ListWindows()
		if err != nil {
			return err
		}

		for _, w := range windows {
			if !strings.HasPrefix(w.Name, constants.EmojiDone) {
				continue
			}

			// Extract task name
			taskName := strings.TrimPrefix(w.Name, constants.EmojiDone)

			fmt.Printf("Merging task: %s\n", taskName)

			// Merge branch
			err := gitClient.Merge(app.ProjectDir, taskName, true, fmt.Sprintf("Merge branch '%s'", taskName))
			if err != nil {
				fmt.Printf("Failed to merge %s: %v\n", taskName, err)
				gitClient.MergeAbort(app.ProjectDir)
				continue
			}

			// End task
			tawBin, _ := os.Executable()
			exec.Command(tawBin, "internal", "end-task", sessionName, w.ID).Run()
		}

		return nil
	},
}

var popupShellCmd = &cobra.Command{
	Use:   "popup-shell [session]",
	Short: "Toggle shell pane at bottom 40%",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Check if shell pane exists
		paneID, _ := tm.GetOption("@taw_shell_pane_id")
		if paneID != "" && tm.HasPane(paneID) {
			// Shell pane exists, kill it and clear option
			tm.KillPane(paneID)
			tm.SetOption("@taw_shell_pane_id", "", true)
			return nil
		}

		// Get current pane's working directory (worktree path)
		panePath, err := tm.Display("#{pane_current_path}")
		if err != nil || panePath == "" {
			// Fallback to project dir
			app, err := getAppFromSession(sessionName)
			if err != nil {
				return err
			}
			panePath = app.ProjectDir
		}

		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
		shellName := filepath.Base(shell)

		// Build shell command with Alt+P binding to close pane
		// We create a temporary rcfile that binds Alt+P to exit
		var shellCmd string

		// Command to kill the shell pane and clear the option
		// Note: We'll get the pane_id after creation and update the cleanup command
		cleanupCmd := fmt.Sprintf("tmux -L \"taw-%s\" set-option -g @taw_shell_pane_id \"\" 2>/dev/null; exit", sessionName)

		switch shellName {
		case "zsh":
			// For zsh: create temp ZDOTDIR with .zshrc that binds Alt+P
			shellCmd = fmt.Sprintf(
				"TMPZD=$(mktemp -d) && "+
					"printf '%%s\\n' '[[ -f ~/.zshrc ]] && source ~/.zshrc' '_taw_close_shell() { %s; }' \"bindkey -s '\\\\ep' '\\\\C-u_taw_close_shell\\\\n'\" > \"$TMPZD/.zshrc\" && "+
					"ZDOTDIR=\"$TMPZD\" zsh; "+
					"rm -rf \"$TMPZD\" 2>/dev/null",
				cleanupCmd)
		default:
			// For bash: use --rcfile with temp file
			shellCmd = fmt.Sprintf(
				"TMPRC=$(mktemp) && "+
					"printf '%%s\\n' '[ -f ~/.bashrc ] && source ~/.bashrc' '_taw_close_shell() { %s; }' \"bind '\\\"\\\\ep\\\": \\\"\\\\C-u_taw_close_shell\\\\n\\\"'\" > \"$TMPRC\" && "+
					"bash --rcfile \"$TMPRC\"; "+
					"rm -f \"$TMPRC\" 2>/dev/null",
				cleanupCmd)
		}

		// Create shell pane at bottom 40% spanning full window width
		newPaneID, err := tm.SplitWindowPane(tmux.SplitOpts{
			Horizontal: false, // vertical split (top/bottom)
			Size:       "40%",
			StartDir:   panePath,
			Command:    shellCmd,
			Full:       true, // span entire window width
		})
		if err != nil {
			return fmt.Errorf("failed to create shell pane: %w", err)
		}

		// Store pane ID for toggle
		tm.SetOption("@taw_shell_pane_id", newPaneID, true)

		return nil
	},
}

var toggleLogCmd = &cobra.Command{
	Use:   "toggle-log [session]",
	Short: "Toggle log viewer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Check if log popup is open
		isOpen, _ := tm.GetOption("@taw_log_open")
		if isOpen == "1" {
			// Close popup using display-popup -C
			tm.SetOption("@taw_log_open", "", true)
			tm.Run("display-popup", "-C")
			return nil
		}

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		tm.SetOption("@taw_log_open", "1", true)

		logPath := app.GetLogPath()

		// Get the taw binary path
		tawBin, err := os.Executable()
		if err != nil {
			tawBin = "taw"
		}

		// Build command that clears state on exit
		logCmd := fmt.Sprintf("%s internal log-viewer %s; tmux -L 'taw-%s' set-option -g @taw_log_open '' 2>/dev/null || true",
			tawBin, logPath, sessionName)

		// Ignore error from DisplayPopup - the popup command (log-viewer) may exit
		// with non-zero and we don't want run-shell to show "...returned 1"
		tm.DisplayPopup(tmux.PopupOpts{
			Width:  "90%",
			Height: "80%",
			Title:  " Log Viewer (↑↓:scroll  g/G:top/end  s:tail  w:wrap  q:quit) ",
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

		// Check if help popup is open
		isOpen, _ := tm.GetOption("@taw_help_open")
		if isOpen == "1" {
			// Close popup using display-popup -C
			tm.SetOption("@taw_help_open", "", true)
			tm.Run("display-popup", "-C")
			return nil
		}

		tm.SetOption("@taw_help_open", "1", true)

		// Get help content from embedded assets
		helpContent, err := embed.GetHelp()
		if err != nil {
			return fmt.Errorf("failed to get help content: %w", err)
		}

		// Write to temp file
		tmpFile, err := os.CreateTemp("", "taw-help-*.md")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		if _, err := tmpFile.WriteString(helpContent); err != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to write help content: %w", err)
		}
		tmpFile.Close()

		// Build command with lesskey binding for Alt+/ to quit
		// Creates a temp keyfile, uses LESSKEYIN to load it, then cleans up
		popupCmd := fmt.Sprintf(
			"KEYFILE=$(mktemp) && printf '#command\\n\\\\e/ quit\\n' > \"$KEYFILE\" && "+
				"LESSKEYIN=\"$KEYFILE\" less '%s'; "+
				"rm -f '%s' \"$KEYFILE\" 2>/dev/null || true; "+
				"tmux -L 'taw-%s' set-option -g @taw_help_open '' 2>/dev/null || true",
			tmpPath, tmpPath, sessionName)

		// Ignore error from DisplayPopup - the popup command (less) may exit
		// with non-zero and we don't want run-shell to show "...returned 1"
		tm.DisplayPopup(tmux.PopupOpts{
			Width:  "80%",
			Height: "80%",
			Title:  " Help (⌃⇧/ or q to close) ",
			Close:  true,
			Style:  "fg=terminal,bg=terminal",
		}, popupCmd)
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

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
		t, err := mgr.GetTask(taskName)
		if err != nil {
			return err
		}

		recoveryMgr := task.NewRecoveryManager(app.ProjectDir)
		if err := recoveryMgr.RecoverTask(t); err != nil {
			return fmt.Errorf("failed to recover task: %w", err)
		}

		fmt.Printf("Task %s recovered successfully\n", taskName)
		return nil
	},
}

var loadingScreenCmd = &cobra.Command{
	Use:    "loading-screen [message]",
	Short:  "Show a loading screen with braille animation",
	Args:   cobra.MaximumNArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		message := "Generating task name..."
		if len(args) > 0 {
			message = args[0]
		}

		// Run the spinner TUI
		spinner := tui.NewSpinner(message)
		p := tea.NewProgram(spinner)

		// Block forever until killed (spawn-task will kill the window when done)
		_, err := p.Run()
		return err
	},
}

var toggleTaskListCmd = &cobra.Command{
	Use:   "toggle-task-list [session]",
	Short: "Toggle task list popup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		tm := tmux.New(sessionName)

		// Check if task list popup is open
		isOpen, _ := tm.GetOption("@taw_tasklist_open")
		if isOpen == "1" {
			// Close popup using display-popup -C
			tm.SetOption("@taw_tasklist_open", "", true)
			tm.Run("display-popup", "-C")
			return nil
		}

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		tm.SetOption("@taw_tasklist_open", "1", true)

		// Get the taw binary path
		tawBin, err := os.Executable()
		if err != nil {
			tawBin = "taw"
		}

		// Build command that clears state on exit
		listCmd := fmt.Sprintf("%s internal task-list-viewer %s; tmux -L 'taw-%s' set-option -g @taw_tasklist_open '' 2>/dev/null || true",
			tawBin, sessionName, sessionName)

		// Ignore error from DisplayPopup
		tm.DisplayPopup(tmux.PopupOpts{
			Width:     "90%",
			Height:    "80%",
			Title:     " Tasks (↑↓:nav  c:cancel  m:merge  p:push  r:resume  ⏎:focus  q:quit) ",
			Close:     true,
			Style:     "fg=terminal,bg=terminal",
			Directory: app.ProjectDir,
		}, listCmd)
		return nil
	},
}

var taskListViewerCmd = &cobra.Command{
	Use:    "task-list-viewer [session]",
	Short:  "Run the task list viewer",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		action, item, err := tui.RunTaskListUI(
			app.AgentsDir,
			app.GetHistoryDir(),
			app.ProjectDir,
			sessionName,
			app.TawDir,
			app.IsGitRepo,
		)
		if err != nil {
			return err
		}

		if item == nil {
			return nil
		}

		tm := tmux.New(sessionName)
		gitClient := git.New()
		tawBin, _ := os.Executable()

		switch action {
		case tui.TaskListActionSelect:
			// Focus the task window
			if item.WindowID != "" {
				return tm.SelectWindow(item.WindowID)
			}

		case tui.TaskListActionCancel:
			// Kill the window and cleanup
			if item.WindowID != "" {
				tm.KillWindow(item.WindowID)
			}
			mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
			if t, err := mgr.GetTask(item.Name); err == nil {
				mgr.CleanupTask(t)
			}

		case tui.TaskListActionMerge:
			// Trigger end-task for merge
			if item.WindowID != "" {
				endCmd := exec.Command(tawBin, "internal", "end-task", sessionName, item.WindowID)
				return endCmd.Start()
			}

		case tui.TaskListActionPush:
			// Push the branch
			if item.AgentDir != "" {
				worktreeDir := filepath.Join(item.AgentDir, "worktree")
				if _, err := os.Stat(worktreeDir); err == nil {
					// Commit any changes first
					if gitClient.HasChanges(worktreeDir) {
						gitClient.AddAll(worktreeDir)
						gitClient.Commit(worktreeDir, "chore: auto-commit before push")
					}
					return gitClient.Push(worktreeDir, "origin", item.Name, true)
				}
			}

		case tui.TaskListActionResume:
			// Resume a completed task from history
			if item.HistoryFile != "" && item.Content != "" {
				// Create a new task with the same content
				mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
				newTask, err := mgr.CreateTask(item.Content)
				if err != nil {
					return fmt.Errorf("failed to create task: %w", err)
				}

				// Handle the task
				handleCmd := exec.Command(tawBin, "internal", "handle-task", sessionName, newTask.AgentDir)
				return handleCmd.Start()
			}
		}

		return nil
	},
}

// getAppFromSession creates an App from session name
func getAppFromSession(sessionName string) (*app.App, error) {
	// Session name is the project directory name
	// We need to find the project directory

	// First, try to get it from environment
	if tawDir := os.Getenv("TAW_DIR"); tawDir != "" {
		projectDir := filepath.Dir(tawDir)
		application, err := app.New(projectDir)
		if err != nil {
			return nil, err
		}
		return loadAppConfig(application)
	}

	// Try current directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Walk up to find .taw directory
	dir := cwd
	for {
		tawDir := filepath.Join(dir, ".taw")
		if _, err := os.Stat(tawDir); err == nil {
			application, err := app.New(dir)
			if err != nil {
				return nil, err
			}
			return loadAppConfig(application)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return nil, fmt.Errorf("could not find project directory for session %s", sessionName)
}

func loadAppConfig(application *app.App) (*app.App, error) {
	tawHome, _ := getTawHome()
	application.SetTawHome(tawHome)

	gitClient := git.New()
	application.SetGitRepo(gitClient.IsGitRepo(application.ProjectDir))

	if err := application.LoadConfig(); err != nil {
		application.Config = config.DefaultConfig()
	}

	return application, nil
}

// getShell returns the user's preferred shell
func getShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return shell
}
