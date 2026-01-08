# PAW (Parallel AI Workers)

Claude Code-based autonomous agent work environment

## Keyboard Shortcuts

### Mouse
  Click           Select pane
  Drag            Select text (copy mode)
  Scroll          Scroll pane
  Border drag     Resize pane

### Navigation
  ⌥Tab        Cycle panes / Cycle options (in new task window)
  ⌥←/→        Move to previous/next window

### Task Commands
  ⌃N          New task
  ⌃K          Cancel task (double-press)
  ⌃F          Finish task (double-press, complete and cleanup)
  ⌃↑          Toggle branch (task ↔ main)
  ⌃↓          Sync from main (rebase)
  ⌃P          Command palette (fuzzy search commands)
  ⌃Q          Quit paw

### Toggle Panels
  ⌃T          Toggle tasks (show task list)
  ⌃O          Toggle logs (show log viewer)
  ⌃G          Toggle git status
  ⌃B          Toggle bottom (shell pane)
  ⌃/          Toggle help

## Directory Structure

  .paw/
  ├── config                 Project configuration file
  ├── PROMPT.md              Project-specific agent instructions
  ├── memory                 Shared project memory (YAML)
  ├── log                    Unified log file
  ├── window-map.json        Window token to task mapping
  ├── history/               Completed task history
  │   └── YYMMDD_HHMMSS_name Task content + work capture
  └── agents/{task-name}/
      ├── task               Task content
      ├── origin/            Project root (symlink)
      └── worktree/          git worktree (auto-created)

## Window Status Icons

  ⭐️  New task input window
  🤖  Agent working
  💬  Waiting for user input
  ✅  Task completed
  ⚠️  Warning (merge failed, needs manual resolution)

## Task List Viewer (⌃T)

View all active and completed tasks with preview panel.

### Navigation
  ↑/↓/j/k     Navigate tasks
  PgUp/PgDn   Scroll preview panel
  Ctrl+U/D    Scroll preview panel (vim-style)
  ⏎/Space     Focus selected task window
  q/Esc       Close the task list

### Actions
  c           Cancel task (active tasks only)
  m           Merge task (triggers finish-task flow)
  p           Push branch to remote
  r           Resume task (history/cancelled items, creates new task)

### Status Icons
  🤖  Working (agent active)
  💬  Waiting (needs user input)
  ✅  Done (ready to merge)
  📁  History (completed, from history)
  ❌  Cancelled (from history, can resume)

## Log Viewer (⌃O)

  ↑/↓         Scroll vertically
  ←/→         Scroll horizontally (when word wrap is off)
  g           Jump to top
  G           Jump to bottom
  PgUp/PgDn   Page scroll
  s           Toggle tail mode (follow new logs)
  w           Toggle word wrap
  l           Cycle log level filter (L0+ → L1+ → ... → L5 only)
  ⌃O/q/Esc    Close the log viewer

## CLI Commands (outside tmux)

  paw logs --since 1h --task my-task
  paw history --task my-task --since 2d --query "error"
  paw history show 1
  paw check --fix

## Task Options (⌥Tab in new task window)

Configure per-task settings before submission:

  Model         Claude model (opus/sonnet/haiku)
  Ultrathink    Extended thinking mode (on/off)
  Depends on    Run after another task (success/failure/always)
  Worktree hook Override project hook for this task

## Environment Variables (for agents)

  TASK_NAME     Task identifier (branch name)
  PAW_DIR       .paw directory path
  PROJECT_DIR   Project root path
  WORKTREE_DIR  Worktree path (git mode) or workspace copy (non-git copy mode)
  WINDOW_ID     tmux window ID
  ON_COMPLETE   Completion mode (confirm/auto-merge/auto-pr)
  PAW_HOME      PAW installation directory
  PAW_BIN       PAW binary path
  SESSION_NAME  tmux session name

## Command Palette (⌃P)

Fuzzy-searchable command palette for quick access to commands.

### Navigation
  ↑/↓/⌃k/⌃j  Navigate commands
  ⏎           Execute selected command
  Esc/⌃P      Close palette

### Available Commands
  Restore Panes  Restore missing panes in current task window

## Help Viewer (⌃/)

  ↑↓j/k       Scroll vertically
  ←/→h/l      Scroll horizontally
  g/G         Jump to top/bottom
  PgUp/PgDn   Page scroll
  ⌃U/D        Half-page scroll
  ⌃//q/Esc    Close the help viewer
