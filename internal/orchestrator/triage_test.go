package orchestrator

import (
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/verdict"
)

func TestFailResolvesThenEscalates(t *testing.T) {
	// AC1: A FAIL on attempt 0 returns resolve_in_place (same model) and the
	// retry carries the S44 feedback; a second FAIL returns escalate_model.
	in := Input{
		Verdict:        verdict.Fail,
		AttemptOnModel: 0,
		ModelIdx:       0,
		EscalationLen:  3,
		MaxResolves:    1,
	}
	out := Decide(in)
	if out.Action != ResolveInPlace {
		t.Fatalf("attempt 0 FAIL: expected resolve_in_place, got %s (reason: %s)", out.Action, out.Reason)
	}
	if !strings.Contains(out.Reason, "resolve_in_place") {
		t.Errorf("reason should mention resolve_in_place, got: %s", out.Reason)
	}

	// Second FAIL on same model after resolve budget exhausted → escalate.
	in.AttemptOnModel = 1
	out = Decide(in)
	if out.Action != EscalateModel {
		t.Fatalf("attempt 1 FAIL: expected escalate_model, got %s (reason: %s)", out.Action, out.Reason)
	}
	if !strings.Contains(out.Reason, "escalating to model 1") {
		t.Errorf("reason should mention escalating, got: %s", out.Reason)
	}
}

func TestExhaustedEscalationHalts(t *testing.T) {
	// AC2: Escalation list exhausted by FAILs returns halt, committing
	// failed_verification (fail-closed), not a loop.
	in := Input{
		Verdict:        verdict.Fail,
		AttemptOnModel: 2, // MaxResolves=1, already exhausted resolve budget
		ModelIdx:       1, // last model (0-indexed, EscalationLen=2)
		EscalationLen:  2,
		MaxResolves:    1,
	}
	out := Decide(in)
	if out.Action != Halt {
		t.Fatalf("exhausted: expected halt, got %s (reason: %s)", out.Action, out.Reason)
	}
	if !strings.Contains(out.Reason, "escalation list exhausted") {
		t.Errorf("reason should mention escalation list exhausted, got: %s", out.Reason)
	}
}

func TestBlockedHaltsCommitsBlocked(t *testing.T) {
	// AC3: A BLOCKED verdict returns halt immediately, committing blocked with
	// the verifier's violations populated — and does NOT re-classify
	// spec-defect vs genuine here (that routing is S58's).
	in := Input{
		Verdict:        verdict.Blocked,
		AttemptOnModel: 0,
		ModelIdx:       0,
		EscalationLen:  5,
		MaxResolves:    1,
	}
	out := Decide(in)
	if out.Action != Halt {
		t.Fatalf("BLOCKED: expected halt, got %s (reason: %s)", out.Action, out.Reason)
	}
	if !strings.Contains(out.Reason, "BLOCKED") {
		t.Errorf("reason should mention BLOCKED, got: %s", out.Reason)
	}
	// Must NOT contain spec-defect re-classification language — that routing
	// is owned by S58.
	if strings.Contains(out.Reason, "spec-defect") || strings.Contains(out.Reason, "genuine") {
		t.Errorf("reason must not re-classify spec-defect vs genuine: %s", out.Reason)
	}
}

func TestFailResolvesMultiplePerModel(t *testing.T) {
	// Edge: with K=2, two resolve_in_place attempts before escalating.
	in := Input{
		Verdict:        verdict.Fail,
		AttemptOnModel: 0,
		ModelIdx:       0,
		EscalationLen:  2,
		MaxResolves:    2,
	}
	out := Decide(in)
	if out.Action != ResolveInPlace {
		t.Fatalf("K=2 attempt 0: expected resolve_in_place, got %s", out.Action)
	}

	in.AttemptOnModel = 1
	out = Decide(in)
	if out.Action != ResolveInPlace {
		t.Fatalf("K=2 attempt 1: expected resolve_in_place, got %s", out.Action)
	}

	in.AttemptOnModel = 2
	out = Decide(in)
	if out.Action != EscalateModel {
		t.Fatalf("K=2 attempt 2: expected escalate_model, got %s", out.Action)
	}
}

func TestInconclusiveTreatedAsFail(t *testing.T) {
	// Inconclusive verdicts follow the same resolve → escalate → halt policy
	// as FAIL.
	in := Input{
		Verdict:        verdict.Inconclusive,
		AttemptOnModel: 0,
		ModelIdx:       0,
		EscalationLen:  2,
		MaxResolves:    1,
	}
	out := Decide(in)
	if out.Action != ResolveInPlace {
		t.Fatalf("Inconclusive attempt 0: expected resolve_in_place, got %s", out.Action)
	}

	in.AttemptOnModel = 1
	out = Decide(in)
	if out.Action != EscalateModel {
		t.Fatalf("Inconclusive attempt 1: expected escalate_model, got %s", out.Action)
	}
}

func TestBlockedIgnoresResolveBudget(t *testing.T) {
	// BLOCKED halts even when resolve budget is high and more models remain.
	in := Input{
		Verdict:        verdict.Blocked,
		AttemptOnModel: 0,
		ModelIdx:       0,
		EscalationLen:  10,
		MaxResolves:    5,
	}
	out := Decide(in)
	if out.Action != Halt {
		t.Fatalf("BLOCKED with budget: expected halt, got %s", out.Action)
	}
}

func TestUnknownVerdictHaltsEventually(t *testing.T) {
	// An unknown verdict (empty string or unrecognised) is treated as FAIL
	// and follows the resolve→escalate→halt path without panicking.
	in := Input{
		Verdict:        verdict.Verdict(""),
		AttemptOnModel: 0,
		ModelIdx:       0,
		EscalationLen:  1,
		MaxResolves:    0, // zero resolve budget → escalate immediately
	}
	out := Decide(in)
	if out.Action != Halt {
		t.Fatalf("unknown verdict: expected halt (0 budget, 1 model), got %s", out.Action)
	}
}

func TestNegativeMaxResolvesClamped(t *testing.T) {
	// Negative MaxResolves is clamped to 0 (no resolve_in_place).
	in := Input{
		Verdict:        verdict.Fail,
		AttemptOnModel: 0,
		ModelIdx:       0,
		EscalationLen:  2,
		MaxResolves:    -1,
	}
	out := Decide(in)
	if out.Action != EscalateModel {
		t.Fatalf("negative MaxResolves: expected escalate_model, got %s (reason: %s)", out.Action, out.Reason)
	}
}

func TestTriageReasonAuditability(t *testing.T) {
	// AC4: Each triage decision logs an explainable rationale.
	tests := []struct {
		name string
		in   Input
	}{
		{"resolve", Input{Verdict: verdict.Fail, AttemptOnModel: 0, ModelIdx: 0, EscalationLen: 3, MaxResolves: 1}},
		{"escalate", Input{Verdict: verdict.Fail, AttemptOnModel: 1, ModelIdx: 0, EscalationLen: 3, MaxResolves: 1}},
		{"halt_exhausted", Input{Verdict: verdict.Fail, AttemptOnModel: 1, ModelIdx: 1, EscalationLen: 2, MaxResolves: 1}},
		{"halt_blocked", Input{Verdict: verdict.Blocked, AttemptOnModel: 0, ModelIdx: 0, EscalationLen: 3, MaxResolves: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := Decide(tt.in)
			if out.Reason == "" {
				t.Errorf("%s: Reason must not be empty", tt.name)
			}
		})
	}
}
