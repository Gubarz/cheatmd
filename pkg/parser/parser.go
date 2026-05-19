package parser

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

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
	p.index.Root = dir
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
	cheats     []*Cheat
	modules    map[string]*Module
	duplicates []DuplicateExport
	errors     []ParseError
}

// parseFilesParallel reads and parses files using a two-stage pipeline
func parseFilesParallel(files []string) []parseResult {
	numWorkers := runtime.NumCPU()
	numFiles := len(files)
	estimatedCheats := max(numFiles*35, 1000)

	resultChan := make(chan parseResult, numWorkers)
	var parseWg sync.WaitGroup

	chunkSize := (numFiles + numWorkers - 1) / numWorkers
	if chunkSize == 0 {
		chunkSize = 1
	}

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		if start >= numFiles {
			break
		}
		end := start + chunkSize
		if end > numFiles {
			end = numFiles
		}

		chunk := files[start:end]
		parseWg.Add(1)

		go func(fileChunk []string) {
			defer parseWg.Done()
			localParser := NewParser()
			localCheats := make([]*Cheat, 0, estimatedCheats/numWorkers)
			localModules := make(map[string]*Module)
			var localDuplicates []DuplicateExport

			for _, path := range fileChunk {
				if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
					localParser.index = NewCheatIndex()
					localParser.parseLines(path, data)
					localCheats = append(localCheats, localParser.index.Cheats...)
					for name, mod := range localParser.index.Modules {
						if existing, ok := localModules[name]; ok {
							localDuplicates = append(localDuplicates, DuplicateExport{
								Name:  name,
								File1: existing.File,
								File2: mod.File,
							})
						}
						localModules[name] = mod
					}
				}
			}
			resultChan <- parseResult{cheats: localCheats, modules: localModules, duplicates: localDuplicates, errors: localParser.index.Errors}
		}(chunk)
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
		// Carry forward any duplicates detected within a single worker
		p.index.Duplicates = append(p.index.Duplicates, r.duplicates...)
		p.index.Errors = append(p.index.Errors, r.errors...)
		for name, mod := range r.modules {
			if p.index.Modules == nil {
				p.index.Modules = make(map[string]*Module)
			}
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
	for _, c := range totalCheats {
		if c.ChainName != "" && c.ChainStep > p.index.ChainMaxSteps[c.ChainName] {
			if p.index.ChainMaxSteps == nil {
				p.index.ChainMaxSteps = make(map[string]int)
			}
			p.index.ChainMaxSteps[c.ChainName] = c.ChainStep
		}
	}
}

// ParseSingleFile parses a single markdown file
func (p *Parser) ParseSingleFile(path string) (*CheatIndex, error) {
	p.index.Root = path
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
	lineNo            int
	inCodeBlock       bool
	codeBlockLang     string
	codeBlockStart    int
	codeBlockDesc     string
	codeBlockBuf      []byte // direct byte buffer, no Builder overhead
	inCheatBlock      bool
	cheatBlockStart   int
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
	startLine   int
	endLine     int
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
	s.lineNo = 0
	s.inCodeBlock = false
	s.codeBlockLang = ""
	s.codeBlockStart = 0
	s.codeBlockDesc = ""
	s.codeBlockBuf = s.codeBlockBuf[:0]
	s.inCheatBlock = false
	s.cheatBlockStart = 0
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
	lineOffset := countNewlines(data[:len(data)-len(body)])
	body, footerTags := extractFooterTags(body)
	state.fileTags = append(state.fileTags, frontTags...)
	state.fileTags = append(state.fileTags, footerTags...)
	state.lineNo = lineOffset

	// Process line by line without allocating []string
	start := 0
	for i := 0; i <= len(body); i++ {
		if i == len(body) || body[i] == '\n' {
			end := i
			if end > start && body[end-1] == '\r' {
				end--
			}
			state.lineNo++
			p.parseLine(path, body[start:end], state)
			start = i + 1
		}
	}

	if state.inCheatBlock {
		p.index.Errors = append(p.index.Errors, ParseError{
			File:    path,
			Line:    state.cheatBlockStart,
			Message: "unterminated `<!-- cheat -->` block (missing `-->`)",
		})
	}

	// Process remaining pending blocks
	p.processPendingBlocks(path, state)
}

func countNewlines(b []byte) int {
	return bytes.Count(b, []byte{'\n'})
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
					startLine:   s.codeBlockStart,
					endLine:     s.lineNo - 1,
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
			if header == "" {
				p.index.Errors = append(p.index.Errors, ParseError{
					File:    path,
					Line:    s.lineNo,
					Message: "empty markdown header",
				})
			}
			return
		}
	}

	// Code block start - starts with ```
	if first == '`' && len(line) >= 3 && line[1] == '`' && line[2] == '`' {
		if lang, desc, ok := parseCodeBlockStart(line); ok {
			s.inCodeBlock = true
			s.codeBlockLang = lang
			s.codeBlockStart = s.lineNo + 1
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
			s.cheatBlockStart = s.lineNo
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

// processCheatComment handles single-line <!-- cheat ... --> comments
func (p *Parser) processCheatComment(path string, s *parseState, content string) {
	if len(s.pendingCodeBlocks) == 0 {
		// Standalone single-line comment without a code block
		cheat := p.createCheat(path, s, codeBlock{}, content, true, s.lineNo)
		if cheat.Export == "" {
			p.index.Errors = append(p.index.Errors, ParseError{
				File:    path,
				Line:    s.lineNo,
				Message: "<!-- cheat --> block has no preceding code block",
			})
		} else {
			p.index.RegisterModule(cheat)
		}
		return
	}
	p.flushLastPendingCheat(path, s, content, s.lineNo)
}

// processCheatBlock handles multi-line cheat blocks
func (p *Parser) processCheatBlock(path string, s *parseState) {
	content := string(s.cheatBlockBuf)

	if len(s.pendingCodeBlocks) > 0 {
		p.flushLastPendingCheat(path, s, content, s.cheatBlockStart)
	} else {
		// Standalone cheat block (module definition)
		cheat := p.createCheat(path, s, codeBlock{}, content, true, s.cheatBlockStart)
		if cheat.Export == "" {
			p.index.Errors = append(p.index.Errors, ParseError{
				File:    path,
				Line:    s.cheatBlockStart,
				Message: "<!-- cheat --> block has no preceding code block",
			})
		} else {
			p.index.RegisterModule(cheat)
		}
	}
}

// flushLastPendingCheat pops the last pending code block and creates a cheat from it
func (p *Parser) flushLastPendingCheat(path string, s *parseState, cheatBlock string, cheatLine int) {
	lastIdx := len(s.pendingCodeBlocks) - 1
	block := s.pendingCodeBlocks[lastIdx]
	cheat := p.createCheat(path, s, block, cheatBlock, true, cheatLine)
	p.index.AddCheat(cheat)
	p.index.RegisterModule(cheat)
	s.pendingCodeBlocks = s.pendingCodeBlocks[:lastIdx]
}

// processPendingBlocks processes remaining code blocks without cheat metadata
func (p *Parser) processPendingBlocks(path string, s *parseState) {
	for _, block := range s.pendingCodeBlocks {
		if IsShellLanguage(block.lang) && block.content != "" {
			cheat := p.createCheat(path, s, block, "", false, block.startLine)
			p.index.AddCheat(cheat)
		}
	}
}

// ============================================================================
// Cheat Creation
// ============================================================================

// createCheat creates a new cheat from parsed data
func (p *Parser) createCheat(path string, s *parseState, block codeBlock, cheatBlock string, hasCheatBlock bool, cheatLine int) *Cheat {
	cheat := NewCheat(path, s.currentHeader)
	cheat.Description = strings.TrimSpace(block.description)
	cheat.Command = block.content
	cheat.CommandLang = block.lang
	cheat.CommandStart = block.startLine
	cheat.CommandEnd = block.endLine
	cheat.HasCheatBlock = hasCheatBlock
	cheat.Tags = p.buildCheatTags(path, s)

	if cheat.Header == "" && hasCheatBlock {
		p.index.Errors = append(p.index.Errors, ParseError{
			File:    path,
			Line:    cheatLine,
			Message: "cheat has no markdown header",
		})
	}

	if cheatBlock != "" {
		errors := parseCheatDSL(cheat, cheatBlock, path, cheatLine)
		p.index.Errors = append(p.index.Errors, errors...)
	}

	s.headerCheats = append(s.headerCheats, cheat)
	return cheat
}
