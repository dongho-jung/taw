# CLAUDE.md

## Build and install

```bash
# Build
make build

# Install to ~/.local/bin
make install

# Uninstall from ~/.local/bin
make uninstall

# Or install directly with go install
go install github.com/dongho-jung/paw@latest
```

> **Note (macOS)**: `make install` automatically runs `xattr -cr` and `codesign -fs -` to prevent the `zsh: killed` error.

## Dependency check

Run `paw check` to verify all dependencies are installed:

```bash
paw check
```

This checks:

| Dependency | Required | Description |
|------------|----------|-------------|
| tmux | ✅ | Terminal multiplexer for managing task windows |
| claude | ✅ | Claude Code CLI for AI-powered task execution |
| git | ❌ | Git for worktree mode (optional, but recommended) |
| gh | ❌ | GitHub CLI for PR creation (optional) |
| paw-notify.app | ❌ | macOS notification helper (optional) |
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
| internal/tui/textarea/internal/* | 100.0% | Memoization and runeutil fully covered |
| internal/constants | 96.1% | Name/window helpers covered |
| internal/config | 86.0% | Config parsing/saving, templates, and hook formatting |
| internal/app | 79.0% | App context and environment handling |
| internal/logging | 76.0% | Core logging behavior covered |
| internal/embed | 75.0% | Embedded asset loading |
| internal/git | 46.7% | Git operations tested with isolated repos |
| internal/service | 33.5% | History and task discovery services |
| internal/notify | 32.1% | Slack/ntfy helpers covered; macOS paths limited |
| internal/claude | 31.3% | CLI client command construction tested |
| internal/task | 24.5% | Manager logic partially covered |
| internal/tui | 10.7% | Interactive UI components partially tested |
| internal/github | 6.1% | gh CLI command construction only |
| cmd/paw | 4.4% | Cobra command handlers |
| internal/tmux | 3.1% | Struct defaults and constants only |
| cmd/paw-notify | 0% | macOS-only CGO binary |

## Directory structure

```
paw/                           # This repository
├── cmd/paw/                   # Go main package
│   ├── main.go                # Entry point and root command
│   ├── check.go               # Dependency check command (paw check)
│   ├── internal.go            # Internal command registration
│   ├── internal_create.go     # Task creation commands (toggleNew, newTask, spawnTask, handleTask)
│   ├── internal_lifecycle.go  # Task lifecycle commands (endTask, cancelTask, doneTask)
│   ├── internal_popup.go      # Popup/UI commands (toggleLog, toggleHelp, toggleTemplate)
│   ├── internal_sync.go       # Sync commands (syncWithMain, syncTask, toggleBranch)
│   ├── internal_stop_hook.go  # Claude stop hook handling (task status classification)
│   ├── internal_user_prompt_hook.go # User prompt submission hook
│   ├── internal_utils.go      # Utility commands and helpers (ctrlC, renameWindow)
│   ├── keybindings.go         # Tmux keybinding definitions
│   └── wait.go                # Wait detection for user input prompts
├── cmd/paw-notify/            # Notification helper (macOS app bundle)
│   ├── main.go                # CGO code for UserNotifications (darwin only)
│   ├── doc.go                 # Stub for non-darwin platforms
│   ├── icon.png               # App bundle icon
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
│   │       └── claude/        # Claude settings
│   ├── git/                   # Git/worktree management
│   ├── github/                # GitHub API client
│   ├── logging/               # Logging (L0-L5 levels)
│   ├── notify/                # Desktop/audio/statusline notifications
│   ├── service/               # Business logic services (history, etc.)
│   ├── task/                  # Task management
│   ├── tmux/                  # Tmux client
│   └── tui/                   # Terminal UI components
│       ├── gitviewer.go       # Git viewer (status, log, graph modes)
│       ├── helpviewer.go      # Help viewer
│       ├── logviewer.go       # Log viewer with filtering
│       └── textarea/          # Custom textarea component (fork of bubbles)
├── Makefile                   # Build script
└── go.mod                     # Go module file

{any-project}/                 # User project (git repo or plain directory)
└── .paw/                      # Created by paw
    ├── config                 # Project config (YAML, created during setup)
    ├── log                    # Consolidated logs (all scripts write here)
    ├── memory                 # Project memory (YAML, shared across tasks)
    ├── input-history          # Task input history (JSON, for Ctrl+R search)
    ├── templates.yaml         # Task templates (YAML, for ⌃T template selector)
    ├── PROMPT.md              # Project prompt (user-customizable)
    ├── bin                    # Symlink to current paw binary (updated on attach)
    ├── .version               # PAW version (for upgrade detection on attach)
    ├── .is-git-repo           # Git mode marker (exists only in git repos)
    ├── .claude/               # Claude settings (copied from embed)
    │   └── settings.local.json
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
        ├── .options.json      # Task options (model, ultrathink, depends_on, worktree_hook)
        └── .pr                # PR number (when created)
```

## Logging levels

PAW uses a 6-level logging system (L0-L5):

| Level | Name  | Description                                      | Output          |
|-------|-------|--------------------------------------------------|-----------------|
| L0    | Trace | Internal state tracking, loop iterations, dumps  | File only (debug mode) |
| L1    | Debug | Retry attempts, state changes, conditional paths | Stderr + file (debug mode) |
| L2    | Info  | Normal operation flow, task lifecycle events     | File only       |
| L3    | Warn  | Non-fatal issues requiring attention             | Stderr + file   |
| L4    | Error | Errors that affect functionality                 | Stderr + file   |
| L5    | Fatal | Critical errors requiring immediate attention    | Stderr + file   |

- Enable debug mode: `PAW_DEBUG=1 paw`
- Log file location: `.paw/log`
- View logs: Press `⌃O` to open the log viewer
- Filter levels in log viewer: Press `l` to cycle through L0+ → L1+ → ... → L5 only

## Notifications

PAW uses multiple notification channels to alert users:

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

PAW supports sending notifications to external services in addition to macOS desktop notifications. Configure these in `.paw/config`:

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
2. Copy the webhook URL to `.paw/config`

**ntfy setup**:
1. Choose a topic name (e.g., `paw-notifications`)
2. Subscribe to the topic in the [ntfy app](https://ntfy.sh/) or web interface
3. Add the topic to `.paw/config`
4. (Optional) For self-hosted ntfy, specify the `server` URL

### Notification Action Buttons

When user input is needed and the prompt has 2-5 simple choices, PAW shows a banner notification with action buttons:

- **Title**: The task name
- **Body**: The question from the prompt
- **Icon**: Uses the app bundle icon (shown on the left side only)
- **Actions**: Up to 5 buttons matching the prompt options

If the user clicks an action button, the response is sent directly to the agent without opening a popup. If the notification times out (30s) or is dismissed, the fallback popup is shown.

**Requirements** (macOS desktop notifications):
- macOS 10.15+
- Notification permissions granted for `PAW Notify` app
- `paw-notify.app` installed to `~/.local/share/paw/`

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
