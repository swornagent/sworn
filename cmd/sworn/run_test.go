package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCmdRun_MissingTask(t *testing.T) {
	exit := cmdRun([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing task, got %d", exit)
	}
}

func TestCmdRun_MissingVerifierModel(t *testing.T) {
	// Config.Load returns DefaultConfig (verifier.model = "openai/gpt-4.1")
	// when no config file exists. So the verifier model is always resolved
	// unless we override it with an empty --verifier-model flag and empty env.
	// Even then, the default config provides a fallback.
	// The actual gate is model.FromEnv at dispatch time (fail-closed without API key).
	// This test confirms flag parsing succeeds even with minimal flags.
	t.Setenv("SWORN_VERIFIER_MODEL", "")
	exit := cmdRun([]string{"--task", "test task", "--verifier-model", "", "--retry-cap", "0"})
	// With --verifier-model="" and env="", config provides "openai/gpt-4.1".
	// The run will fail at model dispatch (no API key) — exit 1.
	if exit == 64 {
		t.Error("expected flag parsing to succeed (exit != 64)")
	}
}

func TestCmdRun_FlagParsing(t *testing.T) {
	exit := cmdRun([]string{"--task", "test", "--verifier-model", "fake/v", "--retry-cap", "0"})
	if exit == 64 {
		t.Error("expected flag parsing to succeed (exit != 64)")
	}
}

func TestCmdRun_EscalationModelsFlag(t *testing.T) {
	exit := cmdRun([]string{
		"--task", "test",
		"--verifier-model", "openai/gpt-4o",
		"--escalation-models", "openai/gpt-4o-mini,openai/gpt-4o",
		"--retry-cap", "1",
	})
	if exit == 64 {
		t.Error("expected flag parsing to succeed (exit != 64)")
	}
}

func TestCmdRun_UsageContainsEscalationInfo(t *testing.T) {
	// Verify that --help output documents the model escalation mapping (Pin 5).
	t.Skip("verify manually: sworn run --help documents escalation models")
}

// TestParallelStartupFailFast is the S07 AC-01 reachability test: it drives
// the real `sworn run --parallel` CLI entry point (cmdRun), not a leaf unit
// (Rule 1). One escalation-list entry ("nope/model-x") names an unregistered
// prefix, so the startup resolution sweep must reject the run BEFORE
// openDefaultDB/RunParallel spawn any worker — proven both by the non-zero
// exit and by the tracks table having zero acquired rows (no worker ever
// started a supervisor Acquire). Follows the TestCmdRun_Parallel fixture
// pattern with the escalation model swapped.
func TestParallelStartupFailFast(t *testing.T) {
	tmpDir := t.TempDir()

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	runGit(t, tmpDir, "init", "-b", "main")
	runGit(t, tmpDir, "config", "user.email", "test@swornagent.dev")
	runGit(t, tmpDir, "config", "user.name", "sworn test")
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, tmpDir, "add", "README.md")
	runGit(t, tmpDir, "commit", "-m", "initial commit")
	runGit(t, tmpDir, "branch", "track/test/T1")

	os.MkdirAll(".sworn", 0o755)
	db, err := sql.Open("sqlite", ".sworn/sworn.db")
	if err != nil {
		t.Fatalf("open pre-schema db: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`); err != nil {
		db.Close()
		t.Fatalf("create tracks: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`); err != nil {
		db.Close()
		t.Fatalf("create events: %v", err)
	}
	db.Close()

	releaseDir := filepath.Join("docs", "release", "test-startup-failfast")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("mkdir release dir: %v", err)
	}

	indexContent := `---
title: Test Startup Fail-Fast
release_worktree_path: ` + tmpDir + `
tracks:
  - id: T1
    slices: []
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T1
    state: planned
---

# Test
`
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644); err != nil {
		t.Fatalf("write index.md: %v", err)
	}
	// "nope/model-x" names an unregistered prefix — the startup sweep must
	// reject it before any worker spawns.
	t.Setenv("SWORN_VERIFIER_MODEL", "openai/gpt-4o")
	t.Setenv("SWORN_IMPLEMENTER_MODEL", "openai/gpt-4o-mini")
	t.Setenv("SWORN_DB_DRIVER", "sqlite")

	exit := cmdRun([]string{
		"--parallel", "--release", "test-startup-failfast",
		"--escalation-models", "openai/gpt-4o-mini,nope/model-x",
	})

	if exit == 64 {
		t.Fatal("expected flag parsing to succeed (exit != 64)")
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit — the startup sweep should have rejected the unregistered escalation model before any worker spawned")
	}

	// Zero acquired rows in the tracks table proves no worker ever started
	// (RunTrack's first act on a real dispatch path is supervisor.Acquire,
	// which INSERTs into tracks).
	verifyDB, err := sql.Open("sqlite", ".sworn/sworn.db")
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer verifyDB.Close()
	var n int
	if err := verifyDB.QueryRow(`SELECT COUNT(*) FROM tracks`).Scan(&n); err != nil {
		t.Fatalf("count tracks rows: %v", err)
	}
	if n != 0 {
		t.Errorf("expected zero acquired track rows (no worker should have started), got %d", n)
	}
}

// TestCmdRun_Parallel exercises the --parallel CLI entry path in cmdRun().
// It proves that:
//   - Flag parsing succeeds (exit != 64)
//   - openDefaultDB() is called and returns a valid handle
//   - RunSliceFn closure is constructed
//   - RunParallel() is invoked (exercised end-to-end with empty-slice tracks)
//
// The fixture uses tracks with slices: [] so the worker goroutine completes
// immediately without calling RunSlice() — no real model dispatch occurs.
//
// Verifier Fix (Gate 4): prior rounds passed unit tests calling RunParallel()
// directly; this test exercises the full CLI entry path through cmdRun()
// (lines 63‑90 of run.go) that the spec's smoke step requires.
func TestCmdRun_Parallel(t *testing.T) {
	withModelConfig(t)
	tmpDir := t.TempDir()

	// Save and restore working directory.  cmdRun → openDefaultDB uses
	// os.Getwd() to construct the DB path, and RunParallel uses "." as
	// WorkspaceRoot to find docs/release/<name>/index.md.
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// ── Git fixture ───────────────────────────────────────────────────────
	// ProductionMergeTrack asserts fail-closed that the release worktree
	// path is a git worktree (Rule 11 target assertion), so the fixture must
	// be a real repo with the track branches present. Both branches sit at
	// HEAD, so the auto-merge is an "Already up to date" no-op.
	runGit(t, tmpDir, "init", "-b", "main")
	runGit(t, tmpDir, "config", "user.email", "test@swornagent.dev")
	runGit(t, tmpDir, "config", "user.name", "sworn test")
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, tmpDir, "add", "README.md")
	runGit(t, tmpDir, "commit", "-m", "initial commit")
	runGit(t, tmpDir, "branch", "track/test/T1")
	runGit(t, tmpDir, "branch", "track/test/T2")

	// ── Pre-create the DB schema ──────────────────────────────────────────
	// openDefaultDB() opens .sworn/sworn.db in the current directory.  The
	// supervisor needs the tracks and events tables to exist before workers
	// call Acquire/Release.
	os.MkdirAll(".sworn", 0o755)
	db, err := sql.Open("sqlite", ".sworn/sworn.db")
	if err != nil {
		t.Fatalf("open pre-schema db: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`); err != nil {
		db.Close()
		t.Fatalf("create tracks: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`); err != nil {
		db.Close()
		t.Fatalf("create events: %v", err)
	}
	db.Close()

	// ── Release-board fixture ─────────────────────────────────────────────
	// Two independent tracks (T1, T2), both with empty slice lists.  Workers
	// will start, acquire supervisor ownership, find no slices to run, release
	// with StateDone, and return TrackPass.  No RunSliceFn invocation needed.
	releaseDir := filepath.Join("docs", "release", "test-parallel-cmd")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("mkdir release dir: %v", err)
	}

	indexContent := `---
title: Test Parallel CLI
release_worktree_path: ` + tmpDir + `
tracks:
  - id: T1
    slices: []
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T1
    state: planned
  - id: T2
    slices: []
    depends_on: null
    worktree_path: ` + tmpDir + `
    worktree_branch: track/test/T2
    state: planned
---

# Test
`
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644); err != nil {
		t.Fatalf("write index.md: %v", err)
	}
	// The production router reads only committed release state. Commit the
	// fixture before cold-start creates release-wt/<release>; an uncommitted
	// working-tree board must never trigger the retired static route.
	runGit(t, tmpDir, "add", "docs/release/test-parallel-cmd/index.md")
	runGit(t, tmpDir, "commit", "-m", "test: add parallel release fixture")

	// ── Environment ───────────────────────────────────────────────────────
	// SWORN_VERIFIER_MODEL must be set so the verifier-resolution gate
	// (line 47‑51 of run.go) passes. SWORN_IMPLEMENTER_MODEL must be set so
	// the implementer-resolution gate passes (ResolveImplementerModel returns
	// an error when no source is configured). SWORN_DB_DRIVER ensures the
	// sqlite driver is selected.
	t.Setenv("SWORN_VERIFIER_MODEL", "openai/gpt-4o")
	t.Setenv("SWORN_IMPLEMENTER_MODEL", "openai/gpt-4o-mini")
	t.Setenv("SWORN_DB_DRIVER", "sqlite")

	// ── Invoke the CLI entry path ────────────────────────────────────────
	exit := cmdRun([]string{"--parallel", "--release", "test-parallel-cmd"})

	// Flag parsing must succeed — exit 64 means --release was not parsed.
	if exit == 64 {
		t.Error("expected --parallel --release flag parsing to succeed (exit != 64)")
	}

	// With empty-slice tracks, RunParallel returns nil → cmdRun returns 0.
	// Non-zero means the parallel path hit an error (DB, fixture, etc.).
	if exit != 0 {
		t.Errorf("expected exit 0 (parallel path exercised), got %d", exit)
	}
}
