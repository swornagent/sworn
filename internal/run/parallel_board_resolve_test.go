package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/git"
)

// TestResolveReleaseBoard_PrefersReleaseWTRef is the regression guard for the
// board-read fix: the loop must read the board STRUCTURE from the release-wt ref
// (where /replan-release commits it), not from the integration-branch working
// tree. Before the fix, a release widened by /replan-release on release-wt was
// invisible to a loop launched from the integration-branch primary worktree —
// it read the stale working-tree board and built the pre-replan track list.
//
// Mutation proof (Rule 12): swap resolveReleaseBoard back to board.ReadBoard and
// this test goes red (it returns the working-tree "T-stale-integration" track).
func TestResolveReleaseBoard_PrefersReleaseWTRef(t *testing.T) {
	dir, _ := setupTestRepo(t)
	release := "2026-07-13-board-resolve"
	absBoard := filepath.Join(dir, "docs", "release", release, "board.json")

	// Integration-branch working tree: the STALE pre-replan plan.
	writeResolveTestBoard(t, absBoard, release, "T-stale-integration")
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "integration: initial plan")

	// release-wt ref: the REPLANNED board (different track).
	runCmd(t, dir, "git", "checkout", "-b", "release-wt/"+release)
	writeResolveTestBoard(t, absBoard, release, "T-replanned-on-release-wt")
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "release-wt: replanned board")

	// Back on the integration branch: the working tree holds the STALE board.
	runCmd(t, dir, "git", "checkout", "main")

	br, err := resolveReleaseBoard(context.Background(), git.New(dir), dir, release, "release-wt/"+release)
	if err != nil {
		t.Fatalf("resolveReleaseBoard: %v", err)
	}
	if len(br.Tracks) != 1 || br.Tracks[0].ID != "T-replanned-on-release-wt" {
		t.Fatalf("expected board read from the release-wt ref (T-replanned-on-release-wt), got %+v", br.Tracks)
	}
}

// TestResolveReleaseBoard_ColdStartFallsBackToWorkingTree proves that before a
// release-wt branch exists (cold start: the initial /plan-release plan is still
// on the integration branch), resolveReleaseBoard falls back to the working-tree
// board rather than failing to find a ref.
func TestResolveReleaseBoard_ColdStartFallsBackToWorkingTree(t *testing.T) {
	dir, _ := setupTestRepo(t)
	release := "2026-07-13-cold-start"
	absBoard := filepath.Join(dir, "docs", "release", release, "board.json")
	writeResolveTestBoard(t, absBoard, release, "T-integration-cold")

	// No release-wt/<release> branch exists yet.
	br, err := resolveReleaseBoard(context.Background(), git.New(dir), dir, release, "release-wt/"+release)
	if err != nil {
		t.Fatalf("resolveReleaseBoard: %v", err)
	}
	if len(br.Tracks) != 1 || br.Tracks[0].ID != "T-integration-cold" {
		t.Fatalf("expected working-tree fallback board (T-integration-cold), got %+v", br.Tracks)
	}
}

func writeResolveTestBoard(t *testing.T, absPath, release, trackID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"$schema":"https://baton.sawy3r.net/schemas/board-v1.json",` +
		`"release":{"name":"` + release + `"},` +
		`"tracks":[{"id":"` + trackID + `","slices":["S01-x"],"depends_on":[]}]}`
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
