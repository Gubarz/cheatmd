package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/parser"
	"github.com/spf13/cobra"
)

var dumpCmd = &cobra.Command{
	Use:   "dump [path]",
	Short: "Dump parsed cheat metadata",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDump,
}

func init() {
	dumpCmd.Flags().Bool("csv", false, "Dump cheats as CSV")
	dumpCmd.Flags().Bool("json", false, "Dump cheats as JSON")
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
