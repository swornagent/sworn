package journal

import (
	"context"
	"database/sql"
	"time"
)

const uncertaintyResolvedEvent = "effect_uncertainty_resolved"
const uncertaintyRearmedEvent = "effect_uncertainty_rearmed"

// MaxUncertainResolutionEffects bounds one owner-held journal transaction.
const MaxUncertainResolutionEffects = 256

// ResolveUncertainOwned terminalizes one exact ambiguous effect after its
// authority has become stale. The owner check, state transition, and event are
// committed together so a failed event write cannot leave a silent terminal
// effect.
func (s *Store) ResolveUncertainOwned(
	ctx context.Context,
	owner OwnerLease,
	runID string,
	effectID string,
	errorCode string,
	at time.Time,
) error {
	return s.ResolveUncertainManyOwned(
		ctx,
		owner,
		runID,
		[]string{effectID},
		errorCode,
		at,
	)
}

// ResolveUncertainManyOwned terminalizes a bounded set of exact ambiguous
// effects in one transaction. Every effect must still be uncertain; otherwise
// none of the effects or their events are changed.
func (s *Store) ResolveUncertainManyOwned(
	ctx context.Context,
	owner OwnerLease,
	runID string,
	effectIDs []string,
	errorCode string,
	at time.Time,
) error {
	if err := validateIdentity(runID, "run"); err != nil {
		return err
	}
	if len(effectIDs) == 0 ||
		len(effectIDs) > MaxUncertainResolutionEffects {
		return fail("INVALID_RECOVERY", nil)
	}
	seen := make(map[string]struct{}, len(effectIDs))
	for _, effectID := range effectIDs {
		if err := validateIdentity(effectID, "effect"); err != nil {
			return err
		}
		if _, duplicate := seen[effectID]; duplicate {
			return fail("INVALID_RECOVERY", nil)
		}
		seen[effectID] = struct{}{}
	}
	if err := validateIdentity(errorCode, "error_code"); err != nil {
		return err
	}
	if owner.RunID != runID {
		return fail("OWNER_FENCED", nil)
	}
	timestamp, err := canonicalTime(at)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		if err := checkOwner(ctx, conn, owner, at); err != nil {
			return err
		}
		for _, effectID := range effectIDs {
			result, err := conn.ExecContext(
				ctx,
				`UPDATE effects SET
					state = 'operational_failed',
					current_claim = NULL,
					result_digest = ?,
					result = ?,
					error_code = ?,
					updated_at = ?
				 WHERE run_id = ? AND effect_id = ? AND state = 'uncertain'`,
				digest(nil),
				[]byte{},
				errorCode,
				timestamp,
				runID,
				effectID,
			)
			if err != nil {
				return dbError(err)
			}
			if err := requireRows(result, "EFFECT_NOT_UNCERTAIN"); err != nil {
				return err
			}
			if err := appendEvent(
				ctx,
				conn,
				runID,
				uncertaintyResolvedEvent,
				[]byte(effectID),
				timestamp,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// RearmUncertainOwned returns one exact ambiguous effect to Pending after the
// caller has proved that its external effect is all-old. The original attempt
// identity is retained; only a new finite claim may execute it again.
func (s *Store) RearmUncertainOwned(
	ctx context.Context,
	owner OwnerLease,
	runID string,
	effectID string,
	at time.Time,
) error {
	return s.RearmUncertainManyOwned(
		ctx,
		owner,
		runID,
		[]string{effectID},
		at,
	)
}

// RearmUncertainManyOwned atomically returns a bounded related set of exact
// ambiguous effects to Pending. Every member must still be Uncertain and the
// runtime owner must remain current, otherwise the transaction changes none.
func (s *Store) RearmUncertainManyOwned(
	ctx context.Context,
	owner OwnerLease,
	runID string,
	effectIDs []string,
	at time.Time,
) error {
	if err := validateIdentity(runID, "run"); err != nil {
		return err
	}
	if len(effectIDs) == 0 ||
		len(effectIDs) > MaxUncertainResolutionEffects {
		return fail("INVALID_RECOVERY", nil)
	}
	seen := make(map[string]struct{}, len(effectIDs))
	for _, effectID := range effectIDs {
		if err := validateIdentity(effectID, "effect"); err != nil {
			return err
		}
		if _, duplicate := seen[effectID]; duplicate {
			return fail("INVALID_RECOVERY", nil)
		}
		seen[effectID] = struct{}{}
	}
	if owner.RunID != runID {
		return fail("OWNER_FENCED", nil)
	}
	timestamp, err := canonicalTime(at)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		if err := checkOwner(ctx, conn, owner, at); err != nil {
			return err
		}
		for _, effectID := range effectIDs {
			result, err := conn.ExecContext(
				ctx,
				`UPDATE effects SET
					state = 'pending',
					current_claim = NULL,
					result_digest = ?,
					result = ?,
					error_code = '',
					updated_at = ?
				 WHERE run_id = ? AND effect_id = ? AND state = 'uncertain'`,
				digest(nil),
				[]byte{},
				timestamp,
				runID,
				effectID,
			)
			if err != nil {
				return dbError(err)
			}
			if err := requireRows(result, "EFFECT_NOT_UNCERTAIN"); err != nil {
				return err
			}
			if err := appendEvent(
				ctx,
				conn,
				runID,
				uncertaintyRearmedEvent,
				[]byte(effectID),
				timestamp,
			); err != nil {
				return err
			}
		}
		return nil
	})
}
