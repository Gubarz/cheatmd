package ui

import (
	"testing"

	"github.com/gubarz/cheatmd/pkg/parser"
)

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
