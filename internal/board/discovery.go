package board

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/state"
)

// CatalogRecord is one selected topology and its centrally elected live state.
type CatalogRecord struct {
	Release   string      `json:"release"`
	SourceRef string      `json:"sourceRef"`
	Board     *BoardState `json:"-"`
}

type topologyCandidate struct{ ref, path string }

// DiscoverCatalog discovers all release plans on locally available branch tips.
func DiscoverCatalog(repo *gitpkg.Repo) ([]CatalogRecord, error) {
	refs, err := repo.ListRefs()
	if err != nil {
		return nil, fmt.Errorf("list refs: %w", err)
	}
	byRelease := map[string][]topologyCandidate{}
	for _, ref := range refs {
		for _, prefix := range docsPrefixes {
			paths, err := repo.ListTreePaths(ref, prefix)
			if err != nil {
				return nil, fmt.Errorf("scan %s: %w", ref, err)
			}
			for _, p := range paths {
				rel := strings.TrimPrefix(p, prefix+"/")
				parts := strings.Split(rel, "/")
				if len(parts) == 2 && (parts[1] == "board.json" || parts[1] == "index.md") {
					byRelease[parts[0]] = append(byRelease[parts[0]], topologyCandidate{ref, p})
				}
			}
		}
		if release, ok := canonicalRelease(ref); ok {
			if _, exists := byRelease[release]; !exists {
				byRelease[release] = nil
			}
		}
	}
	names := make([]string, 0, len(byRelease))
	for name := range byRelease {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]CatalogRecord, 0, len(names))
	for _, release := range names {
		selected, err := selectTopology(release, refs, byRelease[release])
		if err != nil {
			return nil, err
		}
		tracks, err := parseTopology(repo, selected, release)
		if err != nil {
			return nil, fmt.Errorf("release %q ref %s: %w", release, selected.ref, err)
		}
		bs, err := electBoard(repo, refs, release, tracks)
		if err != nil {
			return nil, err
		}
		out = append(out, CatalogRecord{Release: release, SourceRef: selected.ref, Board: bs})
	}
	return out, nil
}

func canonicalRelease(ref string) (string, bool) {
	for _, marker := range []string{"refs/heads/release-wt/", "/release-wt/"} {
		if i := strings.Index(ref, marker); i >= 0 {
			return ref[i+len(marker):], true
		}
	}
	return "", false
}

func refClass(ref, release string) int {
	if ref == "refs/heads/release-wt/"+release {
		return 0
	}
	if strings.HasPrefix(ref, "refs/remotes/") && strings.HasSuffix(ref, "/release-wt/"+release) {
		return 1
	}
	if strings.HasPrefix(ref, "refs/heads/") {
		return 2
	}
	return 3
}

func selectTopology(release string, refs []string, candidates []topologyCandidate) (topologyCandidate, error) {
	sort.Slice(candidates, func(i, j int) bool {
		ci, cj := refClass(candidates[i].ref, release), refClass(candidates[j].ref, release)
		if ci != cj {
			return ci < cj
		}
		if candidates[i].ref != candidates[j].ref {
			return candidates[i].ref < candidates[j].ref
		}
		return candidates[i].path < candidates[j].path
	})
	for _, ref := range refs {
		if r, ok := canonicalRelease(ref); ok && r == release {
			for _, c := range candidates {
				if c.ref == ref {
					return c, nil
				}
			}
			return topologyCandidate{}, fmt.Errorf("release %q ref %s: canonical ref carries no board record", release, ref)
		}
	}
	if len(candidates) == 0 {
		return topologyCandidate{}, fmt.Errorf("release %q: no board record", release)
	}
	return candidates[0], nil
}

func parseTopology(repo *gitpkg.Repo, c topologyCandidate, release string) ([]TrackInfo, error) {
	raw, err := repo.Show(c.ref, c.path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(c.path, "board.json") {
		var br BoardRecord
		if err := json.Unmarshal([]byte(raw), &br); err != nil {
			return nil, fmt.Errorf("parse board.json: %w", err)
		}
		if br.Release.Name != release {
			return nil, fmt.Errorf("board release identity %q does not match directory %q", br.Release.Name, release)
		}
		return boardTracksToTrackInfos(br.Tracks, release), nil
	}
	return ParseTracks(extractFrontmatterBody(raw)), nil
}

func electBoard(repo *gitpkg.Repo, refs []string, release string, tracks []TrackInfo) (*BoardState, error) {
	trackMap := make(map[string]TrackInfo, len(tracks))
	for _, t := range tracks {
		trackMap[t.ID] = t
	}
	bs := &BoardState{Release: release}
	for _, t := range tracks {
		ts := TrackState{ID: t.ID, WorktreeBranch: t.WorktreeBranch}
		for _, sid := range t.Slices {
			winner, err := electSlice(repo, refs, release, sid, t.ID, trackMap)
			if err != nil {
				return nil, err
			}
			ts.Slices = append(ts.Slices, winner)
		}
		ts.State = aggregateState(ts.Slices)
		bs.Tracks = append(bs.Tracks, ts)
	}
	return bs, nil
}

type evidence struct {
	raw, source, durability string
	status                  state.Status
	attention               bool
	rank                    int
	timestamp               int64
}

func electSlice(repo *gitpkg.Repo, refs []string, release, sid, track string, trackMap map[string]TrackInfo) (SliceState, error) {
	path := filepath.ToSlash(filepath.Join("docs", "release", release, sid, "status.json"))
	var candidates []evidence
	for _, ref := range refs {
		raw, err := repo.Show(ref, path)
		if err != nil {
			continue
		}
		if e, ok := validEvidence(raw, ref, "committed", release, sid, track); ok {
			candidates = append(candidates, e)
		}
	}
	if raw, err := os.ReadFile(filepath.Join(repo.Dir, filepath.FromSlash(path))); err == nil {
		head, headErr := repo.Show("HEAD", path)
		if headErr != nil || string(raw) != head {
			if e, ok := validEvidence(string(raw), "working-tree", "uncommitted", release, sid, track); ok {
				candidates = append(candidates, e)
			}
		}
	}
	if len(candidates) == 0 {
		return SliceState{ID: sid, Track: track, State: "unknown"}, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool { return better(candidates[i], candidates[j]) })
	w := candidates[0]
	ss, err := parseStatusJSON(w.raw, sid, track, trackMap)
	if err != nil {
		return SliceState{}, err
	}
	ss.StateSource, ss.StateDurability = w.source, w.durability
	return ss, nil
}

func validEvidence(raw, source, durability, release, sid, track string) (evidence, bool) {
	var s state.Status
	if json.Unmarshal([]byte(raw), &s) != nil || s.Release != release || s.SliceID != sid || s.Track != track {
		return evidence{}, false
	}
	ranks := map[state.State]int{"planned": 0, "design_review": 1, "in_progress": 2, "implemented": 3, "verified": 5, "shipped": 6}
	rank, normal := ranks[s.State]
	attention := s.State == "blocked" || s.State == "failed_verification" || s.State == "deferred" || s.Verification.Result == "blocked"
	if !normal && !attention {
		return evidence{}, false
	}
	if attention {
		rank = 4
	}
	var stamp int64
	if tm, err := timeParse(s.LastUpdatedAt); err == nil {
		stamp = tm
	}
	return evidence{raw: raw, source: source, durability: durability, status: s, attention: attention, rank: rank, timestamp: stamp}, true
}

func timeParse(v string) (int64, error) {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return 0, err
	}
	return t.UnixNano(), nil
}

func better(a, b evidence) bool {
	if a.attention != b.attention {
		if a.attention && (a.timestamp == 0 || b.timestamp == 0 || a.timestamp >= b.timestamp) {
			return true
		}
		if b.attention && (a.timestamp == 0 || b.timestamp == 0 || b.timestamp >= a.timestamp) {
			return false
		}
	}
	if a.rank != b.rank {
		return a.rank > b.rank
	}
	if a.timestamp != b.timestamp {
		return a.timestamp > b.timestamp
	}
	if a.durability != b.durability {
		return a.durability == "committed"
	}
	return a.source < b.source
}

func aggregateState(slices []SliceState) string {
	best := "planned"
	rank := -1
	for _, s := range slices {
		if e, ok := validEvidence(fmt.Sprintf(`{"slice_id":"x","release":"x","track":"x","state":%q,"verification":{"result":%q}}`, s.State, s.VerificationResult), "", "", "x", "x", "x"); ok && e.rank > rank {
			best = string(s.State)
			rank = e.rank
		}
	}
	return best
}
