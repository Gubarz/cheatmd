package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gubarz/cheatmd/pkg/config"
)

// substituteSearchState holds the overlay state for substitute search.
// The user enters via the configured key during phaseVarResolve, picks an
// env/history value, and returns to phaseVarResolve with the chosen value
// loaded into the var prompt.
type substituteSearchState struct {
	picker *Picker[substituteOption]

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
		picker: NewPicker(opts, func(opt substituteOption, words []string) bool {
			hay := strings.ToLower(opt.Display)
			return matchesAllWords(hay, words)
		}),
		prevInput:  m.textInput.Value(),
		prevCursor: m.picker.Cursor,
		prevOffset: m.picker.Offset,
	}
	m.textInput.SetValue("")
	m.textInput.Placeholder = "Search env / history..."
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

	if accept {
		if opt, ok := m.subState.picker.Selected(); ok {
			m.textInput.SetValue(opt.Value)
			m.textInput.CursorEnd()
		} else {
			m.textInput.SetValue(m.subState.prevInput)
			m.textInput.CursorEnd()
		}
	} else {
		m.textInput.SetValue(m.subState.prevInput)
		m.textInput.CursorEnd()
	}
	m.picker.Cursor = m.subState.prevCursor
	m.picker.Offset = m.subState.prevOffset
	m.subState = nil
	m.textInput.Placeholder = "Type to filter or enter value..."
	m.phase = phaseVarResolve

	// Refilter var options if we're in select mode (the input may have changed).
	if m.varState != nil && !m.varState.isPromptOnly && m.varState.picker != nil {
		m.varState.picker.Filter(m.textInput.Value())
	}
}

// filterSubstituteOptions applies the textInput's current value as a
// space-separated AND fuzzy filter over the substitute option list.
func (m *mainModel) filterSubstituteOptions() {
	if m.subState != nil {
		m.subState.picker.Filter(m.textInput.Value())
	}
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
	}
	if m.subState != nil && m.subState.picker.HandleKey(msg) {
		return nil
	}
	return nil
}

// renderSubstituteSearch renders the env/history picker overlay using the
// shared overlay layout.
func (m *mainModel) renderSubstituteSearch() string {
	extra := ""
	matchCount := 0
	var items []string

	if m.subState != nil {
		var varName string
		if m.varState != nil && m.varState.currentIdx < len(m.varState.vars) {
			varName = m.varState.vars[m.varState.currentIdx].def.Name
		}
		if varName != "" {
			extra = styles.Dim.Render("→ ") + styles.Cursor.Render("$" + varName)
		}
		
		matchCount = len(m.subState.picker.Filtered)
		for _, opt := range m.subState.picker.Filtered {
			items = append(items, opt.Display)
		}
	}

	var offset *int
	var cursor int
	if m.subState != nil {
		offset = &m.subState.picker.Offset
		cursor = m.subState.picker.Cursor
	} else {
		zero := 0
		offset = &zero
	}

	return m.renderOverlayWindow(OverlayConfig{
		Title:         "Substitute search",
		TitleExtra:    extra,
		MatchesCount:  matchCount,
		EnterHint:     "Enter use value",
		Items:         items,
		SelectedIndex: cursor,
		Offset:        offset,
		Input:         m.textInput,
	})
}
