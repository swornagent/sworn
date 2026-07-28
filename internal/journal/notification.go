package journal

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	NotificationSchemaVersion = "sworn.notification/v1"
	MaxNotificationAttempts   = 3
)

type NotificationState string

const (
	NotificationPending   NotificationState = "pending"
	NotificationClaimed   NotificationState = "claimed"
	NotificationDelivered NotificationState = "delivered"
	NotificationDead      NotificationState = "dead"
)

type Notification struct {
	RunID             string
	DestinationID     string
	SourceEventOffset int64
	Sequence          int64
	MessageID         string
	SchemaVersion     string
	BodyDigest        string
	Body              []byte
	State             NotificationState
	Attempts          int64
	AvailableAt       time.Time
	ClaimToken        string
	ClaimedUntil      time.Time
	DeliveredAt       time.Time
	LastErrorCode     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type NotificationClaim struct {
	Notification Notification
	Token        string
	ExpiresAt    time.Time
}

type NotificationDisposition string

const (
	NotificationSucceeded NotificationDisposition = "delivered"
	NotificationRetry     NotificationDisposition = "retry"
	NotificationAbandon   NotificationDisposition = "dead"
)

func scanNotification(
	row interface{ Scan(...any) error },
	runID, destinationID string,
) (Notification, error) {
	var result Notification
	var availableAt, claimedUntil, deliveredAt, createdAt, updatedAt sql.NullString
	result.RunID, result.DestinationID = runID, destinationID
	err := row.Scan(
		&result.SourceEventOffset,
		&result.Sequence,
		&result.MessageID,
		&result.SchemaVersion,
		&result.BodyDigest,
		&result.Body,
		&result.State,
		&result.Attempts,
		&availableAt,
		&result.ClaimToken,
		&claimedUntil,
		&deliveredAt,
		&result.LastErrorCode,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return Notification{}, err
	}
	result.AvailableAt, err = parseTime(availableAt.String)
	if err == nil && claimedUntil.Valid {
		result.ClaimedUntil, err = parseTime(claimedUntil.String)
	}
	if err == nil && deliveredAt.Valid {
		result.DeliveredAt, err = parseTime(deliveredAt.String)
	}
	if err == nil {
		result.CreatedAt, err = parseTime(createdAt.String)
	}
	if err == nil {
		result.UpdatedAt, err = parseTime(updatedAt.String)
	}
	if err != nil || result.SchemaVersion != NotificationSchemaVersion ||
		result.BodyDigest != digest(result.Body) {
		return Notification{}, fail("CORRUPT_JOURNAL", nil)
	}
	switch result.State {
	case NotificationPending:
		if result.ClaimToken != "" || !result.ClaimedUntil.IsZero() ||
			!result.DeliveredAt.IsZero() ||
			result.Attempts >= MaxNotificationAttempts {
			return Notification{}, fail("CORRUPT_JOURNAL", nil)
		}
	case NotificationClaimed:
		if len(result.ClaimToken) != 64 || result.ClaimedUntil.IsZero() ||
			!result.DeliveredAt.IsZero() || result.Attempts < 1 ||
			result.Attempts > MaxNotificationAttempts {
			return Notification{}, fail("CORRUPT_JOURNAL", nil)
		}
	case NotificationDelivered:
		if result.ClaimToken != "" || !result.ClaimedUntil.IsZero() ||
			result.DeliveredAt.IsZero() || result.Attempts < 1 ||
			result.Attempts > MaxNotificationAttempts {
			return Notification{}, fail("CORRUPT_JOURNAL", nil)
		}
	case NotificationDead:
		if result.ClaimToken != "" || !result.ClaimedUntil.IsZero() ||
			!result.DeliveredAt.IsZero() || result.Attempts < 1 ||
			result.Attempts > MaxNotificationAttempts {
			return Notification{}, fail("CORRUPT_JOURNAL", nil)
		}
	default:
		return Notification{}, fail("CORRUPT_JOURNAL", nil)
	}
	result.Body = append([]byte(nil), result.Body...)
	return result, nil
}

func (s *Store) ClaimNotification(
	ctx context.Context,
	runID, destinationID string,
	now time.Time,
	lease time.Duration,
) (NotificationClaim, bool, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return NotificationClaim{}, false, err
	}
	if err := validateIdentity(destinationID, "destination"); err != nil {
		return NotificationClaim{}, false, err
	}
	if lease <= 0 || lease > MaxLease {
		return NotificationClaim{}, false, fail("INVALID_LEASE", nil)
	}
	at, err := canonicalTime(now)
	if err != nil {
		return NotificationClaim{}, false, err
	}
	expires, _ := canonicalTime(now.Add(lease))
	token, err := randomToken()
	if err != nil {
		return NotificationClaim{}, false, err
	}
	var result NotificationClaim
	found := false
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		row := conn.QueryRowContext(
			ctx,
			`SELECT source_event_offset, sequence, message_id, schema_version,
			        body_digest, body, state, attempts, available_at,
			        COALESCE(claim_token,''), claimed_until, delivered_at,
			        COALESCE(last_error_code,''), created_at, updated_at
			 FROM notification_outbox
			 WHERE run_id = ? AND destination_id = ?
			   AND state IN ('pending','claimed')
			 ORDER BY sequence
			 LIMIT 1`,
			runID,
			destinationID,
		)
		item, scanErr := scanNotification(row, runID, destinationID)
		if errors.Is(scanErr, sql.ErrNoRows) {
			return nil
		}
		if scanErr != nil {
			return scanErr
		}
		if item.State == NotificationClaimed && item.ClaimedUntil.After(now) {
			return nil
		}
		if item.State == NotificationPending && item.AvailableAt.After(now) {
			return nil
		}
		if item.Attempts >= MaxNotificationAttempts {
			if _, err := conn.ExecContext(
				ctx,
				`UPDATE notification_outbox SET
				     state='dead', claim_token=NULL, claimed_until=NULL,
				     last_error_code='DELIVERY_LEASE_EXPIRED', updated_at=?
				 WHERE run_id=? AND destination_id=? AND sequence=?
				   AND state IN ('pending','claimed')`,
				at,
				runID,
				destinationID,
				item.Sequence,
			); err != nil {
				return dbError(err)
			}
			return nil
		}
		update, err := conn.ExecContext(
			ctx,
			`UPDATE notification_outbox SET
			     state='claimed', attempts=attempts+1, claim_token=?,
			     claimed_until=?, last_error_code=NULL, updated_at=?
			 WHERE run_id=? AND destination_id=? AND sequence=?
			   AND state=? AND attempts=?`,
			token,
			expires,
			at,
			runID,
			destinationID,
			item.Sequence,
			item.State,
			item.Attempts,
		)
		if err != nil {
			return dbError(err)
		}
		if err := requireRows(update, "NOTIFICATION_NOT_CLAIMABLE"); err != nil {
			return err
		}
		item.State = NotificationClaimed
		item.Attempts++
		item.ClaimToken = token
		item.ClaimedUntil = now.Add(lease).UTC()
		item.UpdatedAt = now.UTC()
		result = NotificationClaim{
			Notification: item,
			Token:        token,
			ExpiresAt:    item.ClaimedUntil,
		}
		found = true
		return nil
	})
	return result, found, err
}

func (s *Store) FinishNotification(
	ctx context.Context,
	claim NotificationClaim,
	disposition NotificationDisposition,
	retryAt time.Time,
	errorCode string,
	at time.Time,
) error {
	item := claim.Notification
	if err := validateIdentity(item.RunID, "run"); err != nil {
		return err
	}
	if err := validateIdentity(item.DestinationID, "destination"); err != nil {
		return err
	}
	if item.Sequence < 1 || len(claim.Token) != 64 ||
		claim.Token != item.ClaimToken {
		return fail("INVALID_NOTIFICATION_CLAIM", nil)
	}
	timestamp, err := canonicalTime(at)
	if err != nil {
		return err
	}
	state := NotificationDelivered
	available := timestamp
	delivered := any(timestamp)
	switch disposition {
	case NotificationSucceeded:
		if errorCode != "" || !retryAt.IsZero() {
			return fail("INVALID_NOTIFICATION_RESULT", nil)
		}
	case NotificationRetry:
		if err := validateIdentity(errorCode, "error_code"); err != nil {
			return err
		}
		if retryAt.Before(at) {
			return fail("INVALID_NOTIFICATION_RESULT", nil)
		}
		if item.Attempts >= MaxNotificationAttempts {
			state = NotificationDead
		} else {
			state = NotificationPending
			available, _ = canonicalTime(retryAt)
		}
		delivered = nil
	case NotificationAbandon:
		if err := validateIdentity(errorCode, "error_code"); err != nil {
			return err
		}
		state = NotificationDead
		delivered = nil
	default:
		return fail("INVALID_NOTIFICATION_RESULT", nil)
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		result, err := conn.ExecContext(
			ctx,
			`UPDATE notification_outbox SET
			     state=?, available_at=?, claim_token=NULL, claimed_until=NULL,
			     delivered_at=?, last_error_code=?, updated_at=?
			 WHERE run_id=? AND destination_id=? AND sequence=?
			   AND state='claimed' AND claim_token=? AND attempts=?`,
			state,
			available,
			delivered,
			nullableString(errorCode),
			timestamp,
			item.RunID,
			item.DestinationID,
			item.Sequence,
			claim.Token,
			item.Attempts,
		)
		if err != nil {
			return dbError(err)
		}
		return requireRows(result, "STALE_NOTIFICATION_CLAIM")
	})
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) RedeliverNotification(
	ctx context.Context,
	runID, destinationID, messageID string,
	at time.Time,
) error {
	for value, label := range map[string]string{
		runID: "run", destinationID: "destination", messageID: "message",
	} {
		if err := validateIdentity(value, label); err != nil {
			return err
		}
	}
	timestamp, err := canonicalTime(at)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		result, err := conn.ExecContext(
			ctx,
			`UPDATE notification_outbox SET
			     state='pending', attempts=0, available_at=?,
			     claim_token=NULL, claimed_until=NULL, delivered_at=NULL,
			     last_error_code=NULL, updated_at=?
			 WHERE run_id=? AND destination_id=? AND message_id=? AND state='dead'`,
			timestamp,
			timestamp,
			runID,
			destinationID,
			messageID,
		)
		if err != nil {
			return dbError(err)
		}
		return requireRows(result, "NOTIFICATION_NOT_REDELIVERABLE")
	})
}

func (s *Store) Notifications(
	ctx context.Context,
	runID, destinationID string,
	limit int,
) ([]Notification, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	if err := validateIdentity(destinationID, "destination"); err != nil {
		return nil, err
	}
	if limit < 1 || limit > MaxObserverItems {
		return nil, fail("INVALID_NOTIFICATION_WINDOW", nil)
	}
	var result []Notification
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		rows, err := conn.QueryContext(
			ctx,
			`SELECT source_event_offset, sequence, message_id, schema_version,
			        body_digest, body, state, attempts, available_at,
			        COALESCE(claim_token,''), claimed_until, delivered_at,
			        COALESCE(last_error_code,''), created_at, updated_at
			 FROM notification_outbox
			 WHERE run_id=? AND destination_id=?
			 ORDER BY sequence
			 LIMIT ?`,
			runID,
			destinationID,
			limit,
		)
		if err != nil {
			return dbError(err)
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanNotification(rows, runID, destinationID)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			return dbError(err)
		}
		return nil
	})
	return result, err
}
