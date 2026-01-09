package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadTemplates_NonExistentFile(t *testing.T) {
	tempDir := t.TempDir()

	templates, err := LoadTemplates(tempDir)
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v, want nil", err)
	}

	if templates == nil {
		t.Fatal("LoadTemplates() returned nil")
	}

	if len(templates.Items) != 0 {
		t.Errorf("LoadTemplates() returned %d items, want 0", len(templates.Items))
	}
}

func TestLoadTemplates_ValidFile(t *testing.T) {
	tempDir := t.TempDir()
	templatesPath := filepath.Join(tempDir, TemplatesFileName)

	content := `# PAW Templates
# Task templates for quick task creation

- name: Feature
  created_at: 2024-01-01T10:00:00Z
  updated_at: 2024-01-01T10:00:00Z
  content: Add new feature

- name: Bug Fix
  created_at: 2024-01-02T10:00:00Z
  updated_at: 2024-01-02T10:00:00Z
  content: |
    Fix bug in component
    Add tests
`

	if err := os.WriteFile(templatesPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	templates, err := LoadTemplates(tempDir)
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	if len(templates.Items) != 2 {
		t.Fatalf("LoadTemplates() returned %d items, want 2", len(templates.Items))
	}

	// Check first template
	tmpl1 := templates.Items[0]
	if tmpl1.Name != "Feature" {
		t.Errorf("Template 0 name = %q, want %q", tmpl1.Name, "Feature")
	}
	if tmpl1.Content != "Add new feature" {
		t.Errorf("Template 0 content = %q, want %q", tmpl1.Content, "Add new feature")
	}

	// Check second template
	tmpl2 := templates.Items[1]
	if tmpl2.Name != "Bug Fix" {
		t.Errorf("Template 1 name = %q, want %q", tmpl2.Name, "Bug Fix")
	}
	expectedContent := "Fix bug in component\nAdd tests"
	if tmpl2.Content != expectedContent {
		t.Errorf("Template 1 content = %q, want %q", tmpl2.Content, expectedContent)
	}
}

func TestParseTemplates_SingleLineContent(t *testing.T) {
	content := `- name: Test Template
  content: Simple content
  created_at: 2024-01-01T10:00:00Z
  updated_at: 2024-01-01T10:00:00Z
`

	templates, err := parseTemplates(content)
	if err != nil {
		t.Fatalf("parseTemplates() error = %v", err)
	}

	if len(templates.Items) != 1 {
		t.Fatalf("parseTemplates() returned %d items, want 1", len(templates.Items))
	}

	tmpl := templates.Items[0]
	if tmpl.Name != "Test Template" {
		t.Errorf("Template name = %q, want %q", tmpl.Name, "Test Template")
	}
	if tmpl.Content != "Simple content" {
		t.Errorf("Template content = %q, want %q", tmpl.Content, "Simple content")
	}
}

func TestParseTemplates_MultiLineContent(t *testing.T) {
	content := `- name: Multi Line
  content: |
    Line 1
    Line 2
    Line 3
  created_at: 2024-01-01T10:00:00Z
  updated_at: 2024-01-01T10:00:00Z
`

	templates, err := parseTemplates(content)
	if err != nil {
		t.Fatalf("parseTemplates() error = %v", err)
	}

	if len(templates.Items) != 1 {
		t.Fatalf("parseTemplates() returned %d items, want 1", len(templates.Items))
	}

	tmpl := templates.Items[0]
	expectedContent := "Line 1\nLine 2\nLine 3"
	if tmpl.Content != expectedContent {
		t.Errorf("Template content = %q, want %q", tmpl.Content, expectedContent)
	}
}

func TestParseTemplates_MultiLineContentWithEmptyLines(t *testing.T) {
	// Empty lines without leading spaces are currently skipped by the parser
	content := `- name: With Empty Lines
  content: |
    Line 1

    Line 3
  created_at: 2024-01-01T10:00:00Z
  updated_at: 2024-01-01T10:00:00Z
`

	templates, err := parseTemplates(content)
	if err != nil {
		t.Fatalf("parseTemplates() error = %v", err)
	}

	if len(templates.Items) != 1 {
		t.Fatalf("parseTemplates() returned %d items, want 1", len(templates.Items))
	}

	tmpl := templates.Items[0]
	// Current behavior: empty line without leading spaces is skipped
	expectedContent := "Line 1\nLine 3"
	if tmpl.Content != expectedContent {
		t.Errorf("Template content = %q, want %q", tmpl.Content, expectedContent)
	}
}

func TestParseTemplates_MultiLineContentWithIndentedEmptyLines(t *testing.T) {
	// Empty lines WITH 4 leading spaces are preserved
	content := "- name: With Indented Empty Lines\n" +
		"  content: |\n" +
		"    Line 1\n" +
		"    \n" +  // 4 spaces on empty line
		"    Line 3\n" +
		"  created_at: 2024-01-01T10:00:00Z\n" +
		"  updated_at: 2024-01-01T10:00:00Z\n"

	templates, err := parseTemplates(content)
	if err != nil {
		t.Fatalf("parseTemplates() error = %v", err)
	}

	if len(templates.Items) != 1 {
		t.Fatalf("parseTemplates() returned %d items, want 1", len(templates.Items))
	}

	tmpl := templates.Items[0]
	expectedContent := "Line 1\n\nLine 3"
	if tmpl.Content != expectedContent {
		t.Errorf("Template content = %q, want %q", tmpl.Content, expectedContent)
	}
}

func TestParseTemplates_MultipleTemplates(t *testing.T) {
	content := `- name: First
  content: First content
  created_at: 2024-01-01T10:00:00Z
  updated_at: 2024-01-01T10:00:00Z

- name: Second
  content: Second content
  created_at: 2024-01-02T10:00:00Z
  updated_at: 2024-01-02T10:00:00Z
`

	templates, err := parseTemplates(content)
	if err != nil {
		t.Fatalf("parseTemplates() error = %v", err)
	}

	if len(templates.Items) != 2 {
		t.Fatalf("parseTemplates() returned %d items, want 2", len(templates.Items))
	}

	if templates.Items[0].Name != "First" {
		t.Errorf("Template 0 name = %q, want %q", templates.Items[0].Name, "First")
	}
	if templates.Items[1].Name != "Second" {
		t.Errorf("Template 1 name = %q, want %q", templates.Items[1].Name, "Second")
	}
}

func TestParseTemplates_EmptyContent(t *testing.T) {
	content := ""

	templates, err := parseTemplates(content)
	if err != nil {
		t.Fatalf("parseTemplates() error = %v", err)
	}

	if len(templates.Items) != 0 {
		t.Errorf("parseTemplates() returned %d items, want 0", len(templates.Items))
	}
}

func TestTemplatesSave(t *testing.T) {
	tempDir := t.TempDir()
	templatesPath := filepath.Join(tempDir, TemplatesFileName)

	templates := &Templates{
		Items: []Template{
			{
				Name:      "Test Template",
				Content:   "Test content",
				CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	err := templates.Save(tempDir)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(templatesPath); os.IsNotExist(err) {
		t.Fatal("Save() did not create templates file")
	}

	// Read and verify content
	data, err := os.ReadFile(templatesPath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Test Template") {
		t.Error("Saved content does not contain template name")
	}
	if !strings.Contains(content, "Test content") {
		t.Error("Saved content does not contain template content")
	}
}

func TestTemplatesSave_MultiLineContent(t *testing.T) {
	tempDir := t.TempDir()

	templates := &Templates{
		Items: []Template{
			{
				Name:      "Multi Line",
				Content:   "Line 1\nLine 2\nLine 3",
				CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	err := templates.Save(tempDir)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Read and parse back
	loaded, err := LoadTemplates(tempDir)
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	if len(loaded.Items) != 1 {
		t.Fatalf("Loaded %d items, want 1", len(loaded.Items))
	}

	if loaded.Items[0].Content != "Line 1\nLine 2\nLine 3" {
		t.Errorf("Loaded content = %q, want %q", loaded.Items[0].Content, "Line 1\nLine 2\nLine 3")
	}
}

func TestTemplatesAdd(t *testing.T) {
	templates := &Templates{Items: []Template{}}

	err := templates.Add("New Template", "New content")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if len(templates.Items) != 1 {
		t.Fatalf("Add() resulted in %d items, want 1", len(templates.Items))
	}

	tmpl := templates.Items[0]
	if tmpl.Name != "New Template" {
		t.Errorf("Added template name = %q, want %q", tmpl.Name, "New Template")
	}
	if tmpl.Content != "New content" {
		t.Errorf("Added template content = %q, want %q", tmpl.Content, "New content")
	}
}

func TestTemplatesAdd_Duplicate(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Existing", Content: "Content"},
		},
	}

	err := templates.Add("Existing", "New content")
	if err == nil {
		t.Fatal("Add() with duplicate name should return error")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Add() error = %v, want error containing 'already exists'", err)
	}
}

func TestTemplatesAdd_CaseInsensitiveDuplicate(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Existing", Content: "Content"},
		},
	}

	err := templates.Add("EXISTING", "New content")
	if err == nil {
		t.Fatal("Add() with case-insensitive duplicate should return error")
	}
}

func TestTemplatesAdd_Sorting(t *testing.T) {
	templates := &Templates{Items: []Template{}}

	if err := templates.Add("Zebra", "Content Z"); err != nil {
		t.Fatalf("Add(Zebra) error = %v", err)
	}
	if err := templates.Add("Apple", "Content A"); err != nil {
		t.Fatalf("Add(Apple) error = %v", err)
	}
	if err := templates.Add("Banana", "Content B"); err != nil {
		t.Fatalf("Add(Banana) error = %v", err)
	}

	// Check items are sorted by name (case-insensitive)
	if templates.Items[0].Name != "Apple" {
		t.Errorf("Item 0 name = %q, want %q", templates.Items[0].Name, "Apple")
	}
	if templates.Items[1].Name != "Banana" {
		t.Errorf("Item 1 name = %q, want %q", templates.Items[1].Name, "Banana")
	}
	if templates.Items[2].Name != "Zebra" {
		t.Errorf("Item 2 name = %q, want %q", templates.Items[2].Name, "Zebra")
	}
}

func TestTemplatesUpdate(t *testing.T) {
	now := time.Now()
	templates := &Templates{
		Items: []Template{
			{Name: "Original", Content: "Old content", CreatedAt: now, UpdatedAt: now},
		},
	}

	time.Sleep(10 * time.Millisecond) // Ensure UpdatedAt changes

	err := templates.Update("Original", "Updated", "New content")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if len(templates.Items) != 1 {
		t.Fatalf("Update() changed item count to %d, want 1", len(templates.Items))
	}

	tmpl := templates.Items[0]
	if tmpl.Name != "Updated" {
		t.Errorf("Updated template name = %q, want %q", tmpl.Name, "Updated")
	}
	if tmpl.Content != "New content" {
		t.Errorf("Updated template content = %q, want %q", tmpl.Content, "New content")
	}
	if !tmpl.UpdatedAt.After(now) {
		t.Error("UpdatedAt was not updated")
	}
}

func TestTemplatesUpdate_NotFound(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Existing", Content: "Content"},
		},
	}

	err := templates.Update("NonExistent", "NewName", "New content")
	if err == nil {
		t.Fatal("Update() with non-existent template should return error")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Update() error = %v, want error containing 'not found'", err)
	}
}

func TestTemplatesUpdate_NameConflict(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "First", Content: "Content 1"},
			{Name: "Second", Content: "Content 2"},
		},
	}

	err := templates.Update("First", "Second", "New content")
	if err == nil {
		t.Fatal("Update() with conflicting name should return error")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Update() error = %v, want error containing 'already exists'", err)
	}
}

func TestTemplatesUpdate_SameNameAllowed(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Template", Content: "Old content"},
		},
	}

	err := templates.Update("Template", "Template", "New content")
	if err != nil {
		t.Fatalf("Update() with same name error = %v", err)
	}

	if templates.Items[0].Content != "New content" {
		t.Errorf("Content = %q, want %q", templates.Items[0].Content, "New content")
	}
}

func TestTemplatesDelete(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "First", Content: "Content 1"},
			{Name: "Second", Content: "Content 2"},
		},
	}

	err := templates.Delete("First")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if len(templates.Items) != 1 {
		t.Fatalf("Delete() left %d items, want 1", len(templates.Items))
	}

	if templates.Items[0].Name != "Second" {
		t.Errorf("Remaining item name = %q, want %q", templates.Items[0].Name, "Second")
	}
}

func TestTemplatesDelete_NotFound(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Existing", Content: "Content"},
		},
	}

	err := templates.Delete("NonExistent")
	if err == nil {
		t.Fatal("Delete() with non-existent template should return error")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Delete() error = %v, want error containing 'not found'", err)
	}
}

func TestTemplatesDelete_CaseInsensitive(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Template", Content: "Content"},
		},
	}

	err := templates.Delete("TEMPLATE")
	if err != nil {
		t.Fatalf("Delete() with different case error = %v", err)
	}

	if len(templates.Items) != 0 {
		t.Errorf("Delete() left %d items, want 0", len(templates.Items))
	}
}

func TestTemplatesFind(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "First", Content: "Content 1"},
			{Name: "Second", Content: "Content 2"},
		},
	}

	tmpl := templates.Find("First")
	if tmpl == nil {
		t.Fatal("Find() returned nil")
	}

	if tmpl.Name != "First" {
		t.Errorf("Find() returned template with name %q, want %q", tmpl.Name, "First")
	}
}

func TestTemplatesFind_NotFound(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Existing", Content: "Content"},
		},
	}

	tmpl := templates.Find("NonExistent")
	if tmpl != nil {
		t.Errorf("Find() returned %v, want nil", tmpl)
	}
}

func TestTemplatesFind_CaseInsensitive(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Template", Content: "Content"},
		},
	}

	tmpl := templates.Find("TEMPLATE")
	if tmpl == nil {
		t.Fatal("Find() with different case returned nil")
	}

	if tmpl.Name != "Template" {
		t.Errorf("Find() returned template with name %q, want %q", tmpl.Name, "Template")
	}
}

func TestTemplatesFilter_EmptyQuery(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "First", Content: "Content 1"},
			{Name: "Second", Content: "Content 2"},
		},
	}

	results := templates.Filter("")
	if len(results) != 2 {
		t.Errorf("Filter(\"\") returned %d items, want 2", len(results))
	}
}

func TestTemplatesFilter_NameMatch(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Feature Request", Content: "Add feature"},
			{Name: "Bug Fix", Content: "Fix bug"},
		},
	}

	results := templates.Filter("feature")
	if len(results) != 1 {
		t.Fatalf("Filter(\"feature\") returned %d items, want 1", len(results))
	}

	if results[0].Name != "Feature Request" {
		t.Errorf("Filter result name = %q, want %q", results[0].Name, "Feature Request")
	}
}

func TestTemplatesFilter_ContentMatch(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Template 1", Content: "Add new feature"},
			{Name: "Template 2", Content: "Fix bug"},
		},
	}

	results := templates.Filter("feature")
	if len(results) != 1 {
		t.Fatalf("Filter(\"feature\") returned %d items, want 1", len(results))
	}

	if results[0].Name != "Template 1" {
		t.Errorf("Filter result name = %q, want %q", results[0].Name, "Template 1")
	}
}

func TestTemplatesFilter_FuzzyMatch(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Feature Request", Content: "Content"},
			{Name: "Bug Fix", Content: "Content"},
		},
	}

	// "ftr" should fuzzy match "Feature Request" (f-ea-t-u-r-e)
	results := templates.Filter("ftr")
	if len(results) != 1 {
		t.Fatalf("Filter(\"ftr\") returned %d items, want 1", len(results))
	}

	if results[0].Name != "Feature Request" {
		t.Errorf("Filter result name = %q, want %q", results[0].Name, "Feature Request")
	}
}

func TestTemplatesFilter_NoMatch(t *testing.T) {
	templates := &Templates{
		Items: []Template{
			{Name: "Template 1", Content: "Content 1"},
			{Name: "Template 2", Content: "Content 2"},
		},
	}

	results := templates.Filter("nonexistent")
	if len(results) != 0 {
		t.Errorf("Filter(\"nonexistent\") returned %d items, want 0", len(results))
	}
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		name    string
		str     string
		pattern string
		want    bool
	}{
		{"exact match", "hello", "hello", true},
		{"pattern in order", "hello world", "hlowrd", true},
		{"pattern at start", "hello", "hel", true},
		{"pattern at end", "hello", "llo", true},
		{"pattern scattered", "feature request", "ftr", true},
		{"empty pattern", "hello", "", true},
		{"empty string", "", "hello", false},
		{"both empty", "", "", true},
		{"pattern not in order", "hello", "leh", false},
		{"missing character", "hello", "helloz", false},
		{"case sensitive", "Hello", "hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyMatch(tt.str, tt.pattern)
			if got != tt.want {
				t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.str, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestTemplatesRoundTrip(t *testing.T) {
	tempDir := t.TempDir()

	// Create templates
	original := &Templates{
		Items: []Template{
			{
				Name:      "Feature",
				Content:   "Add new feature",
				CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			},
			{
				Name:      "Bug Fix",
				Content:   "Fix bug\nAdd tests",
				CreatedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	// Save
	if err := original.Save(tempDir); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load
	loaded, err := LoadTemplates(tempDir)
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	// Compare
	if len(loaded.Items) != len(original.Items) {
		t.Fatalf("Loaded %d items, want %d", len(loaded.Items), len(original.Items))
	}

	for i := range original.Items {
		if loaded.Items[i].Name != original.Items[i].Name {
			t.Errorf("Item %d name = %q, want %q", i, loaded.Items[i].Name, original.Items[i].Name)
		}
		if loaded.Items[i].Content != original.Items[i].Content {
			t.Errorf("Item %d content = %q, want %q", i, loaded.Items[i].Content, original.Items[i].Content)
		}
	}
}
