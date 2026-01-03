// Package git provides an interface for git operations.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dongho-jung/taw/internal/constants"
)

// Client defines the interface for git operations.
type Client interface {
	// Repository
	IsGitRepo(dir string) bool
	GetRepoRoot(dir string) (string, error)
	GetMainBranch(dir string) string

	// Worktree
	WorktreeAdd(projectDir, worktreeDir, branch string, createBranch bool) error
	WorktreeRemove(projectDir, worktreeDir string, force bool) error
	WorktreePrune(projectDir string) error
	WorktreeList(projectDir string) ([]Worktree, error)

	// Branch
	BranchExists(dir, branch string) bool
	BranchDelete(dir, branch string, force bool) error
	BranchMerged(dir, branch, into string) bool
	BranchCreate(dir, branch, startPoint string) error
	GetCurrentBranch(dir string) (string, error)

	// Changes
	HasChanges(dir string) bool
	HasStagedChanges(dir string) bool
	HasUntrackedFiles(dir string) bool
	GetUntrackedFiles(dir string) ([]string, error)
	StashCreate(dir string) (string, error)
	StashApply(dir, stashHash string) error
	StashPush(dir, message string) error
	StashPop(dir string) error

	// Commit
	Add(dir, path string) error
	AddAll(dir string) error
	Commit(dir, message string) error
	GetDiffStat(dir string) (string, error)

	// Remote
	Push(dir, remote, branch string, setUpstream bool) error
	Fetch(dir, remote string) error
	Pull(dir string) error

	// Merge
	Merge(dir, branch string, noFF bool, message string) error
	MergeSquash(dir, branch, message string) error
	MergeAbort(dir string) error
	HasConflicts(dir string) (bool, []string, error)
	CheckoutOurs(dir, path string) error
	CheckoutTheirs(dir, path string) error
	FindMergeCommit(dir, branch, into string) (string, error)
	RevertCommit(dir, commitHash, message string) error

	// Status
	Status(dir string) (string, error)
	Checkout(dir, target string) error
}

// Worktree represents a git worktree.
type Worktree struct {
	Path   string
	Branch string
	Head   string
}

// gitClient implements the Client interface.
type gitClient struct {
	timeout time.Duration
}

// New creates a new git client.
func New() Client {
	return &gitClient{
		timeout: constants.WorktreeTimeout,
	}
}

func (c *gitClient) cmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

func (c *gitClient) run(dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := c.cmd(ctx, dir, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

func (c *gitClient) runOutput(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := c.cmd(ctx, dir, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Repository

func (c *gitClient) IsGitRepo(dir string) bool {
	_, err := c.runOutput(dir, "rev-parse", "--git-dir")
	return err == nil
}

func (c *gitClient) GetRepoRoot(dir string) (string, error) {
	return c.runOutput(dir, "rev-parse", "--show-toplevel")
}

func (c *gitClient) GetMainBranch(dir string) string {
	// Try to get from origin/HEAD
	output, err := c.runOutput(dir, "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	if err == nil {
		parts := strings.Split(output, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Check if main exists
	if c.BranchExists(dir, "main") {
		return "main"
	}

	// Check if master exists
	if c.BranchExists(dir, "master") {
		return "master"
	}

	return constants.DefaultMainBranch
}

// Worktree

func (c *gitClient) WorktreeAdd(projectDir, worktreeDir, branch string, createBranch bool) error {
	args := []string{"worktree", "add"}
	if createBranch {
		args = append(args, "-b", branch)
	}
	args = append(args, worktreeDir)
	if !createBranch {
		args = append(args, branch)
	}
	return c.run(projectDir, args...)
}

func (c *gitClient) WorktreeRemove(projectDir, worktreeDir string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreeDir)
	return c.run(projectDir, args...)
}

func (c *gitClient) WorktreePrune(projectDir string) error {
	return c.run(projectDir, "worktree", "prune")
}

func (c *gitClient) WorktreeList(projectDir string) ([]Worktree, error) {
	output, err := c.runOutput(projectDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current Worktree

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = Worktree{}
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Head = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch ")
			// Remove refs/heads/ prefix
			current.Branch = strings.TrimPrefix(branch, "refs/heads/")
		}
	}

	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// Branch

func (c *gitClient) BranchExists(dir, branch string) bool {
	err := c.run(dir, "rev-parse", "--verify", fmt.Sprintf("refs/heads/%s", branch))
	return err == nil
}

func (c *gitClient) BranchDelete(dir, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	return c.run(dir, "branch", flag, branch)
}

func (c *gitClient) BranchMerged(dir, branch, into string) bool {
	output, err := c.runOutput(dir, "branch", "--merged", into)
	if err != nil {
		return false
	}

	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if name == branch {
			return true
		}
	}
	return false
}

func (c *gitClient) BranchCreate(dir, branch, startPoint string) error {
	args := []string{"branch", branch}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	return c.run(dir, args...)
}

func (c *gitClient) GetCurrentBranch(dir string) (string, error) {
	return c.runOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// Changes

func (c *gitClient) HasChanges(dir string) bool {
	output, err := c.runOutput(dir, "status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) != ""
}

func (c *gitClient) HasStagedChanges(dir string) bool {
	output, err := c.runOutput(dir, "diff", "--cached", "--name-only")
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) != ""
}

func (c *gitClient) HasUntrackedFiles(dir string) bool {
	output, err := c.runOutput(dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) != ""
}

func (c *gitClient) GetUntrackedFiles(dir string) ([]string, error) {
	output, err := c.runOutput(dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	return strings.Split(output, "\n"), nil
}

func (c *gitClient) StashCreate(dir string) (string, error) {
	return c.runOutput(dir, "stash", "create")
}

func (c *gitClient) StashApply(dir, stashHash string) error {
	return c.run(dir, "stash", "apply", stashHash)
}

func (c *gitClient) StashPush(dir, message string) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	return c.run(dir, args...)
}

func (c *gitClient) StashPop(dir string) error {
	return c.run(dir, "stash", "pop")
}

// Commit

func (c *gitClient) Add(dir, path string) error {
	return c.run(dir, "add", path)
}

func (c *gitClient) AddAll(dir string) error {
	return c.run(dir, "add", "-A")
}

func (c *gitClient) Commit(dir, message string) error {
	return c.run(dir, "commit", "-m", message)
}

func (c *gitClient) GetDiffStat(dir string) (string, error) {
	return c.runOutput(dir, "diff", "--cached", "--stat")
}

// Remote

func (c *gitClient) Push(dir, remote, branch string, setUpstream bool) error {
	args := []string{"push"}
	if setUpstream {
		args = append(args, "-u")
	}
	args = append(args, remote, branch)
	return c.run(dir, args...)
}

func (c *gitClient) Fetch(dir, remote string) error {
	return c.run(dir, "fetch", remote)
}

func (c *gitClient) Pull(dir string) error {
	return c.run(dir, "pull")
}

// Merge

func (c *gitClient) Merge(dir, branch string, noFF bool, message string) error {
	args := []string{"merge"}
	if noFF {
		args = append(args, "--no-ff")
	}
	if message != "" {
		args = append(args, "-m", message)
	}
	args = append(args, branch)
	return c.run(dir, args...)
}

func (c *gitClient) MergeSquash(dir, branch, message string) error {
	if err := c.run(dir, "merge", "--squash", branch); err != nil {
		return err
	}
	// Check if there are staged changes to commit
	// git merge --squash may succeed but result in no changes (already merged)
	if !c.HasStagedChanges(dir) {
		// No changes to commit - branch was already merged or has no new changes
		return nil
	}
	return c.Commit(dir, message)
}

func (c *gitClient) MergeAbort(dir string) error {
	return c.run(dir, "merge", "--abort")
}

func (c *gitClient) HasConflicts(dir string) (bool, []string, error) {
	output, err := c.runOutput(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, nil, err
	}

	if output == "" {
		return false, nil, nil
	}

	files := strings.Split(output, "\n")
	return true, files, nil
}

func (c *gitClient) CheckoutOurs(dir, path string) error {
	return c.run(dir, "checkout", "--ours", path)
}

func (c *gitClient) CheckoutTheirs(dir, path string) error {
	return c.run(dir, "checkout", "--theirs", path)
}

// FindMergeCommit finds the merge commit where branch was merged into another branch.
// Returns empty string if not found.
func (c *gitClient) FindMergeCommit(dir, branch, into string) (string, error) {
	// Find merge commits that mention the branch name in the commit message
	// or find the actual merge commit by ancestry
	output, err := c.runOutput(dir, "log", into, "--merges", "--oneline", "--grep="+branch, "-1", "--format=%H")
	if err != nil {
		return "", err
	}
	if output != "" {
		return strings.TrimSpace(output), nil
	}

	// Alternative: find by ancestry-path
	output, err = c.runOutput(dir, "log", into, "--merges", "--oneline", "--ancestry-path", branch+".."+into, "-1", "--format=%H")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// RevertCommit creates a revert commit for the given commit hash.
func (c *gitClient) RevertCommit(dir, commitHash, message string) error {
	args := []string{"revert", "--no-edit", "-m", "1", commitHash}
	if message != "" {
		args = []string{"revert", "-m", "1", "--no-edit", commitHash}
	}
	return c.run(dir, args...)
}

// Status

func (c *gitClient) Status(dir string) (string, error) {
	return c.runOutput(dir, "status", "-s")
}

func (c *gitClient) Checkout(dir, target string) error {
	return c.run(dir, "checkout", target)
}

// CopyUntrackedFiles copies untracked files from source to destination.
func CopyUntrackedFiles(files []string, srcDir, dstDir string) error {
	for _, file := range files {
		src := filepath.Join(srcDir, file)
		dst := filepath.Join(dstDir, file)

		// Create parent directory if needed
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", file, err)
		}

		// Read source file
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		// Get source file mode
		info, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("failed to stat %s: %w", file, err)
		}

		// Write destination file
		if err := os.WriteFile(dst, data, info.Mode()); err != nil {
			return fmt.Errorf("failed to write %s: %w", file, err)
		}
	}
	return nil
}
