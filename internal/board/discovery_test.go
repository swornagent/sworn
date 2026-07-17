package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitpkg "github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/state"
)

func TestDiscoverCatalogSourceRefRanking(t *testing.T) {
	release := "r"
	c := []topologyCandidate{{"refs/remotes/z/release-wt/r", "p"}, {"refs/heads/topic", "p"}, {"refs/heads/release-wt/r", "p"}, {"refs/remotes/a/release-wt/r", "p"}}
	got, err := selectTopology(release, []string{"refs/heads/release-wt/r"}, c)
	if err != nil {
		t.Fatal(err)
	}
	if got.ref != "refs/heads/release-wt/r" {
		t.Fatalf("got %s", got.ref)
	}
	got, err = selectTopology(release, nil, []topologyCandidate{{"refs/remotes/z/topic", "p"}, {"refs/heads/topic", "p"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.ref != "refs/heads/topic" {
		t.Fatalf("fallback got %s", got.ref)
	}
}

func TestDiscoverCatalogCanonicalSkewFailsClosed(t *testing.T) {
	_, err := selectTopology("r", []string{"refs/heads/release-wt/r"}, []topologyCandidate{{"refs/heads/topic", "p"}})
	if err == nil {
		t.Fatal("expected canonical skew error")
	}
}

func TestDiscoverCatalogSelectedNoncanonicalMalformedBoardFailsClosed(t *testing.T) {
	repo := gitpkg.New(t.TempDir())
	_, err := parseTopology(repo, topologyCandidate{
		ref:  "refs/heads/a-selected",
		path: "docs/release/r/board.json",
	}, "r")
	if err == nil {
		t.Fatal("expected selected source read error")
	}
	if !strings.Contains(err.Error(), "git show") {
		t.Fatalf("error = %q, want selected-source read failure", err)
	}
}

func TestDiscoverCatalogFilesystemFallbackWithoutUsableHead(t *testing.T) {
	root := t.TempDir()
	writeFilesystemCatalogFixture(t, root, "local-zeta", "T2-zeta", "S02-zeta", "planned")
	writeFilesystemCatalogFixture(t, root, "local-alpha", "T1-core", "S01-alpha", "verified")

	before := snapshotFiles(t, root)
	catalog, err := DiscoverCatalog(gitpkg.New(root))
	if err != nil {
		t.Fatalf("DiscoverCatalog: %v", err)
	}
	after := snapshotFiles(t, root)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("filesystem fallback mutated files:\nbefore=%v\nafter=%v", before, after)
	}

	if got := catalogReleaseIDs(catalog); strings.Join(got, ",") != "local-alpha,local-zeta" {
		t.Fatalf("release order = %v, want [local-alpha local-zeta]", got)
	}
	alpha := catalog[0]
	if alpha.SourceRef != "" {
		t.Fatalf("sourceRef = %q, want empty filesystem identity", alpha.SourceRef)
	}
	if alpha.Board == nil || len(alpha.Board.Tracks) != 1 || alpha.Board.Tracks[0].ID != "T1-core" {
		t.Fatalf("alpha board = %+v, want T1-core", alpha.Board)
	}
	ss := alpha.Board.Tracks[0].Slices[0]
	if ss.ID != "S01-alpha" || ss.State != "verified" || ss.StateSource != "working-tree" || ss.StateDurability != "uncommitted" {
		t.Fatalf("alpha state = %+v", ss)
	}
}

func writeFilesystemCatalogFixture(t *testing.T, root, release, track, sliceID, sliceState string) {
	t.Helper()
	br := &BoardRecord{
		Release: StringRelease(release),
		Tracks: []BoardTrack{{
			ID:     track,
			Slices: []string{sliceID},
		}},
	}
	if err := WriteBoard(root, release, br); err != nil {
		t.Fatalf("WriteBoard(%s): %v", release, err)
	}
	statusPath := filepath.Join(root, "docs", "release", release, sliceID, "status.json")
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}
	if err := state.Write(statusPath, &state.Status{
		SliceID: sliceID,
		Release: release,
		Track:   track,
		State:   state.State(sliceState),
	}); err != nil {
		t.Fatalf("state.Write(%s): %v", release, err)
	}
}

func snapshotFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel)+"="+string(body))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func catalogReleaseIDs(catalog []CatalogRecord) []string {
	ids := make([]string, 0, len(catalog))
	for _, rec := range catalog {
		ids = append(ids, rec.Release)
	}
	return ids
}
