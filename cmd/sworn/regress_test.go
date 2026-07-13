package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/command"
)

// TestRegressCommandRegistered is the reachability check: `sworn regress` must
// be wired into the command registry, else the merge-track gate can't call it.
func TestRegressCommandRegistered(t *testing.T) {
	c, ok := command.Lookup("regress")
	if !ok {
		t.Fatal("command.Lookup(\"regress\") not found — regress is not registered")
	}
	if c.Run == nil {
		t.Error("regress Run must be non-nil")
	}
}

// TestRegressWorktreeFlagFailsClosed verifies the --worktree override (used by
// the merge-track gate to run the affected-package suite in a TRACK worktree):
// a missing/non-directory worktree must fail closed (exit 2) rather than run the
// suite against the wrong place. It also proves the override bypasses index.md
// resolution — the test runs from a temp dir with no docs/release tree at all,
// so reaching the worktree-stat guard at all means index.md was skipped.
func TestRegressWorktreeFlagFailsClosed(t *testing.T) {
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	exit := cmdRegress([]string{"--release", "2026-01-01-x", "--worktree", "/no/such/worktree/path"})
	if exit != 2 {
		t.Fatalf("expected fail-closed exit 2 for nonexistent --worktree, got %d", exit)
	}
}

// TestRegressDefaultResolution_BoardJSON is the AC-02 integration point:
// `sworn regress --release <name>` (no --worktree override) resolves
// release_worktree_path via board.json instead of erroring on the removed
// index.md-frontmatter scraper. index.md is generated through the real
// board.RenderToFile path (AC-04), not hand-authored, so the fixture proves
// the command against what `sworn render` actually produces.
func TestRegressDefaultResolution_BoardJSON(t *testing.T) {
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	repoDir := t.TempDir()
	worktreeTarget := t.TempDir() // stands in for the release worktree — no go.mod, resolves fast
	release := "regress-test-boardjson"

	setupRegressFixture(t, repoDir, release, worktreeTarget)

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	exit := cmdRegress([]string{"--release", release})
	if exit == 2 {
		t.Fatalf("expected resolution to succeed past the worktree-path lookup (exit 0 or 1), got exit 2")
	}
}

// TestRegressDefaultResolution_LegacyIndexMDFallback is the AC-03 regression
// guard: a release with no board.json (genuinely pre-ADR-0009) still
// resolves release_worktree_path via board.ReadBoard's lazy
// migrateFromIndex fallback.
func TestRegressDefaultResolution_LegacyIndexMDFallback(t *testing.T) {
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	repoDir := t.TempDir()
	worktreeTarget := t.TempDir()
	release := "regress-test-legacy"

	releaseDir := filepath.Join(repoDir, "docs", "release", release)
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	indexContent := fmt.Sprintf(`---
release_worktree_path: %s
tracks:
  - id: T1-core
    worktree_branch: track/%s/T1-core
    state: in_progress
    slices:
      - S01-legacy
---
# legacy regress test — no board.json
`, worktreeTarget, release)
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}

	// board-v1 is a pure plan (sworn#80): regress DERIVES the release worktree path
	// from the primary repo root regardless of whether the release has board.json or
	// only a legacy index.md. Make repoDir a real git repo + create the derived dir.
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@test")
	runGit(t, repoDir, "config", "user.name", "test")
	if err := os.MkdirAll(board.ReleaseWorktreePathFrom(repoDir, release), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = worktreeTarget

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	exit := cmdRegress([]string{"--release", release})
	if exit == 2 {
		t.Fatalf("expected the release worktree path to resolve past the lookup (exit 0 or 1), got exit 2")
	}
}

// --- helpers ---

// setupRegressFixture writes a board.json + real board.RenderToFile-generated
// index.md for one slice (S01-boardjson, verified) whose track's worktree
// path is worktreeTarget. Shared by the AC-02 default-resolution test.
func setupRegressFixture(t *testing.T, repoDir, release, worktreeTarget string) {
	t.Helper()

	releaseDir := filepath.Join(repoDir, "docs", "release", release)
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	// board-v1 is a pure plan (sworn#80): regress DERIVES the release worktree path
	// from the primary repo root, no longer reading the persisted worktreeTarget.
	// Make repoDir a real git repo (so PrimaryWorktreeRoot resolves) and create the
	// derived release worktree dir so the fail-closed stat guard passes.
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@test")
	runGit(t, repoDir, "config", "user.name", "test")
	if err := os.MkdirAll(board.ReleaseWorktreePathFrom(repoDir, release), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = worktreeTarget

	boardContent := fmt.Sprintf(`{
  "schema_version": 1,
  "release": {"name": %q},
  "release_worktree_path": %q,
  "release_worktree_branch": "release-wt/%s",
  "tracks": [
    {
      "id": "T1-core",
      "slices": ["S01-boardjson"],
      "worktree_branch": "track/%s/T1-core",
      "state": "in_progress"
    }
  ]
}`, release, worktreeTarget, release, release)
	if err := os.WriteFile(filepath.Join(releaseDir, "board.json"), []byte(boardContent), 0644); err != nil {
		t.Fatal(err)
	}

	sliceDir := filepath.Join(releaseDir, "S01-boardjson")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := fmt.Sprintf(`{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "schema_version": 1,
  "slice_id": "S01-boardjson",
  "release": %q,
  "user_outcome": "fixture slice for regress_test.go",
  "covers_needs": ["N-01"],
  "effort_complexity": {"effort": "low", "complexity": "low", "quadrant": "chore"},
  "touchpoints": ["some-other-file.go"],
  "acceptance_criteria": []
}`, release)
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.json"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
	statusContent := `{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S01-boardjson",
  "release": "` + release + `",
  "track": "T1-core",
  "state": "verified",
  "owner": "agent",
  "last_updated_at": "2026-01-01T00:00:00Z",
  "verification": {"result": "pass", "violations": []}
}`
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(statusContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := board.RenderToFile(repoDir, release); err != nil {
		t.Fatalf("render index.md via board.RenderToFile: %v", err)
	}
}
