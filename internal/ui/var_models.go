package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ============================================================================
// Variable Select Model - For selecting from a list of options
// ============================================================================

// varSelectModel is for selecting from a list of options
type varSelectModel struct {
	varName      string
	header       string
	customHeader string
	options      []string
	filtered     []string
	cursor       int
	textInput    textinput.Model
	width        int
	height       int
	selected     string
	cancelled    bool
}

// newVarSelectModel creates a new variable selection model
func newVarSelectModel(varName string, options []string, header, customHeader, prefill string) varSelectModel {
	ti := textinput.New()
	ti.Placeholder = "Type to filter or enter custom value..."
	ti.Focus()
	ti.CharLimit = 512
	ti.Width = 60

	if prefill != "" {
		ti.SetValue(prefill)
	}

	return varSelectModel{
		varName:      varName,
		header:       header,
		customHeader: customHeader,
		options:      options,
		filtered:     options,
		textInput:    ti,
	}
}

// Init implements tea.Model
func (m varSelectModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model
func (m varSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	m.filterOptions()

	return m, cmd
}

// handleKeyPress processes keyboard input
func (m *varSelectModel) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		m.cancelled = true
		m.selected = "__EXIT__"
		return tea.Quit
	case "esc":
		m.cancelled = true
		return tea.Quit
	case "enter":
		if m.cursor < len(m.filtered) {
			m.selected = m.filtered[m.cursor]
		} else {
			m.selected = m.textInput.Value()
		}
		return tea.Quit
	case "up", "ctrl+p":
		m.moveCursor(-1)
	case "down", "ctrl+n":
		m.moveCursor(1)
	case "tab":
		if m.cursor < len(m.filtered) {
			m.textInput.SetValue(m.filtered[m.cursor])
		}
	}
	return nil
}

// moveCursor moves the cursor by delta
func (m *varSelectModel) moveCursor(delta int) {
	m.cursor += delta
	m.cursor = clamp(m.cursor, 0, maxInt(0, len(m.filtered)-1))
}

// filterOptions filters options based on the input query
func (m *varSelectModel) filterOptions() {
	query := strings.TrimSpace(strings.ToLower(m.textInput.Value()))

	if query == "" {
		m.filtered = m.options
	} else {
		words := strings.Fields(query)
		m.filtered = make([]string, 0, len(m.options))
		for _, opt := range m.options {
			if matchesAllWords(strings.ToLower(opt), words) {
				m.filtered = append(m.filtered, opt)
			}
		}
	}

	m.cursor = clamp(m.cursor, 0, maxInt(0, len(m.filtered)-1))
}

// View implements tea.Model
func (m varSelectModel) View() string {
	width := maxInt(m.width, 80)
	height := maxInt(m.height, 24)

	header := m.renderHeader()
	bottom := m.renderBottom(width)

	headerLines := countLines(header)
	bottomLines := countLines(bottom)
	spacing := maxInt(height-headerLines-bottomLines, 0)

	var b strings.Builder
	b.WriteString(header)
	b.WriteString(strings.Repeat("\n", spacing))
	b.WriteString(bottom)

	return b.String()
}

// renderHeader renders the header section
func (m varSelectModel) renderHeader() string {
	var b strings.Builder
	b.WriteString(m.header)
	b.WriteString("\n\n")

	if m.customHeader != "" {
		b.WriteString(styles.Cursor.Render(m.customHeader))
		b.WriteString(styles.Dim.Render(" • ESC to go back • Enter to select"))
	} else {
		b.WriteString(styles.Dim.Render("Select value for "))
		b.WriteString(styles.Cursor.Render("$" + m.varName))
		b.WriteString(styles.Dim.Render(" • ESC to go back • Enter to select"))
	}

	return b.String()
}

// renderBottom renders the options list and input
func (m varSelectModel) renderBottom(width int) string {
	var b strings.Builder
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Options list
	listHeight := minInt(10, len(m.filtered))
	start, end := scrollWindow(m.cursor, len(m.filtered), listHeight)

	for i := start; i < end; i++ {
		opt := m.filtered[i]
		if i == m.cursor {
			b.WriteString(styles.Cursor.Render("▶ "))
			b.WriteString(styles.Selected.Render(styles.Command.Render(opt)))
		} else {
			b.WriteString("  ")
			b.WriteString(styles.Command.Render(opt))
		}
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())

	return b.String()
}

// ============================================================================
// Variable Input Model - For entering a custom value
// ============================================================================

// varInputModel is for entering a custom value (no options list)
type varInputModel struct {
	varName      string
	header       string
	customHeader string
	textInput    textinput.Model
	width        int
	height       int
	value        string
	cancelled    bool
}

// newVarInputModel creates a new variable input model
func newVarInputModel(varName, header, customHeader, prefill string) varInputModel {
	ti := textinput.New()
	ti.Placeholder = "Enter value..."
	ti.Focus()
	ti.CharLimit = 512
	ti.Width = 60

	if prefill != "" {
		ti.SetValue(prefill)
	}

	return varInputModel{
		varName:      varName,
		header:       header,
		customHeader: customHeader,
		textInput:    ti,
	}
}

// Init implements tea.Model
func (m varInputModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model
func (m varInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// handleKeyPress processes keyboard input
func (m *varInputModel) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		m.cancelled = true
		m.value = "__EXIT__"
		return tea.Quit
	case "esc":
		m.cancelled = true
		return tea.Quit
	case "enter":
		m.value = m.textInput.Value()
		return tea.Quit
	}
	return nil
}

// View implements tea.Model
func (m varInputModel) View() string {
	width := maxInt(m.width, 80)
	height := maxInt(m.height, 24)

	header := m.renderHeader()
	bottom := m.renderBottom(width)

	headerLines := countLines(header)
	bottomLines := countLines(bottom)
	spacing := maxInt(height-headerLines-bottomLines, 0)

	var b strings.Builder
	b.WriteString(header)
	b.WriteString(strings.Repeat("\n", spacing))
	b.WriteString(bottom)

	return b.String()
}

// renderHeader renders the header section
func (m varInputModel) renderHeader() string {
	var b strings.Builder
	b.WriteString(m.header)
	b.WriteString("\n\n")

	if m.customHeader != "" {
		b.WriteString(styles.Cursor.Render(m.customHeader))
		b.WriteString(styles.Dim.Render(" • ESC to go back • Enter to confirm"))
	} else {
		b.WriteString(styles.Dim.Render("Enter value for "))
		b.WriteString(styles.Cursor.Render("$" + m.varName))
		b.WriteString(styles.Dim.Render(" • ESC to go back • Enter to confirm"))
	}

	return b.String()
}

// renderBottom renders the input section
func (m varInputModel) renderBottom(width int) string {
	var b strings.Builder
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())
	return b.String()
}

// ============================================================================
// Public API for Variable Resolution
// ============================================================================

// SelectWithTUI displays options for variable selection
// Returns (value, goBack, error) - if value is "__EXIT__" caller should exit completely
func SelectWithTUI(varName string, options []string, header, customHeader, prefill string) (string, bool, error) {
	ttyIn, ttyOut, cleanup := getTTY()
	RefreshStyles() // Refresh after getTTY sets up the renderer
	defer cleanup()

	m := newVarSelectModel(varName, options, header, customHeader, prefill)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(ttyOut), tea.WithInput(ttyIn))

	finalModel, err := p.Run()
	if err != nil {
		return "", false, err
	}

	result := finalModel.(varSelectModel)
	if result.selected == "__EXIT__" {
		return "__EXIT__", false, nil
	}
	if result.cancelled {
		return "", true, nil
	}
	return result.selected, false, nil
}

// PromptWithTUI displays an input prompt for variable entry
// Returns (value, goBack, error) - if value is "__EXIT__" caller should exit completely
func PromptWithTUI(varName, header, customHeader, prefill string) (string, bool, error) {
	ttyIn, ttyOut, cleanup := getTTY()
	RefreshStyles() // Refresh after getTTY sets up the renderer
	defer cleanup()

	m := newVarInputModel(varName, header, customHeader, prefill)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(ttyOut), tea.WithInput(ttyIn))

	finalModel, err := p.Run()
	if err != nil {
		return "", false, err
	}

	result := finalModel.(varInputModel)
	if result.value == "__EXIT__" {
		return "__EXIT__", false, nil
	}
	if result.cancelled {
		return "", true, nil
	}
	return result.value, false, nil
}

// ============================================================================
// Additional Helpers
// ============================================================================

// minInt returns the smaller of a and b
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
