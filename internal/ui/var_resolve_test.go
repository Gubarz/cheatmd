package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gubarz/cheatmd/pkg/parser"
)

func TestVarResolveArrowKeysMoveSelection(t *testing.T) {
	m := newMainModel([]*parser.Cheat{{Header: "One"}}, parser.NewCheatIndex(), nil)
	items := []FilteredOption{
		{Display: "alpha", Original: "alpha", SearchText: "alpha"},
		{Display: "beta", Original: "beta", SearchText: "beta"},
		{Display: "gamma", Original: "gamma", SearchText: "gamma"},
	}
	m.phase = phaseVarResolve
	m.varState = &varResolveState{
		isPromptOnly: false,
		picker: NewPicker(items, func(opt FilteredOption, words []string) bool {
			return matchesAllWords(opt.SearchText, words)
		}),
	}

	model, _ := m.updateVarResolve(tea.KeyMsg{Type: tea.KeyDown})
	got := model.(*mainModel)
	if got.varState.picker.Cursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", got.varState.picker.Cursor)
	}

	model, _ = got.updateVarResolve(tea.KeyMsg{Type: tea.KeyUp})
	got = model.(*mainModel)
	if got.varState.picker.Cursor != 0 {
		t.Fatalf("cursor after up = %d, want 0", got.varState.picker.Cursor)
	}
}

func TestVarResolvePrefillFiltersSelectionOptions(t *testing.T) {
	m := newMainModel([]*parser.Cheat{{Header: "One"}}, parser.NewCheatIndex(), nil)
	m.phase = phaseVarResolve
	m.varState = &varResolveState{
		vars: []varState{
			{
				def:     parser.VarDef{Name: "mode"},
				prefill: "publish",
			},
		},
	}

	model, _ := m.handleShellResult(shellResultMsg{
		options: []string{
			"preview\tPreview changes",
			"publish\tPublish changes",
			"archive\tArchive output",
		},
	})
	got := model.(*mainModel)

	if got.textInput.Value() != "publish" {
		t.Fatalf("input = %q, want publish", got.textInput.Value())
	}
	if got.varState.picker == nil {
		t.Fatal("picker is nil")
	}
	if len(got.varState.picker.Filtered) != 1 {
		t.Fatalf("filtered options = %d, want 1", len(got.varState.picker.Filtered))
	}
	if got.varState.picker.Filtered[0].Display != "publish\tPublish changes" {
		t.Fatalf("filtered option = %q", got.varState.picker.Filtered[0].Display)
	}
}
