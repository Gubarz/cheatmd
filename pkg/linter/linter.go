// Package linter checks a cheatmd cheats directory for problems.
//
// It runs three categories of checks:
//
//  1. DSL syntax: each line inside a `<!-- cheat -->` block must be one of
//     `var`/`if`/`fi`/`export`/`import`, blank, or a `#` comment. Var names
//     must be valid; `if` must have a matching `fi`.
//  2. References: every `import foo` must resolve to an `export foo` defined
//     somewhere in the index. Duplicate `export` names are flagged.
//     `$var`/`<var>` references in commands should be declared (in a cheat
//     block or imported module), and undeclared references are warnings.
//  3. Structural: empty `##` headers, missing code blocks, duplicate `##`
//     headers within one file.
package linter

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gubarz/cheatmd/pkg/parser"
)

// Severity classifies a finding.
type Severity int

const (
	// SeverityError indicates a genuine problem: malformed DSL, missing
	// import target, duplicate export, etc.
	SeverityError Severity = iota
	// SeverityWarning indicates a recommendation or potential issue:
	// undeclared variables, duplicate headers within a file.
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	default:
		return "warning"
	}
}

// Finding is one issue surfaced by the linter.
type Finding struct {
	File     string
	Line     int // 1-indexed; 0 means "file-level" (no specific line)
	Column   int // 1-indexed; 0 means "line-level"
	Severity Severity
	Message  string
}

// Format renders the finding in GCC-style:
//
//	file:line:col: severity: message
//
// Line/col are omitted (replaced with 1) when not known.
func (f Finding) Format() string {
	line := f.Line
	if line < 1 {
		line = 1
	}
	col := f.Column
	if col < 1 {
		col = 1
	}
	return fmt.Sprintf("%s:%d:%d: %s: %s", f.File, line, col, f.Severity, f.Message)
}

// Lint walks path (file or directory), parses every markdown file, and
// returns all findings sorted by file/line.
func Lint(path string) ([]Finding, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	p := parser.NewParser()
	var index *parser.CheatIndex
	if info.IsDir() {
		index, err = p.ParseDirectory(path)
	} else {
		index, err = p.ParseSingleFile(path)
	}
	if err != nil {
		return nil, err
	}

	var findings []Finding

	for _, e := range index.Errors {
		severity := SeverityError
		if strings.Contains(e.Message, "cheat has no markdown header") ||
			strings.Contains(e.Message, "block has no preceding code block") {
			severity = SeverityWarning
		}
		findings = append(findings, Finding{
			File:     e.File,
			Line:     e.Line,
			Column:   1,
			Severity: severity,
			Message:  e.Message,
		})
	}

	indexFindings := lintIndex(index, info.IsDir())
	findings = append(findings, indexFindings...)

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Column < findings[j].Column
	})
	return findings, nil
}

// ============================================================================
// Whole-index checks
// ============================================================================

// lintIndex runs the parser and checks cross-cheat references: missing
// imports, duplicate exports, and undeclared variable references in commands.
func lintIndex(index *parser.CheatIndex, isDir bool) []Finding {
	var findings []Finding

	// Collect duplicate exports
	for _, dup := range index.Duplicates {
		findings = append(findings, Finding{
			File:     dup.File2,
			Line:     1,
			Column:   1,
			Severity: SeverityError,
			Message:  fmt.Sprintf("duplicate export %q (also defined in %s)", dup.Name, filepath.Base(dup.File1)),
		})
	}

	// Check duplicate cheat headers globally
	type cheatLoc struct {
		file string
		line int
	}
	headerLocs := make(map[string]cheatLoc)
	warnedHeaders := make(map[string]bool)
	for _, c := range index.Cheats {
		if c.Header == "" {
			continue
		}
		if firstLoc, exists := headerLocs[c.Header]; exists {
			if !warnedHeaders[c.Header] {
				msg := fmt.Sprintf("duplicate cheat name %q (also `# %s` at line %d)", c.Header, c.Header, firstLoc.line)
				if firstLoc.file != c.File {
					msg = fmt.Sprintf("duplicate cheat name %q (also `# %s` at %s:%d)", c.Header, c.Header, firstLoc.file, firstLoc.line)
				}
				findings = append(findings, Finding{
					File:     c.File,
					Line:     c.CommandStart,
					Column:   1,
					Severity: SeverityWarning,
					Message:  msg,
				})
				warnedHeaders[c.Header] = true
			}
		} else {
			headerLocs[c.Header] = cheatLoc{file: c.File, line: c.CommandStart}
		}
	}
	seenChainSteps := make(map[string]*parser.Cheat)
	chainSteps := make(map[string]map[int]*parser.Cheat)

	for _, c := range index.Cheats {
		if c.ChainName != "" {
			if chainSteps[c.ChainName] == nil {
				chainSteps[c.ChainName] = make(map[int]*parser.Cheat)
			}
			chainSteps[c.ChainName][c.ChainStep] = c
			key := fmt.Sprintf("%s:%d", c.ChainName, c.ChainStep)
			if existing := seenChainSteps[key]; existing != nil {
				line, col := findDSLRef(c.File, "chain", c.ChainName)
				findings = append(findings, Finding{
					File:     c.File,
					Line:     line,
					Column:   col,
					Severity: SeverityError,
					Message:  fmt.Sprintf("duplicate chain step %q %d (also in %s)", c.ChainName, c.ChainStep, existing.File),
				})
			} else {
				seenChainSteps[key] = c
			}
		}

		// Missing imports.
		for _, imp := range c.Imports {
			if _, ok := index.Modules[imp]; !ok {
				line, col := findDSLRef(c.File, "import", imp)
				findings = append(findings, Finding{
					File:     c.File,
					Line:     line,
					Column:   col,
					Severity: SeverityError,
					Message:  fmt.Sprintf("import %q does not resolve to any exported module", imp),
				})
			}
		}

		// Undeclared $var / <var> references in the command. Plain code
		// fences without a cheat block are allowed to contain ordinary shell
		// variables; only lint cheats that opted into metadata.
		if c.Export != "" || !c.HasCheatBlock {
			continue
		}
		if len(c.Command) > 0 {
			declared := declaredVarNames(c, index)
			addSyntaxDeclarations(c.Command, declared)
			for _, ref := range referencedVars(c) {
				if isMissing(ref, declared, c.Command) {
					findings = append(findings, Finding{
						File:     c.File,
						Line:     ref.Line,
						Column:   ref.Column,
						Severity: SeverityWarning,
						Message:  fmt.Sprintf("undeclared variable %q referenced in command", ref.Name),
					})
				}
			}
		}
	}
	for name, steps := range chainSteps {
		maxStep := 0
		var first *parser.Cheat
		for step, cheat := range steps {
			if first == nil || step < first.ChainStep {
				first = cheat
			}
			if step > maxStep {
				maxStep = step
			}
		}
		for step := 1; step <= maxStep; step++ {
			if steps[step] != nil {
				continue
			}
			line, col := 0, 0
			file := ""
			if first != nil {
				file = first.File
				line, col = findDSLRef(first.File, "chain", name)
			}
			findings = append(findings, Finding{
				File:     file,
				Line:     line,
				Column:   col,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("chain %q is missing step %d", name, step),
			})
		}
	}
	return findings
}

func findDSLRef(file, keyword, name string) (int, int) {
	f, err := os.Open(file)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	inCheat := false
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		if !inCheat {
			if content, ok := parser.ParseCheatSingleLine(line); ok {
				if dslLineMatches(content, keyword, name) {
					return lineNo, stringColumn(raw, name)
				}
				continue
			}
			if parser.IsCheatStart(line) {
				inCheat = true
			}
			continue
		}

		if parser.IsCheatEnd(line) {
			inCheat = false
			continue
		}
		if dslLineMatches(line, keyword, name) {
			return lineNo, stringColumn(raw, name)
		}
	}
	return 0, 0
}

func dslLineMatches(line, keyword, name string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}
	gotKeyword, rest := parser.SplitFirstWord(line)
	gotName, _ := parser.SplitFirstWord(rest)
	return gotKeyword == keyword && gotName == name
}

func stringColumn(line, needle string) int {
	if idx := strings.Index(line, needle); idx >= 0 {
		return idx + 1
	}
	return 1
}

// declaredVarNames returns the set of variable names declared for cheat c,
// counting its own `var` lines plus everything reachable through imports.
func declaredVarNames(c *parser.Cheat, index *parser.CheatIndex) map[string]bool {
	declared := make(map[string]bool)

	var walk func(modName string, seen map[string]bool)
	walk = func(modName string, seen map[string]bool) {
		if seen[modName] {
			return
		}
		seen[modName] = true
		mod, ok := index.Modules[modName]
		if !ok {
			return
		}
		for _, v := range mod.Vars {
			declared[v.Name] = true
		}
		for _, sub := range mod.Imports {
			walk(sub, seen)
		}
	}

	seen := make(map[string]bool)
	for _, imp := range c.Imports {
		walk(imp, seen)
	}
	for _, v := range c.Vars {
		declared[v.Name] = true
	}
	return declared
}

type RefKind int

const (
	RefShellParam RefKind = iota
	RefPowerShellParam
	RefAngleTemplate
	RefPerlParam
	refUnknownParam RefKind = 99
)

type Ref struct {
	Name   string
	Kind   RefKind
	Line   int
	Column int
}

func isMissing(ref Ref, declared map[string]bool, cmd string) bool {
	if declared[ref.Name] || declared[strings.ToLower(ref.Name)] {
		return false
	}
	if ref.Kind == RefAngleTemplate {
		return true
	}

	if isShellSpecial(ref.Name) {
		if ref.Kind == RefShellParam {
			return false
		}
	}
	if isPowerShellAutomatic(ref.Name) {
		if ref.Kind == RefPowerShellParam || isLikelyPowerShellCommand(cmd) || strings.Contains(strings.ToLower(cmd), "powershell") || strings.Contains(strings.ToLower(cmd), "pwsh") {
			return false
		}
	}
	if isPerlAutomatic(ref.Name) {
		if ref.Kind == RefPerlParam || strings.Contains(cmd, "perl") {
			return false
		}
	}

	return true
}

func referencedVars(c *parser.Cheat) []Ref {
	var refs []Ref
	seen := make(map[string]bool)

	cmd := c.Command
	kind := dollarRefKind(c.CommandLang)
	lang := lintLanguage(c.CommandLang)

	heredocBodyLines := heredocAngleSuppression(cmd)
	lines := strings.Split(cmd, "\n")
	for i, line := range lines {
		lineNo := c.CommandStart + i
		if c.CommandStart == 0 {
			lineNo = 0
		}
		for col := 0; col < len(line); col++ {
			switch line[col] {
			case '$':
				if lang == "shell" && inSingleQuotedShellText(line, col) {
					continue
				}
				ref, end, ok := scanDollarRef(line, col, kind, lineNo)
				if !ok {
					continue
				}
				key := fmt.Sprintf("%d:%s", ref.Kind, ref.Name)
				if !seen[key] {
					seen[key] = true
					refs = append(refs, ref)
				}
				col = end - 1
			case '<':
				if heredocBodyLines[i] {
					continue
				}
				ref, end, ok := scanAngleRef(line, col, lineNo)
				if !ok {
					continue
				}
				key := fmt.Sprintf("%d:%s", ref.Kind, ref.Name)
				if !seen[key] {
					seen[key] = true
					refs = append(refs, ref)
				}
				col = end
			}
		}
	}
	return refs
}

func scanDollarRef(line string, pos int, kind RefKind, lineNo int) (Ref, int, bool) {
	if pos > 0 && line[pos-1] == '\\' {
		return Ref{}, pos + 1, false
	}
	if pos+1 >= len(line) {
		return Ref{}, pos + 1, false
	}
	if line[pos+1] == '{' {
		j := pos + 2
		if j >= len(line) || !isShellBracedVarChar(line[j], true) {
			return Ref{}, pos + 1, false
		}
		j++
		for j < len(line) && isShellBracedVarChar(line[j], false) {
			j++
		}
		if j >= len(line) || line[j] != '}' {
			return Ref{}, pos + 1, false
		}
		return Ref{Name: line[pos+2 : j], Kind: kind, Line: lineNo, Column: pos + 1}, j + 1, true
	}
	next := line[pos+1]
	if kind == RefShellParam && isShellSingleSpecial(next) {
		return Ref{Name: string(next), Kind: kind, Line: lineNo, Column: pos + 1}, pos + 2, true
	}
	if !parser.IsVarChar(next, true) {
		return Ref{}, pos + 1, false
	}
	j := pos + 2
	for j < len(line) && parser.IsVarChar(line[j], false) {
		j++
	}
	if kind == RefPowerShellParam && j < len(line) && line[j] == ':' {
		return Ref{}, j + 1, false
	}
	return Ref{Name: line[pos+1 : j], Kind: kind, Line: lineNo, Column: pos + 1}, j, true
}

func scanAngleRef(line string, pos int, lineNo int) (Ref, int, bool) {
	j := pos + 1
	if j >= len(line) || !parser.IsVarChar(line[j], true) {
		return Ref{}, pos + 1, false
	}
	j++
	for j < len(line) && parser.IsVarChar(line[j], false) {
		j++
	}
	if j >= len(line) || line[j] != '>' {
		return Ref{}, pos + 1, false
	}
	return Ref{Name: line[pos+1 : j], Kind: RefAngleTemplate, Line: lineNo, Column: pos + 1}, j, true
}

func inSingleQuotedShellText(line string, pos int) bool {
	inSingle := false
	for i := 0; i < pos && i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '\'':
			inSingle = !inSingle
		}
	}
	return inSingle
}

func dollarRefKind(lang string) RefKind {
	switch strings.ToLower(lang) {
	case "", "sh", "bash", "zsh", "fish", "shell":
		return RefShellParam
	case "powershell", "pwsh", "ps1":
		return RefPowerShellParam
	default:
		return refUnknownParam
	}
}

var (
	localDeclRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:for|foreach)[[:space:]]+(?:\()?[[:space:]]*\$?([a-z_][a-z0-9_]*)[[:space:]]+(?:in|w)\b`),
		regexp.MustCompile(`(?i)(?:^|[;&|[:space:]"'])\$?([a-z_][a-z0-9_]*)[[:space:]]*=`),
		regexp.MustCompile(`(?i)\bread(?:[[:space:]]+-[a-z]+)*[[:space:]]+([a-z_][a-z0-9_]*(?:[[:space:]]+[a-z_][a-z0-9_]*)*)`),
		regexp.MustCompile(`(?i)\b(?:local|declare|typeset|export|readonly)[[:space:]]+(?:-[a-z]+[[:space:]]+)*([^;&|]+)`),
		regexp.MustCompile(`(?i)\b(?:set|gets(?:[[:space:]]+\$?[a-z_][a-z0-9_]*)?)[[:space:]]+([a-z_][a-z0-9_]*)\b`),
		regexp.MustCompile(`(?i)\bmy[[:space:]]+\$(?:\{)?([a-z_][a-z0-9_]*)(?:\})?`),
		regexp.MustCompile(`(?i)\b(?:proc_open|exec)\([^;]*,\s*\$([a-z_][a-z0-9_]*)\b`),
		regexp.MustCompile(`(?i)\b(?:param|function[[:space:]]+[a-z_][a-z0-9_-]*)[[:space:]]*\(([^)]*)\)`),
		regexp.MustCompile(`(?i)\$([a-z_][a-z0-9_]*)(?:(?:\.|->)[a-z_][a-z0-9_]*)+\(`),
	}
	psVarInParamRe = regexp.MustCompile(`(?i)\$([a-z_][a-z0-9_]*)`)
)

func addSyntaxDeclarations(cmd string, declared map[string]bool) {
	for _, re := range localDeclRegexes {
		for _, m := range re.FindAllStringSubmatch(cmd, -1) {
			if len(m) < 2 {
				continue
			}
			fullMatchLower := strings.ToLower(m[0])
			if strings.HasPrefix(fullMatchLower, "param") || strings.HasPrefix(fullMatchLower, "function") {
				for _, pm := range psVarInParamRe.FindAllStringSubmatch(m[1], -1) {
					declared[pm[1]] = true
					declared[strings.ToLower(pm[1])] = true
				}
				continue
			}

			if strings.HasPrefix(fullMatchLower, "read") || strings.HasPrefix(fullMatchLower, "local") || strings.HasPrefix(fullMatchLower, "declare") || strings.HasPrefix(fullMatchLower, "typeset") || strings.HasPrefix(fullMatchLower, "export") || strings.HasPrefix(fullMatchLower, "readonly") {
				for _, field := range strings.Fields(m[1]) {
					field = strings.TrimLeft(field, "-")
					if field == "" || strings.Contains(field, "=") {
						field = strings.SplitN(field, "=", 2)[0]
					}
					if isIdentifier(field) {
						declared[field] = true
						declared[strings.ToLower(field)] = true
					}
				}
				continue
			}

			declared[m[1]] = true
			declared[strings.ToLower(m[1])] = true
		}
	}
}

func isLikelyPowerShellCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return false
	}
	first, _ := parser.SplitFirstWord(trimmed)
	firstLower := strings.ToLower(first)
	if strings.Contains(first, "-") {
		verb := strings.SplitN(firstLower, "-", 2)[0]
		switch verb {
		case "add", "clear", "compare", "convert", "copy", "export", "find", "for", "foreach",
			"format", "get", "import", "invoke", "measure", "move", "new", "out", "read",
			"remove", "rename", "resolve", "select", "set", "sort", "start", "stop", "where",
			"write":
			return true
		}
	}
	lower := strings.ToLower(cmd)
	return strings.Contains(cmd, "where-object") ||
		strings.Contains(cmd, "foreach-object") ||
		strings.Contains(lower, "$true") ||
		strings.Contains(lower, "$false") ||
		strings.Contains(lower, "$null")
}

func lintLanguage(lang string) string {
	switch dollarRefKind(lang) {
	case RefShellParam:
		return "shell"
	case RefPowerShellParam:
		return "powershell"
	default:
		return "unknown"
	}
}

func heredocAngleSuppression(cmd string) map[int]bool {
	suppressed := make(map[int]bool)
	lines := strings.Split(cmd, "\n")
	endMarker := ""
	for i, line := range lines {
		if endMarker != "" {
			if strings.TrimSpace(line) == endMarker {
				endMarker = ""
				continue
			}
			suppressed[i] = true
			continue
		}
		if marker, ok := heredocMarker(line); ok {
			endMarker = marker
		}
	}
	return suppressed
}

func heredocMarker(line string) (string, bool) {
	idx := strings.Index(line, "<<")
	if idx == -1 {
		return "", false
	}
	rest := strings.TrimSpace(line[idx+2:])
	if strings.HasPrefix(rest, "-") {
		rest = strings.TrimSpace(rest[1:])
	}
	if rest == "" {
		return "", false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}
	marker := strings.Trim(fields[0], `"'`)
	if marker == "" {
		return "", false
	}
	return marker, true
}

func isIdentifier(s string) bool {
	if s == "" || !parser.IsVarChar(s[0], true) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !parser.IsVarChar(s[i], false) {
			return false
		}
	}
	return true
}

func isShellSpecial(name string) bool {
	if name == "" {
		return false
	}
	if allDigits(name) {
		return true
	}
	switch name {
	case "@", "*", "#", "?", "$", "!", "-", "_",
		"RANDOM", "SECONDS", "LINENO", "REPLY", "PPID",
		"PWD", "OLDPWD", "HOME", "USER", "UID", "EUID", "HOSTNAME", "SHELL", "PATH", "IFS":
		return true
	default:
		return false
	}
}

func isPowerShellAutomatic(name string) bool {
	switch strings.ToLower(name) {
	case "true", "false", "null", "_", "psitem", "args", "input", "this",
		"error", "matches", "host", "pid", "pwd", "pshome", "psversiontable":
		return true
	default:
		return false
	}
}

func isPerlAutomatic(name string) bool {
	return name == "_"
}

func isShellBracedVarChar(c byte, first bool) bool {
	if c >= '0' && c <= '9' {
		return true
	}
	return parser.IsVarChar(c, first)
}

func isShellSingleSpecial(c byte) bool {
	return (c >= '0' && c <= '9') || strings.ContainsRune("@*#?$!-_", rune(c))
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
