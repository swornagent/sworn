package board

import (
	"strings"
	"testing"

	gitpkg "github.com/swornagent/sworn/internal/git"
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
