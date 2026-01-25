package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/tmux"
)

var killCmd = &cobra.Command{
	Use:   "kill [session]",
	Short: "Kill PAW tmux sessions",
	Long: `Kill a PAW tmux session without removing .paw directory.

If a session name is provided, kills that specific session.
If no session name is provided, lists available sessions to choose from.

Prompts for confirmation before killing.

Unlike 'paw clean', this preserves the .paw directory, worktrees, and branches.

Examples:
  paw kill             # List and select a session to kill
  paw kill myproject   # Kill the 'myproject' session directly

See also: paw kill-all (to kill all sessions at once)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runKill,
}

var killAllCmd = &cobra.Command{
	Use:   "kill-all",
	Short: "Kill all running PAW sessions",
	Long: `Kill all running PAW tmux sessions without removing .paw directories.

Prompts for confirmation before killing all sessions.

Unlike 'paw clean', this preserves .paw directories, worktrees, and branches.`,
	RunE: runKillAll,
}

// Note: killAllCmd is registered in main.go as a root command

func runKill(_ *cobra.Command, args []string) error {
	sessions := findPawSessions()

	if len(sessions) == 0 {
		fmt.Println("No running PAW sessions found.")
		return nil
	}

	var targetSession pawSession

	switch {
	case len(args) == 1:
		// Direct kill of specified session
		sessionName := args[0]
		for _, s := range sessions {
			if s.Name == sessionName {
				targetSession = s
				break
			}
		}
		if targetSession.Name == "" {
			// Try partial match
			var matches []pawSession
			for _, s := range sessions {
				if strings.Contains(s.Name, sessionName) {
					matches = append(matches, s)
				}
			}
			switch {
			case len(matches) == 1:
				targetSession = matches[0]
			case len(matches) > 1:
				fmt.Printf("Multiple sessions match '%s':\n", sessionName)
				for _, m := range matches {
					fmt.Printf("  - %s\n", m.Name)
				}
				return errors.New("please specify a unique session name")
			default:
				fmt.Printf("Session '%s' not found.\n\n", sessionName)
				fmt.Println("Available sessions:")
				for _, s := range sessions {
					fmt.Printf("  - %s\n", s.Name)
				}
				return errors.New("session not found")
			}
		}
	case len(sessions) == 1:
		// Only one session, confirm and kill
		targetSession = sessions[0]
		fmt.Printf("Found session: %s\n", targetSession.Name)
	default:
		// Multiple sessions, prompt for selection
		fmt.Println("Running PAW sessions:")
		fmt.Println()
		for i, s := range sessions {
			fmt.Printf("  %d. %s\n", i+1, s.Name)
		}
		fmt.Println()
		fmt.Print("Select session to kill [1]: ")

		var input string
		_, _ = fmt.Scanln(&input)
		input = strings.TrimSpace(input)

		// Default to first session
		idx := 0
		if input != "" {
			var n int
			if _, err := fmt.Sscanf(input, "%d", &n); err != nil || n < 1 || n > len(sessions) {
				return fmt.Errorf("invalid selection: %s", input)
			}
			idx = n - 1
		}
		targetSession = sessions[idx]
	}

	// Confirm before killing
	if !confirmPrompt(fmt.Sprintf("Kill session '%s'? [y/N]: ", targetSession.Name)) {
		fmt.Println("Cancelled.")
		return nil
	}

	return forceKillSession(targetSession, true)
}

func runKillAll(_ *cobra.Command, _ []string) error {
	sessions := findPawSessions()

	if len(sessions) == 0 {
		fmt.Println("No running PAW sessions found.")
		return nil
	}

	fmt.Printf("Found %d PAW session(s) to kill:\n", len(sessions))
	for _, s := range sessions {
		fmt.Printf("  - %s\n", s.Name)
	}
	fmt.Println()

	// Confirm before killing
	if !confirmPrompt(fmt.Sprintf("Kill all %d session(s)? [y/N]: ", len(sessions))) {
		fmt.Println("Cancelled.")
		return nil
	}

	var failed []string
	for _, s := range sessions {
		if err := forceKillSession(s, false); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", s.Name, err))
		}
	}

	if len(failed) > 0 {
		fmt.Println("\nFailed to kill some sessions:")
		for _, f := range failed {
			fmt.Printf("  - %s\n", f)
		}
		return fmt.Errorf("failed to kill %d session(s)", len(failed))
	}

	fmt.Println("\nAll sessions killed successfully.")
	return nil
}

// forceKillSession kills a PAW session immediately without graceful shutdown.
// If verbose is true, prints progress messages.
func forceKillSession(session pawSession, verbose bool) error {
	tm := tmux.New(session.Name)

	// Check if session still exists
	if !tm.HasSession(session.Name) {
		if verbose {
			fmt.Printf("Session '%s' is no longer running.\n", session.Name)
		}
		return nil
	}

	if verbose {
		fmt.Printf("Killing session '%s'...\n", session.Name)
	}

	if err := tm.KillSession(session.Name); err != nil {
		if verbose {
			fmt.Println("  Failed to kill session")
		}
		return fmt.Errorf("failed to kill session: %w", err)
	}

	if verbose {
		fmt.Println("  Session terminated.")
	} else {
		fmt.Printf("Killed: %s\n", session.Name)
	}

	return nil
}

// confirmPrompt asks the user for confirmation and returns true if confirmed.
func confirmPrompt(prompt string) bool {
	fmt.Print(prompt)
	var input string
	_, _ = fmt.Scanln(&input)
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}
