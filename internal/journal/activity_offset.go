package journal

import (
	"context"
	"database/sql"
	"time"
)

// AppendEventWithOffset is the S2-live-worker-stream cursor discipline:
// it journals one event exactly like AppendEvent and returns the durable
// event_offset assigned by SQLite. The runtime observation hooks feed the
// in-memory activity ring only after this durable append succeeds and carry
// this exact offset, so the ring and the journal share one cursor space and
// the activity route resumes from Last-Event-ID with no gap and no
// duplicate. See internal/cockpit/activity.go for the merge discipline.
func (s *Store) AppendEventWithOffset(
	ctx context.Context,
	runID, kind string,
	body []byte,
	at time.Time,
) (int64, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return 0, err
	}
	if err := validateIdentity(kind, "event_kind"); err != nil {
		return 0, err
	}
	if len(body) > MaxEventBytes {
		return 0, fail("RESOURCE_LIMIT", nil)
	}
	timestamp, err := canonicalTime(at)
	if err != nil {
		return 0, err
	}
	var offset int64
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		var innerErr error
		offset, innerErr = appendEventWithOffset(ctx, conn, runID, kind, body, timestamp)
		return innerErr
	})
	if err != nil {
		return 0, err
	}
	return offset, nil
}

func appendEventWithOffset(
	ctx context.Context,
	conn *sql.Conn,
	runID, kind string,
	body []byte,
	at string,
) (int64, error) {
	if len(body) > MaxEventBytes {
		return 0, fail("RESOURCE_LIMIT", nil)
	}
	_, err := conn.ExecContext(
		ctx,
		`INSERT INTO events(run_id, kind, body_digest, body, created_at)
		 VALUES(?, ?, ?, ?, ?)`,
		runID,
		kind,
		digest(body),
		append([]byte{}, body...),
		at,
	)
	if err != nil {
		return 0, dbError(err)
	}
	var offset int64
	if err := conn.QueryRowContext(ctx, "SELECT last_insert_rowid()").Scan(&offset); err != nil {
		return 0, dbError(err)
	}
	if offset <= 0 {
		return 0, fail("DATABASE_FAILED", nil)
	}
	return offset, nil
}
