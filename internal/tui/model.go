package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
)

const (
	catalogRefreshInterval = 5 * time.Second
	initialCatalogLimit    = 10
	catalogLimitIncrement  = 10
)

type catalogRefreshDueMsg struct {
	generation uint64
}

type catalogRefreshResultMsg struct {
	generation uint64
	limit      int
	window     board.CatalogWindow
	// catalog is retained for deterministic legacy tests that inject complete
	// snapshots; production commands always populate window.
	catalog []board.CatalogRecord
	err     error
}

// viewState is the root model's state machine.
type viewState int

const (
	viewReleases viewState = iota
	viewBoard
	viewLive
	viewLog
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
	// refreshErr is owned exclusively by the catalog-refresh chain. A later
	// successful refresh clears it without masking an unrelated root error.
	refreshErr string

	// Catalog refresh is a serial completion-relative chain. generation names
	// the sole requested/accepted transaction and refreshInFlight prevents a
	// duplicate due message from starting overlapping discovery.
	refreshGeneration    uint64
	refreshInFlight      bool
	discoverCatalog      func(string) ([]board.CatalogRecord, error)
	discoverWindow       func(string, int) (board.CatalogWindow, error)
	desiredCatalogLimit  int
	acceptedCatalogLimit int

	// Credit balance (loaded at startup from ~/.config/sworn/credits.json).
	creditBalance string

	// Width and Height are the real terminal dimensions, stored from every
	// tea.WindowSizeMsg (S03). Width drives all responsive sizing in this
	// slice: pane widths (via paneWidths) and the full-width header/help
	// bars. Height is stored per AC-01 but is NOT yet used for sizing — it is
	// retained for tracked future vertical pagination (design.md design-level
	// risk: no releases-list/board pagination exists before or after this
	// slice). Both are 0 until the first WindowSizeMsg arrives, in which case
	// the render paths fall back to their legacy fixed widths.
	Width  int
	Height int

	// Version is the sworn binary version (the value `sworn --version`
	// reports), passed in from cmd/sworn via tui.Run and shown in the header
	// (S03, AC-03).
	Version string

	// Composed components (exported for S04b/S04c).
	Releases *ReleasesList
	Board    *BoardView

	// S04b: Live is the concurrent status view. Non-nil only when the user
	// has navigated to a release with live tracks (or pressed l from board).
	Live *LiveView

	// Log is the live log view (per-track or consolidated). Non-nil only while
	// state == viewLog, opened by enter on a live row or L from live/board.
	Log *LogView

	// S04c: Blocked is the blocked/failed slice resolution view.
	Blocked *BlockedView

	// S17: Settings is the provider/model configuration panel.
	Settings *SettingsView
}

// Init implements tea.Model. Loads the credit balance at startup.
func (m *Model) Init() tea.Cmd {
	bal, _ := CreditFileBalance()
	m.creditBalance = bal
	if m.refreshGeneration == 0 {
		m.refreshGeneration = 1
	}
	if m.desiredCatalogLimit == 0 {
		m.desiredCatalogLimit = initialCatalogLimit
	}
	return scheduleCatalogRefreshCmd(m.refreshGeneration)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		// Store the real terminal dimensions (S03). Previously discarded,
		// which forced every pane to its hardcoded width regardless of the
		// actual terminal — the root cause of the wrapping/viewport bugs.
		m.Width = msg.Width
		m.Height = msg.Height
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
	case logTickMsg:
		// Forward to LogView ONLY when in the log view (Captain pin M1). A
		// logTickMsg arriving in any other state is a stray from a since-left
		// log view: dropped here, and because it is not re-armed the old chain
		// dies — no tick doubling. Symmetrically, a tickMsg arriving in viewLog
		// is dropped by the tickMsg case above.
		if m.state == viewLog && m.Log != nil {
			lm, cmd := m.Log.Update(msg)
			m.Log = lm
			return m, cmd
		}
		return m, nil
	case catalogRefreshDueMsg:
		if msg.generation != m.refreshGeneration || m.refreshInFlight {
			return m, nil
		}
		m.refreshInFlight = true
		m.Releases.LoadingOlder = m.desiredCatalogLimit > m.acceptedCatalogLimit
		return m, discoverCatalogWindowCmd(m.repoRoot, msg.generation, m.desiredCatalogLimit, m.discoverWindow, m.discoverCatalog)
	case catalogRefreshResultMsg:
		limit := msg.limit
		if limit <= 0 {
			limit = m.desiredCatalogLimit
			if limit <= 0 {
				limit = initialCatalogLimit
				m.desiredCatalogLimit = limit
			}
		}
		window := msg.window
		if window.Records == nil && msg.catalog != nil {
			window.Records = msg.catalog
		}
		if msg.generation != m.refreshGeneration || !m.refreshInFlight {
			return m, nil
		}
		m.refreshInFlight = false
		if limit < m.desiredCatalogLimit {
			m.refreshGeneration++
			m.refreshInFlight = true
			m.Releases.LoadingOlder = true
			return m, discoverCatalogWindowCmd(m.repoRoot, m.refreshGeneration, m.desiredCatalogLimit, m.discoverWindow, m.discoverCatalog)
		}
		m.Releases.LoadingOlder = false
		if msg.err != nil {
			m.refreshErr = msg.err.Error()
		} else if err := m.applyCatalogRefresh(window, limit); err != nil {
			m.refreshErr = err.Error()
		} else {
			m.refreshErr = ""
		}
		m.refreshGeneration++
		return m, scheduleCatalogRefreshCmd(m.refreshGeneration)
	case boardLoadedMsg:
		// Delivered by loadBoardCmd (sworn#82). Discard a stale load — the
		// user may have navigated to a different release or catalog ref before
		// this one finished — rather than clobbering what's now on screen.
		if msg.releaseName != m.Board.ReleaseName {
			return m, nil
		}
		if msg.sourceRef != m.Board.SourceRef {
			return m, nil
		}
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.Board.Loading = false
			return m, nil
		}
		msg.board.Loading = false
		msg.board.Height = m.Board.Height
		m.Board = msg.board
		return m, nil
	case gatesLoadedMsg:
		// Delivered by loadGatesCmd (sworn#82's on-demand 'g' keybinding).
		// Same staleness guard as boardLoadedMsg.
		if msg.releaseName != m.Board.ReleaseName {
			return m, nil
		}
		m.Board.GatesLoading = false
		m.Board.GatesLoaded = true
		m.Board.GateResults = msg.results
		for sid, gr := range msg.results {
			si := m.Board.Slices[sid]
			si.Gate = gr
			m.Board.Slices[sid] = si
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
		body = m.appendRootErrors(body)
		help := m.renderHelp()
		return m.boundHeight(body + "\n" + help)
	}

	// Log view replaces the two-pane layout entirely.
	if m.state == viewLog && m.Log != nil {
		return m.boundHeight(m.appendRootErrors(m.Log.View()) + "\n" + m.renderHelp())
	}

	// Blocked view replaces the two-pane layout entirely.
	if m.state == viewBlocked && m.Blocked != nil {
		return m.boundHeight(m.appendRootErrors(m.Blocked.View()))
	}

	// Settings view replaces the two-pane layout entirely.
	if m.state == viewSettings && m.Settings != nil {
		return m.boundHeight(m.appendRootErrors(m.Settings.View()))
	}
	// Size the two panes from the real terminal width (S03). paneWidths
	// reserves the border columns and floors the left pane; ReleasesList.View
	// then ellipsis-truncates any over-long label to fit its pane.
	leftW, rightW := paneWidths(m.Width)
	if m.Width > 0 {
		m.Releases.Width = leftW
	} else {
		m.Releases.Width = 0
	}
	header := m.renderHeader()
	help := m.renderHelp()
	errors := m.renderRootErrors()
	contentHeight := 0
	if m.Height > 0 {
		separators := 2
		if errors != "" {
			separators++
		}
		contentHeight = max(0, m.Height-lipgloss.Height(header)-lipgloss.Height(help)-lipgloss.Height(errors)-separators-2)
	}
	m.Releases.Height = contentHeight
	m.Board.Height = contentHeight
	left := m.Releases.View()
	right := m.Board.View()
	leftStyle := focusedPaneStyle(ReleaseListStyle, m.state == viewReleases).Width(leftW)
	rightStyle := focusedPaneStyle(BoardStyle, m.state == viewBoard).Width(rightW)
	if m.Height > 0 {
		leftStyle = leftStyle.Height(contentHeight)
		rightStyle = rightStyle.Height(contentHeight)
	}

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftStyle.Render(left),
		rightStyle.Render(right),
	)
	parts := []string{header, body}
	if errors != "" {
		parts = append(parts, errors)
	}
	parts = append(parts, help)
	return m.boundHeight(strings.Join(parts, "\n"))
}

func focusedPaneStyle(base lipgloss.Style, focused bool) lipgloss.Style {
	colour := colBorder
	if focused {
		colour = colAccent
	}
	return base.Copy().BorderForeground(colour)
}

func (m *Model) renderRootErrors() string {
	errStyle := lipgloss.NewStyle().Foreground(colFail).Bold(true).Padding(0, 2)
	var lines []string
	for _, message := range []string{m.errMsg, m.refreshErr} {
		if message != "" {
			lines = append(lines, errStyle.Render("Error: "+message))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) boundHeight(view string) string {
	if m.Height <= 0 || lipgloss.Height(view) <= m.Height {
		return view
	}
	lines := strings.Split(view, "\n")
	if len(lines) > m.Height {
		lines = lines[:m.Height]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) appendRootErrors(body string) string {
	if errors := m.renderRootErrors(); errors != "" {
		body += "\n" + errors
	}
	return body
}

func scheduleCatalogRefreshCmd(generation uint64) tea.Cmd {
	return tea.Tick(catalogRefreshInterval, func(time.Time) tea.Msg {
		return catalogRefreshDueMsg{generation: generation}
	})
}

func discoverCatalogWindowCmd(repoRoot string, generation uint64, limit int, discoverWindow func(string, int) (board.CatalogWindow, error), discoverComplete func(string) ([]board.CatalogRecord, error)) tea.Cmd {
	return func() tea.Msg {
		var window board.CatalogWindow
		var err error
		switch {
		case discoverWindow != nil:
			window, err = discoverWindow(repoRoot, limit)
		case discoverComplete != nil:
			window.Records, err = discoverComplete(repoRoot)
		default:
			window, err = board.DiscoverCatalogWindow(git.New(repoRoot), limit)
		}
		return catalogRefreshResultMsg{generation: generation, limit: limit, window: window, err: err}
	}
}

// applyCatalogRefresh prepares both replacement components before mutating the
// model, then installs them together. The accepted catalog is the only state
// authority consulted; presentation-only merge/gate decorations are copied
// from the prior board for identities that still exist.
func (m *Model) applyCatalogRefresh(window board.CatalogWindow, limit int) error {
	catalog := window.Records
	releases, conversionErr := releaseInfosFromCatalog(catalog)
	if conversionErr != nil && conversionErr != ErrNoReleases {
		return conversionErr
	}

	previousReleaseIndex := m.Releases.Cursor
	selectedReleaseID := selectedReleaseID(m.Releases)
	selectedBoardRelease := m.Board.ReleaseName
	selectedSliceID := selectedBoardSliceID(m.Board)

	newReleaseIndex := clampIndex(previousReleaseIndex, len(releases))
	if idx := releaseIndexByID(releases, selectedReleaseID); idx >= 0 {
		newReleaseIndex = idx
	}

	var refreshedBoard *BoardView
	if selectedBoardRelease != "" {
		if rec, ok := catalogRecordByRelease(catalog, selectedBoardRelease); ok {
			var err error
			refreshedBoard, err = boardViewFromCatalog(rec)
			if err != nil {
				return err
			}
			preserveBoardPresentation(refreshedBoard, m.Board, selectedSliceID)
		}
	}

	m.Releases.Releases = releases
	m.Releases.Cursor = newReleaseIndex
	m.Releases.HasOlder = window.HasOlder
	m.Releases.LoadingOlder = false
	m.acceptedCatalogLimit = limit
	if refreshedBoard != nil {
		m.Board = refreshedBoard
	} else if selectedBoardRelease != "" {
		m.Board = &BoardView{}
		if m.state == viewBoard {
			m.state = viewReleases
		}
	}

	return conversionErr
}

func selectedReleaseID(releases *ReleasesList) string {
	if releases != nil && releases.Cursor >= 0 && releases.Cursor < len(releases.Releases) {
		return releases.Releases[releases.Cursor].ID
	}
	return ""
}

func selectedBoardSliceID(boardView *BoardView) string {
	if boardView != nil && boardView.Cursor >= 0 && boardView.Cursor < len(boardView.orderedSlices) {
		return boardView.orderedSlices[boardView.Cursor]
	}
	return ""
}

func releaseIndexByID(releases []ReleaseInfo, id string) int {
	for i := range releases {
		if releases[i].ID == id {
			return i
		}
	}
	return -1
}

func catalogRecordByRelease(catalog []board.CatalogRecord, release string) (board.CatalogRecord, bool) {
	for i := range catalog {
		if catalog[i].Release == release {
			return catalog[i], true
		}
	}
	return board.CatalogRecord{}, false
}

func clampIndex(index, length int) int {
	if length == 0 || index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func preserveBoardPresentation(refreshed, previous *BoardView, selectedSliceID string) {
	refreshed.SortMode = previous.SortMode
	refreshed.MergeActive = make(map[string]bool)
	for _, track := range refreshed.Tracks {
		if previous.MergeActive[track.ID] {
			refreshed.MergeActive[track.ID] = true
		}
	}

	refreshed.GatesLoaded = previous.GatesLoaded
	refreshed.GatesLoading = previous.GatesLoading
	refreshed.GateResults = make(map[string]GateResult)
	for id, slice := range refreshed.Slices {
		if old, ok := previous.Slices[id]; ok {
			slice.Gate = old.Gate
			refreshed.Slices[id] = slice
		}
		if gate, ok := previous.GateResults[id]; ok {
			refreshed.GateResults[id] = gate
		}
	}

	refreshed.rebuildOrderedSlices()
	if idx := boardSliceIndex(refreshed, selectedSliceID); idx >= 0 {
		refreshed.Cursor = idx
	} else {
		refreshed.Cursor = clampIndex(previous.Cursor, len(refreshed.orderedSlices))
	}
}

func boardSliceIndex(boardView *BoardView, id string) int {
	for i, candidate := range boardView.orderedSlices {
		if candidate == id {
			return i
		}
	}
	return -1
}

// renderHeader renders the top header bar (S03, AC-03): the sworn version and
// the currently-selected release. The release label is "no release selected"
// on the initial releases screen (never navigated into a release) and the
// selected release name otherwise — sourced from the TUI's own navigation
// state (Board.ReleaseName), which persists across `esc` back to the list.
func (m *Model) renderHeader() string {
	label := m.Board.ReleaseName
	if label == "" {
		label = "no release selected"
	}
	w := m.Width
	if w <= 0 {
		w = legacyHelpWidth
	}
	content := fmt.Sprintf("sworn %s  •  %s", m.Version, label)
	return HeaderStyle.Copy().Width(w).Render(content)
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
	case viewLog:
		return m.handleLogKey(msg)
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
	case "enter", "right":
		return m.openSelectedRelease()
	case "o":
		return m.requestOlderCatalog()
	case "esc":
	}
	return m, nil
}

func (m *Model) openSelectedRelease() (*Model, tea.Cmd) {
	if len(m.Releases.Releases) == 0 {
		return m, nil
	}
	sel := m.Releases.Releases[m.Releases.Cursor]
	if sel.Catalog == nil {
		m.errMsg = "no catalog snapshot for selected release"
		return m, nil
	}
	m.Board.ReleaseName = sel.ID
	m.Board.SourceRef = sel.SourceRef
	m.Board.Loaded = false
	m.Board.Loading = true
	m.Board.GateResults = nil
	m.Board.GatesLoaded = false
	m.Board.GatesLoading = false
	m.state = viewBoard
	cmds := []tea.Cmd{loadBoardCmd(m.repoRoot, *sel.Catalog)}
	if HasInProgressTracks(m.repoRoot, sel.ID) {
		lv, err := StartLiveView(m.repoRoot, sel.ID)
		if err == nil {
			m.Live = lv
			m.state = viewLive
			cmds = append(cmds, lv.Init())
		}
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) requestOlderCatalog() (*Model, tea.Cmd) {
	if !m.Releases.HasOlder {
		return m, nil
	}
	if m.desiredCatalogLimit <= 0 {
		m.desiredCatalogLimit = initialCatalogLimit
	}
	m.desiredCatalogLimit += catalogLimitIncrement
	m.Releases.LoadingOlder = true
	if m.refreshInFlight {
		return m, nil
	}
	m.refreshGeneration++
	m.refreshInFlight = true
	return m, discoverCatalogWindowCmd(m.repoRoot, m.refreshGeneration, m.desiredCatalogLimit, m.discoverWindow, m.discoverCatalog)
}

// handleBoardKey handles keyboard input in the board view.
func (m *Model) handleBoardKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "left":
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
	case "L":
		// Open the consolidated log for the current release without first
		// entering the live table (second entry point; Rule 1 affordance owned
		// by the root Model key dispatch). esc returns here to the board.
		if m.Board.ReleaseName != "" {
			m.Log = StartLogView(m.repoRoot, m.Board.ReleaseName, "", viewBoard, m.Height)
			m.state = viewLog
			return m, m.Log.Init()
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
	case "g":
		// Compute gate results for the current release on demand (sworn#82)
		// — LoadGateResults shells `git diff` per slice and is no longer run
		// automatically on every board load. Dispatched as a tea.Cmd, same
		// as the board load itself, so it can't block Update either.
		if m.Board.ReleaseName != "" && !m.Board.GatesLoading {
			m.Board.GatesLoading = true
			return m, loadGatesCmd(m.repoRoot, m.Board.ReleaseName)
		}
	case "o":
		// Toggle track display order between declaration order and
		// dependency (topological) order.
		m.Board.ToggleSort()
		return m, nil
	}
	return m, nil
}

// handleLiveKey handles keyboard input in the live view.
//
// The row cursor (j/k) + enter + L are net-new here (Captain pin M4: the
// "j/k/enter idiom" the design cited actually lived in handleBoardKey, not
// handleLiveKey). enter opens the selected track's log; L opens the
// consolidated interleave. esc/b keep their existing destinations.
func (m *Model) handleLiveKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = viewReleases
		return m, nil
	case "b":
		m.state = viewBoard
		return m, nil
	case "j", "down":
		if m.Live != nil && m.Live.Cursor < len(m.Live.Rows)-1 {
			m.Live.Cursor++
		}
		return m, nil
	case "k", "up":
		if m.Live != nil && m.Live.Cursor > 0 {
			m.Live.Cursor--
		}
		return m, nil
	case "enter":
		if m.Live != nil && len(m.Live.Rows) > 0 {
			track := m.Live.Rows[m.Live.Cursor].ID
			m.Log = StartLogView(m.repoRoot, m.Live.ReleaseName, track, viewLive, m.Height)
			m.state = viewLog
			return m, m.Log.Init()
		}
		return m, nil
	case "L":
		if m.Live != nil {
			m.Log = StartLogView(m.repoRoot, m.Live.ReleaseName, "", viewLive, m.Height)
			m.state = viewLog
			return m, m.Log.Init()
		}
		return m, nil
	}
	return m, nil
}

// handleLogKey handles keyboard input in the log view: scrollback + follow, and
// esc back to the originating view (Captain pin M4 — the back-stack is
// consistent because LogView.origin records where it was opened from).
func (m *Model) handleLogKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	if m.Log == nil {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.state = m.Log.origin
		return m, nil
	case "b":
		m.state = viewBoard
		return m, nil
	case "k", "up":
		m.Log.scrollUp()
	case "j", "down":
		m.Log.scrollDown()
	case "g":
		m.Log.top()
	case "G":
		m.Log.bottom()
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

// renderHelp renders the bottom help bar as a single background-styled bar
// spanning the full terminal width (S03, AC-04). Falls back to the legacy
// fixed width when no tea.WindowSizeMsg has been received yet.
func (m *Model) renderHelp() string {
	w := m.Width
	if w <= 0 {
		w = legacyHelpWidth
	}
	bar := HelpBar.Copy().Width(w)
	if m.showHelp {
		return bar.Render(`
	? help     ↑/k up     ↓/j down     enter select     l live     L logs     b board     g gates     o order     s settings     esc back     q quit`)
	}
	if m.state == viewReleases {
		return bar.Render(fmt.Sprintf(
			"%s help  %s up  %s down  %s  %s  %s quit",
			HelpKey.Render("?"), HelpKey.Render("↑/k"), HelpKey.Render("↓/j"),
			HelpKey.Render("right/enter board"), HelpKey.Render("o older"), HelpKey.Render("q"),
		))
	}
	if m.state == viewBoard {
		return bar.Render(fmt.Sprintf(
			"%s help  %s up  %s down  %s  %s  %s gates  %s quit",
			HelpKey.Render("?"), HelpKey.Render("↑/k"), HelpKey.Render("↓/j"),
			HelpKey.Render("left/esc releases"), HelpKey.Render("o order"), HelpKey.Render("g"), HelpKey.Render("q"),
		))
	}
	return bar.Render(fmt.Sprintf(
		"%s help  %s up  %s down  %s select  %s live  %s logs  %s board  %s gates  %s order  %s settings  %s back  %s quit",
		HelpKey.Render("?"),
		HelpKey.Render("↑/k"),
		HelpKey.Render("↓/j"),
		HelpKey.Render("enter"),
		HelpKey.Render("l"),
		HelpKey.Render("L"),
		HelpKey.Render("b"),
		HelpKey.Render("g"),
		HelpKey.Render("o"),
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
