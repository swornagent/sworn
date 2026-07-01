package supervisor

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// setupDecisionsDB creates an in-memory SQLite DB with the decisions table
// and returns the handle.
func setupDecisionsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	db.SetMaxOpenConns(1)

	// Create decisions table (the schema that db.Open would normally apply).
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS decisions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		slice_id    TEXT NOT NULL,
		release     TEXT NOT NULL,
		role        TEXT NOT NULL,
		action      TEXT NOT NULL,
		reason      TEXT NOT NULL DEFAULT '',
		recorded_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create decisions table: %v", err)
	}
	return db
}

func TestRecordDecision_WritesRow(t *testing.T) {
	db := setupDecisionsDB(t)
	defer db.Close()

	err := RecordDecision(db, "release-1", "S02-test", "implement", "router chose implement")
	if err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	// Verify the row was written.
	var (
		id         int
		sliceID    string
		release    string
		role       string
		action     string
		reason     string
		recordedAt string
	)
	err = db.QueryRow(
		`SELECT id, slice_id, release, role, action, reason, recorded_at
		 FROM decisions WHERE slice_id = 'S02-test'`,
	).Scan(&id, &sliceID, &release, &role, &action, &reason, &recordedAt)
	if err != nil {
		t.Fatalf("query decision: %v", err)
	}
	if sliceID != "S02-test" {
		t.Errorf("slice_id = %q, want %q", sliceID, "S02-test")
	}
	if release != "release-1" {
		t.Errorf("release = %q, want %q", release, "release-1")
	}
	if role != "router" {
		t.Errorf("role = %q, want %q", role, "router")
	}
	if action != "implement" {
		t.Errorf("action = %q, want %q", action, "implement")
	}
	if reason != "router chose implement" {
		t.Errorf("reason = %q, want %q", reason, "router chose implement")
	}
	if recordedAt == "" {
		t.Error("recorded_at is empty")
	}
}

func TestRecordTriage_WritesRow(t *testing.T) {
	db := setupDecisionsDB(t)
	defer db.Close()

	err := RecordTriage(db, "release-2", "S03-test", "resolve_in_place", "retry same model")
	if err != nil {
		t.Fatalf("RecordTriage: %v", err)
	}

	var role, action, reason string
	err = db.QueryRow(
		`SELECT role, action, reason FROM decisions WHERE slice_id = 'S03-test'`,
	).Scan(&role, &action, &reason)
	if err != nil {
		t.Fatalf("query triage: %v", err)
	}
	if role != "triage" {
		t.Errorf("role = %q, want %q", role, "triage")
	}
	if action != "resolve_in_place" {
		t.Errorf("action = %q, want %q", action, "resolve_in_place")
	}
	if reason != "retry same model" {
		t.Errorf("reason = %q, want %q", reason, "retry same model")
	}
}

func TestQueryDecisions_ReturnsInInsertOrder(t *testing.T) {
	db := setupDecisionsDB(t)
	defer db.Close()

	// Insert out of order to prove ordering is by insertion (id ASC).
	_ = RecordDecision(db, "rel", "S01", "verify", "first")
	_ = RecordTriage(db, "rel", "S01", "halt", "halted")
	_ = RecordDecision(db, "rel", "S02", "implement", "second")

	rows, err := QueryDecisions(db, "rel")
	if err != nil {
		t.Fatalf("QueryDecisions: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0].Action != "verify" {
		t.Errorf("row 0 action = %q, want %q", rows[0].Action, "verify")
	}
	if rows[1].Action != "halt" {
		t.Errorf("row 1 action = %q, want %q", rows[1].Action, "halt")
	}
	if rows[2].Action != "implement" {
		t.Errorf("row 2 action = %q, want %q", rows[2].Action, "implement")
	}
}

func TestQueryDecisions_FiltersByRelease(t *testing.T) {
	db := setupDecisionsDB(t)
	defer db.Close()

	_ = RecordDecision(db, "release-A", "S01", "implement", "a")
	_ = RecordDecision(db, "release-B", "S01", "verify", "b")

	rowsA, _ := QueryDecisions(db, "release-A")
	if len(rowsA) != 1 {
		t.Fatalf("release-A: got %d rows, want 1", len(rowsA))
	}
	if rowsA[0].Action != "implement" {
		t.Errorf("release-A action = %q", rowsA[0].Action)
	}

	rowsB, _ := QueryDecisions(db, "release-B")
	if len(rowsB) != 1 {
		t.Fatalf("release-B: got %d rows, want 1", len(rowsB))
	}
	if rowsB[0].Action != "verify" {
		t.Errorf("release-B action = %q", rowsB[0].Action)
	}
}

func TestRecordDecision_DoesNotAbortOnError(t *testing.T) {
	// AC4: decision-log failure must not abort the run.
	// The caller pattern is: _ = supervisor.RecordDecision(...)
	// i.e. the error is discarded and the run continues.
	// Test that a DB write failure (closed handle) returns an error
	// that the caller can safely discard.
	db := setupDecisionsDB(t)
	db.Close()

	err := RecordDecision(db, "rel", "S01", "implement", "test")
	if err == nil {
		t.Error("expected error for closed DB")
	}
	// The caller discards: _ = err -- this is AC4.
}