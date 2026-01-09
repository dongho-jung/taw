package main

import (
	"strings"
)

// detectDoneInContent checks if the content contains the PAW_DONE marker.
// Returns true if the marker is found within doneMarkerMaxDistance lines from the end
// AND in the last segment (after the last ⏺ marker, which indicates a new Claude response).
// This prevents a previously completed task from staying "done" when given new work.
func detectDoneInContent(content string) bool {
	lines := strings.Split(content, "\n")
	lines = trimTrailingEmpty(lines)
	if len(lines) == 0 {
		return false
	}

	// Find the last segment (after the last ⏺ marker)
	// This ensures we only detect PAW_DONE in the most recent agent response
	segmentStart := findLastSegmentStart(lines)

	// Check the last N lines from the segment for the marker
	start := len(lines) - doneMarkerMaxDistance
	if start < segmentStart {
		start = segmentStart
	}
	for _, line := range lines[start:] {
		if matchesDoneMarker(line) {
			return true
		}
	}
	return false
}

// findLastSegmentStart finds the index of the last line starting with ⏺.
// Returns 0 if no segment marker is found (search entire content).
func findLastSegmentStart(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "⏺") {
			return i
		}
	}
	return 0
}

// matchesDoneMarker checks if a line contains the PAW_DONE marker.
// Allows prefix (like "⏺ " from Claude Code) but requires marker at end of line.
func matchesDoneMarker(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Exact match
	if trimmed == doneMarker {
		return true
	}
	// Allow prefix (e.g., "⏺ PAW_DONE") but marker must be at end
	if strings.HasSuffix(trimmed, " "+doneMarker) {
		return true
	}
	return false
}

func detectWaitInContent(content string) (bool, string) {
	lines := strings.Split(content, "\n")
	lines = trimTrailingEmpty(lines)
	if len(lines) == 0 {
		return false, ""
	}

	index, reason := findWaitMarker(lines)
	if index != -1 {
		linesAfter := len(lines) - index - 1
		maxDistance := waitMarkerMaxDistance
		if reason == "AskUserQuestion" {
			maxDistance = waitAskUserMaxDistance
		}
		if linesAfter <= maxDistance {
			return true, reason
		}
	}

	if index := findAskUserQuestionUIIndex(lines); index != -1 {
		linesAfter := len(lines) - index - 1
		if linesAfter <= waitAskUserMaxDistance {
			return true, "AskUserQuestionUI"
		}
	}

	if hasInputPrompt(lines) {
		return true, "prompt"
	}

	return false, ""
}

func trimTrailingEmpty(lines []string) []string {
	end := len(lines)
	for end > 0 {
		if strings.TrimSpace(lines[end-1]) != "" {
			break
		}
		end--
	}
	return lines[:end]
}

func hasInputPrompt(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	return strings.HasPrefix(last, ">")
}

func findWaitMarker(lines []string) (int, string) {
	index := -1
	reason := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == waitMarker:
			index = i
			reason = "marker"
		case strings.HasPrefix(trimmed, "AskUserQuestion"):
			index = i
			reason = "AskUserQuestion"
		}
	}
	return index, reason
}

func findAskUserQuestionUIIndex(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		for _, marker := range askUserQuestionUIMarkers {
			if strings.Contains(trimmed, marker) {
				return i
			}
		}
	}
	return -1
}
