package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type pathCompletionCandidate struct {
	Display string
	Token   string
}

type pathCompletionResult struct {
	Value      string
	Cursor     int
	Candidates []pathCompletionCandidate
}

func completePathValue(value string, cursor int) (pathCompletionResult, bool) {
	runes := []rune(value)
	if cursor < 0 || cursor > len(runes) {
		cursor = len(runes)
	}

	start := pathTokenStart(runes, cursor)
	token := string(runes[start:cursor])
	info := parsePathToken(token)
	if result, ok := completeEnvPathToken(runes, start, cursor, info); ok {
		return result, true
	}
	entries, ok := pathCompletionEntries(info)
	if !ok || len(entries) == 0 {
		return pathCompletionResult{}, false
	}

	common := commonCandidatePrefix(entries)
	completedToken := info.format(info.root + common)
	newRunes := make([]rune, 0, len(runes)-cursor+start+len([]rune(completedToken)))
	newRunes = append(newRunes, runes[:start]...)
	newRunes = append(newRunes, []rune(completedToken)...)
	newCursor := len(newRunes)
	newRunes = append(newRunes, runes[cursor:]...)

	candidates := make([]pathCompletionCandidate, len(entries))
	for i, entry := range entries {
		candidates[i] = pathCompletionCandidate{
			Display: entry.name,
			Token:   info.format(info.root + entry.name),
		}
	}

	return pathCompletionResult{
		Value:      string(newRunes),
		Cursor:     newCursor,
		Candidates: candidates,
	}, true
}

func completeEnvPathToken(runes []rune, start, cursor int, info pathTokenInfo) (pathCompletionResult, bool) {
	if info.root != "" || !strings.HasPrefix(info.base, "$") {
		return pathCompletionResult{}, false
	}
	prefix := strings.TrimPrefix(info.base, "$")
	var matches []string
	for _, env := range os.Environ() {
		eq := strings.IndexByte(env, '=')
		if eq <= 0 {
			continue
		}
		name := env[:eq]
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		token := "$" + name
		if stat, err := os.Stat(env[eq+1:]); err == nil && stat.IsDir() {
			token += "/"
		}
		matches = append(matches, token)
	}
	if len(matches) == 0 {
		return pathCompletionResult{}, false
	}
	sort.Strings(matches)
	common := matches[0]
	for _, match := range matches[1:] {
		common = commonStringPrefix(common, match)
	}
	completedToken := formatEnvToken(info.quoted, common)
	newRunes := make([]rune, 0, len(runes)-cursor+start+len([]rune(completedToken)))
	newRunes = append(newRunes, runes[:start]...)
	newRunes = append(newRunes, []rune(completedToken)...)
	newCursor := len(newRunes)
	newRunes = append(newRunes, runes[cursor:]...)

	candidates := make([]pathCompletionCandidate, len(matches))
	for i, match := range matches {
		candidates[i] = pathCompletionCandidate{
			Display: match,
			Token:   formatEnvToken(info.quoted, match),
		}
	}
	return pathCompletionResult{Value: string(newRunes), Cursor: newCursor, Candidates: candidates}, true
}

func formatEnvToken(quote rune, token string) string {
	if quote != 0 {
		return string(quote) + token
	}
	return token
}

func pathTokenStart(runes []rune, cursor int) int {
	start := 0
	var quote rune
	escaped := false
	for i := 0; i < cursor; i++ {
		r := runes[i]
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			start = i + 1
		}
	}
	return start
}

type pathTokenInfo struct {
	root   string
	base   string
	dir    string
	quoted rune
}

func parsePathToken(token string) pathTokenInfo {
	info := pathTokenInfo{}
	if token != "" && (token[0] == '\'' || token[0] == '"') {
		info.quoted = rune(token[0])
		token = token[1:]
	}
	token = unescapePathToken(token)

	slash := strings.LastIndexByte(token, '/')
	if slash >= 0 {
		info.root = token[:slash+1]
		info.base = token[slash+1:]
	} else {
		info.base = token
	}
	info.dir = expandPathRoot(info.root)
	if info.dir == "" {
		info.dir = "."
	}
	return info
}

func (info pathTokenInfo) format(token string) string {
	if info.quoted != 0 {
		return string(info.quoted) + token
	}
	if strings.HasPrefix(token, "$") {
		return escapePathTokenNoDollar(token)
	}
	return escapePathToken(token)
}

func unescapePathToken(token string) string {
	var b strings.Builder
	b.Grow(len(token))
	escaped := false
	for _, r := range token {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func escapePathToken(token string) string {
	var b strings.Builder
	b.Grow(len(token))
	for _, r := range token {
		switch r {
		case ' ', '\t', '\'', '"', '\\', '(', ')', '[', ']', '{', '}', '$', '&', ';', '*', '?', '|', '<', '>', '#', '`':
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func escapePathTokenNoDollar(token string) string {
	return strings.ReplaceAll(escapePathToken(token), `\$`, `$`)
}

func expandPathRoot(root string) string {
	switch {
	case root == "":
		return "."
	case root == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			return root
		}
		return home
	case strings.HasPrefix(root, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return root
		}
		return filepath.Join(home, root[2:])
	default:
		return os.ExpandEnv(root)
	}
}

type pathEntry struct {
	name string
}

func pathCompletionEntries(info pathTokenInfo) ([]pathEntry, bool) {
	entries, err := os.ReadDir(info.dir)
	if err != nil {
		return nil, false
	}
	out := make([]pathEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, info.base) {
			continue
		}
		if entry.IsDir() {
			name += "/"
		}
		out = append(out, pathEntry{name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, true
}

func commonCandidatePrefix(entries []pathEntry) string {
	if len(entries) == 0 {
		return ""
	}
	prefix := entries[0].name
	for _, entry := range entries[1:] {
		prefix = commonStringPrefix(prefix, entry.name)
		if prefix == "" {
			break
		}
	}
	return prefix
}

func commonStringPrefix(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return a[:i]
}
