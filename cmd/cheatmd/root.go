package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gubarz/cheatmd/internal/ui"
	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/executor"
	"github.com/gubarz/cheatmd/pkg/parser"
	"github.com/spf13/cobra"
)

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

	rootCmd.PersistentFlags().StringP("query", "q", "", "Initial search query")
	rootCmd.PersistentFlags().StringP("match", "m", "", "Match a full command pattern and auto-fill its variables")
	rootCmd.PersistentFlags().BoolP("print", "p", false, "Print command (default)")
	rootCmd.PersistentFlags().BoolP("copy", "c", false, "Copy command")
	rootCmd.PersistentFlags().BoolP("exec", "e", false, "Execute command")
	rootCmd.PersistentFlags().BoolP("auto", "a", false, "Auto-select if query matches exactly one result")
	rootCmd.PersistentFlags().BoolP("benchmark", "b", false, "Benchmark load time and exit")
	rootCmd.PersistentFlags().BoolP("history", "H", false, "Open the execution history picker")
	rootCmd.PersistentFlags().BoolP("lint", "l", false, "Lint cheats and exit")
	rootCmd.PersistentFlags().BoolP("strict", "s", false, "Treat lint warnings as errors")
}

func initConfig() {
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
	}
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
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: duplicate exports found:")
		for _, dup := range index.Duplicates {
			fmt.Fprintf(cmd.ErrOrStderr(), "  export %q defined in:\n    - %s\n    - %s\n", dup.Name, dup.File1, dup.File2)
		}
		fmt.Fprintln(cmd.ErrOrStderr())
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
		fmt.Fprintf(cmd.OutOrStdout(), "Loaded %d cheats in %v\n", len(index.Cheats), elapsed)
		fmt.Fprintf(cmd.OutOrStdout(), "Memory: Alloc=%dMB, TotalAlloc=%dMB, Sys=%dMB, HeapObjects=%d\n",
			m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.HeapObjects)
		return nil
	}

	// Run the TUI (history view if --history was passed)
	var finalCmd string
	if historyFlag, _ := cmd.Flags().GetBool("history"); historyFlag {
		finalCmd, err = ui.RunHistory(index, exec)
	} else {
		finalCmd, err = ui.Run(index, exec, query, match)
	}

	if err != nil {
		return err
	}
	if finalCmd == "" {
		return nil
	}

	// Apply hooks
	if preHook := config.GetPreHook(); preHook != "" {
		finalCmd = preHook + finalCmd
	}
	if postHook := config.GetPostHook(); postHook != "" {
		finalCmd = finalCmd + postHook
	}

	switch config.GetOutput() {
	case "exec":
		fmt.Fprint(cmd.ErrOrStderr(), finalCmd)
		return exec.OutputWithMode(finalCmd, executor.OutputExec)
	case "copy":
		if err := exec.OutputWithMode(finalCmd, executor.OutputCopy); err != nil {
			return err
		}
		return nil
	default: // print
		fmt.Fprint(cmd.OutOrStdout(), finalCmd)
		return nil
	}
}
