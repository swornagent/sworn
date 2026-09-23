package journal

import (
	"context"
	"testing"
	"time"
)

// TestApplyControlAdmitsRetryWithEconomyContextRetryFlag is this attempt's
// own required end-to-end proof (S6-context-window-clamp A3): a work whose
// current-epoch try 1 failed ECONOMY_CONTEXT_EXHAUSTED is refused
// WORK_NOT_EXHAUSTED by a bare Retry exactly as before, but the identical
// Retry carrying the internal-only EconomyContextRetry flag is admitted
// immediately - at try 1, not only the third - and advances the epoch so
// the next epoch's try 1 can dispatch. It also proves the flag changes
// nothing for an unrelated work with no crossing at all.
func TestApplyControlAdmitsRetryWithEconomyContextRetryFlag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, _ := journalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	work := digest([]byte("context-exhausted-work"))
	id := AttemptEffectID(work, 1, 1)
	if err := store.EnsureAttempt(ctx, Command{
		RunID: run.ID, ReplayKey: id, Kind: "driver.dispatch",
		Payload: []byte("context-exhausted"), CreatedAt: now,
	}, Effect{
		RunID: run.ID, ID: id, ReplayKey: id, Kind: "driver.dispatch",
		BeforeDigest:   digest([]byte("before")),
		ExpectedDigest: digest([]byte("after")),
		UpdatedAt:      now,
	}, EffectAttempt{WorkID: work, Epoch: 1, Try: 1}); err != nil {
		t.Fatal(err)
	}
	claim, err := store.Claim(ctx, run.ID, id, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: id, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "ECONOMY_CONTEXT_EXHAUSTED",
		EventKind: "dispatch_operational_failure",
		At:        now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	at := now.Add(2 * time.Second)

	// A bare Retry at try 1 (the try budget is nowhere near exhausted) is
	// refused exactly as it would be for any other cause.
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "retry-bare", Kind: Retry,
		WorkID: work, ExpectedEpoch: 1,
	}, at); !IsCode(err, "WORK_NOT_EXHAUSTED") {
		t.Fatalf("bare retry at try 1 = %v, want WORK_NOT_EXHAUSTED", err)
	}

	// The identical Retry, carrying the internal-only flag a runtime-layer
	// admission gate would stamp after proving the crossing, is admitted
	// and advances the epoch.
	receipt, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "retry-context", Kind: Retry,
		WorkID: work, ExpectedEpoch: 1, EconomyContextRetry: true,
	}, at)
	if err != nil || receipt.Epoch != 2 {
		t.Fatalf("flagged retry = %#v, %v, want epoch 2", receipt, err)
	}

	// The next epoch's try 1 can now dispatch: EnsureAttempt admits it.
	nextID := AttemptEffectID(work, 2, 1)
	if err := store.EnsureAttempt(ctx, Command{
		RunID: run.ID, ReplayKey: nextID, Kind: "driver.dispatch",
		Payload: []byte("epoch-2-try-1"), CreatedAt: at,
	}, Effect{
		RunID: run.ID, ID: nextID, ReplayKey: nextID, Kind: "driver.dispatch",
		BeforeDigest:   digest([]byte("before-2")),
		ExpectedDigest: digest([]byte("after-2")),
		UpdatedAt:      at,
	}, EffectAttempt{WorkID: work, Epoch: 2, Try: 1}); err != nil {
		t.Fatalf("next epoch try 1 refused: %v", err)
	}

	// An exact replay of the same admitted command returns the cached
	// receipt rather than re-deriving admissibility (and rather than
	// REPLAY_CONFLICT), even though the epoch this stamp was originally
	// checked against has since advanced.
	replay, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "retry-context", Kind: Retry,
		WorkID: work, ExpectedEpoch: 1, EconomyContextRetry: true,
	}, at)
	if err != nil || replay.Epoch != 2 {
		t.Fatalf("exact replay = %#v, %v, want the cached epoch-2 receipt", replay, err)
	}

	// The flag changes nothing for an unrelated work with no crossing at
	// all when it is false: an ordinary Retry at try 1 with no exhaustion
	// still refuses WORK_NOT_EXHAUSTED.
	other := digest([]byte("unrelated-work"))
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "retry-unrelated", Kind: Retry,
		WorkID: other, ExpectedEpoch: 1, ExpectedGeneration: 1,
	}, at); !IsCode(err, "WORK_NOT_EXHAUSTED") {
		t.Fatalf("unrelated bare retry = %v, want WORK_NOT_EXHAUSTED", err)
	}
}

// TestEconomyContextRetryForbiddenOnNonRetryKinds pins validControl's
// closed vocabulary: the internal-only flag is never legitimate on Pause,
// Resume, Cancel, Takeover or Grant, so a stray true on any of them is
// refused INVALID_CONTROL rather than silently ignored.
func TestEconomyContextRetryForbiddenOnNonRetryKinds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, _ := journalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	work := digest([]byte("grant-forbidden-work"))

	for _, command := range []ControlCommand{
		{RunID: run.ID, ID: "pause-1", Kind: Pause, EconomyContextRetry: true},
		{RunID: run.ID, ID: "resume-1", Kind: Resume, EconomyContextRetry: true},
		{RunID: run.ID, ID: "cancel-1", Kind: Cancel, EconomyContextRetry: true},
		{RunID: run.ID, ID: "takeover-1", Kind: Takeover, EconomyContextRetry: true},
		{
			RunID: run.ID, ID: "grant-1", Kind: Grant,
			WorkID: work, ExpectedEpoch: 1, Unit: grantUnitTurns, Amount: 1,
			EconomyContextRetry: true,
		},
	} {
		if _, err := store.ApplyControl(ctx, command, now); !IsCode(err, "INVALID_CONTROL") {
			t.Fatalf("kind %s with EconomyContextRetry=true = %v, want INVALID_CONTROL", command.Kind, err)
		}
	}
}
