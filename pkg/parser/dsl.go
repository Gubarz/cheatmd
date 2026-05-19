package parser

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCheatDSL parses the DSL content within a cheat block.
//
// Hand-rolled dispatch on the first keyword (var / if / fi / export / import / chain)
// avoids per-line regex matching. Each non-comment, non-blank line is matched
// against at most one branch.
func parseCheatDSL(cheat *Cheat, content string, path string, startLine int) []ParseError {
	lines := joinContinuationLines(strings.Split(content, "\n"))

	var currentCondition string
	var errs []ParseError
	ifDepth := 0
	ifLines := []int{}

	for i, line := range lines {
		lineNo := startLine + i
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}

		keyword, rest := splitFirstWord(line)
		switch keyword {
		case "fi":
			if rest != "" {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: fmt.Sprintf("`fi` takes no arguments, got %q", rest)})
			}
			if ifDepth == 0 {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: "`fi` without a matching `if`"})
			} else {
				ifDepth--
				ifLines = ifLines[:len(ifLines)-1]
			}
			currentCondition = ""
		case "if":
			if rest == "" {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: "`if` requires a condition"})
			} else {
				currentCondition = rest
			}
			ifDepth++
			ifLines = append(ifLines, lineNo)
		case "export":
			if rest == "" {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: "`export` requires a name"})
			} else if containsWhitespace(rest) {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: "`export` name must be a single token"})
			} else {
				cheat.Export = rest
			}
		case "import":
			if rest == "" {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: "`import` requires a name"})
			} else if containsWhitespace(rest) {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: "`import` name must be a single token"})
			} else {
				cheat.Imports = append(cheat.Imports, rest)
			}
		case "chain":
			if err := parseChainLine(cheat, rest); err != "" {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: err})
			}
		case "var":
			if err := parseVarLine(cheat, rest, currentCondition); err != "" {
				errs = append(errs, ParseError{File: path, Line: lineNo, Message: err})
			}
		default:
			errs = append(errs, ParseError{File: path, Line: lineNo, Message: fmt.Sprintf("unknown DSL keyword %q (expected one of: var, if, fi, export, import, chain)", keyword)})
		}
	}
	for _, ln := range ifLines {
		errs = append(errs, ParseError{File: path, Line: ln, Message: "`if` without a matching `fi`"})
	}
	return errs
}

func parseChainLine(cheat *Cheat, rest string) string {
	name, after := splitFirstWord(rest)
	stepText, extra := splitFirstWord(after)
	if name == "" || stepText == "" || extra != "" {
		return "`chain` requires exactly a name and positive step number"
	}
	if containsWhitespace(name) {
		return "`chain` name must be a single token"
	}
	step, err := strconv.Atoi(stepText)
	if err != nil || step < 1 {
		return "`chain` step must be a positive number"
	}
	cheat.ChainName = name
	cheat.ChainStep = step
	return ""
}

// parseVarLine handles the three var declaration forms:
//
//	var NAME           -> prompt-only
//	var NAME --- args  -> prompt-only with selector/prompt args
//	var NAME := value  -> literal
//	var NAME = value   -> shell
func parseVarLine(cheat *Cheat, rest, condition string) string {
	name, after := splitFirstWord(rest)
	if name == "" {
		return "`var` requires a name"
	}
	if !isValidDSLVarName(name) {
		return fmt.Sprintf("invalid var name %q", name)
	}

	if after == "" {
		cheat.Vars = append(cheat.Vars, VarDef{
			Name:      name,
			Condition: condition,
		})
		return ""
	}

	switch {
	case strings.HasPrefix(after, "---"):
		cheat.Vars = append(cheat.Vars, VarDef{
			Name:      name,
			Args:      strings.TrimSpace(after[3:]),
			Condition: condition,
		})
	case strings.HasPrefix(after, ":="):
		value := strings.TrimSpace(after[2:])
		if value == "" {
			return fmt.Sprintf("`var %s :=` has no value", name)
		}
		cheat.Vars = append(cheat.Vars, ParseVarDefWithCondition(name, value, condition, true))
	case after[0] == '=':
		value := strings.TrimSpace(after[1:])
		if value == "" {
			return fmt.Sprintf("`var %s =` has no shell command", name)
		}
		cheat.Vars = append(cheat.Vars, ParseVarDefWithCondition(name, value, condition, false))
	default:
		return fmt.Sprintf("`var %s ...` is missing an assignment operator (use `=` for shell or `:=` for literal)", name)
	}
	return ""
}

// splitFirstWord returns the leading whitespace-delimited token and the
// remainder with leading whitespace trimmed. If the input has no token, the
// keyword is "".
func splitFirstWord(s string) (keyword, rest string) {
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	if i == 0 {
		return "", ""
	}
	keyword = s[:i]
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	rest = s[i:]
	return
}

// isValidDSLVarName reports whether s is a valid var name in the DSL:
// letters, digits, and underscores (matching the `\w+` regex that the
// previous regex-based implementation used).
func isValidDSLVarName(s string) bool {
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

// containsWhitespace reports whether s has any space or tab.
func containsWhitespace(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			return true
		}
	}
	return false
}

// joinContinuationLines joins lines that end with backslash.
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
