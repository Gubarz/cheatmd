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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gubarz/cheatmd/internal/parser"
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

	files, err := collectFiles(path, info.IsDir())
	if err != nil {
		return nil, err
	}

	var findings []Finding

	// Per-file syntax + structural checks.
	for _, f := range files {
		fs, err := lintFile(f)
		if err != nil {
			findings = append(findings, Finding{
				File:     f,
				Severity: SeverityError,
				Message:  fmt.Sprintf("read error: %v", err),
			})
			continue
		}
		findings = append(findings, fs...)
	}

	// Whole-index checks (imports, undeclared refs, duplicate exports).
	indexFindings, err := lintIndex(path, info.IsDir())
	if err == nil {
		findings = append(findings, indexFindings...)
	}

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

// collectFiles walks path and returns every markdown file. Single-file paths
// are returned as a one-element slice.
func collectFiles(path string, isDir bool) ([]string, error) {
	if !isDir {
		if !isMarkdown(path) {
			return nil, fmt.Errorf("%s is not a markdown file", path)
		}
		return []string{path}, nil
	}
	var files []string
	err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isMarkdown(p) {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

func isMarkdown(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".md"
}

// ============================================================================
// Per-file checks
// ============================================================================

// lintFile reads the file once and produces findings for DSL syntax,
// structural, and intra-file issues.
func lintFile(path string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []Finding

	type cheatBlock struct {
		startLine int
		lines     []string // lines inside the block, in order
		lineNos   []int    // 1-indexed line numbers for each entry above
	}
	var (
		inCheat              bool
		inCodeFence          bool
		curBlock             cheatBlock
		cheatNameCounts      = map[string]int{}
		cheatNameLineNos     = map[string]int{}
		cheatNameDisplays    = map[string]string{}
		currentHeader        string
		currentHeaderDisplay string
		currentHeaderLine    int
		// Track whether the previous non-blank line was a fence close, so we
		// can flag `<!-- cheat -->` blocks not preceded by a fence.
		pendingFence bool
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		// Code fence boundaries.
		if strings.HasPrefix(line, "```") {
			if inCodeFence {
				inCodeFence = false
				pendingFence = true
			} else if !inCheat {
				inCodeFence = true
			}
			continue
		}
		if inCodeFence {
			continue
		}

		// Cheat block boundaries.
		if !inCheat {
			if content, ok := parseCheatSingleLine(line); ok {
				isExport := cheatBlockExports(content)
				if !pendingFence && !isExport {
					findings = append(findings, Finding{
						File:     path,
						Line:     lineNo,
						Column:   1,
						Severity: SeverityWarning,
						Message:  "<!-- cheat --> block has no preceding code block",
					})
				}
				if pendingFence && !isExport {
					if currentHeader == "" {
						findings = append(findings, Finding{
							File:     path,
							Line:     lineNo,
							Column:   1,
							Severity: SeverityWarning,
							Message:  "cheat has no markdown header",
						})
					} else {
						findings = append(findings, lintDuplicateCheatName(
							path, currentHeader, currentHeaderDisplay, currentHeaderLine,
							cheatNameCounts, cheatNameLineNos, cheatNameDisplays,
						)...)
					}
				}
				findings = append(findings, lintCheatBlock(path, cheatBlock{
					startLine: lineNo,
					lines:     []string{content},
					lineNos:   []int{lineNo},
				})...)
				pendingFence = false
				continue
			}
		}
		if !inCheat && isCheatStart(line) {
			inCheat = true
			curBlock = cheatBlock{startLine: lineNo}
			// Flag if not preceded by a fence (no command to attach to).
			continue
		}
		if inCheat {
			if isCheatEnd(line) {
				findings = append(findings, lintCheatBlock(path, curBlock)...)
				isExport := cheatBlockExports(strings.Join(curBlock.lines, "\n"))
				if !pendingFence && !isExport {
					findings = append(findings, Finding{
						File:     path,
						Line:     curBlock.startLine,
						Column:   1,
						Severity: SeverityWarning,
						Message:  "<!-- cheat --> block has no preceding code block",
					})
				}
				if pendingFence && !isExport {
					if currentHeader == "" {
						findings = append(findings, Finding{
							File:     path,
							Line:     curBlock.startLine,
							Column:   1,
							Severity: SeverityWarning,
							Message:  "cheat has no markdown header",
						})
					} else {
						findings = append(findings, lintDuplicateCheatName(
							path, currentHeader, currentHeaderDisplay, currentHeaderLine,
							cheatNameCounts, cheatNameLineNos, cheatNameDisplays,
						)...)
					}
				}
				inCheat = false
				curBlock = cheatBlock{}
				pendingFence = false
				continue
			}
			curBlock.lines = append(curBlock.lines, line)
			curBlock.lineNos = append(curBlock.lineNos, lineNo)
			continue
		}

		// Markdown header tracking for empty-header and code-block/comment-block
		// naming checks. Duplicate checks happen only for actual cheats.
		if header, level, ok := parseMarkdownHeader(line); ok {
			if header == "" {
				currentHeader = ""
				currentHeaderDisplay = ""
				currentHeaderLine = 0
				findings = append(findings, Finding{
					File:     path,
					Line:     lineNo,
					Column:   1,
					Severity: SeverityWarning,
					Message:  "empty markdown header",
				})
			} else {
				currentHeader = header
				currentHeaderDisplay = fmt.Sprintf("%s %s", strings.Repeat("#", level), header)
				currentHeaderLine = lineNo
			}
		}

		// Only fences reset the pending-fence flag toward the next cheat block.
		if line != "" {
			pendingFence = false
		}
	}
	if err := scanner.Err(); err != nil {
		return findings, err
	}

	// Unterminated cheat block.
	if inCheat {
		findings = append(findings, Finding{
			File:     path,
			Line:     curBlock.startLine,
			Column:   1,
			Severity: SeverityError,
			Message:  "unterminated `<!-- cheat -->` block (missing `-->`)",
		})
	}

	return findings, nil
}

func lintDuplicateCheatName(file, header, display string, lineNo int, counts map[string]int, lineNos map[string]int, displays map[string]string) []Finding {
	counts[header]++
	if counts[header] == 1 {
		lineNos[header] = lineNo
		displays[header] = display
		return nil
	}
	if counts[header] > 2 {
		return nil
	}
	return []Finding{{
		File:     file,
		Line:     lineNo,
		Column:   1,
		Severity: SeverityWarning,
		Message: fmt.Sprintf(
			"duplicate cheat name %q (also `%s` at line %d)",
			header, displays[header], lineNos[header],
		),
	}}
}

func isCheatStart(trimmed string) bool {
	// "<!-- cheat" (case-insensitive), nothing else after besides whitespace.
	if !strings.HasPrefix(trimmed, "<!--") {
		return false
	}
	inner := strings.TrimSpace(strings.TrimPrefix(trimmed, "<!--"))
	return strings.EqualFold(inner, "cheat")
}

func parseMarkdownHeader(trimmed string) (header string, level int, ok bool) {
	level = 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return "", 0, false
	}
	if level == len(trimmed) {
		return "", level, true
	}
	if trimmed[level] != ' ' && trimmed[level] != '\t' {
		return "", 0, false
	}
	return strings.TrimSpace(trimmed[level:]), level, true
}

func parseCheatSingleLine(trimmed string) (string, bool) {
	if !strings.HasPrefix(trimmed, "<!--") || !strings.HasSuffix(trimmed, "-->") {
		return "", false
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "<!--"), "-->"))
	if len(inner) < len("cheat") || !strings.EqualFold(inner[:len("cheat")], "cheat") {
		return "", false
	}
	if len(inner) > len("cheat") {
		next := inner[len("cheat")]
		if next != ' ' && next != '\t' {
			return "", false
		}
	}
	return strings.TrimSpace(inner[len("cheat"):]), true
}

func isCheatEnd(trimmed string) bool {
	return trimmed == "-->"
}

func cheatBlockExports(content string) bool {
	lines := joinContinuationLines(strings.Split(content, "\n"))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keyword, rest := splitFirstWord(line)
		if keyword == "export" && rest != "" && !containsWS(rest) {
			return true
		}
	}
	return false
}

func joinContinuationLines(lines []string) []string {
	var result []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.HasSuffix(trimmed, "\\") {
			current.WriteString(strings.TrimSuffix(trimmed, "\\"))
		} else {
			current.WriteString(line)
			result = append(result, current.String())
			current.Reset()
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

func joinContinuationLinesWithLineNos(lines []string, lineNos []int) ([]string, []int) {
	var (
		result    []string
		resultNos []int
		current   strings.Builder
		startNo   int
	)

	for i, line := range lines {
		if current.Len() == 0 {
			startNo = lineNos[i]
		}

		trimmed := strings.TrimRight(line, " \t")
		if strings.HasSuffix(trimmed, "\\") {
			current.WriteString(strings.TrimSuffix(trimmed, "\\"))
			continue
		}

		current.WriteString(line)
		result = append(result, current.String())
		resultNos = append(resultNos, startNo)
		current.Reset()
	}

	if current.Len() > 0 {
		result = append(result, current.String())
		resultNos = append(resultNos, startNo)
	}

	return result, resultNos
}

// ============================================================================
// DSL syntax check (inside a single <!-- cheat --> block)
// ============================================================================

// lintCheatBlock validates the DSL inside one cheat block. It expects all
// lines to be one of: `var`/`if`/`fi`/`export`/`import`, blank, or `#`-comment.
func lintCheatBlock(file string, b struct {
	startLine int
	lines     []string
	lineNos   []int
}) []Finding {
	var findings []Finding
	ifDepth := 0
	ifLines := []int{} // stack of `if` line numbers awaiting matching `fi`
	lines, lineNos := joinContinuationLinesWithLineNos(b.lines, b.lineNos)

	for i, line := range lines {
		lineNo := lineNos[i]
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		keyword, rest := splitFirstWord(line)
		switch keyword {
		case "fi":
			if rest != "" {
				findings = append(findings, Finding{
					File: file, Line: lineNo, Column: 1,
					Severity: SeverityError,
					Message:  fmt.Sprintf("`fi` takes no arguments, got %q", rest),
				})
				continue
			}
			if ifDepth == 0 {
				findings = append(findings, Finding{
					File: file, Line: lineNo, Column: 1,
					Severity: SeverityError,
					Message:  "`fi` without a matching `if`",
				})
			} else {
				ifDepth--
				ifLines = ifLines[:len(ifLines)-1]
			}
		case "if":
			if rest == "" {
				findings = append(findings, Finding{
					File: file, Line: lineNo, Column: 1,
					Severity: SeverityError,
					Message:  "`if` requires a condition",
				})
				continue
			}
			ifDepth++
			ifLines = append(ifLines, lineNo)
		case "export", "import":
			if rest == "" {
				findings = append(findings, Finding{
					File: file, Line: lineNo, Column: 1,
					Severity: SeverityError,
					Message:  fmt.Sprintf("`%s` requires a name", keyword),
				})
			} else if containsWS(rest) {
				findings = append(findings, Finding{
					File: file, Line: lineNo, Column: 1,
					Severity: SeverityError,
					Message:  fmt.Sprintf("`%s` name must be a single token", keyword),
				})
			}
		case "var":
			findings = append(findings, lintVarLine(file, lineNo, rest)...)
		default:
			findings = append(findings, Finding{
				File: file, Line: lineNo, Column: 1,
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"unknown DSL keyword %q (expected one of: var, if, fi, export, import)",
					keyword,
				),
			})
		}
	}

	for _, ln := range ifLines {
		findings = append(findings, Finding{
			File: file, Line: ln, Column: 1,
			Severity: SeverityError,
			Message:  "`if` without a matching `fi`",
		})
	}
	return findings
}

func lintVarLine(file string, lineNo int, rest string) []Finding {
	name, after := splitFirstWord(rest)
	if name == "" {
		return []Finding{{
			File: file, Line: lineNo, Column: 1,
			Severity: SeverityError,
			Message:  "`var` requires a name",
		}}
	}
	if !isValidVarName(name) {
		return []Finding{{
			File: file, Line: lineNo, Column: 1,
			Severity: SeverityError,
			Message:  fmt.Sprintf("invalid var name %q", name),
		}}
	}
	if after == "" {
		return nil // prompt-only var, valid
	}
	switch {
	case strings.HasPrefix(after, ":="):
		if strings.TrimSpace(after[2:]) == "" {
			return []Finding{{
				File: file, Line: lineNo, Column: 1,
				Severity: SeverityError,
				Message:  fmt.Sprintf("`var %s :=` has no value", name),
			}}
		}
	case after[0] == '=':
		if strings.TrimSpace(after[1:]) == "" {
			return []Finding{{
				File: file, Line: lineNo, Column: 1,
				Severity: SeverityError,
				Message:  fmt.Sprintf("`var %s =` has no shell command", name),
			}}
		}
	default:
		return []Finding{{
			File: file, Line: lineNo, Column: 1,
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"`var %s ...` is missing an assignment operator (use `=` for shell or `:=` for literal)",
				name,
			),
		}}
	}
	return nil
}

func splitFirstWord(s string) (head, rest string) {
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	head = s[:i]
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	rest = s[i:]
	return
}

func isValidVarName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func containsWS(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			return true
		}
	}
	return false
}

// ============================================================================
// Whole-index checks
// ============================================================================

// lintIndex runs the parser and checks cross-cheat references: missing
// imports, duplicate exports, and undeclared variable references in commands.
func lintIndex(path string, isDir bool) ([]Finding, error) {
	p := parser.NewParser()
	var (
		index *parser.CheatIndex
		err   error
	)
	if isDir {
		index, err = p.ParseDirectory(path)
	} else {
		index, err = p.ParseSingleFile(path)
	}
	if err != nil {
		return nil, err
	}
	if index == nil {
		return nil, errors.New("parser returned no index")
	}

	var findings []Finding

	// Duplicate exports.
	for _, dup := range index.Duplicates {
		line, col := findDSLRef(dup.File2, "export", dup.Name)
		findings = append(findings, Finding{
			File:     dup.File2,
			Line:     line,
			Column:   col,
			Severity: SeverityError,
			Message:  fmt.Sprintf("duplicate export %q (also exported in %s)", dup.Name, dup.File1),
		})
	}

	for _, c := range index.Cheats {
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
		declared := declaredVarNames(c, index)
		for _, name := range referencedVarNames(c.Command) {
			if _, ok := declared[name]; !ok {
				line, col := findCommandVarRef(c.File, c.Header, name)
				findings = append(findings, Finding{
					File:     c.File,
					Line:     line,
					Column:   col,
					Severity: SeverityWarning,
					Message: fmt.Sprintf(
						"variable %q referenced in header %q but not declared.",
						name, c.Header,
					),
				})
			}
		}
	}
	return findings, nil
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
			if content, ok := parseCheatSingleLine(line); ok {
				if dslLineMatches(content, keyword, name) {
					return lineNo, stringColumn(raw, name)
				}
				continue
			}
			if isCheatStart(line) {
				inCheat = true
			}
			continue
		}

		if isCheatEnd(line) {
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
	gotKeyword, rest := splitFirstWord(line)
	gotName, _ := splitFirstWord(rest)
	return gotKeyword == keyword && gotName == name
}

func findCommandVarRef(file, header, name string) (int, int) {
	f, err := os.Open(file)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	inTargetHeader := header == ""
	inCodeFence := false
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		if current, _, ok := parseMarkdownHeader(line); ok {
			inTargetHeader = current == header
			inCodeFence = false
			continue
		}
		if !inTargetHeader {
			continue
		}

		if strings.HasPrefix(line, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if !inCodeFence {
			continue
		}
		if col := commandRefColumn(raw, name); col > 0 {
			return lineNo, col
		}
	}
	return 0, 0
}

func commandRefColumn(line, name string) int {
	needle := "$" + name
	for start := 0; ; {
		idx := strings.Index(line[start:], needle)
		if idx == -1 {
			break
		}
		pos := start + idx
		end := pos + len(needle)
		if (pos == 0 || line[pos-1] != '\\') && (end == len(line) || !isVarChar(line[end], false)) {
			return pos + 1
		}
		start = end
	}

	needle = "<" + name + ">"
	if idx := strings.Index(line, needle); idx >= 0 {
		return idx + 1
	}
	return 0
}

func stringColumn(line, needle string) int {
	if idx := strings.Index(line, needle); idx >= 0 {
		return idx + 1
	}
	return 1
}

// declaredVarNames returns the set of variable names declared for cheat c,
// counting its own `var` lines plus everything reachable through imports.
func declaredVarNames(c *parser.Cheat, index *parser.CheatIndex) map[string]struct{} {
	declared := make(map[string]struct{})

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
			declared[v.Name] = struct{}{}
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
		declared[v.Name] = struct{}{}
	}
	return declared
}

// referencedVarNames extracts $name and <name> tokens from cmd. Always
// recognizes both forms so the linter flags issues regardless of the user's
// `var_syntax` config.
func referencedVarNames(cmd string) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}

	for i := 0; i < len(cmd); i++ {
		switch cmd[i] {
		case '$':
			if i+1 >= len(cmd) {
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
				add(cmd[i+1 : j])
			}
			i = j - 1
		case '<':
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
			if j >= len(cmd) || cmd[j] != '>' {
				continue
			}
			add(cmd[i+1 : j])
			i = j
		}
	}
	return out
}

func isVarChar(c byte, first bool) bool {
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
		return true
	}
	return !first && c >= '0' && c <= '9'
}
