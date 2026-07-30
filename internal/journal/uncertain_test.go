package journal

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func uncertainEffectFixture(
	t *testing.T,
) (*Store, Run, Effect, OwnerLease, time.Time) {
	t.Helper()
	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimOwned(ctx, owner, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileOwned(ctx, owner, Completion{
		RunID:     run.ID,
		EffectID:  effect.ID,
		Token:     claim.Token,
		EventKind: "effect_uncertain",
		At:        now.Add(time.Second),
	}, RecoveryAmbiguous); err != nil {
		t.Fatal(err)
	}
	return store, run, effect, owner, now.Add(2 * time.Second)
}

func addUncertainEffect(
	t *testing.T,
	store *Store,
	run Run,
	template Effect,
	owner OwnerLease,
	effectID string,
	at time.Time,
) Effect {
	t.Helper()
	command := Command{
		RunID:     run.ID,
		ReplayKey: effectID,
		Kind:      template.Kind,
		Payload:   []byte("payload"),
		CreatedAt: at,
	}
	if err := store.RecordCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	effect := template
	effect.ID = effectID
	effect.ReplayKey = effectID
	effect.UpdatedAt = at
	if err := store.EnsureEffect(context.Background(), effect); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimOwned(
		context.Background(),
		owner,
		effect.ID,
		at,
		time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileOwned(
		context.Background(),
		owner,
		Completion{
			RunID:     run.ID,
			EffectID:  effect.ID,
			Token:     claim.Token,
			EventKind: "effect_uncertain",
			At:        at,
		},
		RecoveryAmbiguous,
	); err != nil {
		t.Fatal(err)
	}
	return effect
}

func TestResolveUncertainOwnedPersistsTerminalFailureAndEvent(t *testing.T) {
	t.Parallel()

	store, run, effect, owner, at := uncertainEffectFixture(t)
	ctx := context.Background()
	if err := store.ResolveUncertainOwned(
		ctx,
		owner,
		run.ID,
		effect.ID,
		"stale_authority",
		at,
	); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.Effect(ctx, run.ID, effect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.State != OperationalFailed ||
		resolved.CurrentClaim != "" ||
		resolved.ErrorCode != "stale_authority" ||
		resolved.ResultDigest != digest(nil) ||
		len(resolved.Result) != 0 ||
		!resolved.UpdatedAt.Equal(at) {
		t.Fatalf("resolved effect = %#v", resolved)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	for _, event := range snapshot.Events {
		if event.Kind == uncertaintyResolvedEvent {
			events = append(events, event)
		}
	}
	if len(events) != 1 ||
		!bytes.Equal(events[0].Body, []byte(effect.ID)) ||
		!events[0].CreatedAt.Equal(at) {
		t.Fatalf("resolution events = %#v", events)
	}
}

func TestResolveUncertainManyOwnedPersistsOneEventPerEffect(t *testing.T) {
	t.Parallel()

	store, run, first, owner, at := uncertainEffectFixture(t)
	second := addUncertainEffect(
		t,
		store,
		run,
		first,
		owner,
		"effect-2",
		at,
	)
	ctx := context.Background()
	if err := store.ResolveUncertainManyOwned(
		ctx,
		owner,
		run.ID,
		[]string{first.ID, second.ID},
		"stale_authority",
		at.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	for _, effectID := range []string{first.ID, second.ID} {
		resolved, err := store.Effect(ctx, run.ID, effectID)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.State != OperationalFailed ||
			resolved.ErrorCode != "stale_authority" {
			t.Fatalf("resolved %s = %#v", effectID, resolved)
		}
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var bodies [][]byte
	for _, event := range snapshot.Events {
		if event.Kind == uncertaintyResolvedEvent {
			bodies = append(bodies, event.Body)
		}
	}
	if len(bodies) != 2 ||
		!bytes.Equal(bodies[0], []byte(first.ID)) ||
		!bytes.Equal(bodies[1], []byte(second.ID)) {
		t.Fatalf("resolution event bodies = %q", bodies)
	}
}

func TestRearmUncertainManyOwnedAtomicallyRestoresPendingEffects(t *testing.T) {
	t.Parallel()

	store, run, first, owner, at := uncertainEffectFixture(t)
	second := addUncertainEffect(
		t,
		store,
		run,
		first,
		owner,
		"effect-2",
		at,
	)
	ctx := context.Background()
	rearmedAt := at.Add(time.Second)
	if err := store.RearmUncertainManyOwned(
		ctx,
		owner,
		run.ID,
		[]string{first.ID, second.ID},
		rearmedAt,
	); err != nil {
		t.Fatal(err)
	}
	for _, effectID := range []string{first.ID, second.ID} {
		rearmed, err := store.Effect(ctx, run.ID, effectID)
		if err != nil {
			t.Fatal(err)
		}
		if rearmed.State != Pending ||
			rearmed.CurrentClaim != "" ||
			rearmed.ErrorCode != "" ||
			rearmed.ResultDigest != digest(nil) ||
			len(rearmed.Result) != 0 ||
			!rearmed.UpdatedAt.Equal(rearmedAt) {
			t.Fatalf("rearmed %s = %#v", effectID, rearmed)
		}
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var bodies [][]byte
	for _, event := range snapshot.Events {
		if event.Kind == uncertaintyRearmedEvent {
			bodies = append(bodies, event.Body)
		}
	}
	if len(bodies) != 2 ||
		!bytes.Equal(bodies[0], []byte(first.ID)) ||
		!bytes.Equal(bodies[1], []byte(second.ID)) {
		t.Fatalf("rearm event bodies = %q", bodies)
	}
}

func TestRearmUncertainManyOwnedRollsBackOnWrongState(t *testing.T) {
	t.Parallel()

	store, run, uncertain, owner, at := uncertainEffectFixture(t)
	ctx := context.Background()
	pendingID := "effect-pending"
	if err := store.RecordCommand(ctx, Command{
		RunID: run.ID, ReplayKey: pendingID, Kind: uncertain.Kind,
		Payload: []byte("pending"), CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	pending := uncertain
	pending.ID = pendingID
	pending.ReplayKey = pendingID
	pending.UpdatedAt = at
	if err := store.EnsureEffect(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if err := store.RearmUncertainManyOwned(
		ctx,
		owner,
		run.ID,
		[]string{uncertain.ID, pending.ID},
		at,
	); !IsCode(err, "EFFECT_NOT_UNCERTAIN") {
		t.Fatalf("mixed-state rearm = %v", err)
	}
	for effectID, want := range map[string]EffectState{
		uncertain.ID: Uncertain,
		pending.ID:   Pending,
	} {
		got, err := store.Effect(ctx, run.ID, effectID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != want {
			t.Fatalf("rolled back %s = %#v", effectID, got)
		}
	}
}

func TestResolveUncertainOwnedIsOwnerFenced(t *testing.T) {
	t.Parallel()

	store, run, effect, owner, at := uncertainEffectFixture(t)
	ctx := context.Background()
	tests := []struct {
		name  string
		owner OwnerLease
		runID string
	}{
		{
			name: "substituted token",
			owner: OwnerLease{
				RunID:      owner.RunID,
				Token:      strings.Repeat("f", 64),
				Generation: owner.Generation,
				ExpiresAt:  owner.ExpiresAt,
			},
			runID: run.ID,
		},
		{
			name:  "substituted run",
			owner: owner,
			runID: "run-2",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := store.ResolveUncertainOwned(
				ctx,
				test.owner,
				test.runID,
				effect.ID,
				"stale_authority",
				at,
			); !IsCode(err, "OWNER_FENCED") {
				t.Fatalf("fenced resolution = %v", err)
			}
			got, err := store.Effect(ctx, run.ID, effect.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != Uncertain || got.ErrorCode != "" {
				t.Fatalf("fenced resolution mutated effect = %#v", got)
			}
		})
	}
}

func TestResolveUncertainManyOwnedRollsBackOnWrongState(t *testing.T) {
	t.Parallel()

	store, run, uncertain, owner, at := uncertainEffectFixture(t)
	ctx := context.Background()
	pendingID := "effect-pending"
	command := Command{
		RunID: run.ID, ReplayKey: pendingID, Kind: uncertain.Kind,
		Payload: []byte("pending"), CreatedAt: at,
	}
	if err := store.RecordCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	pending := uncertain
	pending.ID = pendingID
	pending.ReplayKey = pendingID
	pending.UpdatedAt = at
	if err := store.EnsureEffect(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveUncertainManyOwned(
		ctx,
		owner,
		run.ID,
		[]string{uncertain.ID, pending.ID},
		"stale_authority",
		at,
	); !IsCode(err, "EFFECT_NOT_UNCERTAIN") {
		t.Fatalf("mixed-state resolution = %v", err)
	}
	for effectID, want := range map[string]EffectState{
		uncertain.ID: Uncertain,
		pending.ID:   Pending,
	} {
		got, err := store.Effect(ctx, run.ID, effectID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != want || got.ErrorCode != "" {
			t.Fatalf("rolled back %s = %#v", effectID, got)
		}
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range snapshot.Events {
		if event.Kind == uncertaintyResolvedEvent {
			t.Fatalf("mixed-state resolution persisted event %#v", event)
		}
	}
}

func TestResolveUncertainOwnedRejectsWrongStateAndInvalidIdentity(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	at := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, at, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveUncertainOwned(
		ctx,
		owner,
		run.ID,
		effect.ID,
		"stale_authority",
		at,
	); !IsCode(err, "EFFECT_NOT_UNCERTAIN") {
		t.Fatalf("pending effect resolution = %v", err)
	}
	if err := store.ResolveUncertainOwned(
		ctx,
		owner,
		run.ID,
		"",
		"stale_authority",
		at,
	); !IsCode(err, "INVALID_EFFECT") {
		t.Fatalf("invalid effect identity = %v", err)
	}
	if err := store.ResolveUncertainOwned(
		ctx,
		owner,
		run.ID,
		effect.ID,
		"",
		at,
	); !IsCode(err, "INVALID_ERROR_CODE") {
		t.Fatalf("invalid error code = %v", err)
	}
	got, err := store.Effect(ctx, run.ID, effect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Pending || got.ErrorCode != "" {
		t.Fatalf("rejected resolutions mutated effect = %#v", got)
	}
	if err := store.ResolveUncertainManyOwned(
		ctx,
		owner,
		run.ID,
		nil,
		"stale_authority",
		at,
	); !IsCode(err, "INVALID_RECOVERY") {
		t.Fatalf("empty batch = %v", err)
	}
	if err := store.ResolveUncertainManyOwned(
		ctx,
		owner,
		run.ID,
		[]string{effect.ID, effect.ID},
		"stale_authority",
		at,
	); !IsCode(err, "INVALID_RECOVERY") {
		t.Fatalf("duplicate batch = %v", err)
	}
	tooMany := make([]string, MaxUncertainResolutionEffects+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("effect-%d", index)
	}
	if err := store.ResolveUncertainManyOwned(
		ctx,
		owner,
		run.ID,
		tooMany,
		"stale_authority",
		at,
	); !IsCode(err, "INVALID_RECOVERY") {
		t.Fatalf("oversized batch = %v", err)
	}
}

func TestResolveUncertainManyOwnedRollsBackWhenEventCannotPersist(t *testing.T) {
	t.Parallel()

	store, run, first, owner, at := uncertainEffectFixture(t)
	second := addUncertainEffect(
		t,
		store,
		run,
		first,
		owner,
		"effect-2",
		at,
	)
	ctx := context.Background()
	if _, err := store.conn.ExecContext(
		ctx,
		`CREATE TEMP TRIGGER reject_uncertainty_resolution
		 BEFORE INSERT ON events
		 WHEN NEW.kind = 'effect_uncertainty_resolved'
		  AND CAST(NEW.body AS TEXT) = 'effect-2'
		 BEGIN
		   SELECT RAISE(ABORT, 'blocked');
		 END`,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveUncertainManyOwned(
		ctx,
		owner,
		run.ID,
		[]string{first.ID, second.ID},
		"stale_authority",
		at,
	); !IsCode(err, "DATABASE_FAILED") {
		t.Fatalf("event failure = %v", err)
	}
	for _, effectID := range []string{first.ID, second.ID} {
		got, err := store.Effect(ctx, run.ID, effectID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != Uncertain ||
			got.CurrentClaim != "" ||
			got.ErrorCode != "" ||
			got.ResultDigest != "" ||
			len(got.Result) != 0 {
			t.Fatalf("event rollback left partial %s = %#v", effectID, got)
		}
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range snapshot.Events {
		if event.Kind == uncertaintyResolvedEvent {
			t.Fatalf("event rollback persisted partial event %#v", event)
		}
	}
}
