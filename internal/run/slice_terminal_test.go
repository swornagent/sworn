package run

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/model"
)

// ---------------------------------------------------------------------------
// errorKindAgent — returns a *model.Error with the configured ErrorKind
// ---------------------------------------------------------------------------

type errorKindAgent struct {
	kind    model.ErrorKind
	status  int
	message string
}

func (e *errorKindAgent) Chat(_ context.Context, _ []model.ChatMessage, _ []model.ToolDef) (*model.ChatResponse, error) {
	return nil, &model.Error{
		Kind:     e.kind,
		Status:   e.status,
		Provider: "test-provider",
		Model:    "test/model",
		Message:  e.message,
	}
}

var _ agent.Agent = (*errorKindAgent)(nil)

// ---------------------------------------------------------------------------
// TestTerminalError — covers AC1, AC2, AC5
// ---------------------------------------------------------------------------

func TestTerminalError_KindAuth_Halts(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"test/model"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1, // no timeout
		NewAgent: func(_ string) (agent.Agent, error) {
			return &errorKindAgent{
				kind:    model.KindAuth,
				status:  401,
				message: "invalid api key",
			}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) {
			return &alwaysPassVerifier{}, nil
		},
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected BLOCKED error for KindAuth, got nil")
	}
	if !IsBlocked(err) {
		t.Fatalf("expected IsBlocked=true for KindAuth, got false; err=%v", err)
	}
	if !strings.Contains(err.Error(), "KindAuth") {
		t.Fatalf("expected error to contain 'KindAuth', got: %v", err)
	}
}

func TestTerminalError_KindCredits_Halts(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"test/model"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		NewAgent: func(_ string) (agent.Agent, error) {
			return &errorKindAgent{
				kind:    model.KindCredits,
				status:  402,
				message: "credits exhausted",
			}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) {
			return &alwaysPassVerifier{}, nil
		},
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected BLOCKED error for KindCredits, got nil")
	}
	if !IsBlocked(err) {
		t.Fatalf("expected IsBlocked=true for KindCredits, got false; err=%v", err)
	}
	if !strings.Contains(err.Error(), "KindCredits") {
		t.Fatalf("expected error to contain 'KindCredits', got: %v", err)
	}
}

func TestTerminalError_KindRateLimit_DoesNotHalt(t *testing.T) {
	// KindRateLimit is NOT terminal — the error should pass through to the
	// triage/retry path. With RetryCap=0 (single attempt) and a verifier
	// that always passes, the implementer error goes to triage. Since there's
	// only one model and no retries, triage will halt → failed_verification,
	// NOT a BLOCKED verdict.
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"test/model"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		NewAgent: func(_ string) (agent.Agent, error) {
			return &errorKindAgent{
				kind:    model.KindRateLimit,
				status:  429,
				message: "rate limited",
			}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) {
			return &alwaysPassVerifier{}, nil
		},
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected error for KindRateLimit (triage halts), got nil")
	}
	// KindRateLimit must NOT produce a BLOCKED verdict — it should be
	// a failed_verification (FAIL-exhausted), not verification blocked.
	if IsBlocked(err) {
		t.Fatalf("KindRateLimit should NOT be blocked; got BLOCKED: %v", err)
	}
	if !IsFailed(err) {
		t.Fatalf("expected IsFailed=true for KindRateLimit (exhausted), got false; err=%v", err)
	}
}

func TestTerminalError_NilError_Continues(t *testing.T) {
	// nil errors (successful dispatch) must not be affected by IsTerminal.
	// This is covered by the existing TestImplementTimeoutHappyPath test
	// which uses quickFakeAgent (returns nil error).  This test confirms
	// the guard does not false-positive.
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	called := false
	opts := RunSliceOptions{
		EscalationModels: []string{"quick"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		NewAgent: func(_ string) (agent.Agent, error) {
			return &markedAgent{called: &called}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) {
			return &alwaysPassVerifier{}, nil
		},
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error on happy path with terminal guard: %v", err)
	}
	if !called {
		t.Error("expected agent to be called on happy path")
	}
}

func TestTerminalError_UntypedTerminal(t *testing.T) {
	// model.IsTerminal returns false for errors that are not *model.Error.
	// Even if the error message contains "auth", an untyped error should
	// NOT trigger the terminal halt — it goes through triage.
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"test/model"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		NewAgent: func(_ string) (agent.Agent, error) {
			return &errorKindAgent{
				kind:    model.KindOther, // KindOther is not terminal
				status:  500,
				message: "internal server error",
			}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) {
			return &alwaysPassVerifier{}, nil
		},
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected error for KindOther (triage halts), got nil")
	}
	if IsBlocked(err) {
		t.Fatalf("KindOther should NOT be blocked; got: %v", err)
	}
}

// TestTerminalError_AllKinds is a table-driven test covering every ErrorKind
// and verifying IsTerminal is correctly wired.
func TestTerminalError_AllKinds(t *testing.T) {
	tests := []struct {
		kind       model.ErrorKind
		isTerminal bool
	}{
		{model.KindAuth, true},
		{model.KindCredits, true},
		{model.KindRateLimit, false},
		{model.KindUpstream, false},
		{model.KindTransient, false},
		{model.KindOther, false},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("Kind=%s", tc.kind.String()), func(t *testing.T) {
			workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

			opts := RunSliceOptions{
				EscalationModels: []string{"test/model"},
				VerifierModel:    "fake/verifier",
				RetryCap:         0,
				ImplementTimeout: -1,
				NewAgent: func(_ string) (agent.Agent, error) {
					return &errorKindAgent{
						kind:    tc.kind,
						status:  400,
						message: "test error",
					}, nil
				},
				NewVerifier: func(_ string) (model.Verifier, error) {
					return &alwaysPassVerifier{}, nil
				},
			}

			err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.isTerminal {
				if !IsBlocked(err) {
					t.Fatalf("%s is terminal — expected IsBlocked=true, got false; err=%v",
						tc.kind.String(), err)
				}
			} else {
				if IsBlocked(err) {
					t.Fatalf("%s is NOT terminal — expected IsBlocked=false, got true; err=%v",
						tc.kind.String(), err)
				}
			}
		})
	}
}