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
	Releases     []ReleaseInfo
	Cursor       int
	Height       int
	HasOlder     bool
	LoadingOlder bool

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

// LoadReleaseWindow primes the TUI from a bounded board-owned snapshot.
func (r *ReleasesList) LoadReleaseWindow(repoRoot string, limit int) error {
	window, err := board.DiscoverCatalogWindow(git.New(repoRoot), limit)
	if err != nil {
		return err
	}
	return r.InstallWindow(window)
}

// InstallWindow replaces the list from one immutable bounded snapshot.
func (r *ReleasesList) InstallWindow(window board.CatalogWindow) error {
	releases, err := releaseInfosFromCatalog(window.Records)
	r.Releases = releases
	r.HasOlder = window.HasOlder
	r.LoadingOlder = false
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

	rows := make([]string, 0, len(r.Releases))
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
			rows = append(rows, ReleaseItemSelected.Render("▸ "+label))
		} else {
			rows = append(rows, ReleaseItem.Render("  "+label))
		}
	}

	footer := "all releases loaded"
	if r.LoadingOlder {
		footer = "loading older"
	} else if r.HasOlder {
		footer = "o older"
	}
	if r.Height > 0 {
		rowBudget := max(0, r.Height-2) // title + footer
		start, end := cursorWindow(len(rows), r.Cursor, rowBudget)
		rows = rows[start:end]
	}

	var b strings.Builder
	b.WriteString(ReleaseListTitle.Render("Releases"))
	if r.Height == 1 {
		return b.String()
	}
	if len(rows) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(rows, "\n"))
		if r.Height == 0 {
			b.WriteString("\n")
		}
	}
	if r.Height >= 2 {
		b.WriteString("\n")
		b.WriteString(EmptyMessage.Render(footer))
	}
	return b.String()
}

func cursorWindow(length, cursor, budget int) (int, int) {
	if budget <= 0 || length == 0 {
		return 0, 0
	}
	if budget >= length {
		return 0, length
	}
	cursor = max(0, min(cursor, length-1))
	start := cursor - budget/2
	if start < 0 {
		start = 0
	}
	if start+budget > length {
		start = length - budget
	}
	return start, start + budget
}
