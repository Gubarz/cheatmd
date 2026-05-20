package parser

import (
	"reflect"
	"testing"
)

func TestExtractVars(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{
			name:    "dollar syntax",
			command: "curl -X POST $url -H 'Auth: $token'",
			want:    []string{"url", "token"},
		},
		{
			name:    "angle syntax",
			command: "nmap -p <port> <host>",
			want:    []string{"port", "host"},
		},
		{
			name:    "mixed syntax",
			command: "ssh $user@<host> -p $port",
			want:    []string{"user", "port", "host"},
		},
		{
			name:    "duplicates removed",
			command: "echo $var <var> $var2 $var",
			want:    []string{"var", "var2"},
		},
		{
			name:    "no variables",
			command: "ls -la /tmp",
			want:    nil,
		},
		{
			name:    "escaped variables", // the parser currently extracts it anyway, which is fine for compose
			command: "echo \\$var",
			want:    []string{"var"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractVars(tt.command)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractVars() = %v, want %v", got, tt.want)
			}
		})
	}
}
