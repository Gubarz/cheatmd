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
	Editor            string `mapstructure:"editor"`
	PreHook           string `mapstructure:"pre_hook"`
	PostHook          string `mapstructure:"post_hook"`
	RequireCheatBlock   bool `mapstructure:"require_cheat_block"`
	AllowUndeclaredVars bool   `mapstructure:"allow_undeclared_vars"`
	VarSyntax           string `mapstructure:"var_syntax"`
	AutoSelect          bool `mapstructure:"auto_select"`
	AutoContinue        bool `mapstructure:"auto_continue"`

	// Keybindings
	KeyWidget     string `mapstructure:"key_widget"`
	KeyOpen       string `mapstructure:"key_open"`
	KeySubstitute string `mapstructure:"key_substitute"`
	KeyPreview    string `mapstructure:"key_preview"`

	// Substitute search
	SubstituteSources []string `mapstructure:"substitute_sources"`

	// Display options
	ShowFolder    bool `mapstructure:"show_folder"`
	ShowFile      bool `mapstructure:"show_file"`
	PreviewHeight int  `mapstructure:"preview_height"`

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
	editor            string
	preHook             string
	postHook            string
	requireCheatBlock   bool
	allowUndeclaredVars bool
	varSyntax           string
	autoSelect          bool
	autoContinue        bool
	keyWidget         string
	keyOpen           string
	keySubstitute     string
	keyPreview        string
	substituteSources []string
	showFolder        bool
	showFile          bool
	previewHeight     int
	colors            ColorConfig
	columns           ColumnConfig
}{
	path:              ".",
	output:            "print",
	shell:             "", // Set dynamically
	editor:            "", // Empty means use system default (xdg-open/open/start)
	preHook:           "",
	postHook:          "",
	requireCheatBlock:   false,
	allowUndeclaredVars: false,
	varSyntax:           "dollar",
	autoSelect:          false,
	autoContinue:        false,
	keyWidget:         "\\C-g",            // Ctrl+G for shell widgets
	keyOpen:           "ctrl+o",           // Ctrl+O in TUI
	keySubstitute:     "ctrl+t",           // Ctrl+T opens substitute search during var resolution
	keyPreview:        "ctrl+y",           // Ctrl+Y opens markdown preview of current cheat's file
	substituteSources: []string{"env", "history"},
	showFolder:        true,
	showFile:          true,
	previewHeight:     6,
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
	viper.SetDefault("editor", defaults.editor)
	viper.SetDefault("pre_hook", defaults.preHook)
	viper.SetDefault("post_hook", defaults.postHook)
	viper.SetDefault("require_cheat_block", defaults.requireCheatBlock)
	viper.SetDefault("allow_undeclared_vars", defaults.allowUndeclaredVars)
	viper.SetDefault("var_syntax", defaults.varSyntax)
	viper.SetDefault("auto_select", defaults.autoSelect)
	viper.SetDefault("auto_continue", defaults.autoContinue)

	// Keybindings
	viper.SetDefault("key_widget", defaults.keyWidget)
	viper.SetDefault("key_open", defaults.keyOpen)
	viper.SetDefault("key_substitute", defaults.keySubstitute)
	viper.SetDefault("key_preview", defaults.keyPreview)

	// Substitute search
	viper.SetDefault("substitute_sources", defaults.substituteSources)

	// Display options
	viper.SetDefault("show_folder", defaults.showFolder)
	viper.SetDefault("show_file", defaults.showFile)
	viper.SetDefault("preview_height", defaults.previewHeight)

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

// GetEditor returns the configured editor command (empty = system default)
func GetEditor() string {
	return viper.GetString("editor")
}

// GetAllowUndeclaredVars returns whether variables referenced in a cheat's
// command but not declared in any <!-- cheat --> block should be prompted at
// runtime. When false (default), undeclared variables are silently skipped.
//
// Reads from the cached struct populated at Init() because this getter is
// called in hot paths (per-variable, per-render).
func GetAllowUndeclaredVars() bool {
	return cfg.AllowUndeclaredVars
}

// GetVarSyntax returns the configured variable syntax mode.
// Valid values: "dollar" (default), "angle", "both".
//
// Reads from the cached struct populated at Init() because this getter is
// called in hot paths (per-variable, per-render).
func GetVarSyntax() string {
	v := cfg.VarSyntax
	switch v {
	case "dollar", "angle", "both":
		return v
	default:
		return "dollar"
	}
}

// VarSyntaxAllowsDollar reports whether $name is recognized as a variable.
func VarSyntaxAllowsDollar() bool {
	s := GetVarSyntax()
	return s == "dollar" || s == "both"
}

// VarSyntaxAllowsAngle reports whether <name> is recognized as a variable.
func VarSyntaxAllowsAngle() bool {
	s := GetVarSyntax()
	return s == "angle" || s == "both"
}

// GetRequireCheatBlock returns whether to require cheat blocks
func GetRequireCheatBlock() bool {
	return viper.GetBool("require_cheat_block")
}

// GetAutoSelect returns whether to auto-select single matches
func GetAutoSelect() bool {
	return viper.GetBool("auto_select")
}

// GetAutoContinue returns whether to auto-continue when vars are prefilled from environment
func GetAutoContinue() bool {
	return viper.GetBool("auto_continue")
}

// ============================================================================
// Getters - Keybindings
// ============================================================================

// GetKeyWidget returns the keybinding for shell widget activation (e.g., "\C-g" for Ctrl+G)
func GetKeyWidget() string {
	return viper.GetString("key_widget")
}

// GetKeyOpen returns the keybinding for opening markdown in editor (e.g., "ctrl+o")
func GetKeyOpen() string {
	return viper.GetString("key_open")
}

// GetKeySubstitute returns the keybinding for opening the substitute search
// during variable resolution (e.g., "ctrl+t").
func GetKeySubstitute() string {
	return viper.GetString("key_substitute")
}

// GetKeyPreview returns the keybinding for opening the markdown preview of
// the current cheat's source file (e.g., "ctrl+y").
func GetKeyPreview() string {
	return viper.GetString("key_preview")
}

// GetSubstituteSources returns the enabled sources for substitute search.
// Valid entries: "env", "history". Empty disables the feature.
func GetSubstituteSources() []string {
	return viper.GetStringSlice("substitute_sources")
}

// ============================================================================
// Getters - Display Options
// ============================================================================

// GetShowFolder returns whether to show folder in title/list
func GetShowFolder() bool {
	return viper.GetBool("show_folder")
}

// GetShowFile returns whether to show file in title/list
func GetShowFile() bool {
	return viper.GetBool("show_file")
}

// GetPreviewHeight returns the preview section height in lines
func GetPreviewHeight() int {
	return viper.GetInt("preview_height")
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



// ============================================================================
// Setters
// ============================================================================

// SetOutput sets the output mode at runtime
func SetOutput(mode string) {
	viper.Set("output", mode)
	cfg.Output = mode
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
