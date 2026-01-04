---
description: Record lessons learned from this session to prevent future mistakes
---

# Self-Improve: Record Lessons Learned

This command helps you reflect on the current session and record lessons learned to prevent repeating mistakes.

## Step 1: Ask User Where to Save

Before analyzing, ask the user where they want to save the lessons using a question with these exact options:

**Question**: "Where should I save the lessons learned?"

**Options**:
1. **Current directory** - Save to `./CLAUDE.md` (for project-specific learnings in this folder)
2. **Project (.paw/)** - Save to `.paw/CLAUDE.md` (shared across all tasks in this project)
3. **Global (~/.claude/)** - Save to `~/.claude/CLAUDE.md` (applies to all projects)

Wait for user response before proceeding.

## Step 2: Analyze the Session

Review the conversation history and identify:

1. **Mistakes Made**
   - Commands that failed and why
   - Wrong assumptions about the codebase
   - Incorrect solutions that had to be revised
   - Misunderstandings of user intent

2. **Lessons Learned**
   - Patterns that worked well
   - Tools or approaches that were effective
   - Codebase-specific knowledge discovered
   - User preferences identified

3. **Rules to Add**
   - Concrete, actionable rules to prevent future mistakes
   - Format: "When X, do Y" or "Never do X without Y"
   - Be specific, not generic

## Step 3: Format the Content

Create a section to append to CLAUDE.md:

```markdown
## Session Learnings (YYYY-MM-DD)

### Mistakes to Avoid
- [Specific mistake and why it was wrong]

### Rules
- [Actionable rule derived from this session]

### Notes
- [Any other relevant observations]
```

## Step 4: Write to CLAUDE.md

Based on user's choice in Step 1:

### Option 1: Current Directory
```bash
# Check if CLAUDE.md exists
if [ -f "./CLAUDE.md" ]; then
  # Append to existing file
  echo "" >> ./CLAUDE.md
  # Append the formatted content
else
  # Create new file with header
  echo "# CLAUDE.md" > ./CLAUDE.md
  echo "" >> ./CLAUDE.md
  # Append the formatted content
fi
```

### Option 2: Project (.paw/)
```bash
PAW_DIR="${PAW_DIR:-.paw}"
TARGET="$PAW_DIR/CLAUDE.md"

if [ -f "$TARGET" ]; then
  echo "" >> "$TARGET"
  # Append the formatted content
else
  echo "# Project-Specific Instructions" > "$TARGET"
  echo "" >> "$TARGET"
  # Append the formatted content
fi
```

### Option 3: Global (~/.claude/)
```bash
mkdir -p ~/.claude
TARGET="$HOME/.claude/CLAUDE.md"

if [ -f "$TARGET" ]; then
  echo "" >> "$TARGET"
  # Append the formatted content
else
  echo "# Global Claude Instructions" > "$TARGET"
  echo "" >> "$TARGET"
  # Append the formatted content
fi
```

## Step 5: Confirm

After writing, show the user:
1. What was written
2. Where it was saved
3. Remind them that Claude will read this file in future sessions

## Important Notes

- Be honest about mistakes - the goal is improvement, not blame
- Focus on actionable lessons, not vague observations
- Keep rules concise and specific
- If no significant mistakes were made, still look for optimization opportunities
- Don't include sensitive information (passwords, API keys, etc.)
