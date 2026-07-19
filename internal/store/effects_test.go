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

func TestPendingRunningUnknownReconciliationNeverRetriesBlindly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	control := openTestStore(t, path)
	effectID := createActivateAndDispatch(t, control)
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	control = openTestStore(t, path)
	pending, err := control.Effects(ctx, EffectPending)
	if err != nil || len(pending) != 1 || pending[0].ID != effectID {
		t.Fatalf("pending after reopen = %+v, %v", pending, err)
	}
	claimed, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil || claimed.EffectID() != effectID ||
		claimed.DeliveryRunID() != "run-1" || claimed.Attempt() != 1 || claimed.Kind() != string(engine.EffectBuild) {
		t.Fatalf("claimed = %+v, %v", claimed, err)
	}
	request, err := engine.ParseBuildEffectRequest(claimed.Request())
	if err != nil || request.DeliveryRunID != "run-1" || request.DeliveryID != "delivery-1" ||
		request.WorkID != "work-1" || request.WorkAttempt != 1 {
		t.Fatalf("claimed request = %+v, %v", request, err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	control = openTestStore(t, path)
	t.Cleanup(func() { _ = control.Close() })
	if _, err := control.ClaimNextEffect(ctx, "worker-2"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("running effect was claimable after restart: %v", err)
	}
	recovered, err := control.RecoverInterruptedEffects(ctx, "previous process ended before result binding")
	if err != nil || recovered != 1 {
		t.Fatalf("RecoverInterruptedEffects = %d, %v", recovered, err)
	}
	unknown, err := control.Effects(ctx, EffectUnknown)
	if err != nil || len(unknown) != 1 {
		t.Fatalf("unknown = %+v, %v", unknown, err)
	}
	if _, err := control.ClaimNextEffect(ctx, "worker-2"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("unknown effect was claimable: %v", err)
	}

	if err := control.ReconcileUnknownEffect(
		ctx, effectID, unknown[0].Attempt, "reconciler-1", ReconcileNotApplied,
		"external system proves no invocation",
	); err != nil {
		t.Fatal(err)
	}
	claimedAgain, err := control.ClaimNextEffect(ctx, "worker-2")
	if err != nil || claimedAgain.Attempt() != 2 {
		t.Fatalf("second claim = %+v, %v", claimedAgain, err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, claimedAgain, result); err != nil {
		t.Fatalf("bind result: %v", err)
	}
	if err := control.CompleteEffect(ctx, claimedAgain); err != nil {
		t.Fatal(err)
	}
	succeeded, err := control.Effects(ctx, EffectSucceeded)
	if err != nil || len(succeeded) != 1 || string(succeeded[0].Result) != string(result) {
		t.Fatalf("succeeded = %+v, %v", succeeded, err)
	}
	assertCount(t, control, "effect_observations", 5)
	if _, err := control.ClaimNextEffect(ctx, "worker-3"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("completed effect was claimable: %v", err)
	}
}

func TestBoundResultSurvivesUnknownRecoveryAndReopen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	control := openTestStore(t, path)
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.RecoverInterruptedEffects(ctx, "worker exited after binding result"); err != nil || recovered != 1 {
		t.Fatalf("recover bound result = %d, %v", recovered, err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	control = openTestStore(t, path)
	t.Cleanup(func() { _ = control.Close() })
	unknown, err := control.Effects(ctx, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != effectID || !bytes.Equal(unknown[0].Result, result) {
		t.Fatalf("unknown after reopen = %+v, %v", unknown, err)
	}
	if err := control.ReconcileUnknownEffect(ctx, effectID, lease.Attempt(), "reconciler-1", ReconcileSucceeded, ""); err != nil {
		t.Fatalf("reconcile bound result: %v", err)
	}
	succeeded, err := control.SucceededEffect(ctx, effectID)
	if err != nil || succeeded.Attempt != lease.Attempt() || !bytes.Equal(succeeded.Result, result) {
		t.Fatalf("succeeded journal result = %+v, %v", succeeded, err)
	}
	rows, err := control.Effects(ctx, EffectSucceeded)
	if err != nil || len(rows) != 1 || !bytes.Equal(rows[0].Result, result) {
		t.Fatalf("succeeded effect = %+v, %v", rows, err)
	}
}

func TestOrphanResultJSONCannotReconcileSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if _, err := control.PutArtifact(ctx, "application/json", result); err != nil {
		t.Fatalf("store orphan result JSON: %v", err)
	}
	if recovered, err := control.RecoverInterruptedEffects(ctx, "result was never bound to the lease"); err != nil || recovered != 1 {
		t.Fatalf("recover = %d, %v", recovered, err)
	}
	if err := control.ReconcileUnknownEffect(ctx, effectID, lease.Attempt(), "reconciler-1", ReconcileSucceeded, ""); err == nil {
		t.Fatal("orphan result artifact reconciled an unbound effect as succeeded")
	}
	unknown, err := control.Effects(ctx, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != effectID || len(unknown[0].Result) != 0 {
		t.Fatalf("unknown changed after rejected orphan reconciliation = %+v, %v", unknown, err)
	}
}

func TestBoundResultRejectsFailureAndNonSuccessReconciliation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if err := control.FailEffect(ctx, lease, "runner disconnected after returning a result"); err == nil {
		t.Fatal("infrastructure failure discarded a bound domain result")
	}
	if recovered, err := control.RecoverInterruptedEffects(ctx, "completion interrupted"); err != nil || recovered != 1 {
		t.Fatalf("recover = %d, %v", recovered, err)
	}
	if err := control.ReconcileUnknownEffect(
		ctx, effectID, lease.Attempt(), "reconciler-1", ReconcileNotApplied, "invocation did not start",
	); err == nil {
		t.Fatal("bound result reconciled as not applied")
	}
	if err := control.ReconcileUnknownEffect(
		ctx, effectID, lease.Attempt(), "reconciler-1", ReconcileFailed, "infrastructure failure",
	); err == nil {
		t.Fatal("bound result reconciled as failed")
	}
	if err := control.ReconcileUnknownEffect(ctx, effectID, lease.Attempt(), "reconciler-1", ReconcileSucceeded, ""); err != nil {
		t.Fatalf("reconcile bound success: %v", err)
	}
}

func TestFailedEffectRequiresCurrentLeaseDetailAndNoBoundResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil || lease.EffectID() != effectID {
		t.Fatalf("lease = %+v, %v", lease, err)
	}
	if err := control.FailEffect(ctx, EffectLease{}, "failed"); err == nil {
		t.Fatal("zero lease failed effect")
	}
	if err := control.FailEffect(ctx, lease, ""); err == nil {
		t.Fatal("failure without detail was accepted")
	}
	if err := control.FailEffect(ctx, lease, "runner exited before producing a result"); err != nil {
		t.Fatal(err)
	}
	failed, err := control.Effects(ctx, EffectFailed)
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
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, lease, result); err != nil {
		t.Fatalf("initial bind: %v", err)
	}
	if err := control.BindEffectResult(ctx, lease, append(json.RawMessage(nil), result...)); err != nil {
		t.Fatalf("exact idempotent bind: %v", err)
	}
	different := validBuildResult(t, effectID, "sworn-builder/2")
	if bytes.Equal(result, different) {
		t.Fatal("different fixture encoded to identical bytes")
	}
	if err := control.BindEffectResult(ctx, lease, different); err == nil {
		t.Fatal("effect result was rebound to different canonical bytes")
	}
	if err := control.CompleteEffect(ctx, lease); err != nil {
		t.Fatal(err)
	}
	succeeded, err := control.Effects(ctx, EffectSucceeded)
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
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, lease, result); err != nil {
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
	if recovered, err := control.RecoverInterruptedEffects(ctx, "worker stopped after binding"); err != nil || recovered != 1 {
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
	if err := control.ReconcileUnknownEffect(
		ctx, effectID, lease.Attempt(), "reconciler-1", ReconcileSucceeded, "",
	); err != nil {
		t.Fatalf("legitimate reconciliation after rejected rewrites: %v", err)
	}
}

func TestEffectLeaseRejectsStaleAttemptAfterSameOwnerReclaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	stale, err := control.ClaimNextEffect(ctx, "worker-reused")
	if err != nil || stale.Attempt() != 1 {
		t.Fatalf("first lease = %+v, %v", stale, err)
	}
	if recovered, err := control.RecoverInterruptedEffects(ctx, "worker disappeared"); err != nil || recovered != 1 {
		t.Fatalf("recover = %d, %v", recovered, err)
	}
	if err := control.ReconcileUnknownEffect(
		ctx, effectID, stale.Attempt(), "reconciler-1", ReconcileNotApplied, "no process started",
	); err != nil {
		t.Fatal(err)
	}
	current, err := control.ClaimNextEffect(ctx, "worker-reused")
	if err != nil || current.Attempt() != 2 {
		t.Fatalf("current lease = %+v, %v", current, err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, stale, result); err == nil {
		t.Fatal("stale same-owner lease bound a result to a later attempt")
	}
	if err := control.CompleteEffect(ctx, stale); err == nil {
		t.Fatal("stale same-owner lease completed a later attempt")
	}
	running, err := control.Effects(ctx, EffectRunning)
	if err != nil || len(running) != 1 || running[0].ID != effectID || running[0].Attempt != 2 || len(running[0].Result) != 0 {
		t.Fatalf("later attempt changed after stale operation: %+v, %v", running, err)
	}
	if err := control.BindEffectResult(ctx, current, result); err != nil {
		t.Fatalf("bind current lease: %v", err)
	}
	if err := control.CompleteEffect(ctx, current); err != nil {
		t.Fatalf("complete current lease: %v", err)
	}
}

func TestReconciliationRejectsObservationFromEarlierAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	first, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.RecoverInterruptedEffects(ctx, "attempt one interrupted"); err != nil || recovered != 1 {
		t.Fatalf("recover attempt one = %d, %v", recovered, err)
	}
	if err := control.ReconcileUnknownEffect(
		ctx, effectID, first.Attempt(), "reconciler-1", ReconcileNotApplied, "attempt one did not start",
	); err != nil {
		t.Fatal(err)
	}
	second, err := control.ClaimNextEffect(ctx, "worker-2")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, second, result); err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.RecoverInterruptedEffects(ctx, "attempt two interrupted"); err != nil || recovered != 1 {
		t.Fatalf("recover attempt two = %d, %v", recovered, err)
	}
	if err := control.ReconcileUnknownEffect(ctx, effectID, first.Attempt(), "reconciler-stale", ReconcileSucceeded, ""); err == nil {
		t.Fatal("observation from attempt one resolved unknown attempt two")
	}
	unknown, err := control.Effects(ctx, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].Attempt != second.Attempt() {
		t.Fatalf("later unknown attempt changed after stale reconciliation: %+v, %v", unknown, err)
	}
	if err := control.ReconcileUnknownEffect(ctx, effectID, second.Attempt(), "reconciler-current", ReconcileSucceeded, ""); err != nil {
		t.Fatalf("reconcile current attempt: %v", err)
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
	firstLease, err := first.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	secondLease, err := second.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	request := firstLease.Request()
	request[0] ^= 0xff
	if !json.Valid(firstLease.Request()) {
		t.Fatal("caller mutation escaped through lease request accessor")
	}
	result := validBuildResult(t, secondID, "sworn-builder/1")
	if err := second.BindEffectResult(ctx, firstLease, result); err == nil {
		t.Fatal("lease from another store bound an effect result")
	}
	if err := second.CompleteEffect(ctx, secondLease); err == nil {
		t.Fatal("effect completed without a bound result")
	}
	if err := second.BindEffectResult(ctx, secondLease, result); err != nil {
		t.Fatalf("bind store-owned lease: %v", err)
	}
	if err := second.CompleteEffect(ctx, secondLease); err != nil {
		t.Fatalf("complete store-owned lease: %v", err)
	}
	if err := second.CompleteEffect(ctx, secondLease); err == nil {
		t.Fatal("one lease completed the same effect twice")
	}
}

func TestCompletionAndRecoveryRaceConvergesBoundResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	result := validBuildResult(t, effectID, "sworn-builder/1")
	if err := control.BindEffectResult(ctx, lease, result); err != nil {
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
		completion <- control.CompleteEffect(ctx, lease)
	}()
	go func() {
		<-start
		count, err := control.RecoverInterruptedEffects(ctx, "process ownership changed")
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
		unknown, err := control.Effects(ctx, EffectUnknown)
		if err != nil || len(unknown) != 1 {
			t.Fatalf("recovery winner state = %+v, %v", unknown, err)
		}
		if err := control.ReconcileUnknownEffect(
			ctx, effectID, lease.Attempt(), "reconciler-1", ReconcileSucceeded, "",
		); err != nil {
			t.Fatalf("reconcile recovery winner: %v", err)
		}
	}
	succeeded, err := control.SucceededEffect(ctx, effectID)
	if err != nil || !bytes.Equal(succeeded.Result, result) {
		t.Fatalf("converged result = %+v, %v", succeeded, err)
	}
}

func validBuildResult(t *testing.T, effectID, agent string) json.RawMessage {
	t.Helper()
	commit := strings.Repeat("c", 40)
	result, err := engine.EncodeBuildEffectResult(engine.BuildEffectResult{
		SchemaVersion: engine.BuildEffectResultSchemaVersion,
		Outcome:       engine.BuildOutcomeCandidateReady,
		Builder: protocol.BuilderRun{
			RunID: effectID, Agent: agent, StartedAt: "2026-07-20T00:00:00Z",
			CompletedAt: "2026-07-20T00:00:01.000000001Z",
		},
		Candidate: repo.Candidate{
			RepositoryID: "repo-1", TargetRef: "refs/heads/main", BaseCommit: strings.Repeat("a", 40),
			BaseTree: strings.Repeat("b", 40), Commit: commit, Tree: strings.Repeat("d", 40),
			Ref: "refs/sworn/v1/candidates/" + commit, ChangedPaths: []string{"README.md"},
		},
	})
	if err != nil {
		t.Fatalf("encode build effect result: %v", err)
	}
	return result
}
