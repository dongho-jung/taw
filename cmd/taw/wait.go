package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/donghojung/taw/internal/constants"
	"github.com/donghojung/taw/internal/logging"
	"github.com/donghojung/taw/internal/notify"
	"github.com/donghojung/taw/internal/tmux"
)

const (
	waitMarker             = "TAW_WAITING"
	waitCaptureLines       = 200
	waitPollInterval       = 2 * time.Second
	waitMarkerMaxDistance  = 8
	waitAskUserMaxDistance = 32
	waitPopupWidth         = "70%"
	waitPopupHeight        = "50%"
)

var askUserQuestionUIMarkers = []string{
	"Enter to select",
	"Tab/Arrow keys to navigate",
	"Esc to cancel",
	"Type something.",
}

var watchWaitCmd = &cobra.Command{
	Use:   "watch-wait [session] [window-id] [task-name]",
	Short: "Watch agent output and notify when user input is needed",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		windowID := args[1]
		taskName := args[2]

		app, err := getAppFromSession(sessionName)
		if err != nil {
			return err
		}

		logger, _ := logging.New(app.GetLogPath(), app.Debug)
		if logger != nil {
			defer logger.Close()
			logger.SetScript("watch-wait")
			logger.SetTask(taskName)
			logging.SetGlobal(logger)
		}

		tm := tmux.New(sessionName)
		paneID := windowID + ".0"

		var lastContent string
		var lastPromptKey string
		notified := false
		promptActive := false
		wasWaiting := false
		waitReason := ""
		waitMessage := ""
		var waitDialog *dialogProcess
		waitDialogStopping := false

		for {
			if !tm.HasPane(paneID) {
				logging.Debug("Pane %s no longer exists, stopping wait watcher", paneID)
				return nil
			}

			isFinal := false
			windowName, err := getWindowName(tm, windowID)
			if err != nil {
				// Window doesn't exist anymore, stop watcher
				logging.Debug("Window %s no longer exists, stopping wait watcher", windowID)
				return nil
			}

			// Verify this window still belongs to this task (prevents stale watcher issues)
			if extractedName, isTask := constants.ExtractTaskName(windowName); isTask {
				expectedName := taskName
				if len(expectedName) > 12 {
					expectedName = expectedName[:12]
				}
				if extractedName != expectedName {
					// Window was reassigned to a different task, stop this watcher
					logging.Debug("Window %s now belongs to different task (%s vs %s), stopping wait watcher",
						windowID, extractedName, expectedName)
					return nil
				}
			}

			isFinal = isFinalWindow(windowName)
			if isWaitingWindow(windowName) {
				if !notified {
					notifyWaitingWithDisplay(tm, taskName, "window")
					notified = true
				}
			} else {
				notified = false
			}

			content, err := tm.CapturePane(paneID, waitCaptureLines)
			if err != nil {
				logging.Trace("Failed to capture pane: %v", err)
				time.Sleep(waitPollInterval)
				continue
			}

			contentChanged := content != lastContent
			if contentChanged {
				lastContent = content
			}

			currentWait := wasWaiting
			currentReason := waitReason
			if contentChanged {
				waitDetected, reason := detectWaitInContent(content)
				currentWait = waitDetected && !isFinal
				if waitDetected {
					currentReason = reason
				} else if !currentWait {
					currentReason = ""
				}
				if waitDetected && !isFinal {
					if err := ensureWaitingWindow(tm, windowID, taskName); err != nil {
						logging.Trace("Failed to rename window: %v", err)
					}
					if !notified {
						logging.Debug("Wait detected: %s", reason)
						notifyWaitingWithDisplay(tm, taskName, reason)
						notified = true
					}
					if !promptActive {
						if prompt, ok := parseAskUserQuestion(content); ok {
							promptKey := prompt.key()
							if promptKey != "" && promptKey != lastPromptKey {
								promptActive = true
								lastPromptKey = promptKey
								choice, err := promptUserChoice(tm, prompt)
								promptActive = false
								if err != nil {
									logging.Trace("Prompt choice failed: %v", err)
								} else if choice != "" {
									if err := sendAgentResponse(tm, paneID, choice); err != nil {
										logging.Trace("Failed to send prompt response: %v", err)
									} else {
										logging.Debug("Sent prompt response: %s", choice)
									}
								}
							}
						}
					}
				}
			}

			if isFinal {
				currentWait = false
			}

			if waitDialog != nil {
				select {
				case err := <-waitDialog.done:
					endedKind := waitDialog.kind
					waitDialog = nil
					if waitDialogStopping {
						waitDialogStopping = false
						break
					}
					if err != nil {
						logging.Trace("Wait dialog exited: %v", err)
						if currentWait && endedKind == dialogKindAppleScript {
							fallbackDialog, fallbackErr := startWaitDialog(waitMessage, dialogKindJXA)
							if fallbackErr != nil {
								logging.Trace("Wait dialog fallback failed: %v", fallbackErr)
							} else if fallbackDialog != nil {
								waitDialog = fallbackDialog
							}
						}
					}
				default:
				}
			}

			if currentWait && !wasWaiting {
				waitMessage = buildWaitMessage(taskName, currentReason)
				if waitDialog == nil {
					dialog, err := startWaitDialog(waitMessage, dialogKindAppleScript)
					if err != nil {
						logging.Trace("Wait dialog failed: %v", err)
					} else if dialog != nil {
						waitDialog = dialog
					}
				}
			}

			if !currentWait && wasWaiting && waitDialog != nil {
				waitDialogStopping = true
				stopWaitDialog(waitDialog)
			}

			wasWaiting = currentWait
			waitReason = currentReason

			time.Sleep(waitPollInterval)
		}
	},
}

func getWindowName(tm tmux.Client, windowID string) (string, error) {
	return tm.RunWithOutput("display-message", "-t", windowID, "-p", "#{window_name}")
}

func isWaitingWindow(name string) bool {
	return strings.HasPrefix(name, constants.EmojiWaiting)
}

func isFinalWindow(name string) bool {
	return strings.HasPrefix(name, constants.EmojiDone) ||
		strings.HasPrefix(name, constants.EmojiWarning)
}

func ensureWaitingWindow(tm tmux.Client, windowID, taskName string) error {
	windowName, err := getWindowName(tm, windowID)
	if err != nil {
		// Window doesn't exist, nothing to do
		return nil
	}

	// Check if this window belongs to this task (prevents cross-task renaming)
	extractedName, isTask := constants.ExtractTaskName(windowName)
	if !isTask {
		// Not a task window, don't rename
		return nil
	}

	// Verify task name matches (accounting for truncation to 12 chars)
	expectedName := taskName
	if len(expectedName) > 12 {
		expectedName = expectedName[:12]
	}
	if extractedName != expectedName {
		// Wrong task window, don't rename (prevents race condition)
		return nil
	}

	// Already in final state, don't change
	if isWaitingWindow(windowName) || isFinalWindow(windowName) {
		return nil
	}

	return tm.RenameWindow(windowID, waitingWindowName(taskName))
}

func waitingWindowName(taskName string) string {
	name := taskName
	if len(name) > 12 {
		name = name[:12]
	}
	return constants.EmojiWaiting + name
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

func notifyWaiting(taskName, reason string) {
	title := "TAW: Waiting for input"
	message := fmt.Sprintf("Task %s needs your response.", taskName)
	if err := notify.Send(title, message); err != nil {
		logging.Trace("Failed to send notification: %v", err)
	}
	// Play sound to alert user
	notify.PlaySound(notify.SoundNeedInput)
}

func notifyWaitingWithDisplay(tm tmux.Client, taskName, reason string) {
	notifyWaiting(taskName, reason)
	// Show message in tmux status bar
	displayMsg := fmt.Sprintf("💬 %s needs input", taskName)
	if reason != "" && reason != "window" && reason != "marker" {
		displayMsg = fmt.Sprintf("💬 %s: %s", taskName, reason)
	}
	if err := tm.DisplayMessage(displayMsg, 3000); err != nil {
		logging.Trace("Failed to display message: %v", err)
	}
}

type dialogKind string

const (
	dialogKindAppleScript dialogKind = "applescript"
	dialogKindJXA        dialogKind = "jxa"
)

type dialogProcess struct {
	cmd  *exec.Cmd
	done chan error
	kind dialogKind
}

func buildWaitMessage(taskName, reason string) string {
	message := fmt.Sprintf("TAW needs your input for task %s.", taskName)
	if reason != "" && reason != "window" {
		message = fmt.Sprintf("TAW needs your input for task %s (%s).", taskName, reason)
	}
	return message
}

func startWaitDialog(message string, kind dialogKind) (*dialogProcess, error) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}

	script := ""
	args := []string{}
	switch kind {
	case dialogKindJXA:
		script = buildWaitDialogJXAScript(message)
		args = []string{"-l", "JavaScript"}
	default:
		script = buildWaitDialogScript(message)
	}

	cmd := exec.Command("osascript", args...)
	return startDialogProcess(cmd, script, kind)
}

func startDialogProcess(cmd *exec.Cmd, script string, kind dialogKind) (*dialogProcess, error) {
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		if errOutput != "" {
			return nil, fmt.Errorf("%w: %s", err, errOutput)
		}
		return nil, err
	}

	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			errOutput := strings.TrimSpace(stderr.String())
			if errOutput != "" {
				err = fmt.Errorf("%w: %s", err, errOutput)
			}
		}
		done <- err
	}()

	return &dialogProcess{
		cmd:  cmd,
		done: done,
		kind: kind,
	}, nil
}

func stopWaitDialog(dialog *dialogProcess) {
	if dialog == nil || dialog.cmd == nil || dialog.cmd.Process == nil {
		return
	}
	_ = dialog.cmd.Process.Kill()
}

type askPrompt struct {
	Question string
	Options  []string
}

func (p askPrompt) key() string {
	if p.Question == "" || len(p.Options) == 0 {
		return ""
	}
	return p.Question + "\n" + strings.Join(p.Options, "\n")
}

func parseAskUserQuestion(content string) (askPrompt, bool) {
	lines := strings.Split(content, "\n")
	index := findAskUserQuestionIndex(lines)
	if index == -1 {
		return askPrompt{}, false
	}

	var prompt askPrompt
	foundQuestion := false
	for _, line := range lines[index+1:] {
		if value, ok := parseAskField(line, "question"); ok {
			if foundQuestion {
				break
			}
			prompt.Question = value
			foundQuestion = true
			continue
		}
		if !foundQuestion {
			continue
		}
		if value, ok := parseAskField(line, "label"); ok {
			if value != "" {
				prompt.Options = append(prompt.Options, value)
			}
		}
	}

	if prompt.Question == "" || len(prompt.Options) == 0 {
		return askPrompt{}, false
	}
	return prompt, true
}

func findAskUserQuestionIndex(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "AskUserQuestion") {
			return i
		}
	}
	return -1
}

func parseAskField(line, field string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	prefixes := []string{field + ":", "- " + field + ":"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			value = strings.Trim(value, "\"'")
			return value, value != ""
		}
	}
	return "", false
}

func promptUserChoice(tm tmux.Client, prompt askPrompt) (string, error) {
	if prompt.Question == "" || len(prompt.Options) == 0 {
		return "", nil
	}

	outFile, err := os.CreateTemp("", "taw-choice-*.txt")
	if err != nil {
		return "", err
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)

	scriptFile, err := os.CreateTemp("", "taw-choice-*.sh")
	if err != nil {
		return "", err
	}
	scriptPath := scriptFile.Name()
	scriptContent := buildPromptScript(prompt.Question, prompt.Options, outPath)
	if _, err := scriptFile.WriteString(scriptContent); err != nil {
		scriptFile.Close()
		os.Remove(scriptPath)
		return "", err
	}
	scriptFile.Close()
	if err := os.Chmod(scriptPath, 0700); err != nil {
		os.Remove(scriptPath)
		return "", err
	}
	defer os.Remove(scriptPath)

	opts := tmux.PopupOpts{
		Width:  waitPopupWidth,
		Height: waitPopupHeight,
		Title:  " TAW: " + truncate(strings.ReplaceAll(prompt.Question, "\n", " "), 60) + " ",
		Close:  true,
	}

	if err := tm.DisplayPopup(opts, scriptPath); err != nil {
		logging.Trace("Popup prompt failed, falling back to dialog: %v", err)
		return promptUserChoiceDialog(prompt)
	}

	choiceBytes, err := os.ReadFile(outPath)
	if err != nil {
		return "", err
	}

	choice := strings.TrimSpace(string(choiceBytes))
	return choice, nil
}

func newOSAScriptCommand(args ...string) *exec.Cmd {
	if runtime.GOOS != "darwin" {
		return exec.Command("osascript", args...)
	}
	uid := os.Getuid()
	if uid > 0 {
		cmdArgs := append([]string{"asuser", fmt.Sprintf("%d", uid), "osascript"}, args...)
		return exec.Command("launchctl", cmdArgs...)
	}
	return exec.Command("osascript", args...)
}

func runAppleScript(script string, captureOutput bool) (string, error) {
	return runOSAScript(script, captureOutput)
}

func runOSAScript(script string, captureOutput bool, args ...string) (string, error) {
	output, err := runOSAScriptCommand(newOSAScriptCommand(args...), script, captureOutput)
	if err == nil || runtime.GOOS != "darwin" {
		return output, err
	}
	fallbackOutput, fallbackErr := runOSAScriptCommand(exec.Command("osascript", args...), script, captureOutput)
	if fallbackErr == nil {
		return fallbackOutput, nil
	}
	return "", fmt.Errorf("osascript failed: %v; fallback failed: %v", err, fallbackErr)
}

func runOSAScriptCommand(cmd *exec.Cmd, script string, captureOutput bool) (string, error) {
	cmd.Stdin = strings.NewReader(script)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if captureOutput {
		cmd.Stdout = &stdout
	} else {
		cmd.Stdout = io.Discard
	}
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		if errOutput != "" {
			return "", fmt.Errorf("%w: %s", err, errOutput)
		}
		return "", err
	}
	if captureOutput {
		return strings.TrimSpace(stdout.String()), nil
	}
	return "", nil
}

func promptUserChoiceDialog(prompt askPrompt) (string, error) {
	if prompt.Question == "" || len(prompt.Options) == 0 {
		return "", nil
	}

	script := buildAppleScript(prompt.Question, prompt.Options)
	output, err := runAppleScript(script, true)
	if err != nil {
		logging.Trace("Dialog failed: %v", err)
		return "", nil
	}
	return strings.TrimSpace(output), nil
}

func buildPromptScript(question string, options []string, outPath string) string {
	var optionsLine strings.Builder
	optionsLine.WriteString("options=(")
	for i, option := range options {
		if i > 0 {
			optionsLine.WriteString(" ")
		}
		optionsLine.WriteString(shellQuote(option))
	}
	optionsLine.WriteString(")")

	appleScript := buildAppleScript(question, options)

	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

question=%s
%s
out=%s

tmpfile=$(mktemp)
cleanup() { rm -f "$tmpfile"; }
trap cleanup EXIT

osascript_cmd=(osascript)
if command -v launchctl >/dev/null 2>&1; then
  uid=$(id -u)
  if [[ -n "$uid" ]]; then
    osascript_cmd=(launchctl asuser "$uid" osascript)
  fi
fi

"${osascript_cmd[@]}" <<'TAW_EOF' >"$tmpfile" &
%s
TAW_EOF

osascript_pid=$!

printf "%%s\n\n" "$question"
index=1
for option in "${options[@]}"; do
  printf "  %%d) %%s\n" "$index" "$option"
  index=$((index+1))
done
printf "\nSelect [1-%%d]: " "${#options[@]}"

while true; do
  if read -r -n1 -t 0.2 key; then
    if [[ "$key" =~ [0-9] ]]; then
      idx=$((key-1))
      if [[ $idx -ge 0 && $idx -lt ${#options[@]} ]]; then
        kill "$osascript_pid" 2>/dev/null || true
        echo "${options[$idx]}" > "$out"
        exit 0
      fi
    fi
  fi

  if ! kill -0 "$osascript_pid" 2>/dev/null; then
    result=$(cat "$tmpfile" | tr -d '\n')
    if [[ -n "$result" ]]; then
      echo "$result" > "$out"
    fi
    exit 0
  fi
done
`, shellQuote(question), optionsLine.String(), shellQuote(outPath), appleScript)
}

func buildAppleScript(question string, options []string) string {
	escapedQuestion := escapeAppleScriptString(question)
	buttons := make([]string, 0, len(options))
	for _, option := range options {
		buttons = append(buttons, fmt.Sprintf(`"%s"`, escapeAppleScriptString(option)))
	}
	defaultButton := `""`
	if len(options) > 0 {
		defaultButton = fmt.Sprintf(`"%s"`, escapeAppleScriptString(options[0]))
	}

	return fmt.Sprintf(`display dialog "%s" buttons {%s} default button %s
button returned of result`, escapedQuestion, strings.Join(buttons, ", "), defaultButton)
}

func buildWaitDialogScript(message string) string {
	escapedMessage := escapeAppleScriptString(message)
	return fmt.Sprintf(`display dialog "%s" buttons {"OK"} default button "OK"`, escapedMessage)
}

func buildWaitDialogJXAScript(message string) string {
	escapedMessage := escapeJavaScriptString(message)
	return fmt.Sprintf(`ObjC.import('AppKit');
var app = $.NSApplication.sharedApplication;
app.activateIgnoringOtherApps(true);
var alert = $.NSAlert.alloc.init;
alert.messageText = "%s";
alert.addButtonWithTitle("OK");
alert.runModal();`, escapedMessage)
}

func escapeAppleScriptString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func escapeJavaScriptString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func sendAgentResponse(tm tmux.Client, paneID, response string) error {
	if err := tm.SendKeysLiteral(paneID, response); err != nil {
		return err
	}
	if err := tm.SendKeys(paneID, "Escape"); err != nil {
		return err
	}
	return tm.SendKeys(paneID, "Enter")
}
