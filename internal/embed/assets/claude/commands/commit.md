---
description: Smart commit with auto-generated message
---

# Smart Commit

Analyzes changes and generates a meaningful commit message.

## Step 1: Check Status

```bash
git status -s
```

If no changes, inform user and stop.

## Step 2: Stage Changes

### Option A: Stage all
```bash
git add -A
```

### Option B: Interactive (if user wants selective staging)
Show unstaged files and ask which to include.

## Step 3: Analyze Changes

```bash
git diff --cached --stat
```

```bash
git diff --cached
```

Review:
- Which files changed
- What type of change (new feature, bug fix, refactor, docs, etc.)
- Scope of change (specific component/module)

## Step 4: Generate Commit Message

### Format: Conventional Commits

```
type(scope): short description

[optional body with more details]
```

### Types:
- `feat`: New feature
- `fix`: Bug fix
- `refactor`: Code restructuring
- `docs`: Documentation only
- `style`: Formatting, whitespace
- `test`: Adding/updating tests
- `chore`: Maintenance, dependencies
- `perf`: Performance improvement

### Rules:
- Subject line ≤ 50 characters
- Use imperative mood ("add" not "added")
- No period at end
- Body explains "why" not "what"

### Examples:
```
feat(auth): add password reset flow

fix(api): handle null response from user endpoint

refactor(utils): extract date formatting to helper

docs(readme): update installation steps

test(cart): add unit tests for checkout process
```

## Step 5: Confirm with User

Show generated message and ask:
- Use this message? (y)
- Edit message? (e)
- Cancel? (n)

## Step 6: Commit

```bash
git commit -m "generated message"
```

## Step 7: Log

```bash
echo "Commit: {SHORT_HASH} {MESSAGE_SUBJECT}" >> $PAW_DIR/agents/$TASK_NAME/log
```

## Step 8: Show Result

```bash
git log --oneline -1
```

Proceed with smart commit.
