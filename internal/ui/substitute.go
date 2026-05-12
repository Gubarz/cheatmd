package ui

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// substituteOption is one row shown in the substitute search picker.
//
// Display is what the user sees while filtering ("env: USER=alice").
// Value is what gets inserted into the var prompt if the row is chosen.
type substituteOption struct {
	Display string
	Value   string
}

// collectSubstituteOptions builds the picker list from the configured sources.
// `sources` may contain "env" and/or "history".
//
// Order: env vars first (sorted by name), then history (newest first).
func collectSubstituteOptions(sources []string) []substituteOption {
	wantEnv, wantHistory := false, false
	for _, s := range sources {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "env":
			wantEnv = true
		case "history":
			wantHistory = true
		}
	}

	var out []substituteOption
	if wantEnv {
		out = append(out, collectEnvOptions()...)
	}
	if wantHistory {
		out = append(out, collectHistoryOptions()...)
	}
	return out
}

// collectEnvOptions returns one row per exported environment variable.
// Display: "env: NAME=value", Value: the value alone.
func collectEnvOptions() []substituteOption {
	entries := os.Environ()
	sort.Strings(entries)

	out := make([]substituteOption, 0, len(entries))
	for _, e := range entries {
		eq := strings.IndexByte(e, '=')
		if eq <= 0 {
			continue
		}
		name := e[:eq]
		value := e[eq+1:]
		if value == "" {
			continue
		}
		// Multiline values (e.g. exported bash functions) would explode the
		// list into many visible rows. Collapse control chars so each option
		// stays exactly one row tall.
		displayValue := sanitizeOneLine(value)
		out = append(out, substituteOption{
			Display: "env: " + name + "=" + displayValue,
			Value:   value,
		})
	}
	return out
}

// sanitizeOneLine collapses newlines, tabs, and other control characters into
// single spaces so a value renders as exactly one visible row. The underlying
// Value is preserved separately for substitution.
func sanitizeOneLine(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || (r >= 0 && r < 32) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return b.String()
}

// collectHistoryOptions scans shell history for variable assignments
// (`VAR=value`, `export VAR=value`, etc.) and returns one row per unique
// assignment, newest first. Plain commands are ignored. Empty result if no
// readable history file is found.
func collectHistoryOptions() []substituteOption {
	path := findHistoryFile()
	if path == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	type entry struct {
		name, value string
	}
	var entries []entry
	for scanner.Scan() {
		line := strings.TrimSpace(stripHistoryPrefix(scanner.Text()))
		if line == "" {
			continue
		}
		for _, a := range extractAssignments(line) {
			entries = append(entries, entry{a.name, a.value})
		}
	}

	// Newest first, dedupe by "name=value".
	seen := make(map[string]struct{}, len(entries))
	out := make([]substituteOption, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		key := entries[i].name + "=" + entries[i].value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, substituteOption{
			Display: "hist: " + entries[i].name + "=" + sanitizeOneLine(entries[i].value),
			Value:   entries[i].value,
		})
	}
	return out
}

// assignment is a single VAR=value pair extracted from a history line.
type assignment struct {
	name, value string
}

// extractAssignments returns every assignment-shaped token in a line.
// Handles:
//
//	VAR=value
//	export VAR=value
//	declare -x VAR=value
//	typeset VAR=value
//	set VAR=value
//	FOO=bar BAR=baz some-command       (leading assignments)
//
// Values may be quoted with ' or "; the quotes are stripped.
func extractAssignments(line string) []assignment {
	tokens := splitShellTokens(line)
	var out []assignment
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok {
		case "export", "set", "typeset", "local", "readonly":
			continue
		case "declare":
			// skip any flag-like tokens that follow (e.g. -x, -gx)
			for i+1 < len(tokens) && strings.HasPrefix(tokens[i+1], "-") {
				i++
			}
			continue
		}
		if a, ok := parseAssignment(tok); ok {
			out = append(out, a)
		}
	}
	return out
}

// parseAssignment splits "NAME=value" if NAME is a valid shell var name.
// Returns false on anything else (commands, paths, flags, etc.).
func parseAssignment(tok string) (assignment, bool) {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return assignment{}, false
	}
	name := tok[:eq]
	value := tok[eq+1:]
	if !isValidVarName(name) {
		return assignment{}, false
	}
	// Strip a single layer of matching quotes.
	if n := len(value); n >= 2 {
		if (value[0] == '"' && value[n-1] == '"') || (value[0] == '\'' && value[n-1] == '\'') {
			value = value[1 : n-1]
		}
	}
	if value == "" {
		return assignment{}, false
	}
	return assignment{name: name, value: value}, true
}

// isValidVarName reports whether s is a valid POSIX shell variable name:
// starts with letter or underscore, then letters/digits/underscores.
func isValidVarName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
		if i == 0 {
			if !isLetter {
				return false
			}
			continue
		}
		isDigit := c >= '0' && c <= '9'
		if !isLetter && !isDigit {
			return false
		}
	}
	return true
}

// splitShellTokens splits a command line into whitespace-separated tokens,
// keeping quoted regions intact. It's not a full shell parser, just enough
// to handle the common cases in history.
func splitShellTokens(line string) []string {
	var tokens []string
	var cur strings.Builder
	var quote byte // 0, '\'', or '"'
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			cur.WriteByte(c)
			quote = c
		case c == ' ' || c == '\t':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// findHistoryFile returns the first readable history file path, or "".
func findHistoryFile() string {
	if p := strings.TrimSpace(os.Getenv("HISTFILE")); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{".bash_history", ".zsh_history", ".history"} {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// stripHistoryPrefix removes the leading metadata that zsh's extended history
// format prepends to each command (": <timestamp>:<elapsed>;command").
// Bash plain history has no prefix and is returned unchanged.
func stripHistoryPrefix(line string) string {
	if !strings.HasPrefix(line, ": ") {
		return line
	}
	semi := strings.IndexByte(line, ';')
	if semi < 0 {
		return line
	}
	return line[semi+1:]
}
