package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/gubarz/cheatmd/internal/config"
)

// StyleManager encapsulates all TUI styles and provides methods for style operations
type StyleManager struct {
	// List view styles
	Header   lipgloss.Style
	Desc     lipgloss.Style
	Command  lipgloss.Style
	Path     lipgloss.Style
	Selected lipgloss.Style
	Cursor   lipgloss.Style
	Dim      lipgloss.Style

	// Preview styles
	PreviewHeader lipgloss.Style
	PreviewDesc   lipgloss.Style
	PreviewCmd    lipgloss.Style
	PreviewPath   lipgloss.Style

	// Chrome styles
	Border  lipgloss.Style
	Divider lipgloss.Style

	// Colors for direct access
	SelectedBg lipgloss.Color
}

// DefaultStyles returns a StyleManager with default styles
func DefaultStyles() *StyleManager {
	return &StyleManager{
		Header:        lipgloss.NewStyle().Bold(true),
		Desc:          lipgloss.NewStyle(),
		Command:       lipgloss.NewStyle(),
		Path:          lipgloss.NewStyle(),
		Selected:      lipgloss.NewStyle().Background(lipgloss.Color("236")),
		Cursor:        lipgloss.NewStyle().Foreground(lipgloss.Color("212")),
		Dim:           lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		PreviewHeader: lipgloss.NewStyle().Bold(true),
		PreviewDesc:   lipgloss.NewStyle(),
		PreviewCmd:    lipgloss.NewStyle(),
		PreviewPath:   lipgloss.NewStyle(),
		Border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")),
		Divider:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		SelectedBg:    lipgloss.Color("236"),
	}
}

// LoadFromConfig updates styles based on configuration
func (s *StyleManager) LoadFromConfig() {
	// Get colors from config
	headerColor := parseANSIColor(config.GetColorHeader())
	descColor := parseANSIColor(config.GetColorDesc())
	cmdColor := parseANSIColor(config.GetColorCommand())
	pathColor := parseANSIColor(config.GetColorPath())
	borderColor := lipgloss.Color(config.GetColorBorder())
	cursorColor := lipgloss.Color(config.GetColorCursor())
	selectedBg := lipgloss.Color(config.GetColorSelected())
	dimColor := lipgloss.Color(config.GetColorDim())

	// List view styles
	s.Header = lipgloss.NewStyle().Foreground(headerColor)
	s.Desc = lipgloss.NewStyle().Foreground(descColor)
	s.Command = lipgloss.NewStyle().Foreground(cmdColor)
	s.Path = lipgloss.NewStyle().Foreground(pathColor)
	s.Selected = lipgloss.NewStyle().Background(selectedBg)
	s.Cursor = lipgloss.NewStyle().Foreground(cursorColor)
	s.Dim = lipgloss.NewStyle().Foreground(dimColor)

	// Preview styles (same colors, header is bold)
	s.PreviewHeader = lipgloss.NewStyle().Bold(true).Foreground(headerColor)
	s.PreviewDesc = lipgloss.NewStyle().Foreground(descColor)
	s.PreviewCmd = lipgloss.NewStyle().Foreground(cmdColor)
	s.PreviewPath = lipgloss.NewStyle().Foreground(pathColor)

	// Chrome styles
	s.Border = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor)
	s.Divider = lipgloss.NewStyle().Foreground(borderColor)
	s.SelectedBg = selectedBg
}

// WithSelection returns a copy of the given style with the selected background applied
func (s *StyleManager) WithSelection(style lipgloss.Style) lipgloss.Style {
	return style.Background(s.SelectedBg)
}

// parseANSIColor converts ANSI color codes to lipgloss colors
func parseANSIColor(code string) lipgloss.Color {
	ansiToLipgloss := map[string]string{
		"30": "0", "31": "1", "32": "2", "33": "3",
		"34": "4", "35": "5", "36": "6", "37": "7",
		"90": "8", "91": "9", "92": "10", "93": "11",
		"94": "12", "95": "13", "96": "14", "97": "15",
	}
	if mapped, ok := ansiToLipgloss[code]; ok {
		return lipgloss.Color(mapped)
	}
	return lipgloss.Color(code)
}

// Global style manager instance
var styles = DefaultStyles()

// RefreshStyles updates the global styles from config
func RefreshStyles() {
	styles.LoadFromConfig()
}
