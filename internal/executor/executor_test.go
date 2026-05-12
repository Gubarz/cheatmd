package executor

import (
	"testing"

	"github.com/gubarz/cheatmd/internal/parser"
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

func TestSubstituteVars(t *testing.T) {
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
			expected: "echo alice and bob", // longest match first prevents $user replacing start of $username
		},
		{
			name:     "missing var is left as is",
			input:    "echo $missing",
			scope:    map[string]string{"other": "val"},
			expected: "echo $missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubstituteVars(tt.input, tt.scope)
			if got != tt.expected {
				t.Errorf("SubstituteVars() = %q, want %q", got, tt.expected)
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
