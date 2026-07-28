package journal

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"
)

const (
	EvalSchemaVersion    = "sworn.eval/v1"
	MaxObserverItems     = 256
	MaxObserverBodyBytes = 64 * 1024
)

type EvalRecord struct {
	RunID             string
	Observer          string
	SourceEventOffset int64
	ID                string
	SchemaVersion     string
	BodyDigest        string
	Body              []byte
	CreatedAt         time.Time
}

type EvalDraft struct {
	SourceEventOffset int64
	ID                string
	Body              []byte
}

type NotificationDraft struct {
	DestinationID     string
	SourceEventOffset int64
	MessageID         string
	Body              []byte
}

type ObserverAdvance struct {
	RunID          string
	Observer       string
	ExpectedOffset int64
	ThroughOffset  int64
	Eval           []EvalDraft
	Notifications  []NotificationDraft
	At             time.Time
}

type observerBatchIdentity struct {
	RunID          string                      `json:"run_id"`
	Observer       string                      `json:"observer"`
	ExpectedOffset int64                       `json:"expected_offset"`
	ThroughOffset  int64                       `json:"through_offset"`
	Eval           []observerBatchEval         `json:"eval"`
	Notifications  []observerBatchNotification `json:"notifications"`
}

type observerBatchEval struct {
	SourceEventOffset int64  `json:"source_event_offset"`
	ID                string `json:"id"`
	BodyDigest        string `json:"body_digest"`
}

type observerBatchNotification struct {
	DestinationID     string `json:"destination_id"`
	SourceEventOffset int64  `json:"source_event_offset"`
	MessageID         string `json:"message_id"`
	BodyDigest        string `json:"body_digest"`
}

func observerBatchDigest(value ObserverAdvance) string {
	identity := observerBatchIdentity{
		RunID:          value.RunID,
		Observer:       value.Observer,
		ExpectedOffset: value.ExpectedOffset,
		ThroughOffset:  value.ThroughOffset,
		Eval:           make([]observerBatchEval, 0, len(value.Eval)),
		Notifications:  make([]observerBatchNotification, 0, len(value.Notifications)),
	}
	for _, record := range value.Eval {
		identity.Eval = append(identity.Eval, observerBatchEval{
			SourceEventOffset: record.SourceEventOffset,
			ID:                record.ID,
			BodyDigest:        digest(record.Body),
		})
	}
	for _, notification := range value.Notifications {
		identity.Notifications = append(
			identity.Notifications,
			observerBatchNotification{
				DestinationID:     notification.DestinationID,
				SourceEventOffset: notification.SourceEventOffset,
				MessageID:         notification.MessageID,
				BodyDigest:        digest(notification.Body),
			},
		)
	}
	body, _ := json.Marshal(identity)
	return digest(body)
}

func canonicalObserverAdvance(value ObserverAdvance) ObserverAdvance {
	value.Eval = append([]EvalDraft(nil), value.Eval...)
	sort.Slice(value.Eval, func(left, right int) bool {
		if value.Eval[left].SourceEventOffset !=
			value.Eval[right].SourceEventOffset {
			return value.Eval[left].SourceEventOffset <
				value.Eval[right].SourceEventOffset
		}
		return value.Eval[left].ID < value.Eval[right].ID
	})
	value.Notifications = append(
		[]NotificationDraft(nil),
		value.Notifications...,
	)
	sort.Slice(value.Notifications, func(left, right int) bool {
		if value.Notifications[left].DestinationID !=
			value.Notifications[right].DestinationID {
			return value.Notifications[left].DestinationID <
				value.Notifications[right].DestinationID
		}
		if value.Notifications[left].SourceEventOffset !=
			value.Notifications[right].SourceEventOffset {
			return value.Notifications[left].SourceEventOffset <
				value.Notifications[right].SourceEventOffset
		}
		return value.Notifications[left].MessageID <
			value.Notifications[right].MessageID
	})
	return value
}

func validateObserverAdvance(value ObserverAdvance) error {
	if err := validateIdentity(value.RunID, "run"); err != nil {
		return err
	}
	if err := validateIdentity(value.Observer, "observer"); err != nil {
		return err
	}
	if value.ExpectedOffset < 0 || value.ThroughOffset <= value.ExpectedOffset ||
		len(value.Eval)+len(value.Notifications) > MaxObserverItems {
		return fail("INVALID_OBSERVER_ADVANCE", nil)
	}
	if _, err := canonicalTime(value.At); err != nil {
		return err
	}
	evalOffsets := make(map[int64]struct{}, len(value.Eval))
	recordIDs := make(map[string]struct{}, len(value.Eval))
	for _, record := range value.Eval {
		if record.SourceEventOffset <= value.ExpectedOffset ||
			record.SourceEventOffset > value.ThroughOffset ||
			len(record.Body) == 0 || len(record.Body) > MaxObserverBodyBytes {
			return fail("INVALID_EVAL_RECORD", nil)
		}
		if err := validateIdentity(record.ID, "eval_record"); err != nil {
			return err
		}
		if _, found := evalOffsets[record.SourceEventOffset]; found {
			return fail("INVALID_EVAL_RECORD", nil)
		}
		if _, found := recordIDs[record.ID]; found {
			return fail("INVALID_EVAL_RECORD", nil)
		}
		evalOffsets[record.SourceEventOffset] = struct{}{}
		recordIDs[record.ID] = struct{}{}
	}
	notificationKeys := make(map[string]struct{}, len(value.Notifications))
	messageIDs := make(map[string]struct{}, len(value.Notifications))
	for _, notification := range value.Notifications {
		if notification.SourceEventOffset <= value.ExpectedOffset ||
			notification.SourceEventOffset > value.ThroughOffset ||
			len(notification.Body) == 0 ||
			len(notification.Body) > MaxObserverBodyBytes {
			return fail("INVALID_NOTIFICATION", nil)
		}
		if err := validateIdentity(notification.DestinationID, "destination"); err != nil {
			return err
		}
		if err := validateIdentity(notification.MessageID, "message"); err != nil {
			return err
		}
		key := notification.DestinationID + "\x00" +
			strconv.FormatInt(notification.SourceEventOffset, 10)
		if _, found := notificationKeys[key]; found {
			return fail("INVALID_NOTIFICATION", nil)
		}
		if _, found := messageIDs[notification.MessageID]; found {
			return fail("INVALID_NOTIFICATION", nil)
		}
		notificationKeys[key] = struct{}{}
		messageIDs[notification.MessageID] = struct{}{}
	}
	return nil
}

func observerCursorOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	observer, runID string,
) (int64, string, error) {
	var offset int64
	var batchDigest string
	err := conn.QueryRowContext(
		ctx,
		`SELECT event_offset, batch_digest
		 FROM observer_cursors WHERE observer = ? AND run_id = ?`,
		observer,
		runID,
	).Scan(&offset, &batchDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", dbError(err)
	}
	if err := validateDigest(batchDigest); err != nil {
		return 0, "", fail("CORRUPT_JOURNAL", nil)
	}
	return offset, batchDigest, nil
}

func requireSourceEvent(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
	offset int64,
) error {
	var found int
	if err := conn.QueryRowContext(
		ctx,
		"SELECT count(*) FROM events WHERE run_id = ? AND event_offset = ?",
		runID,
		offset,
	).Scan(&found); err != nil {
		return dbError(err)
	}
	if found != 1 {
		return fail("EVENT_NOT_FOUND", nil)
	}
	return nil
}

func verifyObserverReplay(
	ctx context.Context,
	conn *sql.Conn,
	value ObserverAdvance,
) error {
	for _, expected := range value.Eval {
		var recordID, schemaVersion, bodyDigest string
		var body []byte
		err := conn.QueryRowContext(
			ctx,
			`SELECT record_id, schema_version, body_digest, body
			 FROM eval_records
			 WHERE run_id = ? AND observer = ? AND source_event_offset = ?`,
			value.RunID,
			value.Observer,
			expected.SourceEventOffset,
		).Scan(&recordID, &schemaVersion, &bodyDigest, &body)
		if err != nil || recordID != expected.ID ||
			schemaVersion != EvalSchemaVersion ||
			bodyDigest != digest(expected.Body) ||
			!bytes.Equal(body, expected.Body) {
			return fail("REPLAY_CONFLICT", nil)
		}
	}
	for _, expected := range value.Notifications {
		var messageID, schemaVersion, bodyDigest string
		var body []byte
		err := conn.QueryRowContext(
			ctx,
			`SELECT message_id, schema_version, body_digest, body
			 FROM notification_outbox
			 WHERE run_id = ? AND destination_id = ? AND source_event_offset = ?`,
			value.RunID,
			expected.DestinationID,
			expected.SourceEventOffset,
		).Scan(&messageID, &schemaVersion, &bodyDigest, &body)
		if err != nil || messageID != expected.MessageID ||
			schemaVersion != NotificationSchemaVersion ||
			bodyDigest != digest(expected.Body) ||
			!bytes.Equal(body, expected.Body) {
			return fail("REPLAY_CONFLICT", nil)
		}
	}
	return nil
}

// AdvanceObserver atomically persists all derived facts and their source
// cursor. Replaying the exact already-committed batch is idempotent.
func (s *Store) AdvanceObserver(ctx context.Context, value ObserverAdvance) error {
	if err := validateObserverAdvance(value); err != nil {
		return err
	}
	value = canonicalObserverAdvance(value)
	at, _ := canonicalTime(value.At)
	batchDigest := observerBatchDigest(value)
	return s.immediate(ctx, func(conn *sql.Conn) error {
		cursor, observedBatchDigest, err := observerCursorOnConnection(
			ctx,
			conn,
			value.Observer,
			value.RunID,
		)
		if err != nil {
			return err
		}
		if cursor == value.ThroughOffset {
			if observedBatchDigest != batchDigest {
				return fail("REPLAY_CONFLICT", nil)
			}
			return verifyObserverReplay(ctx, conn, value)
		}
		if cursor != value.ExpectedOffset {
			return fail("STALE_OBSERVER_CURSOR", nil)
		}
		// Even a batch that emits no derived facts must bind its cursor to one
		// event that is durable for this run.
		seenOffsets := map[int64]struct{}{value.ThroughOffset: {}}
		for _, record := range value.Eval {
			seenOffsets[record.SourceEventOffset] = struct{}{}
		}
		for _, notification := range value.Notifications {
			seenOffsets[notification.SourceEventOffset] = struct{}{}
		}
		for offset := range seenOffsets {
			if err := requireSourceEvent(ctx, conn, value.RunID, offset); err != nil {
				return err
			}
		}
		for _, record := range value.Eval {
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO eval_records(
				     run_id, observer, source_event_offset, record_id,
				     schema_version, body_digest, body, created_at
				 ) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
				value.RunID,
				value.Observer,
				record.SourceEventOffset,
				record.ID,
				EvalSchemaVersion,
				digest(record.Body),
				append([]byte(nil), record.Body...),
				at,
			); err != nil {
				return dbError(err)
			}
		}
		sequences := make(map[string]int64)
		for _, notification := range value.Notifications {
			sequence, found := sequences[notification.DestinationID]
			if !found {
				if err := conn.QueryRowContext(
					ctx,
					`SELECT COALESCE(max(sequence), 0)
					 FROM notification_outbox
					 WHERE run_id = ? AND destination_id = ?`,
					value.RunID,
					notification.DestinationID,
				).Scan(&sequence); err != nil {
					return dbError(err)
				}
			}
			sequence++
			sequences[notification.DestinationID] = sequence
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO notification_outbox(
				     run_id, destination_id, source_event_offset, sequence,
				     message_id, schema_version, body_digest, body, state,
				     attempts, available_at, created_at, updated_at
				 ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, ?, ?, ?)`,
				value.RunID,
				notification.DestinationID,
				notification.SourceEventOffset,
				sequence,
				notification.MessageID,
				NotificationSchemaVersion,
				digest(notification.Body),
				append([]byte(nil), notification.Body...),
				at,
				at,
				at,
			); err != nil {
				return dbError(err)
			}
		}
		result, err := conn.ExecContext(
			ctx,
			`INSERT INTO observer_cursors(
			     observer, run_id, event_offset, batch_digest, updated_at
			 ) VALUES(?, ?, ?, ?, ?)
			 ON CONFLICT(observer, run_id) DO UPDATE SET
			     event_offset=excluded.event_offset,
			     batch_digest=excluded.batch_digest,
			     updated_at=excluded.updated_at
			 WHERE observer_cursors.event_offset = ?`,
			value.Observer,
			value.RunID,
			value.ThroughOffset,
			batchDigest,
			at,
			value.ExpectedOffset,
		)
		if err != nil {
			return dbError(err)
		}
		return requireRows(result, "STALE_OBSERVER_CURSOR")
	})
}

func (s *Store) ObserverCursor(
	ctx context.Context,
	runID, observer string,
) (int64, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return 0, err
	}
	if err := validateIdentity(observer, "observer"); err != nil {
		return 0, err
	}
	var result int64
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		if _, err = runOnConnection(ctx, conn, runID); err != nil {
			return err
		}
		result, _, err = observerCursorOnConnection(ctx, conn, observer, runID)
		return err
	})
	return result, err
}

func (s *Store) EvalRecords(
	ctx context.Context,
	runID, observer string,
	afterOffset int64,
	limit int,
) ([]EvalRecord, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	if err := validateIdentity(observer, "observer"); err != nil {
		return nil, err
	}
	if afterOffset < 0 || limit < 1 || limit > MaxObserverItems {
		return nil, fail("INVALID_EVAL_WINDOW", nil)
	}
	var result []EvalRecord
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		if _, err := runOnConnection(ctx, conn, runID); err != nil {
			return err
		}
		rows, err := conn.QueryContext(
			ctx,
			`SELECT observer, source_event_offset, record_id, schema_version,
			        body_digest, body, created_at
			 FROM eval_records
			 WHERE run_id = ? AND observer = ?
			   AND source_event_offset > ?
			 ORDER BY source_event_offset
			 LIMIT ?`,
			runID,
			observer,
			afterOffset,
			limit,
		)
		if err != nil {
			return dbError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var item EvalRecord
			var createdAt string
			item.RunID = runID
			if err := rows.Scan(
				&item.Observer,
				&item.SourceEventOffset,
				&item.ID,
				&item.SchemaVersion,
				&item.BodyDigest,
				&item.Body,
				&createdAt,
			); err != nil {
				return dbError(err)
			}
			item.CreatedAt, err = parseTime(createdAt)
			if err != nil || item.SchemaVersion != EvalSchemaVersion ||
				item.BodyDigest != digest(item.Body) {
				return fail("CORRUPT_JOURNAL", nil)
			}
			item.Body = append([]byte(nil), item.Body...)
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			return dbError(err)
		}
		return nil
	})
	return result, err
}
