// Package config handles PAW configuration parsing and management.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Model represents the Claude model to use.
type Model string

const (
	ModelHaiku  Model = "haiku"
	ModelSonnet Model = "sonnet"
	ModelOpus   Model = "opus"
)

// DefaultModel is the default model for new tasks.
const DefaultModel = ModelOpus

// validModels is a pre-allocated slice of valid models (avoids allocation on each call).
var validModels = []Model{ModelOpus, ModelSonnet, ModelHaiku}

// ValidModels returns all valid model options.
// The returned slice should not be modified.
func ValidModels() []Model {
	return validModels
}

// DependsOnCondition defines when a task should run relative to another task.
type DependsOnCondition string

const (
	DependsOnNone    DependsOnCondition = ""
	DependsOnSuccess DependsOnCondition = "success"
	DependsOnFailure DependsOnCondition = "failure"
	DependsOnAlways  DependsOnCondition = "always"
)

// TaskDependency represents a dependency on another task.
type TaskDependency struct {
	TaskName  string             `json:"task_name"`
	Condition DependsOnCondition `json:"condition"`
}

// TaskOptions represents per-task settings that can override project config.
type TaskOptions struct {
	// Model specifies which Claude model to use (haiku, sonnet, opus)
	Model Model `json:"model,omitempty"`

	// DependsOn specifies a task dependency
	DependsOn *TaskDependency `json:"depends_on,omitempty"`

	// PreWorktreeHook overrides the project's pre-worktree hook for this task
	PreWorktreeHook string `json:"pre_worktree_hook,omitempty"`

	// BranchName specifies a custom branch name (default: auto-generated from task content)
	BranchName string `json:"branch_name,omitempty"`
}

// DefaultTaskOptions returns the default task options.
func DefaultTaskOptions() *TaskOptions {
	return &TaskOptions{
		Model: DefaultModel,
	}
}

// GetOptionsPath returns the path to the task options file.
func GetOptionsPath(agentDir string) string {
	return filepath.Join(agentDir, ".options.json")
}

// Save writes the task options to a file in the agent directory.
func (o *TaskOptions) Save(agentDir string) error {
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal task options: %w", err)
	}

	optionsPath := GetOptionsPath(agentDir)
	if err := os.WriteFile(optionsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write task options: %w", err)
	}

	return nil
}

// LoadTaskOptions loads task options from the agent directory.
func LoadTaskOptions(agentDir string) (*TaskOptions, error) {
	optionsPath := GetOptionsPath(agentDir)

	data, err := os.ReadFile(optionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultTaskOptions(), nil
		}
		return nil, fmt.Errorf("failed to read task options: %w", err)
	}

	var opts TaskOptions
	if err := json.Unmarshal(data, &opts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task options: %w", err)
	}

	return &opts, nil
}

// Merge applies non-zero values from another TaskOptions.
func (o *TaskOptions) Merge(other *TaskOptions) {
	if other == nil {
		return
	}

	if other.Model != "" {
		o.Model = other.Model
	}

	if other.DependsOn != nil {
		o.DependsOn = other.DependsOn
	}

	if other.PreWorktreeHook != "" {
		o.PreWorktreeHook = other.PreWorktreeHook
	}

	if other.BranchName != "" {
		o.BranchName = other.BranchName
	}
}

// Clone creates a deep copy of the task options.
func (o *TaskOptions) Clone() *TaskOptions {
	clone := &TaskOptions{
		Model:           o.Model,
		PreWorktreeHook: o.PreWorktreeHook,
		BranchName:      o.BranchName,
	}

	if o.DependsOn != nil {
		clone.DependsOn = &TaskDependency{
			TaskName:  o.DependsOn.TaskName,
			Condition: o.DependsOn.Condition,
		}
	}

	return clone
}
