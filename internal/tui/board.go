package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/state"
	"gopkg.in/yaml.v3"
)

// TrackInfo holds data about one track from the index.md frontmatter.
type TrackInfo struct {
	ID      string   `yaml:"id"`
	Slices  []string `yaml:"slices"`
	Depends string   `yaml:"depends_on"`
	State   string   `yaml:"state"`
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
	Cursor        int                       // index of the selected slice in orderedSlices
	orderedSlices []string                  // slice IDs in display order
	MergeActive   map[string]bool           // track IDs with an active merge in flight
	GateResults   map[string]GateResult     // per-slice gate check results (S72)
}
// LoadBoard reads the selected release's index.md and all slice status.json files.
// repoRoot is the repo root path.
func (b *BoardView) LoadBoard(repoRoot, releaseName string) error {
	b.ReleaseName = releaseName
	b.Tracks = nil
	b.Slices = nil
	b.Loaded = false

	indexPath := filepath.Join(repoRoot, "docs", "release", releaseName, "index.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", indexPath, err)
	}

	// Parse frontmatter for tracks.
	type indexFM struct {
		Tracks []TrackInfo `yaml:"tracks"`
	}
	var fm indexFM

	// Extract YAML frontmatter.
	rest := strings.TrimPrefix(string(data), "---")
	parts := strings.SplitN(rest, "---", 2)
	if len(parts) >= 1 {
		if err := yaml.Unmarshal([]byte(parts[0]), &fm); err != nil {
			// Try with just the raw bytes (frontmatter may be short).
		}
	}
	b.Tracks = fm.Tracks

	// Load live state from each slice's status.json.
	b.Slices = map[string]SliceBoardInfo{}
	releaseDir := filepath.Join(repoRoot, "docs", "release", releaseName)
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return fmt.Errorf("reading release dir %s: %w", releaseDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "S") {
			continue
		}
		statusPath := filepath.Join(releaseDir, entry.Name(), "status.json")
		st, errR := state.Read(statusPath)
		if errR != nil {
			b.Slices[entry.Name()] = SliceBoardInfo{
				ID:    entry.Name(),
				State: "unknown",
			}
			continue
		}
		lastUp := st.LastUpdatedAt
		if lastUp == "" {
			lastUp = "—"
		}
		b.Slices[entry.Name()] = SliceBoardInfo{
			ID:                 entry.Name(),
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
