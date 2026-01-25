package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGitCmd runs a git command with a clean environment to avoid parent repo influence.
func runGitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Filter out git environment variables to ensure isolation from parent repository
	var cleanEnv []string
	gitVars := map[string]bool{
		"GIT_DIR":                          true,
		"GIT_WORK_TREE":                    true,
		"GIT_INDEX_FILE":                   true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_CEILING_DIRECTORIES":          true,
	}
	for _, env := range os.Environ() {
		// Keep all non-git environment variables
		keep := true
		for gitVar := range gitVars {
			if len(env) >= len(gitVar) && env[:len(gitVar)] == gitVar && (len(env) == len(gitVar) || env[len(gitVar)] == '=') {
				keep = false
				break
			}
		}
		if keep {
			cleanEnv = append(cleanEnv, env)
		}
	}
	cmd.Env = cleanEnv
	return cmd
}

// setupGitRepo creates a temporary git repository for testing.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repo with clean environment (no parent repo influence)
	output, err := runGitCmd(dir, "init").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to init git repo: %v\nOutput: %s", err, output)
	}

	// Configure git user (required for commits)
	if err := runGitCmd(dir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("Failed to config user.name: %v", err)
	}
	if err := runGitCmd(dir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("Failed to config user.email: %v", err)
	}

	// Disable git hooks to avoid triggering pre-commit hooks during tests
	if err := runGitCmd(dir, "config", "core.hooksPath", "/dev/null").Run(); err != nil {
		t.Fatalf("Failed to config core.hooksPath: %v", err)
	}

	return dir
}

// createCommit creates a file and commits it to the repository.
func createCommit(t *testing.T, dir, filename, content, message string) {
	t.Helper()

	// Create file
	filePath := filepath.Join(dir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Add file
	if err := runGitCmd(dir, "add", filename).Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	// Commit
	output, err := runGitCmd(dir, "commit", "-m", message).CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to commit: %v\nOutput: %s", err, output)
	}
}

func TestNew(t *testing.T) {
	client := New()
	if client == nil {
		t.Fatal("New() returned nil")
	}
}

func TestCopyUntrackedFiles(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	dstDir := filepath.Join(tempDir, "dst")

	// Create source directory structure
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	// Create source files
	files := map[string]string{
		"file1.txt":        "content 1",
		"file2.txt":        "content 2",
		"subdir/file3.txt": "content 3",
	}

	for path, content := range files {
		fullPath := filepath.Join(srcDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir for %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", path, err)
		}
	}

	// Copy files
	fileList := []string{"file1.txt", "file2.txt", "subdir/file3.txt"}
	if err := CopyUntrackedFiles(fileList, srcDir, dstDir); err != nil {
		t.Fatalf("CopyUntrackedFiles() error = %v", err)
	}

	// Verify copied files
	for path, wantContent := range files {
		dstPath := filepath.Join(dstDir, path)
		data, err := os.ReadFile(dstPath)
		if err != nil {
			t.Errorf("Failed to read copied file %s: %v", path, err)
			continue
		}
		if string(data) != wantContent {
			t.Errorf("Copied file %s content = %q, want %q", path, string(data), wantContent)
		}
	}
}

func TestCopyUntrackedFilesPreservesMode(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	dstDir := filepath.Join(tempDir, "dst")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	// Create executable file
	srcPath := filepath.Join(srcDir, "script.sh")
	if err := os.WriteFile(srcPath, []byte("#!/bin/bash\necho hello"), 0755); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	// Copy file
	if err := CopyUntrackedFiles([]string{"script.sh"}, srcDir, dstDir); err != nil {
		t.Fatalf("CopyUntrackedFiles() error = %v", err)
	}

	// Verify mode
	dstPath := filepath.Join(dstDir, "script.sh")
	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Failed to stat copied file: %v", err)
	}

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("Failed to stat source file: %v", err)
	}

	if info.Mode() != srcInfo.Mode() {
		t.Errorf("Copied file mode = %v, want %v", info.Mode(), srcInfo.Mode())
	}
}

func TestCopyUntrackedFilesEmptyList(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	dstDir := filepath.Join(tempDir, "dst")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	// Copy empty list
	if err := CopyUntrackedFiles([]string{}, srcDir, dstDir); err != nil {
		t.Fatalf("CopyUntrackedFiles() with empty list error = %v", err)
	}
}

func TestCopyUntrackedFilesNonexistentSource(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	dstDir := filepath.Join(tempDir, "dst")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	// Try to copy non-existent file
	err := CopyUntrackedFiles([]string{"nonexistent.txt"}, srcDir, dstDir)
	if err == nil {
		t.Error("CopyUntrackedFiles() should return error for non-existent source")
	}
}

func TestCopyUntrackedFilesCreatesDirectories(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	dstDir := filepath.Join(tempDir, "dst")

	// Create source file in nested directory
	nestedDir := filepath.Join(srcDir, "a", "b", "c")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Create empty dst dir (without nested structure)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	// Copy file - should create nested directories
	if err := CopyUntrackedFiles([]string{"a/b/c/file.txt"}, srcDir, dstDir); err != nil {
		t.Fatalf("CopyUntrackedFiles() error = %v", err)
	}

	// Verify file exists
	dstPath := filepath.Join(dstDir, "a", "b", "c", "file.txt")
	if _, err := os.Stat(dstPath); err != nil {
		t.Errorf("Copied file not found: %v", err)
	}
}

func TestGenerateMergeCommitMessage(t *testing.T) {
	tests := []struct {
		name            string
		taskName        string
		commits         []CommitInfo
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:     "fix task with no commits",
			taskName: "fix-kanban-drag-select",
			commits:  nil,
			wantContains: []string{
				"fix: kanban drag select",
			},
			wantNotContains: []string{
				"Changes:",
			},
		},
		{
			name:     "fix task with commits",
			taskName: "fix-login-bug",
			commits: []CommitInfo{
				{Hash: "abc123", Subject: "Fix null pointer exception"},
				{Hash: "def456", Subject: "Add error handling"},
			},
			wantContains: []string{
				"fix: login bug",
				"Changes:",
				"- Fix null pointer exception",
				"- Add error handling",
			},
		},
		{
			name:     "feature task",
			taskName: "add-dark-mode",
			commits: []CommitInfo{
				{Hash: "abc123", Subject: "Implement dark mode toggle"},
			},
			wantContains: []string{
				"feat: dark mode",
				"Changes:",
				"- Implement dark mode toggle",
			},
		},
		{
			name:     "refactor task with improve keyword",
			taskName: "improve-commit-messages",
			commits: []CommitInfo{
				{Hash: "abc123", Subject: "Add commit type inference"},
				{Hash: "def456", Subject: "Add commit body generation"},
			},
			wantContains: []string{
				"refactor: improve commit messages",
				"Changes:",
				"- Add commit type inference",
				"- Add commit body generation",
			},
		},
		{
			name:     "docs task",
			taskName: "docs-update-readme",
			commits: []CommitInfo{
				{Hash: "abc123", Subject: "Update installation instructions"},
			},
			wantContains: []string{
				"docs: update readme",
				"Changes:",
				"- Update installation instructions",
			},
		},
		{
			name:     "long commit subject truncated",
			taskName: "fix-bug",
			commits: []CommitInfo{
				{Hash: "abc123", Subject: "This is a very long commit message that exceeds the seventy-two character limit and should be truncated"},
			},
			wantContains: []string{
				"fix: bug",
				"...",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateMergeCommitMessage(tt.taskName, tt.commits)

			for _, want := range tt.wantContains {
				if !contains(result, want) {
					t.Errorf("GenerateMergeCommitMessage() result does not contain %q\nGot:\n%s", want, result)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if contains(result, notWant) {
					t.Errorf("GenerateMergeCommitMessage() result should not contain %q\nGot:\n%s", notWant, result)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Git client operation tests

func TestIsGitRepo(t *testing.T) {
	client := New()

	// Test with git repo
	gitDir := setupGitRepo(t)
	if !client.IsGitRepo(gitDir) {
		t.Error("IsGitRepo() = false for git repository, want true")
	}

	// Test with non-git directory
	nonGitDir := t.TempDir()
	if client.IsGitRepo(nonGitDir) {
		t.Error("IsGitRepo() = true for non-git directory, want false")
	}
}

func TestGetRepoRoot(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	root, err := client.GetRepoRoot(gitDir)
	if err != nil {
		t.Fatalf("GetRepoRoot() error = %v", err)
	}

	if root == "" {
		t.Error("GetRepoRoot() returned empty string")
	}
}

func TestGetMainBranch(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit (needed for branch to exist)
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	mainBranch := client.GetMainBranch(gitDir)
	if mainBranch == "" {
		t.Error("GetMainBranch() returned empty string")
	}
}

func TestBranchOperations(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Test BranchExists - should not exist yet
	if client.BranchExists(gitDir, "feature") {
		t.Error("BranchExists('feature') = true before creation, want false")
	}

	// Test BranchCreate
	if err := client.BranchCreate(gitDir, "feature", ""); err != nil {
		t.Fatalf("BranchCreate() error = %v", err)
	}

	// Test BranchExists - should exist now
	if !client.BranchExists(gitDir, "feature") {
		t.Error("BranchExists('feature') = false after creation, want true")
	}

	// Test GetCurrentBranch
	currentBranch, err := client.GetCurrentBranch(gitDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch() error = %v", err)
	}
	if currentBranch == "" {
		t.Error("GetCurrentBranch() returned empty string")
	}

	// Test BranchDelete
	if err := client.BranchDelete(gitDir, "feature", false); err != nil {
		t.Fatalf("BranchDelete() error = %v", err)
	}

	// Verify deletion
	if client.BranchExists(gitDir, "feature") {
		t.Error("BranchExists('feature') = true after deletion, want false")
	}
}

func TestGetHeadCommit(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	hash, err := client.GetHeadCommit(gitDir)
	if err != nil {
		t.Fatalf("GetHeadCommit() error = %v", err)
	}

	if len(hash) != 40 { // Git SHA-1 hash length
		t.Errorf("GetHeadCommit() returned hash with length %d, want 40", len(hash))
	}
}

func TestHasChanges(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// No changes initially
	if client.HasChanges(gitDir) {
		t.Error("HasChanges() = true with no changes, want false")
	}

	// Create a new file (untracked)
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)

	// Should have changes now
	if !client.HasChanges(gitDir) {
		t.Error("HasChanges() = false with untracked file, want true")
	}
}

func TestHasStagedChanges(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// No staged changes initially
	if client.HasStagedChanges(gitDir) {
		t.Error("HasStagedChanges() = true with no staged changes, want false")
	}

	// Create and stage a file
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)
	_ = runGitCmd(gitDir, "add", "new.txt").Run()

	// Should have staged changes now
	if !client.HasStagedChanges(gitDir) {
		t.Error("HasStagedChanges() = false with staged file, want true")
	}
}

func TestHasUntrackedFiles(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// No untracked files initially
	if client.HasUntrackedFiles(gitDir) {
		t.Error("HasUntrackedFiles() = true with no untracked files, want false")
	}

	// Create a new file
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)

	// Should have untracked files now
	if !client.HasUntrackedFiles(gitDir) {
		t.Error("HasUntrackedFiles() = false with untracked file, want true")
	}
}

func TestGetUntrackedFiles(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create untracked files
	_ = os.WriteFile(filepath.Join(gitDir, "file1.txt"), []byte("content"), 0644)
	_ = os.WriteFile(filepath.Join(gitDir, "file2.txt"), []byte("content"), 0644)

	files, err := client.GetUntrackedFiles(gitDir)
	if err != nil {
		t.Fatalf("GetUntrackedFiles() error = %v", err)
	}

	if len(files) != 2 {
		t.Errorf("GetUntrackedFiles() returned %d files, want 2", len(files))
	}
}

func TestAddAndCommit(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create a new file
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)

	// Test Add
	if err := client.Add(gitDir, "new.txt"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Test Commit
	if err := client.Commit(gitDir, "Add new file"); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	// Verify commit was created
	hash, err := client.GetHeadCommit(gitDir)
	if err != nil {
		t.Fatalf("GetHeadCommit() error = %v", err)
	}
	if hash == "" {
		t.Error("No commit hash after commit")
	}
}

func TestAddAll(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create multiple new files
	_ = os.WriteFile(filepath.Join(gitDir, "file1.txt"), []byte("content1"), 0644)
	_ = os.WriteFile(filepath.Join(gitDir, "file2.txt"), []byte("content2"), 0644)

	// Test AddAll
	if err := client.AddAll(gitDir); err != nil {
		t.Fatalf("AddAll() error = %v", err)
	}

	// Verify files are staged
	if !client.HasStagedChanges(gitDir) {
		t.Error("HasStagedChanges() = false after AddAll, want true")
	}
}

func TestGetDiffStat(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create and stage a file
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content\n"), 0644)
	_ = client.Add(gitDir, "new.txt")

	// Test GetDiffStat
	stat, err := client.GetDiffStat(gitDir)
	if err != nil {
		t.Fatalf("GetDiffStat() error = %v", err)
	}

	if stat == "" {
		t.Error("GetDiffStat() returned empty string")
	}
}

func TestStatus(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create a new file
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)

	// Test Status
	status, err := client.Status(gitDir)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if status == "" {
		t.Error("Status() returned empty string with untracked file")
	}
}

func TestCheckout(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create and switch to a new branch
	_ = client.BranchCreate(gitDir, "feature", "")

	// Test Checkout
	if err := client.Checkout(gitDir, "feature"); err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	// Verify we're on the feature branch
	currentBranch, _ := client.GetCurrentBranch(gitDir)
	if currentBranch != "feature" {
		t.Errorf("Current branch = %q after checkout, want %q", currentBranch, "feature")
	}
}

func TestBranchMerged(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit on main
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	mainBranch, _ := client.GetCurrentBranch(gitDir)

	// Create feature branch
	_ = client.BranchCreate(gitDir, "feature", "")
	_ = client.Checkout(gitDir, "feature")
	createCommit(t, gitDir, "feature.txt", "feature content", "Add feature")

	// Switch back to main and merge
	_ = client.Checkout(gitDir, mainBranch)
	_ = runGitCmd(gitDir, "merge", "feature", "--no-edit").Run()

	// Test BranchMerged
	if !client.BranchMerged(gitDir, "feature", mainBranch) {
		t.Error("BranchMerged() = false for merged branch, want true")
	}
}

func TestGetBranchCommits(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit on main
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	mainBranch, _ := client.GetCurrentBranch(gitDir)

	// Create feature branch and add commits
	_ = client.BranchCreate(gitDir, "feature", "")
	_ = client.Checkout(gitDir, "feature")
	createCommit(t, gitDir, "feature1.txt", "content1", "Feature commit 1")
	createCommit(t, gitDir, "feature2.txt", "content2", "Feature commit 2")

	// Test GetBranchCommits
	commits, err := client.GetBranchCommits(gitDir, "feature", mainBranch, 0)
	if err != nil {
		t.Fatalf("GetBranchCommits() error = %v", err)
	}

	if len(commits) != 2 {
		t.Errorf("GetBranchCommits() returned %d commits, want 2", len(commits))
	}

	// Check commit subjects
	if commits[0].Subject != "Feature commit 2" {
		t.Errorf("Commit 0 subject = %q, want %q", commits[0].Subject, "Feature commit 2")
	}
	if commits[1].Subject != "Feature commit 1" {
		t.Errorf("Commit 1 subject = %q, want %q", commits[1].Subject, "Feature commit 1")
	}
}

func TestHasOngoingMerge(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// No ongoing merge initially
	if client.HasOngoingMerge(gitDir) {
		t.Error("HasOngoingMerge() = true with no merge, want false")
	}
}

func TestHasOngoingRebase(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// No ongoing rebase initially
	if client.HasOngoingRebase(gitDir) {
		t.Error("HasOngoingRebase() = true with no rebase, want false")
	}
}

func TestIsFileStaged(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create a new file
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)

	// File should not be staged initially
	staged, err := client.IsFileStaged(gitDir, "new.txt")
	if err != nil {
		t.Fatalf("IsFileStaged() error = %v", err)
	}
	if staged {
		t.Error("IsFileStaged() = true for unstaged file, want false")
	}

	// Stage the file
	_ = client.Add(gitDir, "new.txt")

	// File should be staged now
	staged, err = client.IsFileStaged(gitDir, "new.txt")
	if err != nil {
		t.Fatalf("IsFileStaged() error = %v", err)
	}
	if !staged {
		t.Error("IsFileStaged() = false for staged file, want true")
	}
}

func TestResetPath(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create and stage a file
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)
	_ = client.Add(gitDir, "new.txt")

	// Verify file is staged
	staged, _ := client.IsFileStaged(gitDir, "new.txt")
	if !staged {
		t.Fatal("Setup failed: file should be staged")
	}

	// Test ResetPath
	if err := client.ResetPath(gitDir, "new.txt"); err != nil {
		t.Fatalf("ResetPath() error = %v", err)
	}

	// Verify file is no longer staged
	staged, _ = client.IsFileStaged(gitDir, "new.txt")
	if staged {
		t.Error("IsFileStaged() = true after reset, want false")
	}
}

func TestIsFileTracked(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit with a tracked file
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// README.md should be tracked
	if !client.IsFileTracked(gitDir, "README.md") {
		t.Error("IsFileTracked() = false for tracked file, want true")
	}

	// Create an untracked file
	_ = os.WriteFile(filepath.Join(gitDir, "untracked.txt"), []byte("content"), 0644)

	// untracked.txt should not be tracked
	if client.IsFileTracked(gitDir, "untracked.txt") {
		t.Error("IsFileTracked() = true for untracked file, want false")
	}

	// Non-existent file should not be tracked
	if client.IsFileTracked(gitDir, "nonexistent.txt") {
		t.Error("IsFileTracked() = true for non-existent file, want false")
	}
}

func TestStashList(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// No stashes initially
	entries, err := client.StashList(gitDir)
	if err != nil {
		t.Fatalf("StashList() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("StashList() returned %d entries, want 0", len(entries))
	}

	// Create a file and stash it
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)
	if err := client.StashPush(gitDir, "test-stash-message"); err != nil {
		t.Fatalf("StashPush() error = %v", err)
	}

	// Should have one stash now
	entries, err = client.StashList(gitDir)
	if err != nil {
		t.Fatalf("StashList() error = %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("StashList() returned %d entries, want 1", len(entries))
	}
	if entries[0].Index != 0 {
		t.Errorf("StashEntry.Index = %d, want 0", entries[0].Index)
	}
	if !contains(entries[0].Message, "test-stash-message") {
		t.Errorf("StashEntry.Message = %q, want to contain 'test-stash-message'", entries[0].Message)
	}
}

func TestStashPopByMessage(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create a file and stash it with a specific message
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)
	if err := client.StashPush(gitDir, "paw-merge-temp"); err != nil {
		t.Fatalf("StashPush() error = %v", err)
	}

	// Verify stash was created
	entries, _ := client.StashList(gitDir)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 stash entry, got %d", len(entries))
	}

	// Pop by message
	if err := client.StashPopByMessage(gitDir, "paw-merge-temp"); err != nil {
		t.Fatalf("StashPopByMessage() error = %v", err)
	}

	// Stash should be gone
	entries, _ = client.StashList(gitDir)
	if len(entries) != 0 {
		t.Errorf("StashList() after pop returned %d entries, want 0", len(entries))
	}

	// File should be restored
	if !client.HasChanges(gitDir) {
		t.Error("HasChanges() = false after StashPopByMessage, want true (file should be restored)")
	}
}

func TestStashPopByMessageWithMultipleStashes(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create first stash (the one we want to pop)
	_ = os.WriteFile(filepath.Join(gitDir, "first.txt"), []byte("first"), 0644)
	if err := client.StashPush(gitDir, "paw-merge-temp"); err != nil {
		t.Fatalf("StashPush() error = %v", err)
	}

	// Create second stash (a different one)
	_ = os.WriteFile(filepath.Join(gitDir, "second.txt"), []byte("second"), 0644)
	if err := client.StashPush(gitDir, "other-stash"); err != nil {
		t.Fatalf("StashPush() error = %v", err)
	}

	// Verify both stashes exist
	entries, _ := client.StashList(gitDir)
	if len(entries) != 2 {
		t.Fatalf("Expected 2 stash entries, got %d", len(entries))
	}

	// Pop by specific message (should pop the older one, not the recent one)
	if err := client.StashPopByMessage(gitDir, "paw-merge-temp"); err != nil {
		t.Fatalf("StashPopByMessage() error = %v", err)
	}

	// Only one stash should remain
	entries, _ = client.StashList(gitDir)
	if len(entries) != 1 {
		t.Errorf("StashList() after pop returned %d entries, want 1", len(entries))
	}

	// The remaining stash should be "other-stash"
	if !contains(entries[0].Message, "other-stash") {
		t.Errorf("Remaining stash message = %q, want to contain 'other-stash'", entries[0].Message)
	}
}

func TestStashPopByMessageNotFound(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Pop a non-existent stash should not error (no-op)
	if err := client.StashPopByMessage(gitDir, "non-existent-stash"); err != nil {
		t.Errorf("StashPopByMessage() error = %v, want nil for non-existent stash", err)
	}
}

func TestStashDropByMessage(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Create a file and stash it
	_ = os.WriteFile(filepath.Join(gitDir, "new.txt"), []byte("content"), 0644)
	if err := client.StashPush(gitDir, "paw-merge-temp"); err != nil {
		t.Fatalf("StashPush() error = %v", err)
	}

	// Verify stash was created
	entries, _ := client.StashList(gitDir)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 stash entry, got %d", len(entries))
	}

	// Drop by message
	if err := client.StashDropByMessage(gitDir, "paw-merge-temp"); err != nil {
		t.Fatalf("StashDropByMessage() error = %v", err)
	}

	// Stash should be gone
	entries, _ = client.StashList(gitDir)
	if len(entries) != 0 {
		t.Errorf("StashList() after drop returned %d entries, want 0", len(entries))
	}

	// File should NOT be restored (drop doesn't apply)
	if client.HasChanges(gitDir) {
		t.Error("HasChanges() = true after StashDropByMessage, want false (drop should not apply changes)")
	}
}

func TestStashDropByMessageNotFound(t *testing.T) {
	client := New()
	gitDir := setupGitRepo(t)

	// Create initial commit
	createCommit(t, gitDir, "README.md", "test", "Initial commit")

	// Drop a non-existent stash should not error (no-op)
	if err := client.StashDropByMessage(gitDir, "non-existent-stash"); err != nil {
		t.Errorf("StashDropByMessage() error = %v, want nil for non-existent stash", err)
	}
}
