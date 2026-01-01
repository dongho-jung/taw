package main

import (
	"github.com/spf13/cobra"
)

// internalCmd groups all internal commands
var internalCmd = &cobra.Command{
	Use:    "internal",
	Short:  "Internal commands (used by tmux keybindings)",
	Hidden: true,
}

func init() {
	// Task creation commands
	internalCmd.AddCommand(toggleNewCmd)
	internalCmd.AddCommand(newTaskCmd)
	internalCmd.AddCommand(spawnTaskCmd)
	internalCmd.AddCommand(handleTaskCmd)

	// Task lifecycle commands
	internalCmd.AddCommand(endTaskCmd)
	internalCmd.AddCommand(endTaskUICmd)
	internalCmd.AddCommand(cancelTaskCmd)
	internalCmd.AddCommand(cancelTaskUICmd)
	internalCmd.AddCommand(doneTaskCmd)
	internalCmd.AddCommand(recoverTaskCmd)

	// Popup/UI commands
	internalCmd.AddCommand(popupShellCmd)
	internalCmd.AddCommand(toggleLogCmd)
	internalCmd.AddCommand(logViewerCmd)
	internalCmd.AddCommand(toggleHelpCmd)
	internalCmd.AddCommand(toggleGitStatusCmd)
	internalCmd.AddCommand(toggleTaskListCmd)
	internalCmd.AddCommand(taskListViewerCmd)
	internalCmd.AddCommand(loadingScreenCmd)

	// Utility commands
	internalCmd.AddCommand(ctrlCCmd)
	internalCmd.AddCommand(renameWindowCmd)
	internalCmd.AddCommand(watchWaitCmd)

	// Add flags to end-task command
	endTaskCmd.Flags().StringVar(&paneCaptureFile, "pane-capture-file", "", "Path to pre-captured pane content file")
}
