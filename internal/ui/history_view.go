package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/history"
	"github.com/gubarz/cheatmd/pkg/parser"
)

// historyState holds the overlay state for the execution-history picker.
type historyState struct {
	picker *Picker[history.Entry]

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
		picker: NewPicker(entries, func(e history.Entry, words []string) bool {
			hay := strings.ToLower(e.Command + " " + e.Header + " " + e.File)
			return matchesAllWords(hay, words)
		}),
		prevInput:  m.textInput.Value(),
		prevCursor: m.picker.Cursor,
		prevOffset: m.picker.Offset,
	}
	m.textInput.SetValue("")
	m.textInput.Placeholder = "Search history..."
	m.phase = phaseHistory
	return true
}

// exitHistory returns to phaseCheatSelect without selecting an entry.
func (m *mainModel) exitHistory() {
	if m.histState != nil {
		m.textInput.SetValue(m.histState.prevInput)
		m.picker.Cursor = m.histState.prevCursor
		m.picker.Offset = m.histState.prevOffset
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
	if m.histState == nil {
		m.exitHistory()
		return nil
	}
	entry, ok := m.histState.picker.Selected()
	if !ok {
		m.exitHistory()
		return nil
	}
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
	if m.histState != nil {
		m.histState.picker.Filter(m.textInput.Value())
	}
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
	}
	if m.histState != nil && m.histState.picker.HandleKey(msg) {
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

// renderHistory renders the history overlay using the shared overlay layout.
func (m *mainModel) renderHistory() string {
	extra := ""
	matchCount := 0
	var items []string
	if m.histState != nil {
		extra = styles.Dim.Render(fmt.Sprintf("(%d entries)", len(m.histState.picker.Items)))
		matchCount = len(m.histState.picker.Filtered)
		for _, e := range m.histState.picker.Filtered {
			items = append(items, e.Display(max(m.width-2, 10)))
		}
	}

	var offset *int
	var cursor int
	if m.histState != nil {
		offset = &m.histState.picker.Offset
		cursor = m.histState.picker.Cursor
	} else {
		zero := 0
		offset = &zero
	}

	return m.renderOverlayWindow(OverlayConfig{
		Title:         "History",
		TitleExtra:    extra,
		MatchesCount:  matchCount,
		EnterHint:     "Enter re-run cheat",
		Items:         items,
		SelectedIndex: cursor,
		Offset:        offset,
		Input:         m.textInput,
	})
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
