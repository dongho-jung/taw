package main

import (
	"fmt"
	"os"
	"time"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/service"
	"github.com/dongho-jung/paw/internal/task"
	"github.com/dongho-jung/paw/internal/tmux"
)

// waitForDependency waits for a dependency task to complete.
// Returns true if the task should proceed, false if blocked.
func waitForDependency(appCtx *app.App, tm tmux.Client, windowID string, t *task.Task, dep *config.TaskDependency) bool {
	if dep == nil || dep.TaskName == "" || dep.Condition == config.DependsOnNone {
		return true
	}
	if dep.TaskName == t.Name {
		logging.Warn("Dependency points to same task: %s", dep.TaskName)
		return true
	}

	firstWait := true
	for {
		status, found, terminal := resolveDependencyStatus(appCtx, dep.TaskName)
		if !found {
			logging.Warn("Dependency task not found: %s", dep.TaskName)
			return true
		}

		if dependencySatisfied(dep.Condition, status) {
			if !firstWait {
				workingName := windowNameForStatus(t.Name, task.StatusWorking)
				_ = renameWindowWithStatus(tm, windowID, workingName, appCtx.PawDir, t.Name, "depends-on")
			}
			return true
		}

		if terminal {
			warningName := windowNameForStatus(t.Name, task.StatusCorrupted)
			_ = renameWindowWithStatus(tm, windowID, warningName, appCtx.PawDir, t.Name, "depends-on")
			msg := fmt.Sprintf("⚠️ Dependency %s did not satisfy %s", dep.TaskName, dep.Condition)
			_ = tm.DisplayMessage(msg, 3000)
			logging.Warn("Dependency %s ended with %s; blocking task %s", dep.TaskName, status, t.Name)
			return false
		}

		if firstWait {
			waitName := windowNameForStatus(t.Name, task.StatusWaiting)
			_ = renameWindowWithStatus(tm, windowID, waitName, appCtx.PawDir, t.Name, "depends-on")
			msg := fmt.Sprintf("⏳ Waiting for %s (%s)", dep.TaskName, dep.Condition)
			_ = tm.DisplayMessage(msg, 3000)
			firstWait = false
		}

		time.Sleep(5 * time.Second)
	}
}

// dependencySatisfied checks if the given status satisfies the dependency condition.
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

// resolveDependencyStatus resolves the status of a dependency task.
// Returns: status, found, terminal (whether the status is terminal - done or corrupted)
func resolveDependencyStatus(appCtx *app.App, taskName string) (task.Status, bool, bool) {
	agentDir := appCtx.GetAgentDir(taskName)
	if _, err := os.Stat(agentDir); err == nil {
		depTask := task.New(taskName, agentDir)
		status, err := depTask.LoadStatus()
		if err != nil {
			logging.Trace("Dependency status read failed task=%s err=%v", taskName, err)
			return task.StatusWorking, true, false
		}
		return status, true, status == task.StatusDone || status == task.StatusCorrupted
	}

	historyService := service.NewHistoryService(appCtx.GetHistoryDir())
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
