package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/executor"
	"github.com/gubarz/cheatmd/pkg/parser"
)

// ============================================================================
// Types
// ============================================================================

// shellResultMsg is sent when a shell command completes.
type shellResultMsg struct {
	options []string
	err     error
}

// ============================================================================
// Lifecycle
// ============================================================================

// startVarResolution initiates variable resolution and returns a command.
func (m *mainModel) startVarResolution() tea.Cmd {
	m.startVarResolutionInternal()
	if m.phase != phaseVarResolve {
		// No variables to resolve - finish immediately.
		return tea.Quit
	}
	return m.prepareCurrentVar()
}

// startVarResolutionInternal sets up variable resolution state.
func (m *mainModel) startVarResolutionInternal() {
	cheat := m.selected
	if cheat == nil {
		return
	}

	if cheat.Scope == nil {
		cheat.Scope = make(map[string]string)
	}

	vars := collectVariables(cheat, m.cheatIndex)
	if len(vars) == 0 {
		// No variables - stay in cheat select phase (will quit immediately).
		return
	}

	// Pre-fill from cheat.Scope (populated by --match) or environment.
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

	// Save query and reset text input for variable resolution.
	m.lastQuery = m.textInput.Value()
	m.textInput.SetValue("")
	m.textInput.Placeholder = "Type to filter or enter value..."
	m.picker.Cursor = 0
	m.picker.Offset = 0
}

// prepareCurrentVar prepares the current variable for display. May return a
// command to run a shell command to get options.
func (m *mainModel) prepareCurrentVar() tea.Cmd {
	if m.varState == nil || m.varState.currentIdx >= len(m.varState.vars) {
		// All variables resolved - copy to scope and quit.
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

	scope := make(map[string]string)
	for _, v := range m.varState.vars {
		if v.resolved {
			scope[v.def.Name] = v.value
		}
	}

	// Select the matching variant based on conditions.
	selectedDef := selectVariant(vs.variants, scope)
	if selectedDef == nil {
		allConditional := true
		for _, v := range vs.variants {
			if v.Condition == "" {
				allConditional = false
				break
			}
		}
		if allConditional && len(vs.variants) > 0 {
			// All variants conditional and none matched - skip.
			vs.resolved = true
			vs.value = ""
			m.varState.currentIdx++
			return m.prepareCurrentVar()
		}
		selectedDef = &vs.def
	}
	vs.def = *selectedDef

	// Auto-continue if the prefill is good enough.
	autoContinue := config.GetAutoContinue()
	if autoContinue && vs.prefill != "" && !vs.skipAutoCont {
		vs.value = vs.prefill
		vs.resolved = true
		m.varState.currentIdx++
		return m.prepareCurrentVar()
	}

	m.varState.customHeader = extractCustomHeader(vs.def.Args)
	m.varState.selectOpts = parseSelectorOpts(vs.def.Args)

	// Literal value: substitute scope vars and either show or auto-resolve.
	if vs.def.Literal != "" {
		result := executor.SubstituteVars(vs.def.Literal, scope, "dollar")
		if vs.skipAutoCont {
			m.varState.isPromptOnly = true
			m.varState.options = nil
			if m.varState.picker != nil {
				m.varState.picker.SetItems(nil)
			}
			m.textInput.SetValue(result)
			m.textInput.CursorEnd()
			return nil
		}
		vs.value = result
		vs.resolved = true
		m.varState.currentIdx++
		return m.prepareCurrentVar()
	}

	// Prompt only.
	if strings.TrimSpace(vs.def.Shell) == "" {
		m.varState.isPromptOnly = true
		m.varState.options = nil
		if m.varState.picker != nil {
			m.varState.picker.SetItems(nil)
		}
		if vs.prefill != "" {
			m.textInput.SetValue(vs.prefill)
			m.textInput.CursorEnd()
		}
		return nil
	}

	// Run shell command asynchronously to get options.
	shellCmd := executor.SubstituteVars(vs.def.Shell, scope, "dollar")
	return func() tea.Msg {
		output, err := m.executor.RunShell(shellCmd)
		if err != nil {
			return shellResultMsg{nil, err}
		}
		lines := parser.SplitLines(output)
		return shellResultMsg{lines, nil}
	}
}

// parseSelectorOpts parses selector options from args.
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
		case "--select-column":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.SelectColumn)
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

// ============================================================================
// Update
// ============================================================================

// updateVarResolve handles updates during variable resolution phase.
func (m *mainModel) updateVarResolve(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if cmd := m.handleVarResolveKey(msg); cmd != nil {
			return m, cmd
		}
	case shellResultMsg:
		return m.handleShellResult(msg)
	}

	if m.varState == nil {
		return m, nil
	}

	prevQuery := m.textInput.Value()
	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)

	if m.textInput.Value() != prevQuery {
		m.clearPathCompletions()
		if !m.varState.isPromptOnly && m.varState.picker != nil {
			m.varState.picker.Filter(m.textInput.Value())
		}
	}

	return m, tiCmd
}

// handleShellResult processes the result of a shell command.
func (m *mainModel) handleShellResult(msg shellResultMsg) (tea.Model, tea.Cmd) {
	if m.varState == nil {
		return m, nil
	}

	vs := &m.varState.vars[m.varState.currentIdx]

	if msg.err != nil {
		m.varState.shellErr = msg.err
		m.varState.isPromptOnly = true
		m.varState.options = nil
		if m.varState.picker != nil {
			m.varState.picker.SetItems(nil)
		}
		m.textInput.SetValue(vs.prefill)
		return m, nil
	}

	m.varState.options = msg.options
	m.varState.shellErr = nil

	switch len(msg.options) {
	case 0:
		m.varState.isPromptOnly = true
		if vs.prefill != "" {
			m.textInput.SetValue(vs.prefill)
			m.textInput.CursorEnd()
		}
	case 1:
		m.varState.isPromptOnly = true
		prefill := vs.prefill
		if prefill == "" {
			prefill = applyMapTransform(msg.options[0], m.varState.selectOpts)
		}
		m.textInput.SetValue(prefill)
		m.textInput.CursorEnd()
	default:
		m.varState.isPromptOnly = false

		// Build options list
		opts := m.varState.selectOpts
		items := make([]FilteredOption, len(msg.options))
		for i, opt := range msg.options {
			display := getDisplayColumn(opt, opts.Delimiter, opts.Column)
			items[i] = FilteredOption{
				Display:    display,
				Original:   opt,
				SearchText: strings.ToLower(display),
			}
		}

		if m.varState.picker == nil {
			m.varState.picker = NewPicker(items, func(opt FilteredOption, words []string) bool {
				return matchesAllWords(opt.SearchText, words)
			})
		} else {
			m.varState.picker.SetItems(items)
		}
		m.varState.picker.Filter(m.textInput.Value())

		if vs.prefill != "" {
			m.textInput.SetValue(vs.prefill)
			m.textInput.CursorEnd()
		}
	}

	return m, nil
}

// handleVarResolveKey processes keyboard input during variable resolution.
func (m *mainModel) handleVarResolveKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		m.selected = nil
		return tea.Quit
	case "esc":
		// Go back to previous var or cheat selection.
		if m.varState.currentIdx > 0 {
			m.varState.currentIdx--
			vs := &m.varState.vars[m.varState.currentIdx]
			vs.resolved = false
			vs.value = ""
			vs.skipAutoCont = true
			m.textInput.SetValue("")
			m.picker.Cursor = 0
			m.picker.Offset = 0
			return m.prepareCurrentVar()
		}
		m.phase = phaseCheatSelect
		m.varState = nil
		m.selected = nil
		m.textInput.SetValue(m.lastQuery)
		m.textInput.Placeholder = "Type to search..."
		m.filterCheats()
		m.picker.Cursor = 0
		m.picker.Offset = 0
		return nil
	case "enter":
		return m.acceptVarValue()
	case "tab":
		if m.completePathFromInput() {
			return nil
		}
		if !m.varState.isPromptOnly && m.varState.picker != nil {
			if opt, ok := m.varState.picker.Selected(); ok {
				m.textInput.SetValue(opt.Display)
				m.textInput.CursorEnd()
			}
		}
	default:
		if msg.String() == config.GetKeyOpen() {
			if m.varState != nil && m.varState.cheat != nil {
				openFileInViewer(m.varState.cheat.File)
			}
		}
		if msg.String() == config.GetKeySubstitute() {
			if m.enterSubstituteSearch() {
				return tea.Batch(tea.ClearScreen, textinput.Blink)
			}
		}
		if msg.String() == config.GetKeyPreview() {
			if m.varState != nil && m.varState.cheat != nil {
				if m.enterPreview(m.varState.cheat) {
					return tea.ClearScreen
				}
			}
		}
	}
	return nil
}

func (m *mainModel) completePathFromInput() bool {
	if m.varState == nil {
		return false
	}
	value := m.textInput.Value()
	cursor := m.textInput.Position()
	if !m.varState.isPromptOnly && !looksPathLikeInput(value, cursor) {
		return false
	}
	result, ok := completePathValue(value, cursor)
	if !ok {
		m.clearPathCompletions()
		return false
	}

	m.textInput.SetValue(result.Value)
	m.textInput.SetCursor(result.Cursor)

	show := len(result.Candidates) > 1
	m.varState.pathCompletions = result.Candidates
	m.varState.showPathCompletions = show
	if !m.varState.isPromptOnly && m.varState.picker != nil {
		m.varState.picker.Filter(m.textInput.Value())
	}
	return true
}

func looksPathLikeInput(value string, cursor int) bool {
	runes := []rune(value)
	if cursor < 0 || cursor > len(runes) {
		cursor = len(runes)
	}
	start := pathTokenStart(runes, cursor)
	token := strings.TrimLeft(string(runes[start:cursor]), `"'`)
	return strings.HasPrefix(token, "/") ||
		strings.HasPrefix(token, "./") ||
		strings.HasPrefix(token, "../") ||
		strings.HasPrefix(token, "~/") ||
		strings.HasPrefix(token, "$") ||
		strings.Contains(token, "/")
}

func (m *mainModel) clearPathCompletions() {
	if m.varState == nil {
		return
	}
	m.varState.pathCompletions = nil
	m.varState.showPathCompletions = false
}

// acceptVarValue accepts the current value and moves to next variable.
func (m *mainModel) acceptVarValue() tea.Cmd {
	if m.varState == nil {
		return tea.Quit
	}

	vs := &m.varState.vars[m.varState.currentIdx]
	var value string

	if m.varState.isPromptOnly {
		value = m.textInput.Value()
	} else if m.varState.picker != nil {
		if opt, ok := m.varState.picker.Selected(); ok {
			selected := opt.Original
			if m.varState.selectOpts.MapCmd != "" {
				selected = applyMapTransform(selected, m.varState.selectOpts)
			}
			value = selected
		} else {
			value = m.textInput.Value()
		}
	} else {
		value = m.textInput.Value()
	}

	vs.value = value
	vs.resolved = true
	m.varState.currentIdx++

	m.textInput.SetValue("")
	m.clearPathCompletions()
	m.picker.Cursor = 0
	m.picker.Offset = 0

	return m.prepareCurrentVar()
}

// ============================================================================
// Render
// ============================================================================

// renderVarResolve renders the variable resolution view.
func (m *mainModel) renderVarResolve() string {
	if m.varState == nil {
		return ""
	}

	width := max(m.width, 80)
	height := m.height
	if height < 1 {
		height = 24
	}

	b := getBuilder()
	defer putBuilder(b)

	header := m.renderVarHeader(width)
	headerLines := countLines(header)

	availableForBottom := max(height-headerLines, 5)
	bottom := m.renderVarBottomWithHeight(width, availableForBottom)
	bottomLines := countLines(bottom)

	padding := max(height-headerLines-bottomLines, 0)

	b.WriteString(header)
	b.WriteString(strings.Repeat("\n", padding))
	b.WriteString(bottom)

	return b.String()
}

// renderVarBottomWithHeight renders the options list and input with a max height.
func (m *mainModel) renderVarBottomWithHeight(width int, maxHeight int) string {
	b := getBuilder()
	defer putBuilder(b)

	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Fixed lines: top divider(1) + bottom divider(1) + info line(1) + input(1) = 4
	fixedLines := 4

	availableForList := max(maxHeight-fixedLines, 1)

	if m.varState.showPathCompletions && len(m.varState.pathCompletions) > 0 {
		listHeight := min(availableForList, min(10, len(m.varState.pathCompletions)))
		for i := 0; i < listHeight; i++ {
			candidate := m.varState.pathCompletions[i]
			b.WriteString("  ")
			b.WriteString(styles.Command.Render(candidate.Display))
			b.WriteString("\n")
		}
	} else if !m.varState.isPromptOnly && m.varState.picker != nil && len(m.varState.picker.Filtered) > 0 {
		listHeight := min(availableForList, min(10, len(m.varState.picker.Filtered)))
		start, end := scrollWindow(m.varState.picker.Cursor, len(m.varState.picker.Filtered), listHeight, &m.varState.picker.Offset)

		for i := start; i < end; i++ {
			opt := m.varState.picker.Filtered[i]
			if i == m.varState.picker.Cursor {
				b.WriteString(styles.Cursor.Render("▶ "))
				b.WriteString(styles.Selected.Render(styles.Command.Render(opt.Display)))
			} else {
				b.WriteString("  ")
				b.WriteString(styles.Command.Render(opt.Display))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	if m.varState.showPathCompletions && len(m.varState.pathCompletions) > 0 {
		b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d path matches", len(m.varState.pathCompletions))))
		b.WriteString(" • ")
	} else if !m.varState.isPromptOnly && m.varState.picker != nil && len(m.varState.picker.Filtered) > 0 {
		b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d options", len(m.varState.picker.Filtered))))
		b.WriteString(" • ")
	}
	b.WriteString(styles.Dim.Render("ESC back"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("Enter accept"))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())

	return b.String()
}

// renderVarHeader renders the progress header for variable resolution.
func (m *mainModel) renderVarHeader(width int) string {
	if m.varState == nil {
		return ""
	}

	b := getBuilder()
	defer putBuilder(b)

	progressCmd := m.varState.cheat.Command
	for i, vs := range m.varState.vars {
		if vs.resolved {
			progressCmd = executor.ReplaceVar(progressCmd, vs.def.Name, styles.Header.Render(vs.value), config.GetVarSyntax())
		} else if i == m.varState.currentIdx {
			displayStr := formatVarName(m.varState.cheat.Command, vs.def.Name)
			progressCmd = executor.ReplaceVar(progressCmd, vs.def.Name, styles.Cursor.Render(displayStr), config.GetVarSyntax())
		}
	}
	b.WriteString(progressCmd)
	b.WriteString("\n")

	for i, vs := range m.varState.vars {
		displayStr := formatVarName(m.varState.cheat.Command, vs.def.Name)
		if vs.resolved {
			b.WriteString(styles.Command.Render("✓"))
			b.WriteString(" ")
			b.WriteString(styles.Dim.Render(displayStr))
			b.WriteString(" = ")
			b.WriteString(styles.Header.Render(vs.value))
		} else if i == m.varState.currentIdx {
			b.WriteString(styles.Cursor.Render("▶ " + displayStr))
		} else {
			b.WriteString(styles.Dim.Render("○ " + displayStr))
		}
		b.WriteString("\n")
	}

	if m.varState.customHeader != "" {
		b.WriteString("\n")
		b.WriteString(styles.Header.Render(m.varState.customHeader))
		b.WriteString("\n")
	}

	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	return b.String()
}

// formatVarName returns the variable name formatted according to how it appears in the command,
// or defaults based on the syntax config.
func formatVarName(cmd string, name string) string {
	if config.GetVarSyntax() == "angle" {
		return "<" + name + ">"
	} else if config.GetVarSyntax() == "both" {
		if strings.Contains(cmd, "<"+name+">") {
			return "<" + name + ">"
		}
	}
	return "$" + name
}
