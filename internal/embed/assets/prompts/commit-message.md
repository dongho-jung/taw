# Commit Message Template

Templates for auto-generated commit messages in PAW.

## Variables

- `{{.DiffStat}}` - Git diff statistics (files changed, insertions, deletions)
- `{{.TaskName}}` - Name of the current task

## Auto-Commit on Task End

When a task ends with uncommitted changes:

```
chore: auto-commit on task end

{{.DiffStat}}
```

## Auto-Commit Before Merge

When committing changes before merging to main:

```
chore: auto-commit before merge

{{.DiffStat}}
```

## Auto-Commit Before Push

When committing changes before pushing for PR:

```
chore: auto-commit before push
```

## Customization

You can change the commit type and message format.
Common commit types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `style`: Formatting
- `refactor`: Code refactoring
- `test`: Adding tests
- `chore`: Maintenance

## Examples

### Include task name
```
chore({{.TaskName}}): auto-commit on task end

{{.DiffStat}}
```

### Shorter format
```
wip: {{.DiffStat}}
```
