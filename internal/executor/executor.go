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

// ============================================================================
// Shell Runner Interface
// ============================================================================

// ShellRunner defines the interface for shell command execution
type ShellRunner interface {
	RunShell(command string) (string, error)
	Execute(command string) error
}

// ============================================================================
// Clipboard Interface
// ============================================================================

// Clipboard defines the interface for clipboard operations
type Clipboard interface {
	Copy(text string) error
}

// systemClipboard implements Clipboard using system commands
type systemClipboard struct{}

// Copy copies text to the system clipboard
func (c *systemClipboard) Copy(text string) error {
	cmd := c.findClipboardCommand()
	if cmd == nil {
		// No clipboard tool found, just print
		fmt.Println(text)
		return nil
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// findClipboardCommand returns the appropriate clipboard command for the system
func (c *systemClipboard) findClipboardCommand() *exec.Cmd {
	switch {
	case commandExists("wl-copy"):
		return exec.Command("wl-copy")
	case commandExists("xclip"):
		return exec.Command("xclip", "-selection", "clipboard")
	case commandExists("xsel"):
		return exec.Command("xsel", "--clipboard", "--input")
	case commandExists("pbcopy"):
		return exec.Command("pbcopy")
	default:
		return nil
	}
}

// commandExists checks if a command is available in PATH
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ============================================================================
// Executor
// ============================================================================

// Executor handles shell command execution and variable substitution
type Executor struct {
	index     *parser.CheatIndex
	shell     string
	clipboard Clipboard
}

// NewExecutor creates a new executor with the given cheat index
func NewExecutor(index *parser.CheatIndex) *Executor {
	return &Executor{
		index:     index,
		shell:     config.GetShell(),
		clipboard: &systemClipboard{},
	}
}

// WithClipboard sets a custom clipboard implementation (useful for testing)
func (e *Executor) WithClipboard(c Clipboard) *Executor {
	e.clipboard = c
	return e
}

// Index returns the cheat index
func (e *Executor) Index() *parser.CheatIndex {
	return e.index
}

// Shell returns the configured shell
func (e *Executor) Shell() string {
	return e.shell
}

// ============================================================================
// Command Execution
// ============================================================================

// RunShell executes a shell command and returns stdout
func (e *Executor) RunShell(command string) (string, error) {
	cmd := exec.Command(e.shell, "-c", command)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("shell error: %w: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Execute runs a command interactively with inherited stdin/stdout/stderr
func (e *Executor) Execute(command string) error {
	cmd := exec.Command(e.shell, "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

// ============================================================================
// Command Building
// ============================================================================

// BuildFinalCommand substitutes all variables in a cheat's command
func (e *Executor) BuildFinalCommand(cheat *parser.Cheat) string {
	result := cheat.Command

	// Substitute all scope variables
	for name, value := range cheat.Scope {
		result = strings.ReplaceAll(result, "$"+name, value)
	}

	// Handle escaped dollar signs
	result = strings.ReplaceAll(result, "\\$", "$")

	return result
}

// SubstituteVars replaces variables in a string using the given scope
func SubstituteVars(s string, scope map[string]string) string {
	for name, value := range scope {
		s = strings.ReplaceAll(s, "$"+name, value)
	}
	return s
}

// ============================================================================
// Output Handling
// ============================================================================

// OutputMode represents how the final command should be handled
type OutputMode string

const (
	OutputPrint OutputMode = "print"
	OutputCopy  OutputMode = "copy"
	OutputExec  OutputMode = "exec"
)

// Output handles command output based on the configured mode
func (e *Executor) Output(command string) error {
	mode := OutputMode(config.GetOutput())
	return e.OutputWithMode(command, mode)
}

// OutputWithMode handles command output with an explicit mode
func (e *Executor) OutputWithMode(command string, mode OutputMode) error {
	switch mode {
	case OutputExec:
		return e.Execute(command)
	case OutputCopy:
		return e.clipboard.Copy(command)
	default: // print
		fmt.Println(command)
		return nil
	}
}
