// Package config handles PAW configuration parsing and management.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Template represents a reusable task template.
type Template struct {
	Name      string    `yaml:"name"`
	Content   string    `yaml:"content"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

// Templates represents a collection of templates.
type Templates struct {
	Items []Template `yaml:"templates"`
}

// TemplatesFileName is the file name for templates storage.
const TemplatesFileName = "templates.yaml"

// LoadTemplates loads templates from the paw directory.
func LoadTemplates(pawDir string) (*Templates, error) {
	templatesPath := filepath.Join(pawDir, TemplatesFileName)

	if _, err := os.Stat(templatesPath); os.IsNotExist(err) {
		return &Templates{Items: []Template{}}, nil
	}

	data, err := os.ReadFile(templatesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read templates: %w", err)
	}

	return parseTemplates(string(data))
}

// parseTemplates parses templates from YAML content.
func parseTemplates(content string) (*Templates, error) {
	templates := &Templates{Items: []Template{}}
	lines := strings.Split(content, "\n")

	var currentTemplate *Template
	var inContent bool
	var contentLines []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments at root level
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if inContent && strings.HasPrefix(line, "    ") {
				// Empty line within content block
				contentLines = append(contentLines, "")
			}
			continue
		}

		// Check for new template entry (- name:)
		if strings.HasPrefix(trimmed, "- name:") {
			// Save previous template
			if currentTemplate != nil {
				if inContent && len(contentLines) > 0 {
					currentTemplate.Content = strings.TrimRight(strings.Join(contentLines, "\n"), "\n")
				}
				templates.Items = append(templates.Items, *currentTemplate)
			}

			// Start new template
			currentTemplate = &Template{
				Name:      strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:")),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			inContent = false
			contentLines = nil
			continue
		}

		if currentTemplate == nil {
			continue
		}

		// Parse template fields
		if strings.HasPrefix(trimmed, "content:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "content:"))
			if value == "|" {
				// Multi-line content
				inContent = true
				contentLines = nil
			} else {
				currentTemplate.Content = value
			}
			continue
		}

		if strings.HasPrefix(trimmed, "created_at:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "created_at:"))
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				currentTemplate.CreatedAt = t
			}
			continue
		}

		if strings.HasPrefix(trimmed, "updated_at:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "updated_at:"))
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				currentTemplate.UpdatedAt = t
			}
			continue
		}

		// Multi-line content (indented with 4 spaces)
		if inContent && (strings.HasPrefix(line, "    ") || line == "") {
			contentLine := line
			if strings.HasPrefix(line, "    ") {
				contentLine = line[4:] // Remove 4-space indent
			}
			contentLines = append(contentLines, contentLine)
		} else if inContent && !strings.HasPrefix(line, "  ") {
			// End of content block
			inContent = false
		}
	}

	// Save last template
	if currentTemplate != nil {
		if inContent && len(contentLines) > 0 {
			currentTemplate.Content = strings.TrimRight(strings.Join(contentLines, "\n"), "\n")
		}
		templates.Items = append(templates.Items, *currentTemplate)
	}

	return templates, nil
}

// Save writes templates to the paw directory.
func (t *Templates) Save(pawDir string) error {
	templatesPath := filepath.Join(pawDir, TemplatesFileName)

	var sb strings.Builder
	sb.WriteString("# PAW Templates\n")
	sb.WriteString("# Task templates for quick task creation\n\n")

	for _, tmpl := range t.Items {
		sb.WriteString(fmt.Sprintf("- name: %s\n", tmpl.Name))
		sb.WriteString(fmt.Sprintf("  created_at: %s\n", tmpl.CreatedAt.Format(time.RFC3339)))
		sb.WriteString(fmt.Sprintf("  updated_at: %s\n", tmpl.UpdatedAt.Format(time.RFC3339)))

		// Write content
		if strings.Contains(tmpl.Content, "\n") {
			sb.WriteString("  content: |\n")
			for _, line := range strings.Split(tmpl.Content, "\n") {
				sb.WriteString("    ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		} else {
			sb.WriteString(fmt.Sprintf("  content: %s\n", tmpl.Content))
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(templatesPath, []byte(sb.String()), 0644)
}

// Add adds a new template.
func (t *Templates) Add(name, content string) error {
	// Check for duplicate name
	for _, tmpl := range t.Items {
		if strings.EqualFold(tmpl.Name, name) {
			return fmt.Errorf("template with name %q already exists", name)
		}
	}

	now := time.Now()
	t.Items = append(t.Items, Template{
		Name:      name,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	})

	// Sort by name
	sort.Slice(t.Items, func(i, j int) bool {
		return strings.ToLower(t.Items[i].Name) < strings.ToLower(t.Items[j].Name)
	})

	return nil
}

// Update updates an existing template.
func (t *Templates) Update(name, newName, content string) error {
	for i, tmpl := range t.Items {
		if strings.EqualFold(tmpl.Name, name) {
			// Check if new name conflicts with another template
			if !strings.EqualFold(name, newName) {
				for _, other := range t.Items {
					if strings.EqualFold(other.Name, newName) {
						return fmt.Errorf("template with name %q already exists", newName)
					}
				}
			}

			t.Items[i].Name = newName
			t.Items[i].Content = content
			t.Items[i].UpdatedAt = time.Now()

			// Re-sort by name
			sort.Slice(t.Items, func(i, j int) bool {
				return strings.ToLower(t.Items[i].Name) < strings.ToLower(t.Items[j].Name)
			})

			return nil
		}
	}

	return fmt.Errorf("template %q not found", name)
}

// Delete removes a template by name.
func (t *Templates) Delete(name string) error {
	for i, tmpl := range t.Items {
		if strings.EqualFold(tmpl.Name, name) {
			t.Items = append(t.Items[:i], t.Items[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("template %q not found", name)
}

// Find finds a template by name.
func (t *Templates) Find(name string) *Template {
	for i, tmpl := range t.Items {
		if strings.EqualFold(tmpl.Name, name) {
			return &t.Items[i]
		}
	}
	return nil
}

// Filter returns templates matching the search query (fuzzy match on name).
func (t *Templates) Filter(query string) []Template {
	if query == "" {
		return t.Items
	}

	query = strings.ToLower(query)
	var results []Template

	for _, tmpl := range t.Items {
		if fuzzyMatch(strings.ToLower(tmpl.Name), query) ||
			strings.Contains(strings.ToLower(tmpl.Content), query) {
			results = append(results, tmpl)
		}
	}

	return results
}

// fuzzyMatch performs a simple fuzzy matching.
// Returns true if all characters in pattern appear in str in order.
func fuzzyMatch(str, pattern string) bool {
	if pattern == "" {
		return true
	}
	if str == "" {
		return false
	}

	patternIdx := 0
	for i := 0; i < len(str) && patternIdx < len(pattern); i++ {
		if str[i] == pattern[patternIdx] {
			patternIdx++
		}
	}

	return patternIdx == len(pattern)
}
