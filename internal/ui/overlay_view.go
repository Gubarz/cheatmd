package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
)

// OverlayConfig provides the dynamic parts for renderOverlayWindow.
type OverlayConfig struct {
	Title         string
	TitleExtra    string // E.g. "(12 entries)" or "-> $var"
	MatchesCount  int
	EnterHint     string // E.g. "Enter use value" or "Enter re-run cheat"
	Items         []string
	SelectedIndex int
	Offset        *int // Pointer to the state's offset for scrolling
	Input         textinput.Model
}

// renderOverlayWindow provides a generic layout for the history and substitute search overlays.
// It handles the fixed-height header, the scrolling list, and the fixed-height input footer.
func (m *mainModel) renderOverlayWindow(cfg OverlayConfig) string {
	width := max(m.width, 80)
	height := m.height
	if height < 1 {
		height = 24
	}

	inputLines := 3 // divider + info + input
	previewHeight := 2

	preview := renderOverlayPreview(width, previewHeight, cfg.Title, cfg.TitleExtra)
	previewLines := countLines(preview)

	listHeight := max(height-previewLines-inputLines, 1)
	list := renderOverlayList(listHeight, width, cfg.Items, cfg.SelectedIndex, cfg.Offset)

	input := renderOverlayInput(width, cfg.MatchesCount, cfg.EnterHint, cfg.Input)

	return renderWindowLayout(height, preview, list, input)
}

func renderOverlayPreview(width, maxLines int, title, extra string) string {
	b := getBuilder()
	defer putBuilder(b)
	lines := 0

	if lines < maxLines {
		b.WriteString(styles.Header.Render(title))
		if extra != "" {
			b.WriteString("  ")
			b.WriteString(extra)
		}
		b.WriteString("\n")
		lines++
	}

	for lines < maxLines {
		b.WriteString("\n")
		lines++
	}

	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	return b.String()
}

func renderOverlayList(maxHeight, width int, items []string, selectedIdx int, offset *int) string {
	if len(items) == 0 {
		return ""
	}

	start, end := scrollWindow(selectedIdx, len(items), maxHeight, offset)
	maxLen := max(width-2, 10)

	b := getBuilder()
	defer putBuilder(b)
	for i := start; i < end; i++ {
		display := truncateString(items[i], maxLen)
		if i == selectedIdx {
			b.WriteString(styles.Cursor.Render("▶ "))
			b.WriteString(styles.Selected.Render(styles.Command.Render(display)))
		} else {
			b.WriteString("  ")
			b.WriteString(styles.Command.Render(display))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderOverlayInput(width, matchCount int, enterHint string, input textinput.Model) string {
	b := getBuilder()
	defer putBuilder(b)
	b.WriteString(styles.Divider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(styles.Dim.Render(fmt.Sprintf("  %d matches", matchCount)))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("ESC cancel"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render(enterHint))
	b.WriteString("\n")
	b.WriteString(input.View())
	return b.String()
}
