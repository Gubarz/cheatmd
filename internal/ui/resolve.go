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
func Run(index *parser.CheatIndex, exec *executor.Executor, initialQuery string) error {
	return RunTUI(index, exec, initialQuery)
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

// resolveAllVariables resolves all variables with back navigation support
// Returns (goBackToMain, error)
func resolveAllVariables(cheat *parser.Cheat, index *parser.CheatIndex, exec *executor.Executor) (bool, error) {
	vars := collectVariables(cheat, index)
	if len(vars) == 0 {
		return false, nil
	}

	// Pre-fill from environment
	for i := range vars {
		if envVal := os.Getenv(vars[i].def.Name); envVal != "" {
			vars[i].prefill = envVal
		}
	}

	// Resolve with back support
	autoContinue := config.GetAutoContinue()
	currentIdx := 0
	for currentIdx < len(vars) {
		if currentIdx < 0 {
			return true, nil // Go back to main selection
		}

		vs := &vars[currentIdx]
		if vs.resolved {
			currentIdx++
			continue
		}

		scope := buildScope(vars)

		// Select the matching variant based on conditions
		selectedDef := selectVariant(vs.variants, scope)
		if selectedDef == nil {
			// No variant matched - check if ALL variants have conditions
			allConditional := true
			for _, v := range vs.variants {
				if v.Condition == "" {
					allConditional = false
					break
				}
			}
			if allConditional && len(vs.variants) > 0 {
				// All variants are conditional and none matched - skip this var
				vs.resolved = true
				vs.value = ""
				currentIdx++
				continue
			}
			// Fall back to the primary def (unconditional or empty)
			selectedDef = &vs.def
		}
		vs.def = *selectedDef

		// Auto-continue if var is prefilled from environment and auto_continue is enabled
		// But NOT if user went back to this var (skipAutoCont is set)
		if autoContinue && vs.prefill != "" && !vs.skipAutoCont {
			vs.value = vs.prefill
			vs.resolved = true
			currentIdx++
			continue
		}

		header := buildProgressHeader(cheat.Command, vars, currentIdx)

		value, goBack, err := resolveVar(vs.def, scope, exec, header, vs.prefill)
		if err != nil {
			return false, err
		}

		if value == "__EXIT__" {
			os.Exit(0)
		}

		if goBack {
			currentIdx--
			if currentIdx >= 0 {
				// Clear the previous var so it can be re-prompted
				vars[currentIdx].resolved = false
				vars[currentIdx].value = ""
				vars[currentIdx].skipAutoCont = true // User went back - don't auto-continue
			}
			continue
		}

		vs.value = value
		vs.resolved = true
		currentIdx++
	}

	// Copy resolved values to cheat scope
	for _, vs := range vars {
		if vs.resolved {
			cheat.Scope[vs.def.Name] = vs.value
		}
	}

	return false, nil
}

// collectVariables gathers all variable definitions from imports and local
// Also detects variable dependencies (vars referenced in other vars' shell commands)
// Supports conditional var definitions (if/fi blocks) - stores all variants per name
func collectVariables(cheat *parser.Cheat, index *parser.CheatIndex) []varState {
	// Store list of definitions per var name (for conditional variants)
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

	// Local definitions are added (can have multiple conditional variants)
	for _, v := range cheat.Vars {
		varDefs[v.Name] = append(varDefs[v.Name], v)
	}

	// Find vars used in the command (quote-aware - only real variable refs)
	usedVars := findCommandVars(cheat.Command, nil)

	// Find dependencies (transitive closure) - quote-aware for shell commands
	// but check ALL vars in conditions (conditions are our DSL, not shell)
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

		// Check all variants of this var for dependencies
		for _, def := range varDefs[varName] {
			// Shell commands: use quote-aware parsing (vars in single quotes are literal)
			if def.Shell != "" {
				deps := findCommandVars(def.Shell, nil)
				for _, dep := range deps {
					if !allNeeded[dep] {
						queue = append(queue, dep)
					}
				}
			}
			// Literal values: all $vars are references (our DSL, not shell)
			if def.Literal != "" {
				deps := findAllVars(def.Literal)
				for _, dep := range deps {
					if !allNeeded[dep] {
						queue = append(queue, dep)
					}
				}
			}
			// Conditions: use findAllVars (our DSL, not shell - all $vars are refs)
			if def.Condition != "" {
				deps := findAllVars(def.Condition)
				for _, dep := range deps {
					if !allNeeded[dep] {
						queue = append(queue, dep)
					}
				}
			}
		}
	}

	// Build dependency graph and topologically sort
	var orderedVars []string
	added := make(map[string]bool)
	visiting := make(map[string]bool)

	var addWithDeps func(varName string)
	addWithDeps = func(varName string) {
		if added[varName] || !allNeeded[varName] {
			return
		}
		if visiting[varName] {
			return
		}
		visiting[varName] = true
		// Check all variants for dependencies
		for _, def := range varDefs[varName] {
			if def.Shell != "" {
				for _, dep := range findCommandVars(def.Shell, nil) {
					addWithDeps(dep)
				}
			}
			if def.Literal != "" {
				for _, dep := range findAllVars(def.Literal) {
					addWithDeps(dep)
				}
			}
			if def.Condition != "" {
				for _, dep := range findAllVars(def.Condition) {
					addWithDeps(dep)
				}
			}
		}
		visiting[varName] = false
		added[varName] = true
		orderedVars = append(orderedVars, varName)
	}

	for _, v := range usedVars {
		addWithDeps(v)
	}

	// Build final list - store all variants for each var
	var vars []varState
	for _, varName := range orderedVars {
		if defs, ok := varDefs[varName]; ok && len(defs) > 0 {
			vars = append(vars, varState{
				def:      defs[0], // Primary definition
				variants: defs,    // All conditional variants
			})
		} else {
			vars = append(vars, varState{
				def: parser.VarDef{Name: varName, Shell: ""},
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

// buildScope creates a scope map from resolved variables
func buildScope(vars []varState) map[string]string {
	scope := make(map[string]string)
	for _, vs := range vars {
		if vs.resolved {
			scope[vs.def.Name] = vs.value
		}
	}
	return scope
}

// buildProgressHeader creates a header showing command progress
// Uses the global styles which are refreshed by getTTY before this is called
// Note: Does not include dividers - the TUI adds those with proper terminal width
func buildProgressHeader(cmd string, vars []varState, currentIdx int) string {
	var sb strings.Builder

	progressCmd := cmd
	for i, vs := range vars {
		if vs.resolved {
			progressCmd = replaceVar(progressCmd, vs.def.Name, styles.Header.Render(vs.value))
		} else if i == currentIdx {
			progressCmd = replaceVar(progressCmd, vs.def.Name, styles.Cursor.Render("$"+vs.def.Name))
		}
	}

	sb.WriteString(progressCmd)

	for i, vs := range vars {
		sb.WriteString("\n")
		if vs.resolved {
			sb.WriteString(styles.Command.Render("✓"))
			sb.WriteString(" ")
			sb.WriteString(styles.Dim.Render("$" + vs.def.Name))
			sb.WriteString(" = ")
			sb.WriteString(styles.Header.Render(vs.value))
		} else if i == currentIdx {
			sb.WriteString(styles.Cursor.Render("▶ $" + vs.def.Name))
		} else {
			sb.WriteString(styles.Dim.Render("○ $" + vs.def.Name))
		}
	}

	return sb.String()
}

// replaceVar replaces $varname in cmd with replacement
func replaceVar(cmd, varName, replacement string) string {
	re := regexp.MustCompile(`\$` + regexp.QuoteMeta(varName) + `\b`)
	return re.ReplaceAllLiteralString(cmd, replacement)
}

// resolveVar resolves a single variable using the TUI
func resolveVar(v parser.VarDef, scope map[string]string, exec *executor.Executor, header, prefill string) (string, bool, error) {
	customHeader := extractCustomHeader(v.Args)

	// Debug: show what we're working with
	if os.Getenv("CHEATMD_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] resolveVar: %s\n", v.Name)
		fmt.Fprintf(os.Stderr, "[DEBUG]   Shell: %q\n", v.Shell)
		fmt.Fprintf(os.Stderr, "[DEBUG]   Literal: %q\n", v.Literal)
		fmt.Fprintf(os.Stderr, "[DEBUG]   Args: %q\n", v.Args)
		fmt.Fprintf(os.Stderr, "[DEBUG]   customHeader: %q\n", customHeader)
	}

	// Handle literal values (no shell execution, just variable substitution)
	if v.Literal != "" {
		result := v.Literal
		for name, value := range scope {
			result = strings.ReplaceAll(result, "$"+name, value)
		}
		if os.Getenv("CHEATMD_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG]   Literal result: %q\n", result)
		}
		return result, false, nil
	}

	if strings.TrimSpace(v.Shell) == "" {
		// No shell command defined - just prompt
		return PromptWithTUI(v.Name, header, customHeader, prefill)
	}

	// Substitute scope into shell command
	shellCmd := v.Shell
	for name, value := range scope {
		shellCmd = strings.ReplaceAll(shellCmd, "$"+name, value)
	}

	if os.Getenv("CHEATMD_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG]   Running: %s\n", shellCmd)
	}

	output, err := exec.RunShell(shellCmd)
	if err != nil {
		if os.Getenv("CHEATMD_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG]   Error: %v\n", err)
		}
		// Command failed - show prompt with customHeader
		return PromptWithTUI(v.Name, header, customHeader, prefill)
	}

	lines := splitLines(output)
	if os.Getenv("CHEATMD_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG]   Output lines: %d\n", len(lines))
	}

	// Parse selector options
	opts := parseSelectorOptions(v.Args)

	switch len(lines) {
	case 0:
		// No output - show prompt
		return PromptWithTUI(v.Name, header, customHeader, prefill)
	case 1:
		// Single result - prefill the prompt with it so user can accept or modify
		if prefill == "" {
			prefill = applyMapTransform(lines[0], opts)
		}
		return PromptWithTUI(v.Name, header, customHeader, prefill)
	default:
		// Multiple results - show selection with options
		return SelectWithTUIOptions(v.Name, lines, header, customHeader, prefill, opts)
	}
}

// selectorOptions holds parsed selector arguments
type selectorOptions struct {
	header    string
	delimiter string
	column    int    // 1-indexed, 0 means all columns
	mapCmd    string // command to transform selected value
}

// parseSelectorOptions parses all selector arguments
func parseSelectorOptions(selectorArgs string) selectorOptions {
	opts := selectorOptions{column: 0} // default: show all
	if selectorArgs == "" {
		return opts
	}

	args := parseShellArgs(selectorArgs)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--header":
			if i+1 < len(args) {
				opts.header = args[i+1]
				i++
			}
		case "--delimiter":
			if i+1 < len(args) {
				opts.delimiter = args[i+1]
				i++
			}
		case "--column":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.column)
				i++
			}
		case "--map":
			if i+1 < len(args) {
				opts.mapCmd = args[i+1]
				i++
			}
		}
	}
	return opts
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

// findCommandVars finds $varname patterns that will be expanded by shell.
// Ignores variables inside single quotes (literal strings in shell).
// Used for determining execution order (what vars a command needs before running).
func findCommandVars(cmd string, scope map[string]string) []string {
	var vars []string
	seen := make(map[string]bool)
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]

		// Track quote state (handle escapes)
		if c == '\\' && i+1 < len(cmd) {
			i++ // Skip escaped char
			continue
		}
		if c == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if c == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}

		// Skip variables inside single quotes - they're literal
		if inSingleQuote {
			continue
		}

		if c != '$' || i+1 >= len(cmd) {
			continue
		}

		j := i + 1
		for j < len(cmd) && isVarChar(cmd[j], j == i+1) {
			j++
		}

		if j > i+1 {
			varName := cmd[i+1 : j]
			if !seen[varName] && (scope == nil || scope[varName] == "") {
				vars = append(vars, varName)
				seen[varName] = true
			}
		}
		i = j - 1
	}

	return vars
}

// splitLines splits text into non-empty trimmed lines
func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
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

// applyMapTransform runs the map command on the selected value
func applyMapTransform(value string, opts selectorOptions) string {
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

// getDisplayValue extracts the display column from a line
func getDisplayValue(line string, opts selectorOptions) string {
	if opts.delimiter == "" || opts.column == 0 {
		return line
	}
	parts := strings.Split(line, opts.delimiter)
	if opts.column > 0 && opts.column <= len(parts) {
		return strings.TrimSpace(parts[opts.column-1])
	}
	return line
}
