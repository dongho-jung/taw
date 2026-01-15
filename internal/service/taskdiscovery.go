// Package service provides business logic services for PAW.
package service

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

// DiscoveredTask represents a task discovered from any PAW session.
type DiscoveredTask struct {
	Name          string // Task name (without emoji)
	Session       string // Session name (project name)
	Status        DiscoveredStatus
	WindowID      string    // Tmux window ID
	Preview       string    // Last 3 lines from agent pane
	CurrentAction string    // Agent's current action (extracted from ⏺ spinner line)
	Duration      string    // Task duration (e.g., "1m 36s") extracted from Claude status
	Tokens        string    // Token count (e.g., "↓ 5.9k") extracted from Claude status
	CreatedAt     time.Time // Estimated creation time
}

// DiscoveredStatus represents the status of a discovered task.
type DiscoveredStatus string

const (
	DiscoveredWorking DiscoveredStatus = "working"
	DiscoveredWaiting DiscoveredStatus = "waiting"
	DiscoveredDone    DiscoveredStatus = "done"
	// DiscoveredWarning is kept for backward compatibility with old windows.
	// New windows no longer use Warning status; it maps to Waiting.
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
// Returns tasks grouped by status for Kanban display (3 columns: Working, Waiting, Done).
// Note: Warning status is merged into Waiting (Warning column removed from UI).
func (s *TaskDiscoveryService) DiscoverAll() (working, waiting, done []*DiscoveredTask) {
	sockets := s.findPawSockets()

	var allTasks []*DiscoveredTask

	for _, socket := range sockets {
		tasks := s.discoverFromSocket(socket)
		allTasks = append(allTasks, tasks...)
	}

	// Sort by creation time (oldest first within each status)
	sort.Slice(allTasks, func(i, j int) bool {
		return allTasks[i].CreatedAt.Before(allTasks[j].CreatedAt)
	})

	// Group by status (Warning merged into Waiting)
	for _, task := range allTasks {
		switch task.Status {
		case DiscoveredWorking:
			working = append(working, task)
		case DiscoveredWaiting, DiscoveredWarning:
			// Warning tasks now display in Waiting column
			waiting = append(waiting, task)
		case DiscoveredDone:
			done = append(done, task)
		}
	}

	return working, waiting, done
}

// findPawSockets finds all PAW tmux sockets.
func (s *TaskDiscoveryService) findPawSockets() []string {
	entries, err := os.ReadDir(s.socketDir)
	if err != nil {
		// Socket dir not found is expected when tmux is not running
		if !os.IsNotExist(err) {
			logging.Warn("Failed to read tmux socket dir %s: %v", s.socketDir, err)
		}
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

	pawDir := resolvePawDir(tm, sessionName)
	tokenMap := buildTokenMap(pawDir)

	// List windows
	windows, err := tm.ListWindows()
	if err != nil {
		logging.Warn("Failed to list windows for session %s: %v", sessionName, err)
		return nil
	}

	var tasks []*DiscoveredTask

	for _, w := range windows {
		// Parse window name to extract task name and status
		taskToken, status := parseWindowName(w.Name)
		if taskToken == "" {
			continue // Not a task window
		}

		taskName := resolveTaskName(taskToken, tokenMap)

		task := &DiscoveredTask{
			Name:      taskName,
			Session:   sessionName,
			Status:    status,
			WindowID:  w.ID,
			CreatedAt: time.Now(), // We don't have exact creation time
		}

		// Only capture pane content for Working tasks (performance optimization)
		// Done and Waiting tasks don't need continuous monitoring since their
		// action/duration/tokens won't be changing
		if status == DiscoveredWorking {
			// Capture pane content for preview and current action (pane .0)
			// Use more lines (50) to find the spinner indicator which shows current action
			agentPane := w.ID + ".0"
			if capture, err := tm.CapturePane(agentPane, 50); err == nil {
				task.Preview = trimPreview(capture)
				task.CurrentAction = extractCurrentAction(capture)
				task.Duration, task.Tokens = extractDurationAndTokens(capture)
			}
		}

		tasks = append(tasks, task)
	}

	return tasks
}

func resolvePawDir(tm tmux.Client, sessionName string) string {
	sessionPath, err := tm.RunWithOutput("display-message", "-p", "-t", sessionName, "#{session_path}")
	if err != nil {
		logging.Debug("Failed to resolve session path for %s: %v", sessionName, err)
		return ""
	}
	if sessionPath == "" {
		return ""
	}
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return ""
	}
	pawDir := filepath.Join(sessionPath, constants.PawDirName)
	if _, err := os.Stat(pawDir); err != nil {
		return ""
	}
	return pawDir
}

func buildTokenMap(pawDir string) map[string]string {
	mapping := map[string]string{}
	if pawDir == "" {
		return mapping
	}

	if fileMap, err := LoadWindowMap(pawDir); err == nil {
		for token, name := range fileMap {
			mapping[token] = name
		}
	} else if !os.IsNotExist(err) {
		logging.Debug("Failed to load window map from %s: %v", pawDir, err)
	}

	agentsDir := filepath.Join(pawDir, constants.AgentsDirName)
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return mapping
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		mapping[constants.TruncateForWindowName(name)] = name
		mapping[constants.LegacyTruncateForWindowName(name)] = name
	}

	return mapping
}

func resolveTaskName(token string, tokenMap map[string]string) string {
	if token == "" {
		return token
	}
	if name, ok := tokenMap[token]; ok {
		return name
	}
	return token
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

// extractCurrentAction extracts the agent's current action from pane capture.
// Looks for the last line containing "⏺" (spinner indicator) which shows what
// the agent is currently working on (e.g., "⏺ Reading file...", "⏺ Running tests...").
func extractCurrentAction(capture string) string {
	lines := strings.Split(capture, "\n")

	// Find the last line containing the spinner indicator "⏺"
	var lastSpinnerLine string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "⏺") {
			lastSpinnerLine = trimmed
		}
	}

	if lastSpinnerLine == "" {
		return ""
	}

	// Extract the text after "⏺" (the action description)
	// The spinner line format is typically: "⏺ Action description"
	idx := strings.Index(lastSpinnerLine, "⏺")
	if idx == -1 {
		return ""
	}

	action := strings.TrimSpace(lastSpinnerLine[idx+len("⏺"):])

	// Truncate if too long (max 60 chars for display)
	if len(action) > 60 {
		action = action[:57] + "..."
	}

	return action
}

// extractDurationAndTokens extracts duration and token count from pane capture.
// Looks for the Claude status line which shows duration and tokens.
// Example formats:
//   - "✻ Whirring… (ctrl+c to interrupt · 54s · ↓ 2.7k tokens)"
//   - "⏺ Reading file… (ctrl+c to interrupt · 1m 36s · ↓ 5.9k tokens · thought for 2s)"
// Returns duration (e.g., "54s", "1m 36s") and tokens (e.g., "↓ 2.7k").
func extractDurationAndTokens(capture string) (duration, tokens string) {
	lines := strings.Split(capture, "\n")

	// Find the last Claude status line containing duration/token info
	// Status lines typically start with spinner indicators (✻, ⏺, etc.)
	// and contain parenthetical metadata
	var lastStatusLine string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Look for lines with parenthetical metadata containing "·" separator
		if strings.Contains(trimmed, "(ctrl+c") && strings.Contains(trimmed, "·") {
			lastStatusLine = trimmed
		}
	}

	if lastStatusLine == "" {
		return "", ""
	}

	// Extract the parenthetical part
	startIdx := strings.Index(lastStatusLine, "(")
	endIdx := strings.LastIndex(lastStatusLine, ")")
	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return "", ""
	}

	metadata := lastStatusLine[startIdx+1 : endIdx]
	parts := strings.Split(metadata, "·")

	// Parse each part to find duration and tokens
	// Duration format: "1m 36s", "54s", "2h 5m", etc.
	// Tokens format: "↓ 2.7k tokens", "↑ 1.2k tokens"
	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check for tokens (contains "tokens" or arrow symbol)
		if strings.Contains(part, "tokens") {
			// Extract just the arrow and number (e.g., "↓ 2.7k")
			tokens = strings.TrimSuffix(part, " tokens")
			tokens = strings.TrimSpace(tokens)
			continue
		}

		// Check for duration (contains time units like 's', 'm', 'h')
		// Skip "ctrl+c to interrupt" and "thought for Xs"
		if strings.Contains(part, "ctrl+c") || strings.Contains(part, "thought for") {
			continue
		}

		// Duration typically ends with 's' (seconds) or contains 'm' (minutes) or 'h' (hours)
		if isDurationString(part) {
			duration = part
		}
	}

	return duration, tokens
}

// isDurationString checks if a string looks like a duration (e.g., "54s", "1m 36s", "2h 5m").
func isDurationString(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	// Duration strings contain digits and time unit suffixes
	hasDigit := false
	hasTimeUnit := false

	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
		if r == 's' || r == 'm' || r == 'h' {
			hasTimeUnit = true
		}
	}

	return hasDigit && hasTimeUnit
}
