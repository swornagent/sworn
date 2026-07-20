package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

func listEffects(ctx context.Context, control *Store, state EffectState) ([]Effect, error) {
	rows, err := control.db.QueryContext(ctx, `
		SELECT effect_id, run_id, command_id, ordinal, kind, request_json, state,
		       attempt, owner_id, receipt_json, last_error, created_at_us,
		       started_at_us, completed_at_us
		FROM effects WHERE state = ? ORDER BY created_at_us, effect_id`, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var effects []Effect
	for rows.Next() {
		effect, err := scanEffect(rows)
		if err != nil {
			return nil, err
		}
		effects = append(effects, effect)
	}
	return effects, rows.Err()
}

func TestInterruptedUnboundEffectNeverRetriesBlindly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	control := openTestStore(t, path)
	effectID := createActivateAndDispatch(t, control)
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	control = openTestStore(t, path)
	pending, err := listEffects(ctx, control, EffectPending)
	if err != nil || len(pending) != 1 || pending[0].ID != effectID {
		t.Fatalf("pending after reopen = %+v, %v", pending, err)
	}
	claimed, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil || claimed.Invocation().ID != effectID ||
		claimed.Invocation().DeliveryRunID != "run-1" || claimed.Invocation().Attempt != 1 ||
		claimed.Invocation().Kind != engine.EffectBuild {
		t.Fatalf("claimed = %+v, %v", claimed, err)
	}
	request, err := engine.ParseBuildEffectRequest(claimed.Invocation().Request)
	if err != nil || request.DeliveryRunID != "run-1" || request.DeliveryID != "delivery-1" ||
		request.WorkID != "work-1" || request.WorkAttempt != 1 {
		t.Fatalf("claimed request = %+v, %v", request, err)
	}
	if _, err := loadBuildAttemptIdentity(ctx, control.db, claimed.effect); err == nil ||
		!strings.Contains(err.Error(), "predates the durable attempt witness") {
		t.Fatalf("legacy build claim witness error = %v", err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	control = openTestStore(t, path)
	t.Cleanup(func() { _ = control.Close() })
	if _, err := control.claimNextEffectForStoreTest(ctx, "worker-2"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("running effect was claimable after restart: %v", err)
	}
	recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "previous process ended before result binding")
	if err != nil || recovered != 1 {
		t.Fatalf("RecoverInterruptedEffects = %d, %v", recovered, err)
	}
	unknown, err := listEffects(ctx, control, EffectUnknown)
	if err != nil || len(unknown) != 1 {
		t.Fatalf("unknown = %+v, %v", unknown, err)
	}
	if _, err := control.claimNextEffectForStoreTest(ctx, "worker-2"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("unknown effect was claimable: %v", err)
	}

	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, unknown[0].Attempt, "reconciler-1"); err == nil {
		t.Fatal("unbound interrupted effect recovered without external evidence")
	}
	if _, err := control.db.ExecContext(ctx, `
		UPDATE effects SET state = 'pending', owner_id = NULL, started_at_us = NULL,
		last_error = NULL WHERE effect_id = ?`, effectID); err == nil ||
		!strings.Contains(err.Error(), "invalid effect transition") {
		t.Fatalf("SQL boundary admitted unknown retry: %v", err)
	}
	if _, err := control.claimNextEffectForStoreTest(ctx, "worker-3"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("rejected recovery made effect claimable: %v", err)
	}
}

func TestInterruptedLeaseCannotPublishLateResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "worker ownership ended"); err != nil || recovered != 1 {
		t.Fatalf("mark lease unknown = %d, %v", recovered, err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.bindEffectResultForStoreTest(ctx, lease, result); err == nil {
		t.Fatal("interrupted lease bound a late result")
	}
	if err := control.completeEffectForStoreTest(ctx, lease); err == nil {
		t.Fatal("interrupted lease completed late")
	}
	unknown, err := listEffects(ctx, control, EffectUnknown)
	if err != nil || len(unknown) != 1 || len(unknown[0].Result) != 0 {
		t.Fatalf("late worker changed unknown effect: %+v, %v", unknown, err)
	}
}

func TestClaimNextEffectSerializesSameCommandOrdinals(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	firstID := createActivateAndDispatch(t, control)
	secondID := derivedID("eff", "cmd-dispatch", 1)
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json,
			state, attempt, created_at_us
		)
		SELECT ?, run_id, command_id, 1, kind, request_json,
		       'pending', 0, created_at_us
		FROM effects WHERE effect_id = ?`, secondID, firstID); err != nil {
		t.Fatal(err)
	}

	first, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil || first.Invocation().ID != firstID {
		t.Fatalf("first ordinal lease = %q, %v; want %q", first.Invocation().ID, err, firstID)
	}
	if lease, err := control.claimNextEffectForStoreTest(ctx, "worker-2"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("later sibling leased while first was running: %+v, %v", lease, err)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "worker-1 was interrupted"); err != nil || recovered != 1 {
		t.Fatalf("recover first ordinal = %d, %v", recovered, err)
	}
	if lease, err := control.claimNextEffectForStoreTest(ctx, "worker-2"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("later sibling leased while first was unknown: %+v, %v", lease, err)
	}
	if err := control.recoverBoundEffectForStoreTest(ctx, firstID, first.Invocation().Attempt, "reconciler-1"); err == nil {
		t.Fatal("unbound first ordinal recovered")
	}
	if lease, err := control.claimNextEffectForStoreTest(ctx, "worker-3"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("later sibling leased after rejected recovery: %+v, %v", lease, err)
	}
}

func TestClaimPendingBuildScopesAuthorityToExactRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)

	if _, err := control.claimPendingBuildForStoreTest(ctx, "other-run", "builder-worker"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("foreign run claim error = %v", err)
	}
	if _, err := control.claimPendingBuildForStoreTest(ctx, "../run-1", "builder-worker"); err == nil ||
		!strings.Contains(err.Error(), "delivery run id") {
		t.Fatalf("invalid run claim error = %v", err)
	}
	lease, err := control.claimPendingBuildForStoreTest(ctx, "run-1", "builder-worker")
	if err != nil || lease.Invocation().ID != effectID ||
		lease.Invocation().DeliveryRunID != "run-1" || lease.Invocation().Kind != engine.EffectBuild {
		t.Fatalf("scoped build claim = %+v, %v", lease.Invocation(), err)
	}
}

func TestOrphanResultJSONCannotReconcileSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if _, err := control.PutArtifact(ctx, "application/json", result); err != nil {
		t.Fatalf("store orphan result JSON: %v", err)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "result was never bound to the lease"); err != nil || recovered != 1 {
		t.Fatalf("recover = %d, %v", recovered, err)
	}
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err == nil {
		t.Fatal("orphan result artifact reconciled an unbound effect as succeeded")
	}
	unknown, err := listEffects(ctx, control, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != effectID || len(unknown[0].Result) != 0 {
		t.Fatalf("unknown changed after rejected orphan reconciliation = %+v, %v", unknown, err)
	}
}

func TestBoundResultCannotBeDiscardedOrRecoveredWithoutGitTruth(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.bindEffectResultForStoreTest(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if err := control.failEffectForStoreTest(ctx, lease, "runner disconnected after returning a result"); err == nil {
		t.Fatal("infrastructure failure discarded a bound domain result")
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "completion interrupted"); err != nil || recovered != 1 {
		t.Fatalf("recover = %d, %v", recovered, err)
	}
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err == nil ||
		!strings.Contains(err.Error(), "configured repository") {
		t.Fatalf("bound build recovered without Git truth: %v", err)
	}
	unknown, err := listEffects(ctx, control, EffectUnknown)
	if err != nil || len(unknown) != 1 || !bytes.Equal(unknown[0].Result, result) {
		t.Fatalf("failed Git recovery changed bound result: %+v, %v", unknown, err)
	}
}

func TestFailedEffectRequiresCurrentLeaseDetailAndNoBoundResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil || lease.Invocation().ID != effectID {
		t.Fatalf("lease = %+v, %v", lease, err)
	}
	if err := control.failEffectForStoreTest(ctx, EffectLease{}, "failed"); err == nil {
		t.Fatal("zero lease failed effect")
	}
	if err := control.failEffectForStoreTest(ctx, lease, ""); err == nil {
		t.Fatal("failure without detail was accepted")
	}
	if err := control.failEffectForStoreTest(ctx, lease, "runner exited before producing a result"); err != nil {
		t.Fatal(err)
	}
	failed, err := listEffects(ctx, control, EffectFailed)
	if err != nil || len(failed) != 1 || failed[0].LastError != "runner exited before producing a result" || len(failed[0].Result) != 0 {
		t.Fatalf("failed = %+v, %v", failed, err)
	}
}

func TestEffectResultBindingIsIdempotentAndImmutable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.bindEffectResultForStoreTest(ctx, lease, result); err != nil {
		t.Fatalf("initial bind: %v", err)
	}
	if err := control.bindEffectResultForStoreTest(ctx, lease, append(json.RawMessage(nil), result...)); err != nil {
		t.Fatalf("exact idempotent bind: %v", err)
	}
	different := validBuildResult(t, effectID, "sworn-builder/2")
	if bytes.Equal(result, different) {
		t.Fatal("different fixture encoded to identical bytes")
	}
	if err := control.bindEffectResultForStoreTest(ctx, lease, different); err == nil {
		t.Fatal("effect result was rebound to different canonical bytes")
	}
	if err := control.completeEffectForStoreTest(ctx, lease); err != nil {
		t.Fatal(err)
	}
	succeeded, err := listEffects(ctx, control, EffectSucceeded)
	if err != nil || len(succeeded) != 1 || !bytes.Equal(succeeded[0].Result, result) {
		t.Fatalf("immutable result = %+v, %v", succeeded, err)
	}
}

func TestTypedResultTriggerFreezesLifecycleOwnershipAndTiming(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.bindEffectResultForStoreTest(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	assertRejected := func(name, update string) {
		t.Helper()
		if _, err := control.db.ExecContext(ctx, update, effectID); err == nil ||
			!strings.Contains(err.Error(), "invalid effect transition") {
			t.Fatalf("%s lifecycle rewrite error = %v", name, err)
		}
	}
	for name, update := range map[string]string{
		"running owner": `UPDATE effects SET state = 'succeeded', owner_id = 'forged',
			last_error = NULL, completed_at_us = 1 WHERE effect_id = ?`,
		"running start": `UPDATE effects SET state = 'succeeded', started_at_us = started_at_us + 1,
			last_error = NULL, completed_at_us = 1 WHERE effect_id = ?`,
		"running error": `UPDATE effects SET state = 'succeeded', last_error = 'forged',
			completed_at_us = 1 WHERE effect_id = ?`,
		"interruption owner": `UPDATE effects SET state = 'unknown', owner_id = 'forged',
			last_error = 'interrupted' WHERE effect_id = ?`,
		"interruption start": `UPDATE effects SET state = 'unknown', started_at_us = started_at_us + 1,
			last_error = 'interrupted' WHERE effect_id = ?`,
	} {
		assertRejected(name, update)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "worker stopped after binding"); err != nil || recovered != 1 {
		t.Fatalf("recover = %d, %v", recovered, err)
	}
	for name, update := range map[string]string{
		"unknown owner": `UPDATE effects SET state = 'succeeded', owner_id = 'forged',
			last_error = NULL, completed_at_us = 1 WHERE effect_id = ?`,
		"unknown start": `UPDATE effects SET state = 'succeeded', started_at_us = started_at_us + 1,
			last_error = NULL, completed_at_us = 1 WHERE effect_id = ?`,
		"unknown error": `UPDATE effects SET state = 'succeeded', last_error = 'forged',
			completed_at_us = 1 WHERE effect_id = ?`,
	} {
		assertRejected(name, update)
	}
}

func TestEffectLeaseRejectsForeignStoreAndProtectsRequestBytes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	first := openTestStore(t, filepath.Join(t.TempDir(), "first.db"))
	second := openTestStore(t, filepath.Join(t.TempDir(), "second.db"))
	t.Cleanup(func() { _ = first.Close(); _ = second.Close() })
	firstID := createActivateAndDispatch(t, first)
	secondID := createActivateAndDispatch(t, second)
	if firstID != secondID {
		t.Fatalf("fixture effect ids differ: %q != %q", firstID, secondID)
	}
	firstLease, err := first.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	secondLease, err := second.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	request := firstLease.Invocation().Request
	request[0] ^= 0xff
	if !json.Valid(firstLease.Invocation().Request) {
		t.Fatal("caller mutation escaped through lease request accessor")
	}
	result := validBuildResult(t, secondID, "sworn-builder/1")
	if err := second.bindEffectResultForStoreTest(ctx, firstLease, result); err == nil {
		t.Fatal("lease from another store bound an effect result")
	}
	if err := second.completeEffectForStoreTest(ctx, secondLease); err == nil {
		t.Fatal("effect completed without a bound result")
	}
	if err := second.bindEffectResultForStoreTest(ctx, secondLease, result); err != nil {
		t.Fatalf("bind store-owned lease: %v", err)
	}
	if err := second.completeEffectForStoreTest(ctx, secondLease); err != nil {
		t.Fatalf("complete store-owned lease: %v", err)
	}
	if err := second.completeEffectForStoreTest(ctx, secondLease); err == nil {
		t.Fatal("one lease completed the same effect twice")
	}
}

func TestCompletionAndRecoveryRaceConvergesBoundResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository, candidate := atomicAdmissionCandidate(t, false)
	control := openRecoveryTestStore(t, filepath.Join(t.TempDir(), "control.db"), repository)
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatchForRepository(t, control, repository.Binding().RepositoryID)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResultForCandidate(t, effectID, "sworn-builder/1", candidate)
	if err := control.bindEffectResultForStoreTest(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	type recoveryResult struct {
		count int
		err   error
	}
	start := make(chan struct{})
	completion := make(chan error, 1)
	recovery := make(chan recoveryResult, 1)
	go func() {
		<-start
		completion <- control.completeEffectForStoreTest(ctx, lease)
	}()
	go func() {
		<-start
		count, err := control.recoverInterruptedEffectsForStoreTest(ctx, "process ownership changed")
		recovery <- recoveryResult{count: count, err: err}
	}()
	close(start)
	completionErr := <-completion
	recovered := <-recovery
	if recovered.err != nil {
		t.Fatalf("recover race: %v", recovered.err)
	}
	if completionErr == nil {
		if recovered.count != 0 {
			t.Fatalf("completion won but recovery changed %d effects", recovered.count)
		}
	} else {
		if recovered.count != 1 {
			t.Fatalf("completion failed (%v) but recovery changed %d effects", completionErr, recovered.count)
		}
		unknown, err := listEffects(ctx, control, EffectUnknown)
		if err != nil || len(unknown) != 1 {
			t.Fatalf("recovery winner state = %+v, %v", unknown, err)
		}
		if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err != nil {
			t.Fatalf("reconcile recovery winner: %v", err)
		}
	}
	succeeded, err := control.SucceededEffect(ctx, effectID)
	if err != nil || !bytes.Equal(succeeded.Result, result) {
		t.Fatalf("converged result = %+v, %v", succeeded, err)
	}
}

func TestBoundBuildRecoveryRepairsGitAndIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository, candidate := atomicAdmissionCandidate(t, false)
	path := filepath.Join(t.TempDir(), "control.db")
	control := openRecoveryTestStore(t, path, repository)
	effectID := createActivateAndDispatchForRepository(t, control, repository.Binding().RepositoryID)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResultForCandidate(t, effectID, "sworn-builder/1", candidate)
	if err := control.bindEffectResultForStoreTest(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "worker exited after binding"); err != nil || recovered != 1 {
		t.Fatalf("mark bound build unknown = %d, %v", recovered, err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	runAtomicAdmissionGit(t, repository.Root(), "update-ref", "-d", candidate.Ref)

	control = openRecoveryTestStore(t, path, repository)
	t.Cleanup(func() { _ = control.Close() })
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt+1, "reconciler-1"); err == nil {
		t.Fatal("stale attempt recovered bound build")
	}
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err != nil {
		t.Fatalf("recover bound build: %v", err)
	}
	if got := strings.TrimSpace(runAtomicAdmissionGit(t, repository.Root(), "rev-parse", candidate.Ref)); got != candidate.Commit {
		t.Fatalf("repaired candidate ref = %s, want %s", got, candidate.Commit)
	}
	assertCount(t, control, "effect_observations", 3)
	runAtomicAdmissionGit(t, repository.Root(), "update-ref", "-d", candidate.Ref)
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-2"); err != nil {
		t.Fatalf("idempotent recovery replay: %v", err)
	}
	if got := strings.TrimSpace(runAtomicAdmissionGit(t, repository.Root(), "rev-parse", candidate.Ref)); got != candidate.Commit {
		t.Fatalf("replay-repaired candidate ref = %s, want %s", got, candidate.Commit)
	}
	assertCount(t, control, "effect_observations", 3)
	succeeded, err := control.SucceededEffect(ctx, effectID)
	if err != nil || !bytes.Equal(succeeded.Result, result) {
		t.Fatalf("recovered build result = %+v, %v", succeeded, err)
	}
}

func TestBoundBuildRecoveryFailureNeverProjectsSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository, candidate := atomicAdmissionCandidate(t, false)
	control := openRecoveryTestStore(t, filepath.Join(t.TempDir(), "control.db"), repository)
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatchForRepository(t, control, repository.Binding().RepositoryID)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResultForCandidate(t, effectID, "sworn-builder/1", candidate)
	if err := control.bindEffectResultForStoreTest(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "completion interrupted"); err != nil || recovered != 1 {
		t.Fatalf("mark build unknown = %d, %v", recovered, err)
	}
	runAtomicAdmissionGit(t, repository.Root(), "update-ref", "-d", candidate.Ref)
	if _, err := control.db.ExecContext(ctx, `
		CREATE TEMP TRIGGER fail_bound_recovery_observation
		BEFORE INSERT ON effect_observations WHEN NEW.kind = 'succeeded' BEGIN
			SELECT RAISE(ABORT, 'injected recovery persistence failure');
		END`); err != nil {
		t.Fatal(err)
	}
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err == nil {
		t.Fatal("bound recovery survived injected persistence failure")
	}
	unknown, err := listEffects(ctx, control, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != effectID {
		t.Fatalf("failed recovery projected success: %+v, %v", unknown, err)
	}
	assertCount(t, control, "effect_observations", 2)
	if _, err := control.db.ExecContext(ctx, "DROP TRIGGER fail_bound_recovery_observation"); err != nil {
		t.Fatal(err)
	}
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err != nil {
		t.Fatalf("retry recovery after persistence restored: %v", err)
	}
}

func TestBoundBuildRecoveryRejectsCandidateRefCollision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository, candidate := atomicAdmissionCandidate(t, false)
	control := openRecoveryTestStore(t, filepath.Join(t.TempDir(), "control.db"), repository)
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatchForRepository(t, control, repository.Binding().RepositoryID)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := control.bindEffectResultForStoreTest(
		ctx, lease, validBuildResultForCandidate(t, effectID, "sworn-builder/1", candidate),
	); err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "completion interrupted"); err != nil || recovered != 1 {
		t.Fatalf("mark build unknown = %d, %v", recovered, err)
	}
	runAtomicAdmissionGit(t, repository.Root(), "update-ref", candidate.Ref, candidate.BaseCommit, candidate.Commit)
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err == nil ||
		!strings.Contains(err.Error(), "candidate ref collision") {
		t.Fatalf("candidate ref collision recovery error = %v", err)
	}
	unknown, err := listEffects(ctx, control, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != effectID {
		t.Fatalf("collision changed unknown effect: %+v, %v", unknown, err)
	}
}

func TestBoundBuildRecoveryRejectsClaimedGitFactMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository, candidate := atomicAdmissionCandidate(t, false)
	control := openRecoveryTestStore(t, filepath.Join(t.TempDir(), "control.db"), repository)
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatchForRepository(t, control, repository.Binding().RepositoryID)
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	candidate.ChangedPaths = []string{"forged.txt"}
	if err := control.bindEffectResultForStoreTest(
		ctx, lease, validBuildResultForCandidate(t, effectID, "sworn-builder/1", candidate),
	); err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "completion interrupted"); err != nil || recovered != 1 {
		t.Fatalf("mark build unknown = %d, %v", recovered, err)
	}
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err == nil ||
		!strings.Contains(err.Error(), "changed paths mismatch") {
		t.Fatalf("claimed Git fact mismatch recovery error = %v", err)
	}
	unknown, err := listEffects(ctx, control, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != effectID {
		t.Fatalf("Git fact mismatch changed unknown effect: %+v, %v", unknown, err)
	}
}

func TestBoundBuildRecoveryRejectsDeliveryRepositoryMismatchBeforeGit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository, candidate := atomicAdmissionCandidate(t, false)
	control := openRecoveryTestStore(t, filepath.Join(t.TempDir(), "control.db"), repository)
	t.Cleanup(func() { _ = control.Close() })
	configuredBuilder := control.builderDispatchDigest
	control.builderDispatchDigest = ""
	effectID := createActivateAndDispatch(t, control)
	control.builderDispatchDigest = configuredBuilder
	lease, err := control.claimNextEffectForStoreTest(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := control.bindEffectResultForStoreTest(
		ctx, lease, validBuildResultForCandidate(t, effectID, "sworn-builder/1", candidate),
	); err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.recoverInterruptedEffectsForStoreTest(ctx, "completion interrupted"); err != nil || recovered != 1 {
		t.Fatalf("mark build unknown = %d, %v", recovered, err)
	}
	runAtomicAdmissionGit(t, repository.Root(), "update-ref", candidate.Ref, candidate.BaseCommit, candidate.Commit)
	if err := control.recoverBoundEffectForStoreTest(ctx, effectID, lease.Invocation().Attempt, "reconciler-1"); err == nil ||
		!strings.Contains(err.Error(), "delivery repository and target") {
		t.Fatalf("delivery repository mismatch recovery error = %v", err)
	}
}

func validBuildResult(t *testing.T, effectID, agent string) json.RawMessage {
	t.Helper()
	commit := strings.Repeat("c", 40)
	return validBuildResultForCandidate(t, effectID, agent, repo.Candidate{
		RepositoryID: "repo-1", TargetRef: "refs/heads/main", BaseCommit: strings.Repeat("a", 40),
		BaseTree: strings.Repeat("b", 40), Commit: commit, Tree: strings.Repeat("d", 40),
		Ref: "refs/sworn/v1/candidates/" + commit, ChangedPaths: []string{"README.md"},
	})
}

func validBuildResultForCandidate(
	t *testing.T,
	effectID string,
	agent string,
	candidate repo.Candidate,
) json.RawMessage {
	t.Helper()
	result, err := engine.EncodeBuildEffectResult(engine.BuildEffectResult{
		SchemaVersion: engine.BuildEffectResultSchemaVersion,
		Outcome:       engine.BuildOutcomeCandidateReady,
		Builder: protocol.BuilderRun{
			RunID: effectID, Agent: agent, StartedAt: "2026-07-20T00:00:00Z",
			CompletedAt: "2026-07-20T00:00:01.000000001Z",
		},
		Candidate: candidate,
	})
	if err != nil {
		t.Fatalf("encode build effect result: %v", err)
	}
	return result
}

func openRecoveryTestStore(t *testing.T, path string, repository *repo.Repository) *Store {
	t.Helper()
	control, err := OpenConfigured(context.Background(), path, ControlConfiguration{
		LocalCheckRuntimeManifestDigest: "sha256:" + strings.Repeat("e", 64),
		BuilderDispatchDigest:           dispatchDigest,
		Repository:                      repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	return control
}

func createActivateAndDispatchForRepository(t *testing.T, control *Store, repositoryID string) string {
	t.Helper()
	create := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: "delivery-1", PlanDigest: planDigest, Repository: repositoryID,
		TargetRef: "refs/heads/main", Work: []string{"work-1"},
	})
	if result, err := control.Apply(context.Background(), create); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("create = %+v, %v", result, err)
	}
	activate := testCommand(t, "cmd-activate", engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: authorityDigest,
	})
	if result, err := control.Apply(context.Background(), activate); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("activate = %+v, %v", result, err)
	}
	configuredBuilder := control.builderDispatchDigest
	control.builderDispatchDigest = ""
	dispatch := testCommand(t, "cmd-dispatch", engine.CommandDispatchBuild, 1, engine.DispatchBuildPayload{
		WorkID: "work-1", DispatchDigest: dispatchDigest,
	})
	result, err := control.applyCommand(context.Background(), dispatch, nil)
	control.builderDispatchDigest = configuredBuilder
	if err != nil || len(result.EffectIDs) != 1 {
		t.Fatalf("dispatch = %+v, %v", result, err)
	}
	return result.EffectIDs[0]
}
