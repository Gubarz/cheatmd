package ui

import (
	"reflect"
	"testing"

	"github.com/gubarz/cheatmd/pkg/parser"
)

func TestParseShellArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "double quoted delimiter",
			input: `--delimiter "\t" --column 2`,
			want:  []string{"--delimiter", `\t`, "--column", "2"},
		},
		{
			name:  "single quoted",
			input: `--delimiter '\t' --column 1`,
			want:  []string{"--delimiter", `\t`, "--column", "1"},
		},
		{
			name:  "double quoted with space",
			input: `--header "Pick a host" --column 1`,
			want:  []string{"--header", "Pick a host", "--column", "1"},
		},
		{
			name:  "no args",
			input: "",
			want:  nil,
		},
		{
			name:  "extra whitespace",
			input: `  --delimiter   ","  `,
			want:  []string{"--delimiter", ","},
		},
		{
			name:  "map command",
			input: `--map "awk '{print $1}'"`,
			want:  []string{"--map", "awk '{print $1}'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseShellArgs(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseShellArgs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSelectorOpts(t *testing.T) {
	tests := []struct {
		name string
		args string
		want SelectOptions
	}{
		{
			name: "delimiter and column",
			args: `--delimiter "\t" --column 2`,
			want: SelectOptions{Delimiter: `\t`, Column: 2},
		},
		{
			name: "all options",
			args: `--delimiter "," --column 2 --select-column 1 --map "cut -d: -f1"`,
			want: SelectOptions{Delimiter: ",", Column: 2, SelectColumn: 1, MapCmd: "cut -d: -f1"},
		},
		{
			name: "header is ignored in SelectOptions",
			args: `--header "Pick one" --delimiter ":"`,
			want: SelectOptions{Delimiter: ":"},
		},
		{
			name: "empty args",
			args: "",
			want: SelectOptions{},
		},
		{
			name: "select-column only",
			args: `--select-column 3`,
			want: SelectOptions{SelectColumn: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSelectorOpts(tt.args)
			if got != tt.want {
				t.Errorf("parseSelectorOpts(%q) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestGetDisplayColumn(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		delimiter string
		column    int
		want      string
	}{
		{
			name:      "tab delimited column 2",
			line:      "192.168.1.1\twebserver\tlinux",
			delimiter: "\t",
			column:    2,
			want:      "webserver",
		},
		{
			name:      "comma delimited column 1",
			line:      "primary,blue,active",
			delimiter: ",",
			column:    1,
			want:      "primary",
		},
		{
			name:      "column out of range returns full line",
			line:      "one\ttwo",
			delimiter: "\t",
			column:    5,
			want:      "one\ttwo",
		},
		{
			name:      "column 0 returns full line",
			line:      "one\ttwo\tthree",
			delimiter: "\t",
			column:    0,
			want:      "one\ttwo\tthree",
		},
		{
			name:      "empty delimiter returns full line",
			line:      "one\ttwo",
			delimiter: "",
			column:    2,
			want:      "one\ttwo",
		},
		{
			name:      "trims whitespace",
			line:      "one\t  two  \tthree",
			delimiter: "\t",
			column:    2,
			want:      "two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDisplayColumn(tt.line, tt.delimiter, tt.column)
			if got != tt.want {
				t.Errorf("getDisplayColumn(%q, %q, %d) = %q, want %q", tt.line, tt.delimiter, tt.column, got, tt.want)
			}
		})
	}
}

func TestApplyMapTransform_SelectColumn(t *testing.T) {
	// Test select-column extraction (without --map, which requires shell)
	tests := []struct {
		name  string
		value string
		opts  SelectOptions
		want  string
	}{
		{
			name:  "extract column 1 from tab-delimited",
			value: "192.168.1.1\twebserver\tlinux",
			opts:  SelectOptions{Delimiter: "\t", SelectColumn: 1},
			want:  "192.168.1.1",
		},
		{
			name:  "extract column 2 from comma-delimited",
			value: "admin,secret,active",
			opts:  SelectOptions{Delimiter: ",", SelectColumn: 2},
			want:  "secret",
		},
		{
			name:  "column out of range returns original",
			value: "one\ttwo",
			opts:  SelectOptions{Delimiter: "\t", SelectColumn: 10},
			want:  "one\ttwo",
		},
		{
			name:  "no select-column returns original",
			value: "one\ttwo\tthree",
			opts:  SelectOptions{Delimiter: "\t", SelectColumn: 0},
			want:  "one\ttwo\tthree",
		},
		{
			name:  "no delimiter returns original",
			value: "one\ttwo",
			opts:  SelectOptions{Delimiter: "", SelectColumn: 1},
			want:  "one\ttwo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyMapTransform(tt.value, tt.opts)
			if got != tt.want {
				t.Errorf("applyMapTransform(%q, %+v) = %q, want %q", tt.value, tt.opts, got, tt.want)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple lines",
			input: "one\ntwo\nthree",
			want:  []string{"one", "two", "three"},
		},
		{
			name:  "empty lines filtered",
			input: "one\n\ntwo\n\n\nthree\n",
			want:  []string{"one", "two", "three"},
		},
		{
			name:  "whitespace trimmed",
			input: "  one  \n\ttwo\t\n  three  ",
			want:  []string{"one", "two", "three"},
		},
		{
			name:  "carriage returns handled",
			input: "one\r\ntwo\r\nthree\r\n",
			want:  []string{"one", "two", "three"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.SplitLines(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitLines(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestEndToEnd_DelimiterColumnPipeline tests the full pipeline:
// shell output -> splitLines -> parseSelectorOpts -> getDisplayColumn -> applyMapTransform
func TestEndToEnd_DelimiterColumnPipeline(t *testing.T) {
	// Simulate: var host = echo -e "192.168.1.1,webserver\n10.0.0.1,db" --- --delimiter "," --column 2 --select-column 1
	shellOutput := "192.168.1.1,webserver\n10.0.0.1,db"
	selectorArgs := `--delimiter "," --column 2 --select-column 1`

	// 1. Split shell output into lines
	lines := parser.SplitLines(shellOutput)
	if len(lines) != 2 {
		t.Fatalf("splitLines() = %d lines, want 2", len(lines))
	}

	// 2. Parse selector options
	opts := parseSelectorOpts(selectorArgs)
	if opts.Delimiter != "," {
		t.Errorf("opts.Delimiter = %q, want %q", opts.Delimiter, ",")
	}
	if opts.Column != 2 {
		t.Errorf("opts.Column = %d, want 2", opts.Column)
	}
	if opts.SelectColumn != 1 {
		t.Errorf("opts.SelectColumn = %d, want 1", opts.SelectColumn)
	}

	// 3. Display column (what the user sees in the picker)
	display0 := getDisplayColumn(lines[0], opts.Delimiter, opts.Column)
	display1 := getDisplayColumn(lines[1], opts.Delimiter, opts.Column)
	if display0 != "webserver" {
		t.Errorf("display[0] = %q, want %q", display0, "webserver")
	}
	if display1 != "db" {
		t.Errorf("display[1] = %q, want %q", display1, "db")
	}

	// 4. Select column (the value that gets substituted into the command)
	// User picks line 0 -> applyMapTransform extracts select-column 1
	selected := applyMapTransform(lines[0], opts)
	if selected != "192.168.1.1" {
		t.Errorf("applyMapTransform() = %q, want %q", selected, "192.168.1.1")
	}

	// User picks line 1
	selected2 := applyMapTransform(lines[1], opts)
	if selected2 != "10.0.0.1" {
		t.Errorf("applyMapTransform() = %q, want %q", selected2, "10.0.0.1")
	}
}
