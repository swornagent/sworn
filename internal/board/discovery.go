package board

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	gitpkg "github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/state"
)

// CatalogRecord is one selected topology and its centrally elected live state.
type CatalogRecord struct {
	Release        string              `json:"release"`
	SourceRef      string              `json:"sourceRef"`
	Board          *BoardState         `json:"-"`
	TrackDependsOn map[string][]string `json:"-"`
}

type topologyCandidate struct{ ref, path string }

func (c topologyCandidate) objectSpec() string { return c.ref + ":" + c.path }

// DiscoverCatalog discovers all release plans on locally available branch tips.
func DiscoverCatalog(repo *gitpkg.Repo) ([]CatalogRecord, error) {
	if _, err := repo.RevParse("HEAD"); err != nil {
		if headIsUnavailable(err) {
			return discoverFilesystemCatalog(repo.Dir)
		}
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}

	refs, err := repo.ListRefs()
	if err != nil {
		return nil, fmt.Errorf("list refs: %w", err)
	}
	type scanResult struct {
		paths []string
		err   error
	}
	scans := make([]scanResult, len(refs))
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for i, ref := range refs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			scans[i].paths, scans[i].err = repo.ListTreePaths(ref, docsPrefixes...)
		}()
	}
	wg.Wait()
	byRelease := map[string][]topologyCandidate{}
	statusRefs := map[string][]string{}
	for i, ref := range refs {
		if scans[i].err != nil {
			return nil, fmt.Errorf("scan %s: %w", ref, scans[i].err)
		}
		for _, p := range scans[i].paths {
			for _, prefix := range docsPrefixes {
				rel := strings.TrimPrefix(p, prefix+"/")
				if rel == p {
					continue
				}
				parts := strings.Split(rel, "/")
				if len(parts) == 2 && (parts[1] == "board.json" || parts[1] == "index.md") {
					byRelease[parts[0]] = append(byRelease[parts[0]], topologyCandidate{ref, p})
				}
				if len(parts) == 3 && parts[2] == "status.json" {
					statusRefs[p] = append(statusRefs[p], ref)
				}
			}
		}
		if release, ok := canonicalRelease(ref); ok {
			if _, exists := byRelease[release]; !exists {
				byRelease[release] = nil
			}
		}
	}
	var objectSpecs []string
	for path, pathRefs := range statusRefs {
		for _, ref := range pathRefs {
			objectSpecs = append(objectSpecs, ref+":"+path)
		}
	}
	for _, candidates := range byRelease {
		for _, candidate := range candidates {
			objectSpecs = append(objectSpecs, candidate.objectSpec())
		}
	}
	sort.Strings(objectSpecs)
	objects, err := repo.ReadObjects(objectSpecs)
	if err != nil {
		return nil, fmt.Errorf("read catalog objects: %w", err)
	}
	names := make([]string, 0, len(byRelease))
	for name := range byRelease {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]CatalogRecord, 0, len(names))
	for _, release := range names {
		candidates, err := validTopologyCandidates(release, byRelease[release], objects)
		if err != nil {
			return nil, err
		}
		if len(candidates) == 0 && !hasCanonicalTopologyRef(refs, release) {
			// A historical noncanonical ref is not authoritative. Without a
			// readable direct plan it cannot create a catalog entry or poison
			// the otherwise valid aggregate.
			continue
		}
		selected, err := selectTopology(release, refs, candidates)
		if err != nil {
			return nil, err
		}
		raw, ok := objects[selected.objectSpec()]
		if !ok {
			return nil, fmt.Errorf("release %q ref %s: selected board record disappeared", release, selected.ref)
		}
		tracks, err := parseTopologyRaw(raw, selected, release)
		if err != nil {
			return nil, fmt.Errorf("release %q ref %s: %w", release, selected.ref, err)
		}
		bs, err := electBoard(repo, statusRefs, objects, release, tracks)
		if err != nil {
			return nil, err
		}
		out = append(out, CatalogRecord{
			Release:        release,
			SourceRef:      selected.ref,
			Board:          bs,
			TrackDependsOn: trackDependencies(tracks),
		})
	}
	return out, nil
}

func headIsUnavailable(err error) bool {
	message := err.Error()
	for _, fragment := range []string{
		"not a git repository",
		"unknown revision or path not in the working tree",
		"Needed a single revision",
		"does not have any commits yet",
		"bad revision 'HEAD'",
		"ambiguous argument 'HEAD'",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func discoverFilesystemCatalog(repoRoot string) ([]CatalogRecord, error) {
	type filesystemTopology struct {
		release string
		prefix  string
		path    string
		raw     string
	}

	byRelease := map[string]filesystemTopology{}
	for _, prefix := range docsPrefixes {
		root := filepath.Join(repoRoot, filepath.FromSlash(prefix))
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read release directory %s: %w", root, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			release := entry.Name()
			if _, exists := byRelease[release]; exists {
				continue
			}
			releaseDir := filepath.Join(root, release)
			boardPath := filepath.Join(releaseDir, "board.json")
			indexPath := filepath.Join(releaseDir, "index.md")

			path, raw, err := readFilesystemTopology(boardPath, indexPath)
			if err != nil {
				return nil, fmt.Errorf("release %q filesystem: %w", release, err)
			}
			if path == "" {
				continue
			}
			byRelease[release] = filesystemTopology{
				release: release,
				prefix:  prefix,
				path:    filepath.ToSlash(path),
				raw:     raw,
			}
		}
	}

	names := make([]string, 0, len(byRelease))
	for release := range byRelease {
		names = append(names, release)
	}
	sort.Strings(names)

	out := make([]CatalogRecord, 0, len(names))
	for _, release := range names {
		topology := byRelease[release]
		tracks, err := parseTopologyRaw(topology.raw, topologyCandidate{path: topology.path}, release)
		if err != nil {
			return nil, fmt.Errorf("release %q filesystem: %w", release, err)
		}
		boardState, err := filesystemBoard(repoRoot, topology.prefix, release, tracks)
		if err != nil {
			return nil, err
		}
		out = append(out, CatalogRecord{
			Release:        release,
			Board:          boardState,
			TrackDependsOn: trackDependencies(tracks),
		})
	}
	return out, nil
}

func trackDependencies(tracks []TrackInfo) map[string][]string {
	dependencies := make(map[string][]string, len(tracks))
	for _, track := range tracks {
		dependencies[track.ID] = append([]string(nil), track.DependsOn...)
	}
	return dependencies
}

func readFilesystemTopology(boardPath, indexPath string) (string, string, error) {
	if raw, err := os.ReadFile(boardPath); err == nil {
		return boardPath, string(raw), nil
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("read board.json: %w", err)
	}
	if raw, err := os.ReadFile(indexPath); err == nil {
		return indexPath, string(raw), nil
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("read index.md: %w", err)
	}
	return "", "", nil
}

func filesystemBoard(repoRoot, prefix, release string, tracks []TrackInfo) (*BoardState, error) {
	trackMap := make(map[string]TrackInfo, len(tracks))
	for _, track := range tracks {
		trackMap[track.ID] = track
	}

	bs := &BoardState{Release: release}
	for _, track := range tracks {
		ts := TrackState{
			ID:             track.ID,
			WorktreeBranch: track.WorktreeBranch,
		}
		for _, sliceID := range track.Slices {
			statusPath := filepath.Join(repoRoot, filepath.FromSlash(prefix), release, sliceID, "status.json")
			raw, err := os.ReadFile(statusPath)
			if err != nil {
				if os.IsNotExist(err) {
					ts.Slices = append(ts.Slices, SliceState{ID: sliceID, Track: track.ID, State: "unknown"})
					continue
				}
				return nil, fmt.Errorf("release %q slice %q: read status.json: %w", release, sliceID, err)
			}
			e, ok := validEvidence(string(raw), "working-tree", "uncommitted", release, sliceID, track.ID)
			if !ok {
				ts.Slices = append(ts.Slices, SliceState{ID: sliceID, Track: track.ID, State: "unknown"})
				continue
			}
			ss, err := parseStatusJSON(e.raw, sliceID, track.ID, trackMap)
			if err != nil {
				return nil, fmt.Errorf("release %q slice %q: %w", release, sliceID, err)
			}
			ss.StateSource = e.source
			ss.StateDurability = e.durability
			ts.Slices = append(ts.Slices, ss)
		}
		ts.State = aggregateState(ts.Slices)
		bs.Tracks = append(bs.Tracks, ts)
	}
	return bs, nil
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

func hasCanonicalTopologyRef(refs []string, release string) bool {
	for _, ref := range refs {
		if candidateRelease, ok := canonicalRelease(ref); ok && candidateRelease == release {
			return true
		}
	}
	return false
}

func isCanonicalTopologyRef(ref, release string) bool {
	candidateRelease, ok := canonicalRelease(ref)
	return ok && candidateRelease == release
}

// validTopologyCandidates restricts rank selection to parseable direct plans.
// Canonical release-worktree records are the exception: their presence is
// authoritative, so a missing or malformed record remains a deterministic
// fail-closed error rather than allowing a lower-priority candidate to win.
func validTopologyCandidates(release string, candidates []topologyCandidate, objects map[string]string) ([]topologyCandidate, error) {
	valid := make([]topologyCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		raw, ok := objects[candidate.objectSpec()]
		if !ok {
			if isCanonicalTopologyRef(candidate.ref, release) {
				return nil, fmt.Errorf("release %q ref %s: canonical ref carries no board record", release, candidate.ref)
			}
			continue
		}
		if _, err := parseTopologyRaw(raw, candidate, release); err != nil {
			if isCanonicalTopologyRef(candidate.ref, release) {
				return nil, fmt.Errorf("release %q ref %s: %w", release, candidate.ref, err)
			}
			continue
		}
		valid = append(valid, candidate)
	}
	return valid, nil
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
	return parseTopologyRaw(raw, c, release)
}

func parseTopologyRaw(raw string, c topologyCandidate, release string) ([]TrackInfo, error) {
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

func electBoard(repo *gitpkg.Repo, statusRefs map[string][]string, statusObjects map[string]string, release string, tracks []TrackInfo) (*BoardState, error) {
	trackMap := make(map[string]TrackInfo, len(tracks))
	for _, t := range tracks {
		trackMap[t.ID] = t
	}
	bs := &BoardState{Release: release}
	for _, t := range tracks {
		ts := TrackState{
			ID:             t.ID,
			WorktreeBranch: t.WorktreeBranch,
		}
		for _, sid := range t.Slices {
			winner, err := electSlice(repo, statusRefs, statusObjects, release, sid, t.ID, trackMap)
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

func electSlice(repo *gitpkg.Repo, statusRefs map[string][]string, statusObjects map[string]string, release, sid, track string, trackMap map[string]TrackInfo) (SliceState, error) {
	path := filepath.ToSlash(filepath.Join("docs", "release", release, sid, "status.json"))
	var candidates []evidence
	for _, ref := range statusRefs[path] {
		raw, ok := statusObjects[ref+":"+path]
		if !ok {
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
