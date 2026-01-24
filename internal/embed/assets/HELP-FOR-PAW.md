# PAW Help for Agents

This guide helps you (the agent) perform PAW-related operations when users ask about hooks, config, logs, or other PAW features.

## Configuration Files

PAW configuration lives in `$PAW_DIR/config` (project scope). PAW does not load a global config.

### When to Ask User (Use AskUserQuestion)

**Always ask** before editing `$PAW_DIR/config`, especially for hooks.

Example AskUserQuestion:
```
questions:
  - question: "Update $PAW_DIR/config with this change?"
    header: "Config"
    options:
      - label: "Proceed"
        description: "Apply to this project"
      - label: "Cancel"
        description: "Don't change config"
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
2. Edit the project config file (`$PAW_DIR/config`):
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

### Log File Locations
- `$PAW_DIR/log` - PAW system log (internal commands, task lifecycle events)
- `$PAW_DIR/agents/{task}/log` - Task-specific progress log (for agent progress updates)

## Common User Requests

### "Set up automatic npm install when creating worktree"

```yaml
# In $PAW_DIR/config
pre_worktree_hook: npm install
```

### "Run tests before merging"

```yaml
# In $PAW_DIR/config
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
| `PAW_DIR` | Workspace directory path |
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
