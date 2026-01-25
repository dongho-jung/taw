package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/tmux"
)

// hiddenWindowPrefix is the prefix for windows that should be hidden from navigation.
const hiddenWindowPrefix = "_paw_"

var selectPrevWindowCmd = &cobra.Command{
	Use:    "select-prev-window [session]",
	Short:  "Select previous window, skipping hidden windows",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return selectAdjacentWindow(args[0], -1)
	},
}

var selectNextWindowCmd = &cobra.Command{
	Use:    "select-next-window [session]",
	Short:  "Select next window, skipping hidden windows",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return selectAdjacentWindow(args[0], 1)
	},
}

// selectAdjacentWindow selects the previous (direction=-1) or next (direction=1) window,
// skipping hidden windows whose names start with hiddenWindowPrefix.
func selectAdjacentWindow(sessionName string, direction int) error {
	tm := tmux.New(sessionName)

	windows, err := tm.ListWindows()
	if err != nil {
		return err
	}

	if len(windows) == 0 {
		return nil
	}

	// Find visible windows (not starting with hiddenWindowPrefix)
	visibleWindows := make([]tmux.Window, 0, len(windows))
	currentIdx := -1
	for _, w := range windows {
		if strings.HasPrefix(w.Name, hiddenWindowPrefix) {
			continue
		}
		if w.Active {
			currentIdx = len(visibleWindows)
		}
		visibleWindows = append(visibleWindows, w)
	}

	if len(visibleWindows) <= 1 || currentIdx < 0 {
		// No other visible windows to navigate to
		return nil
	}

	// Calculate target index with wrapping
	targetIdx := (currentIdx + direction + len(visibleWindows)) % len(visibleWindows)
	targetWindow := visibleWindows[targetIdx]

	return tm.SelectWindow(targetWindow.ID)
}
