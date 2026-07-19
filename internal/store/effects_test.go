package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
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
	if err != nil || claimed.EffectID() != effectID || claimed.ProtocolRunID() != effectID ||
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
	recovered, err := control.RecoverInterruptedEffects(ctx, "previous process ended before receipt")
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

	notAppliedReceipt := json.RawMessage(`{"observed":"not_applied","system":"test-runner"}`)
	if err := control.ReconcileUnknownEffect(ctx, effectID, unknown[0].Attempt, "reconciler-1", ReconcileNotApplied, notAppliedReceipt, "external system proves no invocation"); err != nil {
		t.Fatal(err)
	}
	claimedAgain, err := control.ClaimNextEffect(ctx, "worker-2")
	if err != nil || claimedAgain.Attempt() != 2 {
		t.Fatalf("second claim = %+v, %v", claimedAgain, err)
	}
	receipt := json.RawMessage(`{"exit_code":0,"output_digest":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}`)
	if err := control.CompleteEffect(ctx, claimedAgain, true, receipt, ""); err != nil {
		t.Fatal(err)
	}
	succeeded, err := control.Effects(ctx, EffectSucceeded)
	if err != nil || len(succeeded) != 1 || string(succeeded[0].Receipt) != string(receipt) {
		t.Fatalf("succeeded = %+v, %v", succeeded, err)
	}
	assertCount(t, control, "effect_observations", 5)
	if _, err := control.ClaimNextEffect(ctx, "worker-3"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("completed effect was claimable: %v", err)
	}
}

func TestFailedEffectRequiresLeaseAndError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil || lease.EffectID() != effectID {
		t.Fatalf("lease = %+v, %v", lease, err)
	}
	failureReceipt := json.RawMessage(`{"exit_code":17}`)
	if err := control.CompleteEffect(ctx, EffectLease{}, false, failureReceipt, "failed"); err == nil {
		t.Fatal("zero lease completed effect")
	}
	if err := control.CompleteEffect(ctx, lease, false, failureReceipt, ""); err == nil {
		t.Fatal("failure without detail was accepted")
	}
	if err := control.CompleteEffect(ctx, lease, false, failureReceipt, "runner exited 17"); err != nil {
		t.Fatal(err)
	}
	failed, err := control.Effects(ctx, EffectFailed)
	if err != nil || len(failed) != 1 || failed[0].LastError != "runner exited 17" {
		t.Fatalf("failed = %+v, %v", failed, err)
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
	notApplied := json.RawMessage(`{"observed":"not_applied"}`)
	if err := control.ReconcileUnknownEffect(ctx, effectID, stale.Attempt(), "reconciler-1", ReconcileNotApplied, notApplied, "no process started"); err != nil {
		t.Fatal(err)
	}
	current, err := control.ClaimNextEffect(ctx, "worker-reused")
	if err != nil || current.Attempt() != 2 {
		t.Fatalf("current lease = %+v, %v", current, err)
	}
	receipt := json.RawMessage(`{"exit_code":0}`)
	if err := control.CompleteEffect(ctx, stale, true, receipt, ""); err == nil {
		t.Fatal("stale same-owner lease completed a later attempt")
	}
	running, err := control.Effects(ctx, EffectRunning)
	if err != nil || len(running) != 1 || running[0].ID != effectID || running[0].Attempt != 2 || len(running[0].Receipt) != 0 {
		t.Fatalf("later attempt changed after stale completion: %+v, %v", running, err)
	}
	if err := control.CompleteEffect(ctx, current, true, receipt, ""); err != nil {
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
	notApplied := json.RawMessage(`{"observed":"not_applied"}`)
	if err := control.ReconcileUnknownEffect(ctx, effectID, first.Attempt(), "reconciler-1", ReconcileNotApplied, notApplied, "attempt one did not start"); err != nil {
		t.Fatal(err)
	}
	second, err := control.ClaimNextEffect(ctx, "worker-2")
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := control.RecoverInterruptedEffects(ctx, "attempt two interrupted"); err != nil || recovered != 1 {
		t.Fatalf("recover attempt two = %d, %v", recovered, err)
	}
	observedSuccess := json.RawMessage(`{"observed":"succeeded"}`)
	if err := control.ReconcileUnknownEffect(ctx, effectID, first.Attempt(), "reconciler-stale", ReconcileSucceeded, observedSuccess, ""); err == nil {
		t.Fatal("observation from attempt one resolved unknown attempt two")
	}
	unknown, err := control.Effects(ctx, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].Attempt != second.Attempt() {
		t.Fatalf("later unknown attempt changed after stale reconciliation: %+v, %v", unknown, err)
	}
	if err := control.ReconcileUnknownEffect(ctx, effectID, second.Attempt(), "reconciler-current", ReconcileSucceeded, observedSuccess, ""); err != nil {
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
	receipt := json.RawMessage(`{"exit_code":0}`)
	if err := second.CompleteEffect(ctx, firstLease, true, receipt, ""); err == nil {
		t.Fatal("lease from another store completed an effect")
	}
	if err := second.CompleteEffect(ctx, secondLease, true, receipt, ""); err != nil {
		t.Fatalf("complete store-owned lease: %v", err)
	}
	if err := second.CompleteEffect(ctx, secondLease, true, receipt, ""); err == nil {
		t.Fatal("one lease completed the same effect twice")
	}
}

func TestCompletionAndRecoveryRaceHasOneLegalWinner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	createActivateAndDispatch(t, control)
	lease, err := control.ClaimNextEffect(ctx, "worker-1")
	if err != nil {
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
		completion <- control.CompleteEffect(ctx, lease, true, json.RawMessage(`{"exit_code":0}`), "")
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
		succeeded, err := control.Effects(ctx, EffectSucceeded)
		if err != nil || len(succeeded) != 1 {
			t.Fatalf("completion winner state = %+v, %v", succeeded, err)
		}
		return
	}
	if recovered.count != 1 {
		t.Fatalf("completion failed but recovery changed %d effects", recovered.count)
	}
	unknown, err := control.Effects(ctx, EffectUnknown)
	if err != nil || len(unknown) != 1 {
		t.Fatalf("recovery winner state = %+v, %v", unknown, err)
	}
}
