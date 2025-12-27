// Package logging provides unified logging functionality for TAW.
package logging

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Logger provides logging capabilities for TAW.
type Logger interface {
	// Debug outputs debug information (only when TAW_DEBUG=1)
	Debug(format string, args ...interface{})

	// Log writes to the unified log file with timestamp
	Log(format string, args ...interface{})

	// Warn outputs warning to stderr and log file
	Warn(format string, args ...interface{})

	// Error outputs error to stderr and log file
	Error(format string, args ...interface{})

	// SetScript sets the current script name for context
	SetScript(script string)

	// SetTask sets the current task name for context
	SetTask(task string)

	// Close closes the log file
	Close() error
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

func (l *fileLogger) Debug(format string, args ...interface{}) {
	if !l.debug {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", msg)
}

func (l *fileLogger) Log(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	context := l.getContext()

	line := fmt.Sprintf("[%s] [%s] %s\n", timestamp, context, msg)
	if _, err := l.file.WriteString(line); err != nil {
		// Log file write failed - output to stderr as fallback
		fmt.Fprintf(os.Stderr, "Failed to write to log file: %v\n", err)
	}
}

func (l *fileLogger) Warn(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)

	if l.file != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		context := l.getContext()
		line := fmt.Sprintf("[%s] [%s] WARN: %s\n", timestamp, context, msg)
		if _, err := l.file.WriteString(line); err != nil {
			// Silently fail - we already output to stderr
		}
	}
}

func (l *fileLogger) Error(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)

	if l.file != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		context := l.getContext()
		line := fmt.Sprintf("[%s] [%s] ERROR: %s\n", timestamp, context, msg)
		if _, err := l.file.WriteString(line); err != nil {
			// Silently fail - we already output to stderr
		}
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

// Warn logs a warning using the global logger.
func Warn(format string, args ...interface{}) {
	globalLogger.Warn(format, args...)
}

// Error logs an error using the global logger.
func Error(format string, args ...interface{}) {
	globalLogger.Error(format, args...)
}
