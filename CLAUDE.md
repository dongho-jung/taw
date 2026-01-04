# CLAUDE.md

## Build and install

```bash
# Build
make build

# Install to ~/.local/bin
make install

# Or install directly with go install
go install github.com/dongho-jung/taw@latest
```

> **Note (macOS)**: `make install` automatically runs `xattr -cr` and `codesign -fs -` to prevent the `zsh: killed` error.

## Dependency check

Run `taw check` to verify all dependencies are installed:

```bash
taw check
```

This checks:

| Dependency | Required | Description |
|------------|----------|-------------|
| tmux | ✅ | Terminal multiplexer for managing task windows |
| claude | ✅ | Claude Code CLI for AI-powered task execution |
| git | ❌ | Git for worktree mode (optional, but recommended) |
| gh | ❌ | GitHub CLI for PR creation (optional) |
| taw-notify.app | ❌ | macOS notification helper (optional) |
| notifications | ❌ | macOS notification permissions (optional) |
| sounds | ❌ | macOS system sounds for alerts (optional) |

## Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run tests with coverage report
go test ./... -cover

# Run tests for a specific package
go test ./internal/config -v

# Run a specific test
go test ./internal/config -run TestParseConfig -v

# Generate coverage HTML report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Test coverage by package

| Package | Coverage | Notes |
|---------|----------|-------|
| internal/config | ~93% | Fully tested |
| internal/logging | ~93% | Fully tested |
| internal/app | ~79% | Core functionality tested |
| internal/service | ~80% | History service with mocks |
| internal/embed | ~69% | Asset loading tested |
| internal/constants | ~57% | Pattern matching tested |
| internal/task | ~22% | Manager tests require mocks |
| internal/claude | ~15% | CLI operations need mocks |
| internal/git | ~7% | Requires git repository |
| internal/github | 0% | Requires `gh` CLI |
| internal/tmux | 0% | Requires tmux server |
| internal/notify | 0% | Platform-specific (macOS) |
| internal/tui | 0% | Interactive UI components |
| cmd/taw | ~4% | Cobra command handlers |
| cmd/taw-notify | 0% | macOS-only CGO binary |

## Directory structure

```
taw/                           # This repository
├── cmd/taw/                   # Go main package
│   ├── main.go                # Entry point and root command
│   ├── check.go               # Dependency check command (taw check)
│   ├── internal.go            # Internal command registration
│   ├── internal_create.go     # Task creation commands (toggleNew, newTask, spawnTask, handleTask)
│   ├── internal_lifecycle.go  # Task lifecycle commands (endTask, cancelTask, doneTask)
│   ├── internal_popup.go      # Popup/UI commands (toggleLog, toggleHelp, taskList)
│   ├── internal_utils.go      # Utility commands and helpers (ctrlC, renameWindow)
│   ├── keybindings.go         # Tmux keybinding definitions
│   └── wait.go                # Wait detection for user input prompts
├── cmd/taw-notify/            # Notification helper (macOS app bundle)
│   ├── main.go                # CGO code for UserNotifications (darwin only)
│   ├── doc.go                 # Stub for non-darwin platforms
│   └── Info.plist             # App bundle configuration
├── internal/                  # Go internal packages
│   ├── app/                   # Application context
│   ├── claude/                # Claude API client
│   ├── config/                # Configuration management
│   ├── constants/             # Constants and magic numbers
│   ├── embed/                 # Embedded assets
│   │   └── assets/            # Embedded files (compiled into binary)
│   │       ├── HELP.md        # Help text
│   │       ├── PROMPT.md      # System prompt (git mode)
│   │       ├── PROMPT-nogit.md # System prompt (non-git mode)
│   │       ├── tmux.conf      # Base tmux configuration
│   │       └── claude/        # Claude settings and slash commands
│   ├── git/                   # Git/worktree management
│   ├── github/                # GitHub API client
│   ├── logging/               # Logging (L0-L5 levels)
│   ├── notify/                # Desktop/audio/statusline notifications
│   ├── service/               # Business logic services (history, etc.)
│   ├── task/                  # Task management
│   ├── tmux/                  # Tmux client
│   └── tui/                   # Terminal UI (log viewer)
├── Makefile                   # Build script
└── go.mod                     # Go module file

{any-project}/                 # User project (git repo or plain directory)
└── .taw/                      # Created by taw
    ├── config                 # Project config (YAML, created during setup)
    ├── log                    # Consolidated logs (all scripts write here)
    ├── memory                 # Project memory (YAML, shared across tasks)
    ├── PROMPT.md              # Project prompt (user-customizable)
    ├── .is-git-repo           # Git mode marker (exists only in git repos)
    ├── .claude/               # Claude settings and slash commands (copied from embed)
    │   ├── settings.local.json
    │   └── commands/          # Slash commands (/commit, /test, /pr, /merge)
    ├── history/               # Task history directory
    │   └── YYMMDD_HHMMSS_task-name  # Task + summary + pane capture at task end
    └── agents/{task-name}/    # Per-task workspace
        ├── task               # Task contents
        ├── end-task           # Per-task end-task script (called for auto-merge)
        ├── origin             # -> Project root (symlink)
        ├── worktree/          # Git worktree (auto-created in git mode)
        ├── .tab-lock/         # Tab creation lock (atomic mkdir prevents races)
        │   └── window_id      # Tmux window ID (used in cleanup)
        ├── .session-started   # Session marker (for resume on reopen)
        ├── .status            # Task status (working/waiting/done, persisted for resume)
        └── .pr                # PR number (when created)
```

## Logging levels

TAW uses a 6-level logging system (L0-L5):

| Level | Name  | Description                                      | Output          |
|-------|-------|--------------------------------------------------|-----------------|
| L0    | Trace | Internal state tracking, loop iterations, dumps  | File only (debug mode) |
| L1    | Debug | Retry attempts, state changes, conditional paths | Stderr + file (debug mode) |
| L2    | Info  | Normal operation flow, task lifecycle events     | File only       |
| L3    | Warn  | Non-fatal issues requiring attention             | Stderr + file   |
| L4    | Error | Errors that affect functionality                 | Stderr + file   |
| L5    | Fatal | Critical errors requiring immediate attention    | Stderr + file   |

- Enable debug mode: `TAW_DEBUG=1 taw`
- Log file location: `.taw/log`
- View logs: Press `⌃O` to open the log viewer
- Filter levels in log viewer: Press `l` to cycle through L0+ → L1+ → ... → L5 only

## Notifications

TAW uses multiple notification channels to alert users:

| Event                    | Sound       | Desktop Notification | Slack/ntfy | Statusline Message |
|--------------------------|-------------|----------------------|------------|-------------------|
| Task created             | Glass       | Yes                  | Yes        | `🤖 Task started: {name}` |
| Task completed           | Hero        | Yes                  | Yes        | `✅ Task completed: {name}` |
| User input needed        | Funk        | Yes (with actions)   | Yes        | `💬 {name} needs input` |
| Cancel pending (⌃K)      | Tink        | -                    | -          | -                 |
| Error (merge failed etc) | Basso       | Yes                  | Yes        | `⚠️ Merge failed: {name} - manual resolution needed` |

- Sounds use macOS system sounds (`/System/Library/Sounds/`)
- Statusline messages display via `tmux display-message -d 2000`

### External Notification Channels

TAW supports sending notifications to external services in addition to macOS desktop notifications. Configure these in `.taw/config`:

```yaml
# Slack notifications via incoming webhook
notifications:
  slack:
    webhook: https://hooks.slack.com/services/YOUR/WEBHOOK/URL

# ntfy.sh notifications (or self-hosted ntfy server)
notifications:
  ntfy:
    topic: your-topic-name
    server: https://ntfy.sh  # Optional, defaults to https://ntfy.sh
```

**Slack setup**:
1. Create an [Incoming Webhook](https://api.slack.com/messaging/webhooks) in your Slack workspace
2. Copy the webhook URL to `.taw/config`

**ntfy setup**:
1. Choose a topic name (e.g., `taw-notifications`)
2. Subscribe to the topic in the [ntfy app](https://ntfy.sh/) or web interface
3. Add the topic to `.taw/config`
4. (Optional) For self-hosted ntfy, specify the `server` URL

### Notification Action Buttons

When user input is needed and the prompt has 2-5 simple choices, TAW shows a banner notification with action buttons:

- **Title**: The task name
- **Body**: The question from the prompt
- **Icon**: Uses the app bundle icon (shown on the left side only)
- **Actions**: Up to 5 buttons matching the prompt options

If the user clicks an action button, the response is sent directly to the agent without opening a popup. If the notification times out (30s) or is dismissed, the fallback popup is shown.

**Requirements** (macOS desktop notifications):
- macOS 10.15+
- Notification permissions granted for `TAW Notify` app
- `taw-notify.app` installed to `~/.local/share/taw/`

## Working rules

### Verification required

- **Always run code after changes to confirm it works.**
- Test before saying "done."
- A successful build is not enough—verify the feature actually works.
- If interactive testing is impossible (e.g., terminal attach), create a test script to validate.

### Test after every change

- **Always run `go test ./...` after making any code changes.**
- If tests fail, fix the test code or implementation before proceeding.
- Update existing tests when behavior changes.
- Add new tests for new functionality when appropriate.

### Keep docs in sync

- Reflect any changes you make in docs such as README or CLAUDE.md.

### English only

- **All code, comments, and documentation must be written in English.**
- This includes: variable names, function names, commit messages, PR descriptions, inline comments, and documentation files.
