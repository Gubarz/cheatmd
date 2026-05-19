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
