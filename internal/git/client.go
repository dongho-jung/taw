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
	"sync"
	"time"

	"github.com/dongho-jung/paw/internal/constants"
)

// bufferPool reuses bytes.Buffer instances to reduce allocations in run/runOutput.
var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// Client defines the interface for git operations.
type Client interface {
	// Repository
	IsGitRepo(dir string) bool
	GetRepoRoot(dir string) (string, error)
	GetMainBranch(dir string) string
	HasRemote(dir, remote string) bool

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
	BranchCreateOrphan(dir, branch string) error // Create orphan branch (no parent)
	GetCurrentBranch(dir string) (string, error)
	GetHeadCommit(dir string) (string, error)

	// Changes
	HasChanges(dir string) bool
	HasStagedChanges(dir string) bool
	HasUntrackedFiles(dir string) bool
	GetUntrackedFiles(dir string) ([]string, error)
	StashCreate(dir string) (string, error)
	StashApply(dir, stashHash string) error
	StashPush(dir, message string) error
	StashPop(dir string) error
	StashList(dir string) ([]StashEntry, error)
	StashPopByMessage(dir, message string) error
	StashDropByMessage(dir, message string) error

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
	HasOngoingMerge(dir string) bool
	CheckoutOurs(dir, path string) error
	CheckoutTheirs(dir, path string) error
	FindMergeCommit(dir, branch, into string) (string, error)
	RevertCommit(dir, commitHash, message string) error

	// Rebase
	Rebase(dir, onto string) error
	RebaseAbort(dir string) error
	HasOngoingRebase(dir string) bool

	// Status
	Status(dir string) (string, error)
	Checkout(dir, target string) error

	// Log
	GetBranchCommits(dir, branch, baseBranch string, maxCount int) ([]CommitInfo, error)

	// Index
	UpdateIndexAssumeUnchanged(dir, path string) error
	IsFileStaged(dir, path string) (bool, error)
	IsFileTracked(dir, path string) bool
	ResetPath(dir, path string) error
}

// CommitInfo represents basic information about a git commit.
type CommitInfo struct {
	Hash    string
	Subject string
}

// Worktree represents a git worktree.
type Worktree struct {
	Path   string
	Branch string
	Head   string
}

// StashEntry represents a git stash entry.
type StashEntry struct {
	Index   int    // 0 for stash@{0}, 1 for stash@{1}, etc.
	Message string // The stash message
}

// gitClient implements the Client interface.
type gitClient struct {
	timeout time.Duration
}

// Compile-time check that gitClient implements Client interface.
var _ Client = (*gitClient)(nil)

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

func (c *gitClient) runOutput(dir string, args ...string) (string, error) {
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

// HasRemote checks if a remote with the given name exists.
func (c *gitClient) HasRemote(dir, remote string) bool {
	output, err := c.runOutput(dir, "remote")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == remote {
			return true
		}
	}
	return false
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

		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
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
	err := c.run(dir, "rev-parse", "--verify", "refs/heads/"+branch)
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

// BranchCreateOrphan creates an orphan branch with an empty "init" commit.
// This is useful for creating a main branch in a repository that doesn't have one.
func (c *gitClient) BranchCreateOrphan(dir, branch string) error {
	// Create orphan branch (no parent commits)
	if err := c.run(dir, "checkout", "--orphan", branch); err != nil {
		return err
	}
	// Remove all files from index (but keep in working directory)
	_ = c.run(dir, "rm", "-rf", "--cached", ".")
	// Create empty init commit
	return c.run(dir, "commit", "--allow-empty", "-m", "init")
}

func (c *gitClient) GetCurrentBranch(dir string) (string, error) {
	return c.runOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

func (c *gitClient) GetHeadCommit(dir string) (string, error) {
	return c.runOutput(dir, "rev-parse", "HEAD")
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
	args := []string{"stash", "push", "--include-untracked"}
	if message != "" {
		args = append(args, "-m", message)
	}
	return c.run(dir, args...)
}

func (c *gitClient) StashPop(dir string) error {
	return c.run(dir, "stash", "pop")
}

// StashList returns all stash entries with their indices and messages.
func (c *gitClient) StashList(dir string) ([]StashEntry, error) {
	output, err := c.runOutput(dir, "stash", "list", "--format=%gs")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	entries := make([]StashEntry, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entries = append(entries, StashEntry{
			Index:   i,
			Message: line,
		})
	}
	return entries, nil
}

// StashPopByMessage finds a stash by message and pops it.
// If the stash is not found, returns nil (no-op).
// If pop fails (e.g., due to conflicts), drops the stash to prevent orphaned entries.
func (c *gitClient) StashPopByMessage(dir, message string) error {
	entries, err := c.StashList(dir)
	if err != nil {
		return fmt.Errorf("failed to list stashes: %w", err)
	}

	// Find stash with matching message
	for _, entry := range entries {
		if strings.Contains(entry.Message, message) {
			// Try to pop this specific stash
			stashRef := fmt.Sprintf("stash@{%d}", entry.Index)
			if popErr := c.run(dir, "stash", "pop", stashRef); popErr != nil {
				// Pop failed (likely conflict), drop the stash to prevent orphaned entry
				_ = c.run(dir, "stash", "drop", stashRef)
				return fmt.Errorf("failed to pop stash (dropped to prevent orphan): %w", popErr)
			}
			return nil
		}
	}

	// Stash not found - this is OK, might have been cleaned up already
	return nil
}

// StashDropByMessage finds a stash by message and drops it without applying.
// If the stash is not found, returns nil (no-op).
func (c *gitClient) StashDropByMessage(dir, message string) error {
	entries, err := c.StashList(dir)
	if err != nil {
		return fmt.Errorf("failed to list stashes: %w", err)
	}

	// Find stash with matching message
	for _, entry := range entries {
		if strings.Contains(entry.Message, message) {
			stashRef := fmt.Sprintf("stash@{%d}", entry.Index)
			return c.run(dir, "stash", "drop", stashRef)
		}
	}

	// Stash not found - this is OK
	return nil
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

// HasOngoingMerge checks if there's an ongoing merge operation.
// This is indicated by the presence of MERGE_HEAD in the git directory.
func (c *gitClient) HasOngoingMerge(dir string) bool {
	gitDir, err := c.runOutput(dir, "rev-parse", "--git-dir")
	if err != nil {
		return false
	}

	// Make gitDir absolute if it's relative
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}

	mergeHeadPath := filepath.Join(gitDir, "MERGE_HEAD")
	_, err = os.Stat(mergeHeadPath)
	return err == nil
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

// Rebase

// Rebase rebases the current branch onto the given target.
func (c *gitClient) Rebase(dir, onto string) error {
	return c.run(dir, "rebase", onto)
}

// RebaseAbort aborts an ongoing rebase operation.
func (c *gitClient) RebaseAbort(dir string) error {
	return c.run(dir, "rebase", "--abort")
}

// HasOngoingRebase checks if there's an ongoing rebase operation.
// This is indicated by the presence of rebase-merge or rebase-apply directory.
func (c *gitClient) HasOngoingRebase(dir string) bool {
	gitDir, err := c.runOutput(dir, "rev-parse", "--git-dir")
	if err != nil {
		return false
	}

	// Make gitDir absolute if it's relative
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}

	// Check for interactive rebase
	rebaseMergePath := filepath.Join(gitDir, "rebase-merge")
	if _, err := os.Stat(rebaseMergePath); err == nil {
		return true
	}

	// Check for non-interactive rebase
	rebaseApplyPath := filepath.Join(gitDir, "rebase-apply")
	if _, err := os.Stat(rebaseApplyPath); err == nil {
		return true
	}

	return false
}

// Status

func (c *gitClient) Status(dir string) (string, error) {
	return c.runOutput(dir, "status", "-s")
}

func (c *gitClient) Checkout(dir, target string) error {
	return c.run(dir, "checkout", target)
}

// GetBranchCommits returns commit information for commits unique to a branch.
// It returns commits that are in 'branch' but not in 'baseBranch'.
func (c *gitClient) GetBranchCommits(dir, branch, baseBranch string, maxCount int) ([]CommitInfo, error) {
	// Use git log with a specific format to get commits unique to the branch
	args := []string{"log", "--oneline", "--format=%H %s", fmt.Sprintf("%s..%s", baseBranch, branch)}
	if maxCount > 0 {
		args = append(args, fmt.Sprintf("-n%d", maxCount))
	}

	output, err := c.runOutput(dir, args...)
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	commits := make([]CommitInfo, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format is "hash subject..."
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		commits = append(commits, CommitInfo{
			Hash:    parts[0],
			Subject: parts[1],
		})
	}

	return commits, nil
}

// GenerateMergeCommitMessage generates a well-formatted merge commit message.
// It includes the inferred commit type, formatted subject, and commit history.
func GenerateMergeCommitMessage(taskName string, commits []CommitInfo) string {
	commitType := constants.InferCommitType(taskName)
	subject := constants.FormatTaskNameForCommit(taskName)

	// Build the commit message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("%s: %s\n", commitType, subject))

	// Add commit history as body if there are any commits
	if len(commits) > 0 {
		msg.WriteString("\n")
		msg.WriteString("Changes:\n")
		for _, commit := range commits {
			// Truncate long subjects
			subj := commit.Subject
			if len(subj) > 72 {
				subj = subj[:69] + "..."
			}
			msg.WriteString(fmt.Sprintf("- %s\n", subj))
		}
	}

	return msg.String()
}

// CopyUntrackedFiles copies untracked files from source to destination.
func CopyUntrackedFiles(files []string, srcDir, dstDir string) error {
	for _, file := range files {
		src := filepath.Join(srcDir, file)
		dst := filepath.Join(dstDir, file)

		// Create parent directory if needed
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil { //nolint:gosec // G301: standard directory permissions
			return fmt.Errorf("failed to create directory for %s: %w", file, err)
		}

		// Read source file
		data, err := os.ReadFile(src) //nolint:gosec // G304: src is from controlled list
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

// Index operations

// UpdateIndexAssumeUnchanged marks a file as assume-unchanged in the git index.
// This tells git to ignore changes to the file even if explicitly staged.
func (c *gitClient) UpdateIndexAssumeUnchanged(dir, path string) error {
	return c.run(dir, "update-index", "--assume-unchanged", path)
}

// IsFileStaged checks if a file is currently staged in the git index.
func (c *gitClient) IsFileStaged(dir, path string) (bool, error) {
	// Use git diff --cached --name-only to list staged files
	output, err := c.runOutput(dir, "diff", "--cached", "--name-only", path)
	if err != nil {
		return false, err
	}
	// If output is not empty, the file is staged
	return strings.TrimSpace(output) != "", nil
}

// IsFileTracked checks if a file is tracked in the git index.
// Returns true if the file exists in the index (tracked), false otherwise.
func (c *gitClient) IsFileTracked(dir, path string) bool {
	// git ls-files --error-unmatch exits with status 1 if file is not tracked
	err := c.run(dir, "ls-files", "--error-unmatch", path)
	return err == nil
}

// ResetPath unstages a file from the git index.
func (c *gitClient) ResetPath(dir, path string) error {
	return c.run(dir, "reset", "HEAD", "--", path)
}

// AddToExcludeFile adds a pattern to the worktree's .git/info/exclude file.
// This provides worktree-specific exclusion without modifying .gitignore.
func AddToExcludeFile(worktreeDir, pattern string) error {
	// In a worktree, .git is a file (not a directory), so we need to resolve the actual git directory
	cmd := exec.Command("git", "-C", worktreeDir, "rev-parse", "--git-dir")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get git directory: %w", err)
	}

	gitDir := strings.TrimSpace(string(output))
	// Make gitDir absolute if it's relative
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktreeDir, gitDir)
	}

	excludePath := filepath.Join(gitDir, "info", "exclude")

	// Create info directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(excludePath), 0755); err != nil { //nolint:gosec // G301: standard directory permissions
		return fmt.Errorf("failed to create info directory: %w", err)
	}

	// Read existing content
	content, err := os.ReadFile(excludePath) //nolint:gosec // G304: excludePath is constructed from gitDir
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read exclude file: %w", err)
	}

	existingContent := string(content)

	// Check if pattern already exists
	for _, line := range strings.Split(existingContent, "\n") {
		if strings.TrimSpace(line) == pattern {
			// Pattern already exists, no need to add
			return nil
		}
	}

	// Append pattern
	newContent := existingContent
	if len(newContent) > 0 && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += pattern + "\n"

	// Write back
	if err := os.WriteFile(excludePath, []byte(newContent), 0644); err != nil { //nolint:gosec // G306: git exclude file needs standard permissions
		return fmt.Errorf("failed to write exclude file: %w", err)
	}

	return nil
}
