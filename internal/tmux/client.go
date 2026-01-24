// Package tmux provides an interface for interacting with tmux.
package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dongho-jung/paw/internal/constants"
	"github.com/dongho-jung/paw/internal/embed"
	"github.com/dongho-jung/paw/internal/logging"
)

// bufferPool reuses bytes.Buffer instances to reduce allocations in RunWithOutput.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// configPath holds the path to PAW's custom tmux configuration file.
var configPath string

func init() {
	// Write PAW-specific tmux config to temp directory
	content, err := embed.GetTmuxConfig()
	if err != nil {
		// Fallback: use /dev/null to ignore user's config
		configPath = "/dev/null"
		return
	}

	tmpDir := os.TempDir()
	configPath = filepath.Join(tmpDir, "paw-tmux.conf")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		// Fallback: use /dev/null to ignore user's config
		configPath = "/dev/null"
	}
}

// Client defines the interface for tmux operations.
type Client interface {
	// Session management
	HasSession(name string) bool
	NewSession(opts SessionOpts) error
	AttachSession(name string) error
	SwitchClient(target string) error
	KillSession(name string) error
	KillServer() error

	// Window management
	NewWindow(opts WindowOpts) (string, error)
	KillWindow(target string) error
	RenameWindow(target, name string) error
	ListWindows() ([]Window, error)
	SelectWindow(target string) error
	MoveWindow(source, target string) error

	// Pane operations
	SplitWindow(target string, horizontal bool, startDir string, command string) error
	SplitWindowPane(opts SplitOpts) (string, error)
	SelectPane(target string) error
	KillPane(target string) error
	HasPane(target string) bool
	SendKeys(target string, keys ...string) error
	SendKeysLiteral(target, text string) error
	CapturePane(target string, lines int) (string, error)
	ClearHistory(target string) error
	RespawnPane(target, startDir, command string) error
	WaitForPane(target string, maxWait time.Duration, minContentLen int) error
	GetPaneCommand(target string) (string, error)

	// Display popup
	DisplayPopup(opts PopupOpts, command string) error

	// Options
	SetOption(key, value string, global bool) error
	GetOption(key string) (string, error)
	SetMultipleOptions(options map[string]string) error
	SetEnv(key, value string) error

	// Keybindings
	Bind(opts BindOpts) error
	Unbind(key string) error

	// Utility
	Run(args ...string) error
	RunWithOutput(args ...string) (string, error)
	Display(format string) (string, error)
	DisplayMultiple(formats ...string) ([]string, error)

	// Notifications
	DisplayMessage(message string, durationMs int) error
}

// SessionOpts contains options for creating a new session.
type SessionOpts struct {
	Name       string
	StartDir   string
	WindowName string
	Detached   bool
	Width      int
	Height     int
	Command    string // Initial command to run in the session
}

// WindowOpts contains options for creating a new window.
type WindowOpts struct {
	Target     string // session:index or session name
	Name       string
	StartDir   string
	Command    string
	Detached   bool
	AfterIndex int // -1 means append
}

// PopupOpts contains options for display-popup.
type PopupOpts struct {
	Width       string
	Height      string
	Title       string
	Style       string
	Close       bool // -E flag: close on exit
	NoBorder    bool   // -B flag: no border
	BorderStyle string // -b flag: border lines
	Directory   string            // -d flag: working directory
	Env         map[string]string // -e flag: environment variables
}

// BindOpts contains options for key binding.
type BindOpts struct {
	Key      string
	Command  string
	NoPrefix bool // -n flag
	Table    string
}

// SplitOpts contains options for split-window with pane return.
type SplitOpts struct {
	Target     string // Target window or pane
	Horizontal bool   // -h for horizontal split, default is vertical (-v)
	Size       string // -l flag: percentage (e.g., "40%") or lines
	StartDir   string // -c flag: working directory
	Command    string // Command to run in the new pane
	Before     bool   // -b flag: create pane before target
	Full       bool   // -f flag: full-width/height split spanning entire window
}

// Window represents a tmux window.
type Window struct {
	ID     string
	Index  int
	Name   string
	Active bool
}

// tmuxClient implements the Client interface.
type tmuxClient struct {
	socket      string
	sessionName string
}

// New creates a new tmux client with the given socket name.
func New(sessionName string) Client {
	return &tmuxClient{
		socket:      constants.TmuxSocketPrefix + sessionName,
		sessionName: sessionName,
	}
}

func (c *tmuxClient) cmd(args ...string) *exec.Cmd {
	allArgs := append([]string{"-f", configPath, "-L", c.socket}, args...)
	return exec.Command("tmux", allArgs...)
}

func (c *tmuxClient) cmdContext(ctx context.Context, args ...string) *exec.Cmd {
	allArgs := append([]string{"-f", configPath, "-L", c.socket}, args...)
	return exec.CommandContext(ctx, "tmux", allArgs...)
}

func (c *tmuxClient) Run(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), constants.TmuxCommandTimeout)
	defer cancel()
	cmd := c.cmdContext(ctx, args...)
	return cmd.Run()
}

func (c *tmuxClient) RunWithOutput(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.TmuxCommandTimeout)
	defer cancel()
	cmd := c.cmdContext(ctx, args...)

	// Reuse buffers from pool to reduce allocations
	stdout := bufferPool.Get().(*bytes.Buffer)
	stderr := bufferPool.Get().(*bytes.Buffer)
	stdout.Reset()
	stderr.Reset()
	defer bufferPool.Put(stdout)
	defer bufferPool.Put(stderr)

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("tmux command timeout: %w", err)
		}
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Session management

func (c *tmuxClient) HasSession(name string) bool {
	err := c.Run("has-session", "-t", name)
	return err == nil
}

func (c *tmuxClient) NewSession(opts SessionOpts) error {
	args := []string{"new-session", "-s", opts.Name}

	if opts.Detached {
		args = append(args, "-d")
	}
	if opts.WindowName != "" {
		args = append(args, "-n", opts.WindowName)
	}
	if opts.StartDir != "" {
		args = append(args, "-c", opts.StartDir)
	}
	if opts.Width > 0 {
		args = append(args, "-x", fmt.Sprintf("%d", opts.Width))
	}
	if opts.Height > 0 {
		args = append(args, "-y", fmt.Sprintf("%d", opts.Height))
	}
	// Command must be the last argument
	if opts.Command != "" {
		args = append(args, opts.Command)
	}

	return c.Run(args...)
}

func (c *tmuxClient) AttachSession(name string) error {
	args := []string{"attach-session", "-t", name}
	cmd := c.cmd(args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *tmuxClient) KillSession(name string) error {
	return c.Run("kill-session", "-t", name)
}

func (c *tmuxClient) SwitchClient(target string) error {
	return c.Run("switch-client", "-t", target)
}

func (c *tmuxClient) KillServer() error {
	return c.Run("kill-server")
}

// Window management

func (c *tmuxClient) NewWindow(opts WindowOpts) (string, error) {
	logging.Trace("tmux.NewWindow: name=%s target=%s detached=%v", opts.Name, opts.Target, opts.Detached)
	args := []string{"new-window", "-P", "-F", "#{window_id}"}

	if opts.Target != "" {
		args = append(args, "-t", opts.Target)
	}
	if opts.Name != "" {
		args = append(args, "-n", opts.Name)
	}
	if opts.StartDir != "" {
		args = append(args, "-c", opts.StartDir)
	}
	if opts.Detached {
		args = append(args, "-d")
	}
	if opts.AfterIndex >= 0 {
		args = append(args, "-a", "-t", fmt.Sprintf(":%d", opts.AfterIndex))
	}
	if opts.Command != "" {
		args = append(args, opts.Command)
	}

	windowID, err := c.RunWithOutput(args...)
	if err != nil {
		logging.Trace("tmux.NewWindow: failed err=%v", err)
	} else {
		logging.Trace("tmux.NewWindow: created windowID=%s", windowID)
	}
	return windowID, err
}

func (c *tmuxClient) KillWindow(target string) error {
	logging.Trace("tmux.KillWindow: target=%s", target)
	return c.Run("kill-window", "-t", target)
}

func (c *tmuxClient) RenameWindow(target, name string) error {
	logging.Trace("tmux.RenameWindow: target=%s name=%s", target, name)
	err := c.Run("rename-window", "-t", target, name)
	if err != nil {
		logging.Trace("tmux.RenameWindow: failed err=%v", err)
	}
	return err
}

func (c *tmuxClient) ListWindows() ([]Window, error) {
	// Specify session target (-t) to ensure correct results when running from outside tmux
	output, err := c.RunWithOutput("list-windows", "-t", c.sessionName, "-F", "#{window_id}|#{window_index}|#{window_name}|#{window_active}")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	var windows []Window
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		var index int
		_, _ = fmt.Sscanf(parts[1], "%d", &index)

		windows = append(windows, Window{
			ID:     parts[0],
			Index:  index,
			Name:   parts[2],
			Active: parts[3] == "1",
		})
	}

	return windows, nil
}

func (c *tmuxClient) SelectWindow(target string) error {
	return c.Run("select-window", "-t", target)
}

func (c *tmuxClient) MoveWindow(source, target string) error {
	return c.Run("move-window", "-s", source, "-t", target)
}

// Pane operations

func (c *tmuxClient) SplitWindow(target string, horizontal bool, startDir string, command string) error {
	args := []string{"split-window", "-t", target}

	if horizontal {
		args = append(args, "-h")
	} else {
		args = append(args, "-v")
	}

	if startDir != "" {
		args = append(args, "-c", startDir)
	}

	if command != "" {
		args = append(args, command)
	}

	return c.Run(args...)
}

func (c *tmuxClient) SplitWindowPane(opts SplitOpts) (string, error) {
	args := []string{"split-window", "-P", "-F", "#{pane_id}"}

	if opts.Target != "" {
		args = append(args, "-t", opts.Target)
	}

	if opts.Horizontal {
		args = append(args, "-h")
	} else {
		args = append(args, "-v")
	}

	if opts.Size != "" {
		args = append(args, "-l", opts.Size)
	}

	if opts.StartDir != "" {
		args = append(args, "-c", opts.StartDir)
	}

	if opts.Before {
		args = append(args, "-b")
	}

	if opts.Full {
		args = append(args, "-f")
	}

	if opts.Command != "" {
		args = append(args, opts.Command)
	}

	return c.RunWithOutput(args...)
}

func (c *tmuxClient) SelectPane(target string) error {
	return c.Run("select-pane", "-t", target)
}

func (c *tmuxClient) KillPane(target string) error {
	return c.Run("kill-pane", "-t", target)
}

func (c *tmuxClient) HasPane(target string) bool {
	// Check if pane exists by trying to get its info
	_, err := c.RunWithOutput("display-message", "-t", target, "-p", "#{pane_id}")
	return err == nil
}

func (c *tmuxClient) GetPaneCommand(target string) (string, error) {
	return c.RunWithOutput("display-message", "-t", target, "-p", "#{pane_current_command}")
}

func (c *tmuxClient) SendKeys(target string, keys ...string) error {
	args := []string{"send-keys", "-t", target}
	args = append(args, keys...)
	return c.Run(args...)
}

func (c *tmuxClient) SendKeysLiteral(target, text string) error {
	return c.Run("send-keys", "-t", target, "-l", text)
}

func (c *tmuxClient) CapturePane(target string, lines int) (string, error) {
	args := []string{"capture-pane", "-t", target, "-p"}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	return c.RunWithOutput(args...)
}

func (c *tmuxClient) ClearHistory(target string) error {
	return c.Run("clear-history", "-t", target)
}

func (c *tmuxClient) RespawnPane(target, startDir, command string) error {
	args := []string{"respawn-pane", "-k", "-t", target}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	if command != "" {
		args = append(args, command)
	}
	return c.Run(args...)
}

// WaitForPane waits for a pane to become responsive and optionally have content.
// It polls the pane until it exists, is responsive, and has at least minContentLen characters.
// Set minContentLen to 0 to only verify the pane exists.
func (c *tmuxClient) WaitForPane(target string, maxWait time.Duration, minContentLen int) error {
	pollInterval := 100 * time.Millisecond
	maxAttempts := int(maxWait / pollInterval)
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for i := 0; i < maxAttempts; i++ {
		// Check if pane exists
		if !c.HasPane(target) {
			time.Sleep(pollInterval)
			continue
		}

		// If we don't need content, we're done
		if minContentLen == 0 {
			return nil
		}

		// Check if pane has enough content
		content, err := c.CapturePane(target, 10)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		if len(strings.TrimSpace(content)) >= minContentLen {
			return nil
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for pane %s after %v", target, maxWait)
}

// Display popup

func (c *tmuxClient) DisplayPopup(opts PopupOpts, command string) error {
	logging.Debug("-> tmux.DisplayPopup(title=%s, w=%s, h=%s, cmd=%s)", opts.Title, opts.Width, opts.Height, command)
	defer logging.Debug("<- tmux.DisplayPopup")

	args := []string{"display-popup"}

	if opts.NoBorder {
		args = append(args, "-B")
	}
	if opts.Close {
		args = append(args, "-E")
	}
	if opts.Width != "" {
		args = append(args, "-w", opts.Width)
	}
	if opts.Height != "" {
		args = append(args, "-h", opts.Height)
	}
	if opts.Directory != "" {
		args = append(args, "-d", opts.Directory)
	}
	if opts.Title != "" {
		args = append(args, "-T", opts.Title)
	}
	if opts.BorderStyle != "" {
		args = append(args, "-b", opts.BorderStyle)
	}
	if opts.Style != "" {
		args = append(args, "-s", opts.Style)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", k+"="+v)
	}
	if command != "" {
		args = append(args, command)
	}

	err := c.Run(args...)
	if err != nil {
		logging.Debug("tmux.DisplayPopup: failed: %v", err)
	}
	return err
}

// Options

func (c *tmuxClient) SetOption(key, value string, global bool) error {
	args := []string{"set-option"}
	if global {
		args = append(args, "-g")
	}
	args = append(args, key, value)
	return c.Run(args...)
}

func (c *tmuxClient) GetOption(key string) (string, error) {
	return c.RunWithOutput("show-option", "-gv", key)
}

func (c *tmuxClient) SetEnv(key, value string) error {
	return c.Run("set-environment", key, value)
}

// Keybindings

func (c *tmuxClient) Bind(opts BindOpts) error {
	args := []string{"bind"}

	if opts.NoPrefix {
		args = append(args, "-n")
	}
	if opts.Table != "" {
		args = append(args, "-T", opts.Table)
	}

	args = append(args, opts.Key, opts.Command)
	return c.Run(args...)
}

func (c *tmuxClient) Unbind(key string) error {
	return c.Run("unbind", key)
}

// Display

func (c *tmuxClient) Display(format string) (string, error) {
	return c.RunWithOutput("display-message", "-p", format)
}

// DisplayMultiple returns multiple tmux variables/options in a single command.
// formats is a list of tmux format strings (e.g., "#{window_id}", "#{@paw_option}").
// Returns the values in the same order as the formats.
// This is more efficient than calling Display multiple times.
func (c *tmuxClient) DisplayMultiple(formats ...string) ([]string, error) {
	if len(formats) == 0 {
		return nil, nil
	}

	// Use tab as delimiter since it's unlikely to appear in values
	const delimiter = "\t"
	combined := strings.Join(formats, delimiter)

	output, err := c.RunWithOutput("display-message", "-p", combined)
	if err != nil {
		return nil, err
	}

	return strings.Split(output, delimiter), nil
}

// SetMultipleOptions sets multiple global options in a single tmux command.
// This is more efficient than calling SetOption multiple times.
func (c *tmuxClient) SetMultipleOptions(options map[string]string) error {
	if len(options) == 0 {
		return nil
	}

	// Build a single command with all options
	args := make([]string, 0, len(options)*4)
	first := true
	for key, value := range options {
		if !first {
			args = append(args, ";", "set-option", "-g")
		} else {
			args = append(args, "set-option", "-g")
			first = false
		}
		args = append(args, key, value)
	}

	return c.Run(args...)
}

// DisplayMessage shows a message in the status bar for the specified duration.
func (c *tmuxClient) DisplayMessage(message string, durationMs int) error {
	logging.Trace("tmux.DisplayMessage: message=%s durationMs=%d", message, durationMs)
	return c.Run("display-message", "-d", fmt.Sprintf("%d", durationMs), message)
}
