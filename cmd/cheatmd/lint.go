package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/gubarz/cheatmd/pkg/linter"
	"github.com/spf13/cobra"
)

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
