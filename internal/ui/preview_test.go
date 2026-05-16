package ui

import (
	"testing"

	"github.com/gubarz/cheatmd/pkg/parser"
)

func TestFindCheatHeaderSourceLineSkipsSameNamedPageHeader(t *testing.T) {
	raw := `# Responder

Overview prose.

<!-- cheat
export interface
var interface
-->

## Responder

` + "```sh" + `
sudo responder -I $interface
` + "```" + `
<!-- cheat
import interface
-->
`

	line := findCheatHeaderSourceLine(raw, &parser.Cheat{
		Header:  "Responder",
		Command: "sudo responder -I $interface",
	})
	if line != 9 {
		t.Fatalf("expected executable cheat header line 9, got %d", line)
	}
}

func TestFindCheatHeaderSourceLineSupportsPlainCodeFenceCheats(t *testing.T) {
	raw := `# Notes

## Plain

` + "```sh" + `
whoami
` + "```" + `
`

	line := findCheatHeaderSourceLine(raw, &parser.Cheat{
		Header:  "Plain",
		Command: "whoami",
	})
	if line != 2 {
		t.Fatalf("expected plain code fence header line 2, got %d", line)
	}
}
