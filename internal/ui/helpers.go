package ui

import "strings"

// clamp restricts v to the range [minV, maxV].
func clamp(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// safeTextInputWidth clamps text input width to a positive value.
// Terminal APIs can briefly report very small/zero sizes in edge cases.
func safeTextInputWidth(totalWidth int) int {
	return max(totalWidth-4, 1)
}

// countLines counts the number of lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// scrollWindow calculates the visible range for a scrollable list, adjusting
// offset so the cursor stays visible.
func scrollWindow(cursor, total, height int, offset *int) (start, end int) {
	if cursor < *offset {
		*offset = cursor
	}
	if cursor >= *offset+height {
		*offset = cursor - height + 1
	}
	maxOffset := max(0, total-height)
	*offset = clamp(*offset, 0, maxOffset)

	start = *offset
	end = min(start+height, total)
	return
}

// truncateString truncates a string to maxLen with ellipsis.
func truncateString(s string, maxLen int) string {
	if maxLen <= 3 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// truncateLines truncates text to maxLines with optional maxLen per content.
func truncateLines(text string, maxLines int, maxLen int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		text = strings.Join(lines[:maxLines], "\n") + "..."
	}
	if maxLen > 0 && len(text) > maxLen {
		text = text[:maxLen-3] + "..."
	}
	return text
}

// matchesAllWords returns true if text contains all words.
func matchesAllWords(text string, words []string) bool {
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

// formatKeyDisplay turns "ctrl+x" into "Ctrl+X" for display purposes.
func formatKeyDisplay(key string) string {
	if strings.HasPrefix(key, "ctrl+") {
		return "Ctrl+" + strings.ToUpper(key[5:])
	}
	return key
}

// firstLine returns the substring up to the first newline, or s if none.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// renderWindowLayout composes a vertical layout with padding inserted before the bottom section.
func renderWindowLayout(height int, top, middle, bottom string) string {
	topLines := countLines(top)
	middleLines := countLines(middle)
	bottomLines := countLines(bottom)
	padding := max(height-topLines-middleLines-bottomLines, 0)

	var b strings.Builder
	b.WriteString(top)
	b.WriteString(middle)
	b.WriteString(strings.Repeat("\n", padding))
	b.WriteString(bottom)
	return b.String()
}
