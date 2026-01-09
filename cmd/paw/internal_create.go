package main

import (
	"encoding/base64"
	"encoding/json"
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
	"github.com/dongho-jung/paw/internal/tui"
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
			defer func() { _ = logger.Close() }()
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

		// Wait for shell to be ready before sending keys
		// This prevents the race condition where keys are lost if sent before shell initializes
		paneID := windowID + ".0"
		if err := tm.WaitForPane(paneID, 5*time.Second, 1); err != nil {
			logging.Warn("toggleNewCmd: WaitForPane timed out, continuing anyway: %v", err)
		}

		// Send new-task command to the new window
		pawBin, _ := os.Executable()
		newTaskCmd := fmt.Sprintf("%s internal new-task %s", pawBin, sessionName)
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
			defer func() { _ = logger.Close() }()
			logger.SetScript("new-task")
			logging.SetGlobal(logger)
		}

		// Set project name for TUI display
		tui.SetProjectName(sessionName)

		// Initialize input history service
		inputHistorySvc := service.NewInputHistoryService(app.PawDir)

		// Get active task names for dependency selection
		activeTasks := getActiveTaskNames(app.AgentsDir)
		if !app.IsGitRepo && app.Config != nil && app.Config.NonGitWorkspace != string(config.NonGitWorkspaceCopy) && len(activeTasks) > 0 {
			logging.Warn("Non-git shared workspace: parallel tasks may conflict")
			fmt.Println("  ⚠️  Non-git shared workspace: parallel tasks may conflict")
		}

		// Track pre-filled content from history
		prefillContent := ""

		// Loop continuously for task creation
		for {
			// Use inline task input TUI with active task list and optional pre-filled content
			result, err := tui.RunTaskInputWithTasksAndContent(activeTasks, prefillContent)
			prefillContent = "" // Reset after use
			if err != nil {
				fmt.Printf("Failed to get task input: %v\n", err)
				continue
			}

			// Handle history search request
			if result.HistoryRequested {
				// Load history and show picker
				history, err := inputHistorySvc.GetAllContents()
				if err != nil {
					logging.Warn("Failed to load input history: %v", err)
					continue
				}
				if len(history) == 0 {
					fmt.Println("No task history yet.")
					continue
				}

				action, selected, err := tui.RunInputHistoryPicker(history)
				if err != nil {
					logging.Warn("Failed to run history picker: %v", err)
					continue
				}

				if action == tui.InputHistorySelect && selected != "" {
					prefillContent = selected
				}
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
			tmpFile, err := os.CreateTemp("", "paw-task-content-*.txt")
			if err != nil {
				fmt.Printf("Failed to create temp file: %v\n", err)
				continue
			}
			if _, err := tmpFile.WriteString(content); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
				fmt.Printf("Failed to write task content: %v\n", err)
				continue
			}
			_ = tmpFile.Close()

			// Save options to temp file
			var optsTmpPath string
			if result.Options != nil {
				optsTmpFile, err := os.CreateTemp("", "paw-task-opts-*.json")
				if err != nil {
					_ = os.Remove(tmpFile.Name())
					fmt.Printf("Failed to create options temp file: %v\n", err)
					continue
				}
				optsData, _ := json.Marshal(result.Options)
				if _, err := optsTmpFile.Write(optsData); err != nil {
					_ = optsTmpFile.Close()
					_ = os.Remove(optsTmpFile.Name())
					_ = os.Remove(tmpFile.Name())
					fmt.Printf("Failed to write task options: %v\n", err)
					continue
				}
				_ = optsTmpFile.Close()
				optsTmpPath = optsTmpFile.Name()
			}

			// Spawn task creation in a separate window (non-blocking)
			pawBin, _ := os.Executable()
			spawnArgs := []string{"internal", "spawn-task", sessionName, tmpFile.Name()}
			if optsTmpPath != "" {
				spawnArgs = append(spawnArgs, optsTmpPath)
			}
			spawnCmd := exec.Command(pawBin, spawnArgs...)
			if err := spawnCmd.Start(); err != nil {
				_ = os.Remove(tmpFile.Name())
				if optsTmpPath != "" {
					_ = os.Remove(optsTmpPath)
				}
				logging.Warn("Failed to start spawn-task: %v", err)
				fmt.Printf("Failed to start task: %v\n", err)
				continue
			}

			// Save content to input history (after successful spawn)
			if err := inputHistorySvc.SaveInput(content); err != nil {
				logging.Warn("Failed to save input history: %v", err)
			}

			logging.Debug("Task spawned in background, content file: %s, opts file: %s", tmpFile.Name(), optsTmpPath)

			// Refresh active tasks list
			activeTasks = getActiveTaskNames(app.AgentsDir)

			// Immediately loop back to create another task
			// The spawn-task process will handle everything in a separate window
		}
	},
}

// getActiveTaskNames returns a list of active task names from the agents directory.
func getActiveTaskNames(agentsDir string) []string {
	var names []string
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return names
	}
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

var spawnTaskCmd = &cobra.Command{
	Use:   "spawn-task [session] [content-file] [options-file]",
	Short: "Spawn a task in a separate window (shows progress)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Trace("spawnTaskCmd: start session=%s contentFile=%s", args[0], args[1])
		defer logging.Trace("spawnTaskCmd: end")

		sessionName := args[0]
		contentFile := args[1]
		var optsFile string
		if len(args) > 2 {
			optsFile = args[2]
		}

		// Read content from temp file
		contentBytes, err := os.ReadFile(contentFile)
		if err != nil {
			return fmt.Errorf("failed to read content file: %w", err)
		}
		content := string(contentBytes)

		// Read options from temp file if provided
		var taskOpts *config.TaskOptions
		if optsFile != "" {
			optsBytes, err := os.ReadFile(optsFile)
			if err != nil {
				logging.Warn("Failed to read options file: %v", err)
			} else {
				taskOpts = &config.TaskOptions{}
				if err := json.Unmarshal(optsBytes, taskOpts); err != nil {
					logging.Warn("Failed to parse options file: %v", err)
					taskOpts = nil
				}
			}
			_ = os.Remove(optsFile)
		}

		// Clean up temp file
		_ = os.Remove(contentFile)

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		// Setup logging
		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer func() { _ = logger.Close() }()
			logger.SetScript("spawn-task")
			logging.SetGlobal(logger)
		}

		tm := tmux.New(sessionName)
		pawBin, _ := os.Executable()

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
		loadingCmd := fmt.Sprintf("sh -c %q", fmt.Sprintf("%s internal loading-screen 'Generating task name...'", pawBin))
		if err := tm.RespawnPane(progressWindowID+".0", app.ProjectDir, loadingCmd); err != nil {
			logging.Warn("Failed to run loading screen: %v", err)
		}

		// Create task (loading screen shows while this runs)
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
		newTask, err := mgr.CreateTask(content)
		if err != nil {
			logging.Error("Failed to create task: %v", err)
			return fmt.Errorf("failed to create task: %w", err)
		}

		logging.Log("Task created: %s", newTask.Name)

		// Save task options if provided
		if taskOpts != nil {
			if err := taskOpts.Save(newTask.AgentDir); err != nil {
				logging.Warn("Failed to save task options: %v", err)
			} else {
				logging.Debug("Task options saved: model=%s, ultrathink=%v", taskOpts.Model, taskOpts.Ultrathink)
			}
		}

		// Handle task (creates actual window, starts Claude)
		handleCmd := exec.Command(pawBin, "internal", "handle-task", sessionName, newTask.AgentDir)
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
			defer func() { _ = logger.Close() }()
			logger.SetScript("handle-task")
			logger.SetTask(taskName)
			logging.SetGlobal(logger)
		}

		logging.Debug("New task detected: name=%s, agentDir=%s", taskName, agentDir)

		// Get task
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.PawDir, app.IsGitRepo, app.Config)
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
		if app.IsWorktreeMode() {
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
		} else if !app.IsGitRepo && app.Config != nil && app.Config.NonGitWorkspace == string(config.NonGitWorkspaceCopy) {
			workspaceDir := t.GetWorktreeDir()
			if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
				timer := logging.StartTimer("workspace setup")
				if err := mgr.SetupWorkspace(t); err != nil {
					timer.StopWithResult(false, err.Error())
					_ = t.RemoveTabLock()
					return fmt.Errorf("failed to setup workspace: %w", err)
				}
				timer.StopWithResult(true, fmt.Sprintf("path=%s", t.WorktreeDir))
			} else {
				logging.Debug("Workspace already exists, reusing: %s", workspaceDir)
				t.WorktreeDir = workspaceDir
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
			_ = t.RemoveTabLock()
			return fmt.Errorf("failed to create window: %w", err)
		}
		logging.Trace("handleTaskCmd: task window created windowID=%s name=%s", windowID, windowName)
		logging.Debug("Tmux window created: windowID=%s, name=%s", windowID, windowName)

		// Save window ID
		if err := t.SaveWindowID(windowID); err != nil {
			logging.Warn("Failed to save window ID: %v", err)
		}
		if _, err := service.UpdateWindowMap(app.PawDir, t.Name); err != nil {
			logging.Trace("Failed to update window map: %v", err)
		}

		if !isReopen {
			prevStatus, valid, err := t.TransitionStatus(task.StatusWorking)
			if err != nil {
				logging.Trace("Failed to persist working status: %v", err)
			} else {
				logging.Trace("Status set to working for new task")
			}
			if !valid {
				logging.Warn("Invalid status transition: %s -> %s", prevStatus, task.StatusWorking)
			}
			historyService := service.NewHistoryService(app.GetHistoryDir())
			if err := historyService.RecordStatusTransition(t.Name, prevStatus, task.StatusWorking, "handle-task", "task started", valid); err != nil {
				logging.Trace("Failed to record status transition: %v", err)
			}
		}

		if !isReopen && app.Config != nil && app.Config.PreTaskHook != "" {
			hookEnv := app.GetEnvVars(taskName, workDir, windowID)
			_, err := service.RunHook(
				"pre-task",
				app.Config.PreTaskHook,
				workDir,
				hookEnv,
				t.GetHookOutputPath("pre-task"),
				t.GetHookMetaPath("pre-task"),
				time.Duration(app.Config.VerifyTimeout)*time.Second,
			)
			if err != nil {
				logging.Warn("Pre-task hook failed: %v", err)
			}
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

		// Get paw binary path for end-task script
		pawBin, _ := os.Executable()
		// Use symlink path for PAW_BIN so running agents can use updated binary
		pawBinSymlink := filepath.Join(app.PawDir, constants.BinSymlinkName)

		// Build user prompt with context
		var userPrompt strings.Builder
		userPrompt.WriteString(fmt.Sprintf("# Task: %s\n\n", taskName))
		if app.IsWorktreeMode() {
			userPrompt.WriteString(fmt.Sprintf("**Worktree**: %s\n", workDir))
		} else if app.Config != nil && !app.IsGitRepo && app.Config.NonGitWorkspace == string(config.NonGitWorkspaceCopy) {
			userPrompt.WriteString(fmt.Sprintf("**Workspace**: %s\n", workDir))
		}
		userPrompt.WriteString(fmt.Sprintf("**Project**: %s\n\n", app.ProjectDir))

		// Add ON_COMPLETE setting
		userPrompt.WriteString(fmt.Sprintf("**ON_COMPLETE**: %s\n", app.Config.OnComplete))
		userPrompt.WriteString("**Finish**: User triggers completion with Ctrl+F. Do not call end-task automatically.\n\n")
		endTaskScriptPath := filepath.Join(t.AgentDir, "end-task")

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

		// Save prompts (errors are non-fatal but should be logged)
		if err := os.WriteFile(t.GetSystemPromptPath(), []byte(systemPrompt), 0644); err != nil {
			logging.Warn("Failed to save system prompt: %v", err)
		}
		if err := os.WriteFile(t.GetUserPromptPath(), []byte(userPrompt.String()), 0644); err != nil {
			logging.Warn("Failed to save user prompt: %v", err)
		}

		// Create task-specific end-task script (user-initiated only)
		endTaskContent := fmt.Sprintf(`#!/bin/bash
# Auto-generated end-task script for this task
# Finish is user-initiated (Ctrl+F). This script is retained for reference.
exec "%s" internal end-task "%s" "%s"
`, pawBin, sessionName, windowID)
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
		if workDir != "" && workDir != app.ProjectDir {
			worktreeDirExport = fmt.Sprintf("export WORKTREE_DIR='%s'\n", workDir)
		}

		// Build model flag for Claude command
		modelFlag := ""
		if taskOpts.Model != "" && taskOpts.Model != config.DefaultModel {
			modelFlag = fmt.Sprintf(" --model %s", taskOpts.Model)
		}

		var startAgentContent string
		if isReopen {
			// Resume mode: use --continue to automatically continue previous session
			startAgentContent = fmt.Sprintf(`#!/bin/bash
# Auto-generated start-agent script for this task (RESUME MODE)
export TASK_NAME='%s'
export PAW_DIR='%s'
export PROJECT_DIR='%s'
%sexport WINDOW_ID='%s'
export ON_COMPLETE='%s'
export PAW_HOME='%s'
export PAW_BIN='%s'
export SESSION_NAME='%s'

# Continue the previous Claude session (--continue auto-selects last session)
exec claude --continue --dangerously-skip-permissions%s
`, taskName, app.PawDir, app.ProjectDir, worktreeDirExport, windowID,
				app.Config.OnComplete, filepath.Dir(filepath.Dir(pawBin)), pawBinSymlink, sessionName, modelFlag)
			logging.Log("Session resume: using --continue flag for task %s", taskName)
		} else {
			// New session: start fresh with system prompt
			encodedPrompt := base64.StdEncoding.EncodeToString([]byte(systemPrompt))
			startAgentContent = fmt.Sprintf(`#!/bin/bash
# Auto-generated start-agent script for this task
export TASK_NAME='%s'
export PAW_DIR='%s'
export PROJECT_DIR='%s'
%sexport WINDOW_ID='%s'
export ON_COMPLETE='%s'
export PAW_HOME='%s'
export PAW_BIN='%s'
export SESSION_NAME='%s'

# System prompt is base64 encoded to avoid shell escaping issues
# Using heredoc with single-quoted delimiter prevents any shell interpretation
exec claude --dangerously-skip-permissions%s --system-prompt "$(base64 -d <<'__PROMPT_END__'
%s
__PROMPT_END__
)"
`, taskName, app.PawDir, app.ProjectDir, worktreeDirExport, windowID,
				app.Config.OnComplete, filepath.Dir(filepath.Dir(pawBin)), pawBinSymlink, sessionName,
				modelFlag, encodedPrompt)
		}

		if err := os.WriteFile(startAgentScriptPath, []byte(startAgentContent), 0755); err != nil {
			logging.Warn("Failed to create start-agent script: %v", err)
		} else {
			logging.Debug("Start-agent script created: %s (resume=%v)", startAgentScriptPath, isReopen)
		}

		agentPane := windowID + ".0"

		if taskOpts.DependsOn != nil && taskOpts.DependsOn.TaskName != "" {
			if proceed := waitForDependency(app, tm, windowID, t, taskOpts.DependsOn); !proceed {
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
			// Include "ultrathink" prefix if enabled in task options
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
			// This prevents scrolling back to the banner and initial command
			go func() {
				if err := claudeClient.ScrollToFirstSpinner(tm, agentPane, 30*time.Second); err != nil {
					logging.Trace("Failed to scroll to first spinner: %v", err)
				} else {
					logging.Debug("Scrollback trimmed to first spinner for task: %s", taskName)
				}
			}()
		}

		// Start wait watcher to handle window status + notifications when user input is needed
		watchCmd := exec.Command(pawBin, "internal", "watch-wait", sessionName, windowID, taskName)
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
			// Send to all configured notification channels (macOS, Slack, ntfy)
			notify.SendAll(app.Config.Notifications, "Session resumed", fmt.Sprintf("🔄 %s resumed", taskName))
			logging.Trace("handleTaskCmd: displaying session resumed message for task=%s", taskName)
			if err := tm.DisplayMessage(fmt.Sprintf("🔄 Session resumed: %s", taskName), 2000); err != nil {
				logging.Trace("Failed to display message: %v", err)
			}
		} else {
			// Send to all configured notification channels (macOS, Slack, ntfy)
			notify.SendAll(app.Config.Notifications, "Task started", fmt.Sprintf("🤖 %s started", taskName))
			logging.Trace("handleTaskCmd: displaying task started message for task=%s", taskName)
			if err := tm.DisplayMessage(fmt.Sprintf("🤖 Task started: %s", taskName), 2000); err != nil {
				logging.Trace("Failed to display message: %v", err)
			}
		}

		return nil
	},
}

func waitForDependency(app *app.App, tm tmux.Client, windowID string, t *task.Task, dep *config.TaskDependency) bool {
	if dep == nil || dep.TaskName == "" || dep.Condition == config.DependsOnNone {
		return true
	}
	if dep.TaskName == t.Name {
		logging.Warn("Dependency points to same task: %s", dep.TaskName)
		return true
	}

	firstWait := true
	for {
		status, found, terminal := resolveDependencyStatus(app, dep.TaskName)
		if !found {
			logging.Warn("Dependency task not found: %s", dep.TaskName)
			return true
		}

		if dependencySatisfied(dep.Condition, status) {
			if !firstWait {
				workingName := windowNameForStatus(t.Name, task.StatusWorking)
				_ = renameWindowWithStatus(tm, windowID, workingName, app.PawDir, t.Name, "depends-on")
			}
			return true
		}

		if terminal {
			warningName := windowNameForStatus(t.Name, task.StatusCorrupted)
			_ = renameWindowWithStatus(tm, windowID, warningName, app.PawDir, t.Name, "depends-on")
			msg := fmt.Sprintf("⚠️ Dependency %s did not satisfy %s", dep.TaskName, dep.Condition)
			_ = tm.DisplayMessage(msg, 3000)
			logging.Warn("Dependency %s ended with %s; blocking task %s", dep.TaskName, status, t.Name)
			return false
		}

		if firstWait {
			waitName := windowNameForStatus(t.Name, task.StatusWaiting)
			_ = renameWindowWithStatus(tm, windowID, waitName, app.PawDir, t.Name, "depends-on")
			msg := fmt.Sprintf("⏳ Waiting for %s (%s)", dep.TaskName, dep.Condition)
			_ = tm.DisplayMessage(msg, 3000)
			firstWait = false
		}

		time.Sleep(5 * time.Second)
	}
}

func dependencySatisfied(condition config.DependsOnCondition, status task.Status) bool {
	switch condition {
	case config.DependsOnSuccess:
		return status == task.StatusDone
	case config.DependsOnFailure:
		return status == task.StatusCorrupted
	case config.DependsOnAlways:
		return status == task.StatusDone || status == task.StatusCorrupted
	default:
		return true
	}
}

func resolveDependencyStatus(app *app.App, taskName string) (task.Status, bool, bool) {
	agentDir := app.GetAgentDir(taskName)
	if _, err := os.Stat(agentDir); err == nil {
		depTask := task.New(taskName, agentDir)
		status, err := depTask.LoadStatus()
		if err != nil {
			logging.Trace("Dependency status read failed task=%s err=%v", taskName, err)
			return task.StatusWorking, true, false
		}
		return status, true, status == task.StatusDone || status == task.StatusCorrupted
	}

	historyService := service.NewHistoryService(app.GetHistoryDir())
	historyFiles, err := historyService.ListHistoryFiles()
	if err != nil {
		logging.Trace("Dependency history lookup failed task=%s err=%v", taskName, err)
		return "", false, false
	}

	for _, file := range historyFiles {
		if service.ExtractTaskName(file) != taskName {
			continue
		}
		if service.IsCancelled(file) {
			return task.StatusCorrupted, true, true
		}
		return task.StatusDone, true, true
	}

	return "", false, false
}
