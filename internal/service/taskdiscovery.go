// Package service provides business logic services for PAW.
package service

import (
	"fmt"
	"os"
	"os/user"
	"sort"
	"strings"
	"time"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

// DiscoveredTask represents a task discovered from any PAW session.
type DiscoveredTask struct {
	Name        string         // Task name (without emoji)
	Session     string         // Session name (project name)
	Status      DiscoveredStatus
	WindowID    string         // Tmux window ID
	Preview     string         // Last 3 lines from agent pane
	CreatedAt   time.Time      // Estimated creation time
}

// DiscoveredStatus represents the status of a discovered task.
type DiscoveredStatus string

const (
	DiscoveredWorking DiscoveredStatus = "working"
	DiscoveredWaiting DiscoveredStatus = "waiting"
	DiscoveredDone    DiscoveredStatus = "done"
	DiscoveredWarning DiscoveredStatus = "warning"
)

// TaskDiscoveryService discovers tasks across all PAW sessions.
type TaskDiscoveryService struct {
	socketDir string
}

// NewTaskDiscoveryService creates a new task discovery service.
func NewTaskDiscoveryService() *TaskDiscoveryService {
	// tmux stores sockets in /tmp/tmux-<UID>/
	socketDir := "/tmp/tmux-"
	if u, err := user.Current(); err == nil {
		socketDir += u.Uid
	} else {
		// Fallback: use effective UID
		socketDir += fmt.Sprintf("%d", os.Getuid())
	}

	return &TaskDiscoveryService{
		socketDir: socketDir,
	}
}

// DiscoverAll finds all PAW tasks across all sessions.
// Returns tasks grouped by status for Kanban display.
func (s *TaskDiscoveryService) DiscoverAll() (working, waiting, done, warning []*DiscoveredTask) {
	sockets := s.findPawSockets()
	logging.Trace("TaskDiscovery: found %d PAW sockets", len(sockets))

	var allTasks []*DiscoveredTask

	for _, socket := range sockets {
		tasks := s.discoverFromSocket(socket)
		allTasks = append(allTasks, tasks...)
	}

	// Sort by creation time (oldest first within each status)
	sort.Slice(allTasks, func(i, j int) bool {
		return allTasks[i].CreatedAt.Before(allTasks[j].CreatedAt)
	})

	// Group by status
	for _, task := range allTasks {
		switch task.Status {
		case DiscoveredWorking:
			working = append(working, task)
		case DiscoveredWaiting:
			waiting = append(waiting, task)
		case DiscoveredDone:
			done = append(done, task)
		case DiscoveredWarning:
			warning = append(warning, task)
		}
	}

	return working, waiting, done, warning
}

// findPawSockets finds all PAW tmux sockets.
func (s *TaskDiscoveryService) findPawSockets() []string {
	entries, err := os.ReadDir(s.socketDir)
	if err != nil {
		logging.Trace("TaskDiscovery: failed to read socket dir %s: %v", s.socketDir, err)
		return nil
	}

	var sockets []string
	for _, entry := range entries {
		name := entry.Name()
		// PAW sockets have prefix "paw-"
		if strings.HasPrefix(name, constants.TmuxSocketPrefix) {
			sockets = append(sockets, name)
		}
	}

	return sockets
}

// discoverFromSocket discovers tasks from a single PAW session.
func (s *TaskDiscoveryService) discoverFromSocket(socketName string) []*DiscoveredTask {
	// Extract session name from socket name (remove "paw-" prefix)
	sessionName := strings.TrimPrefix(socketName, constants.TmuxSocketPrefix)

	// Create tmux client for this socket
	tm := tmux.New(sessionName)

	// Check if session exists
	if !tm.HasSession(sessionName) {
		return nil
	}

	// List windows
	windows, err := tm.ListWindows()
	if err != nil {
		logging.Trace("TaskDiscovery: failed to list windows for %s: %v", sessionName, err)
		return nil
	}

	var tasks []*DiscoveredTask

	for _, w := range windows {
		// Parse window name to extract task name and status
		taskName, status := parseWindowName(w.Name)
		if taskName == "" {
			continue // Not a task window
		}

		task := &DiscoveredTask{
			Name:      taskName,
			Session:   sessionName,
			Status:    status,
			WindowID:  w.ID,
			CreatedAt: time.Now(), // We don't have exact creation time
		}

		// Capture last 3 lines from agent pane (pane .0)
		agentPane := w.ID + ".0"
		if preview, err := tm.CapturePane(agentPane, 3); err == nil {
			task.Preview = trimPreview(preview)
		}

		tasks = append(tasks, task)
	}

	return tasks
}

// parseWindowName parses a window name to extract task name and status.
// Returns empty string if not a task window.
func parseWindowName(windowName string) (string, DiscoveredStatus) {
	// Check each task emoji prefix
	switch {
	case strings.HasPrefix(windowName, constants.EmojiWorking):
		return strings.TrimPrefix(windowName, constants.EmojiWorking), DiscoveredWorking
	case strings.HasPrefix(windowName, constants.EmojiWaiting):
		return strings.TrimPrefix(windowName, constants.EmojiWaiting), DiscoveredWaiting
	case strings.HasPrefix(windowName, constants.EmojiDone):
		return strings.TrimPrefix(windowName, constants.EmojiDone), DiscoveredDone
	case strings.HasPrefix(windowName, constants.EmojiWarning):
		return strings.TrimPrefix(windowName, constants.EmojiWarning), DiscoveredWarning
	}

	return "", ""
}

// trimPreview cleans up the preview text.
func trimPreview(preview string) string {
	// Remove leading/trailing whitespace
	preview = strings.TrimSpace(preview)

	// Get last 3 non-empty lines
	lines := strings.Split(preview, "\n")
	var nonEmpty []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			nonEmpty = append(nonEmpty, trimmed)
		}
	}

	// Keep only last 3 lines
	if len(nonEmpty) > 3 {
		nonEmpty = nonEmpty[len(nonEmpty)-3:]
	}

	return strings.Join(nonEmpty, "\n")
}
