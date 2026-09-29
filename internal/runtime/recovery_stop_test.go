package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

// #373: the recovery sweep maps only a stop the run itself requested onto
// RUN_STOPPED. A recovery failure keeps its own code even when a cancellation
// sits inside it, and a step's own timeout or cancellation is not a stop.
func TestNormalizeRecoveryStopMapsOnlyAStopTheRunRequested(t *testing.T) {
	stopped, stop := context.WithCancel(context.Background())
	stop()
	live := context.Background()

	// The shape gitx returns when `git update-ref` is killed by a cancelled
	// context mid-transaction: the outcome is ambiguous and the cancellation
	// is joined inside it.
	ambiguousRef := &gitx.Error{
		Code: "REF_TRANSACTION_RECOVERY_REQUIRED",
		Op:   "exact ref transaction outcome is ambiguous",
		Err:  errors.Join(context.Canceled, errors.New("exact ref transaction outcome is ambiguous")),
	}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
		want string
	}{
		{"uncertain recovery wrapping a cancelled ref transaction", stopped,
			runtimeFail("RECOVERY_UNCERTAIN", runtimeFail("RECOVERY_FAILED", ambiguousRef)), "RECOVERY_UNCERTAIN"},
		{"failed recovery wrapping a cancelled ref transaction", stopped,
			runtimeFail("RECOVERY_FAILED", ambiguousRef), "RECOVERY_FAILED"},
		{"bare ambiguous ref transaction", stopped, ambiguousRef, "REF_TRANSACTION_RECOVERY_REQUIRED"},
		{"uncertain recovery wrapping a control stop", stopped,
			runtimeFail("RECOVERY_UNCERTAIN", &journal.Error{Code: "CONTROL_STOPPED"}), "RECOVERY_UNCERTAIN"},
		{"a step's own deadline", live, context.DeadlineExceeded, ""},
		{"a step's own cancellation while the run is live", live, context.Canceled, ""},
		{"a cancellation the run requested", stopped, context.Canceled, "RUN_STOPPED"},
		{"control stopped", live, &journal.Error{Code: "CONTROL_STOPPED"}, "RUN_STOPPED"},
		{"operation cancelled", stopped, &journal.Error{Code: "OPERATION_CANCELLED", Err: context.Canceled}, "RUN_STOPPED"},
		{"already a stop", stopped, runtimeFail("RUN_STOPPED", nil), "RUN_STOPPED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeRecoveryStop(tc.ctx, tc.err)
			if tc.want == "" {
				if got != tc.err {
					t.Fatalf("normalizeRecoveryStop(%v) = %v, want the error unchanged", tc.err, got)
				}
				return
			}
			if IsCode(got, "RUN_STOPPED") != (tc.want == "RUN_STOPPED") || !IsCode(got, tc.want) {
				t.Fatalf("normalizeRecoveryStop(%v) = %v, want code %s", tc.err, got, tc.want)
			}
		})
	}
	if got := normalizeRecoveryStop(stopped, nil); got != nil {
		t.Fatalf("normalizeRecoveryStop(nil) = %v, want nil", got)
	}
}
