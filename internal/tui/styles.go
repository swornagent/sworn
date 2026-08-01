package tui

import "github.com/charmbracelet/lipgloss"

var (
	carbon = lipgloss.Color("#17201e")
	paper  = lipgloss.Color("#f8faf6")
	relay  = lipgloss.Color("#2f687a")
	baton  = lipgloss.Color("#cb432d")
	sworn  = lipgloss.Color("#56508a")
	fault  = lipgloss.Color("#d35d68")
	quiet  = lipgloss.Color("#8a9692")

	headerStyle = lipgloss.NewStyle().
			Background(carbon).
			Foreground(paper).
			Bold(true)
	titleStyle    = lipgloss.NewStyle().Foreground(relay).Bold(true)
	selectedStyle = lipgloss.NewStyle().
			Background(sworn).
			Foreground(paper).
			Bold(true)
	batonStyle  = lipgloss.NewStyle().Foreground(baton).Bold(true)
	swornStyle  = lipgloss.NewStyle().Foreground(sworn).Bold(true)
	faultStyle  = lipgloss.NewStyle().Foreground(fault).Bold(true)
	quietStyle  = lipgloss.NewStyle().Foreground(quiet)
	footerStyle = lipgloss.NewStyle().Background(carbon).Foreground(paper)
)
