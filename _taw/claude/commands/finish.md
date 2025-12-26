---
description: Complete task (commit → PR/merge → update status → done)
---

# Finish Task

Automates the entire task completion workflow based on `ON_COMPLETE` setting.

## Step 1: Get Environment (REQUIRED)

```bash
printenv | grep -E '^(TASK_NAME|TAW_DIR|PROJECT_DIR|WORKTREE_DIR|WINDOW_ID|ON_COMPLETE)='
```

Save these values. If any required values are empty, stop and inform user.
- `ON_COMPLETE` defaults to "confirm" if not set.

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

## Step 3: Branch Based on ON_COMPLETE

Check `ON_COMPLETE` value and proceed accordingly:

### If ON_COMPLETE = "confirm"

Ask user what to do next:
1. Create PR
2. Merge to main
3. Just commit (done)

### If ON_COMPLETE = "auto-commit"

Skip PR and merge. Just update window status and finish.

### If ON_COMPLETE = "auto-merge"

**Perform automatic merge to main branch:**

1. Push current branch:
   ```bash
   git push -u origin $(git branch --show-current)
   ```

2. Get the main branch name from PROJECT_DIR:
   ```bash
   git -C {PROJECT_DIR} symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@' || echo "main"
   ```

3. Merge into main branch in PROJECT_DIR:
   ```bash
   git -C {PROJECT_DIR} merge {TASK_NAME} --no-ff -m "Merge branch '{TASK_NAME}'"
   ```

4. If merge conflicts occur, inform user and stop. Do NOT auto-resolve.

5. Push main branch:
   ```bash
   git -C {PROJECT_DIR} push origin $(git -C {PROJECT_DIR} branch --show-current)
   ```

6. Skip to Step 5 (Update Window Status)

### If ON_COMPLETE = "auto-pr"

**Create PR automatically:**

1. Push branch:
   ```bash
   git push -u origin $(git branch --show-current)
   ```

2. Check if PR already exists:
   ```bash
   gh pr view 2>/dev/null && echo "PR_EXISTS" || echo "NO_PR"
   ```

3. If no PR, create one:
   ```bash
   gh pr create --title "title" --body "$(cat <<'EOF'
   ## Summary
   - Key changes

   ## Changes
   - Detailed changes
   EOF
   )"
   ```

4. Save PR number:
   ```bash
   gh pr view --json number -q '.number' > {TAW_DIR}/agents/{TASK_NAME}/.pr
   ```

## Step 4: (For auto-pr only) Open PR in Browser

```bash
gh pr view --web
```

## Step 5: Update Window Status

```bash
tmux rename-window -t {WINDOW_ID} "✅$(echo {TASK_NAME} | cut -c1-12)"
```

## Step 6: Log Completion

```bash
echo "작업 완료 - $([ \"$ON_COMPLETE\" = 'auto-merge' ] && echo 'Merged to main' || [ \"$ON_COMPLETE\" = 'auto-pr' ] && echo 'PR 생성됨' || echo 'Committed')" >> {TAW_DIR}/agents/{TASK_NAME}/log
```

## Step 7: Process Queue

Check if there are queued tasks and start the next one:

```bash
$TAW_HOME/_taw/bin/process-queue "$(basename {PROJECT_DIR})"
```

If TAW_HOME is not available, find it from the new-task symlink:
```bash
TAW_HOME=$(cd "$(dirname "$(readlink {TAW_DIR}/new-task)")/../.." && pwd)
$TAW_HOME/_taw/bin/process-queue "$(basename {PROJECT_DIR})"
```

This will automatically start the next task from the queue if any exists.

## Step 8: Report to User

Based on ON_COMPLETE mode:

- **auto-merge**: "Changes merged to main branch. Run `/done` to cleanup."
- **auto-pr**: Show PR URL. "Run `/done` to cleanup when PR is merged."
- **auto-commit**: "Changes committed. Run `/merge` to merge or `/pr` to create PR."
- **confirm**: Show what was done and available options.

If queue was processed, mention: "Started next queued task"

Proceed with task completion.
