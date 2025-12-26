---
description: Clean up completed task (worktree, agent dir, window)
---

# Task Cleanup

## Step 1: Get Environment

```bash
printenv | grep -E '^(TASK_NAME|TAW_DIR|PROJECT_DIR|WORKTREE_DIR)='
```

Save these values for later use.

## Step 2: Check Merge Status

### Check if PR exists

```bash
cat {TAW_DIR}/agents/{TASK_NAME}/.pr 2>/dev/null || echo "NO_PR"
```

### If PR number exists, check merge status:

```bash
gh pr view {PR_NUMBER} --json state,merged -q '"state: \(.state), merged: \(.merged)"'
```

### If no PR, check if branch is merged to main:

```bash
git -C {PROJECT_DIR} fetch origin main 2>/dev/null || true
```

```bash
git -C {PROJECT_DIR} branch --merged main 2>/dev/null | grep -q {TASK_NAME} && echo "MERGED" || echo "NOT_MERGED"
```

## Step 3: Decision

- **If PR merged OR branch merged to main**: Proceed to cleanup
- **If NOT merged**: Ask user "This task is not merged yet. Proceed with cleanup anyway? (y/n)"
  - If user says no, stop here
  - If user says yes, proceed to cleanup

## Step 4: Run Cleanup Script

**Using the actual values from Step 1**, run the cleanup script:

```bash
{TAW_DIR}/cleanup "{TASK_NAME}" "{TAW_DIR}" "{PROJECT_DIR}" "{WORKTREE_DIR}"
```

Replace `{placeholders}` with actual values from Step 1.

The script will:
1. Remove the worktree
2. Prune worktree references
3. Delete the local branch
4. Remove the agent directory
5. Close the tmux window

## Step 5: Process Queue (Before window closes)

Before cleanup completes, check if there are queued tasks:

```bash
{TAW_DIR}/../_taw/bin/process-queue "$(basename {PROJECT_DIR})"
```

This will automatically start the next task from the queue if any exists.

Report the cleanup result to user (and mention if a new task was started from queue).
