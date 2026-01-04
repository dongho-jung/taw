---
description: Auto-generate PR with title/body and open in browser
---

# Create Pull Request

## Step 1: Get Environment (REQUIRED)

**You MUST run this first and save the values:**

```bash
printenv | grep -E '^(TASK_NAME|PAW_DIR|WORKTREE_DIR)='
```

If any variable is empty, stop and inform the user.

## Step 2: Check Current State

```bash
git branch --show-current
```

```bash
git status -sb | head -1
```

## Step 3: Review Changes

```bash
git log --oneline main..HEAD 2>/dev/null || git log --oneline master..HEAD 2>/dev/null || git log --oneline -10
```

```bash
git diff --stat main..HEAD 2>/dev/null || git diff --stat master..HEAD 2>/dev/null || git diff --stat -10
```

## Step 4: Create PR

1. Analyze the commits and changes above to create PR title and body.

2. PR Title format:
   - Under 50 characters, English
   - Conventional commit style (feat/fix/refactor/docs etc.)

3. PR Body format:
   ```
   ## Summary
   - Key changes in bullet points

   ## Changes
   - Detailed changes in bullet points
   ```

4. Push branch to remote if needed:
   ```bash
   git push -u origin $(git branch --show-current)
   ```

5. Create PR:
   ```bash
   gh pr create --title "title" --body "$(cat <<'EOF'
   ## Summary
   - content

   ## Changes
   - content
   EOF
   )"
   ```

6. **IMPORTANT**: After PR is created, save the PR number using environment variables:
   ```bash
   gh pr view --json number -q '.number' | tee "$PAW_DIR/agents/$TASK_NAME/.pr"
   ```

7. Open the created PR URL in browser:
   ```bash
   gh pr view --web
   ```

Proceed with PR creation.
