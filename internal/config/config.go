package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ============================================================================
// Configuration Types
// ============================================================================

// Config holds the application configuration
type Config struct {
	Path              string `mapstructure:"path"`
	Output            string `mapstructure:"output"`
	Shell             string `mapstructure:"shell"`
	PreHook           string `mapstructure:"pre_hook"`
	PostHook          string `mapstructure:"post_hook"`
	RequireCheatBlock bool   `mapstructure:"require_cheat_block"`
	AutoSelect        bool   `mapstructure:"auto_select"`

	// Colors
	Colors ColorConfig

	// Columns
	Columns ColumnConfig
}

// ColorConfig holds all color settings
type ColorConfig struct {
	Header   string `mapstructure:"color_header"`
	Command  string `mapstructure:"color_command"`
	Desc     string `mapstructure:"color_desc"`
	Path     string `mapstructure:"color_path"`
	Border   string `mapstructure:"color_border"`
	Cursor   string `mapstructure:"color_cursor"`
	Selected string `mapstructure:"color_selected"`
	Dim      string `mapstructure:"color_dim"`
}

// ColumnConfig holds column width settings
type ColumnConfig struct {
	Gap     int `mapstructure:"column_gap"`
	Header  int `mapstructure:"column_header"`
	Desc    int `mapstructure:"column_desc"`
	Command int `mapstructure:"column_command"`
}

// ============================================================================
// Default Values
// ============================================================================

// Defaults for configuration
var defaults = struct {
	path              string
	output            string
	shell             string
	preHook           string
	postHook          string
	requireCheatBlock bool
	autoSelect        bool
	colors            ColorConfig
	columns           ColumnConfig
}{
	path:              ".",
	output:            "print",
	shell:             "", // Set dynamically
	preHook:           "",
	postHook:          "",
	requireCheatBlock: false,
	autoSelect:        false,
	colors: ColorConfig{
		Header:   "36",  // Cyan
		Command:  "32",  // Green
		Desc:     "90",  // Gray
		Path:     "33",  // Yellow
		Border:   "240", // Dark gray
		Cursor:   "212", // Pink
		Selected: "236", // Dark bg
		Dim:      "241", // Dimmed
	},
	columns: ColumnConfig{
		Gap:     4,
		Header:  40,
		Desc:    40,
		Command: 60,
	},
}

// ============================================================================
// Global Config
// ============================================================================

// cfg is the global config instance
var cfg Config

// Init initializes configuration with viper
func Init() error {
	setDefaults()
	configureViper()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Warning: config file error: %v\n", err)
		}
	}

	return viper.Unmarshal(&cfg)
}

// setDefaults sets all default values in viper
func setDefaults() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	viper.SetDefault("path", defaults.path)
	viper.SetDefault("output", defaults.output)
	viper.SetDefault("shell", shell)
	viper.SetDefault("pre_hook", defaults.preHook)
	viper.SetDefault("post_hook", defaults.postHook)
	viper.SetDefault("require_cheat_block", defaults.requireCheatBlock)
	viper.SetDefault("auto_select", defaults.autoSelect)

	// Colors
	viper.SetDefault("color_header", defaults.colors.Header)
	viper.SetDefault("color_command", defaults.colors.Command)
	viper.SetDefault("color_desc", defaults.colors.Desc)
	viper.SetDefault("color_path", defaults.colors.Path)
	viper.SetDefault("color_border", defaults.colors.Border)
	viper.SetDefault("color_cursor", defaults.colors.Cursor)
	viper.SetDefault("color_selected", defaults.colors.Selected)
	viper.SetDefault("color_dim", defaults.colors.Dim)

	// Columns
	viper.SetDefault("column_gap", defaults.columns.Gap)
	viper.SetDefault("column_header", defaults.columns.Header)
	viper.SetDefault("column_desc", defaults.columns.Desc)
	viper.SetDefault("column_command", defaults.columns.Command)
}

// configureViper sets up viper configuration sources
func configureViper() {
	viper.SetConfigName("cheatmd")
	viper.SetConfigType("yaml")

	if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(home, ".config", "cheatmd"))
		viper.AddConfigPath(home)
	}
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("CHEATMD")
	viper.AutomaticEnv()
}

// ============================================================================
// Getters - Core Settings
// ============================================================================

// GetPath returns the cheat path with tilde expansion
func GetPath() string {
	return expandTilde(viper.GetString("path"))
}

// GetOutput returns the output mode
func GetOutput() string {
	return viper.GetString("output")
}

// GetShell returns the configured shell
func GetShell() string {
	return viper.GetString("shell")
}

// GetPreHook returns the pre-execution hook
func GetPreHook() string {
	return viper.GetString("pre_hook")
}

// GetPostHook returns the post-execution hook
func GetPostHook() string {
	return viper.GetString("post_hook")
}

// GetRequireCheatBlock returns whether to require cheat blocks
func GetRequireCheatBlock() bool {
	return viper.GetBool("require_cheat_block")
}

// GetAutoSelect returns whether to auto-select single matches
func GetAutoSelect() bool {
	return viper.GetBool("auto_select")
}

// ============================================================================
// Getters - Colors
// ============================================================================

// GetColorHeader returns the header color code
func GetColorHeader() string {
	return viper.GetString("color_header")
}

// GetColorCommand returns the command color code
func GetColorCommand() string {
	return viper.GetString("color_command")
}

// GetColorDesc returns the description color code
func GetColorDesc() string {
	return viper.GetString("color_desc")
}

// GetColorPath returns the path color code
func GetColorPath() string {
	return viper.GetString("color_path")
}

// GetColorBorder returns the border color code
func GetColorBorder() string {
	return viper.GetString("color_border")
}

// GetColorCursor returns the cursor color code
func GetColorCursor() string {
	return viper.GetString("color_cursor")
}

// GetColorSelected returns the selected background color code
func GetColorSelected() string {
	return viper.GetString("color_selected")
}

// GetColorDim returns the dimmed text color code
func GetColorDim() string {
	return viper.GetString("color_dim")
}

// GetColors returns all color settings as a ColorConfig
func GetColors() ColorConfig {
	return ColorConfig{
		Header:   GetColorHeader(),
		Command:  GetColorCommand(),
		Desc:     GetColorDesc(),
		Path:     GetColorPath(),
		Border:   GetColorBorder(),
		Cursor:   GetColorCursor(),
		Selected: GetColorSelected(),
		Dim:      GetColorDim(),
	}
}

// ============================================================================
// Getters - Columns
// ============================================================================

// GetColumnGap returns column spacing
func GetColumnGap() int {
	return viper.GetInt("column_gap")
}

// GetColumnHeader returns header column width
func GetColumnHeader() int {
	return viper.GetInt("column_header")
}

// GetColumnDesc returns description column width
func GetColumnDesc() int {
	return viper.GetInt("column_desc")
}

// GetColumnCommand returns command column width
func GetColumnCommand() int {
	return viper.GetInt("column_command")
}

// GetColumns returns all column settings as a ColumnConfig
func GetColumns() ColumnConfig {
	return ColumnConfig{
		Gap:     GetColumnGap(),
		Header:  GetColumnHeader(),
		Desc:    GetColumnDesc(),
		Command: GetColumnCommand(),
	}
}

// ============================================================================
// Setters
// ============================================================================

// SetOutput sets the output mode at runtime
func SetOutput(mode string) {
	viper.Set("output", mode)
	cfg.Output = mode
}

// SetPath sets the cheat path at runtime
func SetPath(path string) {
	viper.Set("path", path)
	cfg.Path = path
}

// SetAutoSelect sets auto-select mode at runtime
func SetAutoSelect(enabled bool) {
	viper.Set("auto_select", enabled)
	cfg.AutoSelect = enabled
}

// ============================================================================
// Helpers
// ============================================================================

// expandTilde expands ~ to the user's home directory
func expandTilde(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, path[1:])
	}
	return path
}
