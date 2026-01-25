// Package service provides business logic services for PAW.
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dongho-jung/paw/internal/claude"
	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/fileutil"
	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/task"
)

// HistoryService handles task history operations.
type HistoryService struct {
	historyDir   string
	claudeClient claude.Client
}

// CommitMetadata describes git commit context for a task.
type CommitMetadata struct {
	Hash   string `json:"hash,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// VerificationMetadata describes verification execution results.
type VerificationMetadata struct {
	Command    string `json:"command,omitempty"`
	Success    bool   `json:"success"`
	ExitCode   int    `json:"exit_code,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
}

// HookMetadata describes hook execution results.
type HookMetadata struct {
	Name       string `json:"name"`
	Command    string `json:"command,omitempty"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
}

// HistoryMetadata describes metadata captured with task history.
type HistoryMetadata struct {
	TaskName        string                `json:"task_name,omitempty"`
	SessionName     string                `json:"session_name,omitempty"`
	ProjectDir      string                `json:"project_dir,omitempty"`
	TaskOptions     *config.TaskOptions   `json:"task_options,omitempty"`
	Commit          *CommitMetadata       `json:"commit,omitempty"`
	Verification    *VerificationMetadata `json:"verification,omitempty"`
	Hooks           []HookMetadata        `json:"hooks,omitempty"`
	StartedAt       string                `json:"started_at,omitempty"`
	FinishedAt      string                `json:"finished_at,omitempty"`
	DurationSeconds int64                 `json:"duration_seconds,omitempty"`
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
	return s.save(taskName, taskContent, paneContent, false, nil, nil)
}

// SaveCancelled saves a cancelled task to history with .cancelled extension.
func (s *HistoryService) SaveCancelled(taskName, taskContent, paneContent string) error {
	return s.save(taskName, taskContent, paneContent, true, nil, nil)
}

// SaveCompletedWithDetails saves a completed task with extra metadata and hook outputs.
func (s *HistoryService) SaveCompletedWithDetails(taskName, taskContent, paneContent string, meta *HistoryMetadata, hookOutputs map[string]string) error {
	return s.save(taskName, taskContent, paneContent, false, meta, hookOutputs)
}

// SaveCancelledWithDetails saves a cancelled task with extra metadata and hook outputs.
func (s *HistoryService) SaveCancelledWithDetails(taskName, taskContent, paneContent string, meta *HistoryMetadata, hookOutputs map[string]string) error {
	return s.save(taskName, taskContent, paneContent, true, meta, hookOutputs)
}

// RecordStatusTransition records a status transition for a task.
func (s *HistoryService) RecordStatusTransition(taskName string, from, to task.Status, source, detail string, valid bool) error {
	statusDir := filepath.Join(s.historyDir, "status")
	if err := os.MkdirAll(statusDir, 0755); err != nil { //nolint:gosec // G301: standard directory permissions
		return fmt.Errorf("failed to create status history directory: %w", err)
	}

	record := map[string]any{
		"ts":     time.Now().Format(time.RFC3339Nano),
		"task":   taskName,
		"from":   string(from),
		"to":     string(to),
		"source": source,
		"detail": detail,
		"valid":  valid,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal status transition: %w", err)
	}

	statusFile := filepath.Join(statusDir, taskName+".jsonl")
	f, err := os.OpenFile(statusFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644) //nolint:gosec // G302: status history file needs to be readable by other tools
	if err != nil {
		return fmt.Errorf("failed to open status history file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write status history: %w", err)
	}

	return nil
}

// save saves a task to history.
func (s *HistoryService) save(taskName, taskContent, paneContent string, cancelled bool, meta *HistoryMetadata, hookOutputs map[string]string) error {
	if err := os.MkdirAll(s.historyDir, 0755); err != nil { //nolint:gosec // G301: standard directory permissions
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	if paneContent == "" {
		return errors.New("empty pane content")
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
	if meta != nil {
		if meta.TaskName == "" {
			meta.TaskName = taskName
		}
		metaData, err := json.MarshalIndent(meta, "", "  ")
		if err == nil {
			historyContent.WriteString("---meta---\n")
			historyContent.Write(metaData)
			historyContent.WriteString("\n")
		} else {
			logging.Warn("Failed to marshal history metadata: %v", err)
		}
		historyContent.WriteString("---task---\n")
	}
	if taskContent != "" {
		historyContent.WriteString(taskContent)
		historyContent.WriteString("\n---summary---\n")
	}
	if summary != "" {
		historyContent.WriteString(summary)
	}
	historyContent.WriteString("\n---capture---\n")
	historyContent.WriteString(paneContent)

	if len(hookOutputs) > 0 {
		historyContent.WriteString("\n---hooks---\n")
		hookNames := make([]string, 0, len(hookOutputs))
		for name := range hookOutputs {
			hookNames = append(hookNames, name)
		}
		sort.Strings(hookNames)
		for _, name := range hookNames {
			output := hookOutputs[name]
			historyContent.WriteString(fmt.Sprintf("## %s\n", name))
			historyContent.WriteString(output)
			if !strings.HasSuffix(output, "\n") {
				historyContent.WriteString("\n")
			}
		}
	}

	// Generate filename: YYMMDD_HHMMSS_taskname[.cancelled]
	timestamp := time.Now().Format("060102_150405")
	filename := fmt.Sprintf("%s_%s", timestamp, taskName)
	if cancelled {
		filename += ".cancelled"
	}

	historyFile := filepath.Join(s.historyDir, filename)
	if err := fileutil.WriteFileAtomic(historyFile, []byte(historyContent.String()), 0644); err != nil {
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
	data, err := os.ReadFile(historyFile) //nolint:gosec // G304: historyFile is from controlled history directory
	if err != nil {
		return "", fmt.Errorf("failed to read history file: %w", err)
	}

	content := string(data)

	// Extract task content (after ---task--- if present)
	if idx := strings.Index(content, "\n---task---\n"); idx != -1 {
		content = content[idx+len("\n---task---\n"):]
		if idxSummary := strings.Index(content, "\n---summary---\n"); idxSummary != -1 {
			return content[:idxSummary], nil
		}
		if idxCapture := strings.Index(content, "\n---capture---\n"); idxCapture != -1 {
			return content[:idxCapture], nil
		}
		return content, nil
	}

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

	files := make([]string, 0, len(entries))
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
	// Timestamp format is "060102_150405" (13 chars) plus "_" separator.
	const timestampPrefixLen = 14 // YYMMDD_HHMMSS_
	if len(base) > timestampPrefixLen {
		return base[timestampPrefixLen:]
	}

	return base
}
