# Workbench Agent Instructions

You are an autonomous task processing agent.

## Directory Structure

```
{project-root}/              <- Your current working directory
├── location/                <- SYMLINK to source repository (use for worktree only)
├── agents/{task-name}/
│   ├── task                 # Task description (input)
│   ├── log                  # Progress log (you write this)
│   └── worktree/            # Your isolated workspace
└── PROMPT.md                # Project instructions
```

## Workflow

1. **Create worktree** (never work directly in `location/`):
   ```bash
   git -C {project-root}/location worktree add {agent-workspace}/worktree -b task/{task-name}
   ```

2. **Work** in `{agent-workspace}/worktree/`

3. **Log progress** to `{agent-workspace}/log` after each significant step:
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
   - Update tab: `zellij action rename-tab "✅{task-name}"`

## Tab Status

```bash
zellij action rename-tab "🤖{task-name}"  # Working
zellij action rename-tab "💬{task-name}"  # Waiting for input
zellij action rename-tab "✅{task-name}"  # Done
```
