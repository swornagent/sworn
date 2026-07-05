package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/state"
)

// TrackInfo holds data about one track, as displayed by the board pane.
type TrackInfo struct {
	ID     string
	Slices []string
	// Depends is the display-ready form of DependsOn (comma-joined), rendered
	// as a track-header badge.
	Depends string
	// DependsOn holds the raw dependency track IDs (board.BoardTrack.DependsOn),
	// used by topoSortTracks to compute dependency-order display. Kept
	// alongside Depends rather than re-parsing it, since Depends is
	// display-formatted (joined, no defined split-back guarantee).
	DependsOn []string
	State     string
}

// SliceBoardInfo holds live state data for one slice on the board.
type SliceBoardInfo struct {
	ID                 string
	State              string
	LastUpdatedAt      string
	VerificationResult string     // from status.json verification.result (e.g. "blocked")
	Gate               GateResult // per-slice gate check results (S72)
}

// BoardView is a Bubble Tea component embedded in the root model.
// It displays the board view for a selected release.
type BoardView struct {
	ReleaseName   string
	Tracks        []TrackInfo
	Slices        map[string]SliceBoardInfo // slice ID -> live data
	Loaded        bool
	Loading       bool                  // true while an async LoadBoard is in flight (sworn#82)
	Cursor        int                   // index of the selected slice in orderedSlices
	orderedSlices []string              // slice IDs in display order
	MergeActive   map[string]bool       // track IDs with an active merge in flight
	GateResults   map[string]GateResult // per-slice gate check results (S72)

	// GatesLoaded/GatesLoading (sworn#82): gate results are no longer
	// computed as part of LoadBoard — LoadGateResults shells `git diff` per
	// slice (trace once + coverage/design/mock per implemented slice) and
	// was measured as ~100% of the board-load cost (21.3s of 21.5s on a
	// 73-slice release). Gates are now on-demand only, via the 'g'
	// keybinding: GatesLoading is true while that async compute is in
	// flight, GatesLoaded is true once GateResults holds a real (possibly
	// stale, session-cached) result set. Both are false after a fresh
	// LoadBoard, which is what makes the board badges render as "unloaded"
	// until the user asks for gates.
	GatesLoaded  bool
	GatesLoading bool

	// SortMode controls track display order in View(): "" (zero value) is
	// declaration order (the order tracks appear in board.json — numerically
	// T1, T2, T3... by convention); trackSortDeps is dependency order, a
	// topological sort so a track always renders after every track it
	// depends on. Toggled by the 'o' key (handleBoardKey) and preserved
	// across LoadBoard reloads.
	SortMode string
}

// trackSortDeps is the BoardView.SortMode value for dependency-order display.
const trackSortDeps = "deps"

// ToggleSort flips the track display order between declaration order and
// dependency (topological) order, then rebuilds orderedSlices so cursor
// navigation (j/k) stays in sync with the newly displayed order.
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
// current display order. Called after LoadBoard and after ToggleSort — both
// change what "display order" means — so cursor navigation always matches
// what's rendered on screen.
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

// topoSortTracks returns tracks ordered so every track appears after all of
// its DependsOn tracks — a stable topological sort (Kahn's algorithm, ties
// broken by declaration order) so the result is deterministic and matches
// declaration order whenever declaration order already satisfies the
// dependency constraints. A DependsOn reference to a track ID absent from
// this slice (dangling ref) is ignored — display-only ordering, not the
// scheduling gate (internal/run/parallel.go owns enforcement). A dependency
// cycle can't be resolved into a valid order; affected tracks are appended in
// declaration order rather than dropped or looping forever, so toggling sort
// can never make a track disappear from the board.
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

// LoadBoard reads the selected release's board (via internal/board's oracle,
// board.ReadBoard) and all slice status.json files. repoRoot is the repo
// root path.
//
// S02-tui-oracle-migration: previously hand-parsed index.md's `tracks:` YAML
// frontmatter, which `sworn render` stopped emitting once tracks moved to a
// Markdown table — silently producing zero tracks with no error. board.ReadBoard
// reads board.json (the current-format oracle) and lazily migrates from
// index.md frontmatter when board.json is absent, which is the AC-06 legacy
// fallback for genuinely pre-migration releases.
func (b *BoardView) LoadBoard(repoRoot, releaseName string) error {
	b.ReleaseName = releaseName
	b.Tracks = nil
	b.Slices = nil
	b.Loaded = false

	br, err := board.ReadBoard(repoRoot, releaseName)
	if err != nil {
		return fmt.Errorf("reading board for %s: %w", releaseName, err)
	}
	for _, t := range br.Tracks {
		b.Tracks = append(b.Tracks, TrackInfo{
			ID:        t.ID,
			Slices:    t.Slices,
			Depends:   strings.Join(t.DependsOn, ", "),
			DependsOn: append([]string(nil), t.DependsOn...),
			State:     t.State,
		})
	}

	// Load live state from each slice's status.json, resolved via the
	// git-ref oracle (the same ownership-keyed path `sworn board` and the
	// MCP ops tools use — sworn#81) so track-branch work is reflected even
	// before it lands in the primary checkout. The working-tree filesystem
	// read remains the fallback for repos with no usable git history (e.g.
	// a release with no branches at all) or when a slice can't be resolved
	// via any ref.
	b.Slices = map[string]SliceBoardInfo{}
	releaseDir := filepath.Join(repoRoot, "docs", "release", releaseName)
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return fmt.Errorf("reading release dir %s: %w", releaseDir, err)
	}

	sliceOracle := newSliceOracle(repoRoot, releaseName)
	trackMap := trackInfoMap(br.Tracks)
	ctx := context.Background()

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "S") {
			continue
		}
		sliceID := entry.Name()
		statusPath := filepath.Join(releaseDir, sliceID, "status.json")

		if sliceOracle != nil {
			ss, resolvedFrom, errO := sliceOracle.oracle.ReadSliceStatus(ctx, sliceOracle.reader, "", sliceOracle.releaseRef, releaseName, sliceID, trackMap)
			if errO == nil {
				// The resolved ref may BE the branch we're sitting on right
				// now (the common serial/solo `sworn run` shape: one
				// worktree, no separate track worktree). In that case a git
				// commit is not the authoritative source — the filesystem
				// is, since internal/run/slice.go writes status.json
				// repeatedly between commit milestones. Only trust the
				// committed ref as-is when it resolved from a DIFFERENT
				// checkout we have no live filesystem access to (a genuine
				// other worktree's track branch or a release-wt branch that
				// isn't the one checked out here).
				if sliceOracle.resolvedRefIsLiveCheckout(resolvedFrom, ss.Track, trackMap) {
					if st, errR := state.Read(statusPath); errR == nil {
						b.Slices[sliceID] = sliceBoardInfoFromStatus(sliceID, st)
						continue
					}
					// Live checkout but the working-tree file is
					// unreadable (e.g. mid-write) — fall through to the
					// oracle's last-known-good committed value below.
				}
				b.Slices[sliceID] = sliceBoardInfoFromOracle(sliceID, ss)
				continue
			}
		}

		// Fallback: working-tree filesystem read (repos with no usable git
		// history, e.g. a release with no branches/commits at all yet).
		st, errR := state.Read(statusPath)
		if errR != nil {
			b.Slices[sliceID] = SliceBoardInfo{
				ID:    sliceID,
				State: "unknown",
			}
			continue
		}
		b.Slices[sliceID] = sliceBoardInfoFromStatus(sliceID, st)
	}

	b.rebuildOrderedSlices()

	b.Loaded = true

	// Gate results are intentionally NOT computed here (sworn#82) — see the
	// GatesLoaded/GatesLoading doc comment on BoardView. A fresh LoadBoard
	// always resets to "not computed"; the 'g' keybinding (loadGatesCmd)
	// populates GateResults on demand.
	b.GateResults = nil
	b.GatesLoaded = false
	b.GatesLoading = false

	// Populate MergeActive from the events table.
	b.MergeActive = map[string]bool{}
	for _, mergeTrackID := range ActiveMerges(repoRoot, releaseName) {
		// mergeTrackID is "merge:<track-id>"; extract the track-id part.
		trackID := strings.TrimPrefix(mergeTrackID, "merge:")
		if trackID != mergeTrackID { // prefix was found
			b.MergeActive[trackID] = true
		}
	}

	return nil
}

// boardLoadedMsg delivers the result of an async LoadBoard call dispatched
// by loadBoardCmd (sworn#82). releaseName is carried so the receiving
// Update() can detect and discard a stale load — one dispatched for a
// release the user has since navigated away from — instead of clobbering
// whatever board is now on screen.
type boardLoadedMsg struct {
	releaseName string
	board       *BoardView
	err         error
}

// loadBoardCmd returns a tea.Cmd that loads a release's board off the
// bubbletea Update goroutine. Before sworn#82, handleReleasesKey called
// BoardView.LoadBoard directly inline on Enter, which — combined with
// LoadBoard eagerly recomputing gates — blocked the UI for up to 21.5s on a
// 73-slice release (measured). Gates are now lazy (see GatesLoaded on
// BoardView), which took LoadBoard itself down to ~1ms even on that
// release, but the load is still dispatched as a Cmd on principle: board
// loading shells out to git via the slice oracle and must never run
// synchronously inside a key handler, regardless of today's measured cost.
func loadBoardCmd(repoRoot, releaseName string) tea.Cmd {
	return func() tea.Msg {
		bv := &BoardView{}
		err := bv.LoadBoard(repoRoot, releaseName)
		return boardLoadedMsg{releaseName: releaseName, board: bv, err: err}
	}
}

// tuiOracleReader adapts *git.Repo to internal/board's (unexported)
// gitContentReader interface, satisfied structurally — the same pattern
// cmd/sworn/board.go's oracleReader and internal/mcp's oracleReader use.
type tuiOracleReader struct {
	repo *git.Repo
}

func (r tuiOracleReader) Show(ref, path string) (string, error) {
	return r.repo.Show(ref, path)
}

func (r tuiOracleReader) CatFileExists(ref, path string) (bool, error) {
	return r.repo.CatFileExists(ref, path)
}

// sliceOracleContext bundles a board.Oracle with the reader and release ref
// LoadBoard resolves once per call, so the per-slice loop can call
// ReadSliceStatus without re-deriving them each iteration.
type sliceOracleContext struct {
	oracle     *board.Oracle
	reader     tuiOracleReader
	releaseRef string
	// currentBranch is the branch checked out in repoRoot at LoadBoard time
	// (empty when detached or unresolvable). Used to detect when an
	// oracle-resolved ref is actually THIS checkout, in which case the
	// working-tree filesystem — not the last commit — is authoritative.
	currentBranch string
}

// newSliceOracle returns a sliceOracleContext backed by repoRoot's git repo,
// or nil when repoRoot has no usable git history (e.g. a plain filesystem
// fixture, or a release with no commits yet) — LoadBoard falls back to the
// working-tree filesystem read in that case.
func newSliceOracle(repoRoot, releaseName string) *sliceOracleContext {
	repo := git.New(repoRoot)
	if _, err := repo.RevParse("HEAD"); err != nil {
		return nil
	}
	reader := tuiOracleReader{repo: repo}
	branch, _ := repo.CurrentBranch() // best-effort; "" (e.g. detached) just disables the live-checkout match for track/release-wt refs
	return &sliceOracleContext{
		oracle:        board.NewGitOracle(repo),
		reader:        reader,
		releaseRef:    resolveOracleReleaseRef(reader, releaseName),
		currentBranch: branch,
	}
}

// resolvedRefIsLiveCheckout reports whether the git ref that
// board.Oracle.ReadSliceStatus resolved a slice's status.json from is the
// SAME checkout LoadBoard is running against — i.e. the filesystem read
// would see everything the ref saw, plus anything written since the last
// commit. This holds when:
//   - the resolution fell through to plain "HEAD" (board.ResolvedByWorkingTree):
//     HEAD is always repoRoot's own commit, by construction;
//   - the resolution came from the owner track's branch and that branch is
//     the one checked out here (a serial/solo `sworn run` with no separate
//     track worktree);
//   - the resolution came from the release-wt ref and that branch is the
//     one checked out here.
//
// It's false for a genuine other worktree's track branch or a release-wt
// branch that differs from repoRoot's — cases where the committed ref is
// the only view LoadBoard has, exactly what sworn#81's fix intended.
func (s *sliceOracleContext) resolvedRefIsLiveCheckout(resolvedFrom board.ResolvedFrom, ownerTrack string, trackMap map[string]board.TrackInfo) bool {
	switch resolvedFrom {
	case board.ResolvedByWorkingTree:
		return true
	case board.ResolvedByTrack:
		if s.currentBranch == "" {
			return false
		}
		ti, ok := trackMap[ownerTrack]
		return ok && ti.WorktreeBranch != "" && ti.WorktreeBranch == s.currentBranch
	case board.ResolvedByReleaseWT:
		if s.currentBranch == "" {
			return false
		}
		return s.releaseRef == "refs/heads/"+s.currentBranch
	default:
		return false
	}
}

// sliceBoardInfoFromStatus builds a SliceBoardInfo from a live working-tree
// state.Status read.
func sliceBoardInfoFromStatus(sliceID string, st *state.Status) SliceBoardInfo {
	lastUp := st.LastUpdatedAt
	if lastUp == "" {
		lastUp = "—"
	}
	return SliceBoardInfo{
		ID:                 sliceID,
		State:              string(st.State),
		LastUpdatedAt:      formatLastUpdated(lastUp),
		VerificationResult: st.Verification.Result,
	}
}

// sliceBoardInfoFromOracle builds a SliceBoardInfo from an oracle-resolved
// board.SliceState (a committed ref — track branch, release-wt, or HEAD).
func sliceBoardInfoFromOracle(sliceID string, ss board.SliceState) SliceBoardInfo {
	lastUp := ss.LastUpdated
	if lastUp == "" {
		lastUp = "—"
	}
	return SliceBoardInfo{
		ID:                 sliceID,
		State:              string(ss.State),
		LastUpdatedAt:      formatLastUpdated(lastUp),
		VerificationResult: ss.VerificationResult,
	}
}

// resolveOracleReleaseRef mirrors cmd/sworn/board.go's release-wt resolution:
// prefer the release-wt branch (docs or Fumadocs prefix), falling back to
// HEAD when no release-wt branch has been materialised for this release.
func resolveOracleReleaseRef(reader tuiOracleReader, releaseName string) string {
	releaseRef := "refs/heads/release-wt/" + releaseName
	for _, prefix := range []string{"docs/release", "apps/docs/content/docs/release"} {
		exists, err := reader.CatFileExists(releaseRef, prefix+"/"+releaseName+"/index.md")
		if err == nil && exists {
			return releaseRef
		}
	}
	return "HEAD"
}

// trackInfoMap converts board.json's on-disk track records into the
// board.TrackInfo map ReadSliceStatus needs to resolve slice ownership.
func trackInfoMap(tracks []board.BoardTrack) map[string]board.TrackInfo {
	trackMap := make(map[string]board.TrackInfo, len(tracks))
	for _, t := range tracks {
		trackMap[t.ID] = board.TrackInfo{
			ID:             t.ID,
			Slices:         t.Slices,
			DependsOn:      []string(t.DependsOn),
			WorktreePath:   t.WorktreePath,
			WorktreeBranch: t.WorktreeBranch,
			State:          t.State,
		}
	}
	return trackMap
}

// formatLastUpdated reformats a timestamp for display.
func formatLastUpdated(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Try ISO 8601 without timezone
		t, err = time.Parse("2006-01-02T15:04:05", ts)
		if err != nil {
			return ts
		}
	}
	// Show relative time for recent, date for older.
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
		// sworn#82: rendered while loadBoardCmd is in flight, so the user
		// sees feedback instead of a frozen screen during the load.
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

	var sb strings.Builder
	title := "Board: " + b.ReleaseName
	if b.SortMode == trackSortDeps {
		title += "  (sorted: dependency order)"
	} else {
		title += "  (sorted: declaration order)"
	}
	sb.WriteString(BoardTitle.Render(title))
	sb.WriteString("\n\n")

	if len(b.Tracks) == 0 {
		sb.WriteString(EmptyMessage.Render("No tracks defined"))
		return sb.String()
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
		sb.WriteString(TrackHeader.Render(header))
		sb.WriteString("\n")
		for _, sliceID := range track.Slices {
			si, ok := b.Slices[sliceID]
			if !ok {
				si = SliceBoardInfo{ID: sliceID, State: "unknown", LastUpdatedAt: "—"}
			}
			sliceState := SliceStateColor(si.State, si.VerificationResult)
			gateLine := b.renderGateLine(si)
			line := fmt.Sprintf("  %s  %s  (%s)  %s", sliceID, sliceState, si.LastUpdatedAt, gateLine)
			if len(b.orderedSlices) > 0 && b.Cursor >= 0 && b.Cursor < len(b.orderedSlices) && b.orderedSlices[b.Cursor] == sliceID {
				sb.WriteString(BoardItemSelected.Render("▸" + line[1:]))
			} else {
				sb.WriteString(SliceItem.Render(line))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderGateLine renders the gate badge for one slice row (sworn#82): the
// computed GateResult once GatesLoaded, a "computing" placeholder while
// loadGatesCmd is in flight, or an "unloaded" hint (press 'g') otherwise.
// Gates are no longer computed as part of LoadBoard, so a freshly-loaded
// board always starts in the "unloaded" state here.
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
