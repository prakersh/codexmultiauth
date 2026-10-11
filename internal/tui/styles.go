package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	sectionStyle  = lipgloss.NewStyle().Bold(true)
	activeStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"})
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"})
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "117"})
	footerStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "237"}).Bold(true)
)
