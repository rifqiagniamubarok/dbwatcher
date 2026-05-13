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
)
