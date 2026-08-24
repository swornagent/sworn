package runtime

import (
	"bytes"
	"context"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// sealedProposalHook binds plan persistence to one claimed dispatch attempt.
// The journal child is claimed and completed under the same runtime owner as
// its parent; the driver merely blocks publication until this callback
// succeeds.
func (s *Service) sealedProposalHook(
	owner journal.OwnerLease,
	parentEffectID string,
) driver.SealedProposalHook {
	if s == nil || s.journal == nil || owner.RunID == "" || parentEffectID == "" {
		return nil
	}
	return func(ctx context.Context, body []byte) error {
		if ctx == nil {
			ctx = context.Background()
		}
		childID := parentEffectID + "/sealed-proposal"
		now := s.now().UTC()
		command := journal.Command{
			RunID: owner.RunID, ReplayKey: childID,
			Kind: "planner.sealed_plan", Payload: append([]byte(nil), body...),
			CreatedAt: now,
		}
		effect := journal.Effect{
			RunID: owner.RunID, ID: childID, ReplayKey: childID,
			Kind:           "planner.sealed_plan",
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
				EventKind: "planner.sealed_plan_persisted", At: now,
			},
		); err != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return nil
	}
}
