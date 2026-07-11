// Package board validates the structural integrity of release-board index.md
// files. This file (board.go) provides the board.json read/write layer —
// the oracle's source of truth for track metadata, replacing the YAML
// frontmatter extraction from index.md (ADR-0009).
//
// Pure stdlib — zero third-party dependencies.
package board

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/baton"
)

// BoardRecord is the on-disk representation of a board.json file — a PURE PLAN
// (baton board-v1 at v0.9.0/v0.10.0). It carries $schema, release, and tracks
// ONLY. The retired schema_version and the release-level worktree fields
// (release_worktree_path/release_worktree_branch) are NO LONGER persisted:
// $schema carries the version, and the release worktree branch/path are DERIVED
// from (release) + repo location (sworn#80, Pin 1). There is no read-path
// normalise shim any more (removed by S12-record-migration): the struct carries
// only the pure-plan fields, so json.Unmarshal drops the retired keys a legacy
// (un-migrated) board still carries — legacy boards parse by construction.
type BoardRecord struct {
	Schema  string       `json:"$schema,omitempty"`
	Release Release      `json:"release"`
	Tracks  []BoardTrack `json:"tracks"`
}

// Release identifies a release on the board. Canonical baton board-v1 emits
// `release` as an object {name, vertical_trace, target_version, ...}. This type
// reads ONLY that canonical object form (strict — S05) and preserves the full
// object verbatim so a write-back never drops a field (the same
// round-trip-fidelity rule as the D6 deferral migration). A legacy bare-string
// release fails closed on read: there is no wild data (every string board is
// operator-owned), so a stray string board is a non-migrated artefact that
// should fail loud and get migrated (AC-06 cutover), not be silently tolerated.
type Release struct {
	Name string
	// raw holds the canonical object form verbatim (nil for the string form),
	// so MarshalJSON can re-emit every field unchanged.
	raw json.RawMessage
}

// StringRelease constructs a Release from a bare name (string form). Used by
// the index.md migration path, which only knows the release name.
func StringRelease(name string) Release { return Release{Name: name} }

// UnmarshalJSON accepts ONLY the canonical baton object form with a required,
// non-empty `name` (S05 strict reader). A bare JSON string (the legacy form)
// fails closed — operator string boards are migrated at cutover (AC-06), never
// read-tolerated, so a string release surfaces as a load error rather than
// lurking unmigrated.
func (r *Release) UnmarshalJSON(b []byte) error {
	var o struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return fmt.Errorf("board release: not a canonical {name} object (a bare string release is no longer read — migrate it to {\"name\":...}): %w", err)
	}
	if o.Name == "" {
		return fmt.Errorf("board release object missing required \"name\"")
	}
	r.Name = o.Name
	r.raw = append(json.RawMessage(nil), b...)
	return nil
}

// MarshalJSON re-emits the canonical object verbatim when present (preserving
// vertical_trace etc.). For a name-only release (constructed in-process via
// StringRelease — the index.md migration path) it emits the canonical object
// form {"name": ...} — sworn never writes the legacy bare-string form (S05:
// strict emit, strict read). Both producer and reader are canonical object-only.
func (r Release) MarshalJSON() ([]byte, error) {
	if r.raw != nil {
		return r.raw, nil
	}
	return json.Marshal(struct {
		Name string `json:"name"`
	}{Name: r.Name})
}

// BoardTrack is one track entry in a BoardRecord — a pure plan: id, ordered
// slices, and dependency edges ONLY. worktree_path/worktree_branch/state are NO
// LONGER persisted (sworn#80): the branch is derived as track/<release>/<track-id>,
// the path as a sibling of the release worktree, and the state from git ref
// ancestry (track-mode invariant 5). See derive.go.
type BoardTrack struct {
	ID        string     `json:"id"`
	Slices    []string   `json:"slices"`
	DependsOn StringList `json:"depends_on,omitempty"`
}

// StringList is a []string that can unmarshal from a JSON string, array, or null.
// board.json records depends_on as a plain string (e.g. "T2-model-layer") or null,
// but the Go type system expects []string. This adapter normalises both.
type StringList []string

// UnmarshalJSON implements json.Unmarshaler. Accepts:
//   - null → empty slice
//   - "string" → single-element slice
//   - ["a","b"] → normal slice
func (sl *StringList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*sl = nil
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*sl = StringList{s}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*sl = StringList(arr)
	return nil
}

// ReadBoard reads board.json from docs/release/<release>/board.json. If the
// file does not exist, it performs a lazy migration: reads the index.md
// frontmatter, builds a BoardRecord from it, and writes board.json so
// subsequent reads hit the JSON path.
func ReadBoard(repoRoot, release string) (*BoardRecord, error) {
	boardPath := filepath.Join(repoRoot, "docs", "release", release, "board.json")
	data, err := os.ReadFile(boardPath)
	if err == nil {
		// No normalise shim (removed by S12-record-migration once the on-disk
		// data was migrated): BoardRecord carries only the pure-plan fields, so
		// json.Unmarshal silently drops any retired schema_version / worktree /
		// state keys a legacy (un-migrated) on-disk board still carries — the
		// reader tolerates legacy boards by construction, not by a tolerance layer.
		var br BoardRecord
		if err := json.Unmarshal(data, &br); err != nil {
			return nil, fmt.Errorf("parse board.json: %w", err)
		}
		return &br, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read board.json: %w", err)
	}

	// Lazy migration: generate from index.md frontmatter.
	return migrateFromIndex(repoRoot, release)
}

// WriteBoard writes board.json to disk and validates it against the board-v1
// schema. Drift between the written board.json and the committed index.md is
// checked separately by `sworn doctor` (internal/board's render-and-diff
// guard is not run here — see cmd/sworn/doctor.go checkRenderDrift), which is
// fail-closed (ERROR + non-zero exit) rather than this function's former
// advisory-only, already-broken driftGuard.
func WriteBoard(repoRoot, release string, br *BoardRecord) error {
	// Set canonical schema metadata. board-v1 at v0.10.0 is a pure plan:
	// $schema carries the version (the retired schema_version integer is gone),
	// and no worktree/state fields are emitted (they are derived on read).
	br.Schema = baton.BoardSchemaURI
	if br.Tracks == nil {
		br.Tracks = []BoardTrack{}
	}

	data, err := json.MarshalIndent(br, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal board.json: %w", err)
	}

	boardPath := filepath.Join(repoRoot, "docs", "release", release, "board.json")
	if err := os.MkdirAll(filepath.Dir(boardPath), 0755); err != nil {
		return fmt.Errorf("mkdir for board.json: %w", err)
	}
	if err := os.WriteFile(boardPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write board.json: %w", err)
	}

	// Validate the written board.json.
	if err := baton.Validate("board-v1", data); err != nil {
		return fmt.Errorf("validate board.json: %w", err)
	}

	return nil
}

// migrateFromIndex reads the index.md frontmatter from the filesystem, parses
// it into a BoardRecord, and writes board.json. Called by ReadBoard when
// board.json does not exist.
func migrateFromIndex(repoRoot, release string) (*BoardRecord, error) {
	indexPath := filepath.Join(repoRoot, "docs", "release", release, "index.md")
	rawIndex, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("lazy migration: read index.md: %w", err)
	}

	fmBody := extractFrontmatterBody(string(rawIndex))
	trackInfos := ParseTracks(fmBody)

	// board-v1 is a pure plan (sworn#80, Pin 1): the release worktree path and
	// branch are DERIVED (release-wt/<release> + sibling-of-repo), never persisted,
	// so the migration no longer scrapes them from the index.md frontmatter.
	br := &BoardRecord{
		Schema:  baton.BoardSchemaURI,
		Release: StringRelease(release),
		Tracks:  trackInfosToBoardTracks(trackInfos),
	}

	// Write the migrated board.json.
	if err := WriteBoard(repoRoot, release, br); err != nil {
		return nil, fmt.Errorf("lazy migration: write board.json: %w", err)
	}

	return br, nil
}

// trackInfosToBoardTracks converts internal TrackInfo structs to BoardTrack.
// It persists ONLY the pure-plan fields (id, slices, depends_on) — the derived
// worktree/state fields on TrackInfo are deliberately dropped so a written
// board.json stays canonical board-v1 (sworn#80).
func trackInfosToBoardTracks(tis []TrackInfo) []BoardTrack {
	tracks := make([]BoardTrack, len(tis))
	for i, ti := range tis {
		slices := ti.Slices
		if slices == nil {
			// board-v1 requires slices to be present as an array; a track with no
			// slices serialises as [] (valid), never null (invalid).
			slices = []string{}
		}
		tracks[i] = BoardTrack{
			ID:        ti.ID,
			Slices:    slices,
			DependsOn: ti.DependsOn,
		}
	}
	return tracks
}

// boardTracksToTrackInfos converts BoardTrack (pure plan) to internal TrackInfo
// structs, DERIVING the worktree branch (track/<release>/<track-id>) rather than
// reading it from persisted data. WorktreePath and State are left empty here;
// the Oracle fills them via deriveTrackWorktrees where the repo root and git
// ancestry are available (a fake content-only reader cannot resolve them).
func boardTracksToTrackInfos(tracks []BoardTrack, release string) []TrackInfo {
	tis := make([]TrackInfo, len(tracks))
	for i, bt := range tracks {
		tis[i] = TrackInfo{
			ID:             bt.ID,
			Slices:         bt.Slices,
			DependsOn:      bt.DependsOn,
			WorktreeBranch: TrackWorktreeBranch(release, bt.ID),
		}
	}
	return tis
}
