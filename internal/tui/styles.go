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

	// Release list pane. Width is applied per render from the real terminal
	// width (Model.View via paneWidths) — no hardcoded Width here.
	ReleaseListStyle = lipgloss.NewStyle().
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

	// Board pane. Width is applied per render from the real terminal width
	// (Model.View via paneWidths) — no hardcoded Width here.
	BoardStyle = lipgloss.NewStyle().
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

	// Help bar. Width is applied per render from the real terminal width
	// (Model.renderHelp) so the background-styled bar spans edge-to-edge on
	// any terminal — no hardcoded Width here.
	HelpBar = lipgloss.NewStyle().
		Background(colHelpBg).
		Foreground(colDim).
		Padding(0, 2)

	HelpKey = lipgloss.NewStyle().
		Foreground(colAccent).
		Bold(true)

	// Header bar (S03). Full-width bar above the two-pane layout showing the
	// sworn version and the currently-selected release. Width is applied per
	// render from the real terminal width (Model.renderHeader), mirroring the
	// help bar for top/bottom visual symmetry.
	HeaderStyle = lipgloss.NewStyle().
			Background(colPrimary).
			Foreground(colText).
			Bold(true).
			Padding(0, 2)

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

	// DependsBadge is the board-view "needs: T2, T3" indicator appended to a
	// track header when the track has a non-empty depends_on.
	DependsBadge = lipgloss.NewStyle().
			Foreground(colDim)
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

const (
	// legacyLeftWidth and legacyRightWidth are the pre-S03 hardcoded pane
	// widths. paneWidths returns them as the fallback when no
	// tea.WindowSizeMsg has been received yet (total <= 0) — every existing
	// test that constructs a Model directly, without driving it through
	// tea.NewProgram, keeps its prior rendering behaviour.
	legacyLeftWidth  = 30
	legacyRightWidth = 80

	// legacyHelpWidth is the pre-S03 hardcoded help/header bar width, used as
	// the same no-WindowSizeMsg fallback in Model.renderHelp/renderHeader.
	legacyHelpWidth = 110

	// minLeftPane is the Coach-decided minimum-width floor for the releases
	// pane (review pin 1, option b): the left pane never shrinks below this,
	// keeping release names legible at an 80-col terminal instead of being
	// squeezed to near-nothing by a pure proportional split. Names still
	// longer than the pane are ellipsis-truncated in ReleasesList.View.
	minLeftPane = 26

	// borderCols is the horizontal overhead of the two rounded-border panes
	// (2 columns each; lipgloss.JoinHorizontal adds no gap between them,
	// verified live against lipgloss v1.1.0). paneWidths reserves these so
	// the joined two-pane render never exceeds the reported terminal width
	// (review pin 2).
	borderCols = 4
)

// paneWidths splits a terminal width total into the releases-list (left) and
// board (right) pane box widths. It reserves borderCols for the two panes'
// rounded borders, so left+right+borderCols <= total (review pin 2), and
// floors the left pane at minLeftPane (Coach pin 1). When total <= 0 (no
// tea.WindowSizeMsg received yet) it returns the legacy fixed widths, leaving
// pre-S03 rendering unchanged.
func paneWidths(total int) (left, right int) {
	if total <= 0 {
		return legacyLeftWidth, legacyRightWidth
	}
	avail := total - borderCols
	if avail < 2 {
		// No room for two bordered panes; degrade gracefully.
		if avail < 1 {
			return 0, 0
		}
		return 1, 0
	}
	// Roughly one-third to the releases list, floored at minLeftPane, but
	// always leaving at least one column for the board.
	left = avail / 3
	if left < minLeftPane {
		left = minLeftPane
	}
	if left > avail-1 {
		left = avail - 1
	}
	right = avail - left
	return left, right
}

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
