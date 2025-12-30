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
│   │   └── assets/            # HELP.md (help text)
│   ├── git/                   # Git/worktree management
│   ├── github/                # GitHub API client
│   ├── logging/               # Logging (L0-L5 levels)
│   ├── notify/                # Desktop/audio/statusline notifications
│   ├── task/                  # Task management
│   ├── tmux/                  # Tmux client
│   └── tui/                   # Terminal UI (log viewer)
├── _taw/                      # Resource files (symlinked into projects)
│   ├── PROMPT.md              # System prompt (git mode)
│   ├── PROMPT-nogit.md        # System prompt (non-git mode)
│   └── claude/commands/       # Slash commands (/commit, /test, /pr, /merge)
├── Makefile                   # Build script
└── go.mod                     # Go module file

{any-project}/                 # User project (git repo or plain directory)
└── .taw/                      # Created by taw
    ├── config                 # Project config (YAML, created during setup)
    ├── log                    # Consolidated logs (all scripts write here)
    ├── PROMPT.md              # Project prompt
    ├── .global-prompt         # -> Global prompt (symlink, varies by git mode)
    ├── .is-git-repo           # Git mode marker (exists only in git repos)
    ├── .claude                # -> _taw/claude (symlink)
    ├── .queue/                # Quick task queue (add with ⌥ u)
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
- View logs: Press `⌥ l` to open the log viewer
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
