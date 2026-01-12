package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

// startNewSession creates a new tmux session
func startNewSession(appCtx *app.App, tm tmux.Client) error {
	logging.Debug("Starting new tmux session...")

	// Create/update bin symlink for hook compatibility
	if _, err := updateBinSymlink(appCtx.PawDir); err != nil {
		logging.Warn("Failed to create bin symlink: %v", err)
	}

	// Save current version
	if err := saveVersion(appCtx.PawDir); err != nil {
		logging.Warn("Failed to save version: %v", err)
	}

	// Clean up merged tasks before starting new session
	mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)

	// Prune stale worktree entries first to prevent git errors
	mgr.PruneWorktrees()

	merged, err := mgr.FindMergedTasks()
	if err == nil && len(merged) > 0 {
		logging.Log("Found %d merged tasks to clean up", len(merged))
		for _, t := range merged {
			logging.Log("Auto-cleaning merged task: %s", t.Name)
			_ = mgr.CleanupTask(t)
			fmt.Printf("✅ Cleaned up merged task: %s\n", t.Name)
		}
	}

	// Get paw binary path for initial command
	pawBin, err := os.Executable()
	if err != nil {
		pawBin = "paw"
	}

	// Create session with a shell (not the new-task command directly)
	if err := tm.NewSession(tmux.SessionOpts{
		Name:       appCtx.SessionName,
		StartDir:   appCtx.ProjectDir,
		WindowName: constants.NewWindowName,
		Detached:   true,
	}); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Setup tmux configuration
	if err := setupTmuxConfig(appCtx, tm); err != nil {
		logging.Warn("Failed to setup tmux config: %v", err)
	}

	// Setup git repo marker if applicable
	if appCtx.IsGitRepo {
		markerPath := filepath.Join(appCtx.PawDir, constants.GitRepoMarker)
		_ = os.WriteFile(markerPath, []byte{}, 0644)
	}

	// Write embedded claude files to .paw/.claude/
	claudeDir := filepath.Join(appCtx.PawDir, constants.ClaudeLink)
	if err := embed.WriteClaudeFiles(claudeDir); err != nil {
		logging.Warn("Failed to write claude files: %v", err)
	}

	// Write PAW help file for agents to .paw/
	if err := embed.WritePawHelpFile(appCtx.PawDir); err != nil {
		logging.Warn("Failed to write PAW help file: %v", err)
	}

	// Update .gitignore (only if using local workspace)
	if appCtx.IsGitRepo && !appCtx.IsGlobalWorkspace() {
		updateGitignore(appCtx.ProjectDir)
	}

	// Reopen incomplete tasks (tasks with worktree but no window)
	mgr.SetTmuxClient(tm)
	incomplete, err := mgr.FindIncompleteTasks(appCtx.SessionName)
	if err == nil && len(incomplete) > 0 {
		logging.Log("Found %d incomplete tasks to reopen", len(incomplete))
		for _, t := range incomplete {
			logging.Log("Reopening incomplete task: %s", t.Name)
			_ = t.RemoveTabLock()
			handleCmd := exec.Command(pawBin, "internal", "handle-task", appCtx.SessionName, t.AgentDir)
			if err := handleCmd.Start(); err != nil {
				logging.Warn("Failed to reopen task %s: %v", t.Name, err)
			} else {
				fmt.Printf("🔄 Reopening task: %s\n", t.Name)
			}
		}
	}

	// Wait for shell to be ready before sending keys
	paneTarget := appCtx.SessionName + ":" + constants.NewWindowName + ".0"
	if err := tm.WaitForPane(paneTarget, 5*time.Second, 1); err != nil {
		logging.Warn("WaitForPane timed out, continuing anyway: %v", err)
	}

	// Send new-task command to the _ window
	newTaskCmd := fmt.Sprintf("%s internal new-task %s", pawBin, appCtx.SessionName)
	_ = tm.SendKeysLiteral(appCtx.SessionName+":"+constants.NewWindowName, newTaskCmd)
	_ = tm.SendKeys(appCtx.SessionName+":"+constants.NewWindowName, "Enter")

	// Attach to session
	return tm.AttachSession(appCtx.SessionName)
}

// attachToSession attaches to an existing session
func attachToSession(appCtx *app.App, tm tmux.Client) error {
	logging.Debug("Running pre-attach cleanup and recovery...")

	// Check if PAW version has changed and update symlink
	versionChanged := checkVersionChanged(appCtx.PawDir)
	if versionChanged {
		logging.Log("PAW version changed, updating bin symlink...")
		if _, err := updateBinSymlink(appCtx.PawDir); err != nil {
			logging.Warn("Failed to update bin symlink: %v", err)
		} else {
			logging.Log("Bin symlink updated to current version")
		}

		// Save new version
		if err := saveVersion(appCtx.PawDir); err != nil {
			logging.Warn("Failed to save version: %v", err)
		}

		// Notify user about the upgrade
		fmt.Printf("🔄 PAW upgraded to %s\n", Version)

		// Respawn the main window so it uses the new binary
		if err := respawnMainWindow(appCtx, tm); err != nil {
			logging.Warn("Failed to respawn main window: %v", err)
		}
	}

	// Ensure .claude directory exists with settings.local.json (for stop-hook support)
	// This is critical for task status updates - without it, tasks stay "working" forever.
	// The .claude directory may be missing if:
	// - Workspace was created before .claude support was added
	// - WriteClaudeFiles failed silently during initial setup
	// - User is using a global workspace (paw_in_project: false)
	claudeDir := filepath.Join(appCtx.PawDir, constants.ClaudeLink)
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		logging.Log("Creating missing .claude directory for stop-hook support...")
		if err := embed.WriteClaudeFiles(claudeDir); err != nil {
			logging.Warn("Failed to write claude files: %v", err)
		} else {
			logging.Log("Claude files created successfully")
		}
	}

	// Run cleanup and recovery before attaching
	mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
	mgr.SetTmuxClient(tm)

	// Prune stale worktree entries first to prevent git errors
	mgr.PruneWorktrees()

	// Auto cleanup merged tasks
	merged, err := mgr.FindMergedTasks()
	if err == nil && len(merged) > 0 {
		logging.Log("Found %d merged tasks to clean up", len(merged))

		// Build a map of task name -> window ID for quick lookup
		taskWindowMap := make(map[string]string)
		if windows, err := tm.ListWindows(); err == nil {
			for _, w := range windows {
				if taskName, ok := constants.ExtractTaskName(w.Name); ok {
					taskWindowMap[taskName] = w.ID
				}
			}
		}

		for _, t := range merged {
			logging.Log("Auto-cleaning merged task: %s", t.Name)
			windowKilled := false
			if t.HasTabLock() {
				if windowID, err := t.LoadWindowID(); err == nil && windowID != "" {
					logging.Debug("Killing window %s for merged task %s", windowID, t.Name)
					_ = tm.KillWindow(windowID)
					windowKilled = true
				}
			}
			if !windowKilled {
				token := constants.TruncateForWindowName(t.Name)
				legacy := constants.LegacyTruncateForWindowName(t.Name)
				if windowID, ok := taskWindowMap[token]; ok {
					logging.Debug("Killing window %s (by name) for merged task %s", windowID, t.Name)
					_ = tm.KillWindow(windowID)
				} else if windowID, ok := taskWindowMap[legacy]; ok {
					logging.Debug("Killing window %s (legacy name) for merged task %s", windowID, t.Name)
					_ = tm.KillWindow(windowID)
				}
			}
			_ = mgr.CleanupTask(t)
			fmt.Printf("✅ Cleaned up merged task: %s\n", t.Name)
		}
	}

	// Clean up orphaned windows (windows without agent directory)
	orphanedWindows, err := mgr.FindOrphanedWindows()
	if err == nil && len(orphanedWindows) > 0 {
		logging.Log("Found %d orphaned windows to close", len(orphanedWindows))
		for _, windowID := range orphanedWindows {
			logging.Debug("Closing orphaned window: %s", windowID)
			_ = tm.KillWindow(windowID)
			fmt.Printf("✅ Closed orphaned window: %s\n", windowID)
		}
	}

	// Reopen incomplete tasks (tasks with tab-lock but no window)
	incomplete, err := mgr.FindIncompleteTasks(appCtx.SessionName)
	if err == nil && len(incomplete) > 0 {
		logging.Log("Found %d incomplete tasks to reopen", len(incomplete))
		pawBin, _ := os.Executable()
		for _, t := range incomplete {
			logging.Log("Reopening incomplete task: %s (reason: window not found)", t.Name)
			_ = t.RemoveTabLock()
			handleCmd := exec.Command(pawBin, "internal", "handle-task", appCtx.SessionName, t.AgentDir)
			if err := handleCmd.Start(); err != nil {
				logging.Warn("Failed to reopen task %s: %v", t.Name, err)
			} else {
				fmt.Printf("🔄 Reopening task: %s\n", t.Name)
			}
		}
	}

	// Resume stopped agents (windows exist but Claude has exited)
	stopped, err := mgr.FindStoppedTasks()
	if err == nil && len(stopped) > 0 {
		logging.Log("Found %d stopped agents to resume", len(stopped))
		pawBin, _ := os.Executable()
		for _, info := range stopped {
			logging.Log("Resuming stopped agent: %s (window=%s)", info.Task.Name, info.WindowID)
			resumeCmd := exec.Command(pawBin, "internal", "resume-agent", appCtx.SessionName, info.WindowID, info.Task.AgentDir)
			if err := resumeCmd.Start(); err != nil {
				logging.Warn("Failed to resume agent %s: %v", info.Task.Name, err)
			} else {
				fmt.Printf("🔄 Resuming agent: %s\n", info.Task.Name)
			}
		}
	}

	logging.Debug("Attaching to session: %s", appCtx.SessionName)

	// Re-apply tmux config to ensure terminal title is set
	if err := reapplyTmuxConfig(appCtx, tm); err != nil {
		logging.Debug("Failed to re-apply tmux config: %v", err)
	}

	// Detect terminal theme and apply theme-aware tmux colors.
	// This ensures status bar, window tabs, and pane borders match the terminal's
	// dark/light mode when re-attaching from a different terminal.
	themePreset := ThemePreset(appCtx.Config.Theme)
	if themePreset == "" {
		themePreset = ThemeAuto
	}
	resolved := resolveThemePreset(themePreset)
	applyTmuxTheme(tm, resolved)
	logging.Debug("Applied theme on reattach: preset=%s resolved=%s", themePreset, resolved)

	// Always respawn main window on reattach to ensure fresh theme detection.
	// When user re-attaches from a terminal with different light/dark mode,
	// the running TUI won't know about the change. Respawning forces theme re-detection.
	if !versionChanged {
		if err := respawnMainWindow(appCtx, tm); err != nil {
			logging.Debug("Failed to respawn main window for theme detection: %v", err)
		}
	}

	// Also set terminal title option on re-attach
	_ = tm.SetOption("set-titles", "on", true)
	_ = tm.SetOption("set-titles-string", "[paw] "+appCtx.SessionName, true)

	// Attach to session
	return tm.AttachSession(appCtx.SessionName)
}

// respawnMainWindow respawns the main window (⭐️main) with the new paw binary.
func respawnMainWindow(appCtx *app.App, tm tmux.Client) error {
	// Find the main window (starts with ⭐️)
	windows, err := tm.ListWindows()
	if err != nil {
		return fmt.Errorf("failed to list windows: %w", err)
	}

	var mainWindowID string
	for _, w := range windows {
		if strings.HasPrefix(w.Name, constants.EmojiNew) {
			mainWindowID = w.ID
			break
		}
	}

	if mainWindowID == "" {
		logging.Debug("Main window not found, nothing to respawn")
		return nil
	}

	logging.Log("Respawning main window %s with new binary", mainWindowID)

	// Get the new paw binary path
	pawBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Respawn the pane with the new-task command using the new binary
	newTaskCmd := fmt.Sprintf("%s internal new-task %s", pawBin, appCtx.SessionName)
	if err := tm.RespawnPane(mainWindowID+".0", appCtx.ProjectDir, newTaskCmd); err != nil {
		return fmt.Errorf("failed to respawn main window: %w", err)
	}

	logging.Log("Main window respawned successfully")
	return nil
}

// updateBinSymlink creates or updates the .paw/bin symlink to point to the current paw binary.
func updateBinSymlink(pawDir string) (string, error) {
	// Get current executable path
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get actual path
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	symlink := filepath.Join(pawDir, constants.BinSymlinkName)

	// Check if symlink exists and points to the same binary
	if target, err := os.Readlink(symlink); err == nil {
		if target == exe {
			return symlink, nil
		}
	}

	// Remove existing symlink/file if exists
	_ = os.Remove(symlink)

	// Create new symlink
	if err := os.Symlink(exe, symlink); err != nil {
		return "", fmt.Errorf("failed to create symlink: %w", err)
	}

	return symlink, nil
}

// saveVersion saves the current PAW version to .paw/.version
func saveVersion(pawDir string) error {
	versionFile := filepath.Join(pawDir, constants.VersionFileName)
	return os.WriteFile(versionFile, []byte(Version), 0644)
}

// checkVersionChanged checks if PAW version has changed since session was started.
func checkVersionChanged(pawDir string) bool {
	versionFile := filepath.Join(pawDir, constants.VersionFileName)
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return true
	}
	savedVersion := strings.TrimSpace(string(data))
	return savedVersion != Version
}
