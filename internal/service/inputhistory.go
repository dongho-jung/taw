// Package service provides business logic services for PAW.
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dongho-jung/paw/internal/fileutil"
)

const (
	// InputHistoryFile is the name of the input history file.
	InputHistoryFile = "input-history"
	// MaxInputHistoryEntries is the maximum number of entries to keep.
	MaxInputHistoryEntries = 100
)

// InputHistoryEntry represents a single input history entry.
type InputHistoryEntry struct {
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// InputHistoryService handles task input history operations.
type InputHistoryService struct {
	pawDir string
}

// NewInputHistoryService creates a new input history service.
func NewInputHistoryService(pawDir string) *InputHistoryService {
	return &InputHistoryService{
		pawDir: pawDir,
	}
}

// getHistoryPath returns the path to the input history file.
func (s *InputHistoryService) getHistoryPath() string {
	return filepath.Join(s.pawDir, InputHistoryFile)
}

// SaveInput saves a task input to history.
func (s *InputHistoryService) SaveInput(content string) error {
	if content == "" {
		return nil
	}

	entries, err := s.LoadHistory()
	if err != nil {
		// If history can't be loaded, start fresh.
		entries = nil
	}

	now := time.Now()
	// Check if the same content already exists at the top (avoid duplicates)
	if len(entries) > 0 && entries[0].Content == content {
		// Update timestamp only
		entries[0].Timestamp = now
	} else {
		// Remove any existing entry with the same content
		filtered := make([]InputHistoryEntry, 0, len(entries))
		for _, e := range entries {
			if e.Content != content {
				filtered = append(filtered, e)
			}
		}

		// Add new entry at the beginning
		newEntry := InputHistoryEntry{
			Content:   content,
			Timestamp: now,
		}
		entries = append([]InputHistoryEntry{newEntry}, filtered...)
	}

	// Limit to max entries
	if len(entries) > MaxInputHistoryEntries {
		entries = entries[:MaxInputHistoryEntries]
	}

	return s.saveEntries(entries)
}

// LoadHistory loads the input history from file.
func (s *InputHistoryService) LoadHistory() ([]InputHistoryEntry, error) {
	historyPath := s.getHistoryPath()

	data, err := os.ReadFile(historyPath) //nolint:gosec // G304: historyPath is from getHistoryPath()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []InputHistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		// Corrupt JSON: backup and return empty list for graceful degradation
		_ = fileutil.BackupCorruptFile(historyPath)
		return nil, nil //nolint:nilerr // Intentional: return empty list on corrupt file
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	return entries, nil
}

// GetAllContents returns all history contents as strings (most recent first).
func (s *InputHistoryService) GetAllContents() ([]string, error) {
	entries, err := s.LoadHistory()
	if err != nil {
		return nil, err
	}

	contents := make([]string, len(entries))
	for i, e := range entries {
		contents[i] = e.Content
	}

	return contents, nil
}

// saveEntries saves entries to the history file.
func (s *InputHistoryService) saveEntries(entries []InputHistoryEntry) error {
	historyPath := s.getHistoryPath()
	if err := os.MkdirAll(filepath.Dir(historyPath), 0755); err != nil { //nolint:gosec // G301: standard directory permissions
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return fileutil.WriteFileAtomic(historyPath, data, 0644)
}
