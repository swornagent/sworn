package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/state"
)

// TrackInfo holds data about one track, as displayed by the board pane.
type TrackInfo struct {
	ID      string
	Slices  []string
	Depends string
	State   string
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
	Cursor        int                   // index of the selected slice in orderedSlices
	orderedSlices []string              // slice IDs in display order
	MergeActive   map[string]bool       // track IDs with an active merge in flight
	GateResults   map[string]GateResult // per-slice gate check results (S72)
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
			ID:      t.ID,
			Slices:  t.Slices,
			Depends: strings.Join(t.DependsOn, ", "),
			State:   t.State,
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

		if sliceOracle != nil {
			ss, _, errO := sliceOracle.oracle.ReadSliceStatus(ctx, sliceOracle.reader, "", sliceOracle.releaseRef, releaseName, sliceID, trackMap)
			if errO == nil {
				lastUp := ss.LastUpdated
				if lastUp == "" {
					lastUp = "—"
				}
				b.Slices[sliceID] = SliceBoardInfo{
					ID:                 sliceID,
					State:              string(ss.State),
					LastUpdatedAt:      formatLastUpdated(lastUp),
					VerificationResult: ss.VerificationResult,
				}
				continue
			}
		}

		// Fallback: working-tree filesystem read.
		statusPath := filepath.Join(releaseDir, sliceID, "status.json")
		st, errR := state.Read(statusPath)
		if errR != nil {
			b.Slices[sliceID] = SliceBoardInfo{
				ID:    sliceID,
				State: "unknown",
			}
			continue
		}
		lastUp := st.LastUpdatedAt
		if lastUp == "" {
			lastUp = "—"
		}
		b.Slices[sliceID] = SliceBoardInfo{
			ID:                 sliceID,
			State:              string(st.State),
			LastUpdatedAt:      formatLastUpdated(lastUp),
			VerificationResult: st.Verification.Result,
		}
	}

	// Populate orderedSlices in display order.
	b.orderedSlices = nil
	for _, track := range b.Tracks {
		for _, sliceID := range track.Slices {
			b.orderedSlices = append(b.orderedSlices, sliceID)
		}
	}
	if b.Cursor >= len(b.orderedSlices) {
		b.Cursor = len(b.orderedSlices) - 1
	}
	if b.Cursor < 0 {
		b.Cursor = 0
	}

	b.Loaded = true

	// Load gate results for display (S72).
	b.GateResults = LoadGateResults(repoRoot, releaseName)
	for sid, gr := range b.GateResults {
		si := b.Slices[sid]
		si.Gate = gr
		b.Slices[sid] = si
	}

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
	return &sliceOracleContext{
		oracle:     board.NewGitOracle(repo),
		reader:     reader,
		releaseRef: resolveOracleReleaseRef(reader, releaseName),
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
	if !b.Loaded {
		return BoardTitle.Render("Board") + "\n" +
			EmptyMessage.Render("Select a release from the left pane")
	}

	var sb strings.Builder
	sb.WriteString(BoardTitle.Render("Board: " + b.ReleaseName))
	sb.WriteString("\n\n")

	if len(b.Tracks) == 0 {
		sb.WriteString(EmptyMessage.Render("No tracks defined"))
		return sb.String()
	}

	for _, track := range b.Tracks {
		stateCol := StateColor(track.State)
		header := fmt.Sprintf("▸ %s  [%s]", track.ID, stateCol)
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
			gateLine := si.Gate.RenderInline()
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
