package parser

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ============================================================================
// Domain Types
// ============================================================================

// Cheat represents a single executable cheat entry
type Cheat struct {
	File          string            // Source file path
	Header        string            // Section header
	Description   string            // Description text
	Command       string            // Shell command template
	Tags          []string          // Tags from path/header
	Export        string            // Module name if exported
	Imports       []string          // Imported modules
	Vars          []VarDef          // Variable definitions
	Scope         map[string]string // Resolved values at runtime
	HasCheatBlock bool              // Whether this cheat has a <!-- cheat --> block
}

// NewCheat creates a new Cheat with initialized scope
func NewCheat(file, header string) *Cheat {
	return &Cheat{
		File:   file,
		Header: header,
		Scope:  make(map[string]string),
	}
}

// VarDef represents a variable definition
type VarDef struct {
	Name  string // Variable name
	Shell string // Shell command to generate values
	Args  string // Selector arguments after ---
}

// ParseVarDef parses a variable definition from name and value
func ParseVarDef(name, value string) VarDef {
	v := VarDef{Name: name}
	if idx := strings.Index(value, "---"); idx != -1 {
		v.Shell = strings.TrimSpace(value[:idx])
		v.Args = strings.TrimSpace(value[idx+3:])
	} else {
		v.Shell = strings.TrimSpace(value)
	}
	return v
}

// Module represents an exportable module with variables
type Module struct {
	Name    string
	Vars    []VarDef
	Imports []string
	File    string
	Cheats  []*Cheat
}

// NewModule creates a Module from a Cheat
func NewModule(cheat *Cheat) *Module {
	return &Module{
		Name:    cheat.Export,
		Vars:    cheat.Vars,
		Imports: cheat.Imports,
		File:    cheat.File,
		Cheats:  []*Cheat{cheat},
	}
}

// ============================================================================
// Cheat Index
// ============================================================================

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

// AddCheat adds a cheat to the index
func (idx *CheatIndex) AddCheat(cheat *Cheat) {
	idx.Cheats = append(idx.Cheats, cheat)
}

// RegisterModule registers a module from a cheat with an export
func (idx *CheatIndex) RegisterModule(cheat *Cheat) {
	if cheat.Export != "" {
		idx.Modules[cheat.Export] = NewModule(cheat)
	}
}

// ============================================================================
// Regex Patterns
// ============================================================================

var patterns = struct {
	header          *regexp.Regexp
	codeBlockStart  *regexp.Regexp
	cheatStart      *regexp.Regexp
	cheatEnd        *regexp.Regexp
	cheatSingleLine *regexp.Regexp
	export          *regexp.Regexp
	importStmt      *regexp.Regexp
	varDef          *regexp.Regexp
}{
	header:          regexp.MustCompile(`^(#{1,6})\s+(.+)$`),
	codeBlockStart:  regexp.MustCompile("^```(\\w*)(?:\\s+title:\"([^\"]*)\")?\\s*$"),
	cheatStart:      regexp.MustCompile(`(?i)^<!--\s*cheat\s*$`),
	cheatEnd:        regexp.MustCompile(`(?i)^-->\s*$`),
	cheatSingleLine: regexp.MustCompile(`(?i)^<!--\s*cheat\s+(.+?)\s*-->$`),
	export:          regexp.MustCompile(`^export\s+(\S+)$`),
	importStmt:      regexp.MustCompile(`^import\s+(\S+)$`),
	varDef:          regexp.MustCompile(`^var\s+(\w+)\s*=\s*(.+)$`),
}

// shellLanguages defines which code block languages are treated as shell
var shellLanguages = map[string]bool{
	"": true, "sh": true, "shell": true, "bash": true,
	"zsh": true, "fish": true, "console": true,
}

// IsShellLanguage returns true if the language is a shell language
func IsShellLanguage(lang string) bool {
	return shellLanguages[strings.ToLower(lang)]
}

// ============================================================================
// Parser
// ============================================================================

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

// ParseDirectory recursively parses all markdown files in a directory
func (p *Parser) ParseDirectory(dir string) (*CheatIndex, error) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isMarkdownFile(path) {
			return p.parseFile(path)
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

// parseFile reads and parses a single file
func (p *Parser) parseFile(path string) error {
	lines, err := readFileLines(path)
	if err != nil {
		return err
	}
	p.parseLines(path, lines)
	return nil
}

// ============================================================================
// Parse State
// ============================================================================

// parseState holds the current parsing state
type parseState struct {
	currentHeader     string
	inCodeBlock       bool
	codeBlockLang     string
	codeBlockDesc     string
	codeBlockContent  strings.Builder
	inCheatBlock      bool
	cheatBlockContent strings.Builder
	pendingCodeBlocks []codeBlock
}

// codeBlock represents a parsed code block
type codeBlock struct {
	lang        string
	content     string
	description string
}

// reset clears pending blocks and updates header
func (s *parseState) reset(newHeader string) {
	s.currentHeader = newHeader
	s.pendingCodeBlocks = nil
}

// ============================================================================
// Line Parsing
// ============================================================================

// parseLines processes all lines in a file
func (p *Parser) parseLines(path string, lines []string) {
	state := &parseState{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		p.parseLine(path, line, state)
	}

	// Process remaining pending blocks
	p.processPendingBlocks(path, state.currentHeader, state.pendingCodeBlocks)
}

// parseLine processes a single line
func (p *Parser) parseLine(path, line string, s *parseState) {
	// Header - starts new section
	if !s.inCodeBlock && !s.inCheatBlock {
		if matches := patterns.header.FindStringSubmatch(line); matches != nil {
			p.processPendingBlocks(path, s.currentHeader, s.pendingCodeBlocks)
			s.reset(matches[2])
			return
		}
	}

	// Single-line cheat comment
	if !s.inCodeBlock {
		if matches := patterns.cheatSingleLine.FindStringSubmatch(line); matches != nil {
			p.processCheatComment(path, s, matches[1])
			return
		}
	}

	// Multi-line cheat block start
	if !s.inCodeBlock && patterns.cheatStart.MatchString(line) {
		s.inCheatBlock = true
		s.cheatBlockContent.Reset()
		return
	}

	// Multi-line cheat block end
	if s.inCheatBlock && patterns.cheatEnd.MatchString(line) {
		s.inCheatBlock = false
		p.processCheatBlock(path, s)
		return
	}

	// Inside cheat block
	if s.inCheatBlock {
		s.cheatBlockContent.WriteString(line + "\n")
		return
	}

	// Code block start
	if !s.inCodeBlock {
		if matches := patterns.codeBlockStart.FindStringSubmatch(line); matches != nil {
			s.inCodeBlock = true
			s.codeBlockLang = matches[1]
			s.codeBlockDesc = ""
			if len(matches) > 2 {
				s.codeBlockDesc = matches[2]
			}
			s.codeBlockContent.Reset()
			return
		}
	}

	// Code block end
	if s.inCodeBlock && line == "```" {
		s.inCodeBlock = false
		content := strings.TrimSpace(s.codeBlockContent.String())
		if content != "" {
			s.pendingCodeBlocks = append(s.pendingCodeBlocks, codeBlock{
				lang:        s.codeBlockLang,
				content:     content,
				description: s.codeBlockDesc,
			})
		}
		return
	}

	// Inside code block
	if s.inCodeBlock {
		s.codeBlockContent.WriteString(line + "\n")
	}
}

// processCheatComment handles single-line <!-- cheat ... --> comments
func (p *Parser) processCheatComment(path string, s *parseState, content string) {
	if len(s.pendingCodeBlocks) == 0 {
		return
	}
	lastIdx := len(s.pendingCodeBlocks) - 1
	block := s.pendingCodeBlocks[lastIdx]
	cheat := p.createCheat(path, s.currentHeader, block.description, block.content, content)
	p.index.AddCheat(cheat)
	p.index.RegisterModule(cheat)
	s.pendingCodeBlocks = s.pendingCodeBlocks[:lastIdx]
}

// processCheatBlock handles multi-line cheat blocks
func (p *Parser) processCheatBlock(path string, s *parseState) {
	content := s.cheatBlockContent.String()

	if len(s.pendingCodeBlocks) > 0 {
		lastIdx := len(s.pendingCodeBlocks) - 1
		block := s.pendingCodeBlocks[lastIdx]
		cheat := p.createCheat(path, s.currentHeader, block.description, block.content, content)
		p.index.AddCheat(cheat)
		p.index.RegisterModule(cheat)
		s.pendingCodeBlocks = s.pendingCodeBlocks[:lastIdx]
	} else {
		// Standalone cheat block (module definition)
		cheat := p.createCheat(path, s.currentHeader, "", "", content)
		if cheat.Export != "" {
			p.index.RegisterModule(cheat)
		}
	}
}

// processPendingBlocks processes remaining code blocks without cheat metadata
func (p *Parser) processPendingBlocks(path, header string, blocks []codeBlock) {
	for _, block := range blocks {
		if IsShellLanguage(block.lang) && block.content != "" {
			cheat := p.createCheat(path, header, block.description, block.content, "")
			p.index.AddCheat(cheat)
		}
	}
}

// ============================================================================
// Cheat Creation
// ============================================================================

// createCheat creates a new cheat from parsed data
func (p *Parser) createCheat(path, header, description, command, cheatBlock string) *Cheat {
	cheat := NewCheat(path, header)
	cheat.Description = strings.TrimSpace(description)
	cheat.Command = command
	cheat.HasCheatBlock = cheatBlock != ""
	cheat.Tags = extractTags(path, header)

	if cheatBlock != "" {
		parseCheatDSL(cheat, cheatBlock)
	}

	return cheat
}

// parseCheatDSL parses the DSL content within a cheat block
func parseCheatDSL(cheat *Cheat, content string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if matches := patterns.export.FindStringSubmatch(line); matches != nil {
			cheat.Export = matches[1]
			continue
		}

		if matches := patterns.importStmt.FindStringSubmatch(line); matches != nil {
			cheat.Imports = append(cheat.Imports, matches[1])
			continue
		}

		if matches := patterns.varDef.FindStringSubmatch(line); matches != nil {
			cheat.Vars = append(cheat.Vars, ParseVarDef(matches[1], matches[2]))
		}
	}
}

// ============================================================================
// Helpers
// ============================================================================

// readFileLines reads all lines from a file
func readFileLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// isMarkdownFile checks if a path is a markdown file
func isMarkdownFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".md")
}

// extractTags extracts tags from path and header
func extractTags(path, header string) []string {
	var tags []string

	// Tags from directory path
	dir := filepath.Dir(path)
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		if part != "" && part != "." {
			tags = append(tags, strings.ToLower(part))
		}
	}

	// Tag from header prefix (e.g., "git: clone" -> "git")
	if idx := strings.Index(header, ":"); idx != -1 {
		tags = append(tags, strings.ToLower(strings.TrimSpace(header[:idx])))
	}

	return tags
}
