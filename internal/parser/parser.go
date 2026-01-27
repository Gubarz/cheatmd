package parser

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Cheat represents a single executable cheat entry
type Cheat struct {
	File          string            // Source file path
	Header        string            // Section header
	Description   string            // Description from blockquotes
	Command       string            // Shell command template
	Tags          []string          // Tags from path/header
	Export        string            // Module name if exported
	Imports       []string          // Imported modules
	Vars          []VarDef          // Variable definitions
	Scope         map[string]string // Resolved values at runtime
	HasCheatBlock bool              // Whether this cheat has a <!-- cheat --> block
}

// VarDef represents a variable definition
type VarDef struct {
	Name    string // Variable name
	Shell   string // Shell command to generate values
	FzfArgs string // fzf arguments after ---
}

// Module represents an exportable module
type Module struct {
	Name    string
	Vars    []VarDef
	Imports []string // Nested imports this module depends on
	File    string
	Cheats  []*Cheat
}

// CheatIndex holds all parsed cheats and modules
type CheatIndex struct {
	Cheats  []*Cheat
	Modules map[string]*Module
}

// NewCheatIndex creates an empty cheat index
func NewCheatIndex() *CheatIndex {
	return &CheatIndex{
		Cheats:  make([]*Cheat, 0),
		Modules: make(map[string]*Module),
	}
}

var (
	headerRegex    = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	codeBlockStart = regexp.MustCompile("^```(\\w*)$")
	codeBlockEnd   = regexp.MustCompile("^```$")
	blockquoteRe   = regexp.MustCompile(`^>\s?(.*)$`)
	// New: <!-- cheat ... --> in one or multiple lines
	cheatCommentStart = regexp.MustCompile(`(?i)^<!--\s*cheat\s*$`)
	cheatCommentEnd   = regexp.MustCompile(`(?i)^-->\s*$`)
	// Single line: <!-- cheat ... -->
	cheatSingleLine = regexp.MustCompile(`(?i)^<!--\s*cheat\s+(.+?)\s*-->$`)
	exportRegex     = regexp.MustCompile(`^export\s+(\S+)$`)
	importRegex     = regexp.MustCompile(`^import\s+(\S+)$`)
	varRegex        = regexp.MustCompile(`^var\s+(\w+)\s*=\s*(.+)$`)
)

// Parser handles markdown file parsing
type Parser struct {
	index *CheatIndex
}

// NewParser creates a new parser
func NewParser() *Parser {
	return &Parser{
		index: NewCheatIndex(),
	}
}

// ParseDirectory recursively parses all markdown files
func (p *Parser) ParseDirectory(dir string) (*CheatIndex, error) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			if err := p.parseFile(path); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p.index, nil
}

// ParseSingleFile parses a single markdown file
func (p *Parser) ParseSingleFile(path string) (*CheatIndex, error) {
	if err := p.parseFile(path); err != nil {
		return nil, err
	}
	return p.index, nil
}

func (p *Parser) parseFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	p.parseLines(path, lines)
	return nil
}

type codeBlock struct {
	lang    string
	content string
}

func (p *Parser) parseLines(path string, lines []string) {
	var currentHeader string
	var currentDescription strings.Builder
	var inCodeBlock bool
	var codeBlockLang string
	var codeBlockContent strings.Builder
	var inCheatBlock bool
	var cheatBlockContent strings.Builder
	var pendingCodeBlocks []codeBlock

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Header check - starts new section
		if matches := headerRegex.FindStringSubmatch(line); matches != nil && !inCodeBlock && !inCheatBlock {
			p.processPending(path, currentHeader, currentDescription.String(), pendingCodeBlocks)
			pendingCodeBlocks = nil
			currentHeader = matches[2]
			currentDescription.Reset()
			continue
		}

		// Blockquote - extract as description
		if matches := blockquoteRe.FindStringSubmatch(line); matches != nil && !inCodeBlock && !inCheatBlock {
			if currentDescription.Len() > 0 {
				currentDescription.WriteString("\n")
			}
			currentDescription.WriteString(matches[1])
			continue
		}

		// Check for single-line cheat comment: <!-- cheat export foo -->
		if matches := cheatSingleLine.FindStringSubmatch(line); matches != nil && !inCodeBlock {
			if len(pendingCodeBlocks) > 0 {
				lastIdx := len(pendingCodeBlocks) - 1
				cheat := p.createCheat(path, currentHeader, currentDescription.String(),
					pendingCodeBlocks[lastIdx].content, matches[1])
				p.index.Cheats = append(p.index.Cheats, cheat)
				p.registerModule(cheat)
				pendingCodeBlocks = pendingCodeBlocks[:lastIdx]
			}
			continue
		}

		// Multi-line cheat comment start: <!-- cheat
		if cheatCommentStart.MatchString(line) && !inCodeBlock {
			inCheatBlock = true
			cheatBlockContent.Reset()
			continue
		}

		// Multi-line cheat comment end: -->
		if cheatCommentEnd.MatchString(line) && inCheatBlock {
			inCheatBlock = false
			cheatContent := cheatBlockContent.String()
			if len(pendingCodeBlocks) > 0 {
				// Cheat block attached to a code block
				lastIdx := len(pendingCodeBlocks) - 1
				cheat := p.createCheat(path, currentHeader, currentDescription.String(),
					pendingCodeBlocks[lastIdx].content, cheatContent)
				p.index.Cheats = append(p.index.Cheats, cheat)
				p.registerModule(cheat)
				pendingCodeBlocks = pendingCodeBlocks[:lastIdx]
			} else {
				// Standalone cheat block (export-only module, no command)
				cheat := p.createCheat(path, currentHeader, currentDescription.String(), "", cheatContent)
				// Only register as module, don't add to cheats list (no command to execute)
				if cheat.Export != "" {
					p.registerModule(cheat)
				}
			}
			continue
		}

		// Inside cheat block
		if inCheatBlock {
			cheatBlockContent.WriteString(line + "\n")
			continue
		}

		// Code block start
		if matches := codeBlockStart.FindStringSubmatch(line); matches != nil && !inCodeBlock {
			inCodeBlock = true
			codeBlockLang = matches[1]
			codeBlockContent.Reset()
			continue
		}

		// Code block end
		if codeBlockEnd.MatchString(line) && inCodeBlock {
			inCodeBlock = false
			content := strings.TrimSpace(codeBlockContent.String())
			if content != "" {
				pendingCodeBlocks = append(pendingCodeBlocks, codeBlock{
					lang:    codeBlockLang,
					content: content,
				})
			}
			continue
		}

		// Inside code block
		if inCodeBlock {
			codeBlockContent.WriteString(line + "\n")
			continue
		}
	}

	// Process remaining pending blocks (code blocks without cheat metadata)
	p.processPending(path, currentHeader, currentDescription.String(), pendingCodeBlocks)
}

func (p *Parser) registerModule(cheat *Cheat) {
	if cheat.Export != "" {
		p.index.Modules[cheat.Export] = &Module{
			Name:    cheat.Export,
			Vars:    cheat.Vars,
			Imports: cheat.Imports, // Store nested imports
			File:    cheat.File,
			Cheats:  []*Cheat{cheat},
		}
	}
}

func (p *Parser) processPending(path, header, description string, blocks []codeBlock) {
	for _, block := range blocks {
		if isShellLang(block.lang) && block.content != "" {
			cheat := p.createCheat(path, header, description, block.content, "")
			p.index.Cheats = append(p.index.Cheats, cheat)
		}
	}
}

func (p *Parser) createCheat(path, header, description, command, cheatBlock string) *Cheat {
	cheat := &Cheat{
		File:          path,
		Header:        header,
		Description:   strings.TrimSpace(description),
		Command:       command,
		Scope:         make(map[string]string),
		HasCheatBlock: cheatBlock != "",
	}

	if cheatBlock != "" {
		p.parseCheatDSL(cheat, cheatBlock)
	}

	cheat.Tags = extractTags(path, header)
	return cheat
}

func (p *Parser) parseCheatDSL(cheat *Cheat, content string) {
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if matches := exportRegex.FindStringSubmatch(line); matches != nil {
			cheat.Export = matches[1]
			continue
		}

		if matches := importRegex.FindStringSubmatch(line); matches != nil {
			cheat.Imports = append(cheat.Imports, matches[1])
			continue
		}

		if matches := varRegex.FindStringSubmatch(line); matches != nil {
			varDef := parseVarDef(matches[1], matches[2])
			cheat.Vars = append(cheat.Vars, varDef)
			continue
		}
	}
}

func parseVarDef(name, value string) VarDef {
	varDef := VarDef{Name: name}

	if idx := strings.Index(value, "---"); idx != -1 {
		varDef.Shell = strings.TrimSpace(value[:idx])
		varDef.FzfArgs = strings.TrimSpace(value[idx+3:])
	} else {
		varDef.Shell = strings.TrimSpace(value)
	}

	return varDef
}

func isShellLang(lang string) bool {
	shellLangs := map[string]bool{
		"": true, "sh": true, "shell": true, "bash": true,
		"zsh": true, "fish": true, "console": true,
	}
	return shellLangs[strings.ToLower(lang)]
}

func extractTags(path, header string) []string {
	var tags []string
	dir := filepath.Dir(path)
	parts := strings.Split(dir, string(filepath.Separator))
	for _, part := range parts {
		if part != "" && part != "." {
			tags = append(tags, strings.ToLower(part))
		}
	}

	if idx := strings.Index(header, ":"); idx != -1 {
		tags = append(tags, strings.ToLower(strings.TrimSpace(header[:idx])))
	}

	return tags
}
