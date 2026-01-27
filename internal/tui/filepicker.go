package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

// FilePickerAction represents the selected action.
type FilePickerAction int

// File picker action options.
const (
	FilePickerCancel FilePickerAction = iota
	FilePickerSelect
)

// FileEntry represents a file or directory.
type FileEntry struct {
	Name  string
	Path  string
	IsDir bool
}

// FilePicker is a fuzzy-searchable file picker with directory navigation.
type FilePicker struct {
	input            textinput.Model
	inputOffset      int
	inputOffsetRight int
	rootDir          string        // Root directory (project dir)
	currentDir       string        // Current directory being browsed
	entries          []FileEntry   // Files/dirs in current directory
	searchEntries    []FileEntry   // All files including subdirectories (for search)
	filtered         []int         // Indices into entries/searchEntries for filtered results
	useSearchEntries bool          // Whether filtered indices refer to searchEntries
	cursor           int           // Current selection
	action           FilePickerAction
	selected         string        // Selected file path
	isDark           bool
	colors           ThemeColors
	width            int
	height           int

	// Preview cache
	previewPath    string   // Path of currently previewed file
	previewContent []string // Cached preview lines
	previewOffset  int      // Preview scroll offset

	// Layout cache (for mouse click handling)
	listStartY  int // Y position where file list starts
	listEndY    int // Y position where file list ends
	listStartIdx int // First visible item index in filtered

	// Style cache (reused across renders)
	styleTitle    lipgloss.Style
	styleInput    lipgloss.Style
	styleItem     lipgloss.Style
	styleDir      lipgloss.Style
	styleSelected lipgloss.Style
	styleHelp     lipgloss.Style
	styleDim      lipgloss.Style
	stylePath     lipgloss.Style
	stylePreview  lipgloss.Style
	stylesCached  bool
}

// NewFilePicker creates a new file picker starting at the given directory.
func NewFilePicker(startDir string) *FilePicker {
	isDark := DetectDarkMode()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Type to search..."
	ti.Focus()
	ti.CharLimit = 100
	ti.SetWidth(50)
	ti.VirtualCursor = false

	fp := &FilePicker{
		input:      ti,
		rootDir:    startDir,
		currentDir: startDir,
		cursor:     0,
		isDark:     isDark,
		colors:     NewThemeColors(isDark),
		width:      70,
		height:     20,
	}

	fp.loadDirectory()
	return fp
}

// loadDirectory loads entries from the current directory.
func (m *FilePicker) loadDirectory() {
	m.entries = nil
	m.searchEntries = nil
	m.filtered = nil
	m.useSearchEntries = false
	m.cursor = 0

	dirEntries, err := os.ReadDir(m.currentDir)
	if err != nil {
		return
	}

	// Separate dirs and files, then sort each
	var dirs, files []FileEntry
	for _, entry := range dirEntries {
		name := entry.Name()
		// Skip hidden files
		if strings.HasPrefix(name, ".") {
			continue
		}
		fe := FileEntry{
			Name:  name,
			Path:  filepath.Join(m.currentDir, name),
			IsDir: entry.IsDir(),
		}
		if entry.IsDir() {
			dirs = append(dirs, fe)
		} else {
			files = append(files, fe)
		}
	}

	// Sort alphabetically
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	// Dirs first, then files
	m.entries = append(dirs, files...)

	// Initialize filtered to all
	m.filtered = make([]int, len(m.entries))
	for i := range m.entries {
		m.filtered[i] = i
	}
}

// collectRecursiveFiles collects all files from current directory and subdirectories.
func (m *FilePicker) collectRecursiveFiles() {
	if len(m.searchEntries) > 0 {
		return // Already collected
	}

	var currentFiles, subFiles []FileEntry

	// First, add current directory files (non-directories)
	for _, e := range m.entries {
		if !e.IsDir {
			currentFiles = append(currentFiles, e)
		}
	}

	// Then, walk subdirectories
	err := filepath.WalkDir(m.currentDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip the root directory itself
		if path == m.currentDir {
			return nil
		}

		name := d.Name()

		// Skip hidden files and directories
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories in results (we only want files)
		if d.IsDir() {
			return nil
		}

		// Get relative path from current directory
		relPath, err := filepath.Rel(m.currentDir, path)
		if err != nil {
			return nil
		}

		// Skip files in current directory (already added)
		if !strings.Contains(relPath, string(filepath.Separator)) {
			return nil
		}

		subFiles = append(subFiles, FileEntry{
			Name:  relPath, // Show relative path as name
			Path:  path,
			IsDir: false,
		})

		return nil
	})
	if err != nil {
		return
	}

	// Sort subFiles by name
	sort.Slice(subFiles, func(i, j int) bool { return subFiles[i].Name < subFiles[j].Name })

	// Current directory files first, then subdirectory files
	m.searchEntries = append(currentFiles, subFiles...)
}

// Init initializes the file picker.
func (m *FilePicker) Init() tea.Cmd {
	if _, ok := cachedDarkModeValue(); ok {
		return nil
	}
	return tea.RequestBackgroundColor
}

// Update handles messages.
func (m *FilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputWidth := min(50, m.width-10)
		if inputWidth > 20 {
			m.input.SetWidth(inputWidth)
		}

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.colors = NewThemeColors(m.isDark)
		m.stylesCached = false
		setCachedDarkMode(m.isDark)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.action = FilePickerCancel
			return m, tea.Quit

		case "enter":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				activeEntries := m.getActiveEntries()
				entry := activeEntries[m.filtered[m.cursor]]
				if entry.IsDir {
					// Navigate into directory
					m.currentDir = entry.Path
					m.input.SetValue("")
					m.loadDirectory()
					return m, nil
				}
				// Select file
				m.selected = entry.Path
				m.action = FilePickerSelect
				return m, tea.Quit
			}
			return m, nil

		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil

		case "left", "ctrl+h":
			// Go to parent directory (but not above root)
			parent := filepath.Dir(m.currentDir)
			if strings.HasPrefix(parent, m.rootDir) || parent == m.rootDir {
				m.currentDir = parent
				m.input.SetValue("")
				m.loadDirectory()
			}
			return m, nil

		case "right", "ctrl+l":
			// Enter selected directory
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				activeEntries := m.getActiveEntries()
				entry := activeEntries[m.filtered[m.cursor]]
				if entry.IsDir {
					m.currentDir = entry.Path
					m.input.SetValue("")
					m.loadDirectory()
				}
			}
			return m, nil

		case "pgup":
			m.cursor -= 5
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil

		case "pgdown":
			m.cursor += 5
			if m.cursor >= len(m.filtered) {
				m.cursor = max(0, len(m.filtered)-1)
			}
			return m, nil
		}

	case tea.MouseWheelMsg:
		// Scroll based on Y position
		if msg.Y >= m.listStartY && msg.Y < m.listEndY {
			// File list scroll
			switch msg.Button {
			case tea.MouseWheelUp:
				if m.cursor > 0 {
					m.cursor--
				}
			case tea.MouseWheelDown:
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
			}
		} else if msg.Y >= m.listEndY {
			// Preview scroll
			switch msg.Button {
			case tea.MouseWheelUp:
				if m.previewOffset > 0 {
					m.previewOffset--
				}
			case tea.MouseWheelDown:
				maxOffset := max(0, len(m.previewContent)-(m.height-m.listEndY-2))
				if m.previewOffset < maxOffset {
					m.previewOffset++
				}
			}
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			// Click in file list area
			if msg.Y >= m.listStartY && msg.Y < m.listEndY {
				clickedIdx := m.listStartIdx + (msg.Y - m.listStartY)
				if clickedIdx >= 0 && clickedIdx < len(m.filtered) {
					if m.cursor == clickedIdx {
						// Double-click behavior: select/enter
						activeEntries := m.getActiveEntries()
						entry := activeEntries[m.filtered[m.cursor]]
						if entry.IsDir {
							m.currentDir = entry.Path
							m.input.SetValue("")
							m.loadDirectory()
						} else {
							m.selected = entry.Path
							m.action = FilePickerSelect
							return m, tea.Quit
						}
					} else {
						m.cursor = clickedIdx
					}
				}
			}
		}
		return m, nil
	}

	// Update text input
	m.input, cmd = m.input.Update(msg)
	m.updateFiltered()
	m.syncInputOffset()

	return m, cmd
}

func (m *FilePicker) syncInputOffset() {
	syncTextInputOffset([]rune(m.input.Value()), m.input.Position(), m.input.Width(), &m.inputOffset, &m.inputOffsetRight)
}

// updateFiltered filters entries based on input.
func (m *FilePicker) updateFiltered() {
	query := m.input.Value()
	if query == "" {
		// No query: show current directory entries only
		m.useSearchEntries = false
		m.filtered = make([]int, len(m.entries))
		for i := range m.entries {
			m.filtered[i] = i
		}
		if m.cursor >= len(m.filtered) {
			m.cursor = 0
		}
		return
	}

	// With query: search in current directory AND subdirectories
	m.collectRecursiveFiles()
	m.useSearchEntries = true

	// Create searchable strings from searchEntries
	searchables := make([]string, 0, len(m.searchEntries))
	for _, e := range m.searchEntries {
		searchables = append(searchables, e.Name)
	}

	// Fuzzy search
	matches := fuzzy.Find(query, searchables)

	m.filtered = make([]int, len(matches))
	for i, match := range matches {
		m.filtered[i] = match.Index
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

// getActiveEntries returns the entries slice currently being used (entries or searchEntries).
func (m *FilePicker) getActiveEntries() []FileEntry {
	if m.useSearchEntries {
		return m.searchEntries
	}
	return m.entries
}

// renderInput prepares the text input line and cursor position.
func (m *FilePicker) renderInput() textInputRender {
	return renderTextInput(m.input.Value(), m.input.Position(), m.input.Width(), m.input.Placeholder, m.inputOffset, m.inputOffsetRight)
}

// relativePath returns the path relative to rootDir.
func (m *FilePicker) relativePath() string {
	rel, err := filepath.Rel(m.rootDir, m.currentDir)
	if err != nil {
		return m.currentDir
	}
	if rel == "." {
		return "/"
	}
	return "/" + rel
}

// loadPreview loads preview content for the selected file.
func (m *FilePicker) loadPreview() {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		m.previewPath = ""
		m.previewContent = nil
		return
	}

	activeEntries := m.getActiveEntries()
	entry := activeEntries[m.filtered[m.cursor]]
	if entry.IsDir {
		m.previewPath = ""
		m.previewContent = nil
		return
	}

	// Skip if already loaded
	if m.previewPath == entry.Path {
		return
	}

	m.previewPath = entry.Path
	m.previewContent = nil
	m.previewOffset = 0 // Reset scroll on file change

	// Check file size first (skip large files)
	info, err := os.Stat(entry.Path)
	if err != nil || info.Size() > 100*1024 { // Skip files > 100KB
		m.previewContent = []string{"(File too large to preview)"}
		return
	}

	// Read file content
	data, err := os.ReadFile(entry.Path) //nolint:gosec // G304: entry.Path is from directory listing
	if err != nil {
		m.previewContent = []string{"(Cannot read file)"}
		return
	}

	// Check if binary
	if isBinaryContent(data) {
		m.previewContent = []string{"(Binary file)"}
		return
	}

	// Split into lines
	lines := strings.Split(string(data), "\n")
	m.previewContent = lines
}

// isBinaryContent checks if content appears to be binary.
func isBinaryContent(data []byte) bool {
	// Check first 512 bytes for null bytes or non-printable characters
	checkLen := min(512, len(data))
	for i := range checkLen {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// truncateRuneString truncates a string to maxWidth runes, adding "..." if truncated.
func truncateRuneString(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-3]) + "..."
}

// View renders the file picker.
func (m *FilePicker) View() tea.View {
	c := m.colors

	if !m.stylesCached {
		m.styleTitle = lipgloss.NewStyle().Bold(true).Foreground(c.Accent)
		m.styleInput = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(c.BorderFocused).
			Padding(0, 1)
		m.styleItem = lipgloss.NewStyle().Foreground(c.TextNormal).PaddingLeft(2)
		m.styleDir = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).PaddingLeft(2) // Blue for dirs
		m.styleSelected = lipgloss.NewStyle().Foreground(c.Accent).Bold(true).PaddingLeft(0)
		m.styleHelp = lipgloss.NewStyle().Foreground(c.TextDim)
		m.styleDim = lipgloss.NewStyle().Foreground(c.TextDim)
		m.stylePath = lipgloss.NewStyle().Foreground(c.TextDim).Bold(true)
		m.stylePreview = lipgloss.NewStyle().Foreground(c.TextDim)
		m.stylesCached = true
	}

	// Load preview for current selection
	m.loadPreview()

	var sb strings.Builder
	sb.Grow((m.width + 1) * (m.height + 1))
	line := 0

	// Title with current path
	sb.WriteString(m.styleTitle.Render("Select File"))
	sb.WriteString("  ")
	sb.WriteString(m.stylePath.Render(m.relativePath()))
	sb.WriteString("\n\n")
	line += 2

	// Input
	inputRender := m.renderInput()
	inputBox := m.styleInput.Render(inputRender.Text)
	inputBoxTopY := line
	sb.WriteString(inputBox)
	sb.WriteString("\n\n")

	// Calculate heights: file list is fixed at 5 lines, rest for preview
	// Reserved: title(2) + input(3) + help(1) + separator(1) = 7
	availableHeight := m.height - 7
	listHeight := 5 // Fixed file list height
	previewHeight := max(3, availableHeight-listHeight)

	// Track layout for mouse click handling
	// Input box takes 3 lines (border top + content + border bottom), plus 1 blank line
	m.listStartY = line + 4 // After input box and blank line
	currentY := m.listStartY

	// Entries
	activeEntries := m.getActiveEntries()
	totalItems := len(m.filtered)

	// Calculate scrollbar for file list
	needsScrollbar := totalItems > listHeight
	scrollbarWidth := 0
	if needsScrollbar {
		scrollbarWidth = 2 // " │" or " ┃"
	}

	if totalItems == 0 {
		m.listStartIdx = 0
		if len(activeEntries) == 0 {
			sb.WriteString(m.styleDim.Render("  Empty directory"))
		} else {
			sb.WriteString(m.styleDim.Render("  No matching files"))
		}
		sb.WriteString("\n")
		currentY++
		// Fill remaining list space
		for i := 1; i < listHeight; i++ {
			sb.WriteString("\n")
			currentY++
		}
	} else {
		start := 0
		if m.cursor >= listHeight {
			start = m.cursor - listHeight + 1
		}
		end := min(start+listHeight, totalItems)
		m.listStartIdx = start

		// Generate scrollbar lines if needed
		var scrollbarLines []string
		if needsScrollbar {
			scrollbar := renderVerticalScrollbar(totalItems, listHeight, start, m.isDark)
			scrollbarLines = strings.Split(scrollbar, "\n")
		}

		for i := start; i < end; i++ {
			idx := m.filtered[i]
			entry := activeEntries[idx]

			name := entry.Name
			if entry.IsDir {
				name += "/"
			}

			// Truncate name if too wide (rune-aware), account for scrollbar
			maxNameWidth := m.width - 4 - scrollbarWidth
			name = truncateRuneString(name, maxNameWidth)

			// Pad name to align scrollbar
			nameRunes := []rune(name)
			padding := maxNameWidth - len(nameRunes)
			if padding < 0 {
				padding = 0
			}

			if i == m.cursor {
				sb.WriteString(m.styleSelected.Render("> " + name))
			} else if entry.IsDir {
				sb.WriteString(m.styleDir.Render(name))
			} else {
				sb.WriteString(m.styleItem.Render(name))
			}

			// Add scrollbar
			if needsScrollbar && i-start < len(scrollbarLines) {
				sb.WriteString(strings.Repeat(" ", padding+1))
				sb.WriteString(scrollbarLines[i-start])
			}

			sb.WriteString("\n")
			currentY++
		}

		// Fill remaining list space
		for i := end - start; i < listHeight; i++ {
			if needsScrollbar && i < len(scrollbarLines) {
				sb.WriteString(strings.Repeat(" ", m.width-3))
				sb.WriteString(scrollbarLines[i])
			}
			sb.WriteString("\n")
			currentY++
		}
	}

	m.listEndY = currentY

	// Separator
	separator := strings.Repeat("─", max(1, m.width-2))
	sb.WriteString(m.styleDim.Render(separator))
	sb.WriteString("\n")

	// Preview section with scroll offset
	if len(m.previewContent) > 0 {
		// Ensure previewOffset is valid
		maxOffset := max(0, len(m.previewContent)-previewHeight)
		if m.previewOffset > maxOffset {
			m.previewOffset = maxOffset
		}

		for i := 0; i < previewHeight; i++ {
			lineIdx := m.previewOffset + i
			if lineIdx >= len(m.previewContent) {
				sb.WriteString("\n")
				continue
			}
			previewLine := m.previewContent[lineIdx]
			// Truncate line if too wide (rune-aware)
			previewLine = truncateRuneString(previewLine, m.width-1)
			sb.WriteString(m.stylePreview.Render(previewLine))
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString(m.styleDim.Render("  (No preview available)"))
		sb.WriteString("\n")
	}

	// Help
	sb.WriteString(m.styleHelp.Render("↑/↓: Select  ←/→: Navigate  Enter: Open/Select  Esc: Cancel"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion // Enable mouse support
	if m.input.Focused() {
		cursor := tea.NewCursor(2+inputRender.CursorX, inputBoxTopY+1)
		cursor.Blink = m.input.Styles.Cursor.Blink
		cursor.Color = m.input.Styles.Cursor.Color
		cursor.Shape = m.input.Styles.Cursor.Shape
		v.Cursor = cursor
	}
	return v
}

// Result returns the action and selected file path.
func (m *FilePicker) Result() (FilePickerAction, string) {
	return m.action, m.selected
}

// RunFilePicker runs the file picker and returns the selected file path.
func RunFilePicker(startDir string) (FilePickerAction, string, error) {
	m := NewFilePicker(startDir)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return FilePickerCancel, "", err
	}

	picker := finalModel.(*FilePicker)
	action, selected := picker.Result()
	return action, selected, nil
}
