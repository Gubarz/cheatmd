package parser

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// ============================================================================
// String Interner - reduces memory by deduplicating strings
// ============================================================================

// stringInterner provides string deduplication (interning)
type stringInterner struct {
	mu      sync.RWMutex
	strings map[string]string
}

func newStringInterner() *stringInterner {
	return &stringInterner{
		strings: make(map[string]string, 1024),
	}
}

// Intern returns a canonical version of the string
// If the string was seen before, returns the previously stored instance
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
	// Double-check after acquiring write lock
	if interned, ok := si.strings[s]; ok {
		si.mu.Unlock()
		return interned
	}
	si.strings[s] = s
	si.mu.Unlock()
	return s
}

// Global interner for file paths and common strings
var pathInterner = newStringInterner()

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

// NewCheat creates a new Cheat
func NewCheat(file, header string) *Cheat {
	return &Cheat{
		File:   pathInterner.Intern(file),
		Header: header,
		// Scope allocated lazily at runtime when needed
	}
}

// VarDef represents a variable definition
type VarDef struct {
	Name      string // Variable name
	Shell     string // Shell command to generate values (for = syntax)
	Literal   string // Literal value with var substitution (for := syntax)
	Args      string // Selector arguments after ---
	Condition string // Conditional expression: "$var == value" or "$var != value"
}

// ParseVarDef parses a variable definition from name and value (shell command)
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

// ParseVarDefLiteral parses a literal variable definition (no shell, just substitution)
func ParseVarDefLiteral(name, value string) VarDef {
	v := VarDef{Name: name}
	if idx := strings.Index(value, "---"); idx != -1 {
		v.Literal = strings.TrimSpace(value[:idx])
		v.Args = strings.TrimSpace(value[idx+3:])
	} else {
		v.Literal = strings.TrimSpace(value)
	}
	return v
}

// ParseVarDefWithCondition parses a variable definition with an optional condition
func ParseVarDefWithCondition(name, value, condition string, isLiteral bool) VarDef {
	var v VarDef
	if isLiteral {
		v = ParseVarDefLiteral(name, value)
	} else {
		v = ParseVarDef(name, value)
	}
	v.Condition = condition
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

// DuplicateExport records a duplicate export definition
type DuplicateExport struct {
	Name  string
	File1 string
	File2 string
}

// CheatIndex holds all parsed cheats and modules
type CheatIndex struct {
	Cheats     []*Cheat
	Modules    map[string]*Module
	Duplicates []DuplicateExport
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
// Tracks duplicates if the same export name is used multiple times
func (idx *CheatIndex) RegisterModule(cheat *Cheat) {
	if cheat.Export == "" {
		return
	}
	if existing, ok := idx.Modules[cheat.Export]; ok {
		idx.Duplicates = append(idx.Duplicates, DuplicateExport{
			Name:  cheat.Export,
			File1: existing.File,
			File2: cheat.File,
		})
	}
	idx.Modules[cheat.Export] = NewModule(cheat)
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
	varDefLiteral   *regexp.Regexp
	varDefPrompt    *regexp.Regexp
	ifStart         *regexp.Regexp
	ifEnd           *regexp.Regexp
}{
	header:          regexp.MustCompile(`^(#{1,6})\s+(.+)$`),
	codeBlockStart:  regexp.MustCompile("^```(\\w*)(?:\\s+title:\"([^\"]*)\")?\\s*$"),
	cheatStart:      regexp.MustCompile(`(?i)^<!--\s*cheat\s*$`),
	cheatEnd:        regexp.MustCompile(`(?i)^-->\s*$`),
	cheatSingleLine: regexp.MustCompile(`(?i)^<!--\s*cheat\s*(.*?)\s*-->$`),
	export:          regexp.MustCompile(`^export\s+(\S+)$`),
	importStmt:      regexp.MustCompile(`^import\s+(\S+)$`),
	varDef:          regexp.MustCompile(`^var\s+(\w+)\s*=\s*(.+)$`),
	varDefLiteral:   regexp.MustCompile(`^var\s+(\w+)\s*:=\s*(.+)$`),
	varDefPrompt:    regexp.MustCompile(`^var\s+(\w+)\s*$`),
	ifStart:         regexp.MustCompile(`^if\s+(.+)$`),
	ifEnd:           regexp.MustCompile(`^fi$`),
}

// IsShellLanguage returns true if the language is a shell language
func IsShellLanguage(lang string) bool {
	lang = strings.ToLower(lang)
	// Exclude diagrams or data formats that would choke on $variable injection
	if lang == "mermaid" || lang == "dot" || lang == "chart" {
		return false
	}
	// Everything else is likely a script or a command snippet
	return true
}

// ============================================================================
// Parser
// ============================================================================

// Parser handles markdown file parsing
type Parser struct {
	index         *CheatIndex
	pathTagsCache map[string][]string // cache tags per directory
}

// NewParser creates a new parser
func NewParser() *Parser {
	return &Parser{
		index:         NewCheatIndex(),
		pathTagsCache: make(map[string][]string),
	}
}

// ParseDirectory recursively parses all markdown files in a directory
func (p *Parser) ParseDirectory(dir string) (*CheatIndex, error) {
	files, err := collectMarkdownFiles(dir)
	if err != nil {
		return nil, err
	}

	results := parseFilesParallel(files)
	p.mergeResults(results)

	return p.index, nil
}

// collectMarkdownFiles walks dir and returns all .md file paths
func collectMarkdownFiles(dir string) ([]string, error) {
	files := make([]string, 0, 4096)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isMarkdownFile(path) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// parseResult holds the output from parsing a batch of files
type parseResult struct {
	cheats  []*Cheat
	modules map[string]*Module
}

// parseFilesParallel reads and parses files using a two-stage pipeline
func parseFilesParallel(files []string) []parseResult {
	numWorkers := runtime.NumCPU()
	numFiles := len(files)
	estimatedCheats := max(numFiles*35, 1000)

	// Stage 1: Parallel I/O - read raw bytes
	type fileData struct {
		path string
		data []byte
	}
	fileDataChan := make(chan fileData, numFiles)
	fileChan := make(chan string, numFiles)

	var ioWg sync.WaitGroup
	ioWorkers := min(numWorkers*2, numFiles)
	for w := 0; w < ioWorkers; w++ {
		ioWg.Add(1)
		go func() {
			defer ioWg.Done()
			for path := range fileChan {
				if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
					fileDataChan <- fileData{path: path, data: data}
				}
			}
		}()
	}

	go func() {
		for _, path := range files {
			fileChan <- path
		}
		close(fileChan)
		ioWg.Wait()
		close(fileDataChan)
	}()

	// Stage 2: Parallel parsing - parse from raw bytes
	resultChan := make(chan parseResult, numWorkers)
	var parseWg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		parseWg.Add(1)
		go func() {
			defer parseWg.Done()
			localParser := NewParser()
			localCheats := make([]*Cheat, 0, estimatedCheats/numWorkers)
			localModules := make(map[string]*Module)

			for fd := range fileDataChan {
				localParser.index = NewCheatIndex()
				localParser.parseLines(fd.path, fd.data)
				localCheats = append(localCheats, localParser.index.Cheats...)
				for name, mod := range localParser.index.Modules {
					localModules[name] = mod
				}
			}
			resultChan <- parseResult{cheats: localCheats, modules: localModules}
		}()
	}

	go func() {
		parseWg.Wait()
		close(resultChan)
	}()

	var results []parseResult
	for r := range resultChan {
		results = append(results, r)
	}
	return results
}

// mergeResults combines parse results into the parser's index
func (p *Parser) mergeResults(results []parseResult) {
	var totalCheats []*Cheat
	for _, r := range results {
		totalCheats = append(totalCheats, r.cheats...)
		for name, mod := range r.modules {
			if existing, ok := p.index.Modules[name]; ok {
				p.index.Duplicates = append(p.index.Duplicates, DuplicateExport{
					Name:  name,
					File1: existing.File,
					File2: mod.File,
				})
			}
			p.index.Modules[name] = mod
		}
	}
	p.index.Cheats = totalCheats
}

// ParseSingleFile parses a single markdown file
func (p *Parser) ParseSingleFile(path string) (*CheatIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p.parseLines(path, data)
	return p.index, nil
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
	codeBlockBuf      []byte // direct byte buffer, no Builder overhead
	inCheatBlock      bool
	cheatBlockBuf     []byte
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
	s.pendingCodeBlocks = s.pendingCodeBlocks[:0]
}

// parseStatePool reduces allocations by reusing parseState objects
var parseStatePool = sync.Pool{
	New: func() interface{} {
		return &parseState{
			pendingCodeBlocks: make([]codeBlock, 0, 8),
			codeBlockBuf:      make([]byte, 0, 512),
			cheatBlockBuf:     make([]byte, 0, 256),
		}
	},
}

func getParseState() *parseState {
	s := parseStatePool.Get().(*parseState)
	s.currentHeader = ""
	s.inCodeBlock = false
	s.codeBlockLang = ""
	s.codeBlockDesc = ""
	s.codeBlockBuf = s.codeBlockBuf[:0]
	s.inCheatBlock = false
	s.cheatBlockBuf = s.cheatBlockBuf[:0]
	s.pendingCodeBlocks = s.pendingCodeBlocks[:0]
	return s
}

func putParseState(s *parseState) {
	// Cap buffer sizes to prevent memory bloat in pool
	if cap(s.codeBlockBuf) > 64*1024 {
		s.codeBlockBuf = make([]byte, 0, 512)
	}
	if cap(s.cheatBlockBuf) > 16*1024 {
		s.cheatBlockBuf = make([]byte, 0, 256)
	}
	parseStatePool.Put(s)
}

// ============================================================================
// Line Parsing
// ============================================================================

// parseLines processes all lines in a file from raw bytes
func (p *Parser) parseLines(path string, data []byte) {
	state := getParseState()
	defer putParseState(state)

	// Process line by line without allocating []string
	start := 0
	for i := 0; i <= len(data); i++ {
		if i == len(data) || data[i] == '\n' {
			end := i
			if end > start && data[end-1] == '\r' {
				end--
			}
			p.parseLine(path, data[start:end], state)
			start = i + 1
		}
	}

	// Process remaining pending blocks
	p.processPendingBlocks(path, state.currentHeader, state.pendingCodeBlocks)
}

// parseLine processes a single line (as bytes, no allocation)
func (p *Parser) parseLine(path string, line []byte, s *parseState) {
	// Fast path: inside code block - just accumulate
	if s.inCodeBlock {
		if len(line) == 3 && line[0] == '`' && line[1] == '`' && line[2] == '`' {
			s.inCodeBlock = false
			content := trimSpaceBytes(s.codeBlockBuf)
			if len(content) > 0 {
				s.pendingCodeBlocks = append(s.pendingCodeBlocks, codeBlock{
					lang:        s.codeBlockLang,
					content:     string(content),
					description: s.codeBlockDesc,
				})
			}
			return
		}
		s.codeBlockBuf = append(s.codeBlockBuf, line...)
		s.codeBlockBuf = append(s.codeBlockBuf, '\n')
		return
	}

	// Fast path: inside cheat block - just accumulate
	if s.inCheatBlock {
		// Fast check: cheat end is "-->" possibly with whitespace
		if len(line) >= 2 && line[0] == '-' && line[1] == '-' {
			if isCheatEnd(line) {
				s.inCheatBlock = false
				p.processCheatBlock(path, s)
				return
			}
		}
		s.cheatBlockBuf = append(s.cheatBlockBuf, line...)
		s.cheatBlockBuf = append(s.cheatBlockBuf, '\n')
		return
	}

	// Quick character checks before expensive operations
	if len(line) == 0 {
		return
	}

	first := line[0]

	// Header - starts with #
	if first == '#' {
		if header, ok := parseHeader(line); ok {
			p.processPendingBlocks(path, s.currentHeader, s.pendingCodeBlocks)
			s.reset(header)
			return
		}
	}

	// Code block start - starts with ```
	if first == '`' && len(line) >= 3 && line[1] == '`' && line[2] == '`' {
		if lang, desc, ok := parseCodeBlockStart(line); ok {
			s.inCodeBlock = true
			s.codeBlockLang = lang
			s.codeBlockDesc = desc
			s.codeBlockBuf = s.codeBlockBuf[:0]
			return
		}
	}

	// Cheat comments - starts with <
	if first == '<' {
		// Single-line cheat comment: <!-- cheat ... -->
		if content, ok := parseCheatSingleLine(line); ok {
			p.processCheatComment(path, s, content)
			return
		}
		// Multi-line cheat block start: <!-- cheat
		if isCheatStart(line) {
			s.inCheatBlock = true
			s.cheatBlockBuf = s.cheatBlockBuf[:0]
			return
		}
	}
}

// parseHeader extracts header text without regex: "## Header" -> "Header"
func parseHeader(line []byte) (string, bool) {
	i := 0
	// Count leading #
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return "", false
	}
	// Must have space after #
	if i >= len(line) || line[i] != ' ' {
		return "", false
	}
	i++ // skip space
	// Rest is header text
	if i >= len(line) {
		return "", false
	}
	return string(line[i:]), true
}

// parseCodeBlockStart parses ```lang title:"desc" without regex for simple cases
func parseCodeBlockStart(line []byte) (lang, desc string, ok bool) {
	if len(line) < 3 || line[0] != '`' || line[1] != '`' || line[2] != '`' {
		return "", "", false
	}
	rest := line[3:]

	// Fast path: just ``` or ```lang
	titleIdx := bytes.Index(rest, []byte("title:"))
	if titleIdx == -1 {
		// No title - just extract lang (word characters until space or end)
		end := 0
		for end < len(rest) && isWordChar(rest[end]) {
			end++
		}
		return string(rest[:end]), "", true
	}

	// Has title - use regex for complex case
	if matches := patterns.codeBlockStart.FindSubmatch(line); matches != nil {
		lang := ""
		desc := ""
		if len(matches) > 1 {
			lang = string(matches[1])
		}
		if len(matches) > 2 {
			desc = string(matches[2])
		}
		return lang, desc, true
	}
	return "", "", false
}

// parseCheatSingleLine parses <!-- cheat ... --> and returns the content
func parseCheatSingleLine(line []byte) (string, bool) {
	// Quick rejection
	if len(line) < 15 { // minimum: <!-- cheat -->
		return "", false
	}
	if !bytes.HasPrefix(line, []byte("<!--")) {
		return "", false
	}
	if !bytes.HasSuffix(line, []byte("-->")) {
		return "", false
	}

	// Check for "cheat" after <!--
	inner := bytes.TrimSpace(line[4 : len(line)-3])
	if len(inner) < 5 {
		return "", false
	}

	// Case-insensitive "cheat" check
	if !bytes.EqualFold(inner[:5], []byte("cheat")) {
		return "", false
	}

	return string(bytes.TrimSpace(inner[5:])), true
}

// isCheatStart checks for <!-- cheat (multiline start)
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

// isCheatEnd checks for --> (multiline end)
func isCheatEnd(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return bytes.Equal(trimmed, []byte("-->"))
}

// isWordChar returns true for [a-zA-Z0-9_]
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// trimSpaceBytes trims leading/trailing whitespace from bytes without allocating
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

// processCheatComment handles single-line <!-- cheat ... --> comments
func (p *Parser) processCheatComment(path string, s *parseState, content string) {
	if len(s.pendingCodeBlocks) == 0 {
		return
	}
	lastIdx := len(s.pendingCodeBlocks) - 1
	block := s.pendingCodeBlocks[lastIdx]
	cheat := p.createCheat(path, s.currentHeader, block.description, block.content, content, true)
	p.index.AddCheat(cheat)
	p.index.RegisterModule(cheat)
	s.pendingCodeBlocks = s.pendingCodeBlocks[:lastIdx]
}

// processCheatBlock handles multi-line cheat blocks
func (p *Parser) processCheatBlock(path string, s *parseState) {
	content := string(s.cheatBlockBuf)

	if len(s.pendingCodeBlocks) > 0 {
		lastIdx := len(s.pendingCodeBlocks) - 1
		block := s.pendingCodeBlocks[lastIdx]
		cheat := p.createCheat(path, s.currentHeader, block.description, block.content, content, true)
		p.index.AddCheat(cheat)
		p.index.RegisterModule(cheat)
		s.pendingCodeBlocks = s.pendingCodeBlocks[:lastIdx]
	} else {
		// Standalone cheat block (module definition)
		cheat := p.createCheat(path, s.currentHeader, "", "", content, true)
		if cheat.Export != "" {
			p.index.RegisterModule(cheat)
		}
	}
}

// processPendingBlocks processes remaining code blocks without cheat metadata
func (p *Parser) processPendingBlocks(path, header string, blocks []codeBlock) {
	for _, block := range blocks {
		if IsShellLanguage(block.lang) && block.content != "" {
			cheat := p.createCheat(path, header, block.description, block.content, "", false)
			p.index.AddCheat(cheat)
		}
	}
}

// ============================================================================
// Cheat Creation
// ============================================================================

// createCheat creates a new cheat from parsed data
func (p *Parser) createCheat(path, header, description, command, cheatBlock string, hasCheatBlock bool) *Cheat {
	cheat := NewCheat(path, header)
	cheat.Description = strings.TrimSpace(description)
	cheat.Command = command
	cheat.HasCheatBlock = hasCheatBlock
	cheat.Tags = p.getTagsForPath(path, header)

	if cheatBlock != "" {
		parseCheatDSL(cheat, cheatBlock)
	}

	return cheat
}

// getTagsForPath returns cached path tags plus header tag
func (p *Parser) getTagsForPath(path, header string) []string {
	dir := filepath.Dir(path)
	pathTags, ok := p.pathTagsCache[dir]
	if !ok {
		// Build path tags once per directory
		for _, part := range strings.Split(dir, string(filepath.Separator)) {
			if part != "" && part != "." {
				pathTags = append(pathTags, strings.ToLower(part))
			}
		}
		p.pathTagsCache[dir] = pathTags
	}

	// Add header tag if present
	if idx := strings.IndexByte(header, ':'); idx != -1 {
		// Copy and append to avoid modifying cached slice
		tags := make([]string, len(pathTags), len(pathTags)+1)
		copy(tags, pathTags)
		return append(tags, strings.ToLower(strings.TrimSpace(header[:idx])))
	}

	return pathTags
}

// parseCheatDSL parses the DSL content within a cheat block
func parseCheatDSL(cheat *Cheat, content string) {
	// First, join lines that end with backslash (line continuation)
	lines := joinContinuationLines(strings.Split(content, "\n"))

	// Track current condition for if/fi blocks
	var currentCondition string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle if/fi blocks
		if matches := patterns.ifStart.FindStringSubmatch(line); matches != nil {
			currentCondition = strings.TrimSpace(matches[1])
			continue
		}

		if patterns.ifEnd.MatchString(line) {
			currentCondition = ""
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

		// Check for literal assignment first (:=) before shell assignment (=)
		if matches := patterns.varDefLiteral.FindStringSubmatch(line); matches != nil {
			cheat.Vars = append(cheat.Vars, ParseVarDefWithCondition(matches[1], matches[2], currentCondition, true))
			continue
		}

		if matches := patterns.varDef.FindStringSubmatch(line); matches != nil {
			cheat.Vars = append(cheat.Vars, ParseVarDefWithCondition(matches[1], matches[2], currentCondition, false))
			continue
		}

		// Check for prompt-only var (no assignment)
		if matches := patterns.varDefPrompt.FindStringSubmatch(line); matches != nil {
			cheat.Vars = append(cheat.Vars, VarDef{
				Name:      matches[1],
				Condition: currentCondition,
				// Shell and Literal both empty = prompt-only
			})
		}
	}
}

// joinContinuationLines joins lines that end with backslash
func joinContinuationLines(lines []string) []string {
	var result []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.HasSuffix(trimmed, "\\") {
			// Line continues - remove backslash and append
			current.WriteString(strings.TrimSuffix(trimmed, "\\"))
		} else {
			// Line ends - append and flush
			current.WriteString(line)
			result = append(result, current.String())
			current.Reset()
		}
	}

	// Don't forget any remaining content
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// ============================================================================
// Helpers
// ============================================================================

// isMarkdownFile checks if a path is a markdown file
func isMarkdownFile(path string) bool {
	if len(path) < 3 {
		return false
	}
	ext := path[len(path)-3:]
	return ext == ".md" || ext == ".MD" || strings.EqualFold(path[len(path)-3:], ".md")
}
