package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTaskOptions(t *testing.T) {
	opts := DefaultTaskOptions()

	if opts.Model != DefaultModel {
		t.Errorf("Expected model %s, got %s", DefaultModel, opts.Model)
	}

	if opts.DependsOn != nil {
		t.Error("Expected DependsOn to be nil by default")
	}

	if opts.PreWorktreeHook != "" {
		t.Error("Expected PreWorktreeHook to be empty by default")
	}
}

func TestValidModels(t *testing.T) {
	models := ValidModels()

	expected := []Model{ModelOpus, ModelSonnet, ModelHaiku}
	if len(models) != len(expected) {
		t.Errorf("Expected %d models, got %d", len(expected), len(models))
	}

	for i, m := range models {
		if m != expected[i] {
			t.Errorf("Expected model %s at position %d, got %s", expected[i], i, m)
		}
	}
}

func TestTaskOptionsSaveLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskopts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create task options with custom values
	opts := &TaskOptions{
		Model: ModelHaiku,
		DependsOn: &TaskDependency{
			TaskName:  "other-task",
			Condition: DependsOnSuccess,
		},
		PreWorktreeHook: "npm install",
	}

	// Save options
	if err := opts.Save(tmpDir); err != nil {
		t.Fatalf("Failed to save options: %v", err)
	}

	// Verify file exists
	optionsPath := GetOptionsPath(tmpDir)
	if _, err := os.Stat(optionsPath); os.IsNotExist(err) {
		t.Fatal("Options file was not created")
	}

	// Load options
	loaded, err := LoadTaskOptions(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load options: %v", err)
	}

	// Verify loaded values
	if loaded.Model != opts.Model {
		t.Errorf("Expected model %s, got %s", opts.Model, loaded.Model)
	}

	if loaded.DependsOn == nil {
		t.Fatal("Expected DependsOn to be non-nil")
	}

	if loaded.DependsOn.TaskName != opts.DependsOn.TaskName {
		t.Errorf("Expected task name %s, got %s", opts.DependsOn.TaskName, loaded.DependsOn.TaskName)
	}

	if loaded.DependsOn.Condition != opts.DependsOn.Condition {
		t.Errorf("Expected condition %s, got %s", opts.DependsOn.Condition, loaded.DependsOn.Condition)
	}

	if loaded.PreWorktreeHook != opts.PreWorktreeHook {
		t.Errorf("Expected pre-worktree hook %s, got %s", opts.PreWorktreeHook, loaded.PreWorktreeHook)
	}
}

func TestLoadTaskOptionsNotExist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskopts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Load options from non-existent file (should return defaults)
	opts, err := LoadTaskOptions(tmpDir)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if opts.Model != DefaultModel {
		t.Errorf("Expected default model %s, got %s", DefaultModel, opts.Model)
	}
}

func TestLoadTaskOptionsInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "taskopts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Write invalid JSON
	optionsPath := filepath.Join(tmpDir, ".options.json")
	if err := os.WriteFile(optionsPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Load should fail
	_, err = LoadTaskOptions(tmpDir)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestTaskOptionsMerge(t *testing.T) {
	base := DefaultTaskOptions()
	other := &TaskOptions{
		Model:           ModelHaiku,
		PreWorktreeHook: "make build",
	}

	base.Merge(other)

	if base.Model != ModelHaiku {
		t.Errorf("Expected model %s after merge, got %s", ModelHaiku, base.Model)
	}

	if base.PreWorktreeHook != "make build" {
		t.Errorf("Expected pre-worktree hook 'make build', got '%s'", base.PreWorktreeHook)
	}
}

func TestTaskOptionsMergeNil(t *testing.T) {
	base := DefaultTaskOptions()
	originalModel := base.Model

	base.Merge(nil)

	if base.Model != originalModel {
		t.Errorf("Expected model to remain %s after nil merge, got %s", originalModel, base.Model)
	}
}

func TestTaskOptionsClone(t *testing.T) {
	original := &TaskOptions{
		Model: ModelSonnet,
		DependsOn: &TaskDependency{
			TaskName:  "task-1",
			Condition: DependsOnFailure,
		},
		PreWorktreeHook: "go build",
	}

	clone := original.Clone()

	// Verify values are the same
	if clone.Model != original.Model {
		t.Errorf("Clone model mismatch: %s vs %s", clone.Model, original.Model)
	}

	// Verify DependsOn is a separate object
	if clone.DependsOn == original.DependsOn {
		t.Error("Clone DependsOn should be a separate object")
	}

	if clone.DependsOn.TaskName != original.DependsOn.TaskName {
		t.Errorf("Clone DependsOn task name mismatch: %s vs %s", clone.DependsOn.TaskName, original.DependsOn.TaskName)
	}

	// Modify clone and verify original is unchanged
	clone.Model = ModelHaiku
	clone.DependsOn.TaskName = "modified-task"

	if original.Model == ModelHaiku {
		t.Error("Original model was modified")
	}

	if original.DependsOn.TaskName == "modified-task" {
		t.Error("Original DependsOn was modified")
	}
}

func TestGetOptionsPath(t *testing.T) {
	path := GetOptionsPath("/test/agent/dir")
	expected := "/test/agent/dir/.options.json"

	if path != expected {
		t.Errorf("Expected path %s, got %s", expected, path)
	}
}

func TestTaskOptionsJSONMarshal(t *testing.T) {
	opts := &TaskOptions{
		Model: ModelSonnet,
		DependsOn: &TaskDependency{
			TaskName:  "my-task",
			Condition: DependsOnAlways,
		},
		PreWorktreeHook: "npm test",
	}

	data, err := json.Marshal(opts)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Verify JSON contains expected fields
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed["model"] != "sonnet" {
		t.Errorf("Expected model 'sonnet' in JSON, got %v", parsed["model"])
	}
}
