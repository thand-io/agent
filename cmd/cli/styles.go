package cli

import (
	"github.com/charmbracelet/lipgloss"
)

// Shared styles for the CLI package
// All terminal colors and styling definitions are centralized here
var (
	// Primary styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B575")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#10B981"))

	// Status styles
	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3B82F6"))

	// Session-specific styles
	expiredStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Strikethrough(true)

	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981"))
)
