package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
)

// TrackInfo holds data about one track, as displayed by the board pane.
type TrackInfo struct {
	ID     string
	Slices []string
	// Depends is the display-ready form of DependsOn (comma-joined), rendered
	// as a track-header badge.
	Depends string
	// DependsOn holds the raw dependency track IDs, used by topoSortTracks to
	// compute dependency-order display.
	DependsOn []string
	State     string
}

// SliceBoardInfo holds live state data for one slice on the board.
type SliceBoardInfo struct {
	ID                 string
	State              string
	LastUpdatedAt      string
	VerificationResult string
	Gate               GateResult
	StateSource        string // from board.SliceState.StateSource
	StateDurability    string // from board.SliceState.StateDurability
}

// BoardView is a Bubble Tea component embedded in the root model.
// It displays the board view for a selected release.
type BoardView struct {
	ReleaseName   string
	SourceRef     string
	Tracks        []TrackInfo
	Slices        map[string]SliceBoardInfo // slice ID -> live data
	Loaded        bool
	Loading       bool     // true while an async LoadBoard is in flight (sworn#82)
	Cursor        int      // index of the selected slice in orderedSlices
	orderedSlices []string // slice IDs in display order
	MergeActive   map[string]bool
	GateResults   map[string]GateResult // per-slice gate check results (S72)

	// GatesLoaded/GatesLoading (sworn#82): gate results are no longer
	// computed as part of LoadBoard — loadGatesCmd computes them on demand.
	GatesLoaded  bool
	GatesLoading bool

	// SortMode controls track display order in View(): "" (zero value) is
	// declaration order; trackSortDeps is dependency order.
	SortMode string
	Height   int
}

// trackSortDeps is the BoardView.SortMode value for dependency-order display.
const trackSortDeps = "deps"

// ToggleSort flips the track display order between declaration order and
// dependency order, then rebuilds orderedSlices so cursor navigation stays in sync.
func (b *BoardView) ToggleSort() {
	if b.SortMode == trackSortDeps {
		b.SortMode = ""
	} else {
		b.SortMode = trackSortDeps
	}
	b.rebuildOrderedSlices()
}

// displayTracks returns b.Tracks in the current SortMode's display order.
func (b *BoardView) displayTracks() []TrackInfo {
	if b.SortMode == trackSortDeps {
		return topoSortTracks(b.Tracks)
	}
	return b.Tracks
}

// rebuildOrderedSlices recomputes orderedSlices (and clamps Cursor) from the
// current display order. Called after LoadBoard and after ToggleSort.
func (b *BoardView) rebuildOrderedSlices() {
	b.orderedSlices = nil
	for _, track := range b.displayTracks() {
		b.orderedSlices = append(b.orderedSlices, track.Slices...)
	}
	if b.Cursor >= len(b.orderedSlices) {
		b.Cursor = len(b.orderedSlices) - 1
	}
	if b.Cursor < 0 {
		b.Cursor = 0
	}
}

// topoSortTracks returns tracks ordered so every track appears after all tracks it
// depends on.
func topoSortTracks(tracks []TrackInfo) []TrackInfo {
	n := len(tracks)
	idx := make(map[string]int, n)
	for i, t := range tracks {
		idx[t.ID] = i
	}

	inDegree := make([]int, n)
	dependents := make([][]int, n)
	for i, t := range tracks {
		for _, dep := range t.DependsOn {
			if j, ok := idx[dep]; ok {
				inDegree[i]++
				dependents[j] = append(dependents[j], i)
			}
		}
	}

	visited := make([]bool, n)
	ordered := make([]TrackInfo, 0, n)
	for len(ordered) < n {
		progressed := false
		for i := range n {
			if !visited[i] && inDegree[i] == 0 {
				visited[i] = true
				ordered = append(ordered, tracks[i])
				progressed = true
				for _, dep := range dependents[i] {
					inDegree[dep]--
				}
			}
		}
		if !progressed {
			for i := range n {
				if !visited[i] {
					visited[i] = true
					ordered = append(ordered, tracks[i])
				}
			}
			break
		}
	}
	return ordered
}

// LoadBoardFromCatalog hydrates the board from a catalog snapshot.
func (b *BoardView) LoadBoardFromCatalog(repoRoot string, rec board.CatalogRecord) error {
	refreshed, err := boardViewFromCatalog(rec)
	if err != nil {
		return err
	}
	*b = *refreshed

	// Startup board loading may decorate the catalog snapshot with live merge
	// presentation. Background catalog refresh deliberately does not use this
	// method: it calls boardViewFromCatalog and preserves the prior decoration,
	// avoiding a second status epoch in the refresh transaction.
	b.MergeActive = map[string]bool{}
	for _, mergeTrackID := range ActiveMerges(repoRoot, rec.Release) {
		trackID := strings.TrimPrefix(mergeTrackID, "merge:")
		if trackID != mergeTrackID {
			b.MergeActive[trackID] = true
		}
	}
	return nil
}

// boardViewFromCatalog performs pure board hydration from one accepted catalog
// record. It must remain free of discovery, database, and filesystem status
// reads so a background refresh cannot combine two snapshot epochs.
func boardViewFromCatalog(rec board.CatalogRecord) (*BoardView, error) {
	if rec.Board == nil {
		return nil, fmt.Errorf("no board snapshot for release %q", rec.Release)
	}

	b := &BoardView{
		ReleaseName: rec.Release,
		SourceRef:   rec.SourceRef,
		Tracks:      catalogTracksToTrackInfos(rec.Board.Tracks, rec.TrackDependsOn),
		Slices:      map[string]SliceBoardInfo{},
		MergeActive: map[string]bool{},
		GateResults: map[string]GateResult{},
		Loaded:      true,
	}
	for _, track := range rec.Board.Tracks {
		for _, ss := range track.Slices {
			b.Slices[ss.ID] = sliceBoardInfoFromCatalog(ss)
		}
	}
	b.rebuildOrderedSlices()
	return b, nil
}

// LoadBoard preserves the existing public signature but resolves the target board
// snapshot strictly from board.DiscoverCatalog, not from index.md or any local
// filesystem fallback parser.
func (b *BoardView) LoadBoard(repoRoot, releaseName string) error {
	catalog, err := board.DiscoverCatalog(git.New(repoRoot))
	if err != nil {
		return fmt.Errorf("discover catalog for %q: %w", releaseName, err)
	}

	// Prefer the currently selected sourceRef when available so refreshes (for
	// example from blocked view) do not silently hop to a newer/older topology.
	if b.SourceRef != "" {
		for _, rec := range catalog {
			if rec.Release == releaseName && rec.SourceRef == b.SourceRef {
				return b.LoadBoardFromCatalog(repoRoot, rec)
			}
		}
	}

	for _, rec := range catalog {
		if rec.Release == releaseName {
			return b.LoadBoardFromCatalog(repoRoot, rec)
		}
	}
	return fmt.Errorf("release %q not found in catalog", releaseName)
}

// boardLoadedMsg delivers the result of an async LoadBoard call dispatched by
// loadBoardCmd (sworn#82). releaseName and sourceRef are carried so Update() can
// discard stale loads.
type boardLoadedMsg struct {
	releaseName string
	sourceRef   string
	board       *BoardView
	err         error
}

// loadBoardCmd returns a tea.Cmd that loads a release board from a catalog
// snapshot off the bubbletea Update goroutine.
func loadBoardCmd(repoRoot string, rec board.CatalogRecord) tea.Cmd {
	return func() tea.Msg {
		bv := &BoardView{}
		err := bv.LoadBoardFromCatalog(repoRoot, rec)
		return boardLoadedMsg{releaseName: rec.Release, sourceRef: rec.SourceRef, board: bv, err: err}
	}
}

// catalogTracksToTrackInfos translates board.TrackState entries to tui TrackInfo.
func catalogTracksToTrackInfos(tracks []board.TrackState, dependencies map[string][]string) []TrackInfo {
	out := make([]TrackInfo, 0, len(tracks))
	for _, t := range tracks {
		dependsOn := dependencies[t.ID]
		out = append(out, TrackInfo{
			ID:        t.ID,
			Slices:    append([]string(nil), boardTrackSliceIDs(t.Slices)...),
			Depends:   strings.Join(dependsOn, ", "),
			DependsOn: append([]string(nil), dependsOn...),
			State:     t.State,
		})
	}
	return out
}

// boardTrackSliceIDs extracts slice IDs from board.SliceState entries.
func boardTrackSliceIDs(states []board.SliceState) []string {
	ids := make([]string, 0, len(states))
	for _, ss := range states {
		ids = append(ids, ss.ID)
	}
	return ids
}

// sliceBoardInfoFromCatalog builds SliceBoardInfo from an authoritative
// catalog entry.
func sliceBoardInfoFromCatalog(ss board.SliceState) SliceBoardInfo {
	lastUp := ss.LastUpdated
	if lastUp == "" {
		lastUp = "—"
	}
	return SliceBoardInfo{
		ID:                 ss.ID,
		State:              string(ss.State),
		LastUpdatedAt:      formatLastUpdated(lastUp),
		VerificationResult: ss.VerificationResult,
		StateSource:        ss.StateSource,
		StateDurability:    ss.StateDurability,
	}
}

// formatLastUpdated reformats a timestamp for display.
func formatLastUpdated(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Try ISO 8601 without timezone.
		t, err = time.Parse("2006-01-02T15:04:05", ts)
		if err != nil {
			return ts
		}
	}

	since := time.Since(t)
	if since < 24*time.Hour {
		h := int(since.Hours())
		m := int(since.Minutes()) % 60
		if h > 0 {
			return fmt.Sprintf("%dh%dm ago", h, m)
		}
		return fmt.Sprintf("%dm ago", m)
	}
	return t.Format("Jan _2")
}

// View renders the board view pane for the currently selected release.
func (b *BoardView) View() string {
	if b.Loading {
		title := "Board"
		if b.ReleaseName != "" {
			title = "Board: " + b.ReleaseName
		}
		return BoardTitle.Render(title) + "\n" + EmptyMessage.Render("Loading…")
	}
	if !b.Loaded {
		return BoardTitle.Render("Board") + "\n" +
			EmptyMessage.Render("Select a release from the left pane")
	}

	title := "Board: " + b.ReleaseName
	if b.SortMode == trackSortDeps {
		title += "  (sorted: dependency order)"
	} else {
		title += "  (sorted: declaration order)"
	}
	if len(b.Tracks) == 0 {
		return BoardTitle.Render(title) + "\n" + EmptyMessage.Render("No tracks defined")
	}

	lines := make([]string, 0, len(b.orderedSlices)+len(b.Tracks)*2)
	selectedLine := 0
	selectedID := ""
	if b.Cursor >= 0 && b.Cursor < len(b.orderedSlices) {
		selectedID = b.orderedSlices[b.Cursor]
	}
	for _, track := range b.displayTracks() {
		stateCol := StateColor(track.State)
		header := fmt.Sprintf("▸ %s  [%s]", track.ID, stateCol)
		if track.Depends != "" {
			header += " " + DependsBadge.Render(fmt.Sprintf("(needs: %s)", track.Depends))
		}
		if b.MergeActive[track.ID] {
			header += " " + MergeBadge.Render("⟪merge⟫")
		}
		lines = append(lines, TrackHeader.Render(header))
		for _, sliceID := range track.Slices {
			si, ok := b.Slices[sliceID]
			if !ok {
				si = SliceBoardInfo{ID: sliceID, State: "unknown", LastUpdatedAt: "—"}
			}
			sliceState := SliceStateColor(si.State, si.VerificationResult)
			gateLine := b.renderGateLine(si)
			line := fmt.Sprintf("  %s  %s  (%s)  %s", sliceID, sliceState, si.LastUpdatedAt, gateLine)
			if si.StateDurability == "uncommitted" {
				line += " [uncommitted]"
			}
			if len(b.orderedSlices) > 0 && b.Cursor >= 0 && b.Cursor < len(b.orderedSlices) && b.orderedSlices[b.Cursor] == sliceID {
				selectedLine = len(lines)
				lines = append(lines, BoardItemSelected.Render("▸"+line[1:]))
			} else {
				lines = append(lines, SliceItem.Render(line))
			}
		}
		lines = append(lines, "")
	}
	if b.Height > 0 && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if selectedID == "" {
		selectedLine = 0
	}

	if b.Height > 0 {
		budget := max(0, b.Height-1)
		start, end := cursorWindow(len(lines), selectedLine, budget)
		lines = lines[start:end]
	}

	var sb strings.Builder
	sb.WriteString(BoardTitle.Render(title))
	if len(lines) > 0 {
		sb.WriteString("\n")
		if b.Height == 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(strings.Join(lines, "\n"))
	}

	return sb.String()
}

// renderGateLine renders the gate badge for one slice row.
func (b *BoardView) renderGateLine(si SliceBoardInfo) string {
	switch {
	case b.GatesLoading:
		return GateNeutralStyle.Render("[computing…]")
	case b.GatesLoaded:
		return si.Gate.RenderInline()
	default:
		return GateNeutralStyle.Render("[· press g]")
	}
}
