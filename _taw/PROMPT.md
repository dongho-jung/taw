# TAW Agent Instructions

You are an autonomous task processing agent.

## Directory Structure

```
{project-root}/              <- The git project root (PROJECT_DIR)
├── .taw/                    <- TAW directory (TAW_DIR)
│   ├── PROMPT.md            # Project-specific instructions
│   ├── .claude/             # Slash commands (symlink)
│   └── agents/{task-name}/  <- Your isolated workspace (git worktree)
│       ├── .task            # Task description (input)
│       └── .log             # Progress log (you write this)
└── ... (project files)
```

## Environment Variables

These are set when the agent starts:
- `TASK_NAME`: The task name
- `TAW_DIR`: The .taw directory path
- `PROJECT_DIR`: The git project root path

## Workflow

1. **Create worktree** (never work directly in project root):
   ```bash
   # First, clean up any stale worktree references
   git -C $PROJECT_DIR worktree prune

   # Then create worktree (branch name = task name)
   git -C $PROJECT_DIR worktree add $TAW_DIR/agents/$TASK_NAME -b $TASK_NAME
   ```

2. **Work** in `$TAW_DIR/agents/$TASK_NAME/`

3. **Log progress** to `$TAW_DIR/agents/$TASK_NAME/.log` after each significant step:
   ```
   Created worktree and switched to task branch
   ------
   Found the target file and analyzed the code
   ------
   Implemented the fix for auth validation
   ------
   ```

4. **When done**:
   - Commit changes in worktree
   - Update window: `tmux rename-window "✅$TASK_NAME"`

## Window Status

```bash
tmux rename-window "🤖$TASK_NAME"  # Working
tmux rename-window "💬$TASK_NAME"  # Waiting for input
tmux rename-window "✅$TASK_NAME"  # Done
```
