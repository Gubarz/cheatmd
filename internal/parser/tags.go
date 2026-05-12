package parser

import (
	"bytes"
	"path/filepath"
	"strings"
)

// ============================================================================
// Tag Assembly
// ============================================================================

// buildCheatTags merges all tag sources for one cheat: path tags, heading-suffix
// tag, file-level tags (front matter + footer), and per-cheat inline #tags.
// Result is lowercased and deduplicated.
//
// Fast path: when there are no extra tag sources we return the cached path
// tags directly (zero allocation). Otherwise we dedupe with a linear scan
// over the accumulator; tag counts per cheat are tiny, so linear beats a
// map and avoids closure allocation.
func (p *Parser) buildCheatTags(path string, s *parseState) []string {
	pathTags := p.getTagsForPath(path, s.currentHeader)

	if len(s.fileTags) == 0 && len(s.currentHeaderTags) == 0 {
		return pathTags
	}

	extra := len(s.fileTags) + len(s.currentHeaderTags)
	out := make([]string, len(pathTags), len(pathTags)+extra)
	copy(out, pathTags)

	out = appendUniqueTags(out, s.fileTags)
	out = appendUniqueTags(out, s.currentHeaderTags)
	return out
}

// appendUniqueTags appends every entry in src to dst, lowercased and trimmed,
// skipping duplicates already in dst. Linear scan; suitable for small tag sets.
func appendUniqueTags(dst []string, src []string) []string {
outer:
	for _, t := range src {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		for _, existing := range dst {
			if existing == t {
				continue outer
			}
		}
		dst = append(dst, t)
	}
	return dst
}

// getTagsForPath returns cached path tags plus header tag.
func (p *Parser) getTagsForPath(path, header string) []string {
	dir := filepath.Dir(path)
	pathTags, ok := p.pathTagsCache[dir]
	if !ok {
		for _, part := range strings.Split(dir, string(filepath.Separator)) {
			if part != "" && part != "." {
				pathTags = append(pathTags, strings.ToLower(part))
			}
		}
		p.pathTagsCache[dir] = pathTags
	}

	// Add header tag if present (header text before a ":" prefix).
	if idx := strings.IndexByte(header, ':'); idx != -1 {
		tags := make([]string, len(pathTags), len(pathTags)+1)
		copy(tags, pathTags)
		return append(tags, strings.ToLower(strings.TrimSpace(header[:idx])))
	}

	return pathTags
}

// mergeTags appends newTags to existing, lowercasing and deduping in place.
func mergeTags(existing []string, newTags []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(newTags))
	for _, t := range existing {
		seen[t] = struct{}{}
	}
	for _, t := range newTags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		existing = append(existing, t)
	}
	return existing
}

// ============================================================================
// Tag Extraction
// ============================================================================

// extractFrontMatterTags strips a leading YAML front-matter block (--- ... ---)
// from data and returns the remainder plus any tags found.
func extractFrontMatterTags(data []byte) ([]byte, []string) {
	i := 0
	for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\r') {
		i++
	}
	if i+3 > len(data) || data[i] != '-' || data[i+1] != '-' || data[i+2] != '-' {
		return data, nil
	}
	j := i + 3
	for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\r') {
		j++
	}
	if j >= len(data) || data[j] != '\n' {
		return data, nil
	}
	bodyStart := j + 1

	closeStart, closeEnd, ok := findYAMLClose(data, bodyStart)
	if !ok {
		return data, nil
	}

	tags := parseYAMLTags(data[bodyStart:closeStart])
	return data[closeEnd:], tags
}

// extractFooterTags strips a trailing tag block from data and returns the
// remainder plus any tags found. Two shapes are recognized:
//
//  1. YAML footer:  --- \n tags: [...] \n ---
//  2. Hashtag footer: optional --- rule, then one or more lines of
//     whitespace-separated #tag tokens
//
// Walks backward from end-of-file; stops at the first non-tag, non-rule,
// non-blank line.
func extractFooterTags(data []byte) ([]byte, []string) {
	end := len(data)
	for end > 0 && (data[end-1] == '\n' || data[end-1] == '\r' || data[end-1] == ' ' || data[end-1] == '\t') {
		end--
	}
	if end == 0 {
		return data, nil
	}

	if body, tags, ok := extractYAMLFooter(data, end); ok {
		return body, tags
	}
	if body, tags, ok := extractHashtagFooter(data, end); ok {
		return body, tags
	}
	return data, nil
}

// extractYAMLFooter recognizes a trailing --- ... --- YAML block.
func extractYAMLFooter(data []byte, end int) ([]byte, []string, bool) {
	if end < 3 {
		return nil, nil, false
	}
	if data[end-1] != '-' || data[end-2] != '-' || data[end-3] != '-' {
		return nil, nil, false
	}
	if end-3 > 0 && data[end-4] != '\n' {
		return nil, nil, false
	}

	openEnd := end - 3
	openStart := openEnd
	for openStart > 0 {
		lineEnd := openStart
		lineStart := lineEnd
		for lineStart > 0 && data[lineStart-1] != '\n' {
			lineStart--
		}
		line := bytes.TrimRight(data[lineStart:lineEnd], " \t\r")
		if bytes.Equal(line, []byte("---")) && lineStart != openEnd-3 {
			tags := parseYAMLTags(data[lineEnd+1 : openEnd])
			return data[:lineStart], tags, true
		}
		if lineStart == 0 {
			break
		}
		openStart = lineStart - 1
	}
	return nil, nil, false
}

// extractHashtagFooter recognizes a trailing block of #tag lines, optionally
// preceded by a "---" horizontal rule.
func extractHashtagFooter(data []byte, end int) ([]byte, []string, bool) {
	var tags []string
	cut := end
	sawTagLine := false

	for cut > 0 {
		lineEnd := cut
		lineStart := lineEnd
		for lineStart > 0 && data[lineStart-1] != '\n' {
			lineStart--
		}
		line := bytes.TrimSpace(data[lineStart:lineEnd])

		if len(line) == 0 {
			if lineStart == 0 {
				break
			}
			cut = lineStart - 1
			continue
		}

		if lineTags, ok := parseHashtagLine(line); ok {
			tags = append(lineTags, tags...)
			sawTagLine = true
			if lineStart == 0 {
				cut = 0
				break
			}
			cut = lineStart - 1
			continue
		}

		if bytes.Equal(line, []byte("---")) && sawTagLine {
			cut = lineStart
			break
		}
		break
	}

	if !sawTagLine {
		return nil, nil, false
	}

	body := data[:cut]
	bodyEnd := len(body)
	for bodyEnd > 0 && (body[bodyEnd-1] == '\n' || body[bodyEnd-1] == '\r' || body[bodyEnd-1] == ' ' || body[bodyEnd-1] == '\t') {
		bodyEnd--
	}
	return body[:bodyEnd], tags, true
}

// parseHashtagLine returns the tags on a line if the line consists *only* of
// whitespace and #tag tokens. Returns false otherwise.
func parseHashtagLine(line []byte) ([]string, bool) {
	var tags []string
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		if line[i] != '#' {
			return nil, false
		}
		j := i + 1
		if j >= len(line) {
			return nil, false
		}
		c := line[j]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return nil, false
		}
		k := consumeTagBody(line, j+1)
		tags = append(tags, pathInterner.InternBytes(line[j:k]))
		i = k
	}
	if len(tags) == 0 {
		return nil, false
	}
	return tags, true
}

// findYAMLClose locates a "---" line at start-of-line at or after pos.
// Returns the byte offset where "---" begins and the offset just past its newline.
func findYAMLClose(data []byte, pos int) (closeStart, closeEnd int, ok bool) {
	lineStart := pos
	for lineStart < len(data) {
		lineEnd := lineStart
		for lineEnd < len(data) && data[lineEnd] != '\n' {
			lineEnd++
		}
		line := bytes.TrimRight(data[lineStart:lineEnd], " \t\r")
		if bytes.Equal(line, []byte("---")) {
			next := lineEnd
			if next < len(data) && data[next] == '\n' {
				next++
			}
			return lineStart, next, true
		}
		if lineEnd >= len(data) {
			return 0, 0, false
		}
		lineStart = lineEnd + 1
	}
	return 0, 0, false
}

// parseYAMLTags reads tags from a YAML block. Supports:
//
//	tags: [a, b, c]
//	tags: a, b, c
//	tags:
//	  - a
//	  - b
func parseYAMLTags(data []byte) []string {
	var tags []string
	lines := bytes.Split(data, []byte("\n"))
	inList := false
	for _, raw := range lines {
		line := string(bytes.TrimRight(raw, " \t\r"))
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if inList {
			if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
				item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
				item = strings.Trim(item, "\"'")
				if item != "" {
					tags = append(tags, item)
				}
				continue
			}
			inList = false
		}

		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "tags:") {
			continue
		}
		rest := strings.TrimSpace(trimmed[len("tags:"):])

		if rest == "" {
			inList = true
			continue
		}

		rest = strings.TrimPrefix(rest, "[")
		rest = strings.TrimSuffix(rest, "]")
		for _, part := range strings.Split(rest, ",") {
			item := strings.Trim(strings.TrimSpace(part), "\"'")
			if item != "" {
				tags = append(tags, item)
			}
		}
	}
	return tags
}

// scanInlineTags appends inline #tag tokens found in a prose line to dst.
// A tag is "#" followed by an ASCII letter, then word chars and -./.
// Excludes hex colors and numeric-only tokens. Stops at heading lines.
func scanInlineTags(line []byte, dst *[]string) {
	if len(line) == 0 {
		return
	}
	// Skip heading lines (start with # space).
	if line[0] == '#' {
		i := 0
		for i < len(line) && line[i] == '#' {
			i++
		}
		if i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			return
		}
	}

	for i := 0; i < len(line); i++ {
		if line[i] != '#' {
			continue
		}
		// Must be at start of token (preceded by space/start/punct).
		if i > 0 {
			prev := line[i-1]
			if prev != ' ' && prev != '\t' && prev != '(' && prev != '[' && prev != ',' {
				continue
			}
		}
		// Next char must be ASCII letter (rules out hex colors, #1, #@, etc.).
		j := i + 1
		if j >= len(line) {
			return
		}
		c := line[j]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			continue
		}
		// Consume tag body: word chars plus - . /
		k := consumeTagBody(line, j+1)
		*dst = append(*dst, pathInterner.InternBytes(line[j:k]))
		i = k - 1
	}
}

// consumeTagBody advances the index past any valid hashtag body characters
func consumeTagBody(line []byte, start int) int {
	k := start
	for k < len(line) {
		b := line[k]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') ||
			b == '_' || b == '-' || b == '.' || b == '/' {
			k++
			continue
		}
		break
	}
	return k
}
