---
description: Auto-generate PR with title/body and open in browser
allowed-tools: Bash(git:*), Bash(gh:*), Bash(open:*), Bash(echo:*), Bash(tee:*)
---

# Create Pull Request

## Current State

- Task name: !`echo $TASK_NAME`
- TAW directory: !`echo $TAW_DIR`
- Current branch: !`git branch --show-current`
- Remote status: !`git status -sb | head -1`

## Commits on this branch

!`git log --oneline main..HEAD 2>/dev/null || git log --oneline master..HEAD 2>/dev/null || git log --oneline -10`

## Changes Summary

!`git diff --stat main..HEAD 2>/dev/null || git diff --stat master..HEAD 2>/dev/null || git diff --stat -10`

## Instructions

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
   git push -u origin <current-branch>
   ```

5. Create PR and capture the URL:
   ```bash
   gh pr create --title "title" --body "$(cat <<'EOF'
   ## Summary
   - content

   ## Changes
   - content
   EOF
   )"
   ```

6. **IMPORTANT**: After PR is created, save the PR number for `/done` command:
   ```bash
   gh pr view --json number -q '.number' | tee $TAW_DIR/agents/$TASK_NAME/.pr
   ```

7. Open the created PR URL in browser:
   ```bash
   gh pr view --web
   ```

Proceed with PR creation.
