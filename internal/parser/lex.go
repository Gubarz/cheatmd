package parser

import (
	"bytes"
	"strings"
	"sync"
)

// ============================================================================
// String Interner
// ============================================================================

// stringInterner provides string deduplication (interning).
type stringInterner struct {
	mu      sync.RWMutex
	strings map[string]string
}

func newStringInterner() *stringInterner {
	return &stringInterner{
		strings: make(map[string]string, 1024),
	}
}

// Intern returns a canonical version of the string.
// If the string was seen before, returns the previously stored instance.
func (si *stringInterner) Intern(s string) string {
	if s == "" {
		return ""
	}
	si.mu.RLock()
	if interned, ok := si.strings[s]; ok {
		si.mu.RUnlock()
		return interned
	}
	si.mu.RUnlock()

	si.mu.Lock()
	if interned, ok := si.strings[s]; ok {
		si.mu.Unlock()
		return interned
	}
	si.strings[s] = s
	si.mu.Unlock()
	return s
}

// InternBytes returns a canonical version of the string from a byte slice.
// Uses unsafe string conversion for map lookup to avoid allocation in Go 1.22+.
func (si *stringInterner) InternBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	si.mu.RLock()
	if interned, ok := si.strings[string(b)]; ok {
		si.mu.RUnlock()
		return interned
	}
	si.mu.RUnlock()

	si.mu.Lock()
	s := string(b)
	if interned, ok := si.strings[s]; ok {
		si.mu.Unlock()
		return interned
	}
	si.strings[s] = s
	si.mu.Unlock()
	return s
}

// pathInterner is the global interner for file paths and common strings.
var pathInterner = newStringInterner()

// ============================================================================
// Language Classification
// ============================================================================

// IsShellLanguage reports whether the code-fence language is a shell language
// (one where $var injection is safe).
func IsShellLanguage(lang string) bool {
	lang = strings.ToLower(lang)
	if lang == "mermaid" || lang == "dot" || lang == "chart" {
		return false
	}
	return true
}

// ============================================================================
// Byte-Level Line Lexing
// ============================================================================

// parseHeader extracts header text without regex: "## Header" -> "Header".
func parseHeader(line []byte) (string, bool) {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return "", false
	}
	if i >= len(line) || line[i] != ' ' {
		return "", false
	}
	i++
	if i >= len(line) {
		return "", false
	}
	return string(line[i:]), true
}

// parseCodeBlockStart parses ```lang title:"desc" without regex.
func parseCodeBlockStart(line []byte) (lang, desc string, ok bool) {
	if len(line) < 3 || line[0] != '`' || line[1] != '`' || line[2] != '`' {
		return "", "", false
	}
	rest := line[3:]

	// Extract language
	langEnd := 0
	for langEnd < len(rest) && isWordChar(rest[langEnd]) {
		langEnd++
	}
	lang = string(rest[:langEnd])

	// Check for title
	titleIdx := bytes.Index(rest[langEnd:], []byte("title:\""))
	if titleIdx == -1 {
		return lang, "", true
	}

	// Extract title
	start := langEnd + titleIdx + 7 // length of `title:"`
	end := bytes.IndexByte(rest[start:], '"')
	if end == -1 {
		return lang, "", true
	}

	return lang, string(rest[start : start+end]), true
}

// parseCheatSingleLine parses <!-- cheat ... --> and returns the content.
func parseCheatSingleLine(line []byte) (string, bool) {
	if len(line) < len("<!--cheat-->") {
		return "", false
	}
	if !bytes.HasPrefix(line, []byte("<!--")) {
		return "", false
	}
	if !bytes.HasSuffix(line, []byte("-->")) {
		return "", false
	}

	inner := bytes.TrimSpace(line[4 : len(line)-3])
	if len(inner) < 5 {
		return "", false
	}
	if !bytes.EqualFold(inner[:5], []byte("cheat")) {
		return "", false
	}
	if len(inner) > 5 && inner[5] != ' ' && inner[5] != '\t' {
		return "", false
	}

	return string(bytes.TrimSpace(inner[5:])), true
}

// isCheatStart checks for "<!-- cheat" (multiline cheat block start).
func isCheatStart(line []byte) bool {
	if len(line) < 10 {
		return false
	}
	if !bytes.HasPrefix(line, []byte("<!--")) {
		return false
	}
	inner := bytes.TrimSpace(line[4:])
	return bytes.EqualFold(inner, []byte("cheat"))
}

// isCheatEnd checks for "-->" (multiline cheat block end).
func isCheatEnd(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return bytes.Equal(trimmed, []byte("-->"))
}

// isWordChar returns true for [a-zA-Z0-9_].
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// trimSpaceBytes trims leading/trailing whitespace from bytes without allocating.
func trimSpaceBytes(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t' || b[start] == '\n' || b[start] == '\r') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\n' || b[end-1] == '\r') {
		end--
	}
	return b[start:end]
}

// ============================================================================
// File-System Helpers
// ============================================================================

// isMarkdownFile reports whether path has a markdown extension.
func isMarkdownFile(path string) bool {
	if len(path) < 3 {
		return false
	}
	ext := path[len(path)-3:]
	return ext == ".md" || ext == ".MD" || strings.EqualFold(path[len(path)-3:], ".md")
}
