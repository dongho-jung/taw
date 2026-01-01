// Package service provides business logic services for TAW.
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/donghojung/taw/internal/claude"
	"github.com/donghojung/taw/internal/logging"
)

// HistoryService handles task history operations.
type HistoryService struct {
	historyDir   string
	claudeClient claude.Client
}

// NewHistoryService creates a new history service.
func NewHistoryService(historyDir string) *HistoryService {
	return &HistoryService{
		historyDir:   historyDir,
		claudeClient: claude.New(),
	}
}

// SetClaudeClient sets a custom Claude client (for testing).
func (s *HistoryService) SetClaudeClient(client claude.Client) {
	s.claudeClient = client
}

// SaveCompleted saves a completed task to history.
func (s *HistoryService) SaveCompleted(taskName, taskContent, paneContent string) error {
	return s.save(taskName, taskContent, paneContent, false)
}

// SaveCancelled saves a cancelled task to history with .cancelled extension.
func (s *HistoryService) SaveCancelled(taskName, taskContent, paneContent string) error {
	return s.save(taskName, taskContent, paneContent, true)
}

// save saves a task to history.
func (s *HistoryService) save(taskName, taskContent, paneContent string, cancelled bool) error {
	if err := os.MkdirAll(s.historyDir, 0755); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	if paneContent == "" {
		return fmt.Errorf("empty pane content")
	}

	// Generate summary using Claude
	summary, err := s.claudeClient.GenerateSummary(paneContent)
	if err != nil {
		logging.Warn("Failed to generate summary: %v", err)
		summary = "" // Continue without summary
	} else {
		logging.Debug("Generated summary: %d chars", len(summary))
	}

	// Build history content: task + summary + pane capture
	var historyContent strings.Builder
	if taskContent != "" {
		historyContent.WriteString(taskContent)
		historyContent.WriteString("\n---summary---\n")
	}
	if summary != "" {
		historyContent.WriteString(summary)
	}
	historyContent.WriteString("\n---capture---\n")
	historyContent.WriteString(paneContent)

	// Generate filename: YYMMDD_HHMMSS_taskname[.cancelled]
	timestamp := time.Now().Format("060102_150405")
	filename := fmt.Sprintf("%s_%s", timestamp, taskName)
	if cancelled {
		filename += ".cancelled"
	}

	historyFile := filepath.Join(s.historyDir, filename)
	if err := os.WriteFile(historyFile, []byte(historyContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write history file: %w", err)
	}

	status := "completed"
	if cancelled {
		status = "cancelled"
	}
	logging.Debug("Task history saved (%s): %s", status, historyFile)

	return nil
}

// LoadTaskContent loads the task content from a history file.
func (s *HistoryService) LoadTaskContent(historyFile string) (string, error) {
	data, err := os.ReadFile(historyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read history file: %w", err)
	}

	content := string(data)

	// Extract task content (before ---summary---)
	if idx := strings.Index(content, "\n---summary---\n"); idx != -1 {
		return content[:idx], nil
	}

	// No summary section, check for capture section
	if idx := strings.Index(content, "\n---capture---\n"); idx != -1 {
		return content[:idx], nil
	}

	// No sections found, return as-is
	return content, nil
}

// ListHistoryFiles returns all history files sorted by modification time (newest first).
func (s *HistoryService) ListHistoryFiles() ([]string, error) {
	entries, err := os.ReadDir(s.historyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read history directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, filepath.Join(s.historyDir, entry.Name()))
		}
	}

	return files, nil
}

// IsCancelled checks if a history file is a cancelled task.
func IsCancelled(historyFile string) bool {
	return strings.HasSuffix(historyFile, ".cancelled")
}

// ExtractTaskName extracts the task name from a history filename.
// Format: YYMMDD_HHMMSS_taskname[.cancelled]
func ExtractTaskName(historyFile string) string {
	base := filepath.Base(historyFile)

	// Remove .cancelled extension if present
	base = strings.TrimSuffix(base, ".cancelled")

	// Format: YYMMDD_HHMMSS_taskname
	// Timestamp is 14 chars (060102_150405) + 1 underscore = 14 chars before task name
	// So task name starts at index 14
	const timestampPrefixLen = 14 // YYMMDD_HHMMSS_
	if len(base) > timestampPrefixLen {
		return base[timestampPrefixLen:]
	}

	return base
}
