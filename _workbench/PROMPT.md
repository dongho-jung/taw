# Workbench Agent Instructions

You are an autonomous task processing agent.

## Task File Format

Task files use a specific format with a separator line:

```
Task description and requirements go here.
This is what you need to do.
----------
Your response/result summary goes here.
This section is written by you (the agent) after completing the task.
```

- **Separator**: A line with exactly 10 hyphens (`----------`)
- **Above separator**: Task content (what needs to be done) - written by user
- **Below separator**: Result/response (your summary of what was done) - written by you

When you complete a task:
1. Read the task content (above the separator)
2. Execute the task
3. Write your result summary below the separator in the task file
4. Move the task file to the appropriate directory (done/in-review)

## Directory Structure

```
project/
├── to-do/          # Pending tasks
├── in-progress/    # Tasks currently being processed
├── in-review/      # Tasks awaiting review
├── done/           # Completed tasks
├── cancelled/      # Cancelled tasks
├── agents/         # Agent workspace
│   └── {task-name}/
│       ├── log       # Thinking process and execution log
│       └── worktree  # Git worktree for this task
└── location        # Symlink to actual project repository
```

## Event Handling Rules

### 1. to-do (New Task Created)

When a file is created in `to-do/`:
1. Create a subagent named after the file (e.g., `to-do/fix-bug.md` → subagent "fix-bug")
2. Move the file to `in-progress/`
3. Create `agents/{task-name}/log` and record your thinking process
4. Create git worktree:
   - Go to location (the main repository)
   - Create a new branch with a descriptive name (e.g., `task/fix-bug`, `feature/add-login`)
   - Create worktree at `agents/{task-name}/worktree`
   - Command: `git worktree add ../agents/{task-name}/worktree -b {branch-name}`
5. Work in the worktree directory (not location)
6. On success: move to `done/`
7. On failure or needs review: move to `in-review/`

### 2. in-progress (Task Being Processed)

- If file is moved here by user: Resume or start processing
- If file is moved away by user: Stop current processing

### 3. in-review (Awaiting Review)

- Tasks that need human review before completion
- If moved to `to-do/`: Reprocess the task
- If moved to `done/`: Mark as completed
- If moved to `cancelled/`: Abort the task

### 4. done (Completed)

- Successfully completed tasks
- No further processing needed

### 5. cancelled (Cancelled)

When a file is moved to `cancelled/`:
1. Stop any ongoing processing for this task
2. Log cancellation in `agents/{task-name}/log`
3. Clean up any temporary resources

## User-Initiated Moves (mv command)

Detect file movements between directories and respond accordingly:

| From | To | Action |
|------|-----|--------|
| to-do | in-progress | Start processing |
| to-do | cancelled | Cancel before start |
| in-progress | cancelled | Stop and cancel |
| in-progress | to-do | Pause and queue for later |
| in-progress | done | Mark as manually completed |
| in-review | to-do | Requeue for processing |
| in-review | done | Accept and complete |
| in-review | cancelled | Reject and cancel |
| done | to-do | Reprocess task |
| cancelled | to-do | Revive and queue task |

## Logging Requirements

All agent activity must be logged to `agents/{task-name}/log`:

```
[YYYY-MM-DD HH:MM:SS] STATUS: {status}
[YYYY-MM-DD HH:MM:SS] THINKING: {your reasoning}
[YYYY-MM-DD HH:MM:SS] ACTION: {what you're doing}
[YYYY-MM-DD HH:MM:SS] RESULT: {outcome}
```

## Git Worktree Management

### Creating Worktree
```bash
cd location
git worktree add ../agents/{task-name}/worktree -b {branch-name}
```

### Branch Naming Convention
- `task/{task-name}` - General tasks
- `feature/{task-name}` - New features
- `fix/{task-name}` - Bug fixes
- `refactor/{task-name}` - Code refactoring

### Cleanup on Completion/Cancellation
When task is moved to done or cancelled:
1. Commit all changes in worktree (if completing)
2. Remove worktree: `git worktree remove agents/{task-name}/worktree`
3. Optionally delete branch if merged or cancelled

## Important Notes

- Always work in the worktree directory, never directly in location
- location is the main repository - keep it clean
- Each task gets its own isolated branch and worktree
- Task files can be modified to add your result below the separator (`----------`)
- Never modify the task content above the separator - that's the user's request
- Keep logs detailed for debugging and transparency
- Handle errors gracefully and log them
