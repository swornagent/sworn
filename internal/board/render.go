// Package board — render.go deterministically generates a release board's
// index.md from board.json plus each referenced slice's spec.json/status.json.
//
// index.md is a VIEW of the board record, not prose (ADR-0009), so it must be
// rendered, not hand-authored — a hand-edited view silently drifts from the
// record (the frontmatter-fusion false merge-ready class this slice exists to
// kill). The renderer decodes board.json via the canonical strict ReadBoard
// (object-only release, S05) — never a second tolerant decoder — and fails
// closed on any missing or invalid input rather than emit a misleading view.
//
// Pure stdlib — zero third-party dependencies.
package board

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/git"
)

// sliceRecord is the subset of a slice's spec.json + status.json the renderer
// needs. Assembled once per slice so the writers below are pure string builders.
type sliceRecord struct {
	ID          string
	Track       string
	Outcome     string   // spec.json user_outcome, collapsed to one line
	State       string   // status.json state
	Quadrant    string   // spec.json effort_complexity.quadrant
	Touchpoints []string // spec.json touchpoints
}

// Render reads docs/release/<release>/board.json (via the canonical strict
// reader) plus each referenced slice's spec.json and status.json, and returns
// the full index.md markdown as a string. It is PURE — it performs no writes,
// so the golden test drives it directly.
//
// Fail closed (AC-04): a missing, malformed, or structurally-invalid board.json,
// or a referenced slice missing its spec/status record, returns a descriptive
// error and no partial output. In particular a MISSING board.json is a hard
// error here — the renderer does NOT fall through to ReadBoard's lazy migration
// (which reconstructs board.json from index.md), because that would invert the
// data flow this renderer exists to establish (index.md derives FROM board.json,
// never the reverse).
func Render(projectRoot, release string) (string, error) {
	relDir := filepath.Join(projectRoot, "docs", "release", release)
	boardPath := filepath.Join(relDir, "board.json")

	// AC-04 missing-board guard: os.Stat before ReadBoard so an absent board.json
	// fails closed instead of triggering the lazy migration-from-index.md path.
	if _, err := os.Stat(boardPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("render %s: board.json not found at %s (fail closed — index.md is derived from board.json, never reconstructed from it)", release, boardPath)
		}
		return "", fmt.Errorf("render %s: stat board.json: %w", release, err)
	}

	// board.json is present — decode via the canonical strict reader. A malformed,
	// still-string, or structurally-invalid board fails closed through ReadBoard.
	rec, err := ReadBoard(projectRoot, release)
	if err != nil {
		return "", fmt.Errorf("render %s: read board.json: %w", release, err)
	}
	if len(rec.Tracks) == 0 {
		return "", fmt.Errorf("render %s: board.json has no tracks (fail closed — nothing to render)", release)
	}

	// Sorted-by-id track order is the single stable ordering every section shares
	// (tracks table rows, slice grouping, matrix columns, dependency graph).
	tracks := sortedTracks(rec.Tracks)

	// Assemble each slice's record (fails closed if a spec/status is missing).
	records := map[string]sliceRecord{}
	for _, t := range tracks {
		if t.ID == "" {
			return "", fmt.Errorf("render %s: a track entry is missing its id (fail closed)", release)
		}
		for _, sid := range t.Slices {
			sr, err := readSliceRecord(relDir, sid, t.ID)
			if err != nil {
				return "", fmt.Errorf("render %s: slice %s: %w", release, sid, err)
			}
			records[sid] = sr
		}
	}

	// board-v1 is a pure plan: a track's state is not persisted, so the tracks
	// table shows the state DERIVED from git refs (track-mode invariant 5 /
	// sworn#80). Best-effort: a track whose branch does not resolve derives to
	// "planned"; if git is unavailable the cell renders "-".
	trackStates := deriveTrackStatesForRender(projectRoot, release, tracks)

	var b strings.Builder
	writeFrontmatter(&b, release)
	writeTracksTable(&b, tracks, trackStates)
	writeSliceTable(&b, tracks, records)
	writeTouchpointMatrix(&b, tracks, records)
	writeDependencyGraph(&b, tracks)
	return b.String(), nil
}

// RenderToFile renders the release and, only on success, writes index.md. The
// build-then-write order guarantees a failed render never leaves a partial or
// empty index.md on disk (AC-04).
func RenderToFile(projectRoot, release string) error {
	out, err := Render(projectRoot, release)
	if err != nil {
		return err
	}
	indexPath := filepath.Join(projectRoot, "docs", "release", release, "index.md")
	if err := os.WriteFile(indexPath, []byte(out), 0644); err != nil {
		return fmt.Errorf("render %s: write index.md: %w", release, err)
	}
	return nil
}

// sortedTracks returns a copy of tracks ordered by id — a total, input-order-
// independent ordering, so the render is idempotent regardless of board.json's
// on-disk track order (AC-02).
func sortedTracks(tracks []BoardTrack) []BoardTrack {
	out := make([]BoardTrack, len(tracks))
	copy(out, tracks)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// readSliceRecord loads the spec.json + status.json fields for one slice.
func readSliceRecord(relDir, sliceID, trackID string) (sliceRecord, error) {
	specData, err := os.ReadFile(filepath.Join(relDir, sliceID, "spec.json"))
	if err != nil {
		return sliceRecord{}, fmt.Errorf("read spec.json: %w", err)
	}
	var spec struct {
		UserOutcome      string   `json:"user_outcome"`
		Touchpoints      []string `json:"touchpoints"`
		EffortComplexity struct {
			Quadrant string `json:"quadrant"`
		} `json:"effort_complexity"`
	}
	if err := json.Unmarshal(specData, &spec); err != nil {
		return sliceRecord{}, fmt.Errorf("parse spec.json: %w", err)
	}

	statusData, err := os.ReadFile(filepath.Join(relDir, sliceID, "status.json"))
	if err != nil {
		return sliceRecord{}, fmt.Errorf("read status.json: %w", err)
	}
	var status struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(statusData, &status); err != nil {
		return sliceRecord{}, fmt.Errorf("parse status.json: %w", err)
	}

	return sliceRecord{
		ID:          sliceID,
		Track:       trackID,
		Outcome:     oneLine(spec.UserOutcome),
		State:       status.State,
		Quadrant:    spec.EffortComplexity.Quadrant,
		Touchpoints: spec.Touchpoints,
	}, nil
}

// writeFrontmatter emits YAML frontmatter with SINGLE-QUOTED scalars (AC-03) so
// the output cannot reproduce the `state: merged---` fence-fusion failure class.
func writeFrontmatter(b *strings.Builder, release string) {
	fmt.Fprintf(b, "---\n")
	fmt.Fprintf(b, "title: 'Release board — %s'\n", yamlSingle(release))
	fmt.Fprintf(b, "description: 'Rendered view of board.json + slice records. Generated by `sworn render %s` — do not hand-edit; edit board.json and re-render.'\n", yamlSingle(release))
	fmt.Fprintf(b, "---\n\n")
	fmt.Fprintf(b, "# Release board: %s\n\n", release)
	fmt.Fprintf(b, "> Generated by `sworn render`. This file is a deterministic view of `board.json`\n")
	fmt.Fprintf(b, "> plus each slice's `spec.json`/`status.json` — never hand-authored.\n\n")
}

// writeTracksTable emits the tracks table (AC-01): id, ordered slices, depends_on,
// and the DERIVED state (sworn#80 — the board no longer persists track state).
func writeTracksTable(b *strings.Builder, tracks []BoardTrack, states map[string]string) {
	fmt.Fprintf(b, "## Tracks\n\n")
	fmt.Fprintf(b, "| Track | Slices | depends_on | State |\n")
	fmt.Fprintf(b, "|-------|--------|------------|-------|\n")
	for _, t := range tracks {
		fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n",
			t.ID, backtickJoin(t.Slices), dependsOn(t.DependsOn), cell(states[t.ID]))
	}
	fmt.Fprintf(b, "\n")
}

// deriveTrackStatesForRender computes each track's state from git refs
// (track-mode invariant 5) for the rendered tracks table. It is best-effort: a
// track whose branch cannot be resolved (absent, or git unavailable) is omitted
// from the map, so cell() renders "-".
func deriveTrackStatesForRender(projectRoot, release string, tracks []BoardTrack) map[string]string {
	states := make(map[string]string, len(tracks))
	repo := git.New(projectRoot)
	for _, t := range tracks {
		if st, err := DeriveTrackState(repo, release, t.ID); err == nil {
			states[t.ID] = st
		}
	}
	return states
}

// writeSliceTable emits the slice table (AC-01): id, track, one-line outcome
// (spec.user_outcome), state (status.state), effort_complexity quadrant.
func writeSliceTable(b *strings.Builder, tracks []BoardTrack, records map[string]sliceRecord) {
	fmt.Fprintf(b, "## Slices\n\n")
	fmt.Fprintf(b, "| Slice | Track | Outcome | State | E×C |\n")
	fmt.Fprintf(b, "|-------|-------|---------|-------|-----|\n")
	for _, t := range tracks {
		for _, sid := range t.Slices {
			r := records[sid]
			fmt.Fprintf(b, "| `%s` | `%s` | %s | %s | %s |\n",
				sid, t.ID, cell(r.Outcome), cell(r.State), cell(r.Quadrant))
		}
	}
	fmt.Fprintf(b, "\n")
}

// writeTouchpointMatrix emits the file × track matrix (AC-01). Rows are the
// union of every slice's touchpoints, sorted by (owning-track-id, file-path);
// a ✓ marks each track that owns the file. When the plan is touchpoint-disjoint
// (AC-05) every row carries exactly one ✓.
func writeTouchpointMatrix(b *strings.Builder, tracks []BoardTrack, records map[string]sliceRecord) {
	// file -> set of owning track ids.
	owners := map[string]map[string]bool{}
	for _, t := range tracks {
		for _, sid := range t.Slices {
			for _, f := range records[sid].Touchpoints {
				if owners[f] == nil {
					owners[f] = map[string]bool{}
				}
				owners[f][t.ID] = true
			}
		}
	}

	files := make([]string, 0, len(owners))
	for f := range owners {
		files = append(files, f)
	}
	// Stable order: by first (min) owning track id, then by file path.
	sort.Slice(files, func(i, j int) bool {
		oi, oj := minKey(owners[files[i]]), minKey(owners[files[j]])
		if oi != oj {
			return oi < oj
		}
		return files[i] < files[j]
	})

	fmt.Fprintf(b, "## Touchpoint matrix\n\n")
	fmt.Fprintf(b, "Every planned-write file × track. A file marked under two tracks is a\n")
	fmt.Fprintf(b, "collision — disjointness is what licenses tracks to run independently.\n\n")

	// Header row: File + one column per track (sorted-id order).
	b.WriteString("| File |")
	for _, t := range tracks {
		fmt.Fprintf(b, " %s |", t.ID)
	}
	b.WriteString("\n|------|")
	for range tracks {
		b.WriteString("----|")
	}
	b.WriteString("\n")

	for _, f := range files {
		fmt.Fprintf(b, "| `%s` |", f)
		for _, t := range tracks {
			if owners[f][t.ID] {
				b.WriteString(" ✓ |")
			} else {
				b.WriteString("  |")
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "\n")
}

// writeDependencyGraph emits a fenced adjacency listing derived from each
// track's depends_on (AC-01). Deterministic: tracks in sorted-id order, deps
// sorted within each line.
func writeDependencyGraph(b *strings.Builder, tracks []BoardTrack) {
	fmt.Fprintf(b, "## Dependency graph\n\n")
	b.WriteString("```\n")
	for _, t := range tracks {
		if len(t.DependsOn) == 0 {
			fmt.Fprintf(b, "%s  (root)\n", t.ID)
			continue
		}
		deps := append([]string(nil), t.DependsOn...)
		sort.Strings(deps)
		fmt.Fprintf(b, "%s  ← depends on: %s\n", t.ID, strings.Join(deps, ", "))
	}
	b.WriteString("```\n")
}

// --- small deterministic helpers ---

// oneLine collapses all runs of whitespace (including newlines) to single
// spaces and trims, so a multi-line user_outcome renders as one table cell.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// cell escapes a value for a Markdown table cell: pipes would break the column
// structure, and an empty value renders as an em-dash for readability.
func cell(s string) string {
	s = strings.ReplaceAll(oneLine(s), "|", "\\|")
	if s == "" {
		return "—"
	}
	return s
}

// yamlSingle escapes a value for a single-quoted YAML scalar (a literal single
// quote is doubled). Release names never contain one, but the escaping keeps
// the emitter correct by construction.
func yamlSingle(s string) string { return strings.ReplaceAll(s, "'", "''") }

// backtickJoin renders a slice-id list as comma-separated backticked ids, or an
// em-dash when empty.
func backtickJoin(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = "`" + it + "`"
	}
	return strings.Join(parts, ", ")
}

// dependsOn renders a track's depends_on cell (sorted, backticked) or an em-dash.
func dependsOn(deps StringList) string {
	if len(deps) == 0 {
		return "—"
	}
	sorted := append([]string(nil), deps...)
	sort.Strings(sorted)
	return backtickJoin(sorted)
}

// minKey returns the lexicographically smallest key of a set (used to give each
// touchpoint file a stable primary sort key = its first owning track).
func minKey(set map[string]bool) string {
	first := ""
	for k := range set {
		if first == "" || k < first {
			first = k
		}
	}
	return first
}
