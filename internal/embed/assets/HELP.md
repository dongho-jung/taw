# TAW (Tmux + Agent + Worktree)

Claude Code-based autonomous agent work environment

## Keyboard Shortcuts

### Mouse
  Click           Select pane
  Drag            Select text (copy mode)
  Scroll          Scroll pane
  Border drag     Resize pane

### Navigation
  ⌥ Tab       Move to next pane (cycle)
  ⌥ ←/→       Move to previous/next window

### Task Management
  ⌥ n         Toggle new window (task ↔ new window)
  ⌥ t         Toggle task list (view all active + completed tasks)
  ⌥ e         Complete task (commit → PR/merge → cleanup, follows ON_COMPLETE setting)
  ⌥ m         Batch merge completed tasks (merge + end all ✅ status tasks)
  ⌥ p         Toggle shell pane (bottom 40%, current worktree path)
  ⌥ l         Toggle log viewer (see Log Viewer section below)
  ⌥ u         Add quick task to queue (auto-processed after completion)

### Session
  ⌥ q         Exit session (detach)
  ⌥ /         Open/close this help (toggle)

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
  ├── .queue/                Quick task queue (add with ⌥u)
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

## Task List Viewer (⌥t)

View all active and completed tasks with preview panel.

### Navigation
  ↑/↓         Navigate tasks
  PgUp/PgDn   Scroll preview panel
  ⏎/Space     Focus selected task window
  q/Esc/⌥t    Close the task list

### Actions
  c           Cancel task (active tasks only)
  m           Merge task (triggers end-task flow)
  p           Push branch to remote
  r           Resume task (history items only, creates new task)

### Status Icons
  🤖  Working (agent active)
  💬  Waiting (needs user input)
  ✅  Done (ready to merge)
  📁  History (completed, from history)

## Log Viewer (⌥l)

  ↑/↓         Scroll vertically
  ←/→         Scroll horizontally (when word wrap is off)
  g           Jump to top
  G           Jump to bottom
  PgUp/PgDn   Page scroll
  s           Toggle tail mode (follow new logs)
  w           Toggle word wrap
  l           Cycle log level filter (L0+ → L1+ → ... → L5 only)
  q/Esc/⌥l    Close the log viewer

## Environment Variables (for agents)

  TASK_NAME     Task name
  TAW_DIR       .taw directory path
  PROJECT_DIR   Project root path
  WORKTREE_DIR  Worktree path
  WINDOW_ID     tmux window ID (for status updates)

---
Press ⌥/ or q to exit
