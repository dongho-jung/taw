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
	internalCmd.AddCommand(mergeTaskCmd)
	internalCmd.AddCommand(mergeTaskUICmd)
	internalCmd.AddCommand(doneTaskCmd)
	internalCmd.AddCommand(recoverTaskCmd)
	internalCmd.AddCommand(resumeAgentCmd)

	// Sync commands
	internalCmd.AddCommand(syncWithMainCmd)
	internalCmd.AddCommand(syncWithMainUICmd)

	// Popup/UI commands
	internalCmd.AddCommand(popupShellCmd)
	internalCmd.AddCommand(toggleLogCmd)
	internalCmd.AddCommand(logViewerCmd)
	internalCmd.AddCommand(toggleHelpCmd)
	internalCmd.AddCommand(helpViewerCmd)
	internalCmd.AddCommand(taskViewerCmd)
	internalCmd.AddCommand(toggleGitStatusCmd)
	internalCmd.AddCommand(gitViewerCmd)
	internalCmd.AddCommand(toggleShowDiffCmd)
	internalCmd.AddCommand(diffViewerCmd)
	internalCmd.AddCommand(toggleHistoryCmd)
	internalCmd.AddCommand(historyPickerCmd)
	internalCmd.AddCommand(toggleTemplateCmd)
	internalCmd.AddCommand(templatePickerCmd)
	internalCmd.AddCommand(toggleProjectPickerCmd)
	internalCmd.AddCommand(projectPickerCmd)
	internalCmd.AddCommand(projectPickerWrapperCmd)
	internalCmd.AddCommand(loadingScreenCmd)
	internalCmd.AddCommand(toggleCmdPaletteCmd)
	internalCmd.AddCommand(cmdPaletteTUICmd)
	internalCmd.AddCommand(restorePanesCmd)
	internalCmd.AddCommand(showCurrentTaskCmd)
	internalCmd.AddCommand(finishPickerTUICmd)
	internalCmd.AddCommand(prPopupTUICmd)
	internalCmd.AddCommand(togglePromptPickerCmd)
	internalCmd.AddCommand(promptPickerTUICmd)
	internalCmd.AddCommand(taskNameInputTUICmd)

	// Navigation commands
	internalCmd.AddCommand(selectPrevWindowCmd)
	internalCmd.AddCommand(selectNextWindowCmd)
	internalCmd.AddCommand(newShellWindowCmd)

	// Utility commands
	internalCmd.AddCommand(renameWindowCmd)
	internalCmd.AddCommand(stopHookCmd)
	internalCmd.AddCommand(userPromptSubmitHookCmd)
	internalCmd.AddCommand(askUserQuestionPreHookCmd)
	internalCmd.AddCommand(askUserQuestionHookCmd)
	internalCmd.AddCommand(watchWaitCmd)
	internalCmd.AddCommand(watchPRCmd)

	// Add flags to end-task command
	endTaskCmd.Flags().StringVar(&paneCaptureFile, "pane-capture-file", "", "Path to pre-captured pane content file")
	endTaskCmd.Flags().BoolVar(&endTaskUserInitiated, "user-initiated", false, "Require explicit user action to finish")
	endTaskCmd.Flags().StringVar(&endTaskAction, "action", "keep", "Finish action: keep, merge, pr, drop")

	// Add flags to end-task-ui command (receives action from finish-picker-tui)
	endTaskUICmd.Flags().StringVar(&endTaskAction, "action", "keep", "Finish action: keep, merge, pr, drop")
}
