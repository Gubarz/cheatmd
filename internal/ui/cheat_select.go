package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/parser"
)

// ============================================================================
// Cheat Item
// ============================================================================

// cheatItem wraps a Cheat with display metadata.
type cheatItem struct {
	cheat  *parser.Cheat
	folder string
	file   string
}

func newCheatItem(cheat *parser.Cheat) cheatItem {
	folder := filepath.Base(filepath.Dir(cheat.File))
	file := strings.TrimSuffix(filepath.Base(cheat.File), filepath.Ext(cheat.File))

	return cheatItem{
		cheat:  cheat,
		folder: folder,
		file:   file,
	}
}

// matchesQuery reports whether the cheat item matches all search words.
// Words must be pre-lowercased.
func (item *cheatItem) matchesQuery(words []string) bool {
	for _, word := range words {
		if !item.containsWord(word) {
			return false
		}
	}
	return true
}

// containsWord reports whether any of the item's searchable fields contains
// the given lowercased word.
func (item *cheatItem) containsWord(word string) bool {
	if containsIgnoreCaseFast(item.folder, word) {
		return true
	}
	if containsIgnoreCaseFast(item.file, word) {
		return true
	}
	if containsIgnoreCaseFast(item.cheat.Header, word) {
		return true
	}
	if containsIgnoreCaseFast(item.cheat.Description, word) {
		return true
	}
	if containsIgnoreCaseFast(item.cheat.Command, word) {
		return true
	}
	for _, tag := range item.cheat.Tags {
		// tags are already lowercased by the parser, but containsIgnoreCaseFast is safe
		if containsIgnoreCaseFast(tag, word) {
			return true
		}
	}
	return false
}

// containsIgnoreCaseFast is a fast, zero-allocation case-insensitive substring check.
// It assumes lowerSubstr is already lowercased.
func containsIgnoreCaseFast(s, lowerSubstr string) bool {
	if len(lowerSubstr) == 0 {
		return true
	}
	if len(lowerSubstr) > len(s) {
		return false
	}
	n := len(lowerSubstr)
	for i := 0; i <= len(s)-n; i++ {
		match := true
		for j := 0; j < n; j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != lowerSubstr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// buildPathDisplay builds the path display string based on config options.
func buildPathDisplay(folder, file string) string {
	showFolder := config.GetShowFolder()
	showFile := config.GetShowFile()

	if showFolder && showFile {
		return folder + "/" + file
	} else if showFolder {
		return folder
	} else if showFile {
		return file
	}
	return ""
}

// ============================================================================
// Column Config
// ============================================================================

// columnConfig holds display column widths and gaps.
type columnConfig struct {
	headerWidth int
	descWidth   int
	cmdWidth    int
	gap         int
}

// loadColumnConfig loads column configuration from config.
func loadColumnConfig() columnConfig {
	return columnConfig{
		headerWidth: config.GetColumnHeader(),
		descWidth:   config.GetColumnDesc(),
		cmdWidth:    config.GetColumnCommand(),
		gap:         config.GetColumnGap(),
	}
}

// ============================================================================
// Debounce
// ============================================================================

// filterMsg triggers filtering after debounce.
type filterMsg struct{}

// debounceFilter returns a command that triggers filtering after a delay.
func debounceFilter() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return filterMsg{}
	})
}

// ============================================================================
// Update
// ============================================================================

// updateCheatSelect handles updates during cheat selection phase.
func (m *mainModel) updateCheatSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if cmd := m.handleCheatSelectKey(msg); cmd != nil {
			return m, cmd
		}
	case filterMsg:
		m.filterCheats()
		return m, nil
	}

	prevQuery := m.textInput.Value()
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	cmds = append(cmds, tiCmd)

	if m.textInput.Value() != prevQuery {
		cmds = append(cmds, debounceFilter())
	}

	return m, tea.Batch(cmds...)
}

// handleCheatSelectKey processes keyboard input during cheat selection.
func (m *mainModel) handleCheatSelectKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return tea.Quit
	case "enter":
		if m.cursor < len(m.filtered) {
			m.selected = m.filtered[m.cursor].cheat
			return m.startVarResolution()
		}
	case "up", "ctrl+p":
		m.moveCursor(-1)
	case "down", "ctrl+n":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-10)
	case "pgdown":
		m.moveCursor(10)
	case "home", "ctrl+a":
		m.cursor = 0
	case "end", "ctrl+e":
		m.cursor = max(0, len(m.filtered)-1)
	default:
		if msg.String() == config.GetKeyOpen() {
			if m.cursor < len(m.filtered) {
				openFileInViewer(m.filtered[m.cursor].cheat.File)
			}
		}
		if msg.String() == config.GetKeyPreview() {
			if m.cursor < len(m.filtered) {
				if m.enterPreview(m.filtered[m.cursor].cheat) {
					return tea.ClearScreen
				}
			}
		}
		if msg.String() == config.GetKeyHistory() {
			if m.enterHistory() {
				return tea.ClearScreen
			}
		}
	}
	return nil
}

func (m *mainModel) moveCursor(delta int) {
	m.cursor += delta
	m.cursor = clamp(m.cursor, 0, max(0, len(m.filtered)-1))
}



// filterCheats filters the cheat list based on the search query.
func (m *mainModel) filterCheats() {
	query := strings.TrimSpace(m.textInput.Value())

	if query == "" {
		m.filtered = m.cheats
	} else {
		words := strings.Fields(strings.ToLower(query))
		m.filtered = make([]cheatItem, 0, min(len(m.cheats), 1000))
		for i := range m.cheats {
			if m.cheats[i].matchesQuery(words) {
				m.filtered = append(m.filtered, m.cheats[i])
				if len(m.filtered) >= 1000 {
					break
				}
			}
		}
	}

	m.cursor = clamp(m.cursor, 0, max(0, len(m.filtered)-1))
}

// ============================================================================
// Render
// ============================================================================

// renderCheatSelect builds the cheat selection view.
func (m *mainModel) renderCheatSelect() string {
	width := max(m.width, 80)
	height := m.height
	if height < 1 {
		height = 24
	}

	inputLines := 3 // divider + info + input

	previewHeight := config.GetPreviewHeight()
	if previewHeight < 1 {
		previewHeight = 6
	}

	minListHeight := 3

	availableForPreviewAndList := height - inputLines
	if availableForPreviewAndList < previewHeight+minListHeight {
		previewHeight = max(availableForPreviewAndList-minListHeight, 2)
	}

	preview := m.renderPreviewWithHeight(width, previewHeight)
	listHeight := max(height-countLines(preview)-inputLines, 1)
	list := m.renderList(listHeight)

	return renderWindowLayout(height, preview, list, m.renderInput(width))
}

// renderPreviewWithHeight renders the preview section with a fixed height.
func (m *mainModel) renderPreviewWithHeight(width int, maxLines int) string {
	b := getBuilder()
	defer putBuilder(b)
	lines := 0

	if m.cursor < len(m.filtered) {
		item := m.filtered[m.cursor]
		pathDisplay := buildPathDisplay(item.folder, item.file)
		if pathDisplay != "" && lines < maxLines {
			b.WriteString(styles.PreviewPath.Render(pathDisplay))
			b.WriteString("\n")
			lines++
		}

		if lines < maxLines {
			b.WriteString(styles.PreviewHeader.Render(item.cheat.Header))
			b.WriteString("\n")
			lines++
		}

		if item.cheat.Description != "" && lines < maxLines {
			desc := truncateLines(item.cheat.Description, 1, 200)
			b.WriteString(styles.PreviewDesc.Render(desc))
			b.WriteString("\n")
			lines++
		}

		if lines < maxLines {
			b.WriteString("\n")
			lines++
		}

		if lines < maxLines {
			cmd := truncateLines(item.cheat.Command, maxLines-lines, 0)
			cmdLines := strings.Count(cmd, "\n") + 1
			b.WriteString(styles.PreviewCmd.Render(cmd))
			b.WriteString("\n")
			lines += cmdLines
		}
	}

	for lines < maxLines {
		b.WriteString("\n")
		lines++
	}

	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	return b.String()
}

// renderList renders the scrollable list of cheats.
func (m *mainModel) renderList(maxHeight int) string {
	if len(m.filtered) == 0 {
		return ""
	}

	start, end := scrollWindow(m.cursor, len(m.filtered), maxHeight, &m.offset)
	gap := strings.Repeat(" ", m.columns.gap)

	b := getBuilder()
	defer putBuilder(b)
	for i := start; i < end; i++ {
		item := m.filtered[i]
		isSelected := i == m.cursor
		b.WriteString(m.renderListItem(item, isSelected, gap))
		b.WriteString("\n")
	}

	return b.String()
}

// renderListItem renders a single list item.
func (m *mainModel) renderListItem(item cheatItem, selected bool, gap string) string {
	pStyle, hStyle, dStyle, cStyle := m.getItemStyles(selected)

	pathPart := buildPathDisplay(item.folder, item.file)
	headerPart := item.cheat.Header
	headerRendered := m.renderHeaderColumn(pathPart, headerPart, pStyle, hStyle, selected)

	desc := truncateString(firstLine(item.cheat.Description), m.columns.descWidth)
	descPadded := fmt.Sprintf("%-*s", m.columns.descWidth, desc)

	maxCmd := m.calculateCommandWidth()
	cmd := truncateString(firstLine(item.cheat.Command), maxCmd)

	gapStr := gap
	if selected {
		gapStr = styles.Selected.Render(gap)
	}

	line := headerRendered + gapStr + dStyle.Render(descPadded) + gapStr + cStyle.Render(cmd)
	if selected {
		return styles.Cursor.Render("▶ ") + line
	}
	return "  " + line
}

// getItemStyles returns the appropriate styles based on selection state.
func (m *mainModel) getItemStyles(selected bool) (path, header, desc, cmd lipgloss.Style) {
	path, header, desc, cmd = styles.Path, styles.Header, styles.Desc, styles.Command
	if selected {
		path = styles.WithSelection(path)
		header = styles.WithSelection(header)
		desc = styles.WithSelection(desc)
		cmd = styles.WithSelection(cmd)
	}
	return
}

// renderHeaderColumn renders the path+header column with proper truncation.
func (m *mainModel) renderHeaderColumn(pathPart, headerPart string, pStyle, hStyle lipgloss.Style, selected bool) string {
	var fullHeader string
	if pathPart != "" {
		fullHeader = pathPart + " " + headerPart
	} else {
		fullHeader = headerPart
	}

	if m.columns.headerWidth > 1 && len(fullHeader) > m.columns.headerWidth {
		fullHeader = fullHeader[:m.columns.headerWidth-1] + "…"
		if pathPart != "" && len(pathPart) >= len(fullHeader) {
			pathPart = fullHeader
			headerPart = ""
		} else if pathPart != "" {
			headerPart = fullHeader[len(pathPart)+1:]
		} else {
			headerPart = fullHeader
		}
	}

	var rendered string
	if pathPart != "" && headerPart != "" {
		rendered = pStyle.Render(pathPart) + " " + hStyle.Render(headerPart)
	} else if pathPart != "" {
		rendered = pStyle.Render(pathPart)
	} else {
		rendered = hStyle.Render(headerPart)
	}

	if padding := m.columns.headerWidth - len(fullHeader); padding > 0 {
		padStr := strings.Repeat(" ", padding)
		if selected {
			padStr = styles.Selected.Render(padStr)
		}
		rendered += padStr
	}
	return rendered
}

// calculateCommandWidth returns the available width for command column.
func (m *mainModel) calculateCommandWidth() int {
	maxCmd := m.columns.cmdWidth
	if m.width > 0 {
		usedWidth := m.columns.headerWidth + m.columns.gap*2 + m.columns.descWidth + 4
		if available := m.width - usedWidth; available > 0 && available < maxCmd {
			maxCmd = available
		}
	}
	return maxCmd
}

// renderInput renders the input section at the bottom.
func (m *mainModel) renderInput(width int) string {
	b := getBuilder()
	defer putBuilder(b)
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d/%d", len(m.filtered), len(m.cheats))))
	b.WriteString(" • ")
	keyOpen := config.GetKeyOpen()
	if keyOpen == "" {
		keyOpen = "ctrl+o"
	}
	b.WriteString(styles.Dim.Render(formatKeyDisplay(keyOpen) + " open"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("ESC exit"))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())
	return b.String()
}
