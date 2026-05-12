package ui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/parser"
)

// getTTY returns file handles for TUI input/output. Uses /dev/tty to bypass
// shell pipes and command substitution when stdout is not a terminal.
func getTTY() (in *os.File, out *os.File, cleanup func()) {
	var closers []func()

	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		// stdout is NOT a terminal - we're being captured.
		out, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			out = os.Stderr
		} else {
			closers = append(closers, func() { out.Close() })
		}

		in, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
		if err != nil {
			in = os.Stdin
		} else {
			closers = append(closers, func() { in.Close() })
		}

		lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(out))

		return in, out, func() {
			for _, c := range closers {
				c()
			}
		}
	}

	return os.Stdin, os.Stdout, func() {}
}

// RunTUI launches the Bubble Tea interface (unified, no flicker).
func RunTUI(index *parser.CheatIndex, exec Executor, initialQuery, matchCmd string) error {
	requireCheatBlock := config.GetRequireCheatBlock()
	autoSelect := config.GetAutoSelect()

	cheats := filterCheatsByConfig(index.Cheats, requireCheatBlock)
	if len(cheats) == 0 {
		return fmt.Errorf("no cheats found")
	}

	m := newMainModel(cheats, index, exec)

	if matchCmd != "" {
		if matched := findMatchingCheat(cheats, matchCmd); matched != nil {
			m.selected = matched
			prefillScopeFromMatch(matched, matchCmd)
			inferDependentVars(matched, index)
			m.startVarResolutionInternal()

			if m.phase != phaseVarResolve {
				finalCmd := exec.BuildFinalCommand(m.selected)
				return executeOutput(finalCmd, exec)
			}

			if m.varState != nil && len(m.varState.vars) > 0 {
				vs := &m.varState.vars[0]
				if vs.prefill != "" {
					m.textInput.SetValue(vs.prefill)
					m.textInput.CursorEnd()
				}
			}
		} else {
			initialQuery = matchCmd
		}
	}

	if initialQuery != "" {
		m.textInput.SetValue(initialQuery)
		m.filterCheats()

		if autoSelect && len(m.filtered) == 1 {
			m.selected = m.filtered[0].cheat
			m.startVarResolutionInternal()

			if m.phase != phaseVarResolve {
				finalCmd := exec.BuildFinalCommand(m.selected)
				return executeOutput(finalCmd, exec)
			}
		}
	}

	ttyIn, ttyOut, cleanup := getTTY()
	RefreshStyles()
	p := tea.NewProgram(&m, tea.WithAltScreen(), tea.WithOutput(ttyOut), tea.WithInput(ttyIn))
	finalModel, err := p.Run()
	cleanup()

	if err != nil {
		return err
	}

	result := finalModel.(*mainModel)
	if result.quitting && result.selected == nil {
		return nil
	}
	if result.selected == nil {
		return nil
	}

	finalCmd := exec.BuildFinalCommand(result.selected)
	return executeOutput(finalCmd, exec)
}

// filterCheatsByConfig returns cheats matching configuration. When
// requireCheatBlock is true, cheats without a <!-- cheat --> block are
// excluded.
func filterCheatsByConfig(cheats []*parser.Cheat, requireCheatBlock bool) []*parser.Cheat {
	if !requireCheatBlock {
		return cheats
	}

	result := make([]*parser.Cheat, 0, len(cheats))
	for _, cheat := range cheats {
		if cheat.HasCheatBlock {
			result = append(result, cheat)
		}
	}
	return result
}

// openFileInViewer opens filePath in the configured editor or system default.
func openFileInViewer(filePath string) {
	var cmd *exec.Cmd

	if editor := config.GetEditor(); editor != "" {
		cmd = exec.Command(editor, filePath)
	} else {
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", filePath)
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", "", filePath)
		default:
			cmd = exec.Command("xdg-open", filePath)
		}
	}
	_ = cmd.Start()
}
