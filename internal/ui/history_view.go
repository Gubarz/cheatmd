package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/history"
	"github.com/gubarz/cheatmd/internal/parser"
)

// historyState holds the overlay state for the execution-history picker.
type historyState struct {
	entries  []history.Entry // all loaded entries, newest first
	filtered []history.Entry
	cursor   int
	offset   int
	// Saved cheat-select state to restore on cancel.
	prevInput  string
	prevCursor int
	prevOffset int
}

// enterHistory transitions from phaseCheatSelect into the history overlay.
// Returns true on success, false if there are no entries or history is
// otherwise unavailable.
func (m *mainModel) enterHistory() bool {
	path, err := history.DefaultPath(config.GetHistoryFile())
	if err != nil {
		return false
	}
	entries, err := history.Load(path, config.GetHistoryMax())
	if err != nil || len(entries) == 0 {
		return false
	}
	m.histState = &historyState{
		entries:    entries,
		filtered:   entries,
		prevInput:  m.textInput.Value(),
		prevCursor: m.cursor,
		prevOffset: m.offset,
	}
	m.textInput.SetValue("")
	m.textInput.Placeholder = "Search history..."
	m.cursor = 0
	m.offset = 0
	m.phase = phaseHistory
	return true
}

// exitHistory returns to phaseCheatSelect without selecting an entry.
func (m *mainModel) exitHistory() {
	if m.histState != nil {
		m.textInput.SetValue(m.histState.prevInput)
		m.cursor = m.histState.prevCursor
		m.offset = m.histState.prevOffset
	}
	m.histState = nil
	m.textInput.Placeholder = "Type to search..."
	m.phase = phaseCheatSelect
}

// acceptHistory takes the currently highlighted entry, finds its cheat in
// the index, copies the recorded scope into it, and transitions to var
// resolution. If the cheat is no longer in the index (file renamed, header
// changed) it falls back to inserting the raw command into the prompt.
func (m *mainModel) acceptHistory() tea.Cmd {
	if m.histState == nil || m.histState.cursor >= len(m.histState.filtered) {
		m.exitHistory()
		return nil
	}
	entry := m.histState.filtered[m.histState.cursor]
	cheat := findCheatByRef(m.cheatIndex, entry.File, entry.Header)
	if cheat == nil {
		// Cheat no longer exists. Bail back to cheat select with the command
		// as a search query so the user has something to act on.
		m.textInput.SetValue(entry.Command)
		m.exitHistory()
		m.filterCheats()
		return nil
	}

	// Reset and pre-fill the cheat's scope from the recorded entry.
	cheat.Scope = make(map[string]string, len(entry.Scope))
	for k, v := range entry.Scope {
		cheat.Scope[k] = v
	}

	m.histState = nil
	m.textInput.Placeholder = "Type to filter or enter value..."
	m.selected = cheat
	return m.startVarResolution()
}

// filterHistoryEntries applies the current input as a case-insensitive AND
// fuzzy filter over entries (command + header + file).
func (m *mainModel) filterHistoryEntries() {
	if m.histState == nil {
		return
	}
	query := strings.ToLower(strings.TrimSpace(m.textInput.Value()))
	if query == "" {
		m.histState.filtered = m.histState.entries
		m.histState.cursor = 0
		m.histState.offset = 0
		return
	}
	words := strings.Fields(query)
	result := make([]history.Entry, 0, len(m.histState.entries))
	for _, e := range m.histState.entries {
		hay := strings.ToLower(e.Command + " " + e.Header + " " + e.File)
		if matchesAllWords(hay, words) {
			result = append(result, e)
		}
	}
	m.histState.filtered = result
	if m.histState.cursor >= len(result) {
		m.histState.cursor = max(0, len(result)-1)
	}
	if m.histState.offset > m.histState.cursor {
		m.histState.offset = m.histState.cursor
	}
}

// moveHistoryCursor clamps the cursor; offset is reconciled at render time.
func (m *mainModel) moveHistoryCursor(delta int) {
	if m.histState == nil {
		return
	}
	m.histState.cursor += delta
	m.histState.cursor = clamp(m.histState.cursor, 0, max(0, len(m.histState.filtered)-1))
}

// handleHistoryKey processes keys while in phaseHistory.
func (m *mainModel) handleHistoryKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		m.selected = nil
		return tea.Quit
	case "esc":
		m.exitHistory()
		return tea.ClearScreen
	case "enter":
		return m.acceptHistory()
	case "up", "ctrl+p":
		m.moveHistoryCursor(-1)
		return nil
	case "down", "ctrl+n":
		m.moveHistoryCursor(1)
		return nil
	case "pgup":
		m.moveHistoryCursor(-10)
		return nil
	case "pgdown":
		m.moveHistoryCursor(10)
		return nil
	}
	return nil
}

// updateHistory handles updates while the history overlay is open.
func (m *mainModel) updateHistory(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if cmd := m.handleHistoryKey(msg); cmd != nil {
			return m, cmd
		}
		if isHistoryNavKey(msg.String()) {
			return m, nil
		}
	}

	prev := m.textInput.Value()
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	if m.textInput.Value() != prev {
		m.filterHistoryEntries()
	}
	return m, tiCmd
}

// isHistoryNavKey mirrors isSubstituteNavKey: navigation/accept/cancel keys
// the overlay swallows rather than passing to the text input.
func isHistoryNavKey(key string) bool {
	switch key {
	case "ctrl+c", "esc", "enter", "up", "down", "ctrl+p", "ctrl+n", "pgup", "pgdown":
		return true
	}
	return false
}

// renderHistory renders the history overlay using the same layout shape as
// renderSubstituteSearch.
func (m *mainModel) renderHistory() string {
	width := max(m.width, 80)
	height := m.height
	if height < 1 {
		height = 24
	}

	inputLines := 3
	previewHeight := 2
	preview := m.renderHistoryPreview(width, previewHeight)

	previewLines := countLines(preview)
	listHeight := max(height-previewLines-inputLines, 1)
	list := m.renderHistoryList(listHeight, width)

	return renderWindowLayout(height, preview, list, m.renderHistoryInput(width))
}

// renderHistoryPreview is the top header: title + divider, padded to fit.
func (m *mainModel) renderHistoryPreview(width, maxLines int) string {
	b := getBuilder()
	defer putBuilder(b)
	lines := 0

	if lines < maxLines {
		b.WriteString(styles.Header.Render("History"))
		if m.histState != nil {
			b.WriteString("  ")
			b.WriteString(styles.Dim.Render(fmt.Sprintf("(%d entries)", len(m.histState.entries))))
		}
		b.WriteString("\n")
		lines++
	}
	for lines < maxLines {
		b.WriteString("\n")
		lines++
	}
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	return b.String()
}

// renderHistoryList renders the scrolling list of history entries.
func (m *mainModel) renderHistoryList(maxHeight, width int) string {
	if m.histState == nil || len(m.histState.filtered) == 0 {
		return ""
	}

	start, end := scrollWindow(m.histState.cursor, len(m.histState.filtered), maxHeight, &m.histState.offset)
	maxLen := max(width-2, 10)

	b := getBuilder()
	defer putBuilder(b)
	for i := start; i < end; i++ {
		entry := m.histState.filtered[i]
		display := entry.Display(maxLen)
		if i == m.histState.cursor {
			b.WriteString(styles.Cursor.Render("▶ "))
			b.WriteString(styles.Selected.Render(styles.Command.Render(display)))
		} else {
			b.WriteString("  ")
			b.WriteString(styles.Command.Render(display))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderHistoryInput renders the bottom divider, hint, and search input.
func (m *mainModel) renderHistoryInput(width int) string {
	b := getBuilder()
	defer putBuilder(b)
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	matchCount := 0
	if m.histState != nil {
		matchCount = len(m.histState.filtered)
	}
	b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d matches", matchCount)))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("ESC cancel"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("Enter re-run cheat"))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())
	return b.String()
}

// findCheatByRef locates a cheat in the index matching the given file path
// and header. Returns nil if no exact match.
func findCheatByRef(index *parser.CheatIndex, file, header string) *parser.Cheat {
	if index == nil {
		return nil
	}
	for _, c := range index.Cheats {
		if c.File == file && c.Header == header {
			return c
		}
	}
	return nil
}
