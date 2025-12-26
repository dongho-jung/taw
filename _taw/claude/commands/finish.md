---
description: Complete task (commit → PR → update status → done)
---

# Finish Task

Automates the entire task completion workflow.

## Step 1: Get Environment (REQUIRED)

```bash
printenv | grep -E '^(TASK_NAME|TAW_DIR|PROJECT_DIR|WORKTREE_DIR|WINDOW_ID)='
```

Save these values. If any are empty, stop and inform user.

## Step 2: Check for Uncommitted Changes

```bash
git status -s
```

### If there are changes:

1. Stage all changes:
   ```bash
   git add -A
   ```

2. Analyze the diff:
   ```bash
   git diff --cached --stat
   ```

3. Generate commit message (conventional commit style):
   - Analyze staged changes
   - Format: `type(scope): description`
   - Types: feat, fix, refactor, docs, style, test, chore

4. Commit:
   ```bash
   git commit -m "generated message"
   ```

### If no changes:

Check if there are commits to push:
```bash
git log --oneline origin/{TASK_NAME}..HEAD 2>/dev/null || git log --oneline -1
```

## Step 3: Push Branch

```bash
git push -u origin $(git branch --show-current)
```

## Step 4: Create PR (if not exists)

Check if PR already exists:
```bash
gh pr view 2>/dev/null && echo "PR_EXISTS" || echo "NO_PR"
```

### If no PR:

1. Get main branch name:
   ```bash
   git -C {PROJECT_DIR} symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@' || echo "main"
   ```

2. Create PR:
   ```bash
   gh pr create --title "title" --body "$(cat <<'EOF'
   ## Summary
   - Key changes

   ## Changes
   - Detailed changes
   EOF
   )"
   ```

3. Save PR number:
   ```bash
   gh pr view --json number -q '.number' > {TAW_DIR}/agents/{TASK_NAME}/.pr
   ```

## Step 5: Update Window Status

```bash
tmux rename-window -t {WINDOW_ID} "✅$(echo {TASK_NAME} | cut -c1-12)"
```

## Step 6: Log Completion

```bash
echo "작업 완료 - PR 생성됨" >> {TAW_DIR}/agents/{TASK_NAME}/log
```

## Step 7: Process Queue

Check if there are queued tasks and start the next one:

```bash
{TAW_DIR}/../_taw/bin/process-queue "$(basename {PROJECT_DIR})"
```

Or if TAW_HOME is available:
```bash
$TAW_HOME/_taw/bin/process-queue "$(basename {PROJECT_DIR})"
```

This will automatically start the next task from the queue if any exists.

## Step 8: Report to User

Show:
- PR URL: `gh pr view --web` (open in browser)
- Remind user: "Run `/done` to cleanup when PR is merged"
- If queue was processed, mention: "Started next queued task"

Proceed with task completion.
