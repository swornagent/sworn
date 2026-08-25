package journal

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestEffectCarriesLiveClaimExpiry(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := store.Effect(ctx, run.ID, effect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if observed.State != Claimed ||
		observed.CurrentClaim != claim.Token ||
		!observed.CurrentClaimExpiresAt.Equal(claim.ExpiresAt) {
		t.Fatalf("claimed effect = %#v, claim = %#v", observed, claim)
	}

	// Completion clears the claim and the expiry goes back to zero.
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: Succeeded, Result: []byte("result"),
		EventKind: "fixture_complete", At: now,
	}); err != nil {
		t.Fatal(err)
	}
	observed, err = store.Effect(ctx, run.ID, effect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if observed.State != Succeeded ||
		observed.CurrentClaim != "" ||
		!observed.CurrentClaimExpiresAt.IsZero() {
		t.Fatalf("completed effect = %#v", observed)
	}
}

func TestClaimedEffectWithoutLiveClaimFailsClosed(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Force the engine-impossible shape: the effect row stays claimed while
	// its claim row is completed. The read seam must fail closed instead of
	// projecting a claim with no visible expiry.
	at, _ := canonicalTime(now.Add(time.Second))
	err = store.immediate(ctx, func(conn *sql.Conn) error {
		result, err := conn.ExecContext(
			ctx,
			`UPDATE claims SET completed_at = ?, outcome = 'ambiguous'
			 WHERE run_id = ? AND effect_id = ? AND token = ?`,
			at,
			run.ID,
			effect.ID,
			claim.Token,
		)
		if err != nil {
			return dbError(err)
		}
		return requireRows(result, "STALE_COMPLETION")
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Effect(ctx, run.ID, effect.ID); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("claimed-without-live-claim = %v", err)
	}
}

func TestEffectCompletionEvidenceSeesSeededRows(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	attempts, receipts, err := store.EffectCompletionEvidence(
		ctx,
		run.ID,
		effect.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts || receipts {
		t.Fatalf("fresh effect evidence = %t/%t", attempts, receipts)
	}
	// Seed one attempt row and one receipt row directly; the accessor must
	// report both for the reconciliation evidence decision.
	err = store.immediate(ctx, func(conn *sql.Conn) error {
		at, _ := canonicalTime(now)
		if _, err := conn.ExecContext(
			ctx,
			`INSERT INTO attempts(
				run_id, effect_id, attempt, responsibility, transport_status,
				observation_digest, usage_digest, usage, handoff_digest,
				created_at, observation_body, observation_partial
			) VALUES(?, ?, 1, 'implementer_design', 'completed', ?, ?, ?, NULL, ?, NULL, 0)`,
			run.ID,
			effect.ID,
			digest(nil),
			digest(nil),
			[]byte{},
			at,
		); err != nil {
			return dbError(err)
		}
		if _, err := conn.ExecContext(
			ctx,
			`INSERT INTO receipts(digest, run_id, effect_id, kind, body, created_at)
			 VALUES(?, ?, ?, 'sealed_driver_handoff', ?, ?)`,
			digest([]byte("fixture receipt")),
			run.ID,
			effect.ID,
			[]byte("fixture receipt"),
			at,
		); err != nil {
			return dbError(err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	attempts, receipts, err = store.EffectCompletionEvidence(
		ctx,
		run.ID,
		effect.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !attempts || !receipts {
		t.Fatalf("seeded evidence = %t/%t", attempts, receipts)
	}
}

func TestRetryGateAdmitsUncertainAndExpiredClaimedDispatches(t *testing.T) {
	t.Parallel()

	work := digest([]byte("wedge-work"))

	newStore := func(t *testing.T) (*Store, Run) {
		t.Helper()
		store, run, _, _ := journalFixture(t)
		return store, run
	}

	seed := func(
		t *testing.T,
		store *Store,
		run Run,
		state EffectState,
		claimAt time.Duration,
	) {
		t.Helper()
		ctx := context.Background()
		now := run.CreatedAt.Add(time.Second)
		id := AttemptEffectID(work, 1, 1)
		if err := store.EnsureAttempt(ctx, Command{
			RunID: run.ID, ReplayKey: id, Kind: "driver.dispatch",
			Payload: []byte("wedge"), CreatedAt: now,
		}, Effect{
			RunID: run.ID, ID: id, ReplayKey: id, Kind: "driver.dispatch",
			BeforeDigest:   digest([]byte("before")),
			ExpectedDigest: digest([]byte("after")),
			UpdatedAt:      now,
		}, EffectAttempt{WorkID: work, Epoch: 1, Try: 1}); err != nil {
			t.Fatal(err)
		}
		switch state {
		case Claimed:
			claim, err := store.Claim(ctx, run.ID, id, now.Add(claimAt), time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			_ = claim
		case Uncertain:
			claim, err := store.Claim(ctx, run.ID, id, now, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.ReconcileOwned(ctx, OwnerLease{}, Completion{
				RunID: run.ID, EffectID: id, Token: claim.Token,
				EventKind: "fixture_uncertain", At: now,
			}, RecoveryAmbiguous); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unsupported state %q", state)
		}
	}

	command := func(runID, id string) ControlCommand {
		return ControlCommand{
			RunID: runID, ID: id, Kind: Retry,
			WorkID: work, ExpectedEpoch: 1,
		}
	}

	t.Run("uncertain admits retry", func(t *testing.T) {
		store, run := newStore(t)
		seed(t, store, run, Uncertain, 0)
		receipt, err := store.ApplyControl(
			context.Background(),
			command(run.ID, "retry-1"),
			run.CreatedAt.Add(time.Second),
		)
		if err != nil || receipt.Epoch != 2 {
			t.Fatalf("uncertain retry = %#v, %v", receipt, err)
		}
	})

	t.Run("expired claimed admits retry", func(t *testing.T) {
		store, run := newStore(t)
		seed(t, store, run, Claimed, -time.Hour)
		receipt, err := store.ApplyControl(
			context.Background(),
			command(run.ID, "retry-2"),
			run.CreatedAt.Add(time.Second),
		)
		if err != nil || receipt.Epoch != 2 {
			t.Fatalf("expired claimed retry = %#v, %v", receipt, err)
		}
	})

	t.Run("unexpired claimed refuses", func(t *testing.T) {
		store, run := newStore(t)
		seed(t, store, run, Claimed, 0)
		if _, err := store.ApplyControl(
			context.Background(),
			command(run.ID, "retry-3"),
			run.CreatedAt.Add(time.Second),
		); !IsCode(err, "WORK_NOT_EXHAUSTED") {
			t.Fatalf("unexpired claimed retry = %v", err)
		}
	})

	t.Run("expired claimed with active owner refuses", func(t *testing.T) {
		store, run := newStore(t)
		seed(t, store, run, Claimed, -time.Hour)
		ctx := context.Background()
		now := run.CreatedAt.Add(time.Second)
		owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyControl(
			ctx,
			command(run.ID, "retry-4"),
			now,
		); !IsCode(err, "WORK_NOT_EXHAUSTED") {
			t.Fatalf("active-owner claimed retry = %v", err)
		}
		_ = owner
	})

	t.Run("no matching shape refuses", func(t *testing.T) {
		store, run := newStore(t)
		if _, err := store.ApplyControl(
			context.Background(),
			command(run.ID, "retry-5"),
			run.CreatedAt.Add(time.Second),
		); !IsCode(err, "WORK_NOT_EXHAUSTED") {
			t.Fatalf("plain retry = %v", err)
		}
	})
}

func TestAttentionAnswerOversizeNamesTheBound(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	recovery := RecoveryBinding{
		CycleID:    "sha256:" + strings.Repeat("a", 64),
		LaneID:     "lane-oversize",
		TurnID:     "sha256:" + strings.Repeat("b", 64),
		ProgressID: "sha256:" + strings.Repeat("c", 64),
	}
	binding := AttentionBinding{
		ID:       AttentionID(recovery, 1),
		Ordinal:  1,
		Recovery: recovery,
	}
	if _, err := store.OpenAttention(ctx, owner, OpenAttentionCommand{
		RunID:     run.ID,
		Attention: binding,
		Question:  "Which value should I use?",
	}, now); err != nil {
		t.Fatal(err)
	}
	oversize := strings.Repeat("x", MaxAttentionAnswerBytes+1)
	_, err = store.AnswerAttention(ctx, AnswerAttentionCommand{
		RunID:              run.ID,
		Attention:          binding,
		ExpectedGeneration: 1,
		Answer:             oversize,
	}, now)
	if !IsCode(err, "ATTENTION_ANSWER_OVERSIZE") {
		t.Fatalf("oversize answer = %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "16384") {
		t.Fatalf("oversize detail names the bound: %v", err)
	}
}

// A4: each control refusal carries its own durable code so a rejected
// control never reads as one anonymous refusal.
func TestControlRefusalsCarryDistinctCodes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, _ := journalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}

	staleGeneration := ControlCommand{
		RunID: run.ID, ID: "c-stale", Kind: Pause, ExpectedGeneration: 7,
	}
	if _, err := store.ApplyControl(ctx, staleGeneration, now); !IsCode(err, "STALE_CONTROL_GENERATION") {
		t.Fatalf("stale generation = %v", err)
	}

	notTakeoverable := ControlCommand{
		RunID: run.ID, ID: "c-no-owner", Kind: Takeover,
	}
	// Release the owner first so no takeoverable owner exists.
	if err := store.ReleaseOwner(ctx, owner, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyControl(ctx, notTakeoverable, now); !IsCode(err, "OWNER_NOT_TAKEOVERABLE") {
		t.Fatalf("not takeoverable = %v", err)
	}

	liveOwner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	ownerActive := ControlCommand{
		RunID: run.ID, ID: "c-active", Kind: Takeover,
	}
	if _, err := store.ApplyControl(ctx, ownerActive, now); !IsCode(err, "OWNER_ACTIVE") {
		t.Fatalf("owner active = %v", err)
	}

	work := digest([]byte("distinct-work"))
	staleEpoch := ControlCommand{
		RunID: run.ID, ID: "c-epoch", Kind: Retry,
		WorkID: work, ExpectedEpoch: 4,
	}
	if _, err := store.ApplyControl(ctx, staleEpoch, now); !IsCode(err, "STALE_RETRY_EPOCH") {
		t.Fatalf("stale epoch = %v", err)
	}

	notExhausted := ControlCommand{
		RunID: run.ID, ID: "c-not-exhausted", Kind: Retry,
		WorkID: work, ExpectedEpoch: 1,
	}
	if _, err := store.ApplyControl(ctx, notExhausted, now); !IsCode(err, "WORK_NOT_EXHAUSTED") {
		t.Fatalf("not exhausted = %v", err)
	}

	if err := store.ReleaseOwner(ctx, liveOwner, now); err != nil {
		t.Fatal(err)
	}
	_ = owner
}
