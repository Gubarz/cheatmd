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
	fileTags          []string // tags from front matter + footer
	currentHeaderTags []string // inline #tags under the current header
	headerCheats      []*Cheat // cheats already created under the current header
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
	s.currentHeaderTags = s.currentHeaderTags[:0]
	s.headerCheats = s.headerCheats[:0]
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
	s.fileTags = s.fileTags[:0]
	s.currentHeaderTags = s.currentHeaderTags[:0]
	s.headerCheats = s.headerCheats[:0]
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

	// Extract front matter and footer YAML blocks before line parsing
	body, frontTags := extractFrontMatterTags(data)
	body, footerTags := extractFooterTags(body)
	state.fileTags = append(state.fileTags, frontTags...)
	state.fileTags = append(state.fileTags, footerTags...)

	// Process line by line without allocating []string
	start := 0
	for i := 0; i <= len(body); i++ {
		if i == len(body) || body[i] == '\n' {
			end := i
			if end > start && body[end-1] == '\r' {
				end--
			}
			p.parseLine(path, body[start:end], state)
			start = i + 1
		}
	}

	// Process remaining pending blocks
	p.processPendingBlocks(path, state)
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
			p.processPendingBlocks(path, s)
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

	// Prose line: scan for inline #tag tokens and attach to current header
	if s.currentHeader != "" && bytes.IndexByte(line, '#') >= 0 {
		before := len(s.currentHeaderTags)
		scanInlineTags(line, &s.currentHeaderTags)
		if len(s.currentHeaderTags) > before && len(s.headerCheats) > 0 {
			newTags := s.currentHeaderTags[before:]
			for _, c := range s.headerCheats {
				c.Tags = mergeTags(c.Tags, newTags)
			}
		}
	}
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
	cheat := p.createCheat(path, s, block.description, block.content, content, true)
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
		cheat := p.createCheat(path, s, block.description, block.content, content, true)
		p.index.AddCheat(cheat)
		p.index.RegisterModule(cheat)
		s.pendingCodeBlocks = s.pendingCodeBlocks[:lastIdx]
	} else {
		// Standalone cheat block (module definition)
		cheat := p.createCheat(path, s, "", "", content, true)
		if cheat.Export != "" {
			p.index.RegisterModule(cheat)
		}
	}
}

// processPendingBlocks processes remaining code blocks without cheat metadata
func (p *Parser) processPendingBlocks(path string, s *parseState) {
	for _, block := range s.pendingCodeBlocks {
		if IsShellLanguage(block.lang) && block.content != "" {
			cheat := p.createCheat(path, s, block.description, block.content, "", false)
			p.index.AddCheat(cheat)
		}
	}
}

// ============================================================================
// Cheat Creation
// ============================================================================

// createCheat creates a new cheat from parsed data
func (p *Parser) createCheat(path string, s *parseState, description, command, cheatBlock string, hasCheatBlock bool) *Cheat {
	cheat := NewCheat(path, s.currentHeader)
	cheat.Description = strings.TrimSpace(description)
	cheat.Command = command
	cheat.HasCheatBlock = hasCheatBlock
	cheat.Tags = p.buildCheatTags(path, s)

	if cheatBlock != "" {
		parseCheatDSL(cheat, cheatBlock)
	}

	s.headerCheats = append(s.headerCheats, cheat)
	return cheat
}

// buildCheatTags merges all tag sources for one cheat: path tags, heading-suffix
// tag, file-level tags (front matter + footer), and per-cheat inline #tags.
// Result is lowercased and deduplicated.
//
// Fast paths avoid allocation when there are no extra tag sources or when
// extras are guaranteed not to duplicate path tags.
func (p *Parser) buildCheatTags(path string, s *parseState) []string {
	pathTags := p.getTagsForPath(path, s.currentHeader)

	extra := len(s.fileTags) + len(s.currentHeaderTags)
	if extra == 0 {
		return pathTags
	}

	out := make([]string, len(pathTags), len(pathTags)+extra)
	copy(out, pathTags)

	var seen map[string]struct{}
	appendUnique := func(tag string) {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			return
		}
		if seen == nil {
			seen = make(map[string]struct{}, len(pathTags)+extra)
			for _, t := range pathTags {
				seen[t] = struct{}{}
			}
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	for _, t := range s.fileTags {
		appendUnique(t)
	}
	for _, t := range s.currentHeaderTags {
		appendUnique(t)
	}
	return out
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
// Tag Extraction
// ============================================================================

// extractFrontMatterTags strips a leading YAML front-matter block (--- ... ---)
// from data and returns the remainder plus any tags found.
func extractFrontMatterTags(data []byte) ([]byte, []string) {
	// Must start with "---" followed by newline (allow optional BOM/whitespace).
	i := 0
	for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\r') {
		i++
	}
	if i+3 > len(data) || data[i] != '-' || data[i+1] != '-' || data[i+2] != '-' {
		return data, nil
	}
	// After "---" we require newline (allow trailing spaces).
	j := i + 3
	for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\r') {
		j++
	}
	if j >= len(data) || data[j] != '\n' {
		return data, nil
	}
	bodyStart := j + 1

	// Find closing "---" at the start of a line.
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
	// Trim trailing whitespace.
	end := len(data)
	for end > 0 && (data[end-1] == '\n' || data[end-1] == '\r' || data[end-1] == ' ' || data[end-1] == '\t') {
		end--
	}
	if end == 0 {
		return data, nil
	}

	// Try YAML form first.
	if body, tags, ok := extractYAMLFooter(data, end); ok {
		return body, tags
	}

	// Try hashtag form.
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
		// Find start of the line ending at cut.
		lineEnd := cut
		lineStart := lineEnd
		for lineStart > 0 && data[lineStart-1] != '\n' {
			lineStart--
		}
		line := bytes.TrimSpace(data[lineStart:lineEnd])

		if len(line) == 0 {
			// Blank line; allow it between tag lines.
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

		// Optional --- rule above the tag block; consume it then stop.
		if bytes.Equal(line, []byte("---")) && sawTagLine {
			cut = lineStart
			break
		}
		break
	}

	if !sawTagLine {
		return nil, nil, false
	}

	// Trim trailing whitespace from the kept body.
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
		// Skip whitespace.
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
		k := j + 1
		for k < len(line) {
			b := line[k]
			if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') ||
				b == '_' || b == '-' || b == '.' || b == '/' {
				k++
				continue
			}
			break
		}
		tags = append(tags, string(line[j:k]))
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
		// Find end of current line
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
			// Non list-item line ends the list mode unless it's another key
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

		// Inline forms: "[a, b]" or "a, b"
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
	// Skip heading lines (start with # space)
	if line[0] == '#' {
		// Real markdown heading: "# foo" with whitespace after some #s.
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
		// Must be at start of token (preceded by space/start/punct)
		if i > 0 {
			prev := line[i-1]
			if prev != ' ' && prev != '\t' && prev != '(' && prev != '[' && prev != ',' {
				continue
			}
		}
		// Next char must be ASCII letter (rules out hex colors, #1, #@, etc.)
		j := i + 1
		if j >= len(line) {
			return
		}
		c := line[j]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			continue
		}
		// Consume tag body: word chars plus - . /
		k := j + 1
		for k < len(line) {
			b := line[k]
			if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') ||
				b == '_' || b == '-' || b == '.' || b == '/' {
				k++
				continue
			}
			break
		}
		*dst = append(*dst, string(line[j:k]))
		i = k - 1
	}
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
