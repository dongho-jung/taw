---
description: Merge worktree branch into project's current branch
---

# Merge Branch

## Step 1: Get Environment (REQUIRED)

**You MUST run this first and save the values:**

```bash
printenv | grep -E '^(TASK_NAME|PROJECT_DIR|WORKTREE_DIR)='
```

If any variable is empty, stop and inform the user.

## Step 2: Check Current State

Using the environment variables from Step 1, check:

```bash
git branch --show-current
```

```bash
git -C "$PROJECT_DIR" branch --show-current
```

```bash
git status -s
```

## Step 3: Pre-merge Checks

### Check for uncommitted changes

If there are uncommitted changes in worktree, ask user whether to:
- Commit them first
- Stash them
- Abort merge

### Confirm merge target

The merge will be:
```
$TASK_NAME (worktree branch) -> project's current branch
```

Ask user to confirm before proceeding.

## Step 4: Perform Merge

Using environment variables from Step 1:

```bash
git -C "$PROJECT_DIR" merge --squash "$TASK_NAME" && git -C "$PROJECT_DIR" commit -m "feat: $TASK_NAME"
```

Use `--squash` to combine all commits from the branch into a single clean commit.

## Step 5: Handle Conflicts (if any)

If merge conflicts occur:

```bash
git -C "$PROJECT_DIR" diff --name-only --diff-filter=U
```

- List conflicted files
- Inform user about conflicts
- Do NOT auto-resolve - let user decide

## Step 6: Show Result

```bash
git -C "$PROJECT_DIR" log --oneline -5
```

## After Merge

Inform user they can now run `⌃F` to clean up the task (worktree, branch, window).

Proceed with merge.
