// Package supervisor manages worker process ownership via a SQLite-backed
// process registry. It provides single-owner enforcement per track (a
// constraint-level guarantee) and stale-PID reaping on restart.
//
// Stdlib + modernc.org/sqlite only. The sqlite dependency is documented in
// ADR-0003 as an exception to the project's stdlib-only policy.
package supervisor

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/swornagent/sworn/internal/db"
)
// State constants for the tracks table.
const (
	StatePlanned = "planned"
	StateRunning = "running"
	StateDone    = "done"
	StateFailed  = "failed"
)

// ErrTrackOwned is returned by Acquire when another process already owns the
// requested track with a live PID.
type ErrTrackOwned struct {
	TrackID string
	Release string
	PID     int
}

func (e *ErrTrackOwned) Error() string {
	return fmt.Sprintf("supervisor: track %q in release %q already owned by PID %d", e.TrackID, e.Release, e.PID)
}

// Supervisor manages worker process ownership for a release.
type Supervisor struct {
	db      *sql.DB
	eventDB *sql.DB
	release string
	pid     int
}
// New creates a Supervisor bound to the given database and release name.
func New(db *sql.DB, release string) *Supervisor {
	return &Supervisor{
		db:      db,
		release: release,
		pid:     os.Getpid(),
	}
}

// Reap scans all tracks for the supervisor's release, checks PID liveness,
// and removes rows where the owning process is dead. Returns the number of
// reaped rows.
//
// Note: this function collects all candidate rows into memory first, then
// deletes them, to avoid nesting a query and exec on the same connection
// (important with SetMaxOpenConns(1) in the db pool).
func (s *Supervisor) Reap() (int, error) {
	rows, err := s.db.Query(
		`SELECT id, pid FROM tracks WHERE release = ? AND pid != 0 AND state = ?`,
		s.release, StateRunning,
	)
	if err != nil {
		return 0, fmt.Errorf("supervisor: query tracks for reap: %w", err)
	}

	// Collect all candidate rows first, then close the result set.
	type staleRow struct {
		trackID string
		pid     int
	}
	var stales []staleRow
	for rows.Next() {
		var trackID string
		var pid int
		if err := rows.Scan(&trackID, &pid); err != nil {
			rows.Close()
			return 0, fmt.Errorf("supervisor: scan track row: %w", err)
		}
		if !pidAlive(pid) {
			stales = append(stales, staleRow{trackID, pid})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("supervisor: iterate tracks: %w", err)
	}

	// Now delete stale rows (no longer holding a rowset open).
	var reaped int
	for _, stale := range stales {
		if _, err := s.db.Exec(
			`DELETE FROM tracks WHERE id = ? AND release = ? AND pid = ?`,
			stale.trackID, s.release, stale.pid,
		); err != nil {
			return reaped, fmt.Errorf("supervisor: delete stale row %q: %w", stale.trackID, err)
		}
		_ = s.logEvent(stale.trackID, "reaped", fmt.Sprintf("stale PID %d", stale.pid))
		reaped++
	}

	return reaped, nil
}

// Acquire attempts to claim ownership of a track. It inserts a row into the
// tracks table with the current PID and state=running. If a row already exists
// for this track+release and the owner PID is alive, it returns ErrTrackOwned.
// If the existing owner PID is dead, it replaces the row (delete + insert).
//
// Concurrency safety: uses a transaction with INSERT first (atomic PRIMARY KEY
// enforcement), then falls back to checking the existing owner on constraint
// violation. This is race-safe without external locking.
func (s *Supervisor) Acquire(trackID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("supervisor: begin tx: %w", err)
	}
	defer tx.Rollback() // no-op if committed

	now := time.Now().UTC().Format(time.RFC3339)

	// Try INSERT first. If the row exists, we get a PRIMARY KEY violation
	// and fall through to the conflict handler.
	_, err = tx.Exec(
		`INSERT INTO tracks (id, release, pid, state, current_slice, started_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		trackID, s.release, s.pid, StateRunning, "", now,
	)
	if err == nil {
		// INSERT succeeded — we own this track.
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("supervisor: commit acquire: %w", err)
		}
		_ = s.logEvent(trackID, "acquired", fmt.Sprintf("PID %d", s.pid))
		return nil
	}

	// INSERT failed — likely a PRIMARY KEY / UNIQUE constraint violation.
	// This means a row already exists. Check who owns it.
	var existingPID int
	var existingState string
	err = tx.QueryRow(
		`SELECT pid, state FROM tracks WHERE id = ? AND release = ?`,
		trackID, s.release,
	).Scan(&existingPID, &existingState)
	if err != nil {
		return fmt.Errorf("supervisor: query existing track %q: %w", trackID, err)
	}

	// If the existing PID is alive, this track is owned.
	if existingPID != 0 && pidAlive(existingPID) {
		if existingPID == s.pid {
			// Same process re-acquiring — allowed.
			_, _ = tx.Exec(
				`UPDATE tracks SET started_at = ? WHERE id = ? AND release = ?`,
				now, trackID, s.release,
			)
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("supervisor: commit re-acquire: %w", err)
			}
			return nil
		}
		return &ErrTrackOwned{
			TrackID: trackID,
			Release: s.release,
			PID:     existingPID,
		}
	}

	// Dead PID. Delete the stale row and re-insert.
	if _, err := tx.Exec(
		`DELETE FROM tracks WHERE id = ? AND release = ?`,
		trackID, s.release,
	); err != nil {
		return fmt.Errorf("supervisor: delete stale row for acquire: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO tracks (id, release, pid, state, current_slice, started_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		trackID, s.release, s.pid, StateRunning, "", now,
	)
	if err != nil {
		return fmt.Errorf("supervisor: re-insert after stale delete: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("supervisor: commit re-acquire after reap: %w", err)
	}

	_ = s.logEvent(trackID, "acquired", fmt.Sprintf("PID %d (replaced stale)", s.pid))
	return nil
}

// Release marks a track as done or failed and clears the PID, releasing
// ownership. It is safe to call multiple times; a row that doesn't exist
// is silently ignored.
func (s *Supervisor) Release(trackID string, state string) error {
	if state != StateDone && state != StateFailed {
		state = StateDone
	}

	result, err := s.db.Exec(
		`UPDATE tracks SET state = ?, pid = 0, current_slice = '' WHERE id = ? AND release = ? AND pid = ?`,
		state, trackID, s.release, s.pid,
	)
	if err != nil {
		return fmt.Errorf("supervisor: release %q: %w", trackID, err)
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		_ = s.logEvent(trackID, "released-"+state, fmt.Sprintf("PID %d", s.pid))
	}
	return nil
}

// MustRelease is a defer-safe convenience wrapper for Release calls. It logs
// the error rather than panicking.
func (s *Supervisor) MustRelease(trackID string, state string) {
	if err := s.Release(trackID, state); err != nil {
		fmt.Fprintf(os.Stderr, "supervisor: release %s/%s: %v\n", s.release, trackID, err)
	}
}

// SetEventDB sets an alternative database for event writes. When non-nil,
// logEvent writes events to this database instead of the main DB. This
// allows process-ownership to use sworn.db while events are routed to a
// release-specific supervisor-<release>.db.
func (s *Supervisor) SetEventDB(db *sql.DB) {
	s.eventDB = db
}

// logEvent writes an audit event to the events table. Errors are silently
// dropped — auditing should never block the critical path.
func (s *Supervisor) logEvent(trackID, event, detail string) error {
	target := s.db
	if s.eventDB != nil {
		target = s.eventDB
	}
	_, err := target.Exec(		`INSERT INTO events (track_id, release, event, detail, ts)
		 VALUES (?, ?, ?, ?, ?)`,
		trackID, s.release, event, detail, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// RecordPage writes a PAGE event to the events table so the Coach can see
// escalations. detail should be "max_turns" or "circuit_breaker". This is
// a best-effort write — failure does not abort the run (AC4 pattern).
func RecordPage(db *sql.DB, release, sliceID, detail string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(
		`INSERT INTO events (track_id, release, event, detail, ts)
		 VALUES (?, ?, 'page', ?, ?)`,
		sliceID, release, detail, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// pidAlive returns true if pid corresponds to a live process.
// Uses syscall.Kill(pid, 0) which is the POSIX-specified way to check
// process existence without sending a signal.
func pidAlive(pid int) bool {	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}

// Open opens (or creates) the SQLite database for the supervisor event store
// at .sworn/supervisor-<release>.db under workspaceRoot. It applies schema
// migrations (events table) and enables WAL mode. Returns the database handle.
func Open(release, workspaceRoot string) (*sql.DB, error) {
	dbPath := filepath.Join(workspaceRoot, ".sworn", "supervisor-"+release+".db")
	return db.Open(dbPath)
}

// Event is a single row from the events table.
type Event struct {
	ID      int64  `json:"id"`
	TrackID string `json:"track_id"`
	Release string `json:"release"`
	Event   string `json:"event"`
	Detail  string `json:"detail"`
	TS      string `json:"ts"`
}

// QueryEvents returns all events for the given release from the database.
func QueryEvents(database *sql.DB, release string) ([]Event, error) {
	rows, err := database.Query(
		`SELECT id, track_id, release, event, detail, ts FROM events WHERE release = ? ORDER BY id`,
		release,
	)
	if err != nil {
		return nil, fmt.Errorf("supervisor: query events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TrackID, &e.Release, &e.Event, &e.Detail, &e.TS); err != nil {
			return events, fmt.Errorf("supervisor: scan event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return events, fmt.Errorf("supervisor: iterate events: %w", err)
	}
	return events, nil
}