package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/parser"
)

var composeCmd = &cobra.Command{
	Use:   "compose [command]",
	Short: "Compose a new cheat template from a raw command line",
	Long: `Quickly templatize a shell command you just used into a CheatMD snippet.
It automatically extracts any $var or <var> variables and writes a complete cheat block.

Examples:
  cheatmd compose -n "My Cheat" "curl -X POST <url> -H 'Auth: <token>'"
  echo "ssh -p $port $user@$host" | cheatmd compose -f ~/cheats.md`,
	RunE: runCompose,
}

func init() {
	composeCmd.Flags().StringP("name", "n", "Snippet", "Name of the cheat")
	composeCmd.Flags().StringP("description", "d", "", "Description of the cheat")
	composeCmd.Flags().StringP("file", "f", "", "File to save to (defaults to first cheat path + /snippets.md)")
	composeCmd.Flags().BoolP("print", "p", false, "Print to stdout instead of saving to a file")
}

func runCompose(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	desc, _ := cmd.Flags().GetString("description")
	file, _ := cmd.Flags().GetString("file")
	printOnly, _ := cmd.Flags().GetBool("print")

	var command string
	if len(args) > 0 {
		command = strings.Join(args, " ")
	} else {
		// Read from stdin
		stat, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return fmt.Errorf("no command provided and no input piped. Use `cheatmd compose \"command\"`")
		}
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		command = strings.TrimSpace(string(b))
	}

	if command == "" {
		return fmt.Errorf("command cannot be empty")
	}

	// Extract variables
	vars := parser.ExtractVars(command)

	// Build markdown
	var sb strings.Builder
	sb.WriteString("\n# ")
	sb.WriteString(name)
	sb.WriteString("\n\n")

	if desc != "" {
		sb.WriteString(desc)
		sb.WriteString("\n\n")
	}

	if len(vars) > 0 {
		sb.WriteString("<!-- cheat\n")
		for _, v := range vars {
			sb.WriteString(fmt.Sprintf("var %s\n", v))
		}
		sb.WriteString("-->\n\n")
	}

	sb.WriteString("```sh\n")
	sb.WriteString(command)
	sb.WriteString("\n```\n")

	outStr := sb.String()

	if printOnly {
		fmt.Fprint(cmd.OutOrStdout(), strings.TrimPrefix(outStr, "\n"))
		return nil
	}

	// Determine output file
	targetFile := file
	if targetFile == "" {
		// Use default
		cfgPath := config.GetPath()
		if cfgPath == "" {
			return fmt.Errorf("no cheat path configured, please use -f or set cheatmd path")
		}
		firstPath := strings.Split(cfgPath, ",")[0]
		stat, err := os.Stat(firstPath)
		if err != nil {
			// Path does not exist, assume it's a directory we should create?
			// To be safe, let's check if it ends in .md
			if strings.HasSuffix(strings.ToLower(firstPath), ".md") {
				targetFile = firstPath
			} else {
				targetFile = filepath.Join(firstPath, "snippets.md")
			}
		} else if stat.IsDir() {
			targetFile = filepath.Join(firstPath, "snippets.md")
		} else {
			targetFile = firstPath
		}
	}

	// Ensure directory exists
	targetDir := filepath.Dir(targetFile)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for snippets: %w", err)
	}

	// Append to file
	f, err := os.OpenFile(targetFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", targetFile, err)
	}
	defer f.Close()

	if _, err := f.WriteString(outStr); err != nil {
		return fmt.Errorf("failed to write to file %s: %w", targetFile, err)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "✓ Cheat '%s' appended to %s\n", name, targetFile)
	return nil
}
