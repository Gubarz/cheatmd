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
	def      parser.VarDef
	value    string
	resolved bool
	prefill  string
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
				vars[currentIdx].resolved = false
				vars[currentIdx].value = ""
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
func collectVariables(cheat *parser.Cheat, index *parser.CheatIndex) []varState {
	usedVars := findCommandVars(cheat.Command, nil)
	usedSet := make(map[string]bool)
	for _, v := range usedVars {
		usedSet[v] = true
	}

	varDefs := make(map[string]parser.VarDef)

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
					if _, exists := varDefs[v.Name]; !exists {
						varDefs[v.Name] = v
					}
				}
			}
		}
	}
	collectFromImports(cheat.Imports, make(map[string]bool))

	// Local definitions override imports
	for _, v := range cheat.Vars {
		varDefs[v.Name] = v
	}

	// Build ordered list
	var vars []varState
	for _, varName := range usedVars {
		if def, ok := varDefs[varName]; ok {
			vars = append(vars, varState{def: def})
		} else {
			vars = append(vars, varState{
				def: parser.VarDef{Name: varName, Shell: ""},
			})
		}
	}

	return vars
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
func buildProgressHeader(cmd string, vars []varState, currentIdx int) string {
	var sb strings.Builder

	progressCmd := cmd
	for i, vs := range vars {
		if vs.resolved {
			progressCmd = replaceVar(progressCmd, vs.def.Name, fmt.Sprintf("\033[1;33m%s\033[0m", vs.value))
		} else if i == currentIdx {
			progressCmd = replaceVar(progressCmd, vs.def.Name, fmt.Sprintf("\033[1;35m$%s\033[0m", vs.def.Name))
		}
	}

	sb.WriteString(fmt.Sprintf("%s\n", progressCmd))
	sb.WriteString("\033[90m─────────────────────────────────────────\033[0m\n")

	for i, vs := range vars {
		if vs.resolved {
			sb.WriteString(fmt.Sprintf("\033[32m✓\033[0m \033[90m$%s\033[0m = \033[33m%s\033[0m\n", vs.def.Name, vs.value))
		} else if i == currentIdx {
			sb.WriteString(fmt.Sprintf("\033[1;35m▶ $%s\033[0m\n", vs.def.Name))
		} else {
			sb.WriteString(fmt.Sprintf("\033[90m○ $%s\033[0m\n", vs.def.Name))
		}
	}
	sb.WriteString("\033[90m─────────────────────────────────────────\033[0m")

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

	if strings.TrimSpace(v.Shell) == "" {
		return PromptWithTUI(v.Name, header, customHeader, prefill)
	}

	// Substitute scope into shell command
	shellCmd := v.Shell
	for name, value := range scope {
		shellCmd = strings.ReplaceAll(shellCmd, "$"+name, value)
	}

	output, err := exec.RunShell(shellCmd)
	if err != nil {
		return PromptWithTUI(v.Name, header, customHeader, prefill)
	}

	lines := splitLines(output)
	switch len(lines) {
	case 0, 1:
		return PromptWithTUI(v.Name, header, customHeader, prefill)
	default:
		return SelectWithTUI(v.Name, lines, header, customHeader, prefill)
	}
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

// findCommandVars finds all $varname patterns in a command
func findCommandVars(cmd string, scope map[string]string) []string {
	var vars []string
	seen := make(map[string]bool)

	for i := 0; i < len(cmd); i++ {
		if cmd[i] != '$' || i+1 >= len(cmd) {
			continue
		}
		if i > 0 && cmd[i-1] == '\\' {
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
