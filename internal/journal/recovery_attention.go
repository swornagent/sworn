package journal

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

// ParkRecoveryAttentionCommand binds the terminal recovery step to the exact
// attention opened for it. The pair is committed atomically so a restart can
// never observe a parked lane without its operator question.
type ParkRecoveryAttentionCommand struct {
	Step      RecoveryStepCommand
	Attention OpenAttentionCommand
	Resolve   *ResolveAttentionCommand
}

type ParkRecoveryAttentionReceipt struct {
	Step      RecoveryStepReceipt
	Attention AttentionReceipt
	Resolved  *AttentionReceipt
}

// RetireRecoveryEffect is one exact member of a stale recovery cycle. Claimed
// members are completed as operational failures; already-terminal members are
// asserted unchanged so the attention cannot be retired against a different
// cycle.
type RetireRecoveryEffect struct {
	EffectID      string
	ExpectedState EffectState
	ClaimToken    string
}

// RetireRecoveryAttentionCommand atomically cancels an obsolete operator
// question and terminalizes the still-claimed effects that it used to wake.
type RetireRecoveryAttentionCommand struct {
	RunID              string
	Attention          AttentionBinding
	ExpectedGeneration int64
	Effects            []RetireRecoveryEffect
	ErrorCode          string
}

func validateParkRecoveryAttention(
	command ParkRecoveryAttentionCommand,
) error {
	if err := validateRecoveryStep(command.Step); err != nil {
		return err
	}
	if command.Step.Kind != RecoveryParkTrack ||
		command.Attention.RunID != command.Step.RunID ||
		command.Attention.ExpectedGeneration != 0 ||
		command.Attention.Attention.Ordinal != command.Step.Ordinal ||
		command.Attention.Attention.Recovery != command.Step.Binding {
		return fail("INVALID_RECOVERY_STEP", nil)
	}
	record := attentionCommandRecord{
		SchemaVersion:      attentionCommandVersion,
		RunID:              command.Attention.RunID,
		Kind:               attentionOpenAction,
		Attention:          command.Attention.Attention,
		ExpectedGeneration: command.Attention.ExpectedGeneration,
		Message:            command.Attention.Question,
	}
	if err := validateAttentionRecord(record); err != nil {
		return err
	}
	if command.Resolve == nil {
		return nil
	}
	if command.Resolve.RunID != command.Step.RunID ||
		command.Resolve.ExpectedGeneration != 2 ||
		command.Resolve.Attention.Recovery.CycleID !=
			command.Step.Binding.CycleID ||
		command.Resolve.Attention.Recovery.LaneID !=
			command.Step.Binding.LaneID ||
		command.Resolve.Attention.Recovery.ProgressID !=
			command.Step.Binding.ProgressID {
		return fail("INVALID_ATTENTION_BINDING", nil)
	}
	return validateAttentionRecord(attentionCommandRecord{
		SchemaVersion:      attentionCommandVersion,
		RunID:              command.Resolve.RunID,
		Kind:               attentionResolveAction,
		Attention:          command.Resolve.Attention,
		ExpectedGeneration: command.Resolve.ExpectedGeneration,
	})
}

func (s *Store) ParkRecoveryAttention(
	ctx context.Context,
	owner OwnerLease,
	command ParkRecoveryAttentionCommand,
	at time.Time,
) (ParkRecoveryAttentionReceipt, error) {
	if err := validateParkRecoveryAttention(command); err != nil {
		return ParkRecoveryAttentionReceipt{}, err
	}
	atValue, err := canonicalTime(at)
	if err != nil {
		return ParkRecoveryAttentionReceipt{}, err
	}
	record := attentionCommandRecord{
		SchemaVersion:      attentionCommandVersion,
		RunID:              command.Attention.RunID,
		Kind:               attentionOpenAction,
		Attention:          command.Attention.Attention,
		ExpectedGeneration: command.Attention.ExpectedGeneration,
		Message:            command.Attention.Question,
	}
	var receipt ParkRecoveryAttentionReceipt
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		if command.Resolve != nil {
			resolved, resolveErr := applyAttentionOnConnection(
				ctx,
				conn,
				&owner,
				attentionCommandRecord{
					SchemaVersion: attentionCommandVersion,
					RunID:         command.Resolve.RunID,
					Kind:          attentionResolveAction,
					Attention:     command.Resolve.Attention,
					ExpectedGeneration: command.Resolve.
						ExpectedGeneration,
				},
				at,
				atValue,
			)
			if resolveErr != nil {
				return resolveErr
			}
			receipt.Resolved = &resolved
		}
		var reserveErr error
		receipt.Step, reserveErr = reserveRecoveryStepOnConnection(
			ctx,
			conn,
			owner,
			command.Step,
			at,
			atValue,
		)
		if reserveErr != nil {
			return reserveErr
		}
		var attentionErr error
		receipt.Attention, attentionErr = applyAttentionOnConnection(
			ctx,
			conn,
			&owner,
			record,
			at,
			atValue,
		)
		return attentionErr
	})
	return receipt, err
}

func retireAttentionRecord(
	command RetireRecoveryAttentionCommand,
) (attentionCommandRecord, error) {
	effects := make([]attentionRetireEffect, len(command.Effects))
	for index, effect := range command.Effects {
		effects[index] = attentionRetireEffect{
			ID:            effect.EffectID,
			ExpectedState: effect.ExpectedState,
			ClaimToken:    effect.ClaimToken,
		}
	}
	sort.Slice(effects, func(left, right int) bool {
		return effects[left].ID < effects[right].ID
	})
	record := attentionCommandRecord{
		SchemaVersion:      attentionCommandVersion,
		RunID:              command.RunID,
		Kind:               attentionRetireAction,
		Attention:          command.Attention,
		ExpectedGeneration: command.ExpectedGeneration,
		RetireEffects:      effects,
		ErrorCode:          command.ErrorCode,
	}
	if err := validateAttentionRecord(record); err != nil {
		return attentionCommandRecord{}, err
	}
	return record, nil
}

func (s *Store) RetireRecoveryAttentionOwned(
	ctx context.Context,
	owner OwnerLease,
	command RetireRecoveryAttentionCommand,
	at time.Time,
) (AttentionReceipt, error) {
	record, err := retireAttentionRecord(command)
	if err != nil {
		return AttentionReceipt{}, err
	}
	if owner.RunID != command.RunID {
		return AttentionReceipt{}, fail("OWNER_MISMATCH", nil)
	}
	atValue, err := canonicalTime(at)
	if err != nil {
		return AttentionReceipt{}, err
	}
	var receipt AttentionReceipt
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		var applyErr error
		receipt, applyErr = applyAttentionOnConnection(
			ctx,
			conn,
			&owner,
			record,
			at,
			atValue,
		)
		if applyErr != nil {
			return applyErr
		}

		retiredClaims := 0
		pendingClaims := 0
		current := make([]Effect, len(record.RetireEffects))
		for index, expected := range record.RetireEffects {
			effect, effectErr := effectOnConnection(
				ctx,
				conn,
				command.RunID,
				expected.ID,
			)
			if effectErr != nil {
				return effectErr
			}
			current[index] = effect
			if expected.ExpectedState == Claimed {
				switch {
				case effect.State == Claimed &&
					effect.CurrentClaim == expected.ClaimToken:
					pendingClaims++
				case effect.State == OperationalFailed &&
					effect.ErrorCode == command.ErrorCode:
					retiredClaims++
				default:
					return fail("STALE_COMPLETION", nil)
				}
				continue
			}
			if effect.State != expected.ExpectedState ||
				effect.CurrentClaim != "" {
				return fail("STALE_COMPLETION", nil)
			}
		}
		if retiredClaims > 0 && pendingClaims > 0 {
			return fail("CORRUPT_JOURNAL", nil)
		}
		if retiredClaims > 0 {
			return nil
		}
		for index, expected := range record.RetireEffects {
			if expected.ExpectedState != Claimed {
				continue
			}
			effect := current[index]
			if err := completeOnConnection(
				ctx,
				conn,
				Completion{
					RunID:     command.RunID,
					EffectID:  effect.ID,
					Token:     expected.ClaimToken,
					State:     OperationalFailed,
					ErrorCode: command.ErrorCode,
					EventKind: "effect_operational_failure",
					EventBody: []byte(effect.Kind),
					At:        at,
				},
			); err != nil {
				return err
			}
		}
		return nil
	})
	return receipt, err
}
