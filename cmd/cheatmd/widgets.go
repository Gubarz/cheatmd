package main

import (
	"fmt"
	"strings"

	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/spf13/cobra"
)

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
