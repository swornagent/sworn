package run

// S14-blocked-terminal — AC-05 exit-report test (new file by design: AC-06
// forbids edits to existing test files, including parallel_test.go).

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/swornagent/sworn/internal/orchestrator"
)

// TestLoopExitReportBlockedVsFail runs two independent tracks — one whose
// slice ends blocked-terminal (the RunSlice sentinel error shape), one that
// plain-fails — and asserts the run exits non-zero with a report that
// distinguishes BLOCKED lanes (blocker verbatim + route-to-/replan-release
// directive) from FAIL lanes (retries exhausted).
func TestLoopExitReportBlockedVsFail(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-report")
	os.MkdirAll(releaseDir, 0o755)

	for _, sid := range []string{"S01-t1-slice", "S02-t2-slice"} {
		d := filepath.Join(tmpDir, "docs", "release", "test-report", sid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "spec.md"), []byte("# test"), 0o644)
		os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"state":"implemented"}`), 0o644)
	}

	indexContent := `---
title: Test Blocked Report
release_worktree_path: ` + tmpDir + `
tracks:
  - id: T1
    slices: [S01-t1-slice]
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T1
    state: planned
  - id: T2
    slices: [S02-t2-slice]
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T2
    state: planned
---

# Test
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644)

	// The reference S05 blocker text, emitted verbatim end to end (R-03).
	const blocker = "spec defect: section-ownership model undefined — only /replan-release can clear it"

	runSliceFn := func(_ context.Context, _, specPath, _ string) error {
		switch filepath.Base(filepath.Dir(specPath)) {
		case "S01-t1-slice":
			// The exact blocked-terminal error shape RunSlice emits for an
			// implementer StatusBlocked result (sentinel + verbatim reason +
			// route-directive suffix).
			return fmt.Errorf("%s %s%s", orchestrator.BlockedLaneSentinel, blocker, orchestrator.BlockedLaneRouteSuffix)
		default:
			return fmt.Errorf("simulated retries-exhausted failure")
		}
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)

	preCreateDerivedWorktrees(t, tmpDir, "test-report")
	err = RunParallel(context.Background(), ParallelOptions{
		LegacyStaticIteration: true,
		ReleaseName:           "test-report",
		WorkspaceRoot:         tmpDir,
		DB:                    db,
		RunSliceFn:            runSliceFn,
		ProjectDir:            "sworn",
	})
	// Non-zero outcome: RunParallel returns a non-nil error, which cmd/sworn
	// prints and converts to exit 1 (existing plumbing).
	if err == nil {
		t.Fatal("expected non-nil error when a lane is blocked, got nil")
	}
	report := err.Error()

	// BLOCKED section: header, lane line with track + slice + verbatim
	// blocker, and the explicit replan directive carrying the release name.
	if !strings.Contains(report, "BLOCKED lanes — terminal for this run; route to /replan-release:") {
		t.Errorf("report missing BLOCKED section header:\n%s", report)
	}
	if !strings.Contains(report, "[T1] S01-t1-slice: "+blocker) {
		t.Errorf("report missing BLOCKED lane with verbatim blocker:\n%s", report)
	}
	if !strings.Contains(report, "-> /replan-release test-report") {
		t.Errorf("report missing route-to-/replan-release directive:\n%s", report)
	}

	// FAIL section: names the exhausted track, distinct from the BLOCKED lane.
	if !strings.Contains(report, "FAIL lanes — retries exhausted:") {
		t.Errorf("report missing FAIL section header:\n%s", report)
	}
	if !strings.Contains(report, "[T2]") {
		t.Errorf("report must name the plain-fail track T2:\n%s", report)
	}

	// The worker trims the route-directive suffix from the recorded reason,
	// so the report renders the directive once per lane, not twice (flag (a)).
	if strings.Contains(report, "(BLOCKED is terminal for this lane)") {
		t.Errorf("report must not duplicate the in-error route directive:\n%s", report)
	}

	// The same report lands in the durable loop log.
	logBytes, logErr := os.ReadFile(filepath.Join(tmpDir, ".sworn", "logs", "test-report", "loop.log"))
	if logErr != nil {
		t.Fatalf("read loop log: %v", logErr)
	}
	if !strings.Contains(string(logBytes), "BLOCKED lanes — terminal for this run") {
		t.Errorf("loop log missing the blocked-vs-fail report:\n%s", string(logBytes))
	}
}
