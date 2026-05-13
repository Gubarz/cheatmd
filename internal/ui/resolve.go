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
func Run(index *parser.CheatIndex, exec Executor, initialQuery, matchCmd string) error {
	return RunTUI(index, exec, initialQuery, matchCmd)
}

// RunHistory launches the TUI with the history overlay open. If history is
// empty or unreadable, an error is returned without entering the TUI.
func RunHistory(index *parser.CheatIndex, exec Executor) error {
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
// Also detects variable dependencies (vars referenced in other vars' shell commands)
// Supports conditional var definitions (if/fi blocks) - stores all variants per name
//
// When config.GetAllowUndeclaredVars() is true, variables referenced in the
// command but not declared anywhere get a prompt-only definition synthesized
// so the user is asked for a value at runtime. When false (default), such
// variables are silently skipped.
func collectVariables(cheat *parser.Cheat, index *parser.CheatIndex) []varState {
	varDefs := collectVarDefinitions(cheat, index)
	usedVars := findAllVars(cheat.Command, config.GetVarSyntax())

	if config.GetAllowUndeclaredVars() {
		for _, name := range usedVars {
			if _, ok := varDefs[name]; !ok {
				varDefs[name] = []parser.VarDef{{Name: name}}
			}
		}
	}

	allNeeded := findAllDependencies(usedVars, varDefs)
	orderedVars := topologicalSort(usedVars, varDefs, allNeeded)

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
	deps = append(deps, findAllVars(def.Shell, "dollar")...)
	deps = append(deps, findAllVars(def.Literal, "dollar")...)
	deps = append(deps, findAllVars(def.Condition, "dollar")...)
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

	// Substitute variables in condition (longest names first to prevent
	// prefix collisions, e.g. $s matching inside $scheme).
	condition = executor.SubstituteVars(condition, scope, "dollar")

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

// replaceVar replaces variable references in cmd with replacement.
// Respects var_syntax config: replaces `$varname` when dollar syntax is
// enabled, and `<varname>` when angle syntax is enabled.
func replaceVar(cmd, varName, replacement string, syntax string) string {
	q := regexp.QuoteMeta(varName)
	var parts []string
	if syntax == "dollar" || syntax == "both" {
		parts = append(parts, `\$`+q+`\b`)
	}
	if syntax == "angle" || syntax == "both" {
		parts = append(parts, `<`+q+`>`)
	}
	if len(parts) == 0 {
		return cmd
	}
	pattern := strings.Join(parts, "|")
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllLiteralString(cmd, replacement)
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
func executeOutput(command string, exec Executor) error {
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
		return exec.OutputWithMode(finalCmd, executor.OutputExec)
	case "copy":
		if err := exec.OutputWithMode(finalCmd, executor.OutputCopy); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "\033[1;33m✓ Copied to clipboard\033[0m\n")
		return nil
	default: // print
		fmt.Print(finalCmd)
		return nil
	}
}

// ============================================================================
// String Utilities
// ============================================================================

// findAllVars finds ALL variable references in a command, ignoring quoting.
// The syntax parameter controls which variable forms are replaced:
// "dollar", "angle", or "both".
// `<name|default>` is never auto-resolved (use a `<!-- cheat -->` block to
// declare defaults).
func findAllVars(cmd string, syntax string) []string {
	allowDollar := syntax == "dollar" || syntax == "both"
	allowAngle := syntax == "angle" || syntax == "both"

	var vars []string
	seen := make(map[string]bool)
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		vars = append(vars, name)
	}

	for i := 0; i < len(cmd); i++ {
		switch cmd[i] {
		case '$':
			if !allowDollar {
				continue
			}
			if i+1 >= len(cmd) {
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
				add(cmd[i+1 : j])
			}
			i = j - 1
		case '<':
			if !allowAngle {
				continue
			}
			j := i + 1
			if j >= len(cmd) {
				continue
			}
			if !isVarChar(cmd[j], true) {
				continue
			}
			j++
			for j < len(cmd) && isVarChar(cmd[j], false) {
				j++
			}
			// Must close with '>' to be a variable reference; skip
			// `<name|default>` (default-bearing form is not auto-resolved).
			if j >= len(cmd) || cmd[j] != '>' {
				continue
			}
			add(cmd[i+1 : j])
			i = j
		}
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
