package journal

import (
	"context"
	"database/sql"
	"time"
)

// EvaluationFactKind identifies the two content-free fact families exposed to
// the local evaluator. The evaluator receives no command, event, result,
// receipt, diagnostic, or model-output body.
type EvaluationFactKind string

const (
	EvaluationEvent   EvaluationFactKind = "event"
	EvaluationAttempt EvaluationFactKind = "attempt"
)

// EvaluationFact is one durable fact from the same SQLite snapshot as the
// returned high-water mark. Usage is the already-sanitized usage receipt, not
// provider output.
type EvaluationFact struct {
	Kind EvaluationFactKind

	EventOffset int64
	EventKind   string

	EffectKind     string
	EffectState    EffectState
	Attempt        int64
	Responsibility string
	Transport      string
	Usage          []byte
	StartedAt      time.Time
	FinishedAt     time.Time
}

type EvaluationWindow struct {
	Run           Run
	ThroughOffset int64
	ObservedAt    time.Time
}

// VisitEvaluation streams every durable event and driver attempt from one
// read transaction. It is deliberately cumulative: callers can aggregate a
// long run exactly with bounded memory, then bind the result to ThroughOffset.
//
// The visitor must not call back into this Store.
func (s *Store) VisitEvaluation(
	ctx context.Context,
	runID string,
	visit func(EvaluationFact),
) (EvaluationWindow, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return EvaluationWindow{}, err
	}
	if visit == nil {
		return EvaluationWindow{}, fail("INVALID_EVALUATION_VISITOR", nil)
	}
	var result EvaluationWindow
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		result.Run, err = runOnConnection(ctx, conn, runID)
		if err != nil {
			return err
		}
		result.ThroughOffset, err = eventHighWatermark(ctx, conn, runID)
		if err != nil {
			return err
		}

		eventRows, err := conn.QueryContext(
			ctx,
			`SELECT event_offset, kind, created_at
			 FROM events
			 WHERE run_id = ? AND event_offset <= ?
			 ORDER BY event_offset`,
			runID,
			result.ThroughOffset,
		)
		if err != nil {
			return dbError(err)
		}
		for eventRows.Next() {
			var offset int64
			var kind, createdAt string
			if err := eventRows.Scan(&offset, &kind, &createdAt); err != nil {
				_ = eventRows.Close()
				return dbError(err)
			}
			observedAt, err := parseTime(createdAt)
			if err != nil {
				_ = eventRows.Close()
				return err
			}
			result.ObservedAt = observedAt
			visit(EvaluationFact{
				Kind:        EvaluationEvent,
				EventOffset: offset,
				EventKind:   kind,
				FinishedAt:  observedAt,
			})
		}
		if err := eventRows.Close(); err != nil {
			return dbError(err)
		}

		attemptRows, err := conn.QueryContext(
			ctx,
			`SELECT e.kind, e.state, a.attempt, a.responsibility,
			        a.transport_status, a.usage_digest, a.usage,
			        c.created_at, a.created_at
			 FROM attempts AS a
			 JOIN effects AS e
			   ON e.run_id = a.run_id AND e.effect_id = a.effect_id
			 JOIN commands AS c
			   ON c.run_id = e.run_id AND c.replay_key = e.replay_key
			 WHERE a.run_id = ?
			 ORDER BY a.created_at, a.effect_id, a.attempt`,
			runID,
		)
		if err != nil {
			return dbError(err)
		}
		for attemptRows.Next() {
			var fact EvaluationFact
			var state, usageDigest, startedAt, finishedAt string
			fact.Kind = EvaluationAttempt
			if err := attemptRows.Scan(
				&fact.EffectKind,
				&state,
				&fact.Attempt,
				&fact.Responsibility,
				&fact.Transport,
				&usageDigest,
				&fact.Usage,
				&startedAt,
				&finishedAt,
			); err != nil {
				_ = attemptRows.Close()
				return dbError(err)
			}
			if !digestPattern.MatchString(usageDigest) ||
				digest(fact.Usage) != usageDigest {
				_ = attemptRows.Close()
				return fail("CORRUPT_JOURNAL", nil)
			}
			fact.EffectState = EffectState(state)
			if !validEvaluationEffectState(fact.EffectState) {
				_ = attemptRows.Close()
				return fail("CORRUPT_JOURNAL", nil)
			}
			fact.StartedAt, err = parseTime(startedAt)
			if err != nil {
				_ = attemptRows.Close()
				return err
			}
			fact.FinishedAt, err = parseTime(finishedAt)
			if err != nil || fact.FinishedAt.Before(fact.StartedAt) {
				_ = attemptRows.Close()
				return fail("CORRUPT_JOURNAL", nil)
			}
			fact.Usage = append([]byte(nil), fact.Usage...)
			visit(fact)
		}
		if err := attemptRows.Close(); err != nil {
			return dbError(err)
		}
		if result.ThroughOffset == 0 {
			result.ObservedAt = result.Run.CreatedAt
		}
		return nil
	})
	return result, err
}

func validEvaluationEffectState(state EffectState) bool {
	switch state {
	case Pending, Claimed, Succeeded, OperationalFailed, Uncertain:
		return true
	default:
		return false
	}
}
