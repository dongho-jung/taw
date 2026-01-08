package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/tmux"
)

var attachCmd = &cobra.Command{
	Use:   "attach [session]",
	Short: "Attach to a running PAW session",
	Long: `Attach to a running PAW session from anywhere.

If no session name is provided, lists all available PAW sessions.
If multiple sessions exist and no name is given, prompts for selection.

Examples:
  paw attach           # List and select from running sessions
  paw attach myproject # Attach directly to 'myproject' session`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAttach,
}

// pawSession represents a running PAW session
type pawSession struct {
	Name       string
	SocketPath string
}

func runAttach(cmd *cobra.Command, args []string) error {
	sessions, err := findPawSessions()
	if err != nil {
		return fmt.Errorf("failed to find PAW sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No running PAW sessions found.")
		fmt.Println("\nStart a new session by running 'paw' in a project directory.")
		return nil
	}

	var targetSession pawSession

	if len(args) == 1 {
		// Direct attach to specified session
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
			if len(matches) == 1 {
				targetSession = matches[0]
			} else if len(matches) > 1 {
				fmt.Printf("Multiple sessions match '%s':\n", sessionName)
				for _, m := range matches {
					fmt.Printf("  - %s\n", m.Name)
				}
				return fmt.Errorf("please specify a unique session name")
			} else {
				fmt.Printf("Session '%s' not found.\n\n", sessionName)
				fmt.Println("Available sessions:")
				for _, s := range sessions {
					fmt.Printf("  - %s\n", s.Name)
				}
				return fmt.Errorf("session not found")
			}
		}
	} else if len(sessions) == 1 {
		// Only one session, attach directly
		targetSession = sessions[0]
		fmt.Printf("Attaching to '%s'...\n", targetSession.Name)
	} else {
		// Multiple sessions, prompt for selection
		fmt.Println("Running PAW sessions:")
		fmt.Println()
		for i, s := range sessions {
			fmt.Printf("  %d. %s\n", i+1, s.Name)
		}
		fmt.Println()
		fmt.Print("Select session [1]: ")

		var input string
		_, _ = fmt.Scanln(&input)
		input = strings.TrimSpace(input)

		// Default to first session
		idx := 0
		if input != "" {
			n, err := strconv.Atoi(input)
			if err != nil || n < 1 || n > len(sessions) {
				return fmt.Errorf("invalid selection: %s", input)
			}
			idx = n - 1
		}
		targetSession = sessions[idx]
	}

	return attachToPawSession(targetSession)
}

// findPawSessions discovers all running PAW tmux sessions
func findPawSessions() ([]pawSession, error) {
	// Find tmux socket directory
	// macOS: /private/tmp/tmux-$UID/
	// Linux: /tmp/tmux-$UID/
	uid := os.Getuid()
	socketDirs := []string{
		fmt.Sprintf("/private/tmp/tmux-%d", uid),
		fmt.Sprintf("/tmp/tmux-%d", uid),
	}

	var sessions []pawSession
	seen := make(map[string]bool)

	for _, dir := range socketDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Directory might not exist
		}

		for _, entry := range entries {
			name := entry.Name()
			// PAW sockets are named "paw-{sessionName}"
			if !strings.HasPrefix(name, constants.TmuxSocketPrefix) {
				continue
			}

			sessionName := strings.TrimPrefix(name, constants.TmuxSocketPrefix)
			if seen[sessionName] {
				continue
			}

			socketPath := filepath.Join(dir, name)

			// Verify the session is actually running by checking if tmux responds
			tm := tmux.New(sessionName)
			if tm.HasSession(sessionName) {
				sessions = append(sessions, pawSession{
					Name:       sessionName,
					SocketPath: socketPath,
				})
				seen[sessionName] = true
			}
		}
	}

	// Sort by name for consistent ordering
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})

	return sessions, nil
}

// attachToPawSession attaches to a PAW tmux session
func attachToPawSession(session pawSession) error {
	// Set terminal title before attaching
	setAttachTerminalTitle("[paw] " + session.Name)

	// Use tmux attach-session with the PAW socket
	tm := tmux.New(session.Name)
	return tm.AttachSession(session.Name)
}

// setAttachTerminalTitle sets the terminal (iTerm) tab title using OSC escape sequences.
// This works with iTerm2 and other terminals that support OSC 0/1/2.
func setAttachTerminalTitle(title string) {
	// OSC 0 sets both window and tab title
	// Format: ESC ] 0 ; <title> BEL
	fmt.Printf("\x1b]0;%s\x07", title)
}
