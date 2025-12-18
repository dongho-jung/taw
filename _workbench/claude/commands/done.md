---
description: Clean up completed task (worktree, agent dir, tab)
allowed-tools: Bash(git:*), Bash(rm:*), Bash(zellij:*), Bash(ls:*), Bash(cat:*), Bash(gh:*), Bash(echo:*), Bash(printenv:*), Bash(test:*), Bash(true:*)
---

# Task Cleanup

## Step 1: Get Task Name

First, get the task name from environment:
```bash
echo $TASK_NAME
```

## Step 2: Check Merge Status

### Method A: Check saved PR number

```bash
cat agents/$TASK_NAME/.pr
```

If PR number exists, check its merge status:
```bash
gh pr view <PR_NUMBER> --json merged,state -q '"\(.state) merged:\(.merged)"'
```

### Method B: Check if commits are in main (fallback)

If no PR file exists:
```bash
git -C location fetch origin main
```

```bash
git -C agents/$TASK_NAME/worktree rev-parse HEAD
```

Then check if that commit is in main:
```bash
git -C location branch -r --contains <COMMIT_HASH> | grep origin/main
```

## Decision Logic

- **PR merged: true** OR **commit in main** → Proceed without confirmation
- Otherwise → Ask user to confirm

## Step 3: Cleanup

Execute these commands. Ignore errors - some resources may already be cleaned up.

### 3.1 Clean stale worktree references

```bash
git -C location worktree prune
```

### 3.2 Remove worktree (if exists)

```bash
test -d agents/$TASK_NAME/worktree && git -C location worktree remove $(pwd)/agents/$TASK_NAME/worktree --force || true
```

### 3.3 Delete local branch (if exists)

```bash
git -C location branch --list $TASK_NAME | grep -q $TASK_NAME && git -C location branch -D $TASK_NAME || true
```

### 3.4 Remove agent directory (if exists)

```bash
test -d agents/$TASK_NAME && rm -rf agents/$TASK_NAME || true
```

### 3.5 Close zellij tab

```bash
zellij --session $ZELLIJ_SESSION_NAME action close-tab
```

If this fails, tell the user to close the tab manually with `Ctrl+O, x`.

## Important

- Use `|| true` to prevent errors from stopping cleanup
- If any command fails, continue with the next step
- Report what was cleaned up at the end

Proceed with cleanup.
