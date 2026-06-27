package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// viewState is the root model's state machine.
type viewState int

const (
	viewReleases  viewState = iota
	viewBoard
	viewLive
	viewBlocked
	viewSettings
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

	// Credit balance (loaded at startup from ~/.config/sworn/credits.json).
	creditBalance string

	// Composed components (exported for S04b/S04c).
	Releases *ReleasesList
	Board    *BoardView

	// S04b: Live is the concurrent status view. Non-nil only when the user
	// has navigated to a release with live tracks (or pressed l from board).
	Live *LiveView

	// S04c: Blocked is the blocked/failed slice resolution view.
	Blocked *BlockedView

	// S17: Settings is the provider/model configuration panel.
	Settings *SettingsView
}
// Init implements tea.Model. Loads the credit balance at startup.
func (m *Model) Init() tea.Cmd {
	bal, _ := CreditFileBalance()
	m.creditBalance = bal
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		return m, nil
	case tickMsg:
		// Forward tickMsg to LiveView when in live view. This keeps the poll
		// chain alive — LiveView.Update() increments TickCount, polls the DB,
		// and returns a new tickCmd that fires the next tickMsg.
		if m.state == viewLive && m.Live != nil {
			lm, cmd := m.Live.Update(msg)
			m.Live = lm
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.state == viewQuit {
		return ""
	}

	// Live view replaces the two-pane layout entirely.
	if m.state == viewLive {
		body := m.Live.View()
		body += "\n" + m.renderCreditBar()
		help := m.renderHelp()
		return body + "\n" + help
	}

	// Blocked view replaces the two-pane layout entirely.
	if m.state == viewBlocked && m.Blocked != nil {
		return m.Blocked.View()
	}

		// Settings view replaces the two-pane layout entirely.
		if m.state == viewSettings && m.Settings != nil {
			return m.Settings.View()
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
		body += "\n" + errStyle.Render("Error: "+m.errMsg)
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
	case viewLive:
		return m.handleLiveKey(msg)
	case viewBlocked:
		return m.handleBlockedKey(msg)
	case viewSettings:
		return m.handleSettingsKey(msg)
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

			// Auto-transition to live view if tracks are in-progress.
			if HasInProgressTracks(m.repoRoot, sel.ID) {
				lv, err := StartLiveView(m.repoRoot, sel.ID)
				if err == nil {
					m.Live = lv
					m.state = viewLive
					return m, lv.Init()
				}
		}
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
		return m, nil
	case "j", "down":
		if m.Board.Cursor < len(m.Board.orderedSlices)-1 {
			m.Board.Cursor++
		}
	case "k", "up":
		if m.Board.Cursor > 0 {
			m.Board.Cursor--
		}
	case "enter":
		if len(m.Board.orderedSlices) > 0 {
			sliceID := m.Board.orderedSlices[m.Board.Cursor]
			si, ok := m.Board.Slices[sliceID]
			// Pin 1: check BOTH failed_verification AND implemented+blocked verdict.
			// A BLOCKED verifier verdict leaves the slice at state "implemented"
			// with verification.result == "blocked" — it is NOT "failed_verification".
			if ok && (si.State == "failed_verification" ||
				(si.State == "implemented" && si.VerificationResult == "blocked")) {
				bv, err := LoadBlockedView(m.repoRoot, m.Board.ReleaseName, sliceID)
				if err != nil {
					m.errMsg = err.Error()
					return m, nil
				}
				m.Blocked = bv
				m.state = viewBlocked
				return m, nil
		}
		}
	case "l":
		// Switch to live view if available.
		if m.Live != nil {
			m.state = viewLive
			return m, m.Live.Init()
		}
		// If no LiveView yet, try to start one for the current release.
		if m.Board.ReleaseName != "" {
			lv, err := StartLiveView(m.repoRoot, m.Board.ReleaseName)
			if err != nil {
				m.errMsg = err.Error()
				return m, nil
		}
			m.Live = lv
			m.state = viewLive
			return m, lv.Init()
		}
		case "s":
			// Open settings panel (S17).
			sv, err := NewSettingsView()
			if err != nil {
				m.errMsg = err.Error()
				return m, nil
		}
			m.Settings = sv
			m.state = viewSettings
			return m, nil
	}
	return m, nil
}

// handleLiveKey handles keyboard input in the live view.
func (m *Model) handleLiveKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = viewReleases
		return m, nil
	case "b":
		m.state = viewBoard
		return m, nil
	}
	return m, nil
}

// handleBlockedKey handles keyboard input in the blocked view.
func (m *Model) handleBlockedKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	if m.Blocked != nil {
		if msg.String() == "esc" && !m.Blocked.viewingProof && !m.Blocked.deferring {
			m.state = viewBoard
			// Reload board to reflect any state changes (e.g. deferred)
			if err := m.Board.LoadBoard(m.repoRoot, m.Board.ReleaseName); err != nil {
				m.errMsg = err.Error()
		}
			return m, nil
		}
		bm, cmd := m.Blocked.Update(msg)
		m.Blocked = bm
		return m, cmd
	}
	return m, nil
}


// handleSettingsKey handles keyboard input in the settings view.
func (m *Model) handleSettingsKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	if m.Settings == nil {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		// If not editing a field, Esc discards and returns to board.
		editing := false
		for _, f := range m.Settings.fields {
			if f.editing {
				editing = true
				break
			}
		}
		if !editing {
			m.state = viewBoard
			m.Settings = nil
			return m, nil
		}
	case "ctrl+s":
		// Save and return to board.
		model, _ := m.Settings.save()
		m.Settings = model.(*SettingsView)
		if m.Settings.message == "Saved!" {
			m.state = viewBoard
			m.Settings = nil
			return m, nil
		}
		return m, nil
	}
	model, cmd := m.Settings.Update(msg)
	m.Settings = model.(*SettingsView)
	return m, cmd
}

// renderHelp renders the bottom help bar.
func (m *Model) renderHelp() string {
	if m.showHelp {
		return HelpBar.Render(`
	? help     ↑/k up     ↓/j down     enter select     l live     b board     s settings     esc back     q quit`)
	}
	return HelpBar.Render(fmt.Sprintf(
		"%s help  %s up  %s down  %s select  %s live  %s board  %s settings  %s back  %s quit",
		HelpKey.Render("?"),
		HelpKey.Render("↑/k"),
		HelpKey.Render("↓/j"),
		HelpKey.Render("enter"),
		HelpKey.Render("l"),
		HelpKey.Render("b"),
		HelpKey.Render("s"),
		HelpKey.Render("esc"),
		HelpKey.Render("q"),
	))
}

// renderCreditBar renders the credit balance line for the live view.
func (m *Model) renderCreditBar() string {
	return lipgloss.NewStyle().
		Foreground(colDim).
		Padding(0, 2).
		Render(fmt.Sprintf("Credits: %s", m.creditBalance))
}
