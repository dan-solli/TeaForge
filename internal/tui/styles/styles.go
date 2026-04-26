// Package styles defines the visual styling constants for the TeaForge TUI.
package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Color palette
	ColorAccent   = lipgloss.Color("#7C3AED") // violet
	ColorUser     = lipgloss.Color("#059669") // emerald
	ColorAI       = lipgloss.Color("#2563EB") // blue
	ColorTool     = lipgloss.Color("#D97706") // amber
	ColorError    = lipgloss.Color("#DC2626") // red
	ColorMuted    = lipgloss.Color("#6B7280") // gray
	ColorBorder   = lipgloss.Color("#374151") // dark gray
	ColorSelected = lipgloss.Color("#1D4ED8") // dark blue

	// Base styles
	Base = lipgloss.NewStyle()

	// Header / title bar
	TitleBar = lipgloss.NewStyle().
			Background(ColorAccent).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 2)

	// Active tab
	TabActive = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true).
			Underline(true).
			Padding(0, 1)

	// Inactive tab
	TabInactive = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1)

	// Panel border
	Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	// Active panel border
	PanelActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(0, 1)

	// Chat message styles
	UserLabel = lipgloss.NewStyle().
			Foreground(ColorUser).
			Bold(true)

	AILabel = lipgloss.NewStyle().
		Foreground(ColorAI).
		Bold(true)

	ToolLabel = lipgloss.NewStyle().
			Foreground(ColorTool).
			Bold(true)

	ErrorText = lipgloss.NewStyle().
			Foreground(ColorError)

	MutedText = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Status bar
	StatusBar = lipgloss.NewStyle().
			Background(ColorBorder).
			Foreground(lipgloss.Color("#E5E7EB")).
			Padding(0, 1)

	StatusKey = lipgloss.NewStyle().
			Background(ColorBorder).
			Foreground(ColorAccent).
			Bold(true).
			Padding(0, 1)

	// Input area
	InputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(0, 1)

	// List item
	ListItem = lipgloss.NewStyle().
			Padding(0, 1)

	ListItemSelected = lipgloss.NewStyle().
				Background(ColorSelected).
				Foreground(lipgloss.Color("#FFFFFF")).
				Padding(0, 1)
)
