package ui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/gubarz/cheatmd/pkg/chainstate"
	"github.com/gubarz/cheatmd/pkg/executor"
	"github.com/gubarz/cheatmd/pkg/parser"
)

// Executor defines the interface required by the UI for command execution and resolution.
type Executor interface {
	RunShell(command string) (string, error)
	BuildFinalCommand(cheat *parser.Cheat) string
	OutputWithMode(command string, mode executor.OutputMode) error
}

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
// Main Model - Unified TUI (Cheat Selection + Variable Resolution)
// ============================================================================

// uiPhase represents which phase the TUI is in
type uiPhase int

const (
	phaseCheatSelect      uiPhase = iota // Selecting a cheat
	phaseVarResolve                      // Resolving variables
	phaseSubstituteSearch                // Substitute-search overlay during var resolution
	phasePreview                         // Full-screen markdown preview of cheat's source file
	phaseHistory                         // Execution-history overlay
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
	chains    []chainGroup
	cursor    int
	offset    int // viewport scroll offset
	selected  *parser.Cheat
	columns   columnConfig
	lastQuery string

	// Variable resolution state (only used in phaseVarResolve)
	varState *varResolveState

	// Substitute search state (only used in phaseSubstituteSearch)
	subState *substituteSearchState

	// Preview overlay state (only used in phasePreview)
	previewState *previewOverlayState

	// History overlay state (only used in phaseHistory)
	histState *historyState

	// Dependencies for variable resolution
	cheatIndex *parser.CheatIndex
	executor   Executor
	chainState *chainstate.State
	chainPath  string
}

// varResolveState holds state for resolving variables within the unified TUI
type varResolveState struct {
	cheat        *parser.Cheat
	vars         []varState
	currentIdx   int
	options      []string // options for current variable (from shell command)
	filtered     []FilteredOption
	selectOpts   SelectOptions
	customHeader string
	shellErr     error // error from running shell command (if any)
	isPromptOnly bool  // true if no options, just text input
}

// FilteredOption pairs display text with original value for variable selection
type FilteredOption struct {
	Display    string
	Original   string
	SearchText string
}

// newMainModel creates a new mainModel with the given cheats
func newMainModel(cheats []*parser.Cheat, index *parser.CheatIndex, exec Executor) mainModel {
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
		chains:     buildChains(cheats),
		textInput:  ti,
		columns:    loadColumnConfig(),
		phase:      phaseCheatSelect,
		cheatIndex: index,
		executor:   exec,
	}
}

// Init implements tea.Model
func (m *mainModel) Init() tea.Cmd {
	// If we're already in variable resolution phase (from --match), prepare the first variable
	if m.phase == phaseVarResolve && m.varState != nil {
		return tea.Batch(textinput.Blink, m.prepareCurrentVar())
	}
	return textinput.Blink
}

// Update implements tea.Model
func (m *mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window size for both phases
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = max(wsMsg.Width, 1)
		m.height = max(wsMsg.Height, 1)
		m.textInput.Width = safeTextInputWidth(m.width)
	}

	// Dispatch based on phase
	switch m.phase {
	case phasePreview:
		return m.updatePreview(msg)
	case phaseHistory:
		return m.updateHistory(msg)
	case phaseSubstituteSearch:
		return m.updateSubstituteSearch(msg)
	case phaseVarResolve:
		return m.updateVarResolve(msg)
	default:
		return m.updateCheatSelect(msg)
	}
}

// View implements tea.Model
func (m *mainModel) View() string {
	if m.quitting && m.selected == nil {
		return ""
	}

	// Dispatch based on phase
	switch m.phase {
	case phasePreview:
		return m.renderPreview()
	case phaseHistory:
		return m.renderHistory()
	case phaseSubstituteSearch:
		return m.renderSubstituteSearch()
	case phaseVarResolve:
		return m.renderVarResolve()
	default:
		return m.renderCheatSelect()
	}
}
