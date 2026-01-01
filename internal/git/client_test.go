package git

import (
	"os"
	"path/filepath"
	"testing"
)

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
