package parser

import (
	"strings"
)

// ParseMarkdownHeader parses a line to see if it is a markdown header.
// It returns the header text, the level (number of #s), and a boolean
// indicating if it is a valid header.
func ParseMarkdownHeader(trimmed string) (header string, level int, ok bool) {
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

// ParseCheatSingleLine extracts the content of a single-line HTML comment if it starts with "cheat".
func ParseCheatSingleLine(trimmed string) (string, bool) {
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

// IsCheatStart checks if the line is the start of a multi-line cheat comment block.
func IsCheatStart(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "<!--") {
		return false
	}
	inner := strings.TrimSpace(strings.TrimPrefix(trimmed, "<!--"))
	return strings.EqualFold(inner, "cheat")
}

// IsCheatEnd checks if the line closes an HTML comment block.
func IsCheatEnd(trimmed string) bool {
	return trimmed == "-->"
}

// SplitFirstWord splits a string into the first word and the rest of the string.
func SplitFirstWord(s string) (head, rest string) {
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

// IsVarChar returns true if c is valid in a variable name.
func IsVarChar(c byte, first bool) bool {
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
		return true
	}
	return !first && c >= '0' && c <= '9'
}

// SplitLines splits text into non-empty trimmed lines.
func SplitLines(s string) []string {
	if s == "" {
		return nil
	}

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
