package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/parser"
)

// Executor handles shell execution
type Executor struct {
	index *parser.CheatIndex
	shell string
}

// NewExecutor creates a new executor
func NewExecutor(index *parser.CheatIndex) *Executor {
	return &Executor{
		index: index,
		shell: config.GetShell(),
	}
}

// RunShell executes a shell command and returns stdout
func (e *Executor) RunShell(command string) (string, error) {
	cmd := exec.Command(e.shell, "-c", command)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("shell error: %w: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// BuildFinalCommand substitutes all variables in command
func (e *Executor) BuildFinalCommand(cheat *parser.Cheat) string {
	result := cheat.Command
	for name, value := range cheat.Scope {
		result = strings.ReplaceAll(result, "$"+name, value)
	}
	// Handle escaped dollar signs
	result = strings.ReplaceAll(result, "\\$", "$")
	return result
}

// Execute runs the final command based on current output mode
func (e *Executor) Execute(command string) error {
	mode := config.GetOutput()

	switch mode {
	case "exec":
		cmd := exec.Command(e.shell, "-c", command)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		return cmd.Run()

	case "copy":
		return e.copyToClipboard(command)

	default: // print
		fmt.Println(command)
		return nil
	}
}

// copyToClipboard copies text to system clipboard
func (e *Executor) copyToClipboard(text string) error {
	var copyCmd *exec.Cmd

	// Try various clipboard commands
	if _, err := exec.LookPath("wl-copy"); err == nil {
		copyCmd = exec.Command("wl-copy")
	} else if _, err := exec.LookPath("xclip"); err == nil {
		copyCmd = exec.Command("xclip", "-selection", "clipboard")
	} else if _, err := exec.LookPath("xsel"); err == nil {
		copyCmd = exec.Command("xsel", "--clipboard", "--input")
	} else if _, err := exec.LookPath("pbcopy"); err == nil {
		copyCmd = exec.Command("pbcopy")
	} else {
		// No clipboard tool found, just print
		fmt.Println(text)
		return nil
	}

	copyCmd.Stdin = strings.NewReader(text)
	return copyCmd.Run()
}

// GetIndex returns the cheat index
func (e *Executor) GetIndex() *parser.CheatIndex {
	return e.index
}
