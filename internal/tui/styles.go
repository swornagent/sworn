// Package tui provides the Bubble Tea TUI for `sworn` (no args) and
// `sworn top` (no release arg). It shows a releases list in the left pane
// and a board view (tracks + slice states) in the right pane.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Colour palette.
	colPrimary = lipgloss.Color("#7C3AED") // purple-600
	colAccent  = lipgloss.Color("#10B981") // emerald-500
	colWarn    = lipgloss.Color("#F59E0B") // amber-500
	colFail    = lipgloss.Color("#EF4444") // red-500
	colMuted   = lipgloss.Color("#6B7280") // gray-500
	colText    = lipgloss.Color("#E5E7EB") // gray-200
	colDim     = lipgloss.Color("#9CA3AF") // gray-400
	colBg      = lipgloss.Color("#1F2937") // gray-800
	colBgSel   = lipgloss.Color("#374151") // gray-700
	colBorder  = lipgloss.Color("#4B5563") // gray-600
	colHelpBg  = lipgloss.Color("#111827") // gray-900

	// Release list pane.
	ReleaseListStyle = lipgloss.NewStyle().
				Width(30).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colBorder).
				Padding(0, 1)

	ReleaseListTitle = lipgloss.NewStyle().
				Foreground(colPrimary).
				Bold(true).
				Padding(0, 1)

	ReleaseItem = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(colText)

	ReleaseItemSelected = ReleaseItem.Copy().
				Background(colBgSel).
				Foreground(colAccent).
				Bold(true)

	// Board pane.
	BoardStyle = lipgloss.NewStyle().
			Width(80).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colBorder).
			Padding(0, 1)

	BoardTitle = lipgloss.NewStyle().
			Foreground(colAccent).
			Bold(true).
			Padding(0, 1)

	TrackHeader = lipgloss.NewStyle().
			Foreground(colPrimary).
			Bold(true)

	SliceItem = lipgloss.NewStyle().
			Foreground(colText).
			Padding(0, 0)

	BoardItemSelected = SliceItem.Copy().
				Background(colBgSel).
				Foreground(colAccent).
				Bold(true)
	SliceStatePlanned = lipgloss.NewStyle().Foreground(colMuted).Render
	SliceStateActive  = lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render
	SliceStateDone    = lipgloss.NewStyle().Foreground(colAccent).Render
	SliceStateFailed  = lipgloss.NewStyle().Foreground(colFail).Render
	SliceStateBlocked = lipgloss.NewStyle().Foreground(colWarn).Render

	// Help bar.
	HelpBar = lipgloss.NewStyle().
		Background(colHelpBg).
		Foreground(colDim).
		Padding(0, 2).
		Width(110)

	HelpKey = lipgloss.NewStyle().
		Foreground(colAccent).
		Bold(true)

	// Live view (concurrent status).
	LiveTitle = lipgloss.NewStyle().
			Foreground(colAccent).
			Bold(true).
			Padding(0, 1)

	LiveRow = lipgloss.NewStyle().
		Foreground(colText).
		Padding(0, 1)

	// MergeRowStyle is the style for merge-actor rows in the live view.
	// Visually distinct from worker/coordinator rows: amber, bold.
	MergeRowStyle = lipgloss.NewStyle().
			Foreground(colWarn).
			Bold(true).
			Padding(0, 1)

	// MergeBadge is the board-view merge indicator appended to track headers.
	MergeBadge = lipgloss.NewStyle().
			Foreground(colWarn).
			Bold(true)
	DividerLine = lipgloss.NewStyle().
			Foreground(colMuted).
			Render(strings.Repeat("─", 70))

	// Gate status indicators (S72).
	GatePassStyle    = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	GateFailStyle    = lipgloss.NewStyle().Foreground(colFail).Bold(true)
	GateWarnStyle    = lipgloss.NewStyle().Foreground(colWarn)
	GateNeutralStyle = lipgloss.NewStyle().Foreground(colMuted)
	GateBracketStyle = lipgloss.NewStyle().Foreground(colMuted)
	GateSepStyle     = lipgloss.NewStyle().Foreground(colMuted)

	// Generic.
	Divider      = lipgloss.NewStyle().Foreground(colMuted).Render("─")
	EmptyMessage = lipgloss.NewStyle().Foreground(colMuted).Italic(true).Padding(0, 2)
)

// StateColor renders a slice state string with the correct colour.
func StateColor(state string) string {
	switch state {
	case "planned", "":
		return SliceStatePlanned(state)
	case "in_progress", "design_review":
		return SliceStateActive(state)
	case "verified", "implemented":
		return SliceStateDone(state)
	case "failed_verification":
		return SliceStateFailed(state)
	case "deferred":
		return SliceStateBlocked(state)
	default:
		return SliceStatePlanned(state)
	}
}

// SliceStateColor renders a slice's state with the correct colour, considering
// the verification result. A slice at state "implemented" with
// verification.result == "blocked" is shown in the blocked colour, not the
// done colour.
func SliceStateColor(state, verificationResult string) string {
	if state == "implemented" && verificationResult == "blocked" {
		return SliceStateBlocked("blocked")
	}
	return StateColor(state)
}
