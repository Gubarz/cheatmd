package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/executor"
	"github.com/gubarz/cheatmd/internal/parser"
)

// ============================================================================
// String Builder Pool - reduces GC pressure from rendering
// ============================================================================

var builderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

func getBuilder() *strings.Builder {
	b := builderPool.Get().(*strings.Builder)
	b.Reset()
	return b
}

func putBuilder(b *strings.Builder) {
	if b.Cap() < 64*1024 { // Don't pool huge builders
		builderPool.Put(b)
	}
}

// ============================================================================
// Cheat Item
// ============================================================================

// cheatItem wraps a Cheat with display metadata
type cheatItem struct {
	cheat  *parser.Cheat
	folder string
	file   string
}

// newCheatItem creates a cheatItem from a Cheat
func newCheatItem(cheat *parser.Cheat) cheatItem {
	folder := filepath.Base(filepath.Dir(cheat.File))
	file := strings.TrimSuffix(filepath.Base(cheat.File), filepath.Ext(cheat.File))

	return cheatItem{
		cheat:  cheat,
		folder: folder,
		file:   file,
	}
}

// matchesQuery checks if the cheat item matches all search words
// Uses case-insensitive substring matching on original strings
func (item *cheatItem) matchesQuery(words []string) bool {
	for _, word := range words {
		if !item.containsWord(word) {
			return false
		}
	}
	return true
}

// containsWord checks if any field contains the word (case-insensitive)
func (item *cheatItem) containsWord(word string) bool {
	// Check smaller fields first for fast rejection
	if containsIgnoreCase(item.folder, word) {
		return true
	}
	if containsIgnoreCase(item.file, word) {
		return true
	}
	if containsIgnoreCase(item.cheat.Header, word) {
		return true
	}
	// Check larger fields only if needed
	if containsIgnoreCase(item.cheat.Description, word) {
		return true
	}
	if containsIgnoreCase(item.cheat.Command, word) {
		return true
	}
	return false
}

// containsIgnoreCase is a fast case-insensitive substring check
func containsIgnoreCase(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	// Use strings.Contains with pre-lowercased substr (caller should cache this)
	// For ASCII, we can do a fast manual check
	return strings.Contains(strings.ToLower(s), substr)
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
// Debounce
// ============================================================================

// filterMsg triggers filtering after debounce
type filterMsg struct{}

// debounceFilter returns a command that triggers filtering after a delay
func debounceFilter() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return filterMsg{}
	})
}

// ============================================================================
// Main Model - Unified TUI (Cheat Selection + Variable Resolution)
// ============================================================================

// uiPhase represents which phase the TUI is in
type uiPhase int

const (
	phaseCheatSelect uiPhase = iota // Selecting a cheat
	phaseVarResolve                 // Resolving variables
)

// mainModel is the Bubble Tea model for cheat selection AND variable resolution
// This unified model prevents flickering by staying in a single alt-screen session
type mainModel struct {
	// Common state
	width     int
	height    int
	textInput textinput.Model
	quitting  bool

	// Phase management
	phase uiPhase

	// Cheat selection state
	cheats    []cheatItem
	filtered  []cheatItem
	cursor    int
	offset    int // viewport scroll offset
	selected  *parser.Cheat
	columns   columnConfig
	lastQuery string

	// Variable resolution state (only used in phaseVarResolve)
	varState *varResolveState

	// Dependencies for variable resolution
	cheatIndex *parser.CheatIndex
	executor   *executor.Executor
}

// varResolveState holds state for resolving variables within the unified TUI
type varResolveState struct {
	cheat        *parser.Cheat
	vars         []varState
	currentIdx   int
	options      []string // options for current variable (from shell command)
	filtered     []filteredVarOption
	selectOpts   SelectOptions
	customHeader string
	shellErr     error // error from running shell command (if any)
	isPromptOnly bool  // true if no options, just text input
}

// filteredVarOption pairs display text with original value for variable selection
type filteredVarOption struct {
	display    string
	original   string
	searchText string
}

// newMainModel creates a new mainModel with the given cheats
func newMainModel(cheats []*parser.Cheat, index *parser.CheatIndex, exec *executor.Executor) mainModel {
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
		cheats:     items,
		filtered:   items,
		textInput:  ti,
		columns:    loadColumnConfig(),
		phase:      phaseCheatSelect,
		cheatIndex: index,
		executor:   exec,
	}
}

// Init implements tea.Model
func (m mainModel) Init() tea.Cmd {
	// If we're already in variable resolution phase (from --match), prepare the first variable
	if m.phase == phaseVarResolve && m.varState != nil {
		return tea.Batch(textinput.Blink, m.prepareCurrentVar())
	}
	return textinput.Blink
}

// Update implements tea.Model
func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window size for both phases
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsMsg.Width
		m.height = wsMsg.Height
		m.textInput.Width = wsMsg.Width - 4
	}

	// Dispatch based on phase
	switch m.phase {
	case phaseVarResolve:
		return m.updateVarResolve(msg)
	default:
		return m.updateCheatSelect(msg)
	}
}

// updateCheatSelect handles updates during cheat selection phase
func (m mainModel) updateCheatSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	// Only trigger debounced filter if query changed
	if m.textInput.Value() != prevQuery {
		cmds = append(cmds, debounceFilter())
	}

	return m, tea.Batch(cmds...)
}

// handleCheatSelectKey processes keyboard input during cheat selection
func (m *mainModel) handleCheatSelectKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return tea.Quit
	case "esc":
		m.quitting = true
		return tea.Quit
	case "enter":
		if m.cursor < len(m.filtered) {
			m.selected = m.filtered[m.cursor].cheat
			// Transition to variable resolution phase
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
	case "ctrl+o":
		if m.cursor < len(m.filtered) {
			openFileInViewer(m.filtered[m.cursor].cheat.File)
		}
	}
	return nil
}

// moveCursor moves the cursor by delta, clamping to valid range
func (m *mainModel) moveCursor(delta int) {
	m.cursor += delta
	m.cursor = clamp(m.cursor, 0, max(0, len(m.filtered)-1))
	m.adjustOffset()
}

// adjustOffset ensures cursor is visible within viewport
func (m *mainModel) adjustOffset() {
	// Estimate visible height (will be adjusted in render, but this keeps offset roughly correct)
	viewHeight := maxInt(m.height-10, 3) // approximate list height
	if viewHeight <= 0 {
		return
	}
	// Scroll up: cursor went above viewport
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	// Scroll down: cursor went below viewport
	if m.cursor >= m.offset+viewHeight {
		m.offset = m.cursor - viewHeight + 1
	}
	// Clamp offset
	maxOffset := max(0, len(m.filtered)-viewHeight)
	m.offset = clamp(m.offset, 0, maxOffset)
}

// filterCheats filters the cheat list based on the search query
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
				// Limit results to prevent UI lag
				if len(m.filtered) >= 1000 {
					break
				}
			}
		}
	}

	m.cursor = clamp(m.cursor, 0, max(0, len(m.filtered)-1))
	m.adjustOffset()
}

// ============================================================================
// Variable Resolution Phase (unified TUI - no flicker)
// ============================================================================

// shellResultMsg is sent when a shell command completes
type shellResultMsg struct {
	options []string
	err     error
}

// startVarResolution initiates variable resolution and returns a command
func (m *mainModel) startVarResolution() tea.Cmd {
	m.startVarResolutionInternal()
	if m.phase != phaseVarResolve {
		// No variables to resolve - finish immediately
		return tea.Quit
	}
	return m.prepareCurrentVar()
}

// startVarResolutionInternal sets up variable resolution state
func (m *mainModel) startVarResolutionInternal() {
	cheat := m.selected
	if cheat == nil {
		return
	}

	// Initialize scope if nil
	if cheat.Scope == nil {
		cheat.Scope = make(map[string]string)
	}

	vars := collectVariables(cheat, m.cheatIndex)
	if len(vars) == 0 {
		// No variables - stay in cheat select phase (will quit immediately)
		return
	}

	// Pre-fill from cheat.Scope (populated by --match) or environment
	for i := range vars {
		varName := vars[i].def.Name
		if scopeVal, ok := cheat.Scope[varName]; ok && scopeVal != "" {
			vars[i].prefill = scopeVal
			vars[i].skipAutoCont = true
		} else if envVal := os.Getenv(varName); envVal != "" {
			vars[i].prefill = envVal
		}
	}

	m.varState = &varResolveState{
		cheat:      cheat,
		vars:       vars,
		currentIdx: 0,
	}
	m.phase = phaseVarResolve

	// Reset text input for variable resolution
	m.textInput.SetValue("")
	m.textInput.Placeholder = "Type to filter or enter value..."
	m.cursor = 0
	m.offset = 0
}

// prepareCurrentVar prepares the current variable for display
// May run a shell command to get options
func (m *mainModel) prepareCurrentVar() tea.Cmd {
	if m.varState == nil || m.varState.currentIdx >= len(m.varState.vars) {
		// All variables resolved - copy to scope and quit
		if m.varState != nil {
			for _, vs := range m.varState.vars {
				if vs.resolved {
					m.selected.Scope[vs.def.Name] = vs.value
				}
			}
		}
		return tea.Quit
	}

	vs := &m.varState.vars[m.varState.currentIdx]

	// Build current scope from already-resolved variables
	scope := make(map[string]string)
	for _, v := range m.varState.vars {
		if v.resolved {
			scope[v.def.Name] = v.value
		}
	}

	// Select the matching variant based on conditions
	selectedDef := selectVariant(vs.variants, scope)
	if selectedDef == nil {
		// Check if all variants are conditional
		allConditional := true
		for _, v := range vs.variants {
			if v.Condition == "" {
				allConditional = false
				break
			}
		}
		if allConditional && len(vs.variants) > 0 {
			// All variants conditional and none matched - skip
			vs.resolved = true
			vs.value = ""
			m.varState.currentIdx++
			return m.prepareCurrentVar()
		}
		selectedDef = &vs.def
	}
	vs.def = *selectedDef

	// Check auto-continue
	autoContinue := config.GetAutoContinue()
	if autoContinue && vs.prefill != "" && !vs.skipAutoCont {
		vs.value = vs.prefill
		vs.resolved = true
		m.varState.currentIdx++
		return m.prepareCurrentVar()
	}

	// Extract custom header from args
	m.varState.customHeader = extractCustomHeader(vs.def.Args)
	m.varState.selectOpts = parseSelectorOpts(vs.def.Args)

	// Handle literal values (no shell execution)
	if vs.def.Literal != "" {
		result := vs.def.Literal
		for name, value := range scope {
			result = strings.ReplaceAll(result, "$"+name, value)
		}
		vs.value = result
		vs.resolved = true
		m.varState.currentIdx++
		return m.prepareCurrentVar()
	}

	// Check if shell command is empty (prompt only)
	if strings.TrimSpace(vs.def.Shell) == "" {
		m.varState.isPromptOnly = true
		m.varState.options = nil
		m.varState.filtered = nil
		if vs.prefill != "" {
			m.textInput.SetValue(vs.prefill)
			m.textInput.CursorEnd()
		}
		return nil
	}

	// Run shell command asynchronously to get options
	shellCmd := vs.def.Shell
	for name, value := range scope {
		shellCmd = strings.ReplaceAll(shellCmd, "$"+name, value)
	}

	return func() tea.Msg {
		output, err := m.executor.RunShell(shellCmd)
		if err != nil {
			return shellResultMsg{nil, err}
		}
		lines := splitLines(output)
		return shellResultMsg{lines, nil}
	}
}

// parseSelectorOpts parses selector options from args
func parseSelectorOpts(selectorArgs string) SelectOptions {
	opts := SelectOptions{}
	if selectorArgs == "" {
		return opts
	}

	args := parseShellArgs(selectorArgs)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--delimiter":
			if i+1 < len(args) {
				opts.Delimiter = args[i+1]
				i++
			}
		case "--column":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.Column)
				i++
			}
		case "--map":
			if i+1 < len(args) {
				opts.MapCmd = args[i+1]
				i++
			}
		}
	}
	return opts
}

// updateVarResolve handles updates during variable resolution phase
func (m mainModel) updateVarResolve(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if cmd := m.handleVarResolveKey(msg); cmd != nil {
			return m, cmd
		}
	case shellResultMsg:
		return m.handleShellResult(msg)
	}

	// Update text input
	prevQuery := m.textInput.Value()
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)

	// Filter options if query changed
	if m.textInput.Value() != prevQuery && !m.varState.isPromptOnly {
		m.filterVarOptions()
	}

	return m, tiCmd
}

// handleShellResult processes the result of a shell command
func (m mainModel) handleShellResult(msg shellResultMsg) (tea.Model, tea.Cmd) {
	if m.varState == nil {
		return m, nil
	}

	vs := &m.varState.vars[m.varState.currentIdx]

	if msg.err != nil {
		// Shell command failed - fall back to prompt only
		m.varState.shellErr = msg.err
		m.varState.isPromptOnly = true
		m.varState.options = nil
		m.varState.filtered = nil
		m.textInput.SetValue(vs.prefill)
		return m, nil
	}

	m.varState.options = msg.options
	m.varState.shellErr = nil

	switch len(msg.options) {
	case 0:
		// No output - prompt only
		m.varState.isPromptOnly = true
		if vs.prefill != "" {
			m.textInput.SetValue(vs.prefill)
			m.textInput.CursorEnd()
		}
	case 1:
		// Single result - prefill
		m.varState.isPromptOnly = true
		prefill := vs.prefill
		if prefill == "" {
			prefill = applyMapTransform(msg.options[0], selectorOptions{
				mapCmd: m.varState.selectOpts.MapCmd,
			})
		}
		m.textInput.SetValue(prefill)
		m.textInput.CursorEnd()
	default:
		// Multiple options - show selection
		m.varState.isPromptOnly = false
		m.buildVarFilteredList()
		if vs.prefill != "" {
			m.textInput.SetValue(vs.prefill)
			m.textInput.CursorEnd()
		}
		m.filterVarOptions()
		m.cursor = 0
		m.offset = 0
	}

	return m, nil
}

// buildVarFilteredList builds the filtered list from options
func (m *mainModel) buildVarFilteredList() {
	if m.varState == nil {
		return
	}

	opts := m.varState.selectOpts
	m.varState.filtered = make([]filteredVarOption, len(m.varState.options))

	for i, opt := range m.varState.options {
		display := getDisplayColumn(opt, opts.Delimiter, opts.Column)
		m.varState.filtered[i] = filteredVarOption{
			display:    display,
			original:   opt,
			searchText: strings.ToLower(display),
		}
	}
}

// filterVarOptions filters the variable options based on search query
func (m *mainModel) filterVarOptions() {
	if m.varState == nil || m.varState.isPromptOnly {
		return
	}

	query := strings.ToLower(strings.TrimSpace(m.textInput.Value()))
	if query == "" {
		// Rebuild full list
		m.buildVarFilteredList()
	} else {
		words := strings.Fields(query)
		opts := m.varState.selectOpts
		result := make([]filteredVarOption, 0, len(m.varState.options))

		for _, opt := range m.varState.options {
			display := getDisplayColumn(opt, opts.Delimiter, opts.Column)
			searchText := strings.ToLower(display)

			matches := true
			for _, word := range words {
				if !strings.Contains(searchText, word) {
					matches = false
					break
				}
			}
			if matches {
				result = append(result, filteredVarOption{
					display:    display,
					original:   opt,
					searchText: searchText,
				})
			}
		}
		m.varState.filtered = result
	}

	m.cursor = clamp(m.cursor, 0, max(0, len(m.varState.filtered)-1))
}

// handleVarResolveKey processes keyboard input during variable resolution
func (m *mainModel) handleVarResolveKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		m.selected = nil // Signal to not execute
		return tea.Quit
	case "esc":
		// Go back to previous var or cheat selection
		if m.varState.currentIdx > 0 {
			m.varState.currentIdx--
			vs := &m.varState.vars[m.varState.currentIdx]
			vs.resolved = false
			vs.value = ""
			vs.skipAutoCont = true
			m.textInput.SetValue("")
			m.cursor = 0
			m.offset = 0
			return m.prepareCurrentVar()
		}
		// Go back to cheat selection
		m.phase = phaseCheatSelect
		m.varState = nil
		m.selected = nil
		m.textInput.SetValue("")
		m.textInput.Placeholder = "Type to search..."
		m.cursor = 0
		m.offset = 0
		return nil
	case "enter":
		return m.acceptVarValue()
	case "up", "ctrl+p":
		if !m.varState.isPromptOnly {
			m.moveVarCursor(-1)
		}
	case "down", "ctrl+n":
		if !m.varState.isPromptOnly {
			m.moveVarCursor(1)
		}
	case "pgup":
		if !m.varState.isPromptOnly {
			m.moveVarCursor(-10)
		}
	case "pgdown":
		if !m.varState.isPromptOnly {
			m.moveVarCursor(10)
		}
	case "tab":
		if !m.varState.isPromptOnly && m.cursor < len(m.varState.filtered) {
			m.textInput.SetValue(m.varState.filtered[m.cursor].display)
		}
	case "ctrl+o":
		if m.varState != nil && m.varState.cheat != nil {
			openFileInViewer(m.varState.cheat.File)
		}
	}
	return nil
}

// moveVarCursor moves the cursor during variable selection
func (m *mainModel) moveVarCursor(delta int) {
	if m.varState == nil {
		return
	}
	m.cursor += delta
	m.cursor = clamp(m.cursor, 0, max(0, len(m.varState.filtered)-1))

	// Adjust offset for viewport
	viewHeight := maxInt(m.height-10, 3)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+viewHeight {
		m.offset = m.cursor - viewHeight + 1
	}
	maxOffset := max(0, len(m.varState.filtered)-viewHeight)
	m.offset = clamp(m.offset, 0, maxOffset)
}

// acceptVarValue accepts the current value and moves to next variable
func (m *mainModel) acceptVarValue() tea.Cmd {
	if m.varState == nil {
		return tea.Quit
	}

	vs := &m.varState.vars[m.varState.currentIdx]
	var value string

	if m.varState.isPromptOnly {
		value = m.textInput.Value()
	} else if m.cursor < len(m.varState.filtered) {
		// Selected from list - get original value
		selected := m.varState.filtered[m.cursor].original

		// Apply select-column if specified
		opts := m.varState.selectOpts
		if opts.Column > 0 && opts.Delimiter != "" {
			parts := strings.Split(selected, opts.Delimiter)
			if opts.Column <= len(parts) {
				selected = strings.TrimSpace(parts[opts.Column-1])
			}
		}

		// Apply map transform if specified
		if opts.MapCmd != "" {
			selected = applyMapTransformCmd(selected, opts.MapCmd)
		}

		value = selected
	} else {
		// Use typed input
		value = m.textInput.Value()
	}

	vs.value = value
	vs.resolved = true
	m.varState.currentIdx++

	// Reset for next variable
	m.textInput.SetValue("")
	m.cursor = 0
	m.offset = 0

	return m.prepareCurrentVar()
}

// renderVarResolve renders the variable resolution view
func (m mainModel) renderVarResolve() string {
	if m.varState == nil {
		return ""
	}

	width := maxInt(m.width, 80)
	height := maxInt(m.height, 24)

	b := getBuilder()
	defer putBuilder(b)

	// Header showing progress (at top)
	header := m.renderVarHeader(width)
	headerLines := countLines(header)

	// Bottom section: list + input
	bottom := m.renderVarBottom(width)
	bottomLines := countLines(bottom)

	// Layout: header at top, padding in middle, bottom at bottom
	padding := maxInt(height-headerLines-bottomLines, 0)

	b.WriteString(header)
	b.WriteString(strings.Repeat("\n", padding))
	b.WriteString(bottom)

	return b.String()
}

// renderVarBottom renders the options list and input at the bottom
func (m mainModel) renderVarBottom(width int) string {
	b := getBuilder()
	defer putBuilder(b)

	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Options list (if not prompt-only)
	if !m.varState.isPromptOnly && len(m.varState.filtered) > 0 {
		listHeight := minInt(10, len(m.varState.filtered))
		start, end := scrollWindow(m.cursor, len(m.varState.filtered), listHeight, &m.offset)

		for i := start; i < end; i++ {
			opt := m.varState.filtered[i]
			if i == m.cursor {
				b.WriteString(styles.Cursor.Render("▶ "))
				b.WriteString(styles.Selected.Render(styles.Command.Render(opt.display)))
			} else {
				b.WriteString("  ")
				b.WriteString(styles.Command.Render(opt.display))
			}
			b.WriteString("\n")
		}
	}

	// Footer with input
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	if !m.varState.isPromptOnly && len(m.varState.filtered) > 0 {
		b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d options", len(m.varState.filtered))))
		b.WriteString(" • ")
	}
	b.WriteString(styles.Dim.Render("ESC back"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("Enter accept"))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())

	return b.String()
}

// renderVarHeader renders the progress header for variable resolution
func (m mainModel) renderVarHeader(width int) string {
	if m.varState == nil {
		return ""
	}

	b := getBuilder()
	defer putBuilder(b)

	// Command with progress highlighting
	progressCmd := m.varState.cheat.Command
	for i, vs := range m.varState.vars {
		if vs.resolved {
			progressCmd = replaceVar(progressCmd, vs.def.Name, styles.Header.Render(vs.value))
		} else if i == m.varState.currentIdx {
			progressCmd = replaceVar(progressCmd, vs.def.Name, styles.Cursor.Render("$"+vs.def.Name))
		}
	}
	b.WriteString(progressCmd)
	b.WriteString("\n")

	// Variable list
	for i, vs := range m.varState.vars {
		if vs.resolved {
			b.WriteString(styles.Command.Render("✓"))
			b.WriteString(" ")
			b.WriteString(styles.Dim.Render("$" + vs.def.Name))
			b.WriteString(" = ")
			b.WriteString(styles.Header.Render(vs.value))
		} else if i == m.varState.currentIdx {
			b.WriteString(styles.Cursor.Render("▶ $" + vs.def.Name))
		} else {
			b.WriteString(styles.Dim.Render("○ $" + vs.def.Name))
		}
		b.WriteString("\n")
	}

	// Custom header if present
	if m.varState.customHeader != "" {
		b.WriteString("\n")
		b.WriteString(styles.Header.Render(m.varState.customHeader))
		b.WriteString("\n")
	}

	// Divider
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	return b.String()
}

// View implements tea.Model
func (m mainModel) View() string {
	if m.quitting && m.selected == nil {
		return ""
	}

	// Dispatch based on phase
	switch m.phase {
	case phaseVarResolve:
		return m.renderVarResolve()
	default:
		return m.renderCheatSelect()
	}
}

// renderCheatSelect builds the cheat selection view
func (m mainModel) renderCheatSelect() string {
	width := maxInt(m.width, 80)
	height := maxInt(m.height, 24)

	preview := m.renderPreview(width)
	previewLines := countLines(preview)

	inputLines := 3 // divider + info + input
	listHeight := maxInt(height-previewLines-inputLines, 3)
	list := m.renderList(listHeight)
	listLines := countLines(list)

	padding := maxInt(height-previewLines-listLines-inputLines, 0)

	b := getBuilder()
	defer putBuilder(b)
	b.WriteString(preview)
	b.WriteString(list)
	b.WriteString(strings.Repeat("\n", padding))
	b.WriteString(m.renderInput(width))

	return b.String()
}

// renderPreview renders the preview section for the selected cheat
func (m mainModel) renderPreview(width int) string {
	b := getBuilder()
	defer putBuilder(b)
	lines := 0
	const maxLines = 6

	if m.cursor < len(m.filtered) {
		item := m.filtered[m.cursor]
		b.WriteString(styles.PreviewPath.Render(item.folder + "/" + item.file))
		b.WriteString("\n")
		lines++

		b.WriteString(styles.PreviewHeader.Render(item.cheat.Header))
		b.WriteString("\n")
		lines++

		if item.cheat.Description != "" {
			desc := truncateLines(item.cheat.Description, 1, 200)
			b.WriteString(styles.PreviewDesc.Render(desc))
			b.WriteString("\n")
			lines++
		}

		b.WriteString("\n")
		lines++

		cmd := truncateLines(item.cheat.Command, maxLines-lines, 0)
		cmdLines := strings.Count(cmd, "\n") + 1
		b.WriteString(styles.PreviewCmd.Render(cmd))
		b.WriteString("\n")
		lines += cmdLines
	}

	// Pad to fixed height
	for lines < maxLines {
		b.WriteString("\n")
		lines++
	}

	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	return b.String()
}

// renderList renders the scrollable list of cheats
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

// renderListItem renders a single list item
func (m mainModel) renderListItem(item cheatItem, selected bool, gap string) string {
	pStyle, hStyle, dStyle, cStyle := m.getItemStyles(selected)

	// Build header column
	pathPart := item.folder + "/" + item.file
	headerPart := item.cheat.Header
	headerRendered := m.renderHeaderColumn(pathPart, headerPart, pStyle, hStyle, selected)

	// Description and command columns
	desc := truncateString(firstLine(item.cheat.Description), m.columns.descWidth)
	descPadded := fmt.Sprintf("%-*s", m.columns.descWidth, desc)

	maxCmd := m.calculateCommandWidth()
	cmd := truncateString(firstLine(item.cheat.Command), maxCmd)

	// Build line
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

// getItemStyles returns the appropriate styles based on selection state
func (m mainModel) getItemStyles(selected bool) (path, header, desc, cmd lipgloss.Style) {
	path, header, desc, cmd = styles.Path, styles.Header, styles.Desc, styles.Command
	if selected {
		path = styles.WithSelection(path)
		header = styles.WithSelection(header)
		desc = styles.WithSelection(desc)
		cmd = styles.WithSelection(cmd)
	}
	return
}

// renderHeaderColumn renders the path+header column with proper truncation
func (m mainModel) renderHeaderColumn(pathPart, headerPart string, pStyle, hStyle lipgloss.Style, selected bool) string {
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

	var rendered string
	if headerPart != "" {
		rendered = pStyle.Render(pathPart) + " " + hStyle.Render(headerPart)
	} else {
		rendered = pStyle.Render(pathPart)
	}

	// Pad to column width
	if padding := m.columns.headerWidth - len(fullHeader); padding > 0 {
		padStr := strings.Repeat(" ", padding)
		if selected {
			padStr = styles.Selected.Render(padStr)
		}
		rendered += padStr
	}
	return rendered
}

// calculateCommandWidth returns the available width for command column
func (m mainModel) calculateCommandWidth() int {
	maxCmd := m.columns.cmdWidth
	if m.width > 0 {
		usedWidth := m.columns.headerWidth + m.columns.gap*2 + m.columns.descWidth + 4
		if available := m.width - usedWidth; available > 0 && available < maxCmd {
			maxCmd = available
		}
	}
	return maxCmd
}

// firstLine returns the first line of a string
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// renderInput renders the input section at the bottom
func (m mainModel) renderInput(width int) string {
	b := getBuilder()
	defer putBuilder(b)
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d/%d", len(m.filtered), len(m.cheats))))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("Ctrl+O open"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("ESC exit"))
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

// RunTUI launches the Bubble Tea interface (unified - no flicker)
func RunTUI(index *parser.CheatIndex, exec *executor.Executor, initialQuery, matchCmd string) error {
	requireCheatBlock := config.GetRequireCheatBlock()
	autoSelect := config.GetAutoSelect()

	cheats := filterCheatsByConfig(index.Cheats, requireCheatBlock)
	if len(cheats) == 0 {
		return fmt.Errorf("no cheats found")
	}

	m := newMainModel(cheats, index, exec)

	// If matchCmd is provided, try to find a cheat whose command matches
	if matchCmd != "" {
		if matched := findMatchingCheat(cheats, matchCmd); matched != nil {
			m.selected = matched
			// Pre-fill scope from the matched command
			prefillScopeFromMatch(matched, matchCmd)
			// Start variable resolution immediately
			m.startVarResolutionInternal()

			// If no variables to resolve, skip TUI entirely
			if m.phase != phaseVarResolve {
				finalCmd := exec.BuildFinalCommand(m.selected)
				return executeOutput(finalCmd, exec)
			}
		} else {
			// No exact match - use as initial query
			initialQuery = matchCmd
		}
	}

	if initialQuery != "" {
		m.textInput.SetValue(initialQuery)
		m.filterCheats()

		// Auto-select if exactly one match and --auto flag is set
		if autoSelect && len(m.filtered) == 1 {
			m.selected = m.filtered[0].cheat
			m.startVarResolutionInternal()

			// If no variables to resolve, skip TUI entirely
			if m.phase != phaseVarResolve {
				finalCmd := exec.BuildFinalCommand(m.selected)
				return executeOutput(finalCmd, exec)
			}
		}
	}

	// Always run the TUI (unified flow handles everything)
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

	// The unified TUI completes with the final command built
	if result.selected == nil {
		return nil
	}

	finalCmd := exec.BuildFinalCommand(result.selected)
	return executeOutput(finalCmd, exec)
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
func scrollWindow(cursor, total, height int, offset *int) (start, end int) {
	// Ensure offset keeps cursor visible (final adjustment)
	if cursor < *offset {
		*offset = cursor
	}
	if cursor >= *offset+height {
		*offset = cursor - height + 1
	}
	maxOffset := max(0, total-height)
	*offset = clamp(*offset, 0, maxOffset)

	start = *offset
	end = min(start+height, total)
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

// openFileInViewer opens the file in the configured editor or system default
func openFileInViewer(filePath string) {
	var cmd *exec.Cmd

	// Check for configured editor first
	if editor := config.GetEditor(); editor != "" {
		cmd = exec.Command(editor, filePath)
	} else {
		// Fall back to system default
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", filePath)
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", "", filePath)
		default: // linux, freebsd, etc.
			cmd = exec.Command("xdg-open", filePath)
		}
	}
	_ = cmd.Start()
}
