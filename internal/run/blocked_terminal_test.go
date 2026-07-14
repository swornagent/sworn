package run

// S14-blocked-terminal — loop-state-machine tests (AC-01, AC-02, AC-03).
//
// These replay the reference S05-section-owned-saves failure with synthetic
// verdict objects (no consumer checkout required): the harness dispatched THREE
// implementer sessions against an unchanged blocker because it had no typed
// blocked signal. The tests assert BLOCKED is terminal-for-the-lane at both
// consumption legs, consumes no retry budget, and that FAIL keeps its
// existing retry semantics unchanged.
//
// New file by design: AC-06 forbids edits to any existing retry test.

import (
	"context"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
)

// implementerPayloads returns the Payload of every RoleImplementer dispatch
// the fake served, in order. Defined here (same package) so AC-03 can assert
// the verifier's violations text is forwarded into the retry dispatch.
func (d *fakeDriver) implementerPayloads() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, c := range d.calls {
		if c.Role == driver.RoleImplementer {
			out = append(out, c.Payload)
		}
	}
	return out
}

// TestLoopBlockedImplementerTerminal is the AC-01 proof — the reference S05
// replay. Dispatch 1 returns a typed blocked signal (Status==StatusBlocked +
// BlockedReason); the runner must make no further implementer dispatches for
// the slice, never dispatch the verifier, and emit the blocker text VERBATIM
// with an explicit route-to-/replan-release directive.
func TestLoopBlockedImplementerTerminal(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	// The reference S05 blocker, verbatim-shaped: a spec defect only a replan can
	// clear. Retries REMAIN (two models in the escalation list) — a blocked
	// lane with budget left is still terminal.
	const blocker = "spec defect: the saves contract references a section-ownership model the spec never defines — not clearable by re-dispatch"

	notifier := &fakeNotifier{webhook: true}
	d := &fakeDriver{
		implement: func(_ context.Context, _ driver.DispatchInput) (driver.Result, error) {
			return driver.Result{Status: driver.StatusBlocked, BlockedReason: blocker}, nil
		},
		verify: func(_ context.Context, _ driver.DispatchInput) (driver.Result, error) {
			t.Error("verifier dispatched for a blocked implementer lane — BLOCKED must terminate before the verify leg")
			return okStructured(structuredVerdictReply("PASS")), nil
		},
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		EscalationModels: []string{"fake/impl1", "fake/impl2"},
		Registry:         testRegistry(d),
		Notifier:         notifier,
	})
	if err == nil {
		t.Fatal("expected blocked-terminal error, got nil")
	}
	if !IsBlocked(err) {
		t.Fatalf("expected IsBlocked(err)=true, got false: %v", err)
	}
	// Blocker text rides in the error VERBATIM, with the replan directive.
	if !strings.Contains(err.Error(), blocker) {
		t.Errorf("error must carry the blocker verbatim; got: %v", err)
	}
	if !strings.Contains(err.Error(), "/replan-release") {
		t.Errorf("error must carry the route-to-/replan-release directive; got: %v", err)
	}

	// Exactly ONE implementer dispatch, ZERO verifier dispatches — no retry
	// budget consumed despite a second escalation model being available.
	roles := d.dispatchedRoles()
	if got := roles[driver.RoleImplementer]; got != 1 {
		t.Errorf("implementer dispatches = %d, want exactly 1 (BLOCKED is terminal, budget untouched)", got)
	}
	if got := roles[driver.RoleVerifier]; got != 0 {
		t.Errorf("verifier dispatches = %d, want 0", got)
	}

	// status.json: blocked verification record, replan routing, blocker
	// verbatim in violations, state NOT advanced (stays in_progress — the
	// blocked dispatch is never certified).
	st, stErr := state.Read(statusPath)
	if stErr != nil {
		t.Fatal(stErr)
	}
	if st.Verification.Result != "blocked" {
		t.Errorf("verification.result = %q, want \"blocked\"", st.Verification.Result)
	}
	if st.Verification.Routing != "needs_planner" {
		t.Errorf("verification.routing = %q, want \"needs_planner\"", st.Verification.Routing)
	}
	violations := st.Verification.ViolationStrings()
	if len(violations) != 1 || violations[0] != blocker {
		t.Errorf("violations = %v, want exactly the blocker verbatim", violations)
	}
	if st.State != state.InProgress {
		t.Errorf("state = %q, want in_progress (a blocked dispatch is never certified)", st.State)
	}

	// Notified exactly once, state "blocked".
	if got := notifier.count(); got != 1 {
		t.Fatalf("Notify called %d times, want exactly 1", got)
	}
	if ev, _ := notifier.lastCall(); ev.State != "blocked" {
		t.Errorf("notify state = %q, want \"blocked\"", ev.State)
	}
}

// TestLoopBlockedVerifierTerminal is the AC-02 anchor: a verifier BLOCKED
// verdict terminates the lane identically to AC-01 — with retries remaining
// on a two-model escalation list — consuming no implementer retry budget and
// never being mapped onto FAIL.
func TestLoopBlockedVerifierTerminal(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Blocked, Rationale: "spec ambiguity: acceptance check 3 contradicts the out-of-scope list"},
		},
	}
	d := &fakeDriver{
		implement: writeFileImplementer("blocked verifier test"),
		verify:    verdictsFrom(verifier),
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		EscalationModels: []string{"fake/impl1", "fake/impl2"}, // retries remain — still terminal
		Registry:         testRegistry(d),
	})
	if err == nil {
		t.Fatal("expected blocked-terminal error, got nil")
	}
	if !IsBlocked(err) {
		t.Fatalf("expected IsBlocked(err)=true, got false: %v", err)
	}
	if IsFailed(err) {
		t.Fatalf("BLOCKED must never be mapped onto FAIL; got: %v", err)
	}

	// One implementer + one verifier dispatch total: no resolve_in_place, no
	// model escalation — the second escalation model is never consulted.
	roles := d.dispatchedRoles()
	if got := roles[driver.RoleImplementer]; got != 1 {
		t.Errorf("implementer dispatches = %d, want exactly 1 (no retry budget consumed on BLOCKED)", got)
	}
	if got := roles[driver.RoleVerifier]; got != 1 {
		t.Errorf("verifier dispatches = %d, want exactly 1", got)
	}

	st, stErr := state.Read(statusPath)
	if stErr != nil {
		t.Fatal(stErr)
	}
	if st.Verification.Result != "blocked" {
		t.Errorf("verification.result = %q, want \"blocked\"", st.Verification.Result)
	}
	if st.State == state.FailedVerification {
		t.Errorf("state = failed_verification — BLOCKED must not take the FAIL-exhausted transition")
	}
}

// TestLoopFailRetrySemanticsUnchanged is the AC-03 guard: FAIL with retries
// remaining keeps today's semantics — the violations/rationale text is
// forwarded to the next implementer dispatch and the retry is consumed.
func TestLoopFailRetrySemanticsUnchanged(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Fail, Rationale: "missing hello output assertion"},
			{Verdict: verdict.Pass, Rationale: "resolved"},
		},
	}
	d := &fakeDriver{
		implement: writeFileImplementer("fail then pass"),
		verify:    verdictsFrom(verifier),
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		EscalationModels: []string{"fake/impl1"},
		Registry:         testRegistry(d),
	})
	if err != nil {
		t.Fatalf("expected FAIL→PASS retry to succeed, got: %v", err)
	}

	// Retry consumed: exactly two implementer dispatches.
	payloads := d.implementerPayloads()
	if len(payloads) != 2 {
		t.Fatalf("implementer dispatches = %d, want exactly 2 (one retry consumed)", len(payloads))
	}
	// The FAIL rationale is forwarded into dispatch 2's payload (S44 feedback).
	if !strings.Contains(payloads[1], "missing hello output assertion") {
		t.Errorf("retry dispatch payload must carry the verifier's violations text; got:\n%s", payloads[1])
	}
	if strings.Contains(payloads[0], "missing hello output assertion") {
		t.Errorf("first dispatch must not carry feedback that does not exist yet")
	}

	st, stErr := state.Read(statusPath)
	if stErr != nil {
		t.Fatal(stErr)
	}
	if st.State != state.Verified {
		t.Errorf("state = %q, want verified after FAIL→PASS retry", st.State)
	}
}
