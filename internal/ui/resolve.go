package ui

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/executor"
	"github.com/gubarz/cheatmd/internal/parser"
)

// ============================================================================
// Entry Point
// ============================================================================

// Run launches the Bubble Tea TUI interface
func Run(index *parser.CheatIndex, exec *executor.Executor, initialQuery, matchCmd string) error {
	return RunTUI(index, exec, initialQuery, matchCmd)
}

// ============================================================================
// Variable Resolution
// ============================================================================

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
// Also detects variable dependencies (vars referenced in other vars' shell commands)
// Supports conditional var definitions (if/fi blocks) - stores all variants per name
func collectVariables(cheat *parser.Cheat, index *parser.CheatIndex) []varState {
	varDefs := collectVarDefinitions(cheat, index)
	usedVars := findAllVars(cheat.Command)
	allNeeded := findAllDependencies(usedVars, varDefs)
	orderedVars := topologicalSort(usedVars, varDefs, allNeeded)

	// Build final list - only include vars that have definitions
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

// collectVarDefinitions gathers all var definitions from imports and local cheat
func collectVarDefinitions(cheat *parser.Cheat, index *parser.CheatIndex) map[string][]parser.VarDef {
	varDefs := make(map[string][]parser.VarDef)

	// Collect from imports recursively
	var collectFromImports func(imports []string, seen map[string]bool)
	collectFromImports = func(imports []string, seen map[string]bool) {
		for _, importName := range imports {
			if seen[importName] {
				continue
			}
			seen[importName] = true
			if module, ok := index.Modules[importName]; ok {
				collectFromImports(module.Imports, seen)
				for _, v := range module.Vars {
					varDefs[v.Name] = append(varDefs[v.Name], v)
				}
			}
		}
	}
	collectFromImports(cheat.Imports, make(map[string]bool))

	// Local definitions
	for _, v := range cheat.Vars {
		varDefs[v.Name] = append(varDefs[v.Name], v)
	}
	return varDefs
}

// varDefDependencies returns all variable references in a VarDef
func varDefDependencies(def parser.VarDef) []string {
	var deps []string
	deps = append(deps, findAllVars(def.Shell)...)
	deps = append(deps, findAllVars(def.Literal)...)
	deps = append(deps, findAllVars(def.Condition)...)
	return deps
}

// findAllDependencies finds transitive closure of all needed variables
func findAllDependencies(usedVars []string, varDefs map[string][]parser.VarDef) map[string]bool {
	allNeeded := make(map[string]bool)
	queue := make([]string, len(usedVars))
	copy(queue, usedVars)

	for len(queue) > 0 {
		varName := queue[0]
		queue = queue[1:]

		if allNeeded[varName] {
			continue
		}
		allNeeded[varName] = true

		for _, def := range varDefs[varName] {
			for _, dep := range varDefDependencies(def) {
				if !allNeeded[dep] {
					queue = append(queue, dep)
				}
			}
		}
	}
	return allNeeded
}

// topologicalSort orders variables by their dependencies
func topologicalSort(usedVars []string, varDefs map[string][]parser.VarDef, allNeeded map[string]bool) []string {
	var orderedVars []string
	added := make(map[string]bool)
	visiting := make(map[string]bool)

	var addWithDeps func(varName string)
	addWithDeps = func(varName string) {
		if added[varName] || !allNeeded[varName] || visiting[varName] {
			return
		}
		visiting[varName] = true
		for _, def := range varDefs[varName] {
			for _, dep := range varDefDependencies(def) {
				addWithDeps(dep)
			}
		}
		visiting[varName] = false
		added[varName] = true
		orderedVars = append(orderedVars, varName)
	}

	for _, v := range usedVars {
		addWithDeps(v)
	}
	return orderedVars
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
		if evaluateCondition(v.Condition, scope) {
			return v
		}
	}

	return defaultDef
}

// evaluateCondition evaluates a condition expression against the scope
// Supports: $var == value, $var != value, $var (truthy check)
func evaluateCondition(condition string, scope map[string]string) bool {
	condition = strings.TrimSpace(condition)

	// Substitute variables in condition
	for name, value := range scope {
		condition = strings.ReplaceAll(condition, "$"+name, value)
	}

	// Check for comparison operators
	if strings.Contains(condition, "==") {
		parts := strings.SplitN(condition, "==", 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			return left == right
		}
	}

	if strings.Contains(condition, "!=") {
		parts := strings.SplitN(condition, "!=", 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			return left != right
		}
	}

	// Truthy check - non-empty after substitution
	return condition != ""
}

// replaceVar replaces $varname in cmd with replacement
func replaceVar(cmd, varName, replacement string) string {
	re := regexp.MustCompile(`\$` + regexp.QuoteMeta(varName) + `\b`)
	return re.ReplaceAllLiteralString(cmd, replacement)
}

// selectorOptions holds parsed selector arguments
type selectorOptions struct {
	header       string
	delimiter    string
	column       int    // 1-indexed, 0 means all columns (display column)
	selectColumn int    // 1-indexed, 0 means use column or full line (return column)
	mapCmd       string // command to transform selected value
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

// executeOutput handles the final command based on output mode
func executeOutput(command string, exec *executor.Executor) error {
	// Apply hooks
	finalCmd := command
	if preHook := config.GetPreHook(); preHook != "" {
		finalCmd = preHook + finalCmd
	}
	if postHook := config.GetPostHook(); postHook != "" {
		finalCmd = finalCmd + postHook
	}

	switch config.GetOutput() {
	case "exec":
		fmt.Fprintf(os.Stderr, "\033[1;32m▶ Executing:\033[0m %s\n", finalCmd)
		return exec.Execute(finalCmd)
	case "copy":
		if err := copyToClipboard(finalCmd); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "\033[1;33m✓ Copied to clipboard\033[0m\n")
		return nil
	default: // print
		fmt.Print(finalCmd)
		return nil
	}
}

// copyToClipboard copies text to the system clipboard
func copyToClipboard(text string) error {
	var copyCmd *exec.Cmd

	switch {
	case commandExists("wl-copy"):
		copyCmd = exec.Command("wl-copy")
	case commandExists("xclip"):
		copyCmd = exec.Command("xclip", "-selection", "clipboard")
	case commandExists("xsel"):
		copyCmd = exec.Command("xsel", "--clipboard", "--input")
	case commandExists("pbcopy"):
		copyCmd = exec.Command("pbcopy")
	default:
		fmt.Print(text)
		return nil
	}

	copyCmd.Stdin = strings.NewReader(text)
	return copyCmd.Run()
}

// commandExists checks if a command is available in PATH
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ============================================================================
// String Utilities
// ============================================================================

// findAllVars finds ALL $varname patterns in a command, ignoring quoting.
// Used for collecting all variables that might need resolution.
func findAllVars(cmd string) []string {
	var vars []string
	seen := make(map[string]bool)

	for i := 0; i < len(cmd); i++ {
		if cmd[i] != '$' || i+1 >= len(cmd) {
			continue
		}
		// Skip escaped $
		if i > 0 && cmd[i-1] == '\\' {
			continue
		}

		j := i + 1
		for j < len(cmd) && isVarChar(cmd[j], j == i+1) {
			j++
		}

		if j > i+1 {
			varName := cmd[i+1 : j]
			if !seen[varName] {
				vars = append(vars, varName)
				seen[varName] = true
			}
		}
		i = j - 1
	}

	return vars
}

// splitLines splits text into non-empty trimmed lines
// Optimized for large inputs - uses strings.Index instead of Split
func splitLines(s string) []string {
	if s == "" {
		return nil
	}

	// Count lines first to pre-allocate (rough estimate)
	lineCount := strings.Count(s, "\n") + 1
	lines := make([]string, 0, lineCount)

	for len(s) > 0 {
		idx := strings.IndexByte(s, '\n')
		var line string
		if idx == -1 {
			line = s
			s = ""
		} else {
			line = s[:idx]
			s = s[idx+1:]
		}

		// Trim inline (avoid TrimSpace allocation if not needed)
		start, end := 0, len(line)
		for start < end && (line[start] == ' ' || line[start] == '\t' || line[start] == '\r') {
			start++
		}
		for end > start && (line[end-1] == ' ' || line[end-1] == '\t' || line[end-1] == '\r') {
			end--
		}
		if start < end {
			lines = append(lines, line[start:end])
		}
	}

	return lines
}

// isVarChar returns true if c is valid in a variable name
func isVarChar(c byte, first bool) bool {
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' {
		return true
	}
	return !first && c >= '0' && c <= '9'
}

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
func applyMapTransform(value string, opts selectorOptions) string {
	// Apply select-column extraction first
	if opts.selectColumn > 0 && opts.delimiter != "" {
		parts := strings.Split(value, opts.delimiter)
		if opts.selectColumn <= len(parts) {
			value = strings.TrimSpace(parts[opts.selectColumn-1])
		}
	}

	// Then apply map command if present
	if opts.mapCmd == "" {
		return value
	}

	// Run the map command with the value as stdin
	cmd := exec.Command(config.GetShell(), "-c", opts.mapCmd)
	cmd.Stdin = strings.NewReader(value)
	out, err := cmd.Output()
	if err != nil {
		return value // fallback to original on error
	}
	return strings.TrimSpace(string(out))
}
