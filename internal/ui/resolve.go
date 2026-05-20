package ui

import (
	"os/exec"
	"strings"

	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/executor"
	"github.com/gubarz/cheatmd/pkg/parser"
)

// ============================================================================
// Entry Point
// ============================================================================

// Run launches the Bubble Tea TUI interface
func Run(index *parser.CheatIndex, exec Executor, initialQuery, matchCmd string) (string, error) {
	return RunTUI(index, exec, initialQuery, matchCmd)
}

// RunHistory launches the TUI with the history overlay open. If history is
// empty or unreadable, an error is returned without entering the TUI.
func RunHistory(index *parser.CheatIndex, exec Executor) (string, error) {
	return RunTUIWithStart(index, exec, "", "", phaseHistory)
}

// ============================================================================
// Variable Resolution
// ============================================================================

// SelectOptions holds display options for selection
type SelectOptions struct {
	Delimiter    string
	Column       int    // 1-indexed, 0 = all (display column)
	SelectColumn int    // 1-indexed, 0 = no extraction (return full/original line)
	MapCmd       string // command to transform selected value
}

// getDisplayColumn extracts the display column from a line
func getDisplayColumn(line, delimiter string, column int) string {
	if delimiter == "" || column == 0 {
		return line
	}
	parts := strings.Split(line, delimiter)
	if column > 0 && column <= len(parts) {
		return strings.TrimSpace(parts[column-1])
	}
	return line
}

// varState tracks a variable and its resolved value
type varState struct {
	def          parser.VarDef   // The selected/active definition
	variants     []parser.VarDef // All conditional variants (for if/fi blocks)
	value        string
	resolved     bool
	prefill      string
	skipAutoCont bool // True if user went back to this var - don't auto-continue
}

// collectVariables gathers all variable definitions from imports and local
// and wraps them in UI-specific varState objects.
func collectVariables(cheat *parser.Cheat, index *parser.CheatIndex) []varState {
	orderedVars, varDefs := executor.CollectDependencies(cheat, index)

	var vars []varState
	for _, varName := range orderedVars {
		if defs, ok := varDefs[varName]; ok && len(defs) > 0 {
			vars = append(vars, varState{
				def:      defs[0],
				variants: defs,
			})
		}
	}
	return vars
}

// selectVariant picks the first variant whose condition matches, or nil if none match
// Returns the first unconditional variant as fallback (default case)
func selectVariant(variants []parser.VarDef, scope map[string]string) *parser.VarDef {
	var defaultDef *parser.VarDef

	for i := range variants {
		v := &variants[i]
		if v.Condition == "" {
			// Unconditional - this is the default/fallback
			if defaultDef == nil {
				defaultDef = v
			}
			continue
		}
		if executor.EvaluateCondition(v.Condition, scope) {
			return v
		}
	}

	return defaultDef
}

// extractCustomHeader parses --header from selector args
func extractCustomHeader(selectorArgs string) string {
	if selectorArgs == "" {
		return ""
	}
	args := parseShellArgs(selectorArgs)
	for i := 0; i < len(args); i++ {
		if args[i] == "--header" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// ============================================================================
// Output Handling
// ============================================================================

// ============================================================================
// String Utilities
// ============================================================================

// parseShellArgs parses a string into arguments, respecting quotes
func parseShellArgs(s string) []string {
	var args []string
	var current strings.Builder
	var inQuote bool
	var quoteChar byte

	for i := 0; i < len(s); i++ {
		c := s[i]

		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(c)
			}
		} else {
			switch c {
			case '"', '\'':
				inQuote = true
				quoteChar = c
			case ' ', '\t':
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			default:
				current.WriteByte(c)
			}
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// applyMapTransform transforms the selected value based on options
func applyMapTransform(value string, opts SelectOptions) string {
	// Apply select-column extraction first
	if opts.SelectColumn > 0 && opts.Delimiter != "" {
		parts := strings.Split(value, opts.Delimiter)
		if opts.SelectColumn <= len(parts) {
			value = strings.TrimSpace(parts[opts.SelectColumn-1])
		}
	}

	// Then apply map command if present
	if opts.MapCmd == "" {
		return value
	}

	// Run the map command with the value as stdin
	cmd := exec.Command(config.GetShell(), "-c", opts.MapCmd)
	cmd.Stdin = strings.NewReader(value)
	out, err := cmd.Output()
	if err != nil {
		return value // fallback to original on error
	}
	return strings.TrimSpace(string(out))
}
