package parser

import "strings"

// ============================================================================
// Domain Types
// ============================================================================

// Cheat represents a single executable cheat entry.
type Cheat struct {
	File          string            // Source file path
	Header        string            // Section header
	Description   string            // Description text
	Command       string            // Shell command template
	CommandLang   string            // Code fence language for Command
	CommandStart  int               // 1-indexed source line of first command line
	CommandEnd    int               // 1-indexed source line of last command line
	Tags          []string          // Tags from path/header
	Export        string            // Module name if exported
	Imports       []string          // Imported modules
	Vars          []VarDef          // Variable definitions
	Scope         map[string]string // Resolved values at runtime
	HasCheatBlock bool              // Whether this cheat has a <!-- cheat --> block
}

// NewCheat creates a new Cheat.
func NewCheat(file, header string) *Cheat {
	return &Cheat{
		File:   pathInterner.Intern(file),
		Header: header,
		// Scope allocated lazily at runtime when needed.
	}
}

// VarDef represents a variable definition.
type VarDef struct {
	Name      string // Variable name
	Shell     string // Shell command to generate values (for = syntax)
	Literal   string // Literal value with var substitution (for := syntax)
	Args      string // Selector arguments after ---
	Condition string // Conditional expression: "$var == value" or "$var != value"
}

// ParseVarDef parses a variable definition from name and value (shell command).
func ParseVarDef(name, value string) VarDef {
	v := VarDef{Name: name}
	if idx := strings.Index(value, "---"); idx != -1 {
		v.Shell = strings.TrimSpace(value[:idx])
		v.Args = strings.TrimSpace(value[idx+3:])
	} else {
		v.Shell = strings.TrimSpace(value)
	}
	return v
}

// ParseVarDefLiteral parses a literal variable definition (no shell, just
// substitution).
func ParseVarDefLiteral(name, value string) VarDef {
	v := VarDef{Name: name}
	if idx := strings.Index(value, "---"); idx != -1 {
		v.Literal = strings.TrimSpace(value[:idx])
		v.Args = strings.TrimSpace(value[idx+3:])
	} else {
		v.Literal = strings.TrimSpace(value)
	}
	return v
}

// ParseVarDefWithCondition parses a variable definition with an optional
// condition.
func ParseVarDefWithCondition(name, value, condition string, isLiteral bool) VarDef {
	var v VarDef
	if isLiteral {
		v = ParseVarDefLiteral(name, value)
	} else {
		v = ParseVarDef(name, value)
	}
	v.Condition = condition
	return v
}

// Module represents an exportable module with variables.
type Module struct {
	Name    string
	Vars    []VarDef
	Imports []string
	File    string
	Cheats  []*Cheat
}

// NewModule creates a Module from a Cheat.
func NewModule(cheat *Cheat) *Module {
	return &Module{
		Name:    cheat.Export,
		Vars:    cheat.Vars,
		Imports: cheat.Imports,
		File:    cheat.File,
		Cheats:  []*Cheat{cheat},
	}
}

// ============================================================================
// Cheat Index
// ============================================================================

// DuplicateExport records a duplicate export definition.
type DuplicateExport struct {
	Name  string
	File1 string
	File2 string
}

// CheatIndex holds all parsed cheats and modules.
type CheatIndex struct {
	Cheats     []*Cheat
	Modules    map[string]*Module
	Duplicates []DuplicateExport
}

// NewCheatIndex creates an empty cheat index.
func NewCheatIndex() *CheatIndex {
	return &CheatIndex{}
}

// AddCheat adds a cheat to the index.
func (idx *CheatIndex) AddCheat(cheat *Cheat) {
	idx.Cheats = append(idx.Cheats, cheat)
}

// RegisterModule registers a module from a cheat with an export. Duplicate
// export names are tracked for later reporting.
func (idx *CheatIndex) RegisterModule(cheat *Cheat) {
	if cheat.Export == "" {
		return
	}
	if idx.Modules == nil {
		idx.Modules = make(map[string]*Module)
	}
	if existing, ok := idx.Modules[cheat.Export]; ok {
		idx.Duplicates = append(idx.Duplicates, DuplicateExport{
			Name:  cheat.Export,
			File1: existing.File,
			File2: cheat.File,
		})
	}
	idx.Modules[cheat.Export] = NewModule(cheat)
}
