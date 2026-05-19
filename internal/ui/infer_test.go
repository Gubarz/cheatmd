package ui

import (
	"testing"

	"github.com/gubarz/cheatmd/pkg/parser"
)

func TestExtractEmbeddedVars(t *testing.T) {
	tests := []struct {
		name     string
		template string
		actual   string
		scope    map[string]string
		expected map[string]string
	}{
		{
			name:     "simple extraction",
			template: "--value $setting",
			actual:   "--value blue",
			scope:    map[string]string{},
			expected: map[string]string{"setting": "blue"},
		},
		{
			name:     "extraction with colon",
			template: "--value $setting",
			actual:   "--value :blue",
			scope:    map[string]string{},
			expected: map[string]string{"setting": ":blue"},
		},
		{
			name:     "compound value extraction",
			template: "--value $setting",
			actual:   "--value alpha:beta",
			scope:    map[string]string{},
			expected: map[string]string{"setting": "alpha:beta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractEmbeddedVars(tt.template, tt.actual, tt.scope)

			assertMapEq(t, tt.expected, result)
		})
	}
}

func assertMapEq(t *testing.T, expected, actual map[string]string) {
	t.Helper()
	for key, exp := range expected {
		if act, ok := actual[key]; !ok {
			t.Errorf("expected map[%q] = %q, but key not found. map=%v", key, exp, actual)
		} else if act != exp {
			t.Errorf("expected map[%q] = %q, got %q", key, exp, act)
		}
	}
}

func TestPrefillScopeFromMatch(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		input         string
		expectedScope map[string]string
	}{
		{
			name:    "mode_flags extraction with dry run",
			command: "deployctl --server $server -p $project $mode_flags",
			input:   "deployctl --server app01 -p website --dry-run",
			expectedScope: map[string]string{
				"server":     "app01",
				"project":    "website",
				"mode_flags": "--dry-run",
			},
		},
		{
			name:    "mode_flags extraction with labeled value",
			command: "deployctl --server $server -p $project $mode_flags",
			input:   "deployctl --server app01 -p website --tag stable",
			expectedScope: map[string]string{
				"server":     "app01",
				"project":    "website",
				"mode_flags": "--tag stable",
			},
		},
		{
			name:    "full deploy command with mid-command flags",
			command: "deployctl --server $server -p $project -u $user $mode_flags apply service $server",
			input:   "deployctl --server app01 -p website -u alice --tag stable apply service app01",
			expectedScope: map[string]string{
				"server":     "app01",
				"project":    "website",
				"user":       "alice",
				"mode_flags": "--tag stable",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cheat := &parser.Cheat{
				Command: tt.command,
				Scope:   make(map[string]string),
			}

			pattern, varNames := buildMatchPattern(cheat.Command)
			t.Logf("pattern=%s varNames=%v", pattern, varNames)
			t.Logf("input=%q", tt.input)
			if matches := pattern.FindStringSubmatch(tt.input); matches != nil {
				t.Logf("matches=%v", matches)
			} else {
				t.Logf("NO MATCH")
			}

			prefillScopeFromMatch(cheat, tt.input)

			assertMapEq(t, tt.expectedScope, cheat.Scope)
		})
	}
}

func TestFindMatchingCheatPrefersSpecificCommandOverVarOnly(t *testing.T) {
	input := "make deploy SERVICE=api ENV=prod LOG=run-$(date +%Y%m%d-%H%M%S).txt"
	varOnly := &parser.Cheat{
		Header:  "Single value",
		Command: "$item_name",
	}
	deploy := &parser.Cheat{
		Header:  "Deploy service",
		Command: "make deploy SERVICE=$service ENV=$env LOG=run-$(date +%Y%m%d-%H%M%S).txt",
	}

	got := findMatchingCheat([]*parser.Cheat{varOnly, deploy}, input)
	if got != deploy {
		t.Fatalf("matched %q, want %q", got.Header, deploy.Header)
	}

	prefillScopeFromMatch(got, input)
	if got.Scope["service"] != "api" {
		t.Fatalf("service = %q, want api", got.Scope["service"])
	}
	if got.Scope["env"] != "prod" {
		t.Fatalf("env = %q, want prod", got.Scope["env"])
	}
}

func TestFindMatchingCheatDoesNotUseVarOnlyAsCatchAll(t *testing.T) {
	got := findMatchingCheat([]*parser.Cheat{
		{Header: "Single value", Command: "$item_name"},
	}, "tool run thing --flag value")
	if got != nil {
		t.Fatalf("matched %q, want nil", got.Header)
	}
}

func TestInferDependentVars(t *testing.T) {
	index := &parser.CheatIndex{
		Modules: map[string]*parser.Module{
			"deploy": {
				Name: "deploy",
				Vars: []parser.VarDef{
					{Name: "run_mode", Shell: "printf 'preview\npublish\narchive\n'"},
					{Name: "mode_flags", Literal: "--dry-run", Condition: "$run_mode == preview"},
					{Name: "mode_flags", Literal: "--tag $label", Condition: "$run_mode == publish"},
					{Name: "mode_flags", Literal: "--archive $label", Condition: "$run_mode == archive"},
					{Name: "label", Shell: ""},
				},
			},
		},
	}

	tests := []struct {
		name          string
		initialScope  map[string]string
		expectedScope map[string]string
		imports       []string
	}{
		{
			name:         "preview flag should infer run_mode",
			initialScope: map[string]string{"mode_flags": "--dry-run"},
			expectedScope: map[string]string{
				"mode_flags": "--dry-run",
				"run_mode":   "preview",
			},
			imports: []string{"deploy"},
		},
		{
			name:         "publish flag should infer run_mode and label",
			initialScope: map[string]string{"mode_flags": "--tag stable"},
			expectedScope: map[string]string{
				"mode_flags": "--tag stable",
				"run_mode":   "publish",
				"label":      "stable",
			},
			imports: []string{"deploy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cheat := &parser.Cheat{
				Scope:   make(map[string]string),
				Imports: tt.imports,
			}
			for k, v := range tt.initialScope {
				cheat.Scope[k] = v
			}

			inferDependentVars(cheat, index)

			assertMapEq(t, tt.expectedScope, cheat.Scope)
		})
	}
}
