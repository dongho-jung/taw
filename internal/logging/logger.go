// Package logging provides unified logging functionality for PAW.
//
// Log Levels (0-5):
//   - L0 (Trace):   Most detailed internal state tracking, loop iterations, variable dumps
//   - L1 (Debug):   Debugging info, retry attempts, state changes (only when PAW_DEBUG=1)
//   - L2 (Info):    Normal operation flow, task lifecycle events
//   - L3 (Warn):    Non-fatal issues requiring attention
//   - L4 (Error):   Errors that may affect functionality
//   - L5 (Fatal):   Critical errors that require immediate attention
package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Level represents a log level.
type Level int

const (
	LevelTrace Level = iota // L0: Most detailed tracing
	LevelDebug              // L1: Debug information
	LevelInfo               // L2: Normal operation info
	LevelWarn               // L3: Warnings
	LevelError              // L4: Errors
	LevelFatal              // L5: Fatal errors
)

// levelStrings contains pre-computed level strings to avoid fmt.Sprintf on each log line.
var levelStrings = [6]string{"L0", "L1", "L2", "L3", "L4", "L5"}

// String returns the level string for log output.
func (l Level) String() string {
	if l >= 0 && l <= 5 {
		return levelStrings[l]
	}
	return "L?"
}

// Name returns the human-readable level name.
func (l Level) Name() string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogFormat defines the log output format.
type LogFormat string

const (
	LogFormatText  LogFormat = "text"
	LogFormatJSONL LogFormat = "jsonl"
)

// Options configures logging behavior.
type Options struct {
	Format     LogFormat
	MaxSizeMB  int
	MaxBackups int
}

// Logger provides logging capabilities for PAW.
type Logger interface {
	// Trace outputs the most detailed tracing information (L0)
	// Use for: loop iterations, variable dumps, internal state tracking
	Trace(format string, args ...interface{})

	// Debug outputs debug information (L1, only when PAW_DEBUG=1)
	// Use for: retry attempts, state changes, conditional logic paths
	Debug(format string, args ...interface{})

	// Log writes informational message to log file (L2)
	// Use for: normal operation flow, task lifecycle events
	Log(format string, args ...interface{})

	// Info is an alias for Log (L2)
	Info(format string, args ...interface{})

	// Warn outputs warning to stderr and log file (L3)
	// Use for: non-fatal issues, recoverable errors
	Warn(format string, args ...interface{})

	// Error outputs error to stderr and log file (L4)
	// Use for: errors that affect functionality
	Error(format string, args ...interface{})

	// Fatal outputs fatal error to stderr and log file (L5)
	// Use for: critical errors requiring immediate attention
	Fatal(format string, args ...interface{})

	// SetScript sets the current script name for context
	SetScript(script string)

	// SetTask sets the current task name for context
	SetTask(task string)

	// StartTimer starts a timer for measuring operation duration
	StartTimer(operation string) *Timer

	// Close closes the log file
	Close() error
}

// Timer represents a timer for measuring operation duration
type Timer struct {
	operation string
	start     time.Time
	logger    *fileLogger
}

// Stop stops the timer and logs the elapsed time
func (t *Timer) Stop() time.Duration {
	elapsed := time.Since(t.start)
	if t.logger != nil {
		t.logger.logWithLevel(LevelDebug, "%s completed in %v", t.operation, elapsed)
	}
	return elapsed
}

// StopWithResult stops the timer and logs the result
func (t *Timer) StopWithResult(success bool, detail string) time.Duration {
	elapsed := time.Since(t.start)
	if t.logger != nil {
		status := "completed"
		level := LevelDebug
		if !success {
			status = "failed"
			level = LevelWarn
		}
		if detail != "" {
			t.logger.logWithLevel(level, "%s %s in %v: %s", t.operation, status, elapsed, detail)
		} else {
			t.logger.logWithLevel(level, "%s %s in %v", t.operation, status, elapsed)
		}
	}
	return elapsed
}

type fileLogger struct {
	file   *os.File
	script string
	task   string
	debug  bool
	format LogFormat
	mu     sync.Mutex
}

// New creates a new Logger that writes to the specified file.
func New(logPath string, debug bool) (Logger, error) {
	opts := optionsFromEnv()
	if err := rotateIfNeeded(logPath, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to rotate log file: %v\n", err)
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &fileLogger{
		file:   file,
		debug:  debug,
		format: opts.Format,
	}, nil
}

// NewStdout creates a logger that only outputs to stdout/stderr.
func NewStdout(debug bool) Logger {
	return &fileLogger{
		debug:  debug,
		format: LogFormatText,
	}
}

func (l *fileLogger) SetScript(script string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.script = script
}

func (l *fileLogger) SetTask(task string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.task = task
}

func (l *fileLogger) getContext() string {
	// Note: caller should hold the lock when calling this method
	if l.task != "" {
		return fmt.Sprintf("%s:%s", l.script, l.task)
	}
	return l.script
}

func isLoggingFrame(fn string) bool {
	return strings.Contains(fn, "internal/logging.")
}

type logEntry struct {
	Timestamp string `json:"ts"`
	Level     string `json:"level"`
	LevelName string `json:"level_name,omitempty"`
	Script    string `json:"script,omitempty"`
	Task      string `json:"task,omitempty"`
	Context   string `json:"context,omitempty"`
	Caller    string `json:"caller,omitempty"`
	Message   string `json:"msg"`
}

// callerPCsPool reuses uintptr slices to reduce allocations in getCaller.
var callerPCsPool = sync.Pool{
	New: func() interface{} {
		pcs := make([]uintptr, 16)
		return &pcs
	},
}

// getCaller returns the caller function name (skipping internal logging frames).
func getCaller() string {
	pcsPtr := callerPCsPool.Get().(*[]uintptr)
	pcs := *pcsPtr
	defer callerPCsPool.Put(pcsPtr)

	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()
		if frame.Function == "" {
			break
		}
		if !isLoggingFrame(frame.Function) {
			name := frame.Function
			// Extract just the function name from the full path
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			// Shorten the package path
			if idx := strings.Index(name, "."); idx >= 0 {
				name = name[idx+1:]
			}
			return name
		}
		if !more {
			break
		}
	}
	return "unknown"
}

func optionsFromEnv() Options {
	opts := Options{
		Format:     LogFormatText,
		MaxSizeMB:  10,
		MaxBackups: 3,
	}

	if value := strings.TrimSpace(os.Getenv("PAW_LOG_FORMAT")); value != "" {
		switch strings.ToLower(value) {
		case string(LogFormatJSONL):
			opts.Format = LogFormatJSONL
		default:
			opts.Format = LogFormatText
		}
	}

	if value := strings.TrimSpace(os.Getenv("PAW_LOG_MAX_SIZE_MB")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			opts.MaxSizeMB = parsed
		}
	}

	if value := strings.TrimSpace(os.Getenv("PAW_LOG_MAX_BACKUPS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
			opts.MaxBackups = parsed
		}
	}

	return opts
}

func rotateIfNeeded(logPath string, opts Options) error {
	if opts.MaxSizeMB <= 0 || opts.MaxBackups <= 0 {
		return nil
	}

	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	maxBytes := int64(opts.MaxSizeMB) * 1024 * 1024
	if info.Size() < maxBytes {
		return nil
	}

	for i := opts.MaxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}

	base := filepath.Base(logPath)
	dir := filepath.Dir(logPath)
	rotated := filepath.Join(dir, base+".1")
	return os.Rename(logPath, rotated)
}

// logWithLevel writes a log entry with the specified level
func (l *fileLogger) logWithLevel(level Level, format string, args ...interface{}) {
	// Debug level only logged when debug mode is enabled
	if (level == LevelDebug || level == LevelTrace) && !l.debug {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	msg := fmt.Sprintf(format, args...)
	context := l.getContext()
	caller := getCaller()

	if l.format == LogFormatJSONL {
		entry := logEntry{
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Level:     level.String(),
			LevelName: level.Name(),
			Script:    l.script,
			Task:      l.task,
			Context:   context,
			Caller:    caller,
			Message:   msg,
		}
		data, err := json.Marshal(entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal log entry: %v\n", err)
			return
		}
		if _, err := l.file.Write(append(data, '\n')); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write to log file: %v\n", err)
		}
		return
	}

	timestamp := time.Now().Format("06-01-02 15:04:05.0")
	// Format: [timestamp] [level] [context] [caller] message
	line := fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n", timestamp, level.String(), context, caller, msg)
	if _, err := l.file.WriteString(line); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write to log file: %v\n", err)
	}
}

func (l *fileLogger) Trace(format string, args ...interface{}) {
	if !l.debug {
		return
	}
	l.logWithLevel(LevelTrace, format, args...)
}

func (l *fileLogger) Debug(format string, args ...interface{}) {
	if !l.debug {
		return
	}

	msg := fmt.Sprintf(format, args...)
	// Only write to log file, not stderr - stderr output interferes with TUI
	l.logWithLevel(LevelDebug, "%s", msg)
}

func (l *fileLogger) Log(format string, args ...interface{}) {
	l.logWithLevel(LevelInfo, format, args...)
}

func (l *fileLogger) Info(format string, args ...interface{}) {
	l.logWithLevel(LevelInfo, format, args...)
}

func (l *fileLogger) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)

	l.logWithLevel(LevelWarn, "%s", msg)
}

func (l *fileLogger) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)

	l.logWithLevel(LevelError, "%s", msg)
}

func (l *fileLogger) Fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "FATAL: %s\n", msg)

	l.logWithLevel(LevelFatal, "%s", msg)
}

func (l *fileLogger) StartTimer(operation string) *Timer {
	// Log start only if file is available (at Debug level)
	if l.file != nil {
		l.logWithLevel(LevelDebug, "%s started", operation)
	}
	return &Timer{
		operation: operation,
		start:     time.Now(),
		logger:    l,
	}
}

func (l *fileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Global logger instance
var globalLogger Logger = NewStdout(os.Getenv("PAW_DEBUG") == "1")

// SetGlobal sets the global logger instance.
func SetGlobal(l Logger) {
	globalLogger = l
}

// Global returns the global logger instance.
func Global() Logger {
	return globalLogger
}

// Trace logs trace information using the global logger.
func Trace(format string, args ...interface{}) {
	globalLogger.Trace(format, args...)
}

// Debug logs debug information using the global logger.
func Debug(format string, args ...interface{}) {
	globalLogger.Debug(format, args...)
}

// Log logs information using the global logger.
func Log(format string, args ...interface{}) {
	globalLogger.Log(format, args...)
}

// Info logs informational message using the global logger.
func Info(format string, args ...interface{}) {
	globalLogger.Info(format, args...)
}

// Warn logs a warning using the global logger.
func Warn(format string, args ...interface{}) {
	globalLogger.Warn(format, args...)
}

// Error logs an error using the global logger.
func Error(format string, args ...interface{}) {
	globalLogger.Error(format, args...)
}

// Fatal logs a fatal error using the global logger.
func Fatal(format string, args ...interface{}) {
	globalLogger.Fatal(format, args...)
}

// StartTimer starts a timer for measuring operation duration using the global logger.
func StartTimer(operation string) *Timer {
	return globalLogger.StartTimer(operation)
}
