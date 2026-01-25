package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/app"
	"github.com/dongho-jung/paw/internal/constants"
)

var (
	logsSince string
	logsTask  string
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show PAW logs with filters",
	RunE: func(_ *cobra.Command, _ []string) error {
		application, err := getAppFromCwd()
		if err != nil {
			return err
		}

		logPath := application.GetLogPath()
		file, err := os.Open(logPath)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		defer func() { _ = file.Close() }()

		var sinceTime time.Time
		if logsSince != "" {
			parsed, err := parseSince(logsSince)
			if err != nil {
				return err
			}
			sinceTime = parsed
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			ts, task, ok := parseLogLine(line)

			if !sinceTime.IsZero() {
				if !ok || ts.Before(sinceTime) {
					continue
				}
			}

			if logsTask != "" {
				if task == "" || task != logsTask {
					continue
				}
			}

			fmt.Println(line)
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read log file: %w", err)
		}

		return nil
	},
}

func init() {
	logsCmd.Flags().StringVar(&logsSince, "since", "", "Show logs since time (duration like 2h or timestamp)")
	logsCmd.Flags().StringVar(&logsTask, "task", "", "Filter logs by task name")
}

func getAppFromCwd() (*app.App, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	dir := cwd
	for {
		pawDir := filepath.Join(dir, constants.PawDirName)
		if _, err := os.Stat(pawDir); err == nil {
			application, err := app.New(dir)
			if err != nil {
				return nil, err
			}
			return loadAppConfig(application)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return nil, fmt.Errorf("could not find .paw directory from %s", cwd)
}

func parseLogLine(line string) (time.Time, string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Timestamp string `json:"ts"`
			Task      string `json:"task"`
			Context   string `json:"context"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			if payload.Task == "" && payload.Context != "" {
				payload.Task = extractTaskFromContext(payload.Context)
			}
			if payload.Timestamp != "" {
				if ts, err := time.Parse(time.RFC3339Nano, payload.Timestamp); err == nil {
					return ts, payload.Task, true
				}
			}
			return time.Time{}, payload.Task, false
		}
	}

	parts := strings.SplitN(trimmed, "] [", 4)
	if len(parts) < 3 {
		return time.Time{}, "", false
	}

	tsStr := strings.TrimPrefix(parts[0], "[")
	ts, err := time.ParseInLocation("06-01-02 15:04:05.0", tsStr, time.Local)
	if err != nil {
		return time.Time{}, "", false
	}

	context := strings.TrimSuffix(parts[2], "]")
	task := extractTaskFromContext(context)
	return ts, task, true
}

func extractTaskFromContext(context string) string {
	parts := strings.SplitN(context, ":", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}
