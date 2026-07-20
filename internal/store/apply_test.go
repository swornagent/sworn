package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
)

const (
	planDigest      = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	authorityDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	dispatchDigest  = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestApplyIsRevisionedIdempotentAndReplaysRejections(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })

	create := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: "delivery-1",
		PlanDigest: planDigest,
		Repository: "repo-1",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-1"},
	})
	created, err := control.Apply(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	if created.Outcome != OutcomeApplied || created.Revision != 0 || created.Replayed {
		t.Fatalf("created = %+v", created)
	}
	replayed, err := control.Apply(ctx, create)
	if err != nil || !replayed.Replayed || replayed.EventID != created.EventID {
		t.Fatalf("replay = %+v, %v", replayed, err)
	}

	conflict := create
	conflict.Payload = json.RawMessage(`{"different":true}`)
	if _, err := control.Apply(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflict error = %v", err)
	}

	stale := testCommand(t, "cmd-stale", engine.CommandActivate, 9, engine.ActivatePayload{
		AuthorityReceiptDigest: authorityDigest,
	})
	rejected, err := control.Apply(ctx, stale)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Outcome != OutcomeRejected || rejected.ErrorCode != "revision_mismatch" || rejected.Revision != 0 {
		t.Fatalf("rejected = %+v", rejected)
	}

	activate := testCommand(t, "cmd-activate", engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: authorityDigest,
	})
	if result, err := control.Apply(ctx, activate); err != nil || result.Revision != 1 {
		t.Fatalf("activate = %+v, %v", result, err)
	}
	rejectedAgain, err := control.Apply(ctx, stale)
	if err != nil || !rejectedAgain.Replayed || rejectedAgain.Revision != 0 {
		t.Fatalf("replayed rejection = %+v, %v", rejectedAgain, err)
	}

	state, err := control.State(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 1 || state.Phase != engine.PhaseActive {
		t.Fatalf("state = %+v", state)
	}
	assertCount(t, control, "commands", 3)
	assertCount(t, control, "events", 2)
	assertCount(t, control, "effects", 0)
}

func TestCommandDigestVectorBindsExactPayload(t *testing.T) {
	t.Parallel()

	command := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: "delivery-1",
		PlanDigest: planDigest,
		Repository: "repo-1",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-1"},
	})
	const want = "sha256:4147c80568c167681e96c2c9a830348e973e1cdfb9575f015162a86b909ea3bf"
	if got := commandDigest(command); got != want {
		t.Fatalf("commandDigest() = %q, want %q", got, want)
	}
	command.Payload = append(command.Payload, ' ')
	if got := commandDigest(command); got == want {
		t.Fatal("command digest did not bind exact payload bytes")
	}
}

func TestCommandEffectTransactionRollsBackCompletely(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	createAndActivate(t, control)

	if _, err := control.db.Exec(`
		CREATE TEMP TRIGGER fail_effect_insert
		BEFORE INSERT ON effects BEGIN
			SELECT RAISE(ABORT, 'injected effect failure');
		END`); err != nil {
		t.Fatal(err)
	}
	dispatch := testCommand(t, "cmd-dispatch", engine.CommandDispatchBuild, 1, engine.DispatchBuildPayload{
		WorkID:         "work-1",
		DispatchDigest: dispatchDigest,
	})
	if _, err := control.Apply(ctx, dispatch); err == nil {
		t.Fatal("Apply succeeded despite injected effect failure")
	}
	state, err := control.State(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 1 || state.Work[0].State != engine.WorkReady {
		t.Fatalf("state changed after rollback: %+v", state)
	}
	assertCount(t, control, "commands", 2)
	assertCount(t, control, "events", 2)
	assertCount(t, control, "effects", 0)

	if _, err := control.db.Exec("DROP TRIGGER fail_effect_insert"); err != nil {
		t.Fatal(err)
	}
	result, err := control.Apply(ctx, dispatch)
	if err != nil || result.Revision != 2 || len(result.EffectIDs) != 1 {
		t.Fatalf("retry = %+v, %v", result, err)
	}
	assertCount(t, control, "commands", 3)
	assertCount(t, control, "events", 3)
	assertCount(t, control, "effects", 1)
}

func TestSQLBoundaryRejectsHistoryMutationAndIllegalEffectTransition(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)

	if _, err := control.db.Exec("UPDATE commands SET kind = 'changed' WHERE command_id = 'cmd-create'"); err == nil {
		t.Fatal("command update bypassed immutability trigger")
	}
	if _, err := control.db.Exec("UPDATE runs SET revision = revision + 2 WHERE run_id = 'run-1'"); err == nil {
		t.Fatal("invalid run revision bypassed transition trigger")
	}
	if _, err := control.db.Exec(`
		UPDATE effects
		SET state = 'succeeded', receipt_json = '{"forged":true}', completed_at_us = 1
		WHERE effect_id = ?`, effectID); err == nil {
		t.Fatal("pending effect jumped directly to succeeded")
	}
	if _, err := control.db.Exec("DELETE FROM effects WHERE effect_id = ?", effectID); err == nil {
		t.Fatal("effect deletion bypassed history trigger")
	}
	if claimed, err := control.ClaimNextEffect(ctx, "worker-1"); err != nil || claimed.Invocation().ID != effectID {
		t.Fatalf("valid claim after rejected mutations = %+v, %v", claimed, err)
	}
}

func TestSecondRunCannotOwnSameRepositoryTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	first := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: "delivery-1",
		PlanDigest: planDigest,
		Repository: "repo-1",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-1"},
	})
	if _, err := control.Apply(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := testCommand(t, "cmd-create-2", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: "delivery-2",
		PlanDigest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		Repository: "repo-1",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-2"},
	})
	second.RunID = "run-2"
	if _, err := control.Apply(ctx, second); err == nil {
		t.Fatal("second non-terminal run acquired busy target")
	}
	if _, err := control.State(ctx, "run-2"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("run-2 state error = %v", err)
	}
	assertCount(t, control, "commands", 1)
	assertCount(t, control, "events", 1)
}

func createAndActivate(t *testing.T, control *Store) {
	t.Helper()
	ctx := context.Background()
	create := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: "delivery-1",
		PlanDigest: planDigest,
		Repository: "repo-1",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-1"},
	})
	if result, err := control.Apply(ctx, create); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("create = %+v, %v", result, err)
	}
	activate := testCommand(t, "cmd-activate", engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: authorityDigest,
	})
	if result, err := control.Apply(ctx, activate); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("activate = %+v, %v", result, err)
	}
}

func createActivateAndDispatch(t *testing.T, control *Store) string {
	t.Helper()
	createAndActivate(t, control)
	dispatch := testCommand(t, "cmd-dispatch", engine.CommandDispatchBuild, 1, engine.DispatchBuildPayload{
		WorkID:         "work-1",
		DispatchDigest: dispatchDigest,
	})
	result, err := control.Apply(context.Background(), dispatch)
	if err != nil || len(result.EffectIDs) != 1 {
		t.Fatalf("dispatch = %+v, %v", result, err)
	}
	return result.EffectIDs[0]
}

func testCommand(t *testing.T, id string, kind engine.CommandKind, revision int64, payload any) engine.Command {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return engine.Command{ID: id, RunID: "run-1", Kind: kind, ExpectedRevision: revision, Payload: encoded}
}

func assertCount(t *testing.T, control *Store, table string, want int) {
	t.Helper()
	var got int
	if err := control.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
