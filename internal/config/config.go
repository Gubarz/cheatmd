package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	CheatPath         string `mapstructure:"path"`
	Output            string `mapstructure:"output"`
	Shell             string `mapstructure:"shell"`
	FzfOpts           string `mapstructure:"fzf_opts"`
	PreHook           string `mapstructure:"pre_hook"`
	PostHook          string `mapstructure:"post_hook"`
	ColorHeader       string `mapstructure:"color_header"`
	ColorCommand      string `mapstructure:"color_command"`
	ColorDesc         string `mapstructure:"color_desc"`
	ColumnGap         int    `mapstructure:"column_gap"`
	ColumnHeader      int    `mapstructure:"column_header"`
	ColumnDesc        int    `mapstructure:"column_desc"`
	ColumnCommand     int    `mapstructure:"column_command"`
	RequireCheatBlock bool   `mapstructure:"require_cheat_block"`
}

// C is the global config instance
var C Config

// Init initializes configuration with viper
func Init() error {
	viper.SetDefault("path", ".")
	viper.SetDefault("output", "print")
	viper.SetDefault("shell", getDefaultShell())
	viper.SetDefault("fzf_opts", "")
	viper.SetDefault("pre_hook", "")
	viper.SetDefault("post_hook", "")
	viper.SetDefault("color_header", "36")         // Cyan
	viper.SetDefault("color_command", "32")        // Green
	viper.SetDefault("color_desc", "90")           // Gray
	viper.SetDefault("column_gap", 4)              // Spaces between columns
	viper.SetDefault("column_header", 40)          // Max header width
	viper.SetDefault("column_desc", 40)            // Max description width
	viper.SetDefault("column_command", 60)         // Max command width
	viper.SetDefault("require_cheat_block", false) // Show all code blocks by default

	viper.SetConfigName("cheatmd")
	viper.SetConfigType("yaml")

	if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(home, ".config", "cheatmd"))
		viper.AddConfigPath(home)
	}
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("CHEATMD")
	viper.AutomaticEnv()

	// Try to read config, but don't fail if not found or malformed
	_ = viper.ReadInConfig()

	return viper.Unmarshal(&C)
}

// GetPath returns the cheat path with tilde expansion
func GetPath() string {
	path := viper.GetString("path")
	return expandTilde(path)
}

// expandTilde expands ~ to the user's home directory
func expandTilde(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// GetOutput returns the output mode
func GetOutput() string {
	return viper.GetString("output")
}

// GetShell returns the shell
func GetShell() string {
	return viper.GetString("shell")
}

// GetFzfOpts returns extra fzf options
func GetFzfOpts() string {
	return viper.GetString("fzf_opts")
}

// GetColorHeader returns ANSI color code for header
func GetColorHeader() string {
	return viper.GetString("color_header")
}

// GetColorCommand returns ANSI color code for command
func GetColorCommand() string {
	return viper.GetString("color_command")
}

// GetColorDesc returns ANSI color code for description
func GetColorDesc() string {
	return viper.GetString("color_desc")
}

// GetColumnGap returns spacing between columns
func GetColumnGap() int {
	return viper.GetInt("column_gap")
}

// GetColumnHeader returns max header column width
func GetColumnHeader() int {
	return viper.GetInt("column_header")
}

// GetColumnDesc returns max description column width
func GetColumnDesc() int {
	return viper.GetInt("column_desc")
}

// GetColumnCommand returns max command column width
func GetColumnCommand() int {
	return viper.GetInt("column_command")
}

// GetRequireCheatBlock returns whether to only show cheats with <!-- cheat --> blocks
func GetRequireCheatBlock() bool {
	return viper.GetBool("require_cheat_block")
}

// GetPreHook returns the pre-execution hook command
func GetPreHook() string {
	return viper.GetString("pre_hook")
}

// GetPostHook returns the post-execution hook command
func GetPostHook() string {
	return viper.GetString("post_hook")
}

// SetOutput sets output mode at runtime
func SetOutput(mode string) {
	viper.Set("output", mode)
	C.Output = mode
}

// SetPath sets path at runtime
func SetPath(path string) {
	viper.Set("path", path)
	C.CheatPath = path
}

func getDefaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/bash"
}
