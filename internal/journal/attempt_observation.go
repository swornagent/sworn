package journal

import (
	"context"
	"database/sql"
	"errors"
)

// AttemptObservation is the durable evidence a failed attempt was digested
// from: the stored marshaled driver observation bound to the digest that
// identifies it. Stored=false is distinguishably absent (a successful
// attempt, whose sealed handoff is its durable record, or a historical row
// predating this slice) — never corruption. Partial=true marks a body the
// MaxPayloadBytes bound truncated; its Digest still covers the full
// marshaled bytes, so the prefix cannot be re-verified by construction.
type AttemptObservation struct {
	RunID          string
	EffectID       string
	Number         int64
	Responsibility string
	Transport      string
	Digest         string
	Body           []byte
	Stored         bool
	Partial        bool
}

// AttemptObservation returns the failed attempt's observation for a given
// effect and attempt number. It is the read half of A1/A2: the body comes
// back digest-verified against the attempt's observation_digest exactly as
// the evaluation read path re-verifies usage, so a tampered journal fails
// CORRUPT_JOURNAL instead of serving altered evidence.
func (s *Store) AttemptObservation(
	ctx context.Context,
	runID, effectID string,
	attempt int64,
) (AttemptObservation, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return AttemptObservation{}, err
	}
	if err := validateIdentity(effectID, "effect"); err != nil {
		return AttemptObservation{}, err
	}
	if attempt < 1 {
		return AttemptObservation{}, fail("INVALID_ATTEMPT", nil)
	}
	if s == nil {
		return AttemptObservation{}, fail("CLOSED", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil || s.conn == nil {
		return AttemptObservation{}, fail("CLOSED", nil)
	}
	result := AttemptObservation{
		RunID:    runID,
		EffectID: effectID,
		Number:   attempt,
	}
	var (
		body    []byte
		partial int64
	)
	err := s.conn.QueryRowContext(
		ctx,
		`SELECT responsibility, transport_status, observation_digest,
		        observation_body, observation_partial
		 FROM attempts
		 WHERE run_id=? AND effect_id=? AND attempt=?`,
		runID,
		effectID,
		attempt,
	).Scan(
		&result.Responsibility,
		&result.Transport,
		&result.Digest,
		&body,
		&partial,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AttemptObservation{}, fail("ATTEMPT_NOT_FOUND", nil)
	}
	if err != nil {
		return AttemptObservation{}, dbError(err)
	}
	if !digestPattern.MatchString(result.Digest) ||
		(partial != 0 && partial != 1) {
		return AttemptObservation{}, fail("CORRUPT_JOURNAL", nil)
	}
	result.Partial = partial == 1
	if body == nil {
		result.Body = nil
		result.Stored = false
		return result, nil
	}
	result.Body = append([]byte(nil), body...)
	result.Stored = true
	if !result.Partial && digest(result.Body) != result.Digest {
		return AttemptObservation{}, fail("CORRUPT_JOURNAL", nil)
	}
	return result, nil
}
