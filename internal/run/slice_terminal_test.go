package run

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

// errKindImplement returns an implement arm whose dispatch fails with the
// configured driver ErrKind — the S06 successor of the wire-typed
// errorKindAgent. The terminal predicate the engine consumes is
// driver.TerminalErrKind over Result.ErrKind (D3; S04 Coach ack binding).
func errKindImplement(kind, message string) func(context.Context, driver.DispatchInput) (driver.Result, error) {
	return func(_ context.Context, _ driver.DispatchInput) (driver.Result, error) {
		return driver.Result{Status: driver.StatusError, ErrKind: kind},
			fmt.Errorf("test-provider: %s", message)
	}
}

// ---------------------------------------------------------------------------
// TestTerminalError — the R-03 regression net at the implement leg: BOTH
// terminal kinds (auth, credits) halt immediately as BLOCKED; non-terminal
// kinds enter the normal triage path.
// ---------------------------------------------------------------------------

func TestTerminalError_KindAuth_Halts(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/model"},
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1, // no timeout
		Registry: testRegistry(&fakeDriver{
			implement: errKindImplement(driver.ErrKindAuth, "invalid api key"),
		}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected BLOCKED error for ErrKind=auth, got nil")
	}
	if !IsBlocked(err) {
		t.Fatalf("expected IsBlocked=true for ErrKind=auth, got false; err=%v", err)
	}
	if !strings.Contains(err.Error(), "terminal driver error (auth)") {
		t.Fatalf("expected error to name the terminal kind '(auth)', got: %v", err)
	}
}

func TestTerminalError_KindCredits_Halts(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/model"},
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		Registry: testRegistry(&fakeDriver{
			implement: errKindImplement(driver.ErrKindCredits, "credits exhausted"),
		}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected BLOCKED error for ErrKind=credits, got nil")
	}
	if !IsBlocked(err) {
		t.Fatalf("expected IsBlocked=true for ErrKind=credits, got false; err=%v", err)
	}
	if !strings.Contains(err.Error(), "terminal driver error (credits)") {
		t.Fatalf("expected error to name the terminal kind '(credits)', got: %v", err)
	}
}

func TestTerminalError_KindRateLimit_DoesNotHalt(t *testing.T) {
	// rate_limit is NOT terminal — the error should pass through to the
	// triage/retry path. With RetryCap=0 (single attempt) and a verifier
	// that always passes, the implementer error goes to triage. Since there's
	// only one model and no retries, triage will halt → failed_verification,
	// NOT a BLOCKED verdict.
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/model"},
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		Registry: testRegistry(&fakeDriver{
			implement: errKindImplement("rate_limit", "rate limited"),
		}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected error for ErrKind=rate_limit (triage halts), got nil")
	}
	// rate_limit must NOT produce a BLOCKED verdict — it should be
	// a failed_verification (FAIL-exhausted), not verification blocked.
	if IsBlocked(err) {
		t.Fatalf("rate_limit should NOT be blocked; got BLOCKED: %v", err)
	}
	if !IsFailed(err) {
		t.Fatalf("expected IsFailed=true for rate_limit (exhausted), got false; err=%v", err)
	}
}

func TestTerminalError_NilError_Continues(t *testing.T) {
	// nil errors (successful dispatch) must not be affected by the terminal
	// guard. This confirms the guard does not false-positive.
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	called := false
	opts := RunSliceOptions{
		EscalationModels: []string{"fake/quick"},
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		Registry:         testRegistry(&fakeDriver{implement: markedImplement(&called)}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error on happy path with terminal guard: %v", err)
	}
	if !called {
		t.Error("expected implement dispatch on happy path")
	}
}

func TestTerminalError_UnclassifiedKind(t *testing.T) {
	// An empty or unclassified ErrKind must NOT trigger the terminal halt —
	// even if the error text contains "auth", the predicate reads
	// Result.ErrKind, not prose. It goes through triage.
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/model"},
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		Registry: testRegistry(&fakeDriver{
			implement: errKindImplement("other", "internal server error mentioning auth"),
		}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected error for ErrKind=other (triage halts), got nil")
	}
	if IsBlocked(err) {
		t.Fatalf("ErrKind=other should NOT be blocked; got: %v", err)
	}
}

// TestTerminalError_AllKinds is a table-driven test covering the driver
// ErrKind vocabulary and verifying driver.TerminalErrKind is correctly wired
// at the implement leg: the terminal set is exactly {auth, credits}.
func TestTerminalError_AllKinds(t *testing.T) {
	tests := []struct {
		kind       string
		isTerminal bool
	}{
		{driver.ErrKindAuth, true},
		{driver.ErrKindCredits, true},
		{"rate_limit", false},
		{"upstream", false},
		{driver.ErrKindTransient, false},
		{driver.ErrKindProtocol, false},
		{driver.ErrKindConfig, false},
		{"other", false},
		{"", false},
	}

	for _, tc := range tests {
		name := tc.kind
		if name == "" {
			name = "(empty)"
		}
		t.Run("ErrKind="+name, func(t *testing.T) {
			workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

			opts := RunSliceOptions{
				EscalationModels: []string{"fake/model"},
				VerifierModel:    "fake/verifier",
				CaptainModel:     "fake/verifier",
				RetryCap:         0,
				ImplementTimeout: -1,
				Registry: testRegistry(&fakeDriver{
					implement: errKindImplement(tc.kind, "test error"),
				}),
			}

			err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.isTerminal {
				if !IsBlocked(err) {
					t.Fatalf("%s is terminal — expected IsBlocked=true, got false; err=%v",
						tc.kind, err)
				}
			} else {
				if IsBlocked(err) {
					t.Fatalf("%s is NOT terminal — expected IsBlocked=false, got true; err=%v",
						tc.kind, err)
				}
			}
		})
	}
}
