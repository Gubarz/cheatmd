package ui

import (
	"testing"

	"github.com/gubarz/cheatmd/pkg/parser"
)

func TestBuildMatchPattern(t *testing.T) {
	tests := []struct {
		name         string
		cmd          string
		wantVarNames []string
	}{
		{
			name:         "simple var",
			cmd:          "echo $name",
			wantVarNames: []string{"name"},
		},
		{
			name:         "var with double quotes",
			cmd:          `curl "$url"`,
			wantVarNames: []string{"url"},
		},
		{
			name:         "var with single quotes",
			cmd:          `ssh '$user'@host`,
			wantVarNames: []string{"user"},
		},
		{
			name:         "multiple vars",
			cmd:          "nmap -p $port $host",
			wantVarNames: []string{"port", "host"},
		},
		{
			name:         "no vars",
			cmd:          "echo hello world",
			wantVarNames: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, varNames := buildMatchPattern(tt.cmd)

			if pattern == nil && len(tt.wantVarNames) > 0 {
				t.Fatalf("buildMatchPattern() returned nil pattern, expected vars %v", tt.wantVarNames)
			}

			if len(varNames) != len(tt.wantVarNames) {
				t.Fatalf("buildMatchPattern() varNames = %v, want %v", varNames, tt.wantVarNames)
			}

			for i := range varNames {
				if varNames[i] != tt.wantVarNames[i] {
					t.Errorf("buildMatchPattern() varNames[%d] = %q, want %q", i, varNames[i], tt.wantVarNames[i])
				}
			}
		})
	}
}

func TestCheatItemMatchesQuery(t *testing.T) {
	cheat := &parser.Cheat{
		File:        "/cheats/networking/nmap.md",
		Header:      "Port Scan",
		Description: "Scan common ports",
		Command:     "nmap -sV $target",
		Tags:        []string{"recon", "pentest"},
	}
	item := newCheatItem(cheat)

	tests := []struct {
		name  string
		words []string
		want  bool
	}{
		{"matches folder", []string{"network"}, true},
		{"matches file", []string{"nmap"}, true},
		{"matches header", []string{"port"}, true},
		{"matches description", []string{"common"}, true},
		{"matches command", []string{"nmap"}, true},
		{"matches tag", []string{"pentest"}, true},
		{"matches multiple words", []string{"nmap", "port"}, true},
		{"no match", []string{"gobuster"}, false},
		{"partial multi match fails", []string{"nmap", "gobuster"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := item.matchesQuery(tt.words)
			if got != tt.want {
				t.Errorf("matchesQuery(%v) = %v, want %v", tt.words, got, tt.want)
			}
		})
	}
}
