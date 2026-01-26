# PAW (Parallel AI Workers)

Claude Code-based autonomous agent work environment

## Keyboard Shortcuts

### Mouse
  Click           Select pane
  Click task      Jump to task (in kanban, works across sessions)
  Drag            Select text (copy mode)
  Scroll          Scroll pane
  Border drag     Resize pane

### Navigation
  ⌥Tab        Cycle panes / Cycle options (in new task window)
  ⌥←/→        Move to previous/next window
  ⌥↑/↓        Swap window left/right (reorder)
  ⌃J          Switch project (jump to other PAW sessions)

### Task Commands
  ⌃N          New task
  ⌃K          New shell window
  ⌃R          Search task history (in new task window)
  ⌃T          Template picker (in new task window)
  ⌃F          Finish task (action picker: merge/merge+push/PR/drop or done)
  ⌃P          Command palette (fuzzy search commands)
  ⌃Q          Quit paw

### Toggle Panels
  ⌃O          Toggle logs (show log viewer)
  ⌃G          Toggle git viewer
  ⌃B          Toggle bottom (shell pane)
  ⌃/          Toggle help
  ⌃Y          Edit prompts (open prompt picker)

## Directory Structure

  .paw/
  ├── config                 Project configuration file
  ├── PROMPT.md              Project-specific agent instructions
  ├── log                    Unified log file
  ├── input-history          Task input history (for ⌃R search)
  ├── input-templates        Task templates (for ⌃T picker)
  ├── window-map.json        Window token to task mapping
  ├── prompts/               Custom prompt templates (⌃Y to edit)
  │   ├── system.md          System prompt override
  │   ├── task-name.md       Task name generation rules
  │   ├── merge-conflict.md  Merge conflict resolution
  │   ├── pr-description.md  PR description template
  │   └── commit-message.md  Commit message template
  ├── history/               Completed task history
  │   └── YYMMDD_HHMMSS_name Task content + work capture
  └── agents/{task-name}/
      ├── task               Task content
      ├── origin/            Project root (symlink)
      └── {project-name}/        git worktree (auto-created)

## Window Status Icons

  ⭐️  New task input window
  🤖  Agent working
  💬  Waiting for user input / needs attention
  ✅  Task completed

## Log Viewer (⌃O)

  ↑/↓         Scroll vertically
  ←/→         Scroll horizontally (when word wrap is off)
  g           Jump to top
  G           Jump to bottom
  PgUp/PgDn   Page scroll
  /           Start search
  n/N         Next/previous match
  s           Toggle tail mode (follow new logs)
  w           Toggle word wrap
  Tab         Cycle log level filter (L0+ → L1+ → ... → L5 only)
  ⌃O/q/Esc    Close the log viewer

## Git Viewer (⌃G)

### Modes (Tab to cycle, 1-4 for quick switch)
  1: STATUS     git status
  2: LOG        git log
  3: LOG --all  git log --all --decorate --oneline --graph
  4: DIFF       git diff main...HEAD

### Navigation
  ↑/↓/j/k     Scroll vertically
  ←/→/h/l     Scroll horizontally (when word wrap is off)
  g/G         Jump to top/bottom
  PgUp/PgDn   Page scroll
  ⌃U/⌃D       Half-page scroll
  w           Toggle word wrap

### Mode Switching
  Tab         Cycle modes (STATUS → LOG → LOG --all → DIFF)
  1/2/3/4     Jump to specific mode
  s/L/a/d     Switch to STATUS/LOG/LOG --all/DIFF

### Search
  /           Start search
  n/N         Next/previous match
  Esc         Clear search

### Other
  ⌃C          Copy selection (drag to select)
  ⌃G/q/Esc    Close the git viewer

## CLI Commands (outside tmux)

  paw logs --since 1h --task my-task
  paw history --task my-task --since 2d --query "error"
  paw history show 1
  paw check --fix

## Task Options (⌥Tab in new task window)

Configure per-task settings before submission:

  Model         Claude model (opus/sonnet/haiku)
  Depends on    Run after another task (success/failure/always)
  Branch name   Custom branch name (git mode only)
  Worktree hook Override project hook for this task

## Environment Variables (for agents)

  TASK_NAME     Task identifier (branch name)
  PAW_DIR       Workspace directory path
  PROJECT_DIR   Project root path
  WORKTREE_DIR  Worktree path (git mode only)
  WINDOW_ID     tmux window ID
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

  ↑/↓/j/k     Scroll vertically
  ←/→/h/l     Scroll horizontally
  g/G         Jump to top/bottom
  PgUp/PgDn   Page scroll
  ⌃U/⌃D       Half-page scroll
  ⌃//q/Esc    Close the help viewer

## Project Picker (⌃J)

Switch between running PAW project sessions.

### Navigation
  ↑/↓         Navigate projects
  Space/Enter Switch to selected project
  Esc/⌃J      Cancel

### Features
  - Fuzzy search by project name
  - Shows all running PAW sessions except current
  - Switches to selected session (keeps active window)

## Prompt Editor (⌃Y)

Edit prompts used by PAW. Opens selected prompt in $EDITOR.

### Available Prompts
  Task Prompt           Project instructions (.paw/PROMPT.md)
  System Prompt         Global system prompt for agents
  Task Name Rules       Rules for generating task names
  Merge Conflict        Prompt for resolving merge conflicts
  PR Description        Template for PR title and body
  Commit Message        Template for auto-commit messages

### Navigation
  ↑/↓         Navigate prompts
  Enter       Open in $EDITOR
  Esc/⌃Y      Cancel

### Prompt Locations
  .paw/PROMPT.md              Task prompt (project-specific)
  .paw/prompts/system.md      System prompt override
  .paw/prompts/task-name.md   Task name generation rules
  .paw/prompts/merge-conflict.md  Merge conflict resolution
  .paw/prompts/pr-description.md  PR description template
  .paw/prompts/commit-message.md  Commit message template
