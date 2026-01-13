# PAW Help for Agents

This guide helps you (the agent) perform PAW-related operations when users ask about hooks, settings, logs, or other PAW features.

## Configuration Files

PAW has two configuration levels:

| Level | Path | Scope |
|-------|------|-------|
| **Global** | `$HOME/.paw/config` | Default settings for all projects |
| **Project** | `.paw/config` | Project-specific overrides |

### When to Ask User (Use AskUserQuestion)

**Always ask** when modifying settings that could apply at either level:
- Hooks (pre_worktree_hook, post_task_hook, etc.)

Example AskUserQuestion:
```
questions:
  - question: "Apply this setting at which level?"
    header: "Scope"
    options:
      - label: "Project (Recommended)"
        description: "Only affects this project"
      - label: "Global"
        description: "Applies to all projects"
```

## Hooks Configuration

### Available Hooks

| Hook | When it runs | Common use case |
|------|--------------|-----------------|
| `pre_worktree_hook` | Before task worktree is created | N/A |
| `pre_task_hook` | Before agent starts | Setup commands |
| `post_task_hook` | After task completes | Cleanup commands |
| `pre_merge_hook` | Before merging to main | Run tests, build |
| `post_merge_hook` | After successful merge | Deploy, notify |

### Setting Up Hooks

**Single-line hook:**
```yaml
pre_task_hook: npm install
```

**Multi-line hook** (use `|` for literal block):
```yaml
pre_worktree_hook: |
  npm install
  npm run build
```

### Example: User requests "run npm install and build when worktree is created"

1. Ask user: project or global level?
2. Edit the appropriate config file:

For project level (`.paw/config`):
```yaml
pre_worktree_hook: |
  npm install
  npm run build
```

For global level (`$HOME/.paw/config`):
```yaml
pre_worktree_hook: |
  npm install
  npm run build
```

## Work Mode

PAW automatically uses worktree mode for git repositories. Each task gets its own git worktree, providing isolation between tasks.

## Viewing Logs

### Interactive (within PAW session)
- Press `⌃O` to open the log viewer
- Use `Tab` to cycle log level filter (L0+ → L1+ → ... → L5 only)
- Press `g` to jump to top, `G` to jump to bottom
- Press `s` to toggle tail mode (follow new logs)

### CLI (outside PAW)
```bash
# View recent logs
paw logs

# View logs from last hour
paw logs --since 1h

# View logs for specific task
paw logs --task my-task

# View logs from last 2 days for a task
paw logs --since 2d --task my-task
```

### Log File Location
- `.paw/log` - unified log file for all tasks

## Settings UI

Users can configure settings via the command palette:
1. Press `⌃P` to open command palette
2. Select "Settings"
3. Press `⌥Tab` to switch between Global and Project scope
4. Use `←/→` to change values
5. Press `i` to toggle inherit from global (project scope only)
6. Press `⌃S` or `Enter` to save

## Inheritance System

Project settings can inherit from global settings:

```yaml
# In .paw/config
self_improve: false

inherit:
  self_improve: true     # Use global self_improve instead
```

When `inherit.<field>: true`, the project uses the global value for that field.

## Common User Requests

### "Set up automatic npm install when creating worktree"

```yaml
# In .paw/config (ask user which level)
pre_worktree_hook: npm install
```

### "Run tests before merging"

```yaml
# In .paw/config
pre_merge_hook: npm test
```

### "Show me the PAW logs"

Tell user: "Press `⌃O` to open the log viewer, or run `paw logs` from terminal."

### "How do I see keyboard shortcuts?"

Tell user: "Press `⌃/` to toggle the help viewer with all keyboard shortcuts."

## Task History

```bash
# View task history
paw history

# Search history
paw history --query "error"

# View specific history entry
paw history show 1
```

## Environment Variables (Available to Agents)

| Variable | Description |
|----------|-------------|
| `TASK_NAME` | Task identifier (branch name) |
| `PAW_DIR` | `.paw` directory path |
| `PROJECT_DIR` | Project root path |
| `WORKTREE_DIR` | Worktree path (git mode) |
| `PAW_BIN` | PAW binary path |
| `SESSION_NAME` | tmux session name |

## Keyboard Shortcuts Reference

| Shortcut | Action |
|----------|--------|
| `⌃N` | New task |
| `⌃F` | Finish task (shows action picker) |
| `⌃O` | Toggle log viewer |
| `⌃G` | Toggle git viewer |
| `⌃/` | Toggle help |
| `⌃P` | Command palette |
| `⌃Q` | Quit PAW |
| `⌥Tab` | Cycle panes |
| `⌥←/→` | Previous/next window |
