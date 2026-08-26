package runtime

import (
	"bytes"
	"context"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// sealedProposalHook binds plan persistence, and any proposal-carried
// new-contract bytes, to one claimed dispatch attempt. Each is its own
// journal child, claimed and completed under the same runtime owner as the
// parent dispatch; the driver merely blocks publication until this callback
// succeeds. Contract bytes ride a separate planner.sealed_contracts child
// keyed off the same parent so the planner.sealed_plan child's payload and
// behavior stay byte-for-byte unchanged when no contracts are carried.
func (s *Service) sealedProposalHook(
	owner journal.OwnerLease,
	parentEffectID string,
) driver.SealedProposalHook {
	if s == nil || s.journal == nil || owner.RunID == "" || parentEffectID == "" {
		return nil
	}
	return func(ctx context.Context, body []byte, contracts map[string][]byte) error {
		if ctx == nil {
			ctx = context.Background()
		}
		now := s.now().UTC()
		if err := s.persistSealedChild(
			ctx, owner, parentEffectID, "sealed-proposal",
			"planner.sealed_plan", "planner.sealed_plan_persisted",
			body, now,
		); err != nil {
			return err
		}
		if len(contracts) == 0 {
			return nil
		}
		return s.persistSealedChild(
			ctx, owner, parentEffectID, "sealed-contracts",
			"planner.sealed_contracts", "planner.sealed_contracts_persisted",
			mustJSON(contracts), now,
		)
	}
}

// persistSealedChild durably records one bounded child effect of a claimed
// dispatch attempt: it is idempotent across recovery (a prior Succeeded
// child with matching bytes is a no-op; a mismatch is CORRUPT_JOURNAL) and
// claims-and-completes a Pending child under the same owner otherwise.
func (s *Service) persistSealedChild(
	ctx context.Context,
	owner journal.OwnerLease,
	parentEffectID, suffix, kind, eventKind string,
	body []byte,
	now time.Time,
) error {
	childID := parentEffectID + "/" + suffix
	command := journal.Command{
		RunID: owner.RunID, ReplayKey: childID,
		Kind: kind, Payload: append([]byte(nil), body...),
		CreatedAt: now,
	}
	effect := journal.Effect{
		RunID: owner.RunID, ID: childID, ReplayKey: childID,
		Kind:           kind,
		BeforeDigest:   driver.Digest([]byte(parentEffectID)),
		ExpectedDigest: driver.Digest(body), UpdatedAt: now,
	}
	if err := s.journal.RecordCommandEffect(
		context.WithoutCancel(ctx), command, effect,
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	observed, err := s.journal.Effect(ctx, owner.RunID, childID)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if observed.State == journal.Succeeded {
		if !bytes.Equal(observed.Result, body) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return nil
	}
	if observed.State != journal.Pending {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	claim, err := s.journal.ClaimOwned(
		context.WithoutCancel(ctx), owner, childID, now, effectLease,
	)
	if err != nil {
		return runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	if err := s.journal.CompleteOwned(
		context.WithoutCancel(ctx), owner,
		journal.Completion{
			RunID: owner.RunID, EffectID: childID, Token: claim.Token,
			State: journal.Succeeded, Result: append([]byte(nil), body...),
			EventKind: eventKind, At: now,
		},
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}
