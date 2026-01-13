package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/git"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

func collectProjectResults() []checkResult {
	appCtx, err := buildAppFromCwd()
	if err != nil {
		return []checkResult{
			{
				name:     "project",
				ok:       false,
				required: false,
				message:  err.Error(),
			},
		}
	}

	return projectChecks(appCtx)
}

func buildAppFromCwd() (*app.App, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	gitClient := git.New()
	isGitRepo := gitClient.IsGitRepo(cwd)
	projectDir := cwd
	if isGitRepo {
		if repoRoot, err := gitClient.GetRepoRoot(cwd); err == nil {
			projectDir = repoRoot
		}
	}

	application, err := app.New(projectDir)
	if err != nil {
		return nil, err
	}

	pawHome, _ := getPawHome()
	application.SetPawHome(pawHome)
	application.SetGitRepo(isGitRepo)

	return application, nil
}

func projectChecks(appCtx *app.App) []checkResult {
	pawDirExists := pathExists(appCtx.PawDir)
	results := []checkResult{
		{
			name:     ".paw directory",
			ok:       pawDirExists,
			required: false,
			message:  boolMessage(pawDirExists, appCtx.PawDir, "missing (run paw to initialize)"),
			fix: func() error {
				return appCtx.Initialize()
			},
		},
	}

	if !pawDirExists {
		return results
	}

	configPath := filepath.Join(appCtx.PawDir, constants.ConfigFileName)
	configExists := pathExists(configPath)
	results = append(results, checkResult{
		name:     "config",
		ok:       configExists,
		required: false,
		message:  boolMessage(configExists, configPath, "missing (run paw setup)"),
		fix: func() error {
			cfg := config.DefaultConfig()
			return cfg.Save(appCtx.PawDir)
		},
	})

	claudeDir := filepath.Join(appCtx.PawDir, constants.ClaudeLink)
	claudeExists := pathExists(claudeDir)
	results = append(results, checkResult{
		name:     "claude settings",
		ok:       claudeExists,
		required: false,
		message:  boolMessage(claudeExists, claudeDir, "missing"),
		fix: func() error {
			return embed.WriteClaudeFiles(claudeDir)
		},
	})

	helpFile := filepath.Join(appCtx.PawDir, "HELP-FOR-PAW.md")
	helpExists := pathExists(helpFile)
	results = append(results, checkResult{
		name:     "paw help file",
		ok:       helpExists,
		required: false,
		message:  boolMessage(helpExists, helpFile, "missing"),
		fix: func() error {
			return embed.WritePawHelpFile(appCtx.PawDir)
		},
	})

	historyDir := filepath.Join(appCtx.PawDir, constants.HistoryDirName)
	historyExists := pathExists(historyDir)
	results = append(results, checkResult{
		name:     "history directory",
		ok:       historyExists,
		required: false,
		message:  boolMessage(historyExists, historyDir, "missing"),
		fix: func() error {
			return os.MkdirAll(historyDir, 0755)
		},
	})

	agentsDir := filepath.Join(appCtx.PawDir, constants.AgentsDirName)
	agentsExists := pathExists(agentsDir)
	results = append(results, checkResult{
		name:     "agents directory",
		ok:       agentsExists,
		required: false,
		message:  boolMessage(agentsExists, agentsDir, "missing"),
		fix: func() error {
			return os.MkdirAll(agentsDir, 0755)
		},
	})

	if configExists {
		cfg, err := config.Load(appCtx.PawDir)
		if err != nil {
			results = append(results, checkResult{
				name:     "config values",
				ok:       false,
				required: false,
				message:  fmt.Sprintf("load failed: %v", err),
			})
		} else {
			appCtx.Config = cfg
			warnings := cfg.Normalize()
			if len(warnings) > 0 {
				results = append(results, checkResult{
					name:     "config values",
					ok:       false,
					required: false,
					message:  fmt.Sprintf("normalize warnings: %s", stringsJoin(warnings)),
				})
			} else {
				results = append(results, checkResult{
					name:     "config values",
					ok:       true,
					required: false,
					message:  "ok",
				})
			}
		}
	}

	results = append(results, worktreeChecks(appCtx)...)
	results = append(results, sessionChecks(appCtx)...)

	return results
}

func worktreeChecks(appCtx *app.App) []checkResult {
	if !appCtx.IsGitRepo || appCtx.Config == nil {
		return nil
	}

	mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
	corrupted, err := mgr.FindCorruptedTasks()
	if err != nil {
		return []checkResult{
			{
				name:     "worktree health",
				ok:       false,
				required: false,
				message:  fmt.Sprintf("error: %v", err),
			},
		}
	}

	if len(corrupted) == 0 {
		return []checkResult{
			{
				name:     "worktree health",
				ok:       true,
				required: false,
				message:  "ok",
			},
		}
	}

	return []checkResult{
		{
			name:     "worktree health",
			ok:       false,
			required: false,
			message:  fmt.Sprintf("corrupted worktrees: %d", len(corrupted)),
		},
	}
}

func sessionChecks(appCtx *app.App) []checkResult {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil
	}

	tm := tmux.New(appCtx.SessionName)
	if !tm.HasSession(appCtx.SessionName) {
		return []checkResult{
			{
				name:     "tmux session",
				ok:       false,
				required: false,
				message:  "not running",
			},
		}
	}

	mgr := task.NewManager(appCtx.AgentsDir, appCtx.ProjectDir, appCtx.PawDir, appCtx.IsGitRepo, appCtx.Config)
	mgr.SetTmuxClient(tm)

	orphaned, err := mgr.FindOrphanedWindows()
	if err != nil {
		return []checkResult{
			{
				name:     "tmux session",
				ok:       false,
				required: false,
				message:  fmt.Sprintf("session check failed: %v", err),
			},
		}
	}

	stopped, err := mgr.FindStoppedTasks()
	if err != nil {
		return []checkResult{
			{
				name:     "tmux session",
				ok:       false,
				required: false,
				message:  fmt.Sprintf("session check failed: %v", err),
			},
		}
	}

	incomplete, err := mgr.FindIncompleteTasks(appCtx.SessionName)
	if err != nil {
		return []checkResult{
			{
				name:     "tmux session",
				ok:       false,
				required: false,
				message:  fmt.Sprintf("session check failed: %v", err),
			},
		}
	}

	msg := fmt.Sprintf("orphaned=%d stopped=%d incomplete=%d", len(orphaned), len(stopped), len(incomplete))
	ok := len(orphaned) == 0 && len(stopped) == 0 && len(incomplete) == 0

	return []checkResult{
		{
			name:     "tmux session",
			ok:       ok,
			required: false,
			message:  msg,
		},
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func boolMessage(ok bool, okMessage, badMessage string) string {
	if ok {
		return okMessage
	}
	return badMessage
}

func stringsJoin(values []string) string {
	return strings.Join(values, "; ")
}
