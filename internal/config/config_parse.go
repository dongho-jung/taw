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

		// Check for nested block (inherit:)
		if key == "inherit" && value == "" {
			cfg.Inherit = parseInheritBlock(lines, &i)
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
		case "work_mode":
			cfg.WorkMode = WorkMode(value)
		case "on_complete":
			cfg.OnComplete = OnComplete(value)
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
		case "self_improve":
			if parsed, err := strconv.ParseBool(value); err == nil {
				cfg.SelfImprove = parsed
			}
		}
	}

	return cfg, nil
}

// parseInheritBlock parses the inherit configuration block.
func parseInheritBlock(lines []string, i *int) *InheritConfig {
	*i++ // Move past "inherit:" line
	inherit := &InheritConfig{}
	baseIndent := getIndentLevel(lines, *i-1)

	for *i < len(lines) {
		line := lines[*i]

		// Check if we're still in the inherit block (more indented than parent)
		if len(line) == 0 {
			*i++
			continue
		}

		currentIndent := countLeadingSpaces(line)
		if currentIndent <= baseIndent && strings.TrimSpace(line) != "" {
			// Less or equal indent, end of inherit block
			break
		}

		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			*i++
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			*i++
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		boolVal, _ := strconv.ParseBool(value)

		switch key {
		case "work_mode":
			inherit.WorkMode = boolVal
		case "on_complete":
			inherit.OnComplete = boolVal
		case "theme":
			inherit.Theme = boolVal
		case "log_format":
			inherit.LogFormat = boolVal
		case "log_max_size_mb":
			inherit.LogMaxSizeMB = boolVal
		case "log_max_backups":
			inherit.LogMaxBackups = boolVal
		case "self_improve":
			inherit.SelfImprove = boolVal
		}

		*i++
	}

	return inherit
}

// getIndentLevel returns the indentation level of a line at the given index.
func getIndentLevel(lines []string, index int) int {
	if index < 0 || index >= len(lines) {
		return 0
	}
	return countLeadingSpaces(lines[index])
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

// formatInheritBlock formats the inherit configuration block for saving.
func formatInheritBlock(inherit *InheritConfig) string {
	if inherit == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n# Inherit settings from global config\n")
	sb.WriteString("# Set to true to use global value, false to use project-specific value\n")
	sb.WriteString("inherit:\n")
	sb.WriteString(fmt.Sprintf("  work_mode: %t\n", inherit.WorkMode))
	sb.WriteString(fmt.Sprintf("  on_complete: %t\n", inherit.OnComplete))
	sb.WriteString(fmt.Sprintf("  theme: %t\n", inherit.Theme))
	sb.WriteString(fmt.Sprintf("  log_format: %t\n", inherit.LogFormat))
	sb.WriteString(fmt.Sprintf("  log_max_size_mb: %t\n", inherit.LogMaxSizeMB))
	sb.WriteString(fmt.Sprintf("  log_max_backups: %t\n", inherit.LogMaxBackups))
	sb.WriteString(fmt.Sprintf("  self_improve: %t\n", inherit.SelfImprove))
	return sb.String()
}
