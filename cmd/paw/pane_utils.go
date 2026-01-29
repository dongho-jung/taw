package main

import (
	"strconv"
	"strings"

	"github.com/dongho-jung/paw/internal/logging"
	"github.com/dongho-jung/paw/internal/tmux"
)

func findLeftmostPaneID(tm tmux.Client, windowID string) (string, error) {
	output, err := tm.RunWithOutput("list-panes", "-t", windowID, "-F", "#{pane_id}\t#{pane_left}")
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return "", nil
	}

	var leftmostID string
	found := false
	minLeft := 0
	for _, line := range lines {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			continue
		}
		left, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			continue
		}
		if !found || left < minLeft {
			found = true
			minLeft = left
			leftmostID = strings.TrimSpace(fields[0])
		}
	}

	return leftmostID, nil
}

func logPaneSnapshot(tm tmux.Client, windowID, reason string) {
	if strings.TrimSpace(windowID) == "" {
		return
	}

	logging.Debug("PaneSnapshot: reason=%s window=%s", reason, windowID)

	windowInfo, err := tm.RunWithOutput("display-message", "-t", windowID, "-p", "#{window_id}\t#{window_width}\t#{window_height}\t#{window_layout}")
	if err != nil {
		logging.Debug("PaneSnapshot: window info failed: %v", err)
	} else {
		logging.Debug("PaneSnapshot: window=%s", windowInfo)
	}

	panesInfo, err := tm.RunWithOutput("list-panes", "-t", windowID, "-F", "#{pane_id}\tidx=#{pane_index}\tleft=#{pane_left}\twidth=#{pane_width}\ttop=#{pane_top}\theight=#{pane_height}\tactive=#{pane_active}")
	if err != nil {
		logging.Debug("PaneSnapshot: list panes failed: %v", err)
		return
	}

	for _, line := range strings.Split(strings.TrimSpace(panesInfo), "\n") {
		if strings.TrimSpace(line) != "" {
			logging.Debug("PaneSnapshot: pane %s", line)
		}
	}
}

func forceFilePickerWidth(tm tmux.Client, windowID, reason string) {
	if strings.TrimSpace(windowID) == "" {
		return
	}

	logPaneSnapshot(tm, windowID, reason+"-before")

	leftmostID, err := findLeftmostPaneID(tm, windowID)
	if err != nil {
		logging.Debug("forceFilePickerWidth: find leftmost failed: %v", err)
		return
	}
	if strings.TrimSpace(leftmostID) == "" {
		logging.Debug("forceFilePickerWidth: leftmost pane empty")
		return
	}

	logging.Debug("forceFilePickerWidth: reason=%s window=%s leftmost=%s target=%s", reason, windowID, leftmostID, filePickerPaneWidth)
	_ = tm.SetOption(filePickerPaneIDKey, leftmostID, true)

	if err := tm.Run("resize-pane", "-t", leftmostID, "-x", filePickerPaneWidth); err != nil {
		logging.Debug("forceFilePickerWidth: resize failed: %v", err)
	}

	logPaneSnapshot(tm, windowID, reason+"-after")
}
