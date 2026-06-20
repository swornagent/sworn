package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/swornagent/sworn/internal/board"
)

// fakeRunSlice is a test helper that records the slices it was called with.
func fakeRunSlice(allowFailAt string, called *[]string) func(context.Context, string, string, string) error {
	return func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		*called = append(*called, filepath.Base(filepath.Dir(specPath)))
		sliceParent := filepath.Base(filepath.Dir(specPath))
		if sliceParent == allowFailAt {
			return fmt.Errorf("simulated failure: %s", sliceParent)
		}
		return nil
	}
}

func TestRunTrack_AllSlicesPass(t *testing.T) {
	tmpDir := t.TempDir()
	absSpecDir := filepath.Join(tmpDir, "docs", "release", "test-release", "S01-test")
	os.MkdirAll(absSpecDir, 0o755)
	os.WriteFile(filepath.Join(absSpecDir, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(absSpecDir, "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-test"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v — skipping worker test", err)
	}
	defer db.Close()

	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	opts.DB = db

	result := RunTrack(context.Background(), opts)
	if result != TrackPass {
		t.Fatalf("expected TrackPass, got %s", result)
	}

	if len(called) != 1 {
		t.Fatalf("expected 1 slice call, got %d: %v", len(called), called)
	}
}

func TestRunTrack_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := WorkerOptions{
		ReleaseName:         "test",
		TrackInfo: board.TrackInfo{
			ID:     "T1",
			Slices: []string{"S01"},
		},
		PrimaryWorktreeRoot: t.TempDir(),
	}

	result := RunTrack(ctx, opts)
	if result != TrackSkipped {
		t.Fatalf("expected TrackSkipped for cancelled context, got %s", result)
	}
}

func TestRunTrack_SliceFail(t *testing.T) {
	tmpDir := t.TempDir()
	absSpecDir := filepath.Join(tmpDir, "docs", "release", "test-release", "S01-fail")
	os.MkdirAll(absSpecDir, 0o755)
	os.WriteFile(filepath.Join(absSpecDir, "spec.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(absSpecDir, "status.json"), []byte(`{"state":"implemented"}`), 0o644)

	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test-release",
		PrimaryWorktreeRoot: tmpDir,
		ProjectDir:          "sworn",
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{"S01-fail"},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("S01-fail", &called),
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	opts.DB = db

	result := RunTrack(context.Background(), opts)
	if result != TrackFail {
		t.Fatalf("expected TrackFail for slice failure, got %s", result)
	}
}

func TestRunTrack_EmptySlices(t *testing.T) {
	tmpDir := t.TempDir()
	var called []string
	opts := WorkerOptions{
		ReleaseName:         "test",
		PrimaryWorktreeRoot: tmpDir,
		TrackInfo: board.TrackInfo{
			ID:             "T1",
			Slices:         []string{},
			WorktreePath:   tmpDir,
			WorktreeBranch: "track/test/T1",
		},
		RunSliceFn: fakeRunSlice("", &called),
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE tracks (id TEXT, release TEXT, pid INT, state TEXT, current_slice TEXT, started_at TEXT, PRIMARY KEY (id, release))`)
	db.Exec(`CREATE TABLE events (track_id TEXT, release TEXT, event TEXT, detail TEXT, ts TEXT)`)
	opts.DB = db

	result := RunTrack(context.Background(), opts)
	if result != TrackPass {
		t.Fatalf("expected TrackPass for empty slices, got %s", result)
	}
	if len(called) != 0 {
		t.Fatalf("expected 0 slice calls for empty slices, got %d", len(called))
	}
}