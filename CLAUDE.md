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
├── cmd/paw/                   # Go main package
│   ├── main.go                # Entry point and root command
│   ├── session.go             # Session management (attach, create)
│   ├── setup.go               # Setup wizard and initialization
│   ├── tmux_config.go         # Tmux configuration generation
│   ├── check.go               # Dependency check command (paw check)
│   ├── check_project.go       # Project-level checks
│   ├── attach.go              # Attach command (paw attach)
│   ├── history.go             # History command (paw history)
│   ├── logs.go                # Logs command (paw logs)
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
│   │   ├── manager*.go        # Task manager (core, find, worktree operations)
│   │   ├── task.go            # Task struct and basic operations
│   │   ├── workspace.go       # Workspace management
│   │   └── recovery.go        # Task recovery logic
│   ├── tmux/                  # Tmux client
│   └── tui/                   # Terminal UI components
│       ├── taskinput*.go      # Task input UI (main, helpers, mouse, options)
│       ├── taskopts.go        # Task options panel
│       ├── gitviewer.go       # Git viewer (status, log, graph modes)
│       ├── helpviewer.go      # Help viewer
│       ├── logviewer.go       # Log viewer with filtering
│       ├── cmdpalette.go      # Command palette (⌃P)
│       ├── settings.go        # Settings UI (global/project config editor)
│       └── textarea/          # Custom textarea component (fork of bubbles)
├── Makefile                   # Build script
└── go.mod                     # Go module file

{any-project}/                 # User project (git repo or plain directory)
└── .paw/                      # Created by paw
    ├── config                 # Project config (YAML, created during setup)
    ├── log                    # Consolidated logs (all scripts write here)
    ├── memory                 # Project memory (YAML, shared across tasks)
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
        ├── end-task           # Per-task end-task script (called for auto-merge)
        ├── origin             # -> Project root (symlink)
        ├── worktree/          # Git worktree (auto-created in git mode)
        ├── .tab-lock/         # Tab creation lock (atomic mkdir prevents races)
        │   └── window_id      # Tmux window ID (used in cleanup)
        ├── .session-started   # Session marker (for resume on reopen)
        ├── .status            # Task status (working/waiting/done, persisted for resume)
        ├── .options.json      # Task options (model, ultrathink, depends_on, pre_worktree_hook)
        └── .pr                # PR number (when created)

$HOME/.paw/                        # Global PAW config (shared across all projects)
└── config                         # Global config (default settings for all projects)
```

### Global vs Project Settings

PAW supports both global and project-level settings:

- **Global settings** (`$HOME/.paw/config`): Default settings applied to all projects
- **Project settings** (`.paw/config`): Project-specific overrides

In the Settings UI (⌃P → Settings), press `⌥Tab` to toggle between Global and Project views.

Project settings can **inherit** from global settings. For fields that support inheritance (Work Mode, On Complete, Self Improve):
- Select "inherit" as an option using ←/→ keys
- When "inherit" is selected, the global value is shown in parentheses (e.g., `< inherit > (worktree)`)
- Other fields (Theme, Non-Git Workspace, Verify settings) do not support inheritance

Example project config with inherit:
```yaml
work_mode: worktree
on_complete: confirm
theme: auto
self_improve: false

# Inherit settings from global config
inherit:
  work_mode: true     # Use global work_mode
  on_complete: false  # Use project-specific value
  theme: true         # Use global theme
  self_improve: true  # Use global self_improve
```

### Self-Improve Feature

When `self_improve` is enabled (default: off), PAW analyzes completed tasks to identify:
- **Mistakes made**: Things the agent got wrong or had to retry
- **Knowledge gaps**: Information the agent didn't know and had to discover
- **Best practices**: Patterns that worked well and should be documented

At task finish, PAW:
1. Uses Claude Opus with ultrathink to analyze the session history
2. Generates learnings and appends them to `CLAUDE.md` in the project root
3. Commits the changes to the default branch (main/master)

This allows the agent to continuously improve its understanding of the project over time.

To enable in `.paw/config`:
```yaml
self_improve: true
```

### Theme Settings

PAW supports 12 theme presets for tmux status bar, window tabs, and pane borders. The theme automatically adapts to your terminal's light/dark mode, or you can set a specific preset.

**Available presets:**

| Dark Themes | Light Themes |
|-------------|--------------|
| `dark` (default dark) | `light` (default light) |
| `dark-blue` | `light-blue` |
| `dark-green` | `light-green` |
| `dark-purple` | `light-purple` |
| `dark-warm` | `light-warm` |
| `dark-mono` | `light-mono` |

**Configuration:**
```yaml
# Auto-detect based on terminal (default)
theme: auto

# Or set a specific preset
theme: light-blue
```

**Theme detection methods (in order):**
1. `COLORFGBG` environment variable (if set by terminal)
2. OSC 11 query to terminal (background color detection)
3. Fallback to dark mode

When attaching to an existing session from a different terminal, PAW re-detects the theme and updates tmux colors automatically.

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
| Cancel pending (⌃K)      | Tink        | -                    | -                 |
| Error (merge failed etc) | Basso       | Yes                  | `⚠️ Merge failed: {name} - manual resolution needed` |

### Terminal-based notifications (cross-platform)

Desktop notifications use terminal escape sequences (OSC) for cross-platform support:

| Terminal | Protocol | Format |
|----------|----------|--------|
| iTerm2 | OSC 9 | `ESC]9;message BEL` |
| Kitty | OSC 99 | `ESC]99;i=1:d=0:p=2;body BEL` |
| WezTerm | OSC 777 | `ESC]777;notify;title;body BEL` |
| Ghostty | OSC 777 | `ESC]777;notify;title;body BEL` |
| rxvt | OSC 777 | `ESC]777;notify;title;body BEL` |
| Others | OSC 9 + Bell | Fallback to OSC 9 and terminal bell |

- When running inside tmux, OSC sequences are wrapped for passthrough
- Terminal bell (`\a`) is always sent as additional fallback
- Sounds use macOS system sounds on darwin, terminal bell on other platforms
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

- Reflect any changes you make in docs such as README or CLAUDE.md.

### English only

- **All code, comments, and documentation must be written in English.**
- This includes: variable names, function names, commit messages, PR descriptions, inline comments, and documentation files.
