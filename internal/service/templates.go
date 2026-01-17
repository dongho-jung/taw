// Package service provides business logic services for PAW.
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
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
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []TemplateEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// SaveTemplates saves templates to file.
func (s *TemplateService) SaveTemplates(entries []TemplateEntry) error {
	path := s.templatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
