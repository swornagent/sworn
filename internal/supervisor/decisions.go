package supervisor

import (
	"database/sql"
	"fmt"
	"time"
)

// DecisionRow is a single row from the decisions table.
type DecisionRow struct {
	ID         int
	SliceID    string
	Release    string
	Role       string
	Action     string
	Reason     string
	RecordedAt string
}

// RecordDecision writes a routing decision to the decisions table.
// role should be "router"; action and reason come from the SliceDecision.
// Failure to write is logged but does not abort the run (AC4: decision-log
// failure must not abort the run).
func RecordDecision(db *sql.DB, release, sliceID, action, reason string) error {
	_, err := db.Exec(
		`INSERT INTO decisions (slice_id, release, role, action, reason, recorded_at)
		 VALUES (?, ?, 'router', ?, ?, ?)`,
		sliceID, release, action, reason, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("supervisor: record decision for %s: %w", sliceID, err)
	}
	return nil
}

// RecordTriage writes a triage output to the decisions table.
// role is "triage"; action and reason come from the triage.Output.
// Failure to write is logged but does not abort the run (AC4).
func RecordTriage(db *sql.DB, release, sliceID, action, reason string) error {
	_, err := db.Exec(
		`INSERT INTO decisions (slice_id, release, role, action, reason, recorded_at)
		 VALUES (?, ?, 'triage', ?, ?, ?)`,
		sliceID, release, action, reason, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("supervisor: record triage for %s: %w", sliceID, err)
	}
	return nil
}

// QueryDecisions returns all decision rows for a release, ordered by
// insertion order (ASC by id). Used by the telemetry subcommand.
func QueryDecisions(db *sql.DB, release string) ([]DecisionRow, error) {
	rows, err := db.Query(
		`SELECT id, slice_id, release, role, action, reason, recorded_at
		 FROM decisions
		 WHERE release = ?
		 ORDER BY id ASC`, release,
	)
	if err != nil {
		return nil, fmt.Errorf("supervisor: query decisions: %w", err)
	}
	defer rows.Close()

	var out []DecisionRow
	for rows.Next() {
		var r DecisionRow
		if err := rows.Scan(&r.ID, &r.SliceID, &r.Release, &r.Role, &r.Action, &r.Reason, &r.RecordedAt); err != nil {
			return out, fmt.Errorf("supervisor: scan decision row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}