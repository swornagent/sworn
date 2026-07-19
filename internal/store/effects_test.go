package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
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
	if err != nil || claimed.ID != effectID || claimed.Attempt != 1 || claimed.State != EffectRunning {
		t.Fatalf("claimed = %+v, %v", claimed, err)
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
	if err := control.ReconcileUnknownEffect(ctx, effectID, "reconciler-1", ReconcileNotApplied, notAppliedReceipt, "external system proves no invocation"); err != nil {
		t.Fatal(err)
	}
	claimedAgain, err := control.ClaimNextEffect(ctx, "worker-2")
	if err != nil || claimedAgain.Attempt != 2 {
		t.Fatalf("second claim = %+v, %v", claimedAgain, err)
	}
	receipt := json.RawMessage(`{"exit_code":0,"output_digest":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}`)
	if err := control.CompleteEffect(ctx, effectID, "worker-2", true, receipt, ""); err != nil {
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

func TestFailedEffectRequiresOwnerAndError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	effectID := createActivateAndDispatch(t, control)
	if _, err := control.ClaimNextEffect(ctx, "worker-1"); err != nil {
		t.Fatal(err)
	}
	failureReceipt := json.RawMessage(`{"exit_code":17}`)
	if err := control.CompleteEffect(ctx, effectID, "wrong-worker", false, failureReceipt, "failed"); err == nil {
		t.Fatal("wrong owner completed effect")
	}
	if err := control.CompleteEffect(ctx, effectID, "worker-1", false, failureReceipt, ""); err == nil {
		t.Fatal("failure without detail was accepted")
	}
	if err := control.CompleteEffect(ctx, effectID, "worker-1", false, failureReceipt, "runner exited 17"); err != nil {
		t.Fatal(err)
	}
	failed, err := control.Effects(ctx, EffectFailed)
	if err != nil || len(failed) != 1 || failed[0].LastError != "runner exited 17" {
		t.Fatalf("failed = %+v, %v", failed, err)
	}
}
