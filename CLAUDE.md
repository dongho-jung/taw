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
- Run `scripts/release.sh vX.Y.Z` (or `make release VERSION=vX.Y.Z`) to update the version map, commit, and tag
- Push the release commit and `vX.Y.Z` tag to trigger the release workflow

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

Note: `internal/fileutil/fileutil_test.go` covers atomic writes and corrupt backup handling.

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
│   ├── setup.go               # Clean/clean-all commands
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
│   ├── internal_create*.go    # Task creation (toggleNew, newTask, spawnTask, handleTask, deps)
│   ├── internal_lifecycle*.go # Task lifecycle (endTask, cancelTask, merge, helpers, misc)
│   ├── internal_popup*.go     # Popup/UI (toggleLog, toggleHelp, shell, prompts, misc, viewers)
│   ├── internal_pr_popup.go   # PR popup TUI command
│   ├── internal_sync.go       # Sync commands (syncWithMain)
│   ├── internal_stop_hook.go  # Claude stop hook handling (task status classification)
│   ├── internal_user_prompt_hook.go # User prompt submission hook
│   ├── internal_utils.go      # Utility commands and helpers (ctrlC, renameWindow)
│   ├── keybindings.go         # Tmux keybinding definitions
│   ├── timeparse.go           # Time parsing utilities for logs/history
│   ├── version_map.go         # Release-generated version-to-commit map
│   ├── version_test.go        # Build info version/commit fallback tests
│   ├── wait*.go               # Wait detection for user input prompts
│   ├── watch_pr.go            # PR merge watcher (auto-cleanup on merge)
│   └── window_map.go          # Window ID to task name mapping
├── internal/                  # Go internal packages
│   ├── app/                   # Application context
│   ├── claude/                # Claude API client
│   ├── config/                # Configuration management
│   ├── constants/             # Constants and magic numbers
│   ├── fileutil/              # File safety helpers
│   ├── embed/                 # Embedded assets
│   │   └── assets/            # Embedded files (compiled into binary)
│   │       ├── HELP.md        # Help text for users
│   │       ├── HELP-FOR-PAW.md # Help text for PAW agent instructions
│   │       ├── PROMPT.md      # System prompt (git mode)
│   │       ├── PROMPT-nogit.md # System prompt (non-git mode)
│   │       ├── tmux.conf      # Base tmux configuration
│   │       ├── hooks/         # Git hooks
│   │       │   └── pre-commit # Pre-commit hook (safety net for .claude)
│   │       ├── prompts/       # Default prompt templates
│   │       │   ├── task-name.md      # Task name generation rules
│   │       │   ├── merge-conflict.md # Merge conflict resolution prompt
│   │       │   ├── pr-description.md # PR description template
│   │       │   └── commit-message.md # Commit message template
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
│       ├── taskinput*.go      # Task input UI (main, helpers, mouse, options, templates)
│       ├── taskopts.go        # Task options panel
│       ├── tasknameinput.go   # Task name input with validation
│       ├── taskviewer.go      # Task content viewer
│       ├── gitviewer.go       # Git viewer (status, log, graph modes)
│       ├── diffviewer.go      # Diff viewer for PR/merge operations
│       ├── helpviewer.go      # Help viewer
│       ├── logviewer.go       # Log viewer with filtering
│       ├── cmdpalette.go      # Command palette (⌃P)
│       ├── finishpicker.go    # Finish action picker (merge/pr/keep/drop)
│       ├── endtask.go         # End task confirmation UI
│       ├── kanban.go          # Kanban board view for tasks
│       ├── projectpicker.go   # Project session picker (⌃J)
│       ├── promptpicker.go    # Prompt editor picker (⌃Y)
│       ├── templatepicker.go  # Template picker (⌃T)
│       ├── prpopup.go         # PR info popup
│       ├── branchmenu.go      # Branch selection menu
│       ├── inputhistory.go    # Task input history (⌃R search)
│       ├── recover.go         # Task recovery UI
│       ├── spinner.go         # Loading spinner component
│       ├── theme.go           # Theme/color definitions
│       ├── tips.go            # UI tips and hints
│       ├── scrollbar.go       # Scrollbar component
│       ├── textinput_helpers.go # Text input helper functions (padding, etc.)
│       └── textarea/          # Custom textarea component (fork of bubbles)
├── Makefile                   # Build script
├── scripts/                   # Release/dev scripts
│   ├── release.sh             # Release helper (version map + tag)
│   └── update-version-map.sh  # Generate version-to-commit map
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
    ├── prompts/               # Custom prompt templates (⌃Y to edit)
    │   ├── system.md          # System prompt override
    │   ├── task-name.md       # Task name generation rules
    │   ├── merge-conflict.md  # Merge conflict resolution prompt
    │   ├── pr-description.md  # PR description template
    │   └── commit-message.md  # Commit message template
    ├── history/               # Task history directory
    │   └── YYMMDD_HHMMSS_task-name  # Task + summary + pane capture at task end
    └── agents/{task-name}/    # Per-task workspace
        ├── task               # Task contents
        ├── log                # Task-specific progress log (for agent progress updates)
        ├── start-agent        # Agent start script (avoids shell escaping issues)
        ├── end-task           # Per-task end-task script (called for auto-merge)
        ├── origin             # -> Project root (symlink)
        ├── worktree/          # Git worktree directory (auto-created in git mode)
        ├── .tab-lock/         # Tab creation lock (atomic mkdir prevents races)
        │   └── window_id      # Tmux window ID (used in cleanup)
        ├── .session-started   # Session marker (for resume on reopen)
        ├── .status            # Task status (working/waiting/done, persisted for resume)
        ├── .status-signal     # Temp file for Claude to signal status (deleted after read)
        ├── .system-prompt     # Generated system prompt for the agent
        ├── .user-prompt       # Generated user prompt for the agent
        ├── .options.json      # Task options (model, depends_on, pre_worktree_hook)
        └── .pr                # PR number (when created)

$HOME/.local/share/paw/            # Global PAW data (auto mode for git projects)
└── workspaces/                    # Workspaces for all projects
    └── {project-name}-{hash}/     # Per-project workspace (PAW uses hash for uniqueness)
```

### Workspace Location

PAW uses auto mode for workspaces:
- **Git repositories**: global workspace under `~/.local/share/paw/workspaces/{project-id}/`
- **Non-git directories**: local `.paw/` inside the project

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

## Performance optimizations

PAW uses several techniques to ensure smooth, responsive UI:

### Caching strategies

| Component | Cache Type | Purpose |
|-----------|------------|---------|
| theme.go | Padding string cache | Pre-computed space strings for common widths |
| LogViewer | Display lines cache | Avoids re-filtering/wrapping on every render |
| LogViewer | Search query cache | Pre-lowercased query + O(1) match lookup map |
| LogViewer | Lowercase lines cache | Avoids repeated ToLower on visible lines |
| LogViewer | Style cache | Reuses lipgloss.Style objects across renders |
| LogViewer | Log level tags | Pre-computed `[L0]`-`[L5]` strings |
| DiffViewer | Search query cache | Pre-lowercased query + O(1) match lookup map |
| DiffViewer | Style cache | Reuses lipgloss.Style objects across renders |
| DiffViewer | Display lines cache | Avoids re-wrapping lines on every render |
| GitViewer | Search query cache | Pre-lowercased query + O(1) match lookup map |
| GitViewer | Style cache | Reuses lipgloss.Style objects across renders |
| GitViewer | Display lines cache | Avoids re-wrapping lines on every render |
| Scrollbar | Style cache | Package-level styles for dark/light themes |
| TaskManager | Truncated name cache | Avoids directory scans for window→task mapping |
| KanbanView | Task count cache | Updated only on Refresh(), not every render |
| KanbanView | Render cache | Full render output cached when state unchanged |
| constants | CamelCase cache | `sync.Map` caches `ToCamelCase()` results for repeated conversions |

### Memory allocation optimizations

| Location | Optimization |
|----------|--------------|
| `internal/tmux/client.go` | `sync.Pool` for `bytes.Buffer` reuse |
| `internal/git/client.go` | `sync.Pool` for `bytes.Buffer` reuse |
| `internal/claude/client.go` | `sync.Pool` for `bytes.Buffer` reuse |
| `internal/github/client.go` | `sync.Pool` for `bytes.Buffer` reuse |
| `internal/logging/logger.go` | `sync.Pool` for caller frame `[]uintptr` reuse |
| `internal/tui/theme.go` | `getPadding()` cache with RWMutex for common widths |
| `internal/tui/logviewer.go` | Pre-allocated filtered lines with capacity |
| `internal/tui/logviewer.go` | Cached lipgloss.Style objects (updated on theme change) |
| `internal/tui/logviewer.go` | Pre-computed log level tags array |
| `internal/tui/diffviewer.go` | Cached lipgloss.Style objects (updated on theme change) |
| `internal/tui/gitviewer.go` | Cached lipgloss.Style objects (updated on theme change) |
| `internal/tui/scrollbar.go` | Package-level style cache for dark/light themes |
| `internal/tui/scrollbar.go` | `strings.Builder.Grow()` pre-allocation for scrollbar rendering |
| `internal/tui/kanban.go` | Pre-allocated slices with estimated capacity |
| `internal/tui/kanban.go` | Package-level `hintPatterns` and indent string |
| `internal/tui/kanban.go` | `containsLower()` for allocation-free hint detection |
| `internal/tui/kanban.go` | Cached lipgloss.Style objects for header/task/action |
| `internal/tui/taskinput.go` | Cached options panel styles (7 styles per theme) |
| `internal/tui/taskinput_templates.go` | Pre-computed rune slice for placeholder token |
| `internal/tui/taskinput_options.go` | Pre-computed padded label constants |
| `internal/tui/spinner.go` | String concatenation instead of `fmt.Sprintf` |
| `internal/tui/diffviewer.go` | Pre-allocated search match positions slice |
| `internal/tui/diffviewer.go` | `strings.Builder.Grow()` pre-allocation in View() |
| `internal/tui/diffviewer.go` | Pre-computed status bar hint widths |
| `internal/tui/diffviewer.go` | `strconv.Itoa` + string concat for status bar (no fmt.Sprintf) |
| `internal/tui/gitviewer.go` | Pre-allocated search match positions slice |
| `internal/tui/gitviewer.go` | `strings.Builder.Grow()` pre-allocation in View() |
| `internal/tui/gitviewer.go` | Pre-computed status bar hint widths |
| `internal/tui/gitviewer.go` | `strconv.Itoa` + string concat for status bar (no fmt.Sprintf) |
| `internal/tui/logviewer.go` | `strings.Builder.Grow()` pre-allocation in View() |
| `internal/tui/logviewer.go` | Pre-computed status bar hint widths |
| `internal/tui/logviewer.go` | `strconv.Itoa` + string concat for status bar (no fmt.Sprintf) |
| `internal/tui/helpviewer.go` | `strings.Builder.Grow()` pre-allocation in View() |
| `internal/tui/helpviewer.go` | Pre-computed status bar hint widths |
| `internal/tui/helpviewer.go` | `strconv.Itoa` + string concat for status bar (no fmt.Sprintf) |
| `internal/tui/taskviewer.go` | `strings.Builder.Grow()` pre-allocation in View() |
| `internal/tui/inputhistory.go` | `strings.Builder.Grow()` pre-allocation in View() |
| `internal/tui/projectpicker.go` | `strings.Builder.Grow()` pre-allocation in View() |
| `internal/tui/promptpicker.go` | `strings.Builder.Grow()` pre-allocation in View() |
| `internal/tui/kanban.go` | Cached separator string for column headers |
| `internal/tui/taskinput.go` | Cached View() styles (help, version, warning, tip, cancel hint) |
| `internal/tui/taskinput.go` | Pre-rendered help text and cached width (avoids lipgloss.Width per render) |
| `internal/tui/textinput_helpers.go` | Uses `getPadding()` cache for padding strings |
| `internal/tui/taskinput_options.go` | Pre-allocated lines and parts slices |
| `internal/tui/cmdpalette.go` | Pre-allocated searchables slice |
| `internal/tui/projectpicker.go` | Pre-allocated searchables slice |
| `internal/tui/taskopts.go` | Pre-allocated modelParts slice |
| `internal/config/taskopts.go` | Package-level validModels slice (avoids allocation per call) |
| `internal/constants/constants.go` | Package-level `commitTypeKeywords` slice (avoids allocation per call) |
| `internal/tui/textarea/textarea.go` | Padding string cache (`getPadding()`) for View() rendering |
| `internal/tui/textarea/textarea.go` | Rune space slice cache (`repeatSpaces()`) with copy-on-read |
| `internal/tui/textarea/textarea.go` | `strconv.FormatUint` for line hash (no fmt.Sprintf) |
| `internal/tui/textarea/textarea.go` | `getPadding()` cache for prompt/line number formatting (no fmt.Sprintf) |
| `internal/service/taskdiscovery.go` | Single `strings.Split()` shared across functions |
| `internal/service/taskdiscovery.go` | Ring buffer for last-N lines extraction (avoids slice growth) |
| `internal/tui/theme.go` | `buildSearchBar()` helper for search bar padding (no fmt.Sprintf) |
| `internal/tui/gitviewer.go` | Uses `buildSearchBar()` for search mode status bar |
| `internal/tui/diffviewer.go` | Uses `buildSearchBar()` for search mode status bar |
| `internal/tui/logviewer.go` | Uses `buildSearchBar()` for search mode status bar |
| `internal/logging/logger.go` | Pre-computed `levelStrings` array for log level formatting |
| `internal/tui/gitviewer.go` | Display lines cache for word wrap (invalidated on width/content change) |
| `internal/tui/diffviewer.go` | Display lines cache for word wrap (invalidated on width/content change) |
| `internal/embed/embed.go` | Uses `bytes.Contains` instead of custom byte search functions |
| `internal/constants/constants.go` | `sync.Map` cache for `ToCamelCase()` results |
| `internal/tui/textarea/internal/memoization/memoization.go` | `strconv.FormatUint`/`strconv.Itoa` for hash formatting (no fmt.Sprintf) |
| `internal/tmux/client.go` | `strconv.Itoa` for popup dimensions (no fmt.Sprintf) |
| `internal/github/client.go` | `strconv.Itoa` for PR number formatting (no fmt.Sprintf) |
| `internal/app/app.go` | `strconv.Itoa` for log config env vars (no fmt.Sprintf) |
| `internal/task/task.go` | `strconv.Itoa` for PR number file (no fmt.Sprintf) |
| `internal/service/taskdiscovery.go` | `strconv.Itoa` for UID fallback (no fmt.Sprintf) |
| `cmd/paw/*` | String concatenation for simple message formatting (no fmt.Sprintf) |
| `cmd/paw/*`, `internal/*` | `errors.New` for static error messages (no fmt.Errorf) |
| `internal/task/manager.go` | Pre-allocated tasks slice in `ListTasks()` |
| `internal/service/history.go` | Pre-allocated files slice in `ListHistoryFiles()` |
| `cmd/paw/internal_create.go` | Pre-allocated names slice in `getActiveTaskNames()` |
| `cmd/paw/wait_prompt.go` | Pre-lowercased UI hints slice (avoids ToLower in loop) |
| `internal/tui/endtask.go` | Pre-allocated steps slice with capacity based on git mode |
| `internal/tui/branchmenu.go` | `strings.Builder` in View() instead of `fmt.Sprintf` |

### Hot path optimizations

- **TaskDiscoveryService**: Lines split once and passed to all extraction functions
- **Viewers (Log/Diff/Git)**: Shared `isMatchLine()` and `isCurrentMatchLine()` helpers with bounds checking
- **LogViewer**: Pre-cached lowercase lines avoid repeated `strings.ToLower()` in render
- **LogViewer**: Pre-computed level tags/filters avoid `fmt.Sprintf` per line
- **KanbanView**: `isCacheValid()` uses cached task count, not recalculated
- **KanbanView**: `containsLower()` does case-insensitive match without allocation
- **Render functions**: Early returns when cache is valid
- **Search highlighting**: Pre-count matches before allocating positions slice
- **Style caching**: lipgloss.Style objects cached and reused (invalidated only on theme change)
- **Padding strings**: `getPadding()` returns cached strings for common widths (80, 100, 120, etc.)
- **String formatting**: String concatenation used instead of `fmt.Sprintf` in hot View() methods
- **Search bar**: `buildSearchBar()` helper avoids `fmt.Sprintf("/%-*s")` on every render
- **Logging**: Pre-computed level strings (`L0`-`L5`) avoid `fmt.Sprintf` on every log line
- **GitViewer/DiffViewer**: `getDisplayLines()` cache avoids re-wrapping on every render call
- **ToCamelCase**: `sync.Map` cache avoids repeated string conversions in kanban rendering

### Best practices for new code

1. **Pre-allocate slices** when size is known or estimable: `make([]T, 0, capacity)`
2. **Use sync.Pool** for frequently allocated buffers in hot paths
3. **Cache computed values** that don't change between renders
4. **Avoid string operations in loops** - use `strings.Builder` or pre-compute
5. **Move constants to package level** - avoid allocations per function call
6. **Cache lipgloss.Style objects** - create once, reuse across renders (invalidate on theme change)
7. **Pre-lowercase strings** once during data load, not on every render
8. **Use `getPadding(n)`** instead of `strings.Repeat(" ", n)` for padding strings
9. **Prefer string concatenation** over `fmt.Sprintf` in View() methods for simple cases
10. **Pre-compute static format strings** as package-level variables (e.g., log level tags)
11. **Pre-allocate strings.Builder** with `sb.Grow(estimatedSize)` when output size is predictable
12. **Pre-compute string widths** for static UI hints (avoid `ansi.StringWidth()` on each render)
13. **Cache repeated string computations** like separators that depend on width
14. **Add compile-time interface checks** for implementations: `var _ Interface = (*concreteType)(nil)`
15. **Use constants for timeouts** - extract repeated duration values to `internal/constants/constants.go`

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
