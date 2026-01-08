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
	internalCmd.AddCommand(syncTaskCmd)
	internalCmd.AddCommand(toggleBranchCmd)

	// Popup/UI commands
	internalCmd.AddCommand(popupShellCmd)
	internalCmd.AddCommand(toggleLogCmd)
	internalCmd.AddCommand(logViewerCmd)
	internalCmd.AddCommand(toggleHelpCmd)
	internalCmd.AddCommand(helpViewerCmd)
	internalCmd.AddCommand(toggleGitStatusCmd)
	internalCmd.AddCommand(toggleShowDiffCmd)
	internalCmd.AddCommand(toggleTemplateCmd)
	internalCmd.AddCommand(templateViewerCmd)
	internalCmd.AddCommand(templateEditorCmd)
	internalCmd.AddCommand(loadingScreenCmd)
	internalCmd.AddCommand(toggleSetupCmd)
	internalCmd.AddCommand(setupWizardCmd)
	internalCmd.AddCommand(toggleCmdPaletteCmd)
	internalCmd.AddCommand(cmdPaletteTUICmd)
	internalCmd.AddCommand(restorePanesCmd)

	// Utility commands
	internalCmd.AddCommand(ctrlCCmd)
	internalCmd.AddCommand(renameWindowCmd)
	internalCmd.AddCommand(stopHookCmd)
	internalCmd.AddCommand(userPromptSubmitHookCmd)
	internalCmd.AddCommand(askUserQuestionHookCmd)
	internalCmd.AddCommand(watchWaitCmd)

	// Add flags to end-task command
	endTaskCmd.Flags().StringVar(&paneCaptureFile, "pane-capture-file", "", "Path to pre-captured pane content file")
	endTaskCmd.Flags().BoolVar(&endTaskUserInitiated, "user-initiated", false, "Require explicit user action to finish")
}
