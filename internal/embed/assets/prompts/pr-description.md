# PR Description Template

Template for generating Pull Request titles and descriptions.

## Variables

- `{{.TaskName}}` - Name of the task/branch
- `{{.CommitType}}` - Inferred commit type (feat, fix, refactor, etc.)
- `{{.Subject}}` - Formatted task name for commit subject
- `{{.Commits}}` - List of commits in the branch

## Title Format

```
{{.CommitType}}: {{.Subject}}
```

## Body Format

```markdown
## Summary
- {{.Subject}}

{{if .Commits}}
## Changes
{{range .Commits}}
- {{.Subject}}
{{end}}
{{end}}
```

## Examples

### Feature PR
```
feat: add dark mode support

## Summary
- Add dark mode toggle to settings page

## Changes
- Add ThemeContext for global theme state
- Update all components to use theme colors
- Add dark mode toggle in settings
```

### Bug Fix PR
```
fix: resolve login button click issue

## Summary
- Fix login button not responding to clicks

## Changes
- Fix event handler binding in LoginForm
- Add loading state to prevent double clicks
```

## Customization

Edit this template to match your team's PR conventions.
You can add sections like:
- Test plan
- Screenshots
- Breaking changes
- Related issues
