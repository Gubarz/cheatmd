package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"

	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/parser"
)

// previewOverlayState holds the state for the markdown preview overlay.
// The user enters via the configured key during phaseCheatSelect or
// phaseVarResolve and returns to the previous phase on Esc.
type previewOverlayState struct {
	viewport  viewport.Model
	cheat     *parser.Cheat // cheat whose file is being shown
	prevPhase uiPhase       // phase to restore on exit
	errorMsg  string        // non-empty if rendering or reading failed
}

type previewPendingCodeBlock struct {
	command    string
	headerLine int
}

// enterPreview transitions to phasePreview with the markdown rendering of the
// given cheat's source file. Returns true on success, false if no cheat is
// available (caller should remain in current phase).
func (m *mainModel) enterPreview(c *parser.Cheat) bool {
	if c == nil || c.File == "" {
		return false
	}

	// Read the entire source file.
	data, err := os.ReadFile(c.File)
	if err != nil {
		m.previewState = &previewOverlayState{
			cheat:     c,
			prevPhase: m.phase,
			errorMsg:  fmt.Sprintf("Could not read %s: %v", c.File, err),
		}
		m.previewState.viewport = newPreviewViewport(m.width, m.height)
		m.previewState.viewport.SetContent(m.previewState.errorMsg)
		m.phase = phasePreview
		return true
	}

	vp := newPreviewViewport(m.width, m.height)
	rendered, err := renderMarkdown(string(data), vp.Width)
	if err != nil {
		// Fall back to the raw source on render failure.
		rendered = string(data)
	}
	vp.SetContent(rendered)

	// Scroll so the cheat's header is near the top of the viewport.
	if line := findCheatHeaderSourceLine(string(data), c); line >= 0 {
		if offset, ok := renderedOffsetForSourceLine(string(data), line, vp.Width); ok {
			vp.SetYOffset(offset)
		}
	}

	m.previewState = &previewOverlayState{
		viewport:  vp,
		cheat:     c,
		prevPhase: m.phase,
	}
	m.phase = phasePreview
	return true
}

// exitPreview returns to whichever phase was active when preview was entered.
func (m *mainModel) exitPreview() {
	if m.previewState == nil {
		m.phase = phaseCheatSelect
		return
	}
	m.phase = m.previewState.prevPhase
	m.previewState = nil
}

// updatePreview handles updates while the preview overlay is open.
func (m *mainModel) updatePreview(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if m.previewState != nil {
			m.previewState.viewport.Width = max(msg.Width, 1)
			m.previewState.viewport.Height = max(msg.Height-1, 1) // 1 row for hint
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			m.selected = nil
			return m, tea.Quit
		case "esc", "q":
			m.exitPreview()
			return m, tea.ClearScreen
		}
	}

	if m.previewState != nil {
		var cmd tea.Cmd
		m.previewState.viewport, cmd = m.previewState.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// renderPreview renders the preview overlay.
func (m *mainModel) renderPreview() string {
	if m.previewState == nil {
		return ""
	}
	b := getBuilder()
	defer putBuilder(b)

	b.WriteString(m.previewState.viewport.View())
	b.WriteString("\n")
	b.WriteString(styles.Dim.Render("  ESC close"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("↑/↓ scroll"))
	b.WriteString(" • ")
	b.WriteString(styles.Dim.Render("PgUp/PgDn page"))
	return b.String()
}

// newPreviewViewport creates a viewport sized to the terminal, reserving one
// row at the bottom for the hint line.
func newPreviewViewport(width, height int) viewport.Model {
	w := max(width, 40)
	h := max(height-1, 5)
	vp := viewport.New(w, h)
	return vp
}

// renderMarkdown returns the glamour-rendered markdown for raw at the given
// terminal width. Uses a custom style configured from cheatmd's color palette
// so the preview matches the rest of the TUI.
func renderMarkdown(raw string, width int) (string, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(cheatmdGlamourStyle()),
		glamour.WithWordWrap(max(width-4, 40)),
	)
	if err != nil {
		return "", err
	}
	return r.Render(raw)
}

// cheatmdGlamourStyle returns an ansi.StyleConfig that maps glamour's style
// slots to cheatmd's configured color palette (color_header, color_command,
// color_path, color_border, color_desc, color_dim). Called once per preview
// open so live config edits take effect on the next preview.
func cheatmdGlamourStyle() ansi.StyleConfig {
	header := config.GetColorHeader()
	command := config.GetColorCommand()
	desc := config.GetColorDesc()
	path := config.GetColorPath()
	border := config.GetColorBorder()
	dim := config.GetColorDim()

	str := func(s string) *string { return &s }
	b := func(v bool) *bool { return &v }
	u := func(v uint) *uint { return &v }

	margin := uint(2)
	listIndent := uint(2)

	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "\n",
				BlockSuffix: "\n",
				Color:       str(desc),
			},
			Margin: u(margin),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: str(dim)},
			Indent:         u(1),
			IndentToken:    str("│ "),
		},
		List: ansi.StyleList{
			LevelIndent: listIndent,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockSuffix: "\n",
				Color:       str(header),
				Bold:        b(true),
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: " ",
				Suffix: " ",
				Color:  str(header),
				Bold:   b(true),
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "## ",
				Color:  str(header),
				Bold:   b(true),
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
				Color:  str(header),
				Bold:   b(true),
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
				Color:  str(header),
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
				Color:  str(header),
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
				Color:  str(dim),
			},
		},
		Strikethrough: ansi.StylePrimitive{CrossedOut: b(true)},
		Emph:          ansi.StylePrimitive{Italic: b(true), Color: str(desc)},
		Strong:        ansi.StylePrimitive{Bold: b(true), Color: str(desc)},
		HorizontalRule: ansi.StylePrimitive{
			Color:  str(border),
			Format: "\n────────\n",
		},
		Item:        ansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration: ansi.StylePrimitive{BlockPrefix: ". "},
		Task: ansi.StyleTask{
			StylePrimitive: ansi.StylePrimitive{},
			Ticked:         "[✓] ",
			Unticked:       "[ ] ",
		},
		Link: ansi.StylePrimitive{
			Color:     str(path),
			Underline: b(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: str(path),
			Bold:  b(true),
		},
		Image: ansi.StylePrimitive{
			Color:     str(path),
			Underline: b(true),
		},
		ImageText: ansi.StylePrimitive{
			Color:  str(dim),
			Format: "Image: {{.text}} →",
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: " ",
				Suffix: " ",
				Color:  str(command),
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: str(command)},
				Margin:         u(margin),
			},
			// Letting Chroma stay nil makes glamour render code blocks as
			// flat-colored text in our command color, which keeps the
			// preview visually consistent with the rest of the TUI.
		},
		Table: ansi.StyleTable{
			StyleBlock: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{}},
		},
		DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n→ "},
	}
}

// findCheatHeaderSourceLine returns the source line of the markdown heading
// attached to cheat. This avoids snapping to an earlier page title with the
// same text after glamour has rendered the whole document.
func findCheatHeaderSourceLine(raw string, cheat *parser.Cheat) int {
	if cheat == nil {
		return -1
	}

	var (
		currentHeaderLine = -1
		inCodeFence       bool
		inCheatBlock      bool
		codeLines         []string
		pending           []previewPendingCodeBlock
	)

	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inCodeFence {
			if strings.HasPrefix(trimmed, "```") {
				inCodeFence = false
				command := strings.TrimSpace(strings.Join(codeLines, "\n"))
				if command != "" {
					pending = append(pending, previewPendingCodeBlock{
						command:    command,
						headerLine: currentHeaderLine,
					})
				}
				codeLines = nil
				continue
			}
			codeLines = append(codeLines, line)
			continue
		}

		if inCheatBlock {
			if trimmed == "-->" {
				inCheatBlock = false
				if len(pending) > 0 {
					last := pending[len(pending)-1]
					if last.command == strings.TrimSpace(cheat.Command) {
						return last.headerLine
					}
					pending = pending[:len(pending)-1]
				}
			}
			continue
		}

		if header, _, ok := parser.ParseMarkdownHeader(trimmed); ok {
			if currentHeaderLine >= 0 {
				if line := findPendingCommandHeaderLine(pending, cheat); line >= 0 {
					return line
				}
				pending = nil
			}
			if header == strings.TrimSpace(cheat.Header) {
				currentHeaderLine = i
			} else {
				currentHeaderLine = -1
			}
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			inCodeFence = true
			codeLines = nil
			continue
		}

		if content, ok := parser.ParseCheatSingleLine(trimmed); ok {
			_ = content
			if len(pending) > 0 {
				last := pending[len(pending)-1]
				if last.command == strings.TrimSpace(cheat.Command) {
					return last.headerLine
				}
				pending = pending[:len(pending)-1]
			}
			continue
		}

		if parser.IsCheatStart(trimmed) {
			inCheatBlock = true
			continue
		}
	}

	return findPendingCommandHeaderLine(pending, cheat)
}

func findPendingCommandHeaderLine(pending []previewPendingCodeBlock, cheat *parser.Cheat) int {
	target := strings.TrimSpace(cheat.Command)
	for _, block := range pending {
		if block.command == target {
			return block.headerLine
		}
	}
	return -1
}

func renderedOffsetForSourceLine(raw string, sourceLine, width int) (int, bool) {
	if sourceLine < 0 {
		return 0, false
	}
	lines := strings.Split(raw, "\n")
	if sourceLine > len(lines) {
		return 0, false
	}
	prefix := strings.Join(lines[:sourceLine], "\n")
	if strings.TrimSpace(prefix) == "" {
		return 0, true
	}
	rendered, err := renderMarkdown(prefix, width)
	if err != nil {
		return 0, false
	}
	offset := strings.Count(rendered, "\n")
	if offset > 0 {
		offset--
	}
	return offset, true
}
