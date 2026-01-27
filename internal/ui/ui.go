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

const (
	fieldSep = "\x1f" // Unit separator
)

// varState tracks a variable and its resolved value
type varState struct {
	def      parser.VarDef
	value    string
	resolved bool
	prefill  string // Pre-filled value from environment (shown but requires confirmation)
}

// Run launches the fzf interface with proper looping for ESC support
func Run(index *parser.CheatIndex, exec *executor.Executor, initialQuery string) error {
	// Get color config
	colorHeader := config.GetColorHeader()
	colorCommand := config.GetColorCommand()
	colorDesc := config.GetColorDesc()
	columnGap := config.GetColumnGap()
	maxHeaderLen := config.GetColumnHeader()
	maxDescLen := config.GetColumnDesc()
	maxCmdLen := config.GetColumnCommand()
	requireCheatBlock := config.GetRequireCheatBlock()

	for {
		// Filter cheats based on config
		var cheats []*parser.Cheat
		for _, cheat := range index.Cheats {
			if requireCheatBlock && !cheat.HasCheatBlock {
				continue
			}
			cheats = append(cheats, cheat)
		}

		if len(cheats) == 0 {
			return fmt.Errorf("no cheats found")
		}

		// Build padding string
		gap := strings.Repeat(" ", columnGap)

		// Build the list of cheats for fzf
		// Simple format: idx<tab>displayLine (tab-separated, fzf searches everything)
		var lines []string

		for i, cheat := range cheats {
			// Truncate header if needed
			header := cheat.Header
			if len(header) > maxHeaderLen {
				header = header[:maxHeaderLen-1] + "…"
			}

			// Short description (first line, truncated)
			descFirstLine := strings.Split(cheat.Description, "\n")[0]
			shortDesc := descFirstLine
			if len(shortDesc) > maxDescLen {
				shortDesc = shortDesc[:maxDescLen-3] + "..."
			}

			// Truncate command if needed
			cmd := cheat.Command
			if len(cmd) > maxCmdLen {
				cmd = cmd[:maxCmdLen-1] + "…"
			}

			// Pad columns
			headerPadded := fmt.Sprintf("%-*s", maxHeaderLen, header)
			descPadded := fmt.Sprintf("%-*s", maxDescLen, shortDesc)

			// Store full description for preview (escape newlines)
			fullDesc := strings.ReplaceAll(cheat.Description, "\n", "\\n")

			// Simple format: idx \x1f colored_display \x1f fullDesc \x1f fullHeader \x1f fullCmd
			// The colored display includes all text so fzf can search it
			coloredDisplay := fmt.Sprintf("\033[%sm%s\033[0m%s\033[%sm%s\033[0m%s\033[%sm%s\033[0m",
				colorHeader, headerPadded, gap,
				colorDesc, descPadded, gap,
				colorCommand, cmd)

			line := fmt.Sprintf("%d\t%s\t%s\t%s\t%s",
				i,
				coloredDisplay,
				fullDesc,
				cheat.Header,
				cheat.Command)
			lines = append(lines, line)
		}

		if len(lines) == 0 {
			return fmt.Errorf("no cheats to display")
		}

		// Run fzf to select a cheat
		selectedIdx, cancelled, err := runMainSelection(lines, initialQuery)
		if err != nil {
			return err
		}

		if cancelled {
			return nil // User pressed ESC/Ctrl-C from main menu - exit
		}

		if selectedIdx < 0 || selectedIdx >= len(cheats) {
			continue
		}

		cheat := cheats[selectedIdx]

		// Resolve variables with back support
		cheat.Scope = make(map[string]string)
		goBack, err := resolveAllVariables(cheat, index, exec)
		if err != nil {
			return err
		}
		if goBack {
			initialQuery = "" // Clear query when going back to main
			continue
		}

		// Build final command
		finalCmd := exec.BuildFinalCommand(cheat)

		// Output - this is the end
		return executeOutput(finalCmd, exec)
	}
}

// runMainSelection runs fzf for the main cheat selection
func runMainSelection(lines []string, query string) (int, bool, error) {
	// Get colors from config for preview
	colorHeader := config.GetColorHeader()
	colorDesc := config.GetColorDesc()
	colorCommand := config.GetColorCommand()

	// Preview shows full header, description, and command
	// Tab-separated fields: idx(1) display(2) fullDesc(3) fullHeader(4) fullCmd(5)
	previewCmd := fmt.Sprintf(`
header=$(echo {} | cut -f4)
cmd=$(echo {} | cut -f5)
desc=$(echo {} | cut -f3 | sed 's/\\n/\n/g')

echo -e "\033[%sm$header\033[0m"
if [ -n "$desc" ]; then
  echo -e "\033[%sm$desc\033[0m"
  echo ""
fi
echo -e "\033[%sm$cmd\033[0m"
`, colorHeader, colorDesc, colorCommand)

	args := []string{
		"--height=100%",
		"--layout=reverse",
		"--border",
		"--info=inline",
		"--prompt", "❯ ",
		"--pointer", "▶",
		"--marker", "✓",
		"--header", "Select a cheat • ESC to exit",
		"--preview-window", "up:6:wrap",
		"--preview", previewCmd,
		"--ansi",
		"--delimiter", "\t",
		"--with-nth", "2", // Display only the colored line
	}

	if query != "" {
		args = append(args, "--query", query)
	}

	if extraOpts := config.GetFzfOpts(); extraOpts != "" {
		args = append(args, strings.Fields(extraOpts)...)
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 130 || exitErr.ExitCode() == 1 {
				return -1, true, nil // Cancelled
			}
		}
		return -1, false, err
	}

	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return -1, true, nil
	}

	parts := strings.Split(selected, "\t")
	if len(parts) < 1 {
		return -1, true, nil
	}

	var idx int
	fmt.Sscanf(parts[0], "%d", &idx)
	return idx, false, nil
}

// resolveAllVariables resolves all variables with back navigation support
// Returns (goBackToMain, error)
func resolveAllVariables(cheat *parser.Cheat, index *parser.CheatIndex, exec *executor.Executor) (bool, error) {
	// Find which variables are actually used in the command
	usedVars := findCommandVars(cheat.Command, nil)
	usedSet := make(map[string]bool)
	for _, v := range usedVars {
		usedSet[v] = true
	}

	// Collect all variable definitions (imports + local), but only include used ones
	varDefs := make(map[string]parser.VarDef)

	// Recursively collect vars from imports (handles nested imports)
	var collectImportVars func(imports []string, seen map[string]bool)
	collectImportVars = func(imports []string, seen map[string]bool) {
		for _, importName := range imports {
			if seen[importName] {
				continue // Avoid circular imports
			}
			seen[importName] = true
			if module, ok := index.Modules[importName]; ok {
				// First resolve nested imports
				collectImportVars(module.Imports, seen)
				// Then add this module's vars
				for _, v := range module.Vars {
					if _, exists := varDefs[v.Name]; !exists {
						varDefs[v.Name] = v
					}
				}
			}
		}
	}
	collectImportVars(cheat.Imports, make(map[string]bool))

	// Local definitions override imports
	for _, v := range cheat.Vars {
		varDefs[v.Name] = v
	}

	// Build ordered list of variables that are actually used in the command
	var allVars []varState
	for _, varName := range usedVars {
		if def, ok := varDefs[varName]; ok {
			allVars = append(allVars, varState{def: def})
		} else {
			// Undefined var - prompt for it
			allVars = append(allVars, varState{
				def: parser.VarDef{Name: varName, Shell: ""},
			})
		}
	}

	if len(allVars) == 0 {
		return false, nil // No vars to resolve
	}

	// Pre-fill variables from environment (don't auto-resolve, just set prefill)
	for i := range allVars {
		envVal := os.Getenv(allVars[i].def.Name)
		if envVal != "" {
			allVars[i].prefill = envVal
		}
	}

	// Resolve variables with back support
	currentIdx := 0

	for currentIdx < len(allVars) {
		if currentIdx < 0 {
			// User pressed back past first var - go back to main selection
			return true, nil
		}

		vs := &allVars[currentIdx]

		// Skip if already resolved from env
		if vs.resolved {
			currentIdx++
			continue
		}

		// Build current scope from resolved vars
		scope := make(map[string]string)
		for i := 0; i < len(allVars); i++ {
			if allVars[i].resolved {
				scope[allVars[i].def.Name] = allVars[i].value
			}
		}

		// Build header showing command progress and var status
		header := buildProgressHeader(cheat.Command, allVars, currentIdx)

		value, goBack, err := resolveVarWithHeader(vs.def, scope, exec, header, vs.prefill)
		if err != nil {
			return false, err
		}

		if goBack {
			// Go back to previous variable
			currentIdx--
			if currentIdx >= 0 {
				allVars[currentIdx].resolved = false
				allVars[currentIdx].value = ""
			}
			continue
		}

		vs.value = value
		vs.resolved = true
		scope[vs.def.Name] = value
		currentIdx++
	}

	// Copy resolved values to cheat scope
	for _, vs := range allVars {
		if vs.resolved {
			cheat.Scope[vs.def.Name] = vs.value
		}
	}

	return false, nil
}

// buildProgressHeader creates a header showing command progress and var status
func buildProgressHeader(cmd string, vars []varState, currentIdx int) string {
	var sb strings.Builder

	// Show command with current progress
	progressCmd := cmd
	for i, vs := range vars {
		if vs.resolved {
			// Replace $varname with the actual value (highlighted)
			progressCmd = replaceVar(progressCmd, vs.def.Name, fmt.Sprintf("\033[1;33m%s\033[0m", vs.value))
		} else if i == currentIdx {
			// Current var being filled - highlight placeholder
			progressCmd = replaceVar(progressCmd, vs.def.Name, fmt.Sprintf("\033[1;35m$%s\033[0m", vs.def.Name))
		}
		// Leave unfilled future vars as-is
	}

	sb.WriteString(fmt.Sprintf("\033[1;32m%s\033[0m\n", progressCmd))
	sb.WriteString("\033[90m─────────────────────────────────────────\033[0m\n")

	// Show variable status list - stacked vertically
	for i, vs := range vars {
		if vs.resolved {
			sb.WriteString(fmt.Sprintf("\033[32m✓\033[0m \033[90m$%s\033[0m = \033[33m%s\033[0m\n", vs.def.Name, vs.value))
		} else if i == currentIdx {
			sb.WriteString(fmt.Sprintf("\033[1;35m▶ $%s\033[0m \033[90m← current\033[0m\n", vs.def.Name))
		} else {
			sb.WriteString(fmt.Sprintf("\033[90m○ $%s\033[0m\n", vs.def.Name))
		}
	}
	sb.WriteString("\033[90m─────────────────────────────────────────\033[0m")

	return sb.String()
}

// replaceVar replaces $varname in cmd with replacement
func replaceVar(cmd, varName, replacement string) string {
	// Match $varname but not $$varname or \$varname
	re := regexp.MustCompile(`\$` + regexp.QuoteMeta(varName) + `\b`)
	return re.ReplaceAllString(cmd, replacement)
}

// resolveVarWithHeader resolves a single variable with a custom header
// prefill is an optional value to pre-populate the input (e.g., from environment variable)
func resolveVarWithHeader(v parser.VarDef, scope map[string]string, exec *executor.Executor, header string, prefill string) (string, bool, error) {
	// If shell command is empty, prompt for input
	if strings.TrimSpace(v.Shell) == "" {
		return promptForValueWithHeader(v.Name, nil, v.FzfArgs, header, prefill)
	}

	shellCmd := v.Shell
	for name, value := range scope {
		shellCmd = strings.ReplaceAll(shellCmd, "$"+name, value)
	}

	output, err := exec.RunShell(shellCmd)
	if err != nil {
		// Shell command failed, prompt for manual input
		return promptForValueWithHeader(v.Name, nil, v.FzfArgs, header, prefill)
	}

	lines := splitLines(output)

	switch len(lines) {
	case 0:
		return promptForValueWithHeader(v.Name, nil, v.FzfArgs, header, prefill)
	case 1:
		// Single value - still show it for confirmation/override option
		return promptForValueWithHeader(v.Name, lines, v.FzfArgs, header, prefill)
	default:
		return selectWithFzfAndHeader(v.Name, lines, v.FzfArgs, header, prefill)
	}
}

// selectWithFzfAndHeader uses fzf to select from options with custom header
// prefill is an optional value to pre-populate the query (e.g., from environment variable)
func selectWithFzfAndHeader(varName string, options []string, fzfArgs string, header string, prefill string) (string, bool, error) {
	// Parse fzfArgs properly to handle quoted strings
	parsedArgs := parseShellArgs(fzfArgs)

	// Check if user provided custom header
	customHeader := ""
	var filteredArgs []string
	for i := 0; i < len(parsedArgs); i++ {
		if parsedArgs[i] == "--header" && i+1 < len(parsedArgs) {
			customHeader = parsedArgs[i+1]
			i++ // Skip next arg
		} else {
			filteredArgs = append(filteredArgs, parsedArgs[i])
		}
	}

	// Build header - use custom header if provided, otherwise default
	var fullHeader string
	if customHeader != "" {
		fullHeader = header + fmt.Sprintf("\n\n%s • ESC to go back", customHeader)
	} else {
		fullHeader = header + fmt.Sprintf("\n\nSelect value for \033[1;35m$%s\033[0m • ESC to go back", varName)
	}

	args := []string{
		"--height=100%",
		"--layout=reverse",
		"--border",
		"--header", fullHeader,
		"--prompt", fmt.Sprintf("%s ❯ ", varName),
		"--ansi",
		"--print-query",
	}

	// Pre-fill query from environment variable if available
	if prefill != "" {
		args = append(args, "--query", prefill)
	}

	// Handle multi-select
	for _, arg := range filteredArgs {
		if arg == "--multi" || arg == "-m" {
			args = append(args, "--multi")
		} else {
			args = append(args, arg)
		}
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(options, "\n"))
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()

	// Parse output - first line is query, rest are selections
	outputStr := string(output)
	outputLines := strings.Split(outputStr, "\n")

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 130 {
				return "", true, nil // ESC pressed - go back
			}
			// Exit code 1 = no match selected, check if query was typed
			if exitErr.ExitCode() == 1 && len(outputLines) > 0 {
				query := strings.TrimSpace(outputLines[0])
				if query != "" {
					return query, false, nil // Use typed query as value
				}
			}
		}
		return "", true, nil
	}

	// Get selected items (skip first line which is query)
	var selected []string
	for i := 1; i < len(outputLines); i++ {
		if t := strings.TrimSpace(outputLines[i]); t != "" {
			selected = append(selected, t)
		}
	}

	if len(selected) == 0 {
		// No selection but query might have been typed
		if len(outputLines) > 0 {
			query := strings.TrimSpace(outputLines[0])
			if query != "" {
				return query, false, nil
			}
		}
		return "", true, nil
	}

	// Join multi-select with spaces
	return strings.Join(selected, " "), false, nil
}

// promptForValueWithHeader prompts for manual input or shows suggestions
// prefill is an optional value to pre-populate the query (e.g., from environment variable)
func promptForValueWithHeader(varName string, suggestions []string, fzfArgs string, header string, prefill string) (string, bool, error) {
	// Parse fzfArgs for custom header
	parsedArgs := parseShellArgs(fzfArgs)
	customHeader := ""
	for i := 0; i < len(parsedArgs); i++ {
		if parsedArgs[i] == "--header" && i+1 < len(parsedArgs) {
			customHeader = parsedArgs[i+1]
			break
		}
	}

	var fullHeader string
	if customHeader != "" {
		fullHeader = header + fmt.Sprintf("\n\n%s • Type and press Enter • ESC to go back", customHeader)
	} else {
		fullHeader = header + fmt.Sprintf("\n\nEnter value for \033[1;35m$%s\033[0m • Type and press Enter • ESC to go back", varName)
	}

	args := []string{
		"--height=100%",
		"--layout=reverse",
		"--border",
		"--print-query",
		"--header", fullHeader,
		"--prompt", fmt.Sprintf("%s ❯ ", varName),
		"--ansi",
	}

	// Pre-fill query from environment variable if available
	if prefill != "" {
		args = append(args, "--query", prefill)
	}

	var input string
	if len(suggestions) > 0 {
		input = strings.Join(suggestions, "\n")
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	outputLines := strings.Split(string(output), "\n")

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 130 = ESC/Ctrl-C = go back
			if exitErr.ExitCode() == 130 {
				return "", true, nil
			}
			// Exit code 1 = no match, use whatever was typed (even if empty)
			if exitErr.ExitCode() == 1 && len(outputLines) > 0 {
				query := strings.TrimSpace(outputLines[0])
				return query, false, nil
			}
		}
		// Any other error = go back
		return "", true, nil
	}

	// Check for selection first (line 2+), then query (line 1)
	if len(outputLines) > 1 {
		for i := 1; i < len(outputLines); i++ {
			if sel := strings.TrimSpace(outputLines[i]); sel != "" {
				return sel, false, nil
			}
		}
	}

	// Use query
	if len(outputLines) > 0 {
		query := strings.TrimSpace(outputLines[0])
		return query, false, nil
	}

	return "", true, nil
}

// executeOutput handles the final command based on output mode
func executeOutput(command string, exec *executor.Executor) error {
	mode := config.GetOutput()

	// Apply pre/post hooks to the command itself
	finalCmd := command
	if preHook := config.GetPreHook(); preHook != "" {
		finalCmd = preHook + finalCmd
	}
	if postHook := config.GetPostHook(); postHook != "" {
		finalCmd = finalCmd + postHook
	}

	switch mode {
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

func copyToClipboard(text string) error {
	var copyCmd *exec.Cmd

	if _, err := exec.LookPath("wl-copy"); err == nil {
		copyCmd = exec.Command("wl-copy")
	} else if _, err := exec.LookPath("xclip"); err == nil {
		copyCmd = exec.Command("xclip", "-selection", "clipboard")
	} else if _, err := exec.LookPath("xsel"); err == nil {
		copyCmd = exec.Command("xsel", "--clipboard", "--input")
	} else if _, err := exec.LookPath("pbcopy"); err == nil {
		copyCmd = exec.Command("pbcopy")
	} else {
		fmt.Print(text)
		return nil
	}

	copyCmd.Stdin = strings.NewReader(text)
	return copyCmd.Run()
}

// findCommandVars finds all $varname patterns in a command
func findCommandVars(cmd string, scope map[string]string) []string {
	var vars []string
	seen := make(map[string]bool)
	i := 0
	for i < len(cmd) {
		if cmd[i] == '$' && i+1 < len(cmd) {
			if i > 0 && cmd[i-1] == '\\' {
				i++
				continue
			}
			j := i + 1
			for j < len(cmd) && isVarChar(cmd[j], j == i+1) {
				j++
			}
			if j > i+1 {
				varName := cmd[i+1 : j]
				if !seen[varName] {
					if scope == nil || scope[varName] == "" {
						vars = append(vars, varName)
						seen[varName] = true
					}
				}
			}
			i = j
		} else {
			i++
		}
	}
	return vars
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func isVarChar(c byte, first bool) bool {
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' {
		return true
	}
	if !first && c >= '0' && c <= '9' {
		return true
	}
	return false
}

// parseShellArgs parses a string into arguments, respecting quotes
func parseShellArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]

		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(c)
			}
		} else {
			if c == '"' || c == '\'' {
				inQuote = true
				quoteChar = c
			} else if c == ' ' || c == '\t' {
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			} else {
				current.WriteByte(c)
			}
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}
