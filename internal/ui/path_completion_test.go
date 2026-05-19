package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gubarz/cheatmd/pkg/parser"
)

func TestCompletePathValueCompletesDirectoryWithSlash(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "wordlists"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := filepath.Join(dir, "wo")
	result, ok := completePathValue(input, len([]rune(input)))
	if !ok {
		t.Fatal("expected completion")
	}
	want := filepath.Join(dir, "wordlists") + string(os.PathSeparator)
	if result.Value != want {
		t.Fatalf("completion = %q, want %q", result.Value, want)
	}
}

func TestCompletePathValueEscapesSpaces(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "with space"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := filepath.Join(dir, "with")
	result, ok := completePathValue(input, len([]rune(input)))
	if !ok {
		t.Fatal("expected completion")
	}
	if !strings.HasSuffix(result.Value, "with\\ space/") {
		t.Fatalf("completion = %q, want escaped space", result.Value)
	}
}

func TestCompletePathValueExpandsEnvRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHEATMD_COMPLETION_ROOT", dir)

	input := "$CHEATMD_COMPLETION_ROOT/rep"
	result, ok := completePathValue(input, len([]rune(input)))
	if !ok {
		t.Fatal("expected completion")
	}
	if result.Value != "$CHEATMD_COMPLETION_ROOT/reports/" {
		t.Fatalf("completion = %q", result.Value)
	}
}

func TestCompletePathValueCompletesEnvVariableName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHEATMD_COMPLETION_DIR", dir)

	input := "$CHEATMD_COMPLETION_D"
	result, ok := completePathValue(input, len([]rune(input)))
	if !ok {
		t.Fatal("expected completion")
	}
	if result.Value != "$CHEATMD_COMPLETION_DIR/" {
		t.Fatalf("completion = %q", result.Value)
	}
}

func TestCompletePathValueListsEmptySegment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	input := dir + string(os.PathSeparator)
	result, ok := completePathValue(input, len([]rune(input)))
	if !ok {
		t.Fatal("expected completion")
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(result.Candidates))
	}
}

func TestVariablePromptTabCompletesPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "loot"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newMainModel([]*parser.Cheat{{Header: "One"}}, parser.NewCheatIndex(), nil)
	m.phase = phaseVarResolve
	m.varState = &varResolveState{isPromptOnly: true}
	input := filepath.Join(dir, "lo")
	m.textInput.SetValue(input)
	m.textInput.CursorEnd()

	model, _ := m.updateVarResolve(tea.KeyMsg{Type: tea.KeyTab})
	got := model.(*mainModel).textInput.Value()
	want := filepath.Join(dir, "loot") + string(os.PathSeparator)
	if got != want {
		t.Fatalf("input after tab = %q, want %q", got, want)
	}
}
