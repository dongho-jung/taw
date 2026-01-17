# Merge Conflict Resolution

You are resolving merge conflicts in a git repository.

## Variables

- `{{.ConflictFiles}}` - List of files with conflicts
- `{{.TaskName}}` - Name of the task/branch being merged
- `{{.TaskContent}}` - Description of the task
- `{{.MainBranch}}` - Target branch (usually main/master)

## Instructions

1. Read each conflicting file listed
2. Look for conflict markers:
   - `<<<<<<< HEAD` - Current branch content
   - `=======` - Separator
   - `>>>>>>> branch` - Incoming branch content
3. Resolve each conflict by keeping the correct code that makes sense for the task
4. Save each resolved file using the Edit tool
5. After resolving ALL conflicts, run: `git add -A`

## Guidelines

- Do NOT abort or skip any files
- Resolve ALL conflicts before running git add
- Make sure the final code is valid and compiles
- If unsure, prefer keeping BOTH changes merged intelligently
- Preserve formatting and code style consistency

## Default Prompt

```
ultrathink You are resolving merge conflicts in a git repository.

## Conflicting Files
{{.ConflictFiles}}

## Task Context
Task name: {{.TaskName}}
Task description:
{{.TaskContent}}

## Instructions
1. Read each conflicting file listed above
2. Look for conflict markers (<<<<<<< HEAD, =======, >>>>>>> branch)
3. Resolve each conflict by keeping the correct code that makes sense for the task
4. Save each resolved file using the Edit tool
5. After resolving ALL conflicts, run: git add -A

IMPORTANT:
- Do NOT abort or skip any files
- Resolve ALL conflicts before running git add
- Make sure the final code is valid and compiles
- If unsure, prefer keeping BOTH changes merged intelligently

Start resolving the conflicts now.
```
