package journal

import (
	"context"
	"testing"
	"time"
)

// TestGrantValidatesClosedShape proves Grant's admission-time shape checks
// (S4-resumable-budget-stops A2): a stale/missing epoch, a unit outside the
// closed economy vocabulary, and a non-positive amount are all refused
// INVALID_CONTROL before anything is journaled.
func TestGrantValidatesClosedShape(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	work := digest([]byte("grant-shape-work"))

	cases := []struct {
		name    string
		command ControlCommand
	}{
		{
			"missing_unit",
			ControlCommand{
				RunID: run.ID, ID: "g-missing-unit", Kind: Grant,
				WorkID: work, ExpectedEpoch: 1, Amount: 10,
			},
		},
		{
			"unknown_unit",
			ControlCommand{
				RunID: run.ID, ID: "g-unknown-unit", Kind: Grant,
				WorkID: work, ExpectedEpoch: 1,
				Unit: "not_a_real_unit", Amount: 10,
			},
		},
		{
			"zero_amount",
			ControlCommand{
				RunID: run.ID, ID: "g-zero-amount", Kind: Grant,
				WorkID: work, ExpectedEpoch: 1,
				Unit: "economy_turns", Amount: 0,
			},
		},
		{
			"negative_amount",
			ControlCommand{
				RunID: run.ID, ID: "g-negative-amount", Kind: Grant,
				WorkID: work, ExpectedEpoch: 1,
				Unit: "economy_turns", Amount: -5,
			},
		},
		{
			"missing_epoch",
			ControlCommand{
				RunID: run.ID, ID: "g-missing-epoch", Kind: Grant,
				WorkID: work, Unit: "economy_turns", Amount: 10,
			},
		},
		{
			"invalid_work",
			ControlCommand{
				RunID: run.ID, ID: "g-invalid-work", Kind: Grant,
				WorkID: "not-a-digest", ExpectedEpoch: 1,
				Unit: "economy_turns", Amount: 10,
			},
		},
	}
	for _, tc := range cases {
		if _, err := store.ApplyControl(ctx, tc.command, now); !IsCode(err, "INVALID_CONTROL") {
			t.Fatalf("%s: got %v, want INVALID_CONTROL", tc.name, err)
		}
	}
}

// TestGrantAdmitsAdvancesEpochAndFoldsProjection proves the durable side of
// A2/A3: an admitted Grant advances the work's retry epoch exactly like
// Retry does, folds its amount cumulatively into ControlProjection keyed by
// work and unit, latches AcknowledgedUnknownUsage once any admitted Grant
// for the work carries it, refuses a stale epoch, and replays idempotently
// under the shared command-replay mechanism (exact replay returns the
// original receipt; a conflicting body under the same command ID is
// refused).
func TestGrantAdmitsAdvancesEpochAndFoldsProjection(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	work := digest([]byte("granted-work"))

	first := ControlCommand{
		RunID: run.ID, ID: "grant-1", Kind: Grant,
		WorkID: work, ExpectedEpoch: 1,
		Unit: "economy_turns", Amount: 50,
	}
	receipt, err := store.ApplyControl(ctx, first, now)
	if err != nil || receipt.Epoch != 2 || receipt.Kind != Grant {
		t.Fatalf("grant = %#v, %v", receipt, err)
	}

	projection, err := store.ControlProjection(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.RetryEpochs[work] != 2 {
		t.Fatalf("retry epoch = %d, want 2", projection.RetryEpochs[work])
	}
	if projection.GrantedAmount[work]["economy_turns"] != 50 {
		t.Fatalf("granted amount = %#v, want 50", projection.GrantedAmount[work])
	}
	if projection.AcknowledgedUnknownUsage[work] {
		t.Fatal("acknowledged unknown usage should stay false")
	}

	// A second grant for the same work and unit, at the new epoch,
	// accumulates rather than replacing, and latches the acknowledgment.
	second := ControlCommand{
		RunID: run.ID, ID: "grant-2", Kind: Grant,
		ExpectedGeneration: 1,
		WorkID:             work, ExpectedEpoch: 2,
		Unit: "economy_turns", Amount: 25, AcknowledgeUnknownUsage: true,
	}
	receipt2, err := store.ApplyControl(ctx, second, now)
	if err != nil || receipt2.Epoch != 3 {
		t.Fatalf("second grant = %#v, %v", receipt2, err)
	}
	projection, err = store.ControlProjection(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.GrantedAmount[work]["economy_turns"] != 75 {
		t.Fatalf("accumulated granted amount = %d, want 75",
			projection.GrantedAmount[work]["economy_turns"])
	}
	if !projection.AcknowledgedUnknownUsage[work] {
		t.Fatal("acknowledged unknown usage should now be true")
	}

	// A stale epoch is refused.
	stale := ControlCommand{
		RunID: run.ID, ID: "grant-3", Kind: Grant,
		ExpectedGeneration: 2,
		WorkID:             work, ExpectedEpoch: 1,
		Unit: "economy_turns", Amount: 10,
	}
	if _, err := store.ApplyControl(ctx, stale, now); !IsCode(err, "STALE_RETRY_EPOCH") {
		t.Fatalf("stale grant = %v, want STALE_RETRY_EPOCH", err)
	}

	// An exact replay of the first grant returns the original receipt.
	replay, err := store.ApplyControl(ctx, first, now.Add(time.Hour))
	if err != nil || replay != receipt {
		t.Fatalf("replay = %#v, %v, want %#v", replay, err, receipt)
	}

	// A conflicting body under the same command ID is refused.
	conflict := first
	conflict.Amount = 51
	if _, err := store.ApplyControl(ctx, conflict, now); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("conflicting replay = %v, want REPLAY_CONFLICT", err)
	}
}

// TestGrantWithRetryWorkIDAdvancesOwnerEpochNotDispatchWork proves the
// S4-resumable-budget-stops V2 repair at the journal layer: a Grant whose
// RetryWorkID names a different work than WorkID (a nested git.seal-wrapped
// dispatch, where the crossing's own dispatch-work identity carries no
// independent retry history of its own) checks and advances RetryEpochs
// under RetryWorkID, leaves WorkID's own RetryEpochs entry untouched, and
// still folds GrantedAmount/AcknowledgedUnknownUsage under WorkID exactly
// as before - the two identities are tracked independently, never
// conflated.
func TestGrantWithRetryWorkIDAdvancesOwnerEpochNotDispatchWork(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	dispatchWork := digest([]byte("nested-dispatch-work"))
	owner := digest([]byte("outer-git-seal-work"))

	grant := ControlCommand{
		RunID: run.ID, ID: "nested-grant-1", Kind: Grant,
		WorkID: dispatchWork, RetryWorkID: owner, ExpectedEpoch: 1,
		Unit: "economy_turns", Amount: 500,
	}
	receipt, err := store.ApplyControl(ctx, grant, now)
	if err != nil || receipt.Epoch != 2 {
		t.Fatalf("grant = %#v, %v", receipt, err)
	}
	projection, err := store.ControlProjection(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.RetryEpochs[owner] != 2 {
		t.Fatalf("owner retry epoch = %d, want 2", projection.RetryEpochs[owner])
	}
	if _, retried := projection.RetryEpochs[dispatchWork]; retried {
		t.Fatalf("dispatch-work retry epoch = %#v, want no entry",
			projection.RetryEpochs[dispatchWork])
	}
	if projection.GrantedAmount[dispatchWork]["economy_turns"] != 500 {
		t.Fatalf("granted amount = %#v, want 500 under the dispatch work",
			projection.GrantedAmount[dispatchWork])
	}

	// A second grant at the stale owner epoch (1, already advanced to 2) is
	// refused, proving the epoch check itself reads RetryWorkID, not WorkID.
	stale := ControlCommand{
		RunID: run.ID, ID: "nested-grant-2", Kind: Grant,
		ExpectedGeneration: 1,
		WorkID:             dispatchWork, RetryWorkID: owner, ExpectedEpoch: 1,
		Unit: "economy_turns", Amount: 10,
	}
	if _, err := store.ApplyControl(ctx, stale, now); !IsCode(err, "STALE_RETRY_EPOCH") {
		t.Fatalf("stale nested grant = %v, want STALE_RETRY_EPOCH", err)
	}

	// An exact replay of the first grant still returns the original receipt.
	replay, err := store.ApplyControl(ctx, grant, now.Add(time.Hour))
	if err != nil || replay != receipt {
		t.Fatalf("replay = %#v, %v, want %#v", replay, err, receipt)
	}
}
