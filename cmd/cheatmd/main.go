package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/executor"
	"github.com/gubarz/cheatmd/internal/parser"
	"github.com/gubarz/cheatmd/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var version = "0.1.4"

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
	Use:   "cheatmd [path]",
	Short: "Executable Markdown Cheatsheets",
	Long: `Command cheatsheet tool that uses real Markdown files.

Browse your cheatsheets interactively, select commands,
fill in variables, and execute or copy the result.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCheats,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.AddCommand(widgetCmd)

	rootCmd.PersistentFlags().StringP("output", "o", "", "Output mode: print, copy, exec")
	rootCmd.PersistentFlags().StringP("query", "q", "", "Initial search query")
	rootCmd.PersistentFlags().StringP("match", "m", "", "Match command and pre-select if found")
	rootCmd.PersistentFlags().Bool("print", false, "Print command (shorthand for -o print)")
	rootCmd.PersistentFlags().Bool("copy", false, "Copy command (shorthand for -o copy)")
	rootCmd.PersistentFlags().Bool("exec", false, "Execute command (shorthand for -o exec)")
	rootCmd.PersistentFlags().Bool("auto", false, "Auto-select if query matches exactly one result")
	rootCmd.PersistentFlags().BoolP("benchmark", "b", false, "Benchmark load time and exit")

	viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
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
	return `#!/usr/bin/env bash

_cheatmd_widget() {
   local -r input="${READLINE_LINE}"

   local output
   if [ -z "${input}" ]; then
      output="$(cheatmd --print)"
   else
      output="$(cheatmd --print --match "$input")"
   fi

   if [ -n "$output" ]; then
      READLINE_LINE="$output"
      READLINE_POINT=${#READLINE_LINE}
   fi
}

if [ ${BASH_VERSION:0:1} -lt 4 ]; then
   echo "cheatmd widget requires bash 4+" >&2
else
   bind -x '"\C-g": _cheatmd_widget'
fi
`
}

func zshWidget() string {
	return `#!/usr/bin/env zsh

_cheatmd_widget() {
   local input="$BUFFER"

   local output
   if [ -z "$input" ]; then
      output="$(cheatmd --print)"
   else
      output="$(cheatmd --print --match "$input")"
   fi

   if [ -n "$output" ]; then
      BUFFER="$output"
      CURSOR=${#BUFFER}
   fi

   zle reset-prompt
}

zle -N _cheatmd_widget
bindkey '^g' _cheatmd_widget
`
}

func fishWidget() string {
	return `function _cheatmd_widget
   set -l input (commandline)

   if test -z "$input"
      set output (cheatmd --print)
   else
      set output (cheatmd --print --match "$input")
   end

   if test -n "$output"
      commandline -r "$output"
      commandline -f end-of-line
   end

   commandline -f repaint
end

bind \cg _cheatmd_widget
`
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

	// Run the TUI
	return ui.Run(index, exec, query, match)
}

func main() {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
