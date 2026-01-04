package app

import "os"

const defaultMemoryTemplate = `# PAW Memory (project-wide)
# Update in place; keep entries concise and deduplicated.
version: 1
tests: {}
commands: {}
notes: {}
`

func ensureMemoryFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.WriteFile(path, []byte(defaultMemoryTemplate), 0644)
}
