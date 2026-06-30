// Package board validates the structural integrity of release-board index.md
// files. This file (oracle.go) provides the git-ref-based slice-state reader
// that the board command, the router (S58), the scheduler (S59), and the TUI
// all read through — the keystone of the orchestration core (T17).
//
// Pure stdlib — zero third-party dependencies.
package board

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/state"
)

// ResolvedFrom records which ref level resolved a slice's status.json.
type ResolvedFrom string

const (
	ResolvedByTrack       ResolvedFrom = "track-branch"
	ResolvedByReleaseWT   ResolvedFrom = "release-wt"
	ResolvedByWorkingTree ResolvedFrom = "working-tree"
)

// BlockedOwner is the routing owner for a BLOCKED verdict.
type BlockedOwner string

const (
	BlockedNeedsPlanner     BlockedOwner = "needs_planner"
	BlockedNeedsHuman       BlockedOwner = "needs_human"
	BlockedNeedsImplementer BlockedOwner = "needs_implementer"
)

// SliceState is the authoritative per-slice board entry — read from the
// slice's owning track branch (or release-wt / working-tree fallback).
type SliceState struct {
	ID              string      `json:"id"`
	State           state.State `json:"state"`
	Owner           string      `json:"owner,omitempty"`
	LastUpdated     string      `json:"lastUpdated,omitempty"`
	Track           string      `json:"track"`
	Actionable      bool        `json:"actionable"`
	DependsOnTracks []string    `json:"dependsOnTracks"`
	// Blocked visibility (S57 spec, ref 2026-06-23 replan):
	// A slice with verification.result == "blocked" MUST be visible
	// regardless of its underlying state.
	Blocked       bool         `json:"blocked"`
	BlockedReason string       `json:"blocked_reason,omitempty"`
	BlockedOwner  BlockedOwner `json:"blocked_owner,omitempty"`
	// VerificationResult is the raw verification.result field from status.json.
	// Exposed for the router (S58) to route on failed_verification and implemented
	// without re-reading status.json.
	VerificationResult string `json:"-"`
	// Violations is the verification.violations array from status.json.
	// Exposed for the router (S58) to classify violations by gate.
	Violations []string `json:"-"`
}

// TrackState is the board-level track entry.
type TrackState struct {
	ID             string       `json:"id"`
	State          string       `json:"state"`
	Slices         []SliceState `json:"slices"`
	WorktreeBranch string       `json:"-"`
}

// BoardState is the full release board.
type BoardState struct {
	Release string       `json:"release"`
	Tracks  []TrackState `json:"tracks"`
}

// gitContentReader abstracts a single git ref read + existence check for
// oracle operators. The production implementation uses git.Repo; tests
// supply a fake (map-based) reader for transient-retry and ref-priority
// tests (transient-retry and ref-priority tests).
type gitContentReader interface {
	// Show returns the content of <ref>:<path>.
	Show(ref, path string) (string, error)
	// CatFileExists returns true when <ref>:<path> exists in the git tree.
	CatFileExists(ref, path string) (bool, error)
}

// gitRepoReader adapts the concrete *git.Repo to gitContentReader.
// Defined here (not in internal/git) to keep the oracle testable
// without importing the git package into test fakes.
type gitRepoReader struct {
	show          func(ref, path string) (string, error)
	catFileExists func(ref, path string) (bool, error)
}

func (g *gitRepoReader) Show(ref, path string) (string, error) {
	return g.show(ref, path)
}

func (g *gitRepoReader) CatFileExists(ref, path string) (bool, error) {
	return g.catFileExists(ref, path)
}

// OracleReader is the consumer contract for the router (S58) and scheduler
// (S59). It hides git-ref resolution and track-map construction behind
// router-friendly signatures: the caller passes a release and slice ID, and
// gets back SliceState / BoardState with no knowledge of track branches,
// release-wt refs, or index.md parsing.
type OracleReader interface {
	ReadSliceStatus(ctx context.Context, release, sliceID string) (SliceState, error)
	ReadBoard(ctx context.Context, release string) (*BoardState, error)
}

// Oracle reads slice state from git refs with ownership resolution.
// All methods accept a gitContentReader; the production caller passes
// a gitRepoReader wrapping *git.Repo.
type Oracle struct {
	reader gitContentReader
}

// NewOracle returns an Oracle backed by the given gitContentReader.
func NewOracle(r gitContentReader) *Oracle {
	return &Oracle{reader: r}
}

// docsPrefixes is the ordered list of release-docs prefixes to probe.
// The first prefix that exists in the git tree wins (using git cat-file -e,
// git cat-file -e, not filesystem existence, to avoid the Fumadocs
// symlink trap).
var docsPrefixes = []string{
	"docs/release",
	"apps/docs/content/docs/release",
}

// resolvePrefix probes the two docs prefixes against <ref> for the given
// release, and returns the first that has a status.json for sliceID.// The result is cached per (release, ref) pair in the Oracle (caller
// typically re-uses one Oracle instance per ReadBoard call, so the cache
// lives for one request).
func (o *Oracle) resolvePrefix(reader gitContentReader, ref, release, sliceID string) (string, error) {
	for _, prefix := range docsPrefixes {
		path := fmt.Sprintf("%s/%s/%s/status.json", prefix, release, sliceID)
		exists, err := reader.CatFileExists(ref, path)
		if err != nil {
			return "", fmt.Errorf("resolve prefix: cat-file -e %s:%s: %w", ref, path, err)
		}
		if exists {
			return prefix, nil
		}
	}
	return docsPrefixes[0], nil // fallback
}

// resolvedStatus holds the raw status.json + where it came from.
type resolvedStatus struct {
	Raw          string
	ResolvedFrom ResolvedFrom
}

// readSliceStatusFromRef attempts to read a slice's status.json from a
// specific git ref. Returns the raw JSON and the resolved-from level.
// If the file doesn't exist at the ref, returns ("", "", nil).
func (o *Oracle) readSliceStatusFromRef(reader gitContentReader, ref, release, sliceID string) (string, ResolvedFrom, error) {
	prefix, err := o.resolvePrefix(reader, ref, release, sliceID)
	if err != nil {
		return "", "", err
	}
	path := fmt.Sprintf("%s/%s/%s/status.json", prefix, release, sliceID)

	exists, err := reader.CatFileExists(ref, path)
	if err != nil {
		return "", "", fmt.Errorf("cat-file -e %s:%s: %w", ref, path, err)
	}
	if !exists {
		return "", "", nil
	}

	raw, err := reader.Show(ref, path)
	if err != nil {
		return "", "", fmt.Errorf("show %s:%s: %w", ref, path, err)
	}

	// Transient-read retry: if the content is empty or state is "unknown",
	// retry once after a short sleep.
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || strings.Contains(raw, `"state":""`) {
		time.Sleep(50 * time.Millisecond)
		raw2, err := reader.Show(ref, path)
		if err != nil {
			return "", "", fmt.Errorf("show retry %s:%s: %w", ref, path, err)
		}
		raw2 = strings.TrimSpace(raw2)
		if raw2 == "" || raw2 == "{}" {
			return "", "", nil // still empty after retry — treat as missing
		}
		raw = raw2
	}

	return raw, ResolvedByTrack, nil
}

// inferBlockedOwner returns the default blocked owner based on verdict.
// When verification.routing is set, it takes precedence (see caller).
func inferBlockedOwner(verdict string) BlockedOwner {
	switch verdict {
	case "blocked":
		return BlockedNeedsPlanner
	case "failed_verification":
		return BlockedNeedsImplementer
	default:
		return ""
	}
}

// parseStatusJSON unmarshals raw status.json and returns a SliceState.
// The trackMap is used to fill DependsOnTracks and Actionable.
func parseStatusJSON(raw string, sliceID, trackID string, trackMap map[string]TrackInfo) (SliceState, error) {
	var s state.Status
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return SliceState{}, fmt.Errorf("parse status.json for %s: %w", sliceID, err)
	}

	// Derive depends_on from the track map.
	var deps []string
	actionable := false
	if ti, ok := trackMap[trackID]; ok {
		deps = ti.DependsOn
		// A slice is actionable when its track has no unmet dependencies
		// AND the slice itself is in a non-terminal, non-verified state.
		// The depends_on-satisfied check is owned by the scheduler (S59);
		// here we compute Actionable from slice state alone.		actionable = isActionable(s.State)
	}

	blocked := s.Verification.Result == "blocked"
	var blockedReason string
	if blocked && len(s.Verification.Violations) > 0 {
		blockedReason = s.Verification.ViolationStrings()[0]
	}

	// Blocked owner: routing field takes precedence, else infer.
	var blockedOwner BlockedOwner
	if s.Verification.Routing != "" {
		blockedOwner = BlockedOwner(s.Verification.Routing)
	} else {
		blockedOwner = inferBlockedOwner(s.Verification.Result)
	}

	return SliceState{
		ID:                 sliceID,
		State:              s.State,
		Owner:              s.Owner,
		LastUpdated:        s.LastUpdatedAt,
		Track:              trackID,
		Actionable:         actionable,
		DependsOnTracks:    deps,
		Blocked:            blocked,
		BlockedReason:      blockedReason,
		BlockedOwner:       blockedOwner,
		VerificationResult: s.Verification.Result,
		Violations:         s.Verification.ViolationStrings(),
	}, nil
}

// isActionable returns true when a slice is in a state where it can be
// picked up by a worker (the router/scheduler can act on it).
func isActionable(s state.State) bool {
	switch s {
	case state.Planned, state.DesignReview, state.InProgress, state.FailedVerification:
		return true
	default:
		return false
	}
}

// ReadSliceStatus reads the authoritative status.json for sliceID in release.
// It resolves via: owner track branch → release-wt → working-tree HEAD.
// Ownership is determined from the index.md frontmatter parsed via ParseTracks.
// The trackBranch and releaseWTRef are git refs (e.g. "refs/heads/track/...",
// "refs/heads/release-wt/...").
//
// The reader parameter is the gitContentReader; in production it wraps
// internal/git.Repo. Tests supply a fake.
func (o *Oracle) ReadSliceStatus(
	ctx context.Context,
	reader gitContentReader,
	trackBranch string,
	releaseWTRef string,
	release,
	sliceID string,
	trackMap map[string]TrackInfo,
) (SliceState, ResolvedFrom, error) {
	// Determine the owning track for this slice.
	ownerTrack := ""
	for _, ti := range trackMap {
		for _, sid := range ti.Slices {
			if sid == sliceID {
				ownerTrack = ti.ID
				break
			}
		}
		if ownerTrack != "" {
			break
		}
	}
	if ownerTrack == "" {
		return SliceState{}, "", fmt.Errorf("slice %s: no owning track found in index.md", sliceID)
	}

	// Priority 1: owner's track branch.
	// The track branch is per-track. We need the correct track branch for the
	// owner. Look it up from trackMap.
	ownerBranch := ""
	if ti, ok := trackMap[ownerTrack]; ok {
		ownerBranch = ti.WorktreeBranch
	}
	if ownerBranch == "" {
		ownerBranch = trackBranch // fallback to the passed-in branch
	}

	if ownerBranch != "" {
		ref := "refs/heads/" + ownerBranch
		raw, resolved, err := o.readSliceStatusFromRef(reader, ref, release, sliceID)
		if err != nil {
			return SliceState{}, "", err
		}
		if raw != "" {
			ss, err := parseStatusJSON(raw, sliceID, ownerTrack, trackMap)
			if err != nil {
				return SliceState{}, "", err
			}
			return ss, resolved, nil
		}
	}

	// Priority 2: release-wt.
	if releaseWTRef != "" {
		raw, _, err := o.readSliceStatusFromRef(reader, releaseWTRef, release, sliceID)
		if err != nil {
			return SliceState{}, "", err
		}
		if raw != "" {
			ss, err := parseStatusJSON(raw, sliceID, ownerTrack, trackMap)
			if err != nil {
				return SliceState{}, "", err
			}
			return ss, ResolvedByReleaseWT, nil
		}
	}

	// Priority 3: working-tree HEAD.
	raw, _, err := o.readSliceStatusFromRef(reader, "HEAD", release, sliceID)
	if err != nil {
		return SliceState{}, "", err
	}
	if raw != "" {
		ss, err := parseStatusJSON(raw, sliceID, ownerTrack, trackMap)
		if err != nil {
			return SliceState{}, "", err
		}
		return ss, ResolvedByWorkingTree, nil
	}

	return SliceState{}, "", fmt.Errorf("slice %s: status.json not found on any ref (track, release-wt, or working tree)", sliceID)
}

// readTrackInfos reads track metadata from board.json (preferred) or
// index.md frontmatter (legacy fallback) using git refs. It returns the
// parsed TrackInfo list used by ReadBoard and NewOracleReaderAdapter.
func (o *Oracle) readTrackInfos(reader gitContentReader, releaseRef, release string) ([]TrackInfo, error) {
	// Try board.json first (post-migration releases).
	boardPath := "docs/release/" + release + "/board.json"
	rawBoard, err := reader.Show(releaseRef, boardPath)
	if err == nil {
		var br BoardRecord
		if err := json.Unmarshal([]byte(rawBoard), &br); err != nil {
			return nil, fmt.Errorf("parse board.json from %s: %w", releaseRef, err)
		}
		return boardTracksToTrackInfos(br.Tracks), nil
	}

	// Try Fumadocs prefix for board.json.
	fumaBoardPath := "apps/docs/content/docs/release/" + release + "/board.json"
	rawBoard, err = reader.Show(releaseRef, fumaBoardPath)
	if err == nil {
		var br BoardRecord
		if err := json.Unmarshal([]byte(rawBoard), &br); err != nil {
			return nil, fmt.Errorf("parse board.json from %s: %w", releaseRef, err)
		}
		return boardTracksToTrackInfos(br.Tracks), nil
	}

	// Fallback: read index.md frontmatter (legacy — board.json not yet migrated).
	indexPath := "docs/release/" + release + "/index.md"
	rawIndex, err := reader.Show(releaseRef, indexPath)
	if err != nil {
		fumaPath := "apps/docs/content/docs/release/" + release + "/index.md"
		rawIndex2, err2 := reader.Show(releaseRef, fumaPath)
		if err2 != nil {
			return nil, fmt.Errorf("read board.json: %v; read index.md: %v (also tried %s: %v)",
				err, err, fumaPath, err2)
		}
		rawIndex = rawIndex2
	}

	fmBody := extractFrontmatterBody(rawIndex)
	return ParseTracks(fmBody), nil
}

// ReadBoard reads the full release board: every track and every slice's
// authoritative state. It first reads board.json from a git ref to build the
// track→slice map (falling back to index.md YAML frontmatter for legacy
// releases that have not yet migrated), then resolves each slice via
// ReadSliceStatus.
// The releaseRef is a git ref (e.g. "refs/heads/release-wt/<release>")
// where the authoritative board.json / index.md lives.
func (o *Oracle) ReadBoard(ctx context.Context,
	reader gitContentReader,
	releaseRef string,
	release string,
) (*BoardState, error) {
	// Step 1: try board.json first (post-migration releases).
	// Fall back to index.md YAML frontmatter for legacy releases.
	trackInfos, err := o.readTrackInfos(reader, releaseRef, release)
	if err != nil {
		return nil, err
	}

	trackMap := make(map[string]TrackInfo, len(trackInfos))
	for _, ti := range trackInfos {
		trackMap[ti.ID] = ti
	}

	// Step 2: for each track, build its SliceState list by resolving each	// slice through ReadSliceStatus using the track's own branch as the
	// primary ref and releaseRef as the release-wt fallback.
	board := &BoardState{
		Release: release,
		Tracks:  make([]TrackState, 0, len(trackInfos)),
	}

	for _, ti := range trackInfos {
		ts := TrackState{
			ID:             ti.ID,
			State:          ti.State,
			WorktreeBranch: ti.WorktreeBranch,
			Slices:         make([]SliceState, 0, len(ti.Slices)),
		}
		for _, sid := range ti.Slices {
			trackBranch := "refs/heads/" + ti.WorktreeBranch
			if ti.WorktreeBranch == "" {
				trackBranch = "" // track not materialised yet
			}
			ss, _, err := o.ReadSliceStatus(ctx, reader, trackBranch, releaseRef, release, sid, trackMap)
			if err != nil {
				// Ghost-slice filter: if the slice is NOT owned by this
				// track, skip it. The authoritative copy is on the owner's
				// branch.
				if !trackOwnsSlice(ti.ID, sid, trackMap) {
					continue // ghost — not this track's authoritative copy
				}
				// Owned but unreadable — include with error state.
				ss = SliceState{
					ID:    sid,
					State: "unknown",
					Track: ti.ID,
				}
			} else {
				// Ghost filter: if the slice resolved from this track's
				// branch but is owned by a different track, the result is
				// a stale ghost copy. Skip it — the real copy will appear
				// under its owner track.
				if ss.Track != ti.ID {
					continue
				}
			}
			ts.Slices = append(ts.Slices, ss)
		}
		board.Tracks = append(board.Tracks, ts)
	}

	return board, nil
}

// trackOwnsSlice returns true when trackID is the owning track for sliceID
// according to the trackMap (index.md frontmatter).
func trackOwnsSlice(trackID, sliceID string, trackMap map[string]TrackInfo) bool {
	ti, ok := trackMap[trackID]
	if !ok {
		return false
	}
	for _, sid := range ti.Slices {
		if sid == sliceID {
			return true
		}
	}
	return false
}

// extractFrontmatterBody returns the YAML frontmatter body (the content
// between the opening and closing --- delimiters) from an index.md text.
// Returns the full text if no frontmatter delimiters are found.
func extractFrontmatterBody(text string) string {
	if !strings.HasPrefix(text, "---") {
		return text
	}
	// Find the second --- (closing delimiter).
	rest := text[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return text
	}
	return rest[:idx]
}

// NewGitOracle returns an Oracle backed by a *git.Repo. This is the
// production constructor; tests use NewOracle with a fake reader.
func NewGitOracle(repo *git.Repo) *Oracle {
	return &Oracle{
		reader: &gitRepoReader{
			show:          repo.Show,
			catFileExists: repo.CatFileExists,
		},
	}
}

// OracleReaderAdapter wraps an *Oracle with resolved parameters so it
// satisfies the router.OracleReader interface (simple 2-param signatures).
// Construct via NewOracleReaderAdapter; the adapter caches the track map
// and release ref for repeated calls.
type OracleReaderAdapter struct {
	oracle     *Oracle
	reader     gitContentReader
	release    string
	releaseRef string
	trackMap   map[string]TrackInfo
}

// NewOracleReaderAdapter reads index.md from releaseRef to build the track map,
// then returns an adapter that satisfies router.OracleReader.
func NewOracleReaderAdapter(
	oracle *Oracle,
	reader gitContentReader,
	release, releaseRef string,
) (*OracleReaderAdapter, error) {
	// Read track metadata from board.json (preferred) or index.md (legacy).
	trackInfos, err := oracle.readTrackInfos(reader, releaseRef, release)
	if err != nil {
		return nil, err
	}
	trackMap := make(map[string]TrackInfo, len(trackInfos))
	for _, ti := range trackInfos {
		trackMap[ti.ID] = ti
	}

	return &OracleReaderAdapter{
		oracle:     oracle,
		reader:     reader,
		release:    release,
		releaseRef: releaseRef,
		trackMap:   trackMap,
	}, nil
}

// ReadSliceStatus reads a single slice's status, resolving via the owner track
// branch → release-wt → HEAD priority chain. Implements router.OracleReader.
func (a *OracleReaderAdapter) ReadSliceStatus(ctx context.Context, release, sliceID string) (SliceState, error) {
	if release != a.release {
		return SliceState{}, fmt.Errorf("adapter: release mismatch (got %q, configured for %q)", release, a.release)
	}
	// Use the first track's branch as default (most callers will pass the right one).
	trackBranch := ""
	for _, ti := range a.trackMap {
		if ti.WorktreeBranch != "" {
			trackBranch = ti.WorktreeBranch
			break
		}
	}
	ss, _, err := a.oracle.ReadSliceStatus(ctx, a.reader, trackBranch, a.releaseRef, release, sliceID, a.trackMap)
	return ss, err
}

// ReadBoard reads the full release board. Implements router.OracleReader.
func (a *OracleReaderAdapter) ReadBoard(ctx context.Context, release string) (*BoardState, error) {
	if release != a.release {
		return nil, fmt.Errorf("adapter: release mismatch (got %q, configured for %q)", release, a.release)
	}
	return a.oracle.ReadBoard(ctx, a.reader, a.releaseRef, release)
}

// NewOracleReaderAdapterFromRepo is the production convenience constructor.
// It wraps a *git.Repo as both the oracle backend and content reader, reads
// index.md from releaseRef to build the track map, and returns an adapter
// that satisfies the router.OracleReader interface.
func NewOracleReaderAdapterFromRepo(
	repo *git.Repo,
	release, releaseRef string,
) (*OracleReaderAdapter, error) {
	oracle := NewGitOracle(repo)
	reader := &gitRepoReader{
		show:          repo.Show,
		catFileExists: repo.CatFileExists,
	}
	return NewOracleReaderAdapter(oracle, reader, release, releaseRef)
}
