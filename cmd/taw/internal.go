package main

import (
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
	"github.com/donghojung/taw/internal/task"
	"github.com/donghojung/taw/internal/tmux"
	"github.com/donghojung/taw/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
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
	internalCmd.AddCommand(handleTaskCmd)
	internalCmd.AddCommand(endTaskCmd)
	internalCmd.AddCommand(endTaskUICmd)
	internalCmd.AddCommand(attachCmd)
	internalCmd.AddCommand(cleanupCmd)
	internalCmd.AddCommand(processQueueCmd)
	internalCmd.AddCommand(quickTaskCmd)
	internalCmd.AddCommand(mergeCompletedCmd)
	internalCmd.AddCommand(popupShellCmd)
	internalCmd.AddCommand(toggleLogCmd)
	internalCmd.AddCommand(logViewerCmd)
	internalCmd.AddCommand(toggleHelpCmd)
	internalCmd.AddCommand(recoverTaskCmd)
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

		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)

		// Loop continuously for task creation
		for {
			// Open editor for task content
			content, err := openEditor(app.ProjectDir)
			if err != nil {
				fmt.Printf("Failed to open editor: %v\n", err)
				continue
			}

			if strings.TrimSpace(content) == "" {
				fmt.Println("Task content is empty, try again.")
				continue
			}

			// Create task with spinner
			var newTask *task.Task
			spinner := tui.NewSpinner("태스크 이름 생성 중...")
			p := tea.NewProgram(spinner)

			// Run task creation in background
			go func() {
				t, err := mgr.CreateTask(content)
				if err != nil {
					p.Send(tui.SpinnerDoneMsg{Err: err})
					return
				}
				newTask = t
				p.Send(tui.SpinnerDoneMsg{Result: t.Name})
			}()

			finalModel, err := p.Run()
			if err != nil {
				fmt.Printf("Spinner error: %v\n", err)
				continue
			}

			spinnerResult := finalModel.(*tui.Spinner)
			if spinnerResult.GetError() != nil {
				fmt.Printf("Failed to create task: %v\n", spinnerResult.GetError())
				continue
			}

			logging.Log("Task created: %s", newTask.Name)

			// Handle task in background
			tawBin, _ := os.Executable()
			handleCmd := exec.Command(tawBin, "internal", "handle-task", sessionName, newTask.AgentDir)
			handleCmd.Start()

			// Wait for window to be created
			windowIDFile := filepath.Join(newTask.AgentDir, ".tab-lock", "window_id")
			for i := 0; i < 60; i++ { // 30 seconds max (60 * 500ms)
				if _, err := os.Stat(windowIDFile); err == nil {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}

			// Loop back to create another task
		}
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

		logging.Log("New task detected: name=%s, agentDir=%s", taskName, agentDir)

		// Get task
		mgr := task.NewManager(app.AgentsDir, app.ProjectDir, app.TawDir, app.IsGitRepo, app.Config)
		t, err := mgr.GetTask(taskName)
		if err != nil {
			logging.Error("Failed to get task: %v", err)
			return err
		}
		logging.Log("Task loaded: content_length=%d", len(t.Content))

		// Create tab-lock atomically
		created, err := t.CreateTabLock()
		if err != nil {
			logging.Error("Failed to create tab-lock: %v", err)
			return err
		}
		if !created {
			logging.Log("Task already being handled by another process")
			return nil
		}
		logging.Log("Tab-lock created successfully")

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
				logging.Log("Worktree already exists, reusing: %s", worktreeDir)
				t.WorktreeDir = worktreeDir
			}
		}

		// Setup symlinks (error is non-fatal)
		tawHome, _ := getTawHome()
		if err := t.SetupSymlinks(tawHome, app.ProjectDir); err != nil {
			logging.Warn("Failed to setup symlinks: %v", err)
		}

		// Create tmux window
		tm := tmux.New(sessionName)
		workDir := mgr.GetWorkingDirectory(t)
		logging.Log("Creating tmux window: session=%s, workDir=%s", sessionName, workDir)

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
		logging.Log("Tmux window created: windowID=%s, name=%s", windowID, t.GetWindowName())

		// Save window ID
		if err := t.SaveWindowID(windowID); err != nil {
			logging.Warn("Failed to save window ID: %v", err)
		}

		// Split window for user pane (error is non-fatal)
		// Pass workDir so user pane starts in the worktree (if git mode) or project dir
		if err := tm.SplitWindow(windowID, true, workDir, ""); err != nil {
			logging.Warn("Failed to split window: %v", err)
		} else {
			logging.Log("Window split for user pane: startDir=%s", workDir)
		}

		// Build system prompt
		globalPrompt, _ := os.ReadFile(app.GetGlobalPromptPath())
		projectPrompt, _ := os.ReadFile(app.GetPromptPath())
		systemPrompt := claude.BuildSystemPrompt(string(globalPrompt), string(projectPrompt))

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
		userPrompt.WriteString("## 📋 PLAN MODE (필수)\n\n")
		userPrompt.WriteString("You are starting in **Plan Mode**. Before writing any code:\n\n")
		userPrompt.WriteString("1. **프로젝트 분석**: 빌드/테스트 명령어 파악\n")
		userPrompt.WriteString("2. **Plan 작성** - 반드시 다음 포함:\n")
		userPrompt.WriteString("   - 작업 단계\n")
		userPrompt.WriteString("   - **✅ 성공 검증 방법** (자동 검증 가능 여부 명시)\n")
		userPrompt.WriteString("3. Plan 승인 후 작업 시작\n\n")

		// Add critical instruction for auto-merge mode
		if app.Config.OnComplete == config.OnCompleteAutoMerge {
			userPrompt.WriteString("## ⚠️ AUTO-MERGE MODE (조건부)\n\n")
			userPrompt.WriteString("**검증 성공 시에만 auto-merge 실행!**\n\n")
			userPrompt.WriteString("✅ **Auto-merge 허용 조건**:\n")
			userPrompt.WriteString("- Plan에서 '자동 검증 가능'으로 명시\n")
			userPrompt.WriteString("- 빌드/테스트/린트 모두 통과\n\n")
			userPrompt.WriteString("❌ **Auto-merge 금지 → 💬 상태로 전환**:\n")
			userPrompt.WriteString("- 자동 검증 불가 (UI 변경, 문서, 설정 등)\n")
			userPrompt.WriteString("- 테스트 없음 또는 관련 테스트 없음\n")
			userPrompt.WriteString("- 검증 실패\n\n")
			userPrompt.WriteString("**검증 성공 시**:\n")
			userPrompt.WriteString(fmt.Sprintf("→ `%s` 실행\n\n", endTaskScriptPath))
			userPrompt.WriteString("**검증 불가/실패 시**:\n")
			userPrompt.WriteString("→ `tmux rename-window \"💬...\"` 후 사용자 확인 대기\n\n")
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
			logging.Log("End-task script created: %s", endTaskScriptPath)
		}

		// Build environment variables and Claude command
		// These are used by PROMPT.md for auto-merge, auto-pr, etc.
		var envVars strings.Builder
		envVars.WriteString(fmt.Sprintf("export TASK_NAME='%s' ", taskName))
		envVars.WriteString(fmt.Sprintf("TAW_DIR='%s' ", app.TawDir))
		envVars.WriteString(fmt.Sprintf("PROJECT_DIR='%s' ", app.ProjectDir))
		if app.IsGitRepo && app.Config.WorkMode == config.WorkModeWorktree {
			envVars.WriteString(fmt.Sprintf("WORKTREE_DIR='%s' ", workDir))
		}
		envVars.WriteString(fmt.Sprintf("WINDOW_ID='%s' ", windowID))
		envVars.WriteString(fmt.Sprintf("ON_COMPLETE='%s' ", app.Config.OnComplete))
		envVars.WriteString(fmt.Sprintf("TAW_HOME='%s' ", filepath.Dir(filepath.Dir(tawBin))))
		envVars.WriteString(fmt.Sprintf("TAW_BIN='%s' ", tawBin))
		envVars.WriteString(fmt.Sprintf("SESSION_NAME='%s'", sessionName))

		claudeCmd := fmt.Sprintf("%s && claude --dangerously-skip-permissions --system-prompt \"$(cat '%s')\"",
			envVars.String(), t.GetSystemPromptPath())
		if err := tm.SendKeysLiteral(windowID+".0", claudeCmd); err != nil {
			return fmt.Errorf("failed to send Claude command: %w", err)
		}
		if err := tm.SendKeys(windowID+".0", "Enter"); err != nil {
			return fmt.Errorf("failed to send Enter: %w", err)
		}

		// Wait for Claude to be ready
		logging.Log("Waiting for Claude to be ready...")
		claudeClient := claude.New()
		claudeTimer := logging.StartTimer("Claude startup")
		if err := claudeClient.WaitForReady(tm, windowID+".0"); err != nil {
			claudeTimer.StopWithResult(false, err.Error())
		} else {
			claudeTimer.StopWithResult(true, "")
		}

		// Send trust response if needed (error is non-fatal)
		if err := claudeClient.SendTrustResponse(tm, windowID+".0"); err != nil {
			logging.Debug("Failed to send trust response: %v", err)
		} else {
			logging.Log("Trust response sent")
		}

		// Wait a bit more for Claude to be fully ready
		time.Sleep(500 * time.Millisecond)

		// Send task instruction - tell Claude to read from file
		taskInstruction := fmt.Sprintf("ultrathink Read and execute the task from '%s'", t.GetUserPromptPath())
		logging.Log("Sending task instruction: length=%d", len(taskInstruction))
		if err := claudeClient.SendInput(tm, windowID+".0", taskInstruction); err != nil {
			logging.Warn("Failed to send task instruction: %v", err)
		}

		logging.Log("Task started successfully: name=%s, windowID=%s", taskName, windowID)
		return nil
	},
}

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
		logging.Log("Configuration: ON_COMPLETE=%s, WorkMode=%s", app.Config.OnComplete, app.Config.WorkMode)

		tm := tmux.New(sessionName)
		gitClient := git.New()
		workDir := mgr.GetWorkingDirectory(targetTask)
		logging.Log("Working directory: %s", workDir)

		// Commit changes if git mode
		if app.IsGitRepo {
			hasChanges := gitClient.HasChanges(workDir)
			logging.Log("Git status: hasChanges=%v", hasChanges)

			if hasChanges {
				commitTimer := logging.StartTimer("git commit")
				if err := gitClient.AddAll(workDir); err != nil {
					logging.Warn("Failed to add changes: %v", err)
				}
				diffStat, _ := gitClient.GetDiffStat(workDir)
				logging.Log("Changes: %s", strings.ReplaceAll(diffStat, "\n", ", "))
				message := fmt.Sprintf("chore: auto-commit on task end\n\n%s", diffStat)
				if err := gitClient.Commit(workDir, message); err != nil {
					commitTimer.StopWithResult(false, err.Error())
				} else {
					commitTimer.StopWithResult(true, "")
				}
			}

			// Push changes
			pushTimer := logging.StartTimer("git push")
			if err := gitClient.Push(workDir, "origin", targetTask.Name, true); err != nil {
				pushTimer.StopWithResult(false, err.Error())
			} else {
				pushTimer.StopWithResult(true, fmt.Sprintf("branch=%s", targetTask.Name))
			}

			// Handle auto-merge mode
			mergeSuccess := true // Track merge result to decide cleanup
			if app.Config != nil && app.Config.OnComplete == config.OnCompleteAutoMerge {
				logging.Log("auto-merge: starting merge process")

				// Get main branch name
				mainBranch := gitClient.GetMainBranch(app.ProjectDir)
				logging.Log("Main branch: %s", mainBranch)

				mergeTimer := logging.StartTimer("auto-merge")

				// Acquire merge lock to prevent concurrent merges
				// This is necessary because we need to checkout main in project dir
				lockFile := filepath.Join(app.TawDir, "merge.lock")
				lockAcquired := false
				for retries := 0; retries < 30; retries++ {
					f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
					if err == nil {
						f.WriteString(fmt.Sprintf("%s\n%d", targetTask.Name, os.Getpid()))
						f.Close()
						lockAcquired = true
						break
					}
					logging.Log("Waiting for merge lock (attempt %d/30)...", retries+1)
					time.Sleep(1 * time.Second)
				}

				if !lockAcquired {
					logging.Warn("Failed to acquire merge lock after 30 seconds")
					mergeTimer.StopWithResult(false, "lock timeout")
					mergeSuccess = false
				} else {
					// Ensure lock is released on exit
					defer os.Remove(lockFile)

					// Stash any uncommitted changes in project dir
					hasLocalChanges := gitClient.HasChanges(app.ProjectDir)
					if hasLocalChanges {
						logging.Log("Stashing local changes...")
						if err := gitClient.StashPush(app.ProjectDir, "taw-merge-temp"); err != nil {
							logging.Warn("Failed to stash changes: %v", err)
						}
					}

					// Remember current branch to restore later
					currentBranch, _ := gitClient.GetCurrentBranch(app.ProjectDir)

					// Fetch latest from origin
					logging.Log("Fetching from origin...")
					if err := gitClient.Fetch(app.ProjectDir, "origin"); err != nil {
						logging.Warn("Failed to fetch: %v", err)
					}

					// Checkout main
					logging.Log("Checking out %s...", mainBranch)
					if err := gitClient.Checkout(app.ProjectDir, mainBranch); err != nil {
						logging.Warn("Failed to checkout %s: %v", mainBranch, err)
						mergeTimer.StopWithResult(false, "checkout failed")
						mergeSuccess = false
					} else {
						// Pull latest
						logging.Log("Pulling latest changes...")
						if err := gitClient.Pull(app.ProjectDir); err != nil {
							logging.Warn("Failed to pull: %v", err)
						}

						// Merge task branch (--no-ff)
						logging.Log("Merging branch %s into %s...", targetTask.Name, mainBranch)
						mergeMsg := fmt.Sprintf("Merge branch '%s'", targetTask.Name)
						if err := gitClient.Merge(app.ProjectDir, targetTask.Name, true, mergeMsg); err != nil {
							logging.Warn("Merge failed: %v - may need manual resolution", err)
							// Abort merge on conflict
							if abortErr := gitClient.MergeAbort(app.ProjectDir); abortErr != nil {
								logging.Warn("Failed to abort merge: %v", abortErr)
							}
							mergeTimer.StopWithResult(false, "merge conflict")
							mergeSuccess = false
						} else {
							// Push merged main
							logging.Log("Pushing merged main to origin...")
							if err := gitClient.Push(app.ProjectDir, "origin", mainBranch, false); err != nil {
								logging.Warn("Failed to push merged main: %v", err)
								mergeTimer.StopWithResult(false, "push failed")
								mergeSuccess = false
							} else {
								mergeTimer.StopWithResult(true, fmt.Sprintf("merged %s into %s", targetTask.Name, mainBranch))
							}
						}

						// Restore original branch if different from main
						if currentBranch != "" && currentBranch != mainBranch {
							logging.Log("Restoring branch %s...", currentBranch)
							if err := gitClient.Checkout(app.ProjectDir, currentBranch); err != nil {
								logging.Warn("Failed to restore branch: %v", err)
							}
						}
					}

					// Restore stashed changes
					if hasLocalChanges {
						logging.Log("Restoring stashed changes...")
						if err := gitClient.StashPop(app.ProjectDir); err != nil {
							logging.Warn("Failed to restore stashed changes: %v", err)
						}
					}
				}

				// If merge failed, rename window to warning and skip cleanup
				if !mergeSuccess {
					logging.Warn("Merge failed - keeping task for manual resolution")
					warningWindowName := constants.EmojiWarning + targetTask.Name
					if err := tm.RenameWindow(windowID, warningWindowName); err != nil {
						logging.Warn("Failed to rename window: %v", err)
					}
					return nil // Exit without cleanup - keep worktree and branch
				}
			}
		}

		// Capture agent pane history before cleanup
		historyDir := app.GetHistoryDir()
		if err := os.MkdirAll(historyDir, 0755); err != nil {
			logging.Warn("Failed to create history directory: %v", err)
		} else {
			// Capture pane content (use a large number to get full history)
			paneContent, err := tm.CapturePane(windowID+".0", 10000)
			if err != nil {
				logging.Warn("Failed to capture pane content: %v", err)
			} else {
				// Generate filename: YYMMDD_HHMMSS_taskname
				timestamp := time.Now().Format("060102_150405")
				historyFile := filepath.Join(historyDir, fmt.Sprintf("%s_%s", timestamp, targetTask.Name))
				if err := os.WriteFile(historyFile, []byte(paneContent), 0644); err != nil {
					logging.Warn("Failed to write history file: %v", err)
				} else {
					logging.Log("Agent history saved: %s", historyFile)
				}
			}
		}

		// Cleanup task (only reached if merge succeeded or not in auto-merge mode)
		cleanupTimer := logging.StartTimer("task cleanup")
		if err := mgr.CleanupTask(targetTask); err != nil {
			cleanupTimer.StopWithResult(false, err.Error())
		} else {
			cleanupTimer.StopWithResult(true, "")
		}

		// Kill window
		if err := tm.KillWindow(windowID); err != nil {
			logging.Warn("Failed to kill window: %v", err)
		}

		// Process queue
		tawBin, _ := os.Executable()
		if err := exec.Command(tawBin, "internal", "process-queue", sessionName).Start(); err != nil {
			logging.Debug("Failed to start process-queue: %v", err)
		}

		return nil
	},
}

var endTaskUICmd = &cobra.Command{
	Use:   "end-task-ui [session] [window-id]",
	Short: "End task with UI feedback",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// For now, just call end-task
		// TODO: Implement proper TUI with progress display
		return endTaskCmd.RunE(cmd, args)
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach [agent-dir]",
	Short: "Attach to an existing task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentDir := args[0]
		// TODO: Implement attach logic
		fmt.Printf("Attaching to task in %s\n", agentDir)
		return nil
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [task-name]",
	Short: "Cleanup a specific task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskName := args[0]
		// TODO: Implement cleanup logic
		fmt.Printf("Cleaning up task %s\n", taskName)
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
			Title:  " Help (⌥/ or q to close) ",
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

// openEditor opens an editor for task input
func openEditor(workDir string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "taw-task-*.md")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write template
	template := `# Task Description
# Lines starting with # will be ignored
# Describe your task below:

`
	if _, err := tmpFile.WriteString(template); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write template: %w", err)
	}
	tmpFile.Close()

	// Build editor command with options
	var cmd *exec.Cmd
	editorBase := filepath.Base(editor)

	// For vim/nvim, start in insert mode at the end of file
	if editorBase == "vim" || editorBase == "nvim" || editorBase == "vi" {
		cmd = exec.Command(editor, "-c", "normal G", "-c", "startinsert", tmpPath)
	} else {
		cmd = exec.Command(editor, tmpPath)
	}

	cmd.Dir = workDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	// Read content
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", err
	}

	// Remove comment lines
	lines := strings.Split(string(data), "\n")
	var contentLines []string
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			contentLines = append(contentLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(contentLines, "\n")), nil
}
