package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelTrace, "L0"},
		{LevelDebug, "L1"},
		{LevelInfo, "L2"},
		{LevelWarn, "L3"},
		{LevelError, "L4"},
		{LevelFatal, "L5"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.want {
				t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestLevelName(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelTrace, "TRACE"},
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.level.Name()
			if got != tt.want {
				t.Errorf("Level(%d).Name() = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestNewStdout(t *testing.T) {
	logger := NewStdout(false)
	if logger == nil {
		t.Fatal("NewStdout returned nil")
	}

	// Should not panic
	logger.Log("test message")
	logger.Info("test info")
	logger.SetScript("test-script")
	logger.SetTask("test-task")
}

func TestNewWithFile(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	logger, err := New(logPath, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	logger.SetScript("test-script")
	logger.SetTask("test-task")
	logger.Log("test message %d", 123)

	// Read log file
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "[L2]") {
		t.Errorf("Log file should contain [L2], got: %s", content)
	}
	if !strings.Contains(content, "test message 123") {
		t.Errorf("Log file should contain message, got: %s", content)
	}
	if !strings.Contains(content, "test-script:test-task") {
		t.Errorf("Log file should contain context, got: %s", content)
	}
}

func TestLoggerDebugMode(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	// Test with debug mode off
	logger, err := New(logPath, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Debug("debug message")
	logger.Trace("trace message")
	logger.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "debug message") {
		t.Errorf("Debug messages should not be logged when debug is off")
	}
	if strings.Contains(content, "trace message") {
		t.Errorf("Trace messages should not be logged when debug is off")
	}

	// Test with debug mode on
	logPath2 := filepath.Join(tempDir, "test2.log")
	logger2, err := New(logPath2, true)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger2.Debug("debug message on")
	logger2.Trace("trace message on")
	logger2.Close()

	data2, err := os.ReadFile(logPath2)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content2 := string(data2)
	if !strings.Contains(content2, "debug message on") {
		t.Errorf("Debug messages should be logged when debug is on, got: %s", content2)
	}
	if !strings.Contains(content2, "trace message on") {
		t.Errorf("Trace messages should be logged when debug is on, got: %s", content2)
	}
}

func TestLoggerWarnError(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	logger, err := New(logPath, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Warn("warning message")
	logger.Error("error message")
	logger.Fatal("fatal message")
	logger.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "[L3]") {
		t.Errorf("Log file should contain [L3] for warnings, got: %s", content)
	}
	if !strings.Contains(content, "[L4]") {
		t.Errorf("Log file should contain [L4] for errors, got: %s", content)
	}
	if !strings.Contains(content, "[L5]") {
		t.Errorf("Log file should contain [L5] for fatal, got: %s", content)
	}
}

func TestTimer(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	logger, err := New(logPath, true)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	timer := logger.StartTimer("test operation")
	time.Sleep(10 * time.Millisecond)
	elapsed := timer.Stop()

	if elapsed < 10*time.Millisecond {
		t.Errorf("Timer elapsed = %v, want >= 10ms", elapsed)
	}

	// Read log file
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "test operation") {
		t.Errorf("Log file should contain timer operation name, got: %s", content)
	}
}

func TestTimerWithResult(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	logger, err := New(logPath, true)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	// Test success
	timer1 := logger.StartTimer("success operation")
	timer1.StopWithResult(true, "completed successfully")

	// Test failure
	timer2 := logger.StartTimer("failure operation")
	timer2.StopWithResult(false, "something went wrong")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "completed") {
		t.Errorf("Log file should contain 'completed', got: %s", content)
	}
	if !strings.Contains(content, "failed") {
		t.Errorf("Log file should contain 'failed', got: %s", content)
	}
}

func TestGlobalLogger(t *testing.T) {
	// Save original global logger
	original := Global()
	defer SetGlobal(original)

	// Create and set a new logger
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "global.log")

	logger, err := New(logPath, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	SetGlobal(logger)

	// Use global functions
	Log("global log message")
	Info("global info message")
	Warn("global warn message")
	Error("global error message")

	// Verify global logger was used
	if Global() != logger {
		t.Error("Global() should return the set logger")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "global log message") {
		t.Errorf("Log file should contain global log message, got: %s", content)
	}
}

func TestNewWithInvalidPath(t *testing.T) {
	_, err := New("/nonexistent/path/to/log/file.log", false)
	if err == nil {
		t.Error("New() should return error for invalid path")
	}
}

func TestLoggerContext(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	logger, err := New(logPath, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	// Test with only script
	logger.SetScript("my-script")
	logger.Log("message with script only")

	// Test with script and task
	logger.SetTask("my-task")
	logger.Log("message with script and task")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "my-script") {
		t.Errorf("Log file should contain script name, got: %s", content)
	}
	if !strings.Contains(content, "my-script:my-task") {
		t.Errorf("Log file should contain script:task context, got: %s", content)
	}
}
