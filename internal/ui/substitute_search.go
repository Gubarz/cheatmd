package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gubarz/cheatmd/internal/config"
)

// substituteSearchState holds the overlay state for substitute search.
// The user enters via the configured key during phaseVarResolve, picks an
// env/history value, and returns to phaseVarResolve with the chosen value
// loaded into the var prompt.
type substituteSearchState struct {
	options    []substituteOption
	filtered   []substituteOption
	cursor     int
	offset     int
	prevInput  string // textInput value before entering the overlay
	prevCursor int
	prevOffset int
}

// updateSubstituteSearch handles updates while the substitute overlay is open.
func (m *mainModel) updateSubstituteSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if cmd := m.handleSubstituteSearchKey(msg); cmd != nil {
			return m, cmd
		}
		// If the key was a navigation/accept/cancel key we already handled it;
		// otherwise fall through and let the text input absorb it.
		if isSubstituteNavKey(msg.String()) {
			return m, nil
		}
	}

	prev := m.textInput.Value()
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	if m.textInput.Value() != prev {
		m.filterSubstituteOptions()
	}
	return m, tiCmd
}

// isSubstituteNavKey reports whether key is a navigation/accept/cancel key
// that the overlay handles directly (rather than passing to the text input).
func isSubstituteNavKey(key string) bool {
	switch key {
	case "ctrl+c", "esc", "enter", "up", "down", "ctrl+p", "ctrl+n", "pgup", "pgdown":
		return true
	}
	return false
}

// enterSubstituteSearch transitions from phaseVarResolve into the substitute
// search overlay. Returns true if the transition happened; false if disabled
// or there are no sources to show.
func (m *mainModel) enterSubstituteSearch() bool {
	sources := config.GetSubstituteSources()
	if len(sources) == 0 {
		return false
	}
	opts := collectSubstituteOptions(sources)
	if len(opts) == 0 {
		return false
	}
	m.subState = &substituteSearchState{
		options:    opts,
		filtered:   opts,
		prevInput:  m.textInput.Value(),
		prevCursor: m.cursor,
		prevOffset: m.offset,
	}
	m.textInput.SetValue("")
	m.textInput.Placeholder = "Search env / history..."
	m.cursor = 0
	m.offset = 0
	m.phase = phaseSubstituteSearch
	return true
}

// exitSubstituteSearch returns to phaseVarResolve. If accept is true the
// currently highlighted option's Value is loaded into the var prompt;
// otherwise the previous input is restored.
func (m *mainModel) exitSubstituteSearch(accept bool) {
	if m.subState == nil {
		m.phase = phaseVarResolve
		return
	}

	if accept && m.subState.cursor < len(m.subState.filtered) {
		m.textInput.SetValue(m.subState.filtered[m.subState.cursor].Value)
		m.textInput.CursorEnd()
	} else {
		m.textInput.SetValue(m.subState.prevInput)
		m.textInput.CursorEnd()
	}
	m.cursor = m.subState.prevCursor
	m.offset = m.subState.prevOffset
	m.subState = nil
	m.textInput.Placeholder = "Type to filter or enter value..."
	m.phase = phaseVarResolve

	// Refilter var options if we're in select mode (the input may have changed).
	if m.varState != nil && !m.varState.isPromptOnly {
		m.filterVarOptions()
	}
}

// filterSubstituteOptions applies the textInput's current value as a
// space-separated AND fuzzy filter over the substitute option list.
func (m *mainModel) filterSubstituteOptions() {
	if m.subState == nil {
		return
	}
	query := strings.ToLower(strings.TrimSpace(m.textInput.Value()))
	if query == "" {
		m.subState.filtered = m.subState.options
		m.subState.cursor = 0
		m.subState.offset = 0
		return
	}
	words := strings.Fields(query)
	result := make([]substituteOption, 0, len(m.subState.options))
	for _, opt := range m.subState.options {
		hay := strings.ToLower(opt.Display)
		if matchesAllWords(hay, words) {
			result = append(result, opt)
		}
	}
	m.subState.filtered = result
	if m.subState.cursor >= len(result) {
		m.subState.cursor = max(0, len(result)-1)
	}
	if m.subState.offset > m.subState.cursor {
		m.subState.offset = m.subState.cursor
	}
}

// moveSubstituteCursor clamps the cursor; offset is reconciled by scrollWindow
// at render time using the actual list height.
func (m *mainModel) moveSubstituteCursor(delta int) {
	if m.subState == nil {
		return
	}
	m.subState.cursor += delta
	m.subState.cursor = clamp(m.subState.cursor, 0, max(0, len(m.subState.filtered)-1))
}

// handleSubstituteSearchKey processes keys while in phaseSubstituteSearch.
func (m *mainModel) handleSubstituteSearchKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		m.selected = nil
		return tea.Quit
	case "esc":
		m.exitSubstituteSearch(false)
		return tea.ClearScreen
	case "enter":
		m.exitSubstituteSearch(true)
		return tea.ClearScreen
	case "up", "ctrl+p":
		m.moveSubstituteCursor(-1)
		return nil
	case "down", "ctrl+n":
		m.moveSubstituteCursor(1)
		return nil
	case "pgup":
		m.moveSubstituteCursor(-10)
		return nil
	case "pgdown":
		m.moveSubstituteCursor(10)
		return nil
	}
	return nil
}

// renderSubstituteSearch renders the env/history picker overlay using the
// same layout shape as renderCheatSelect: fixed-height preview at top,
// scrolling list in the middle, padding, divider + info + input at bottom.
func (m *mainModel) renderSubstituteSearch() string {
	width := max(m.width, 80)
	height := m.height
	if height < 1 {
		height = 24
	}

	inputLines := 3 // divider + info + input

	// Preview block is just two lines: title + divider. Match the cheat
	// select pattern that uses a padded fixed-height block.
	previewHeight := 2
	preview := m.renderSubstitutePreview(width, previewHeight)
	
	previewLines := countLines(preview)
	listHeight := max(height-previewLines-inputLines, 1)
	list := m.renderSubstituteList(listHeight)

	return renderWindowLayout(height, preview, list, m.renderSubstituteInput(width))
}

// renderSubstitutePreview renders the title header at fixed height (padded),
// followed by a divider. Mirrors renderPreviewWithHeight's shape.
func (m *mainModel) renderSubstitutePreview(width, maxLines int) string {
	b := getBuilder()
	defer putBuilder(b)
	lines := 0

	if lines < maxLines {
		var varName string
		if m.varState != nil && m.varState.currentIdx < len(m.varState.vars) {
			varName = m.varState.vars[m.varState.currentIdx].def.Name
		}
		b.WriteString(styles.Header.Render("Substitute search"))
		if varName != "" {
			b.WriteString("  ")
			b.WriteString(styles.Dim.Render("→ "))
			b.WriteString(styles.Cursor.Render("$" + varName))
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

// renderSubstituteList renders the scrolling list, mirrors renderList shape.
// Each row is hard-truncated to terminal width so long env values (e.g. PATH)
// can't wrap and push other rows off-screen.
func (m *mainModel) renderSubstituteList(maxHeight int) string {
	if m.subState == nil || len(m.subState.filtered) == 0 {
		return ""
	}

	start, end := scrollWindow(m.subState.cursor, len(m.subState.filtered), maxHeight, &m.subState.offset)
	width := max(m.width, 80)
	// 2 chars for the "▶ " or "  " prefix.
	maxLen := max(width-2, 10)

	b := getBuilder()
	defer putBuilder(b)
	for i := start; i < end; i++ {
		opt := m.subState.filtered[i]
		display := truncateString(opt.Display, maxLen)
		if i == m.subState.cursor {
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

// renderSubstituteInput renders the bottom divider + hint + input, mirrors
// renderInput's shape.
func (m *mainModel) renderSubstituteInput(width int) string {
	b := getBuilder()
	defer putBuilder(b)
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	matchCount := 0
	if m.subState != nil {
		matchCount = len(m.subState.filtered)
	}
	b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d matches", matchCount)))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("ESC cancel"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("Enter use value"))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())
	return b.String()
}
