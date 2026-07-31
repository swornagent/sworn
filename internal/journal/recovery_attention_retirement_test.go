package journal

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRetireRecoveryAttentionOwnedIsAtomicAndReplaySafe(t *testing.T) {
	t.Parallel()

	for _, answered := range []bool{false, true} {
		answered := answered
		name := "open"
		if answered {
			name = "answered"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, run, fixtureCommand, firstEffect := journalFixture(t)
			ctx := context.Background()
			now := run.CreatedAt.Add(time.Second)
			owner, err := store.AcquireOwner(
				ctx,
				run.ID,
				now,
				time.Minute,
				false,
			)
			if err != nil {
				t.Fatal(err)
			}
			firstClaim, err := store.ClaimOwned(
				ctx,
				owner,
				firstEffect.ID,
				now,
				time.Minute,
			)
			if err != nil {
				t.Fatal(err)
			}
			secondCommand := fixtureCommand
			secondCommand.ReplayKey = "effect-2"
			if err := store.RecordCommand(ctx, secondCommand); err != nil {
				t.Fatal(err)
			}
			secondEffect := firstEffect
			secondEffect.ID = secondCommand.ReplayKey
			secondEffect.ReplayKey = secondCommand.ReplayKey
			if err := store.EnsureEffect(ctx, secondEffect); err != nil {
				t.Fatal(err)
			}
			secondClaim, err := store.ClaimOwned(
				ctx,
				owner,
				secondEffect.ID,
				now,
				time.Minute,
			)
			if err != nil {
				t.Fatal(err)
			}

			recovery := testRecoveryBinding(
				"T3-runtime",
				"retire-cycle-"+name,
				"turn",
				"progress",
			)
			attention := testAttentionBinding(recovery, 1)
			if _, err := store.OpenAttention(
				ctx,
				owner,
				OpenAttentionCommand{
					RunID:              run.ID,
					Attention:          attention,
					ExpectedGeneration: 0,
					Question:           "Which exact cycle should continue?",
				},
				now,
			); err != nil {
				t.Fatal(err)
			}
			generation := int64(1)
			if answered {
				if _, err := store.AnswerAttention(
					ctx,
					AnswerAttentionCommand{
						RunID:              run.ID,
						Attention:          attention,
						ExpectedGeneration: 1,
						Answer:             "Continue this cycle.",
					},
					now,
				); err != nil {
					t.Fatal(err)
				}
				generation = 2
			}
			retireCommand := RetireRecoveryAttentionCommand{
				RunID:              run.ID,
				Attention:          attention,
				ExpectedGeneration: generation,
				ErrorCode:          "stale_authority",
				Effects: []RetireRecoveryEffect{
					{
						EffectID:      firstEffect.ID,
						ExpectedState: Claimed,
						ClaimToken:    firstClaim.Token,
					},
					{
						EffectID:      secondEffect.ID,
						ExpectedState: Claimed,
						ClaimToken:    secondClaim.Token,
					},
				},
			}
			substituted := retireCommand
			substituted.Effects = append(
				[]RetireRecoveryEffect(nil),
				retireCommand.Effects...,
			)
			substituted.Effects[1].ClaimToken = strings.Repeat("0", 64)
			if _, err := store.RetireRecoveryAttentionOwned(
				ctx,
				owner,
				substituted,
				now,
			); !IsCode(err, "STALE_COMPLETION") {
				t.Fatalf("substituted retirement = %v", err)
			}
			projection, err := store.Attention(
				ctx,
				run.ID,
				attention.ID,
			)
			if err != nil {
				t.Fatal(err)
			}
			wantState := AttentionOpen
			if answered {
				wantState = AttentionAnswered
			}
			if projection.State != wantState ||
				projection.Generation != generation {
				t.Fatalf("partial attention retirement = %#v", projection)
			}
			for _, id := range []string{firstEffect.ID, secondEffect.ID} {
				effect, err := store.Effect(ctx, run.ID, id)
				if err != nil {
					t.Fatal(err)
				}
				if effect.State != Claimed {
					t.Fatalf("partial effect retirement = %#v", effect)
				}
			}

			receipt, err := store.RetireRecoveryAttentionOwned(
				ctx,
				owner,
				retireCommand,
				now,
			)
			if err != nil ||
				receipt.State != AttentionCancelled ||
				receipt.Generation != generation+1 {
				t.Fatalf("retirement = %#v, %v", receipt, err)
			}
			replayed, err := store.RetireRecoveryAttentionOwned(
				ctx,
				owner,
				retireCommand,
				now.Add(time.Hour),
			)
			if err != nil || replayed != receipt {
				t.Fatalf("retirement replay = %#v, %v", replayed, err)
			}
			for _, id := range []string{firstEffect.ID, secondEffect.ID} {
				effect, err := store.Effect(ctx, run.ID, id)
				if err != nil {
					t.Fatal(err)
				}
				if effect.State != OperationalFailed ||
					effect.ErrorCode != retireCommand.ErrorCode {
					t.Fatalf("retired effect = %#v", effect)
				}
			}
		})
	}
}
