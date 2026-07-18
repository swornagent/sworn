package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
)

// ReleaseInfo holds metadata about one release for the list view.
type ReleaseInfo struct {
	ID                     string // release ID, e.g. "2026-06-19-safe-parallelism"
	Name                   string
	TrackCount             int
	SourceRef              string
	HasUncommittedEvidence bool
	Catalog                *board.CatalogRecord
	SliceStates            map[string]int // state -> count, for aggregation
}

// ReleasesList is a Bubble Tea component embedded in the root model.
// It holds all discovered releases and a cursor for navigation.
type ReleasesList struct {
	Releases []ReleaseInfo
	Cursor   int

	// Width is the pane box width (from Model.View via paneWidths). When > 0
	// each release label is ellipsis-truncated to fit the pane on one line
	// (Coach pin 1 — legible names at 80 cols instead of wrapping). When 0
	// (no tea.WindowSizeMsg received yet) labels are rendered untruncated,
	// preserving pre-S03 behaviour.
	Width int
}

// ErrNoReleases indicates no releases were found.
var ErrNoReleases = fmt.Errorf("no releases found under docs/release/")

// LoadReleases discovers releases from shared catalog records via
// board.DiscoverCatalog.
func (r *ReleasesList) LoadReleases(repoRoot string) error {
	catalog, err := board.DiscoverCatalog(git.New(repoRoot))
	if err != nil {
		return err
	}
	releases, err := releaseInfosFromCatalog(catalog)

	r.Releases = releases
	if r.Cursor >= len(r.Releases) {
		r.Cursor = len(r.Releases) - 1
	}
	if r.Cursor < 0 {
		r.Cursor = 0
	}
	return err
}

// releaseInfosFromCatalog converts one complete catalog snapshot into the
// releases-list value installed by the root model. It performs no discovery or
// status reads, so callers can use the returned value as one immutable refresh
// transaction. An empty successful catalog returns the established
// ErrNoReleases presentation while still producing an empty replacement value.
func releaseInfosFromCatalog(catalog []board.CatalogRecord) ([]ReleaseInfo, error) {
	releases := make([]ReleaseInfo, 0, len(catalog))
	for i := range catalog {
		releases = append(releases, releaseInfoFromCatalog(catalog[i]))
	}

	sort.Slice(releases, func(i, j int) bool {
		return releases[i].ID < releases[j].ID
	})
	if len(releases) == 0 {
		return releases, ErrNoReleases
	}
	return releases, nil
}

func releaseInfoFromCatalog(rec board.CatalogRecord) ReleaseInfo {
	info := ReleaseInfo{
		ID:          rec.Release,
		Name:        rec.Release,
		SourceRef:   rec.SourceRef,
		TrackCount:  0,
		SliceStates: map[string]int{},
		Catalog:     &rec,
	}
	if rec.Board == nil {
		return info
	}
	info.TrackCount = len(rec.Board.Tracks)
	for _, t := range rec.Board.Tracks {
		for _, ss := range t.Slices {
			info.SliceStates[string(ss.State)]++
			if ss.StateDurability == "uncommitted" {
				info.HasUncommittedEvidence = true
			}
		}
	}
	return info
}

// AggregatedState returns the dominant state across all slices.
func (r ReleaseInfo) AggregatedState() string {
	// Priority order for display: blocked > failed > in_progress > design_review > verified > planned
	priority := []string{"blocked", "failed_verification", "in_progress", "design_review", "verified", "planned"}
	for _, s := range priority {
		if count, ok := r.SliceStates[s]; ok && count > 0 {
			return s
		}
	}
	return "planned"
}

// View renders the releases list pane.
func (r *ReleasesList) View() string {
	if len(r.Releases) == 0 {
		return ReleaseListTitle.Render("Releases") + "\n" +
			EmptyMessage.Render("No releases found")
	}

	var b strings.Builder
	b.WriteString(ReleaseListTitle.Render("Releases"))
	b.WriteString("\n")

	for i, rel := range r.Releases {
		stateStr := rel.AggregatedState()
		label := fmt.Sprintf("%s  %s (%d tracks, %s)",
			rel.ID,
			Divider,
			rel.TrackCount,
			stateStr,
		)
		if rel.HasUncommittedEvidence && i == r.Cursor {
			label += " [uncommitted]"
		}
		// Coach pin 1: when the pane width is known, truncate the label with
		// an ellipsis so a long release name stays on a single line rather
		// than wrapping illegibly.
		if budget := r.Width - 6; r.Width > 0 && budget >= 1 {
			label = ansi.Truncate(label, budget, "…")
		}
		if i == r.Cursor {
			b.WriteString(ReleaseItemSelected.Render("▸ " + label))
		} else {
			b.WriteString(ReleaseItem.Render("  " + label))
		}
		b.WriteString("\n")
	}
	return b.String()
}
