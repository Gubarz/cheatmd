package main

import (
	"fmt"
	"path/filepath"

	"github.com/gubarz/cheatmd/pkg/chainstate"
	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/spf13/cobra"
)

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
