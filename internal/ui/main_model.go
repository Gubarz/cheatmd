package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/executor"
	"github.com/gubarz/cheatmd/internal/parser"
)

// ============================================================================
// Cheat Item
// ============================================================================

// cheatItem wraps a Cheat with display metadata
type cheatItem struct {
	cheat      *parser.Cheat
	searchText string
	folder     string
	file       string
}

// newCheatItem creates a cheatItem from a Cheat
func newCheatItem(cheat *parser.Cheat) cheatItem {
	folder := filepath.Base(filepath.Dir(cheat.File))
	file := strings.TrimSuffix(filepath.Base(cheat.File), filepath.Ext(cheat.File))
	searchText := fmt.Sprintf("%s %s %s %s %s",
		folder, file, cheat.Header, cheat.Description, cheat.Command)

	return cheatItem{
		cheat:      cheat,
		searchText: searchText,
		folder:     folder,
		file:       file,
	}
}

// ============================================================================
// Column Config
// ============================================================================

// columnConfig holds display column widths and gaps
type columnConfig struct {
	headerWidth int
	descWidth   int
	cmdWidth    int
	gap         int
}

// loadColumnConfig loads column configuration from config
func loadColumnConfig() columnConfig {
	return columnConfig{
		headerWidth: config.GetColumnHeader(),
		descWidth:   config.GetColumnDesc(),
		cmdWidth:    config.GetColumnCommand(),
		gap:         config.GetColumnGap(),
	}
}

// ============================================================================
// Main Model - Cheat Selection
// ============================================================================

// mainModel is the Bubble Tea model for cheat selection
type mainModel struct {
	cheats    []cheatItem
	filtered  []cheatItem
	cursor    int
	textInput textinput.Model
	width     int
	height    int
	selected  *parser.Cheat
	quitting  bool
	columns   columnConfig
}

// newMainModel creates a new mainModel with the given cheats
func newMainModel(cheats []*parser.Cheat) mainModel {
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	items := make([]cheatItem, len(cheats))
	for i, cheat := range cheats {
		items[i] = newCheatItem(cheat)
	}

	return mainModel{
		cheats:    items,
		filtered:  items,
		textInput: ti,
		columns:   loadColumnConfig(),
	}
}

// Init implements tea.Model
func (m mainModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model
func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if cmd := m.handleKeyPress(msg); cmd != nil {
			return m, cmd
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 4
	}

	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	cmds = append(cmds, tiCmd)

	m.filterCheats()

	return m, tea.Batch(cmds...)
}

// handleKeyPress processes keyboard input
func (m *mainModel) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return tea.Quit
	case "enter":
		if m.cursor < len(m.filtered) {
			m.selected = m.filtered[m.cursor].cheat
			return tea.Quit
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
	}
	return nil
}

// moveCursor moves the cursor by delta, clamping to valid range
func (m *mainModel) moveCursor(delta int) {
	m.cursor += delta
	m.cursor = clamp(m.cursor, 0, max(0, len(m.filtered)-1))
}

// filterCheats filters the cheat list based on the search query
func (m *mainModel) filterCheats() {
	query := strings.TrimSpace(m.textInput.Value())

	if query == "" {
		m.filtered = m.cheats
	} else {
		words := strings.Fields(strings.ToLower(query))
		m.filtered = make([]cheatItem, 0, len(m.cheats))
		for _, item := range m.cheats {
			if matchesAllWords(strings.ToLower(item.searchText), words) {
				m.filtered = append(m.filtered, item)
			}
		}
	}

	m.cursor = clamp(m.cursor, 0, max(0, len(m.filtered)-1))
}

// View implements tea.Model
func (m mainModel) View() string {
	if m.quitting && m.selected == nil {
		return ""
	}
	return m.renderView()
}

// renderView builds the complete view
func (m mainModel) renderView() string {
	width := maxInt(m.width, 80)
	height := maxInt(m.height, 24)

	preview := m.renderPreview(width)
	previewLines := countLines(preview)

	inputLines := 3 // divider + info + input
	listHeight := maxInt(height-previewLines-inputLines, 3)
	list := m.renderList(listHeight)
	listLines := countLines(list)

	padding := maxInt(height-previewLines-listLines-inputLines, 0)

	var b strings.Builder
	b.WriteString(preview)
	b.WriteString(list)
	b.WriteString(strings.Repeat("\n", padding))
	b.WriteString(m.renderInput(width))

	return b.String()
}

// renderPreview renders the preview section for the selected cheat
func (m mainModel) renderPreview(width int) string {
	var b strings.Builder

	if m.cursor < len(m.filtered) {
		item := m.filtered[m.cursor]
		b.WriteString(styles.PreviewPath.Render(item.folder + "/" + item.file))
		b.WriteString("\n")
		b.WriteString(styles.PreviewHeader.Render(item.cheat.Header))
		b.WriteString("\n")

		if item.cheat.Description != "" {
			desc := truncateLines(item.cheat.Description, 2, 200)
			b.WriteString(styles.PreviewDesc.Render(desc))
			b.WriteString("\n")
		}

		b.WriteString("\n")
		cmd := truncateLines(item.cheat.Command, 3, 0)
		b.WriteString(styles.PreviewCmd.Render(cmd))
		b.WriteString("\n")
	} else {
		b.WriteString("\n\n\n\n\n")
	}

	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	return b.String()
}

// renderList renders the scrollable list of cheats
func (m mainModel) renderList(maxHeight int) string {
	if len(m.filtered) == 0 {
		return ""
	}

	start, end := scrollWindow(m.cursor, len(m.filtered), maxHeight)
	gap := strings.Repeat(" ", m.columns.gap)

	var b strings.Builder
	for i := start; i < end; i++ {
		item := m.filtered[i]
		isSelected := i == m.cursor
		b.WriteString(m.renderListItem(item, isSelected, gap))
		b.WriteString("\n")
	}

	return b.String()
}

// renderListItem renders a single list item
func (m mainModel) renderListItem(item cheatItem, selected bool, gap string) string {
	// Prepare styles based on selection
	pStyle := styles.Path
	hStyle := styles.Header
	dStyle := styles.Desc
	cStyle := styles.Command
	if selected {
		pStyle = styles.WithSelection(pStyle)
		hStyle = styles.WithSelection(hStyle)
		dStyle = styles.WithSelection(dStyle)
		cStyle = styles.WithSelection(cStyle)
	}

	// Build header column (path + header)
	pathPart := item.folder + "/" + item.file
	headerPart := item.cheat.Header
	fullHeader := pathPart + " " + headerPart

	if m.columns.headerWidth > 1 && len(fullHeader) > m.columns.headerWidth {
		fullHeader = fullHeader[:m.columns.headerWidth-1] + "…"
		if len(pathPart) >= len(fullHeader) {
			pathPart = fullHeader
			headerPart = ""
		} else {
			headerPart = fullHeader[len(pathPart)+1:]
		}
	}

	var headerRendered string
	if headerPart != "" {
		headerRendered = pStyle.Render(pathPart) + " " + hStyle.Render(headerPart)
	} else {
		headerRendered = pStyle.Render(pathPart)
	}

	// Pad header to column width
	if padding := m.columns.headerWidth - len(fullHeader); padding > 0 {
		if selected {
			headerRendered += styles.Selected.Render(strings.Repeat(" ", padding))
		} else {
			headerRendered += strings.Repeat(" ", padding)
		}
	}

	// Description column
	descFirstLine := strings.Split(item.cheat.Description, "\n")[0]
	shortDesc := truncateString(descFirstLine, m.columns.descWidth)
	descPadded := fmt.Sprintf("%-*s", m.columns.descWidth, shortDesc)

	// Command column
	cmdFirstLine := strings.Split(item.cheat.Command, "\n")[0]
	maxCmd := m.columns.cmdWidth
	if m.width > 0 {
		usedWidth := m.columns.headerWidth + m.columns.gap*2 + m.columns.descWidth + 4
		if available := m.width - usedWidth; available > 0 && available < maxCmd {
			maxCmd = available
		}
	}
	cmd := truncateString(cmdFirstLine, maxCmd)

	// Build line
	gapRendered := gap
	if selected {
		gapRendered = styles.Selected.Render(gap)
	}

	line := headerRendered + gapRendered + dStyle.Render(descPadded) + gapRendered + cStyle.Render(cmd)

	if selected {
		return styles.Cursor.Render("▶ ") + line
	}
	return "  " + line
}

// renderInput renders the input section at the bottom
func (m mainModel) renderInput(width int) string {
	var b strings.Builder
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d/%d", len(m.filtered), len(m.cheats))))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("ESC to exit"))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())
	return b.String()
}

// ============================================================================
// Run TUI
// ============================================================================

// getTTY returns file handles for TUI input/output
// Uses /dev/tty to bypass shell pipes and command substitution
func getTTY() (in *os.File, out *os.File, cleanup func()) {
	var closers []func()

	// Check if stdout is a terminal
	// If not (e.g., piped or captured by $()), use /dev/tty
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		// stdout is NOT a terminal - we're being captured
		out, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			out = os.Stderr // Last resort fallback
		} else {
			closers = append(closers, func() { out.Close() })
		}

		in, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
		if err != nil {
			in = os.Stdin
		} else {
			closers = append(closers, func() { in.Close() })
		}

		// Tell lipgloss to use the TTY for color detection
		lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(out))

		return in, out, func() {
			for _, c := range closers {
				c()
			}
		}
	}

	// stdout IS a terminal - use normal stdin/stdout
	return os.Stdin, os.Stdout, func() {}
}

// RunTUI launches the Bubble Tea interface
func RunTUI(index *parser.CheatIndex, exec *executor.Executor, initialQuery, matchCmd string) error {
	requireCheatBlock := config.GetRequireCheatBlock()
	autoSelect := config.GetAutoSelect()

	for {
		cheats := filterCheatsByConfig(index.Cheats, requireCheatBlock)
		if len(cheats) == 0 {
			return fmt.Errorf("no cheats found")
		}

		m := newMainModel(cheats)

		// If matchCmd is provided, try to find a cheat whose command matches
		if matchCmd != "" {
			if matched := findMatchingCheat(cheats, matchCmd); matched != nil {
				m.selected = matched
				// Pre-fill scope from the matched command
				prefillScopeFromMatch(matched, matchCmd)
			} else {
				// No exact match - use as initial query
				initialQuery = matchCmd
			}
			matchCmd = "" // Only try to match once
		}

		if initialQuery != "" {
			m.textInput.SetValue(initialQuery)
			m.filterCheats()

			// Auto-select if exactly one match and --auto flag is set
			if autoSelect && len(m.filtered) == 1 {
				m.selected = m.filtered[0].cheat
			}
		}

		// Skip TUI if already auto-selected
		if m.selected == nil {
			ttyIn, ttyOut, cleanup := getTTY()
			RefreshStyles() // Refresh after getTTY sets up the renderer
			p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(ttyOut), tea.WithInput(ttyIn))
			finalModel, err := p.Run()
			cleanup()

			if err != nil {
				return err
			}

			result := finalModel.(mainModel)
			if result.quitting && result.selected == nil {
				return nil
			}
			if result.selected == nil {
				continue
			}
			m.selected = result.selected
		}

		cheat := m.selected
		// Initialize scope if nil, but preserve existing values (from --match)
		if cheat.Scope == nil {
			cheat.Scope = make(map[string]string)
		}

		goBack, err := resolveAllVariables(cheat, index, exec)
		if err != nil {
			return err
		}
		if goBack {
			initialQuery = ""
			continue
		}

		finalCmd := exec.BuildFinalCommand(cheat)
		return executeOutput(finalCmd, exec)
	}
}

// filterCheatsByConfig returns cheats matching configuration
func filterCheatsByConfig(cheats []*parser.Cheat, requireCheatBlock bool) []*parser.Cheat {
	if !requireCheatBlock {
		return cheats
	}

	result := make([]*parser.Cheat, 0, len(cheats))
	for _, cheat := range cheats {
		if cheat.HasCheatBlock {
			result = append(result, cheat)
		}
	}
	return result
}

// ============================================================================
// Helpers
// ============================================================================

// clamp restricts v to the range [minV, maxV]
func clamp(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// maxInt returns the larger of a and b
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// countLines counts the number of lines in a string
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// scrollWindow calculates the visible range for a scrollable list
func scrollWindow(cursor, total, height int) (start, end int) {
	if cursor >= height {
		start = cursor - height + 1
	}
	end = start + height
	if end > total {
		end = total
	}
	return
}

// truncateString truncates a string to maxLen with ellipsis
func truncateString(s string, maxLen int) string {
	if maxLen <= 3 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// truncateLines truncates text to maxLines with optional maxLen per content
func truncateLines(text string, maxLines int, maxLen int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		text = strings.Join(lines[:maxLines], "\n") + "..."
	}
	if maxLen > 0 && len(text) > maxLen {
		text = text[:maxLen-3] + "..."
	}
	return text
}

// matchesAllWords returns true if text contains all words
func matchesAllWords(text string, words []string) bool {
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

// findMatchingCheat finds a cheat whose command pattern matches the input
// It builds a regex from the cheat command (replacing $var with capture groups)
// and returns the first match
func findMatchingCheat(cheats []*parser.Cheat, input string) *parser.Cheat {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	for _, cheat := range cheats {
		pattern := buildMatchPattern(cheat.Command)
		if pattern.MatchString(input) {
			return cheat
		}
	}
	return nil
}

// buildMatchPattern converts a command template to a regex pattern for matching
// e.g. "echo $name" -> "^echo (\S+)$"
// e.g. 'echo "$name"' -> '^echo "([^"]*)"$'
func buildMatchPattern(cmd string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(cmd)
	// After QuoteMeta: "$var" becomes "\$var" (quotes not escaped, $ is escaped)
	// Replace "\$var" inside double quotes with "([^"]*)"
	quotedVarPattern := regexp.MustCompile(`"\\\$(\w+)"`)
	escaped = quotedVarPattern.ReplaceAllString(escaped, `"([^"]*)"`)
	// Same for single quotes
	singleQuotedVarPattern := regexp.MustCompile(`'\\\$(\w+)'`)
	escaped = singleQuotedVarPattern.ReplaceAllString(escaped, `'([^']*)'`)
	// Replace remaining unquoted $var with non-whitespace match
	varPattern := regexp.MustCompile(`\\\$(\w+)`)
	escaped = varPattern.ReplaceAllString(escaped, `(\S+)`)
	pattern := `^\s*` + escaped + `\s*$`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return regexp.MustCompile(`^$`)
	}
	return re
}

// prefillScopeFromMatch extracts variable values from the matched command
func prefillScopeFromMatch(cheat *parser.Cheat, input string) {
	input = strings.TrimSpace(input)
	pattern := buildMatchPattern(cheat.Command)
	if pattern == nil {
		return
	}

	matches := pattern.FindStringSubmatch(input)
	if matches == nil {
		return
	}

	if cheat.Scope == nil {
		cheat.Scope = make(map[string]string)
	}

	varNames := extractVarNames(cheat.Command)
	for i, name := range varNames {
		if i+1 < len(matches) {
			cheat.Scope[name] = matches[i+1]
		}
	}
}

// extractVarNames returns variable names in order of appearance
func extractVarNames(cmd string) []string {
	varPattern := regexp.MustCompile(`\$(\w+)`)
	matches := varPattern.FindAllStringSubmatch(cmd, -1)
	var names []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if !seen[m[1]] {
			names = append(names, m[1])
			seen[m[1]] = true
		}
	}
	return names
}
