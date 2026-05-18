package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/gubarz/cheatmd/internal/ui"
	"github.com/gubarz/cheatmd/pkg/chainstate"
	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/executor"
	"github.com/gubarz/cheatmd/pkg/linter"
	"github.com/gubarz/cheatmd/pkg/parser"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var version = "0.9.0"

var widgetCmd = &cobra.Command{
	Use:   "widget [shell]",
	Short: "Output shell widget script for integration",
	Long: `Outputs a shell script that can be sourced for shell integration.

Usage:
  eval "$(cheatmd widget bash)"

Then press Ctrl+G to trigger the cheatmd selector.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE:      runWidget,
}

var rootCmd = &cobra.Command{
	Use:          "cheatmd [path]",
	Short:        "Executable Markdown Cheatsheets",
	SilenceUsage: true,
	Long: `Command cheatsheet tool that uses real Markdown files.

Browse your cheatsheets interactively, select commands,
fill in variables, and execute or copy the result.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCheats,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.AddCommand(widgetCmd)
	rootCmd.AddCommand(chainCmd)
	rootCmd.AddCommand(dumpCmd)
	chainCmd.AddCommand(chainResetCmd)

	rootCmd.PersistentFlags().StringP("output", "o", "", "Output mode: print, copy, exec")
	rootCmd.PersistentFlags().StringP("query", "q", "", "Initial search query")
	rootCmd.PersistentFlags().StringP("match", "m", "", "Match command and pre-select if found")
	rootCmd.PersistentFlags().Bool("print", false, "Print command (shorthand for -o print)")
	rootCmd.PersistentFlags().Bool("copy", false, "Copy command (shorthand for -o copy)")
	rootCmd.PersistentFlags().Bool("exec", false, "Execute command (shorthand for -o exec)")
	rootCmd.PersistentFlags().Bool("auto", false, "Auto-select if query matches exactly one result")
	rootCmd.PersistentFlags().BoolP("benchmark", "b", false, "Benchmark load time and exit")
	rootCmd.PersistentFlags().Bool("history", false, "Open the execution history picker")
	rootCmd.PersistentFlags().Bool("lint", false, "Lint cheats and exit")
	rootCmd.PersistentFlags().Bool("strict", false, "Treat lint warnings as errors")

	viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))

	dumpCmd.Flags().Bool("csv", false, "Dump cheats as CSV")
	dumpCmd.Flags().Bool("json", false, "Dump cheats as JSON")
}

var chainCmd = &cobra.Command{
	Use:   "chain",
	Short: "Manage chain progress",
}

var chainResetCmd = &cobra.Command{
	Use:   "reset [name]",
	Short: "Reset chain progress",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runChainReset,
}

var dumpCmd = &cobra.Command{
	Use:   "dump [path]",
	Short: "Dump parsed cheat metadata",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDump,
}

type dumpEntry struct {
	Filename    string    `json:"filename"`
	Tags        []string  `json:"tags"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Command     string    `json:"command"`
	ChainName   string    `json:"chain_name,omitempty"`
	ChainStep   int       `json:"chain_step,omitempty"`
	Variables   []dumpVar `json:"variables"`
}

type dumpVar struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Value     string `json:"value,omitempty"`
	Args      string `json:"args,omitempty"`
	Condition string `json:"condition,omitempty"`
}

func runDump(cmd *cobra.Command, args []string) error {
	useCSV, _ := cmd.Flags().GetBool("csv")
	useJSON, _ := cmd.Flags().GetBool("json")
	if useCSV && useJSON {
		return fmt.Errorf("choose only one dump format: --csv or --json")
	}
	if !useCSV && !useJSON {
		useJSON = true
	}

	index, err := parseDumpIndex(args)
	if err != nil {
		return err
	}

	entries := make([]dumpEntry, 0, len(index.Cheats))
	for _, cheat := range index.Cheats {
		entries = append(entries, dumpEntry{
			Filename:    cheat.File,
			Tags:        cheat.Tags,
			Title:       cheat.Header,
			Description: cheat.Description,
			Command:     cheat.Command,
			ChainName:   cheat.ChainName,
			ChainStep:   cheat.ChainStep,
			Variables:   dumpVars(cheat.Vars),
		})
	}

	if useCSV {
		return writeDumpCSV(cmd, entries)
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(entries)
}

func dumpVars(vars []parser.VarDef) []dumpVar {
	out := make([]dumpVar, 0, len(vars))
	for _, v := range vars {
		dv := dumpVar{
			Name:      v.Name,
			Args:      v.Args,
			Condition: v.Condition,
		}
		switch {
		case v.Literal != "":
			dv.Kind = "literal"
			dv.Value = v.Literal
		case v.Shell != "":
			dv.Kind = "shell"
			dv.Value = v.Shell
		default:
			dv.Kind = "prompt"
		}
		out = append(out, dv)
	}
	return out
}

func parseDumpIndex(args []string) (*parser.CheatIndex, error) {
	path := "."
	if len(args) > 0 {
		path = args[0]
	} else if config.GetPath() != "." {
		path = config.GetPath()
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("error resolving path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("path error: %w", err)
	}
	p := parser.NewParser()
	if info.IsDir() {
		return p.ParseDirectory(absPath)
	}
	return p.ParseSingleFile(absPath)
}

func writeDumpCSV(cmd *cobra.Command, entries []dumpEntry) error {
	w := csv.NewWriter(cmd.OutOrStdout())
	if err := w.Write([]string{"filename", "tags", "title", "description", "command", "chain_name", "chain_step", "variables"}); err != nil {
		return err
	}
	for _, entry := range entries {
		stepStr := ""
		if entry.ChainStep > 0 {
			stepStr = strconv.Itoa(entry.ChainStep)
		}
		if err := w.Write([]string{
			entry.Filename,
			strings.Join(entry.Tags, "|"),
			entry.Title,
			entry.Description,
			entry.Command,
			entry.ChainName,
			stepStr,
			formatDumpVars(entry.Variables),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func formatDumpVars(vars []dumpVar) string {
	parts := make([]string, 0, len(vars))
	for _, v := range vars {
		part := v.Name + ":" + v.Kind
		if v.Value != "" {
			part += "=" + v.Value
		}
		if v.Args != "" {
			part += " --- " + v.Args
		}
		if v.Condition != "" {
			part += " if " + v.Condition
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "|")
}

func runChainReset(cmd *cobra.Command, args []string) error {
	root := "."
	if config.GetPath() != "." {
		root = config.GetPath()
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("error resolving path: %w", err)
	}

	statePath, err := chainstate.DefaultPath()
	if err != nil {
		return err
	}
	state, err := chainstate.Load(statePath)
	if err != nil {
		return err
	}

	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	chainstate.Clear(absRoot, name, state)
	if err := chainstate.Save(statePath, state); err != nil {
		return err
	}

	if name == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Cleared chain progress.")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Cleared chain %q.\n", name)
	}
	return nil
}

func initConfig() {
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
	}
}

func runWidget(cmd *cobra.Command, args []string) error {
	shell := args[0]

	switch shell {
	case "bash":
		fmt.Print(bashWidget())
	case "zsh":
		fmt.Print(zshWidget())
	case "fish":
		fmt.Print(fishWidget())
	default:
		return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
	}
	return nil
}

func bashWidget() string {
	keyWidget := config.GetKeyWidget()
	return fmt.Sprintf(`#!/usr/bin/env bash

_cheatmd_widget() {
   local -r input="${READLINE_LINE}"

   local output
   if [ -z "${input}" ]; then
      output="$(cheatmd --print)" || return
   else
      output="$(cheatmd --print --match "$input")" || return
   fi

   if [ -n "$output" ]; then
      READLINE_LINE="$output"
      READLINE_POINT=${#READLINE_LINE}
   fi
}

if [ ${BASH_VERSION:0:1} -lt 4 ]; then
   echo "cheatmd widget requires bash 4+" >&2
else
   bind -x '"%s": _cheatmd_widget'
fi
`, keyWidget)
}

func zshWidget() string {
	keyWidget := config.GetKeyWidget()
	// Convert bash-style keybinding to zsh format (e.g., \C-g -> ^g)
	zshKey := convertToZshKey(keyWidget)
	return fmt.Sprintf(`#!/usr/bin/env zsh

_cheatmd_widget() {
   local input="$BUFFER"

   local output
   if [ -z "$input" ]; then
      output="$(cheatmd --print)" || return
   else
      output="$(cheatmd --print --match "$input")" || return
   fi

   if [ -n "$output" ]; then
      BUFFER="$output"
      CURSOR=${#BUFFER}
   fi

   zle reset-prompt
}

zle -N _cheatmd_widget
bindkey '%s' _cheatmd_widget
`, zshKey)
}

func fishWidget() string {
	keyWidget := config.GetKeyWidget()
	// Convert bash-style keybinding to fish format (e.g., \C-g -> \cg)
	fishKey := convertToFishKey(keyWidget)
	return fmt.Sprintf(`function _cheatmd_widget
   set -l input (commandline)
   set -l output
   set -l cmd_status 0

   if test -z "$input"
      set output (cheatmd --print)
      set cmd_status $status
   else
      set output (cheatmd --print --match "$input")
      set cmd_status $status
   end

   if test $cmd_status -ne 0
      return
   end

   if test -n "$output"
      commandline -r "$output"
      commandline -f end-of-line
   end

   commandline -f repaint
end

bind %s _cheatmd_widget
`, fishKey)
}

// convertToZshKey converts a bash-style keybinding to zsh format
// e.g., \C-g -> ^g, \C-x -> ^x
func convertToZshKey(key string) string {
	if strings.HasPrefix(key, "\\C-") {
		return "^" + strings.ToLower(key[3:])
	}
	// Already in zsh format or other format
	return key
}

// convertToFishKey converts a bash-style keybinding to fish format
// e.g., \C-g -> \cg, \C-x -> \cx
func convertToFishKey(key string) string {
	if strings.HasPrefix(key, "\\C-") {
		return "\\c" + strings.ToLower(key[3:])
	}
	// Already in fish format or other format
	return key
}

func runCheats(cmd *cobra.Command, args []string) error {
	// Determine path
	path := "."
	if len(args) > 0 {
		path = args[0]
	} else if config.GetPath() != "." {
		path = config.GetPath()
	}

	// Handle output mode flags
	if p, _ := cmd.Flags().GetBool("print"); p {
		config.SetOutput("print")
	} else if c, _ := cmd.Flags().GetBool("copy"); c {
		config.SetOutput("copy")
	} else if e, _ := cmd.Flags().GetBool("exec"); e {
		config.SetOutput("exec")
	} else if o, _ := cmd.Flags().GetString("output"); o != "" {
		config.SetOutput(o)
	}

	if auto, _ := cmd.Flags().GetBool("auto"); auto {
		config.SetAutoSelect(true)
	}

	query, _ := cmd.Flags().GetString("query")
	match, _ := cmd.Flags().GetString("match")

	// Resolve path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("error resolving path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path error: %w", err)
	}

	lintFlag, _ := cmd.Flags().GetBool("lint")
	if lintFlag {
		return runLint(cmd, absPath)
	}

	// Parse markdown files
	benchmark, _ := cmd.Flags().GetBool("benchmark")

	start := time.Now()

	p := parser.NewParser()
	var index *parser.CheatIndex

	if info.IsDir() {
		index, err = p.ParseDirectory(absPath)
	} else {
		index, err = p.ParseSingleFile(absPath)
	}

	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	// Check for duplicate exports
	if len(index.Duplicates) > 0 {
		fmt.Fprintln(os.Stderr, "Warning: duplicate exports found:")
		for _, dup := range index.Duplicates {
			fmt.Fprintf(os.Stderr, "  export %q defined in:\n    - %s\n    - %s\n", dup.Name, dup.File1, dup.File2)
		}
		fmt.Fprintln(os.Stderr)
	}

	if len(index.Cheats) == 0 {
		return fmt.Errorf("no cheats found in %s", absPath)
	}

	// Create executor
	exec := executor.NewExecutor(index)

	if benchmark {
		elapsed := time.Since(start)
		// Force GC and get memory stats
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Printf("Loaded %d cheats in %v\n", len(index.Cheats), elapsed)
		fmt.Printf("Memory: Alloc=%dMB, TotalAlloc=%dMB, Sys=%dMB, HeapObjects=%d\n",
			m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.HeapObjects)
		return nil
	}

	// Run the TUI (history view if --history was passed)
	if historyFlag, _ := cmd.Flags().GetBool("history"); historyFlag {
		return ui.RunHistory(index, exec)
	}
	return ui.Run(index, exec, query, match)
}

func runLint(cmd *cobra.Command, path string) error {
	findings, err := linter.Lint(path)
	if err != nil {
		return fmt.Errorf("lint error: %w", err)
	}

	useColor := stdoutIsTerminal()
	hasErrors := false
	hasWarnings := false
	for _, f := range findings {
		if f.Severity == linter.SeverityError {
			hasErrors = true
		}
		if f.Severity == linter.SeverityWarning {
			hasWarnings = true
		}
		fmt.Fprintln(cmd.OutOrStdout(), formatLintFinding(f, useColor))
	}

	if len(findings) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No lint findings.")
		return nil
	}

	strict, _ := cmd.Flags().GetBool("strict")
	if hasErrors || (strict && hasWarnings) {
		return fmt.Errorf("lint failed with %d finding(s)", len(findings))
	}
	return nil
}

func formatLintFinding(f linter.Finding, color bool) string {
	if !color {
		return f.Format()
	}
	severity := f.Severity.String()
	switch f.Severity {
	case linter.SeverityError:
		severity = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render(severity)
	default:
		severity = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render(severity)
	}

	line := f.Line
	if line < 1 {
		line = 1
	}
	col := f.Column
	if col < 1 {
		col = 1
	}
	return fmt.Sprintf("%s:%d:%d: %s: %s", f.File, line, col, severity, f.Message)
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func main() {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
