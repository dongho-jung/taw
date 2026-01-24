// Package github provides an interface for GitHub CLI (gh) operations.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// bufferPool reuses bytes.Buffer instances to reduce allocations.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// Client defines the interface for GitHub CLI operations.
type Client interface {
	// IsInstalled checks if gh CLI is available.
	IsInstalled() bool

	// CreatePR creates a pull request and returns the PR number.
	CreatePR(dir, title, body, base string) (int, string, error)

	// GetPRStatus gets the status of a pull request.
	GetPRStatus(dir string, prNumber int) (*PRStatus, error)

	// IsPRMerged checks if a pull request has been merged.
	IsPRMerged(dir string, prNumber int) (bool, error)

	// ViewPRWeb opens the pull request in a web browser.
	ViewPRWeb(dir string, prNumber int) error
}

// PRStatus represents the status of a pull request.
type PRStatus struct {
	Number int    `json:"number"`
	State  string `json:"state"` // "open", "closed", "merged"
	Merged bool   `json:"merged"`
	URL    string `json:"url"`
}

// ghClient implements the Client interface.
type ghClient struct {
	timeout time.Duration
}

// New creates a new GitHub CLI client.
func New() Client {
	return &ghClient{
		timeout: 30 * time.Second,
	}
}

func (c *ghClient) cmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

func (c *ghClient) run(dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := c.cmd(ctx, dir, args...)

	// Reuse buffer from pool to reduce allocations
	stderr := bufferPool.Get().(*bytes.Buffer)
	stderr.Reset()
	defer bufferPool.Put(stderr)

	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

func (c *ghClient) runOutput(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := c.cmd(ctx, dir, args...)

	// Reuse buffers from pool to reduce allocations
	stdout := bufferPool.Get().(*bytes.Buffer)
	stderr := bufferPool.Get().(*bytes.Buffer)
	stdout.Reset()
	stderr.Reset()
	defer bufferPool.Put(stdout)
	defer bufferPool.Put(stderr)

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// IsInstalled checks if gh CLI is available.
func (c *ghClient) IsInstalled() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// CreatePR creates a pull request and returns the PR number.
func (c *ghClient) CreatePR(dir, title, body, base string) (int, string, error) {
	args := []string{"pr", "create", "--title", title, "--body", body}
	if base != "" {
		args = append(args, "--base", base)
	}

	output, err := c.runOutput(dir, args...)
	if err != nil {
		return 0, "", fmt.Errorf("failed to create PR: %w", err)
	}

	// The output is the PR URL, extract the number
	// Format: https://github.com/owner/repo/pull/123
	prURL := strings.TrimSpace(output)
	parts := strings.Split(prURL, "/")
	if len(parts) < 1 {
		return 0, "", fmt.Errorf("unexpected PR URL format: %s", output)
	}

	var prNumber int
	if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &prNumber); err != nil {
		return 0, "", fmt.Errorf("failed to parse PR number from %s: %w", output, err)
	}

	return prNumber, prURL, nil
}

// GetPRStatus gets the status of a pull request.
func (c *ghClient) GetPRStatus(dir string, prNumber int) (*PRStatus, error) {
	output, err := c.runOutput(dir, "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "number,state,merged,url")
	if err != nil {
		return nil, fmt.Errorf("failed to get PR status: %w", err)
	}

	var status PRStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		return nil, fmt.Errorf("failed to parse PR status: %w", err)
	}

	return &status, nil
}

// IsPRMerged checks if a pull request has been merged.
func (c *ghClient) IsPRMerged(dir string, prNumber int) (bool, error) {
	status, err := c.GetPRStatus(dir, prNumber)
	if err != nil {
		return false, err
	}
	return status.Merged, nil
}

// ViewPRWeb opens the pull request in a web browser.
func (c *ghClient) ViewPRWeb(dir string, prNumber int) error {
	return c.run(dir, "pr", "view", fmt.Sprintf("%d", prNumber), "--web")
}
