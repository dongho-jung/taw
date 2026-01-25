// Package service provides business logic services for PAW.
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dongho-jung/paw/internal/fileutil"
)

const (
	// TemplateFile is the name of the template storage file.
	TemplateFile = "input-templates"
)

// TemplateEntry represents a named task template.
type TemplateEntry struct {
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateService handles task template storage operations.
type TemplateService struct {
	pawDir string
}

// NewTemplateService creates a new template service.
func NewTemplateService(pawDir string) *TemplateService {
	return &TemplateService{
		pawDir: pawDir,
	}
}

func (s *TemplateService) templatePath() string {
	return filepath.Join(s.pawDir, TemplateFile)
}

// LoadTemplates loads templates from file.
func (s *TemplateService) LoadTemplates() ([]TemplateEntry, error) {
	path := s.templatePath()
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from templatePath()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []TemplateEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		// Corrupt JSON: backup and return empty list for graceful degradation
		_ = fileutil.BackupCorruptFile(path)
		return nil, nil //nolint:nilerr // Intentional: return empty list on corrupt file
	}
	return entries, nil
}

// SaveTemplates saves templates to file.
func (s *TemplateService) SaveTemplates(entries []TemplateEntry) error {
	path := s.templatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // G301: standard directory permissions
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(path, data, 0644)
}
