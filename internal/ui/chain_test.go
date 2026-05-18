package ui

import (
	"path/filepath"
	"testing"

	"github.com/gubarz/cheatmd/pkg/chainstate"
	"github.com/gubarz/cheatmd/pkg/parser"
)

func TestChainQuerySelectsNextStoredStep(t *testing.T) {
	index := &parser.CheatIndex{Root: "/tmp/cheats", ChainMaxSteps: make(map[string]int)}
	steps := []*parser.Cheat{
		{Header: "First", ChainName: "demo", ChainStep: 1},
		{Header: "Second", ChainName: "demo", ChainStep: 2},
	}
	index.ChainMaxSteps["demo"] = 2
	m := newMainModel(steps, index, nil)
	m.chainState = &chainstate.State{
		Projects: map[string]*chainstate.ProjectState{
			"/tmp/cheats": {
				Chains: map[string]int{"demo": 2},
			},
		},
	}

	m.textInput.SetValue("/chain demo")
	m.filterCheats()

	if len(m.filtered) != 1 {
		t.Fatalf("filtered chains = %d, want 1", len(m.filtered))
	}
	if got := m.filtered[0].cheat.Header; got != "Second" {
		t.Fatalf("selected chain step = %q, want Second", got)
	}
	if m.filtered[0].chainStep != 2 || m.filtered[0].chainTotal != 2 {
		t.Fatalf("chain display step = %d/%d, want 2/2", m.filtered[0].chainStep, m.filtered[0].chainTotal)
	}
}

func TestParseChainQuery(t *testing.T) {
	query, ok := parseChainQuery("/chain my thing")
	if !ok || query != "my thing" {
		t.Fatalf("parseChainQuery = %q %v, want my thing true", query, ok)
	}
}

func TestAdvanceChainStoresNextStepAndActiveChain(t *testing.T) {
	index := &parser.CheatIndex{Root: "/tmp/cheats", ChainMaxSteps: map[string]int{"demo": 2}}
	first := &parser.Cheat{Header: "First", ChainName: "demo", ChainStep: 1}
	second := &parser.Cheat{Header: "Second", ChainName: "demo", ChainStep: 2}
	index.Cheats = []*parser.Cheat{first, second}
	state := &chainstate.State{Projects: make(map[string]*chainstate.ProjectState)}
	path := filepath.Join(t.TempDir(), "chains.json")

	advanceChain(index, first, path, state)

	if got := chainstate.GetStep(index.Root, "demo", state); got != 2 {
		t.Fatalf("next step = %d, want 2", got)
	}
	if got := chainstate.ActiveName(index.Root, state); got != "demo" {
		t.Fatalf("active chain = %q, want demo", got)
	}
}

func TestAdvanceChainClearsActiveAfterLastStep(t *testing.T) {
	index := &parser.CheatIndex{Root: "/tmp/cheats", ChainMaxSteps: map[string]int{"demo": 2}}
	first := &parser.Cheat{Header: "First", ChainName: "demo", ChainStep: 1}
	second := &parser.Cheat{Header: "Second", ChainName: "demo", ChainStep: 2}
	index.Cheats = []*parser.Cheat{first, second}
	state := &chainstate.State{
		Projects: map[string]*chainstate.ProjectState{
			"/tmp/cheats": {
				ActiveChain: "demo",
				Chains:      map[string]int{"demo": 1},
			},
		},
	}
	path := filepath.Join(t.TempDir(), "chains.json")

	advanceChain(index, second, path, state)

	if got := chainstate.GetStep(index.Root, "demo", state); got != 1 {
		t.Fatalf("next step = %d, want reset to 1", got)
	}
	if got := chainstate.ActiveName(index.Root, state); got != "" {
		t.Fatalf("active chain = %q, want cleared", got)
	}
}
