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
| sounds | ❌ | macOS system sounds for alerts (optional) |

## Release

- GoReleaser config: `.github/goreleaser.yaml`
- Tag and push `v*` to trigger the release workflow

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
| internal/config | 86.0% | Config parsing/saving and hook formatting |
| internal/app | 79.0% | App context and environment handling |
| internal/logging | 76.0% | Core logging behavior covered |
| internal/embed | 75.0% | Embedded asset loading |
| internal/git | 46.7% | Git operations tested with isolated repos |
| internal/service | 33.5% | History and task discovery services |
| internal/notify | 32.1% | macOS desktop notification helpers |
| internal/claude | 31.3% | CLI client command construction tested |
| internal/task | 24.5% | Manager logic partially covered |
| internal/tui | 10.7% | Interactive UI components partially tested |
| internal/github | 6.1% | gh CLI command construction only |
| cmd/paw | 4.4% | Cobra command handlers |
| internal/tmux | 3.1% | Struct defaults and constants only |

## Directory structure

```
paw/                           # This repository
├── .github/                   # GitHub metadata
│   └── goreleaser.yaml        # GoReleaser configuration
├── cmd/paw/                   # Go main package
│   ├── main.go                # Entry point and root command
│   ├── session.go             # Session management (attach, create)
│   ├── tmux_config.go         # Tmux configuration generation
│   ├── tmux_theme.go          # Tmux theme/color management
│   ├── check.go               # Dependency check command (paw check)
│   ├── check_project.go       # Project-level checks
│   ├── attach.go              # Attach command (paw attach)
│   ├── history.go             # History command (paw history)
│   ├── logs.go                # Logs command (paw logs)
│   ├── kill.go                # Kill session command (paw kill)
│   ├── location.go            # Location command (paw location)
│   ├── internal.go            # Internal command registration
│   ├── internal_create*.go    # Task creation (toggleNew, newTask, spawnTask, handleTask)
│   ├── internal_lifecycle*.go # Task lifecycle (endTask, cancelTask, merge, helpers)
│   ├── internal_popup*.go     # Popup/UI (toggleLog, toggleHelp, shell)
│   ├── internal_sync.go       # Sync commands (syncWithMain)
│   ├── internal_stop_hook.go  # Claude stop hook handling (task status classification)
│   ├── internal_user_prompt_hook.go # User prompt submission hook
│   ├── internal_utils.go      # Utility commands and helpers (ctrlC, renameWindow)
│   ├── keybindings.go         # Tmux keybinding definitions
│   ├── timeparse.go           # Time parsing utilities for logs/history
│   ├── wait*.go               # Wait detection for user input prompts
│   └── window_map.go          # Window ID to task name mapping
├── internal/                  # Go internal packages
│   ├── app/                   # Application context
│   ├── claude/                # Claude API client
│   ├── config/                # Configuration management
│   ├── constants/             # Constants and magic numbers
│   ├── embed/                 # Embedded assets
│   │   └── assets/            # Embedded files (compiled into binary)
│   │       ├── HELP.md        # Help text for users
│   │       ├── HELP-FOR-PAW.md # Help text for PAW agent instructions
│   │       ├── PROMPT.md      # System prompt (git mode)
│   │       ├── PROMPT-nogit.md # System prompt (non-git mode)
│   │       ├── tmux.conf      # Base tmux configuration
│   │       └── claude/        # Claude settings
│   │           ├── CLAUDE.md  # Default CLAUDE.md for new workspaces
│   │           └── settings.local.json # Claude Code local settings
│   ├── git/                   # Git/worktree management
│   ├── github/                # GitHub API client
│   ├── logging/               # Logging (L0-L5 levels)
│   ├── notify/                # Desktop/audio/statusline notifications
│   ├── service/               # Business logic services (history, etc.)
│   ├── task/                  # Task management
│   │   ├── manager*.go        # Task manager (core, find, worktree operations)
│   │   ├── task.go            # Task struct and basic operations
│   │   ├── workspace.go       # Workspace management
│   │   └── recovery.go        # Task recovery logic
│   ├── tmux/                  # Tmux client
│   └── tui/                   # Terminal UI components
│       ├── taskinput*.go      # Task input UI (main, helpers, mouse, options)
│       ├── taskopts.go        # Task options panel
│       ├── gitviewer.go       # Git viewer (status, log, graph modes)
│       ├── diffviewer.go      # Diff viewer for PR/merge operations
│       ├── helpviewer.go      # Help viewer
│       ├── logviewer.go       # Log viewer with filtering
│       ├── cmdpalette.go      # Command palette (⌃P)
│       ├── finishpicker.go    # Finish action picker (merge/pr/keep/drop)
│       ├── endtask.go         # End task confirmation UI
│       ├── kanban.go          # Kanban board view for tasks
│       ├── projectpicker.go   # Project session picker (⌃J)
│       ├── branchmenu.go      # Branch selection menu
│       ├── inputhistory.go    # Task input history (⌃R search)
│       ├── recover.go         # Task recovery UI
│       ├── spinner.go         # Loading spinner component
│       ├── theme.go           # Theme/color definitions
│       ├── tips.go            # UI tips and hints
│       ├── scrollbar.go       # Scrollbar component
│       └── textarea/          # Custom textarea component (fork of bubbles)
├── Makefile                   # Build script
└── go.mod                     # Go module file

{any-project}/                 # User project (git repo or plain directory)
└── .paw/                      # Created by paw
    ├── config                 # Project config (YAML, created on first run)
    ├── log                    # Consolidated logs (all scripts write here)
    ├── input-history          # Task input history (JSON, for Ctrl+R search)
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
        ├── log                # Task-specific progress log (for agent progress updates)
        ├── end-task           # Per-task end-task script (called for auto-merge)
        ├── origin             # -> Project root (symlink)
        ├── {project-name}-{hash}/    # Git worktree (auto-created in git mode)
        ├── .tab-lock/         # Tab creation lock (atomic mkdir prevents races)
        │   └── window_id      # Tmux window ID (used in cleanup)
        ├── .session-started   # Session marker (for resume on reopen)
        ├── .status            # Task status (working/waiting/done, persisted for resume)
        ├── .status-signal     # Temp file for Claude to signal status (deleted after read)
        ├── .options.json      # Task options (model, ultrathink, depends_on, pre_worktree_hook)
        └── .pr                # PR number (when created)

$HOME/.local/share/paw/            # Global PAW data (auto mode for git projects)
└── workspaces/                    # Workspaces for all projects
    └── {project-name}-{hash}/     # Per-project workspace (same structure as .paw above)
```

PAW uses claude-mem for shared memory across tasks/workspaces. Memory is stored
globally in `~/.claude-mem` and scoped by project directory name; in worktree
mode PAW appends a short hash suffix to avoid collisions.

### Workspace Location

PAW uses auto mode for workspaces:
- **Git repositories**: global workspace under `~/.local/share/paw/workspaces/{project-id}/`
- **Non-git directories**: local `.paw/` inside the project

A local `.paw/` directory always takes priority if it already exists.

To force a local workspace for a git repo, run `paw --local`.

### Theme

PAW always auto-detects your terminal's light/dark background and applies the
corresponding tmux colors.

**Detection methods (in order):**
1. `COLORFGBG` environment variable (if set by terminal)
2. OSC 11 query to terminal (background color detection)
3. Fallback to dark mode

When attaching to an existing session from a different terminal, PAW re-detects
the theme and updates tmux colors automatically.

## Logging levels

PAW uses a 6-level logging system (L0-L5):

| Level | Name  | Color      | Description                                      | Output          |
|-------|-------|------------|--------------------------------------------------|-----------------|
| L0    | Trace | Gray       | Internal state tracking, loop iterations, dumps  | File only (debug mode) |
| L1    | Debug | Blue       | Retry attempts, state changes, conditional paths | Stderr + file (debug mode) |
| L2    | Info  | Green      | Normal operation flow, task lifecycle events     | File only       |
| L3    | Warn  | Orange     | Non-fatal issues requiring attention             | Stderr + file   |
| L4    | Error | Red        | Errors that affect functionality                 | Stderr + file   |
| L5    | Fatal | Magenta    | Critical errors requiring immediate attention    | Stderr + file   |

- Enable debug mode: `PAW_DEBUG=1 paw`
- Log file location: `.paw/log`
- View logs: Press `⌃O` to open the log viewer
- Filter levels in log viewer: Press `Tab` to cycle through L0+ → L1+ → ... → L5 only
- Log level tags (`[L0]` through `[L5]`) are color-coded for quick identification

## Notifications

PAW uses desktop notifications and sounds to alert users:

| Event                    | Sound       | Desktop Notification | Statusline Message |
|--------------------------|-------------|----------------------|-------------------|
| Task created             | Glass       | Yes                  | `🤖 Task started: {name}` |
| Task completed           | Hero        | Yes                  | `✅ Task completed: {name}` |
| User input needed        | Funk        | Yes                  | `💬 {name} needs input` |
| Error (merge failed etc) | Basso       | Yes                  | `⚠️ Merge failed: {name} - manual resolution needed` |

### Terminal-based notifications (cross-platform)

Desktop notifications use terminal escape sequences (OSC) for cross-platform support:

| Terminal | Protocol | Features |
|----------|----------|----------|
| iTerm2 | OSC 9 | Basic notifications |
| Kitty | OSC 99 | Rich notifications with urgency, icons, click-to-focus |
| WezTerm | OSC 777 | Title + body support |
| Ghostty | OSC 777 | Title + body support |
| Windows Terminal | OSC 9 | Basic notifications |
| VSCode Terminal | OSC 9 | Forwarded through SSH connections |
| foot | OSC 99 | Rich notifications with urgency, icons |
| Contour | OSC 99 | Rich notifications with urgency, icons |
| rxvt-unicode | OSC 777 | Title + body support |
| Linux (fallback) | notify-send | Uses libnotify when available |
| Others | OSC 9 + Bell | Fallback to OSC 9 and terminal bell |

#### OSC Protocol Details

- **OSC 9** (iTerm2 style): `ESC]9;message BEL` - Simple message-only format
- **OSC 99** (Kitty style): `ESC]99;metadata;payload BEL` - Rich format with:
  - Urgency levels (low, normal, critical)
  - Standard icons (info, warning, error, question, help)
  - Occasion control (show only when unfocused)
  - Activation actions (focus window on click)
- **OSC 777** (rxvt style): `ESC]777;notify;title;body BEL` - Title + body support

#### Notification Options

PAW supports notification urgency levels:
- **Low**: For non-critical notifications
- **Normal**: Standard notifications (default)
- **Critical**: Important notifications that should not be missed

Standard icon support (for terminals that support OSC 99):
- `info`, `warning`, `error`, `question`, `help`

#### Platform Notes

- **macOS**: Uses system sounds via `afplay`, OSC sequences for notifications
- **Linux**: Falls back to `notify-send` when terminal doesn't support OSC notifications
- **Windows**: Uses OSC 9 via Windows Terminal
- **tmux**: OSC sequences are automatically wrapped for passthrough (`ESC P tmux;...`)
- Terminal bell (`\a`) is always sent as additional fallback
- Statusline messages display via `tmux display-message -d 2000`

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

Update documentation for ALL affected files (not just one):

| Change Type | Files to Update |
|-------------|-----------------|
| New file added | CLAUDE.md (directory structure section) |
| Config option added/removed | README.md (config table + example) AND CLAUDE.md |
| CLI command changed | README.md AND HELP.md |
| Keyboard shortcut changed | README.md AND HELP.md |
| Feature added/removed | README.md (feature description) |

**Common mistakes to avoid:**
- ❌ Updating CLAUDE.md but forgetting README.md (or vice versa)
- ❌ Adding new files without updating directory structure
- ❌ Removing features from code but leaving them in docs

### Always use AskUserQuestion

- **When asking the user a question, always use the AskUserQuestion tool.**
- Do not ask questions in plain text without the tool—the user may not see it or be able to respond properly.
- AskUserQuestion ensures proper notification and response handling in PAW.

### English only

- **All code, comments, and documentation must be written in English.**
- This includes: variable names, function names, commit messages, PR descriptions, inline comments, and documentation files.
