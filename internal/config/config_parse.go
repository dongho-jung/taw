// Package config handles PAW configuration parsing and management.
package config

import (
	"fmt"
	"strconv"
	"strings"
)

// parseConfig parses the configuration from a string.
// Supports multi-line values using YAML-like '|' syntax.
// Supports nested configuration blocks for notifications.
func parseConfig(content string) (*Config, error) {
	cfg := DefaultConfig()
	lines := strings.Split(content, "\n")

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			i++
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Skip unsupported nested blocks to avoid mis-parsing indented content.
		if value == "" && hasIndentedBlock(lines, i) {
			skipIndentedBlock(lines, &i)
			continue
		}

		// Check for multi-line value (starts with '|')
		if value == "|" {
			// Read subsequent indented lines
			var multiLines []string
			i++
			for i < len(lines) {
				nextLine := lines[i]
				// Empty line within multi-line block
				if strings.TrimSpace(nextLine) == "" {
					multiLines = append(multiLines, "")
					i++
					continue
				}
				// Check if line is indented (part of multi-line block)
				if len(nextLine) > 0 && (nextLine[0] == ' ' || nextLine[0] == '\t') {
					// Remove the leading indentation (first 2 spaces or 1 tab)
					trimmedLine := nextLine
					if strings.HasPrefix(trimmedLine, "  ") {
						trimmedLine = trimmedLine[2:]
					} else if strings.HasPrefix(trimmedLine, "\t") {
						trimmedLine = trimmedLine[1:]
					}
					multiLines = append(multiLines, trimmedLine)
					i++
				} else {
					// Non-indented line, end of multi-line block
					break
				}
			}
			value = strings.Join(multiLines, "\n")
			// Trim trailing newlines from multi-line value
			value = strings.TrimRight(value, "\n")
		} else {
			i++
		}

		switch key {
		case "pre_worktree_hook":
			cfg.PreWorktreeHook = value
		case "pre_task_hook":
			cfg.PreTaskHook = value
		case "post_task_hook":
			cfg.PostTaskHook = value
		case "pre_merge_hook":
			cfg.PreMergeHook = value
		case "post_merge_hook":
			cfg.PostMergeHook = value
		case "log_format":
			cfg.LogFormat = value
		case "log_max_size_mb":
			if parsed, err := strconv.Atoi(value); err == nil {
				cfg.LogMaxSizeMB = parsed
			}
		case "log_max_backups":
			if parsed, err := strconv.Atoi(value); err == nil {
				cfg.LogMaxBackups = parsed
			}
		}
	}

	return cfg, nil
}

// getIndentLevel returns the indentation level of a line at the given index.
func getIndentLevel(lines []string, index int) int {
	if index < 0 || index >= len(lines) {
		return 0
	}
	return countLeadingSpaces(lines[index])
}

func hasIndentedBlock(lines []string, index int) bool {
	baseIndent := getIndentLevel(lines, index)
	for i := index + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		return getIndentLevel(lines, i) > baseIndent
	}
	return false
}

// countLeadingSpaces counts the number of leading spaces/tabs in a string.
// Tabs are counted as 2 spaces.
func countLeadingSpaces(s string) int {
	count := 0
	for _, ch := range s {
		switch ch {
		case ' ':
			count++
		case '\t':
			count += 2
		default:
			return count
		}
	}
	return count
}

// skipIndentedBlock skips a nested YAML-like block.
func skipIndentedBlock(lines []string, i *int) {
	*i++ // Move past the parent line
	baseIndent := getIndentLevel(lines, *i-1)

	for *i < len(lines) {
		line := lines[*i]
		if len(strings.TrimSpace(line)) == 0 {
			*i++
			continue
		}
		currentIndent := countLeadingSpaces(line)
		if currentIndent <= baseIndent {
			break
		}
		*i++
	}
}

// formatHook formats a hook command for saving.
// Multi-line values use YAML-like '|' syntax.
func formatHook(key, hook string) string {
	if strings.Contains(hook, "\n") {
		// Multi-line: use | syntax with indentation
		lines := strings.Split(hook, "\n")
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s: |\n", key))
		for _, line := range lines {
			sb.WriteString("  ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		return sb.String()
	}
	// Single line
	return fmt.Sprintf("%s: %s\n", key, hook)
}
