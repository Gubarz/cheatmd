package ui

import (
	"testing"

	"github.com/gubarz/cheatmd/pkg/history"
	"github.com/gubarz/cheatmd/pkg/parser"
)

func TestApplyFrecencyRanksCheatPicker(t *testing.T) {
	cold := &parser.Cheat{File: "cheats.md", Header: "Cold", Command: "cold"}
	hot := &parser.Cheat{File: "cheats.md", Header: "Hot", Command: "hot"}
	m := newMainModel([]*parser.Cheat{cold, hot}, parser.NewCheatIndex(), nil)

	m.applyFrecency(map[string]float64{
		history.CheatKey(hot.File, hot.Header): 2,
	})

	if got := m.picker.Filtered[0].cheat.Header; got != "Hot" {
		t.Fatalf("top ranked cheat = %q, want Hot", got)
	}
}

func TestApplyFrecencyRanksFilteredResults(t *testing.T) {
	first := &parser.Cheat{File: "cheats.md", Header: "First Target", Command: "run target"}
	second := &parser.Cheat{File: "cheats.md", Header: "Second Target", Command: "run target"}
	m := newMainModel([]*parser.Cheat{first, second}, parser.NewCheatIndex(), nil)
	m.applyFrecency(map[string]float64{
		history.CheatKey(second.File, second.Header): 2,
	})

	m.textInput.SetValue("target")
	m.filterCheats()

	if got := m.picker.Filtered[0].cheat.Header; got != "Second Target" {
		t.Fatalf("top filtered cheat = %q, want Second Target", got)
	}
}
