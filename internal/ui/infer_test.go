package ui

import (
	"testing"

	"github.com/gubarz/cheatmd/internal/parser"
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
			template: "-p $credential",
			actual:   "-p mypassword",
			scope:    map[string]string{},
			expected: map[string]string{"credential": "mypassword"},
		},
		{
			name:     "extraction with colon",
			template: "-p $credential",
			actual:   "-p :mypassword",
			scope:    map[string]string{},
			expected: map[string]string{"credential": ":mypassword"},
		},
		{
			name:     "hash extraction",
			template: "-H $credential",
			actual:   "-H aad3b435b51404eeaad3b435b51404ee:abc123",
			scope:    map[string]string{},
			expected: map[string]string{"credential": "aad3b435b51404eeaad3b435b51404ee:abc123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractEmbeddedVars(tt.template, tt.actual, tt.scope)

			for key, expected := range tt.expected {
				if actual, ok := result[key]; !ok {
					t.Errorf("expected result[%q] = %q, but key not found", key, expected)
				} else if actual != expected {
					t.Errorf("expected result[%q] = %q, got %q", key, expected, actual)
				}
			}
		})
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
			name:    "auth_flags extraction with -k",
			command: "bloodyad --host $dc_ip -d $domain $auth_flags",
			input:   "bloodyad --host 10.0.0.1 -d test.local -k",
			expectedScope: map[string]string{
				"dc_ip":      "10.0.0.1",
				"domain":     "test.local",
				"auth_flags": "-k",
			},
		},
		{
			name:    "auth_flags extraction with -p password",
			command: "bloodyad --host $dc_ip -d $domain $auth_flags",
			input:   "bloodyad --host 10.0.0.1 -d test.local -p mypassword",
			expectedScope: map[string]string{
				"dc_ip":      "10.0.0.1",
				"domain":     "test.local",
				"auth_flags": "-p mypassword",
			},
		},
		{
			name:    "full bloodyAD command with mid-command auth_flags",
			command: "bloodyAD --host $rhost_name -d $domain -u $user $auth_flags add badSuccessor $rhost_name",
			input:   "bloodyAD --host test -d bacon.htb -u Administrator -p 123 add badSuccessor test",
			expectedScope: map[string]string{
				"rhost_name": "test",
				"domain":     "bacon.htb",
				"user":       "Administrator",
				"auth_flags": "-p 123",
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

			for key, expected := range tt.expectedScope {
				if actual, ok := cheat.Scope[key]; !ok {
					t.Errorf("expected scope[%q] = %q, but key not found. scope=%v", key, expected, cheat.Scope)
				} else if actual != expected {
					t.Errorf("expected scope[%q] = %q, got %q", key, expected, actual)
				}
			}
		})
	}
}

func TestInferDependentVars(t *testing.T) {
	index := &parser.CheatIndex{
		Modules: map[string]*parser.Module{
			"bloodyad": {
				Name: "bloodyad",
				Vars: []parser.VarDef{
					{Name: "auth_method", Shell: "echo -e 'kerberos\npassword\nhash'"},
					{Name: "auth_flags", Literal: "-k", Condition: "$auth_method == kerberos"},
					{Name: "auth_flags", Literal: "-p $credential", Condition: "$auth_method == password"},
					{Name: "auth_flags", Literal: "-H $credential", Condition: "$auth_method == hash"},
					{Name: "credential", Shell: ""},
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
			name:         "kerberos flag should infer auth_method",
			initialScope: map[string]string{"auth_flags": "-k"},
			expectedScope: map[string]string{
				"auth_flags":  "-k",
				"auth_method": "kerberos",
			},
			imports: []string{"bloodyad"},
		},
		{
			name:         "password flag should infer auth_method and credential",
			initialScope: map[string]string{"auth_flags": "-p mypassword"},
			expectedScope: map[string]string{
				"auth_flags":  "-p mypassword",
				"auth_method": "password",
				"credential":  "mypassword",
			},
			imports: []string{"bloodyad"},
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

			for key, expected := range tt.expectedScope {
				if actual, ok := cheat.Scope[key]; !ok {
					t.Errorf("expected scope[%q] = %q, but key not found. scope=%v", key, expected, cheat.Scope)
				} else if actual != expected {
					t.Errorf("expected scope[%q] = %q, got %q", key, expected, actual)
				}
			}
		})
	}
}
