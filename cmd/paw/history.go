package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/service"
)

type historyEntry struct {
	Path      string
	Task      string
	Timestamp time.Time
	Cancelled bool
	Summary   string
}

type historyOptions struct {
	task        string
	since       string
	query       string
	limit       int
	withSummary bool
}

var (
	historyTask      string
	historySince     string
	historyQuery     string
	historyLimit     int
	historyNoSummary bool
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "List and view task history",
	RunE: func(_ *cobra.Command, _ []string) error {
		application, err := getAppFromCwd()
		if err != nil {
			return err
		}

		opts := historyOptions{
			task:        historyTask,
			since:       historySince,
			query:       historyQuery,
			limit:       historyLimit,
			withSummary: !historyNoSummary,
		}

		entries, err := loadHistoryEntries(application.GetHistoryDir(), opts)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No history entries found")
			return nil
		}

		printHistoryList(entries, opts.withSummary)
		return nil
	},
}

var historyShowCmd = &cobra.Command{
	Use:   "show [entry]",
	Short: "Show a history entry by index, task name, or path",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		application, err := getAppFromCwd()
		if err != nil {
			return err
		}

		ref := args[0]
		opts := historyOptions{
			task:        historyTask,
			since:       historySince,
			query:       historyQuery,
			limit:       0,
			withSummary: false,
		}

		entries, err := loadHistoryEntries(application.GetHistoryDir(), opts)
		if err != nil {
			return err
		}

		entry, err := resolveHistoryEntry(entries, application.GetHistoryDir(), ref)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(entry.Path)
		if err != nil {
			return fmt.Errorf("failed to read history entry: %w", err)
		}

		fmt.Print(string(data))
		if len(data) == 0 || data[len(data)-1] != '\n' {
			fmt.Println()
		}

		return nil
	},
}

func init() {
	historyCmd.PersistentFlags().StringVar(&historyTask, "task", "", "Filter history by task name (substring)")
	historyCmd.PersistentFlags().StringVar(&historySince, "since", "", "Filter history since time (duration or timestamp)")
	historyCmd.PersistentFlags().StringVar(&historyQuery, "query", "", "Filter history by text in the entry")
	historyCmd.PersistentFlags().IntVar(&historyLimit, "limit", 20, "Limit number of entries shown")
	historyCmd.Flags().BoolVar(&historyNoSummary, "no-summary", false, "Hide summary preview in list")
	historyCmd.AddCommand(historyShowCmd)
}

func loadHistoryEntries(historyDir string, opts historyOptions) ([]historyEntry, error) {
	if historyDir == "" {
		return nil, errors.New("history directory not set")
	}

	svc := service.NewHistoryService(historyDir)
	files, err := svc.ListHistoryFiles()
	if err != nil {
		return nil, err
	}

	sinceTime, err := parseSince(opts.since)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(strings.TrimSpace(opts.query))
	taskLower := strings.ToLower(strings.TrimSpace(opts.task))
	readContent := opts.withSummary || queryLower != ""

	entries := make([]historyEntry, 0, len(files))

	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		ts := parseHistoryTimestamp(path)
		if ts.IsZero() {
			ts = info.ModTime()
		}

		if !sinceTime.IsZero() && ts.Before(sinceTime) {
			continue
		}

		taskName := service.ExtractTaskName(path)
		if taskLower != "" && !strings.Contains(strings.ToLower(taskName), taskLower) {
			continue
		}

		entry := historyEntry{
			Path:      path,
			Task:      taskName,
			Timestamp: ts,
			Cancelled: service.IsCancelled(path),
		}

		var content string
		if readContent {
			data, err := os.ReadFile(path) //nolint:gosec // G304: path is from controlled history directory
			if err != nil {
				continue
			}
			content = string(data)
			if queryLower != "" && !strings.Contains(strings.ToLower(content), queryLower) {
				continue
			}
			if opts.withSummary {
				entry.Summary = summaryLine(extractSummary(content), 120)
			}
		}

		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	if opts.limit > 0 && len(entries) > opts.limit {
		entries = entries[:opts.limit]
	}

	return entries, nil
}

func parseHistoryTimestamp(path string) time.Time {
	base := filepath.Base(strings.TrimSuffix(path, ".cancelled"))
	parts := strings.SplitN(base, "_", 3)
	if len(parts) < 3 {
		return time.Time{}
	}
	tsStr := parts[0] + "_" + parts[1]
	parsed, err := time.ParseInLocation("060102_150405", tsStr, time.Local)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func extractSummary(content string) string {
	const summaryMarker = "\n---summary---\n"
	idx := strings.Index(content, summaryMarker)
	if idx == -1 {
		return ""
	}
	start := idx + len(summaryMarker)
	end := len(content)
	for _, marker := range []string{"\n---capture---\n", "\n---hooks---\n"} {
		if next := strings.Index(content[start:], marker); next != -1 {
			end = start + next
			break
		}
	}
	return strings.TrimSpace(content[start:end])
}

func summaryLine(summary string, maxLen int) string {
	if summary == "" {
		return ""
	}
	fields := strings.Fields(summary)
	joined := strings.Join(fields, " ")
	if maxLen > 0 && len(joined) > maxLen {
		if maxLen <= 3 {
			return joined[:maxLen]
		}
		return joined[:maxLen-3] + "..."
	}
	return joined
}

func printHistoryList(entries []historyEntry, withSummary bool) {
	for i, entry := range entries {
		status := "done"
		if entry.Cancelled {
			status = "cancelled"
		}
		line := fmt.Sprintf("%2d. %s  %-9s %s", i+1, entry.Timestamp.Format("2006-01-02 15:04"), status, entry.Task)
		if withSummary && entry.Summary != "" {
			line += " - " + entry.Summary
		}
		fmt.Println(line)
	}
}

func resolveHistoryEntry(entries []historyEntry, historyDir, ref string) (historyEntry, error) {
	if ref == "" {
		return historyEntry{}, errors.New("entry reference required")
	}

	if idx, err := strconv.Atoi(ref); err == nil {
		if idx < 1 || idx > len(entries) {
			return historyEntry{}, fmt.Errorf("entry index out of range: %d", idx)
		}
		return entries[idx-1], nil
	}

	if path, ok := resolveHistoryPath(historyDir, ref); ok {
		return historyEntry{Path: path}, nil
	}

	for _, entry := range entries {
		if entry.Task == ref {
			return entry, nil
		}
	}

	for _, entry := range entries {
		if strings.Contains(entry.Task, ref) {
			return entry, nil
		}
	}

	return historyEntry{}, fmt.Errorf("history entry not found: %s", ref)
}

func resolveHistoryPath(historyDir, ref string) (string, bool) {
	if ref == "" {
		return "", false
	}
	if _, err := os.Stat(ref); err == nil {
		return ref, true
	}
	if historyDir == "" {
		return "", false
	}
	candidate := filepath.Join(historyDir, ref)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, true
	}
	return "", false
}
