package executor

import (
	"testing"

	"github.com/gubarz/cheatmd/pkg/parser"
)

// mockClipboard implements Clipboard interface for testing
type mockClipboard struct {
	lastCopied string
}

func (m *mockClipboard) Copy(text string) error {
	m.lastCopied = text
	return nil
}

func TestOutputWithMode_Copy(t *testing.T) {
	mockClip := &mockClipboard{}
	exec := NewExecutor(parser.NewCheatIndex()).WithClipboard(mockClip)

	testText := "echo hello"
	err := exec.OutputWithMode(testText, OutputCopy)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if mockClip.lastCopied != testText {
		t.Errorf("expected clipboard to have %q, got %q", testText, mockClip.lastCopied)
	}
}

func TestSubstituteVars_Dollar(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		scope    map[string]string
		expected string
	}{
		{
			name:     "simple substitution",
			input:    "echo $var",
			scope:    map[string]string{"var": "hello"},
			expected: "echo hello",
		},
		{
			name:     "multiple substitutions",
			input:    "curl -u $user:$pass $url",
			scope:    map[string]string{"user": "admin", "pass": "secret", "url": "http://localhost"},
			expected: "curl -u admin:secret http://localhost",
		},
		{
			name:     "prefix collision prevention",
			input:    "echo $username and $user",
			scope:    map[string]string{"user": "bob", "username": "alice"},
			expected: "echo alice and bob",
		},
		{
			name:     "missing var is left as is",
			input:    "echo $missing",
			scope:    map[string]string{"other": "val"},
			expected: "echo $missing",
		},
		{
			name:     "angle brackets ignored in dollar mode",
			input:    "echo <host>",
			scope:    map[string]string{"host": "10.0.0.1"},
			expected: "echo <host>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubstituteVars(tt.input, tt.scope, "dollar")
			if got != tt.expected {
				t.Errorf("SubstituteVars(dollar) = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSubstituteVars_Angle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		scope    map[string]string
		expected string
	}{
		{
			name:     "simple angle substitution",
			input:    "echo <host>",
			scope:    map[string]string{"host": "10.0.0.1"},
			expected: "echo 10.0.0.1",
		},
		{
			name:     "dollar ignored in angle mode",
			input:    "echo $host",
			scope:    map[string]string{"host": "10.0.0.1"},
			expected: "echo $host",
		},
		{
			name:     "multiple angle vars",
			input:    "nmap -p <port> <host>",
			scope:    map[string]string{"port": "80", "host": "10.0.0.1"},
			expected: "nmap -p 80 10.0.0.1",
		},
		{
			name:     "preserves shell vars",
			input:    "echo $HOME <target>",
			scope:    map[string]string{"target": "10.0.0.1"},
			expected: "echo $HOME 10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubstituteVars(tt.input, tt.scope, "angle")
			if got != tt.expected {
				t.Errorf("SubstituteVars(angle) = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSubstituteVars_Both(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		scope    map[string]string
		expected string
	}{
		{
			name:     "dollar and angle both replaced",
			input:    "nmap $host <port>",
			scope:    map[string]string{"host": "10.0.0.1", "port": "443"},
			expected: "nmap 10.0.0.1 443",
		},
		{
			name:     "same var in both syntaxes",
			input:    "echo $host and <host>",
			scope:    map[string]string{"host": "10.0.0.1"},
			expected: "echo 10.0.0.1 and 10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubstituteVars(tt.input, tt.scope, "both")
			if got != tt.expected {
				t.Errorf("SubstituteVars(both) = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildFinalCommand(t *testing.T) {
	cheat := &parser.Cheat{
		Command: "echo $greeting \\$HOME",
		Scope: map[string]string{
			"greeting": "hello",
		},
	}

	exec := NewExecutor(parser.NewCheatIndex())
	got := exec.BuildFinalCommand(cheat)
	want := "echo hello $HOME"

	if got != want {
		t.Errorf("BuildFinalCommand() = %q, want %q", got, want)
	}
}
