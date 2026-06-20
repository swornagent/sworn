package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// viewState is the root model's state machine.
type viewState int

const (
	viewReleases viewState = iota
	viewBoard
	viewQuit
)

// Model is the root Bubble Tea model for the sworn TUI.
// It composes ReleasesList (left pane) and BoardView (right pane).
//
// Exported fields are extension points for S04b/S04c:
//   - S04b upgrades BoardView for live data (replace m.Board)
//   - S04c adds TL;DR overlay
type Model struct {
	// Internal state.
	state viewState
	// Show help overlay.
	showHelp bool
	// Repo root (discovered via git).
	repoRoot string
	// Error message (shown when something fails).
	errMsg string

	// Composed components (exported for S04b/S04c).
	Releases *ReleasesList
	Board    *BoardView
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		return m, nil
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.state == viewQuit {
		return ""
	}

	left := m.Releases.View()
	right := m.Board.View()

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		ReleaseListStyle.Render(left),
		BoardStyle.Render(right),
	)

	if m.errMsg != "" {
		errStyle := lipgloss.NewStyle().
			Foreground(colFail).
			Bold(true).
			Padding(0, 2)
		body += "\n" + errStyle.Render("Error: " + m.errMsg)
	}

	help := m.renderHelp()
	return body + "\n" + help
}

// handleKey dispatches keyboard input based on current state.
func (m *Model) handleKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		if !m.showHelp {
			m.state = viewQuit
			return m, tea.Quit
		}
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	}

	if m.showHelp {
		return m, nil
	}

	switch m.state {
	case viewReleases:
		return m.handleReleasesKey(msg)
	case viewBoard:
		return m.handleBoardKey(msg)
	}
	return m, nil
}

// handleReleasesKey handles keyboard input in the releases list view.
func (m *Model) handleReleasesKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.Releases.Cursor < len(m.Releases.Releases)-1 {
			m.Releases.Cursor++
		}
	case "k", "up":
		if m.Releases.Cursor > 0 {
			m.Releases.Cursor--
		}
	case "enter":
		if len(m.Releases.Releases) > 0 {
			sel := m.Releases.Releases[m.Releases.Cursor]
			if err := m.Board.LoadBoard(m.repoRoot, sel.ID); err != nil {
				m.errMsg = err.Error()
			}
			m.state = viewBoard
		}
	case "esc":
	}
	return m, nil
}

// handleBoardKey handles keyboard input in the board view.
func (m *Model) handleBoardKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = viewReleases
	}
	return m, nil
}

// renderHelp renders the bottom help bar.
func (m *Model) renderHelp() string {
	if m.showHelp {
		return HelpBar.Render(`
? help     ↑/k up     ↓/j down     enter select     esc back     q quit`)
	}
	return HelpBar.Render(fmt.Sprintf(
		"%s help  %s up  %s down  %s select  %s back  %s quit",
		HelpKey.Render("?"),
		HelpKey.Render("↑/k"),
		HelpKey.Render("↓/j"),
		HelpKey.Render("enter"),
		HelpKey.Render("esc"),
		HelpKey.Render("q"),
	))
}