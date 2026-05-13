package parser

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtractFrontMatterTags(t *testing.T) {
	content := []byte("---\ntags:\n  - bash\n  - networking\n  - security\nauthor: goober\n---\n\n# My Cheatsheet\n")
	_, tags := extractFrontMatterTags(content)
	want := []string{"bash", "networking", "security"}

	if !reflect.DeepEqual(tags, want) {
		t.Errorf("extractFrontMatterTags() = %v, want %v", tags, want)
	}
}

func TestExtractFrontMatterTags_NoFrontmatter(t *testing.T) {
	content := []byte("# Just a header\nSome content\n")
	_, tags := extractFrontMatterTags(content)

	if len(tags) != 0 {
		t.Errorf("extractFrontMatterTags() expected no tags, got %v", tags)
	}
}

func TestParseHashtagLine(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		wantOk bool
		want   []string
	}{
		{
			name:   "all hashtags",
			line:   "#bash #linux #networking",
			wantOk: true,
			want:   []string{"bash", "linux", "networking"},
		},
		{
			name:   "single hashtag",
			line:   "#pentest",
			wantOk: true,
			want:   []string{"pentest"},
		},
		{
			name:   "mixed text rejects",
			line:   "# Just a header",
			wantOk: false,
			want:   nil,
		},
		{
			name:   "header with inline tags rejects",
			line:   "# My Header #bash #linux",
			wantOk: false,
			want:   nil,
		},
		{
			name:   "empty line",
			line:   "",
			wantOk: false,
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, ok := parseHashtagLine([]byte(tt.line))
			if ok != tt.wantOk {
				t.Errorf("parseHashtagLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOk)
			}
			if tt.wantOk && !reflect.DeepEqual(tags, tt.want) {
				t.Errorf("parseHashtagLine(%q) = %v, want %v", tt.line, tags, tt.want)
			}
		})
	}
}

func TestParseCheatDSL(t *testing.T) {
	dslBlock := "var host = 192.168.1.1\nvar port = 80,443 --- Target ports\nvar timeout = 10"

	cheat := &Cheat{}
	parseCheatDSL(cheat, dslBlock)

	if len(cheat.Vars) != 3 {
		t.Fatalf("parseCheatDSL() parsed %d vars, want 3", len(cheat.Vars))
	}

	if cheat.Vars[0].Name != "host" || cheat.Vars[0].Shell != "192.168.1.1" {
		t.Errorf("parseCheatDSL() first var = {Name:%q Shell:%q}, want {Name:\"host\" Shell:\"192.168.1.1\"}", cheat.Vars[0].Name, cheat.Vars[0].Shell)
	}

	if cheat.Vars[1].Name != "port" || cheat.Vars[1].Shell != "80,443" || cheat.Vars[1].Args != "Target ports" {
		t.Errorf("parseCheatDSL() second var = {Name:%q Shell:%q Args:%q}", cheat.Vars[1].Name, cheat.Vars[1].Shell, cheat.Vars[1].Args)
	}
}

func TestParseCheatDSL_Literal(t *testing.T) {
	dslBlock := "var greeting := hello world"

	cheat := &Cheat{}
	parseCheatDSL(cheat, dslBlock)

	if len(cheat.Vars) != 1 {
		t.Fatalf("parseCheatDSL() parsed %d vars, want 1", len(cheat.Vars))
	}

	if cheat.Vars[0].Name != "greeting" || cheat.Vars[0].Literal != "hello world" {
		t.Errorf("parseCheatDSL() literal var = {Name:%q Literal:%q}", cheat.Vars[0].Name, cheat.Vars[0].Literal)
	}
}

func TestParseCheatDSL_Conditional(t *testing.T) {
	dslBlock := "if $method == password\nvar cred = echo enter-password\nfi"

	cheat := &Cheat{}
	parseCheatDSL(cheat, dslBlock)

	if len(cheat.Vars) != 1 {
		t.Fatalf("parseCheatDSL() parsed %d vars, want 1", len(cheat.Vars))
	}

	if cheat.Vars[0].Condition != "$method == password" {
		t.Errorf("parseCheatDSL() condition = %q, want %q", cheat.Vars[0].Condition, "$method == password")
	}
}

func TestParseCheatDSL_ExportImport(t *testing.T) {
	dslBlock := "export mymodule\nimport othermodule"

	cheat := &Cheat{}
	parseCheatDSL(cheat, dslBlock)

	if cheat.Export != "mymodule" {
		t.Errorf("parseCheatDSL() Export = %q, want %q", cheat.Export, "mymodule")
	}

	if len(cheat.Imports) != 1 || cheat.Imports[0] != "othermodule" {
		t.Errorf("parseCheatDSL() Imports = %v, want [othermodule]", cheat.Imports)
	}
}

func TestParseCheatDSL_Comments(t *testing.T) {
	dslBlock := "# this is a comment\nvar host = echo localhost"

	cheat := &Cheat{}
	parseCheatDSL(cheat, dslBlock)

	if len(cheat.Vars) != 1 {
		t.Fatalf("parseCheatDSL() parsed %d vars, want 1", len(cheat.Vars))
	}

	if cheat.Vars[0].Name != "host" {
		t.Errorf("parseCheatDSL() var name = %q, want %q", cheat.Vars[0].Name, "host")
	}
}

func TestParseSingleLineEmptyCheatComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "single.md")
	content := "# Cheats\n\n## Empty metadata\n\n```sh\necho hi\n```\n<!-- cheat -->\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewParser()
	index, err := p.ParseSingleFile(path)
	if err != nil {
		t.Fatalf("ParseSingleFile() error: %v", err)
	}

	if len(index.Cheats) != 1 {
		t.Fatalf("ParseSingleFile() cheats = %d, want 1", len(index.Cheats))
	}
	cheat := index.Cheats[0]
	if !cheat.HasCheatBlock {
		t.Fatal("single-line <!-- cheat --> did not mark cheat as having a cheat block")
	}
	if cheat.Command != "echo hi" {
		t.Fatalf("command = %q, want %q", cheat.Command, "echo hi")
	}
}

func TestParseCheatSingleLineRequiresKeywordBoundary(t *testing.T) {
	if _, ok := parseCheatSingleLine([]byte("<!-- cheating -->")); ok {
		t.Fatal("parseCheatSingleLine accepted non-cheat keyword")
	}
}

func TestNewCheatIndex(t *testing.T) {
	idx := NewCheatIndex()
	if idx == nil {
		t.Fatal("NewCheatIndex() returned nil")
	}
	if idx.Cheats != nil {
		t.Errorf("NewCheatIndex() Cheats should be nil, got %v", idx.Cheats)
	}
}

func TestCheatIndexAddCheat(t *testing.T) {
	idx := NewCheatIndex()
	cheat := &Cheat{Header: "test"}
	idx.AddCheat(cheat)

	if len(idx.Cheats) != 1 {
		t.Fatalf("AddCheat() len = %d, want 1", len(idx.Cheats))
	}
	if idx.Cheats[0].Header != "test" {
		t.Errorf("AddCheat() header = %q, want \"test\"", idx.Cheats[0].Header)
	}
}

func TestRegisterModule(t *testing.T) {
	idx := NewCheatIndex()
	cheat := &Cheat{
		Export: "mymod",
		File:   "test.md",
		Vars:   []VarDef{{Name: "host", Shell: "echo localhost"}},
	}
	idx.RegisterModule(cheat)

	if idx.Modules == nil {
		t.Fatal("RegisterModule() did not initialize Modules map")
	}

	mod, ok := idx.Modules["mymod"]
	if !ok {
		t.Fatal("RegisterModule() module not found")
	}

	if mod.Name != "mymod" {
		t.Errorf("RegisterModule() module name = %q, want \"mymod\"", mod.Name)
	}
}

func TestRegisterModule_Duplicate(t *testing.T) {
	idx := NewCheatIndex()
	cheat1 := &Cheat{Export: "dup", File: "a.md"}
	cheat2 := &Cheat{Export: "dup", File: "b.md"}

	idx.RegisterModule(cheat1)
	idx.RegisterModule(cheat2)

	if len(idx.Duplicates) != 1 {
		t.Fatalf("RegisterModule() duplicates = %d, want 1", len(idx.Duplicates))
	}
	if idx.Duplicates[0].File1 != "a.md" || idx.Duplicates[0].File2 != "b.md" {
		t.Errorf("RegisterModule() duplicate = %v", idx.Duplicates[0])
	}
}

func TestParseVarDef(t *testing.T) {
	tests := []struct {
		name      string
		varName   string
		value     string
		wantShell string
		wantArgs  string
	}{
		{
			name:      "simple shell command",
			varName:   "host",
			value:     "echo 127.0.0.1",
			wantShell: "echo 127.0.0.1",
			wantArgs:  "",
		},
		{
			name:      "with description",
			varName:   "port",
			value:     "echo 80 --- Target port",
			wantShell: "echo 80",
			wantArgs:  "Target port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ParseVarDef(tt.varName, tt.value)
			if v.Name != tt.varName {
				t.Errorf("ParseVarDef() Name = %q, want %q", v.Name, tt.varName)
			}
			if v.Shell != tt.wantShell {
				t.Errorf("ParseVarDef() Shell = %q, want %q", v.Shell, tt.wantShell)
			}
			if v.Args != tt.wantArgs {
				t.Errorf("ParseVarDef() Args = %q, want %q", v.Args, tt.wantArgs)
			}
		})
	}
}

func TestParseVarDefLiteral(t *testing.T) {
	v := ParseVarDefLiteral("greeting", "hello world --- A greeting")
	if v.Name != "greeting" || v.Literal != "hello world" || v.Args != "A greeting" {
		t.Errorf("ParseVarDefLiteral() = {Name:%q Literal:%q Args:%q}", v.Name, v.Literal, v.Args)
	}
}

// TestDuplicateExportDetection verifies that duplicate exports are always
// detected regardless of which goroutine processes each file. This is a
// regression test for a race condition where two files with the same export
// landing on the same worker goroutine would silently overwrite instead of
// recording a duplicate.
func TestDuplicateExportDetection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cheats")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	fileA := filepath.Join(dir, "a.md")
	fileB := filepath.Join(dir, "b.md")

	contentA := "# Module A\n\n```bash\necho from-a\n```\n\n<!-- cheat\nexport mymod\nvar host = echo localhost\n-->\n"
	contentB := "# Module B\n\n```bash\necho from-b\n```\n\n<!-- cheat\nexport mymod\nvar host = echo remotehost\n-->\n"
	if err := os.WriteFile(fileA, []byte(contentA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte(contentB), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run multiple times to exercise different goroutine scheduling paths.
	// Before the fix, this would fail intermittently when both files landed
	// on the same worker.
	for i := 0; i < 20; i++ {
		p := NewParser()
		index, err := p.ParseDirectory(dir)
		if err != nil {
			t.Fatalf("run %d: ParseDirectory() error: %v", i, err)
		}

		if len(index.Duplicates) != 1 {
			t.Errorf("run %d: expected 1 duplicate, got %d", i, len(index.Duplicates))
			continue
		}

		dup := index.Duplicates[0]
		if dup.Name != "mymod" {
			t.Errorf("run %d: duplicate name = %q, want \"mymod\"", i, dup.Name)
		}
	}
}
