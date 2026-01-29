package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
	"golang.org/x/term"
)

// startNewSession creates a new tmux session
func startNewSession(appCtx *app.App, tm tmux.Client) error {
	logging.Debug("Starting new tmux session...")

	// Create/update bin symlink for hook execution
	if err := updateBinSymlink(appCtx.PawDir); err != nil {
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
	pawBin := getPawBin()

	// Create session with a shell (not the new-task command directly)
	width, height, ok := getTerminalSize()
	if err := tm.NewSession(tmux.SessionOpts{
		Name:       appCtx.SessionName,
		StartDir:   appCtx.ProjectDir,
		WindowName: constants.NewWindowName,
		Detached:   true,
		Width:      width,
		Height:     height,
	}); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	if ok {
		logging.Debug("Using terminal size for new session: %dx%d", width, height)
	}
	syncSessionEnv(tm, appCtx)

	// Setup tmux configuration
	setupTmuxConfig(appCtx, tm)

	// Setup git repo marker if applicable
	if appCtx.IsGitRepo {
		markerPath := filepath.Join(appCtx.PawDir, constants.GitRepoMarker)
		_ = os.WriteFile(markerPath, []byte{}, 0644) //nolint:gosec // G306: marker file needs to be readable
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
			handleCmd := exec.Command(pawBin, "internal", "handle-task", appCtx.SessionName, t.AgentDir) //nolint:gosec // G204: pawBin is from getPawBin()
			// Pass PAW_DIR and PROJECT_DIR so getAppFromSession can find the project
			// (required for global workspaces where there's no local .paw directory)
			handleCmd.Env = append(os.Environ(),
				"PAW_DIR="+appCtx.PawDir,
				"PROJECT_DIR="+appCtx.ProjectDir,
			)
			if err := handleCmd.Start(); err != nil {
				logging.Warn("Failed to reopen task %s: %v", t.Name, err)
			} else {
				fmt.Printf("🔄 Reopening task: %s\n", t.Name)
			}
		}
	}

	// Wait for shell to be ready before sending keys
	paneTarget := appCtx.SessionName + ":" + constants.NewWindowName + ".0"
	if err := tm.WaitForPane(paneTarget, constants.PaneWaitTimeout, 1); err != nil {
		logging.Warn("WaitForPane timed out, continuing anyway: %v", err)
	}

	// Create yazi pane on the left side of the main window before launching the TUI.
	// This avoids resize artifacts from swapping panes after the TUI starts.
	mainWindowTarget := appCtx.SessionName + ":" + constants.NewWindowName
	if err := createFilePickerPane(tm, appCtx, mainWindowTarget); err != nil {
		logging.Warn("Failed to create yazi pane: %v", err)
		// Non-fatal: continue without yazi pane
	}

	// Send new-task command to the main window
	// Include PAW_DIR, PROJECT_DIR, and DISPLAY_NAME so getAppFromSession can find the project
	// (required for global workspaces where there's no local .paw directory)
	newTaskCmd := buildNewTaskCommand(appCtx, pawBin, appCtx.SessionName)
	_ = tm.SendKeysLiteral(appCtx.SessionName+":"+constants.NewWindowName, newTaskCmd)
	_ = tm.SendKeys(appCtx.SessionName+":"+constants.NewWindowName, "Enter")

	// Attach to session
	if err := tm.AttachSession(appCtx.SessionName); err != nil {
		return fmt.Errorf("failed to attach to newly created session %q: %w (workspace: %s)", appCtx.SessionName, err, appCtx.PawDir)
	}
	return nil
}

func getTerminalSize() (int, int, bool) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return 0, 0, false
	}

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

// attachToSession attaches to an existing session
func attachToSession(appCtx *app.App, tm tmux.Client) error {
	logging.Debug("Running pre-attach cleanup and recovery...")
	syncSessionEnv(tm, appCtx)

	// Check if PAW version has changed and update symlink
	versionChanged := checkVersionChanged(appCtx.PawDir)
	if versionChanged {
		logging.Log("PAW version changed, updating bin symlink...")
		if err := updateBinSymlink(appCtx.PawDir); err != nil {
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
	// - Workspace is stored in the global workspace location
	// Also refresh .claude files on version change to pick up updated CLAUDE.md templates.
	claudeDir := filepath.Join(appCtx.PawDir, constants.ClaudeLink)
	claudeDirMissing := false
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		claudeDirMissing = true
	}
	if claudeDirMissing || versionChanged {
		reason := "missing"
		if versionChanged && !claudeDirMissing {
			reason = "version changed"
		}
		logging.Log("Refreshing .claude directory (%s)...", reason)
		if err := embed.WriteClaudeFiles(claudeDir); err != nil {
			logging.Warn("Failed to write claude files: %v", err)
		} else {
			logging.Log("Claude files refreshed successfully")
		}
	}

	// Also refresh HELP-FOR-PAW.md on version change
	if versionChanged {
		logging.Log("Refreshing HELP-FOR-PAW.md (version changed)...")
		if err := embed.WritePawHelpFile(appCtx.PawDir); err != nil {
			logging.Warn("Failed to write PAW help file: %v", err)
		} else {
			logging.Log("PAW help file refreshed successfully")
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
				if windowID, ok := taskWindowMap[token]; ok {
					logging.Debug("Killing window %s (by name) for merged task %s", windowID, t.Name)
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
		pawBin := getPawBin()
		for _, t := range incomplete {
			logging.Log("Reopening incomplete task: %s (reason: window not found)", t.Name)
			_ = t.RemoveTabLock()
			handleCmd := exec.Command(pawBin, "internal", "handle-task", appCtx.SessionName, t.AgentDir) //nolint:gosec // G204: pawBin is from getPawBin()
			// Pass PAW_DIR and PROJECT_DIR so getAppFromSession can find the project
			// (required for global workspaces where there's no local .paw directory)
			handleCmd.Env = append(os.Environ(),
				"PAW_DIR="+appCtx.PawDir,
				"PROJECT_DIR="+appCtx.ProjectDir,
			)
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
		pawBin := getPawBin()
		for _, info := range stopped {
			logging.Log("Resuming stopped agent: %s (window=%s)", info.Task.Name, info.WindowID)
			resumeCmd := exec.Command(pawBin, "internal", "resume-agent", appCtx.SessionName, info.WindowID, info.Task.AgentDir) //nolint:gosec // G204: pawBin is from getPawBin()
			if err := resumeCmd.Start(); err != nil {
				logging.Warn("Failed to resume agent %s: %v", info.Task.Name, err)
			} else {
				fmt.Printf("🔄 Resuming agent: %s\n", info.Task.Name)
			}
		}
	}

	logging.Debug("Attaching to session: %s", appCtx.SessionName)

	// Re-apply tmux config to ensure terminal title is set
	reapplyTmuxConfig(appCtx, tm)

	// Detect terminal theme and apply theme-aware tmux colors.
	// This ensures status bar, window tabs, and pane borders match the terminal's
	// dark/light mode when re-attaching from a different terminal.
	themePreset := ThemeAuto
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

	primeFilePickerPaneOption(tm)

	// Also set terminal title option on re-attach
	// Use DisplayName for user-friendly display (e.g., "repo/subdir" format)
	_ = tm.SetOption("set-titles", "on", true)
	_ = tm.SetOption("set-titles-string", "[paw] "+appCtx.GetDisplayName(), true)

	// Verify session still exists before attaching
	// (cleanup operations might have killed all windows, destroying the session)
	if !tm.HasSession(appCtx.SessionName) {
		logging.Warn("Session no longer exists after cleanup, creating new session")
		return startNewSession(appCtx, tm)
	}

	// Attach to session
	if err := tm.AttachSession(appCtx.SessionName); err != nil {
		return fmt.Errorf("failed to attach to session %q: %w (workspace: %s)", appCtx.SessionName, err, appCtx.PawDir)
	}
	return nil
}

func primeFilePickerPaneOption(tm tmux.Client) {
	windows, err := tm.ListWindows()
	if err != nil {
		return
	}

	var mainWindowID string
	for _, w := range windows {
		if strings.HasPrefix(w.Name, constants.EmojiNew) {
			mainWindowID = w.ID
			break
		}
	}
	if mainWindowID == "" {
		return
	}

	forceFilePickerWidth(tm, mainWindowID, "prime-file-picker")
}

// respawnMainWindow respawns the main window (⭐️main) with the new paw binary.
// If the main window doesn't exist, it creates a new one.
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

	// Include PAW_DIR, PROJECT_DIR, and DISPLAY_NAME so getAppFromSession can find the project
	// (required for global workspaces where there's no local .paw directory)
	newTaskCmd := buildNewTaskCommand(appCtx, getPawBin(), appCtx.SessionName)

	if mainWindowID == "" {
		// Main window doesn't exist - create it
		logging.Log("Main window not found, creating new one")
		windowID, err := tm.NewWindow(tmux.WindowOpts{
			Name:     constants.NewWindowName,
			StartDir: appCtx.ProjectDir,
		})
		if err != nil {
			return fmt.Errorf("failed to create main window: %w", err)
		}

		// Wait for shell to be ready before sending keys
		paneID := windowID + ".0"
		if err := tm.WaitForPane(paneID, constants.PaneWaitTimeout, 1); err != nil {
			logging.Warn("WaitForPane timed out, continuing anyway: %v", err)
		}

		// Create yazi pane on the left side of the main window before launching the TUI.
		if err := createFilePickerPane(tm, appCtx, windowID); err != nil {
			logging.Warn("Failed to create yazi pane: %v", err)
			// Non-fatal: continue without yazi pane
		}

		// Send new-task command to the new window
		if err := tm.SendKeysLiteral(windowID, newTaskCmd); err != nil {
			return fmt.Errorf("failed to send keys: %w", err)
		}
		if err := tm.SendKeys(windowID, "Enter"); err != nil {
			return fmt.Errorf("failed to send Enter: %w", err)
		}

		logging.Log("Main window created successfully: %s", windowID)
		return nil
	}

	logging.Log("Respawning main window %s with new binary", mainWindowID)

	// Determine which pane is the main TUI pane
	// If pane 1 exists, file picker is pane 0 and main TUI is pane 1
	// Otherwise, main TUI is pane 0
	mainPaneID := mainWindowID + ".0"
	if tm.HasPane(mainWindowID + ".1") {
		mainPaneID = mainWindowID + ".1"
	}

	// Respawn the pane with the new-task command using the new binary
	if err := tm.RespawnPane(mainPaneID, appCtx.ProjectDir, newTaskCmd); err != nil {
		return fmt.Errorf("failed to respawn main window: %w", err)
	}

	logging.Log("Main window respawned successfully")
	return nil
}

// updateBinSymlink creates or updates the .paw/bin symlink to point to the current paw binary.
// Uses atomic rename to prevent race conditions (TOCTOU vulnerability).
func updateBinSymlink(pawDir string) error {
	// Get current executable path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get actual path
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	symlink := filepath.Join(pawDir, constants.BinSymlinkName)

	// Check if symlink exists and points to the same binary
	if target, err := os.Readlink(symlink); err == nil {
		if target == exe {
			return nil
		}
	}

	// Security: Use atomic rename to prevent race conditions
	// Create symlink at a temporary location first, then atomically rename
	tmpSymlink := symlink + ".tmp"

	// Clean up any existing temp symlink
	_ = os.Remove(tmpSymlink)

	// Create new symlink at temp location
	if err := os.Symlink(exe, tmpSymlink); err != nil {
		return fmt.Errorf("failed to create temp symlink: %w", err)
	}

	// Atomically rename to final location (this replaces any existing file)
	if err := os.Rename(tmpSymlink, symlink); err != nil {
		_ = os.Remove(tmpSymlink) // Clean up temp on failure
		return fmt.Errorf("failed to rename symlink: %w", err)
	}

	return nil
}

// saveVersion saves the current PAW version to .paw/.version
func saveVersion(pawDir string) error {
	versionFile := filepath.Join(pawDir, constants.VersionFileName)
	return os.WriteFile(versionFile, []byte(Version), 0644) //nolint:gosec // G306: version file needs to be readable
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
