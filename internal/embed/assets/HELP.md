# TAW (Tmux + Agent + Worktree)

Claude Code-based autonomous agent work environment

## Keyboard Shortcuts

### Mouse
  Click           Select pane
  Drag            Select text (copy mode)
  Scroll          Scroll pane
  Border drag     Resize pane

### Navigation
  ⌥Tab        Move to next pane (cycle)
  ⌥←/→        Move to previous/next window

### Task Commands
  ⌃N          New task
  ⌃K          Cancel task (double-press within 2s)
  ⌃F          Finish task (complete and cleanup)
  ⌃Q          Quit taw

### Toggle Panels
  ⌃T          Toggle tasks (show task list)
  ⌃O          Toggle logs (show log viewer)
  ⌃G          Toggle git status
  ⌃B          Toggle bottom (shell pane)
  ⌃/          Toggle help
  ⌃,          Toggle setup (rerun setup wizard)

## Slash Commands (for agents)

  /commit     Smart commit (auto-generate message from diff analysis)
  /test       Auto-detect and run project tests
  /pr         Auto-create PR and open browser
  /merge      Merge worktree branch to project branch

## Directory Structure

  .taw/
  ├── config                 Project configuration file
  ├── PROMPT.md              Project-specific agent instructions
  ├── memory                 Shared project memory (YAML)
  ├── log                    Unified log file
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

## Environment Variables (for agents)

  TASK_NAME     Task identifier (branch name)
  TAW_DIR       .taw directory path
  PROJECT_DIR   Project root path
  WORKTREE_DIR  Worktree path (git mode only)
  WINDOW_ID     tmux window ID
  ON_COMPLETE   Completion mode (confirm/auto-commit/auto-merge/auto-pr)
  TAW_HOME      TAW installation directory
  TAW_BIN       TAW binary path
  SESSION_NAME  tmux session name

---
Press q to exit
