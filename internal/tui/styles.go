package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorInsert = lipgloss.AdaptiveColor{Light: "#1a7a1a", Dark: "#5af078"}
	colorUpdate = lipgloss.AdaptiveColor{Light: "#7a5a00", Dark: "#f0c040"}
	colorDelete = lipgloss.AdaptiveColor{Light: "#8a1a1a", Dark: "#f07070"}
	colorDim    = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#555555"}
	colorCyan   = lipgloss.AdaptiveColor{Light: "#006b6b", Dark: "#5af0f0"}
	colorOld    = lipgloss.AdaptiveColor{Light: "#8a1a1a", Dark: "#f07070"}
	colorNew    = lipgloss.AdaptiveColor{Light: "#1a7a1a", Dark: "#5af078"}
	colorBlue    = lipgloss.AdaptiveColor{Light: "#1a3a8a", Dark: "#7aa0ff"}
	colorMarker  = lipgloss.AdaptiveColor{Light: "#5a5a5a", Dark: "#a0a0a0"}
	colorDDL     = lipgloss.AdaptiveColor{Light: "#7a1a8a", Dark: "#d080f0"}

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	styleFooter = lipgloss.NewStyle().
			Foreground(colorDim).
			Padding(0, 1)

	styleInsert = lipgloss.NewStyle().Foreground(colorInsert)
	styleUpdate = lipgloss.NewStyle().Foreground(colorUpdate)
	styleDelete = lipgloss.NewStyle().Foreground(colorDelete)

	styleTableName = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	styleTimestamp = lipgloss.NewStyle().Foreground(colorDim)
	styleDim       = lipgloss.NewStyle().Foreground(colorDim)

	styleDiffOld = lipgloss.NewStyle().Foreground(colorOld)
	styleDiffNew = lipgloss.NewStyle().Foreground(colorNew)

	styleCursor = lipgloss.NewStyle().Reverse(true)

	styleSidebar = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(colorDim).
			Padding(0, 1)

	styleSelected = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)

	styleMarker = lipgloss.NewStyle().Foreground(colorMarker).Bold(true)
	styleLog    = lipgloss.NewStyle().Foreground(colorBlue).Italic(true)
	styleDDL    = lipgloss.NewStyle().Foreground(colorDDL).Bold(true)
)

// markerColorStyle returns the lipgloss style for a given marker color name.
// Unknown colors fall back to the default marker style.
func markerColorStyle(color string) lipgloss.Style {
	switch color {
	case "yellow":
		return lipgloss.NewStyle().Foreground(colorUpdate).Bold(true)
	case "green":
		return lipgloss.NewStyle().Foreground(colorInsert).Bold(true)
	case "red":
		return lipgloss.NewStyle().Foreground(colorDelete).Bold(true)
	case "blue":
		return lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	case "dim":
		return lipgloss.NewStyle().Foreground(colorDim)
	default:
		return styleMarker
	}
}
