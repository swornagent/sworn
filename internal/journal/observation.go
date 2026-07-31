package journal

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

const (
	MaxObservationAttempts      = 256
	MaxObservationEvents        = 256
	MaxObservationNotifications = 256
	MaxObservationAttentions    = 64
)

// AttemptFact is the content-free part of one durable driver attempt.
type AttemptFact struct {
	EffectID       string
	Number         int64
	Responsibility string
	Transport      string
	Usage          []byte
	CreatedAt      time.Time
}

// EventFact deliberately excludes the event body and its digest. Operator
// projections can replay durable timing and state facts without exposing
// content that happened to accompany the engine event.
type EventFact struct {
	Offset    int64
	Kind      string
	CreatedAt time.Time
}

// NotificationFact exposes delivery state without exposing the signed body.
type NotificationFact struct {
	DestinationID     string
	SourceEventOffset int64
	Sequence          int64
	MessageID         string
	State             NotificationState
	Attempts          int64
	AvailableAt       time.Time
	ClaimedUntil      time.Time
	DeliveredAt       time.Time
	LastErrorCode     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Observation struct {
	Run                    Run
	Control                ControlProjection
	Owner                  OwnerLease
	OwnerPresent           bool
	Attempts               []AttemptFact
	Attentions             []AttentionProjection
	AttentionsTruncated    bool
	Events                 []EventFact
	Notifications          []NotificationFact
	NotificationsTruncated bool
	EventOffset            int64
	HasPrior               bool
}

type EventWindow struct {
	Events      []EventFact
	Through     int64
	EventOffset int64
	HasMore     bool
}

func runOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
) (Run, error) {
	var result Run
	var createdAt string
	err := conn.QueryRowContext(
		ctx,
		`SELECT run_id, manifest_digest, repository, release_id,
		        target_ref, created_at
		 FROM runs WHERE run_id=?`,
		runID,
	).Scan(
		&result.ID,
		&result.ManifestDigest,
		&result.Repository,
		&result.Release,
		&result.TargetRef,
		&createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, fail("RUN_NOT_FOUND", nil)
	}
	if err != nil {
		return Run{}, dbError(err)
	}
	result.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Run{}, err
	}
	return result, nil
}

func (s *Store) RunBinding(ctx context.Context, runID string) (Run, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return Run{}, err
	}
	var result Run
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		result, err = runOnConnection(ctx, conn, runID)
		return err
	})
	return result, err
}

func observationAttempts(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
	limit int,
) ([]AttemptFact, error) {
	rows, err := conn.QueryContext(
		ctx,
		`SELECT effect_id, attempt, responsibility, transport_status,
		        usage_digest, usage, created_at
		 FROM attempts
		 WHERE run_id=?
		 ORDER BY created_at DESC, effect_id DESC, attempt DESC
		 LIMIT ?`,
		runID,
		limit,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	reversed := make([]AttemptFact, 0, limit)
	for rows.Next() {
		var fact AttemptFact
		var usageDigest string
		var createdAt string
		if err := rows.Scan(
			&fact.EffectID,
			&fact.Number,
			&fact.Responsibility,
			&fact.Transport,
			&usageDigest,
			&fact.Usage,
			&createdAt,
		); err != nil {
			return nil, dbError(err)
		}
		if !digestPattern.MatchString(usageDigest) ||
			digest(fact.Usage) != usageDigest {
			return nil, fail("CORRUPT_JOURNAL", nil)
		}
		fact.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		fact.Usage = append([]byte(nil), fact.Usage...)
		reversed = append(reversed, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(err)
	}
	result := make([]AttemptFact, len(reversed))
	for index := range reversed {
		result[len(reversed)-1-index] = reversed[index]
	}
	return result, nil
}

func recentEventFacts(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
	limit int,
) ([]EventFact, error) {
	rows, err := conn.QueryContext(
		ctx,
		`SELECT event_offset, kind, created_at
		 FROM events
		 WHERE run_id=?
		 ORDER BY event_offset DESC
		 LIMIT ?`,
		runID,
		limit,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	reversed := make([]EventFact, 0, limit)
	for rows.Next() {
		var fact EventFact
		var createdAt string
		if err := rows.Scan(&fact.Offset, &fact.Kind, &createdAt); err != nil {
			return nil, dbError(err)
		}
		fact.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		reversed = append(reversed, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(err)
	}
	result := make([]EventFact, len(reversed))
	for index := range reversed {
		result[len(reversed)-1-index] = reversed[index]
	}
	return result, nil
}

func observationNotifications(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
) ([]NotificationFact, bool, error) {
	rows, err := conn.QueryContext(
		ctx,
		`SELECT destination_id, source_event_offset, sequence, message_id,
		        schema_version, body_digest, body, state, attempts,
		        available_at, COALESCE(claim_token,''), claimed_until,
		        delivered_at, COALESCE(last_error_code,''),
		        created_at, updated_at
		 FROM notification_outbox
		 WHERE run_id=?
		 ORDER BY updated_at DESC, destination_id, sequence DESC
		 LIMIT ?`,
		runID,
		MaxObservationNotifications+1,
	)
	if err != nil {
		return nil, false, dbError(err)
	}
	defer rows.Close()
	items := make([]NotificationFact, 0, MaxObservationNotifications+1)
	for rows.Next() {
		item := Notification{RunID: runID}
		var availableAt, claimedUntil, deliveredAt sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(
			&item.DestinationID,
			&item.SourceEventOffset,
			&item.Sequence,
			&item.MessageID,
			&item.SchemaVersion,
			&item.BodyDigest,
			&item.Body,
			&item.State,
			&item.Attempts,
			&availableAt,
			&item.ClaimToken,
			&claimedUntil,
			&deliveredAt,
			&item.LastErrorCode,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, false, dbError(err)
		}
		item, err = validateScannedNotification(
			item,
			availableAt,
			claimedUntil,
			deliveredAt,
			createdAt,
			updatedAt,
		)
		if err != nil {
			return nil, false, err
		}
		items = append(items, NotificationFact{
			DestinationID:     item.DestinationID,
			SourceEventOffset: item.SourceEventOffset,
			Sequence:          item.Sequence,
			MessageID:         item.MessageID,
			State:             item.State,
			Attempts:          item.Attempts,
			AvailableAt:       item.AvailableAt,
			ClaimedUntil:      item.ClaimedUntil,
			DeliveredAt:       item.DeliveredAt,
			LastErrorCode:     item.LastErrorCode,
			CreatedAt:         item.CreatedAt,
			UpdatedAt:         item.UpdatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, false, dbError(err)
	}
	truncated := len(items) > MaxObservationNotifications
	if truncated {
		items = items[:MaxObservationNotifications]
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].DestinationID != items[right].DestinationID {
			return items[left].DestinationID < items[right].DestinationID
		}
		return items[left].Sequence < items[right].Sequence
	})
	return items, truncated, nil
}

// ReadObservation returns the runtime facts needed by the cockpit from one
// SQLite snapshot. Bodies, receipts, commands, and model output are excluded.
func (s *Store) ReadObservation(
	ctx context.Context,
	runID string,
	attemptLimit, eventLimit int,
) (Observation, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return Observation{}, err
	}
	if attemptLimit < 1 || attemptLimit > MaxObservationAttempts ||
		eventLimit < 1 || eventLimit > MaxObservationEvents {
		return Observation{}, fail("INVALID_OBSERVATION_WINDOW", nil)
	}
	var result Observation
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		result.Run, err = runOnConnection(ctx, conn, runID)
		if err != nil {
			return err
		}
		result.EventOffset, err = eventHighWatermark(ctx, conn, runID)
		if err != nil {
			return err
		}
		result.Control, err = projectionOnConnection(ctx, conn, runID)
		if err != nil {
			return err
		}
		result.Owner, result.OwnerPresent, err =
			currentOwnerOnConnection(ctx, conn, runID)
		if err != nil {
			return err
		}
		result.Attempts, err = observationAttempts(
			ctx,
			conn,
			runID,
			attemptLimit,
		)
		if err != nil {
			return err
		}
		attentionMap, err := attentionProjectionsOnConnection(
			ctx,
			conn,
			runID,
			true,
		)
		if err != nil {
			return err
		}
		for _, attention := range attentionMap {
			if attention.State == AttentionOpen ||
				attention.State == AttentionAnswered {
				result.Attentions = append(
					result.Attentions,
					attention,
				)
			}
		}
		sort.Slice(result.Attentions, func(left, right int) bool {
			if result.Attentions[left].Attention.Recovery.LaneID !=
				result.Attentions[right].Attention.Recovery.LaneID {
				return result.Attentions[left].Attention.Recovery.LaneID <
					result.Attentions[right].Attention.Recovery.LaneID
			}
			return result.Attentions[left].Attention.ID <
				result.Attentions[right].Attention.ID
		})
		if len(result.Attentions) > MaxObservationAttentions {
			result.Attentions = result.Attentions[:MaxObservationAttentions]
			result.AttentionsTruncated = true
		}
		result.Events, err = recentEventFacts(ctx, conn, runID, eventLimit)
		if err != nil {
			return err
		}
		result.Notifications,
			result.NotificationsTruncated,
			err = observationNotifications(ctx, conn, runID)
		if err != nil {
			return err
		}
		var eventCount int64
		if err := conn.QueryRowContext(
			ctx,
			"SELECT count(*) FROM events WHERE run_id=?",
			runID,
		).Scan(&eventCount); err != nil {
			return dbError(err)
		}
		result.HasPrior = eventCount > int64(len(result.Events))
		return nil
	})
	return result, err
}

// EventsAfter returns a bounded, content-free replay page.
func (s *Store) EventsAfter(
	ctx context.Context,
	runID string,
	afterOffset int64,
	limit int,
) (EventWindow, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return EventWindow{}, err
	}
	if afterOffset < 0 || limit < 1 || limit > MaxObservationEvents {
		return EventWindow{}, fail("INVALID_EVENT_WINDOW", nil)
	}
	var result EventWindow
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		if _, err = runOnConnection(ctx, conn, runID); err != nil {
			return err
		}
		result.EventOffset, err = eventHighWatermark(ctx, conn, runID)
		if err != nil {
			return err
		}
		if afterOffset > result.EventOffset {
			return fail("INVALID_EVENT_OFFSET", nil)
		}
		rows, err := conn.QueryContext(
			ctx,
			`SELECT event_offset, kind, created_at
			 FROM events
			 WHERE run_id=? AND event_offset>?
			 ORDER BY event_offset
			 LIMIT ?`,
			runID,
			afterOffset,
			limit+1,
		)
		if err != nil {
			return dbError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var fact EventFact
			var createdAt string
			if err := rows.Scan(&fact.Offset, &fact.Kind, &createdAt); err != nil {
				return dbError(err)
			}
			fact.CreatedAt, err = parseTime(createdAt)
			if err != nil {
				return err
			}
			result.Events = append(result.Events, fact)
		}
		if err := rows.Err(); err != nil {
			return dbError(err)
		}
		if len(result.Events) > limit {
			result.Events = result.Events[:limit]
			result.HasMore = true
		}
		result.Through = afterOffset
		if len(result.Events) != 0 {
			result.Through = result.Events[len(result.Events)-1].Offset
		}
		if !result.HasMore {
			result.Through = result.EventOffset
		}
		return nil
	})
	return result, err
}
