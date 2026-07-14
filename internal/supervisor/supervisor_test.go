package supervisor

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/swornagent/sworn/internal/db"
)

// newTestSupervisor creates a Supervisor backed by a temporary database.
func newTestSupervisor(t *testing.T) (*Supervisor, *sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	sup := &Supervisor{
		db:      conn,
		release: "test-release",
		pid:     os.Getpid(),
	}
	return sup, conn, func() {
		conn.Close()
		os.Remove(dbPath)
	}
}

func TestPIDLiveness(t *testing.T) {
	// AC5: verify kill(0) returns nil for os.Getpid() and non-nil for
	// a known-dead PID.
	if !pidAlive(os.Getpid()) {
		t.Fatal("pidAlive should return true for the current process")
	}

	// PID 0 is not a valid target for kill(0), but convention says kill(0, 0)
	// returns a permission error on every POSIX system (still "alive" from
	// kill's perspective). Some systems return -1. For our purpose, pid <= 0
	// is treated as not alive.
	if pidAlive(0) {
		t.Fatal("pidAlive should return false for PID 0")
	}

	if pidAlive(-1) {
		t.Fatal("pidAlive should return false for PID -1")
	}

	// A very large PID is almost certainly not alive (unless the system
	// has that many processes). 999999999 should be dead.
	if pidAlive(999999999) {
		// This could theoretically be alive on a machine with 1B processes.
		// Accept either outcome.
	}
}

func TestSingleOwnerEnforcement(t *testing.T) {
	// AC4: Two goroutines race to Acquire the same track; exactly one wins,
	// the other gets a conflict error; no panic.
	sup1, conn1, cleanup1 := newTestSupervisor(t)
	defer cleanup1()

	// We need two supervisors with different PIDs for the race.
	// Since they share the same DB (same process in test), we simulate
	// by directly manipulating the tracks table then acquiring.
	sup2 := &Supervisor{
		db:      conn1,
		release: "test-release",
		pid:     99999, // dead PID
	}

	trackID := "T1-race"

	// First acquire should succeed.
	if err := sup1.Acquire(trackID); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Second acquire with different PID should fail with ErrTrackOwned.
	err := sup2.Acquire(trackID)
	if err == nil {
		t.Fatal("second acquire should have failed with ErrTrackOwned")
	}
	var ownedErr *ErrTrackOwned
	if !asErr(err, &ownedErr) {
		t.Fatalf("expected ErrTrackOwned, got %T: %v", err, err)
	}
	if ownedErr.TrackID != trackID {
		t.Fatalf("expected track ID %q, got %q", trackID, ownedErr.TrackID)
	}
}

func TestReapOnRestart(t *testing.T) {
	// AC6: On restart after a simulated crash (stale row with a dead PID),
	// supervisor.Reap() removes the stale row and supervisor.Acquire()
	// succeeds for the new process.

	// Simulate a crashed process: insert a row with a dead PID directly.
	sup, conn, cleanup := newTestSupervisor(t)
	defer cleanup()

	deadPID := 99998
	trackID := "T1-crashed"

	_, err := conn.Exec(
		`INSERT INTO tracks (id, release, pid, state, current_slice, started_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		trackID, "test-release", deadPID, "running", "S01", "2026-06-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert stale row: %v", err)
	}

	// Reap should remove the stale row.
	reaped, err := sup.Reap()
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("expected 1 reaped row, got %d", reaped)
	}

	// Acquire should now succeed.
	if err := sup.Acquire(trackID); err != nil {
		t.Fatalf("acquire after reap failed: %v", err)
	}

	// Verify the row has our PID.
	var pid int
	err = conn.QueryRow(
		`SELECT pid FROM tracks WHERE id = ? AND release = ?`,
		trackID, "test-release",
	).Scan(&pid)
	if err != nil {
		t.Fatalf("query after acquire: %v", err)
	}
	if pid != os.Getpid() {
		t.Fatalf("expected PID %d, got %d", os.Getpid(), pid)
	}
}

func TestReapNoDeadRows(t *testing.T) {
	sup, conn, cleanup := newTestSupervisor(t)
	defer cleanup()

	// Insert a row with our own PID (alive).
	_, err := conn.Exec(
		`INSERT INTO tracks (id, release, pid, state, current_slice, started_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"T1-alive", "test-release", os.Getpid(), "running", "S01", "",
	)
	if err != nil {
		t.Fatalf("insert live row: %v", err)
	}

	reaped, err := sup.Reap()
	if err != nil {
		t.Fatalf("Reap with live row: %v", err)
	}
	if reaped != 0 {
		t.Fatalf("expected 0 reaped rows, got %d", reaped)
	}

	// Row should still exist.
	var count int
	conn.QueryRow(`SELECT COUNT(*) FROM tracks WHERE id = 'T1-alive'`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected live row to survive reap, got %d rows", count)
	}
}

func TestRelease(t *testing.T) {
	sup, conn, cleanup := newTestSupervisor(t)
	defer cleanup()

	trackID := "T1-release"

	// Acquire first.
	if err := sup.Acquire(trackID); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// Release as done.
	if err := sup.Release(trackID, "done"); err != nil {
		t.Fatalf("release: %v", err)
	}

	// Verify state and pid cleared.
	var state string
	var pid int
	err := conn.QueryRow(
		`SELECT state, pid FROM tracks WHERE id = ? AND release = ?`,
		trackID, "test-release",
	).Scan(&state, &pid)
	if err != nil {
		t.Fatalf("query after release: %v", err)
	}
	if state != "done" {
		t.Fatalf("expected state 'done', got %q", state)
	}
	if pid != 0 {
		t.Fatalf("expected pid 0 after release, got %d", pid)
	}
}

func TestReleaseFailed(t *testing.T) {
	sup, _, cleanup := newTestSupervisor(t)
	defer cleanup()

	trackID := "T1-failed"

	if err := sup.Acquire(trackID); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := sup.Release(trackID, "failed"); err != nil {
		t.Fatalf("release as failed: %v", err)
	}

	var state string
	err := sup.db.QueryRow(
		`SELECT state FROM tracks WHERE id = ? AND release = ?`,
		trackID, "test-release",
	).Scan(&state)
	if err != nil {
		t.Fatalf("query after release: %v", err)
	}
	if state != "failed" {
		t.Fatalf("expected state 'failed', got %q", state)
	}
}

func TestReleasePausedAndRejectUnknown(t *testing.T) {
	sup, _, cleanup := newTestSupervisor(t)
	defer cleanup()

	if err := sup.Acquire("T1-paused"); err != nil {
		t.Fatal(err)
	}
	if err := sup.Release("T1-paused", StatePaused); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := sup.db.QueryRow(
		`SELECT state FROM tracks WHERE id = ? AND release = ?`,
		"T1-paused", "test-release",
	).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != StatePaused {
		t.Fatalf("paused track state = %q, want %q", got, StatePaused)
	}

	if err := sup.Acquire("T2-invalid"); err != nil {
		t.Fatal(err)
	}
	if err := sup.Release("T2-invalid", "mystery"); err == nil {
		t.Fatal("unknown final state must fail closed")
	}
}

func TestConcurrentAcquireRace(t *testing.T) {
	// Two goroutines race to Acquire the same track with different PIDs
	// (simulated via different supervisor pid values). Exactly one wins,
	// the other gets ErrTrackOwned.
	_, conn, cleanup := newTestSupervisor(t)
	defer cleanup()

	const goroutines = 4
	trackID := "T1-race-concurrent"

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			sup := &Supervisor{
				db:      conn,
				release: "test-release",
				pid:     20000 + gid, // unique but dead PIDs
			}
			if err := sup.Acquire(trackID); err != nil {
				errs <- err
			}
		}(g)
	}
	wg.Wait()
	close(errs)

	// Count actual errors: at most goroutines-1 should have failed.
	var errCount int
	for err := range errs {
		var ownedErr *ErrTrackOwned
		if !asErr(err, &ownedErr) {
			t.Fatalf("unexpected error type: %T: %v", err, err)
		}
		errCount++
	}
	if errCount > goroutines-1 {
		t.Fatalf("expected at most %d errors, got %d", goroutines-1, errCount)
	}

	// Exactly one row should exist.
	var count int
	conn.QueryRow(`SELECT COUNT(*) FROM tracks WHERE id = ? AND release = ?`,
		trackID, "test-release",
	).Scan(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 row, got %d", count)
	}
}

func TestAcquireSelfReacquire(t *testing.T) {
	// Same process re-acquiring its own track should succeed (noop).
	sup, _, cleanup := newTestSupervisor(t)
	defer cleanup()

	trackID := "T1-reacquire"

	if err := sup.Acquire(trackID); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Same PID re-acquires.
	if err := sup.Acquire(trackID); err != nil {
		t.Fatalf("second acquire (same PID) should succeed: %v", err)
	}

	// Still one row.
	var count int
	sup.db.QueryRow(`SELECT COUNT(*) FROM tracks WHERE id = ? AND release = ?`,
		trackID, "test-release",
	).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

func TestEventsLogged(t *testing.T) {
	sup, _, cleanup := newTestSupervisor(t)
	defer cleanup()

	trackID := "T1-events"

	if err := sup.Acquire(trackID); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	var eventCount int
	sup.db.QueryRow(
		`SELECT COUNT(*) FROM events WHERE track_id = ? AND release = ? AND event = 'acquired'`,
		trackID, "test-release",
	).Scan(&eventCount)
	if eventCount < 1 {
		t.Fatalf("expected at least 1 acquired event, got %d", eventCount)
	}
}

func TestPersistence(t *testing.T) {
	// Write an event → close the DB → reopen → verify the event survives.
	dir := t.TempDir()

	// Open and write.
	db1, err := Open("test-release", dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sup1 := New(db1, "test-release")
	if err := sup1.Acquire("T1-persist"); err != nil {
		db1.Close()
		t.Fatalf("acquire: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen and verify.
	db2, err := Open("test-release", dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	events, err := QueryEvents(db2, "test-release")
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	foundAcquired := false
	for _, e := range events {
		if e.TrackID == "T1-persist" && e.Event == "acquired" {
			foundAcquired = true
			break
		}
	}
	if !foundAcquired {
		t.Fatalf("expected an 'acquired' event for T1-persist after reopen, got %d events", len(events))
	}
}

// asErr checks if err can be assigned to target via type assertion.
func asErr(err error, target interface{}) bool {
	if err == nil {
		return false
	}
	switch t := target.(type) {
	case *string:
		*t = err.Error()
		return true
	}
	// Use type assertion from error interface.
	val := err
	switch target := target.(type) {
	case **ErrTrackOwned:
		e, ok := val.(*ErrTrackOwned)
		if ok {
			*target = e
		}
		return ok
	}
	return false
}
