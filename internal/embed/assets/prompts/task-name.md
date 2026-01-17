# Task Name Generation Rules

Generate a concise, kebab-case task name from the user's input.

## Rules

1. **Format**: Use kebab-case (lowercase words separated by hyphens)
2. **Length**: Keep it between 3-5 words (8-32 characters)
3. **Prefix**: Use conventional commit prefixes when applicable:
   - `fix-` for bug fixes
   - `feat-` for new features
   - `refactor-` for code refactoring
   - `docs-` for documentation
   - `test-` for tests
   - `chore-` for maintenance tasks

## Examples

| User Input | Task Name |
|------------|-----------|
| "Fix the login button not working" | `fix-login-button` |
| "Add dark mode support" | `feat-dark-mode` |
| "Refactor the API client" | `refactor-api-client` |
| "Update README" | `docs-update-readme` |

## Output

Return ONLY the task name, nothing else.
