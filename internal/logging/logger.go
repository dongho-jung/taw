// Package logging provides unified logging functionality for TAW.
package logging

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Logger provides logging capabilities for TAW.
type Logger interface {
	// Debug outputs debug information (only when TAW_DEBUG=1)
	Debug(format string, args ...interface{})

	// Log writes to the unified log file with timestamp
	Log(format string, args ...interface{})

	// Info writes informational message to log file
	Info(format string, args ...interface{})

	// Warn outputs warning to stderr and log file
	Warn(format string, args ...interface{})

	// Error outputs error to stderr and log file
	Error(format string, args ...interface{})

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
		t.logger.logWithLevel("INFO", "%s completed in %v", t.operation, elapsed)
	}
	return elapsed
}

// StopWithResult stops the timer and logs the result
func (t *Timer) StopWithResult(success bool, detail string) time.Duration {
	elapsed := time.Since(t.start)
	if t.logger != nil {
		status := "completed"
		level := "INFO"
		if !success {
			status = "failed"
			level = "WARN"
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
	mu     sync.Mutex
}

// New creates a new Logger that writes to the specified file.
func New(logPath string, debug bool) (Logger, error) {
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &fileLogger{
		file:  file,
		debug: debug,
	}, nil
}

// NewStdout creates a logger that only outputs to stdout/stderr.
func NewStdout(debug bool) Logger {
	return &fileLogger{
		debug: debug,
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

// getCaller returns the caller function name (skipping internal logging frames)
func getCaller(skip int) string {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}
	name := fn.Name()
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

// logWithLevel writes a log entry with the specified level
func (l *fileLogger) logWithLevel(level string, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("06-01-02 15:04:05.0")
	context := l.getContext()
	caller := getCaller(3) // Skip logWithLevel, the public method, and the caller

	// Format: [timestamp] [level] [context] [caller] message
	line := fmt.Sprintf("[%s] [%-5s] [%s] [%s] %s\n", timestamp, level, context, caller, msg)
	if _, err := l.file.WriteString(line); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write to log file: %v\n", err)
	}
}

func (l *fileLogger) Debug(format string, args ...interface{}) {
	if !l.debug {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	caller := getCaller(2)
	fmt.Fprintf(os.Stderr, "[DEBUG] [%s] %s\n", caller, msg)

	if l.file != nil {
		timestamp := time.Now().Format("06-01-02 15:04:05.0")
		context := l.getContext()
		line := fmt.Sprintf("[%s] [DEBUG] [%s] [%s] %s\n", timestamp, context, caller, msg)
		l.file.WriteString(line)
	}
}

func (l *fileLogger) Log(format string, args ...interface{}) {
	l.logWithLevel("INFO", format, args...)
}

func (l *fileLogger) Info(format string, args ...interface{}) {
	l.logWithLevel("INFO", format, args...)
}

func (l *fileLogger) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		timestamp := time.Now().Format("06-01-02 15:04:05.0")
		context := l.getContext()
		caller := getCaller(2)
		line := fmt.Sprintf("[%s] [WARN ] [%s] [%s] %s\n", timestamp, context, caller, msg)
		l.file.WriteString(line)
	}
}

func (l *fileLogger) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		timestamp := time.Now().Format("06-01-02 15:04:05.0")
		context := l.getContext()
		caller := getCaller(2)
		line := fmt.Sprintf("[%s] [ERROR] [%s] [%s] %s\n", timestamp, context, caller, msg)
		l.file.WriteString(line)
	}
}

func (l *fileLogger) StartTimer(operation string) *Timer {
	// Log start only if file is available
	if l.file != nil {
		l.logWithLevel("INFO", "%s started", operation)
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
var globalLogger Logger = NewStdout(os.Getenv("TAW_DEBUG") == "1")

// SetGlobal sets the global logger instance.
func SetGlobal(l Logger) {
	globalLogger = l
}

// Global returns the global logger instance.
func Global() Logger {
	return globalLogger
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

// StartTimer starts a timer for measuring operation duration using the global logger.
func StartTimer(operation string) *Timer {
	return globalLogger.StartTimer(operation)
}
