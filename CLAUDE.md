# CLAUDE.md

## Build and install

```bash
# Build
make build

# Install to ~/.local/bin
make install

# Or install directly with go install
go install github.com/donghojung/taw@latest
```

> **Note (macOS)**: `make install` automatically runs `xattr -cr` and `codesign -fs -` to prevent the `zsh: killed` error.

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
| internal/constants | 100% | Fully tested |
| internal/config | ~93% | Fully tested |
| internal/logging | ~93% | Fully tested |
| internal/app | ~80% | Fully tested |
| internal/embed | ~79% | Fully tested |
| internal/task | ~35% | Manager tests require mocks |
| internal/claude | ~17% | CLI operations need mocks |
| internal/git | ~8% | Requires git repository |
| internal/github | 0% | Requires `gh` CLI |
| internal/tmux | 0% | Requires tmux server |
| internal/notify | 0% | Platform-specific (macOS) |
| internal/tui | 0% | Interactive UI components |

## Directory structure

```
taw/                           # This repository
├── cmd/taw/                   # Go main package
├── internal/                  # Go internal packages
│   ├── app/                   # Application context
│   ├── claude/                # Claude API client
│   ├── config/                # Configuration management
│   ├── constants/             # Constants
│   ├── embed/                 # Embedded assets
│   │   └── assets/            # Embedded files (compiled into binary)
│   │       ├── HELP.md        # Help text
│   │       ├── PROMPT.md      # System prompt (git mode)
│   │       ├── PROMPT-nogit.md # System prompt (non-git mode)
│   │       └── claude/        # Claude settings and slash commands
│   ├── git/                   # Git/worktree management
│   ├── github/                # GitHub API client
│   ├── logging/               # Logging (L0-L5 levels)
│   ├── notify/                # Desktop/audio/statusline notifications
│   ├── task/                  # Task management
│   ├── tmux/                  # Tmux client
│   └── tui/                   # Terminal UI (log viewer)
├── Makefile                   # Build script
└── go.mod                     # Go module file

{any-project}/                 # User project (git repo or plain directory)
└── .taw/                      # Created by taw
    ├── config                 # Project config (YAML, created during setup)
    ├── log                    # Consolidated logs (all scripts write here)
    ├── PROMPT.md              # Project prompt (user-customizable)
    ├── .is-git-repo           # Git mode marker (exists only in git repos)
    ├── .claude/               # Claude settings and slash commands (copied from embed)
    │   ├── settings.local.json
    │   └── commands/          # Slash commands (/commit, /test, /pr, /merge)
    ├── .queue/                # Quick task queue (add with ⌃R → add-queue)
    │   └── 001.task           # Pending tasks (processed in order)
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
- View logs: Press `⌃R` → `show-log` to open the log viewer
- Filter levels in log viewer: Press `l` to cycle through L0+ → L1+ → ... → L5 only

## Notifications

TAW uses multiple notification channels to alert users (macOS only):

| Event                    | Sound       | Desktop Notification | Statusline Message |
|--------------------------|-------------|----------------------|-------------------|
| Task created             | Glass       | -                    | `🤖 Task started: {name}` |
| Task completed           | Hero        | -                    | `✅ Task completed: {name}` |
| User input needed        | Funk        | Yes                  | `💬 {name} needs input` |
| Error (merge failed etc) | Basso       | -                    | `⚠️ Merge failed: {name}` |

- Sounds use macOS system sounds (`/System/Library/Sounds/`)
- Statusline messages display via `tmux display-message -d 2000`

## Working rules

### Verification required

- **Always run code after changes to confirm it works.**
- Test before saying "done."
- A successful build is not enough—verify the feature actually works.
- If interactive testing is impossible (e.g., terminal attach), create a test script to validate.

### Keep docs in sync

- Reflect any changes you make in docs such as README or CLAUDE.md.
