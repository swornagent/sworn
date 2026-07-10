package verify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/verdict"
)

// fakeVerifierDriver implements driver.Driver for the RunAgentic tests (S06
// transport swap: the wire-typed Chat/ChatStructured stubs became driver
// fakes; the acceptStructuredVerdict-level assertions are unchanged — R-01).
type fakeVerifierDriver struct {
	dispatchFn func(ctx context.Context, in driver.DispatchInput) (driver.Result, error)
	calls      []driver.DispatchInput
}

func (d *fakeVerifierDriver) Name() string { return "fake-verifier-driver" }
func (d *fakeVerifierDriver) Roles() driver.RoleSet {
	return driver.RoleSet{driver.RoleVerifier: true}
}
func (d *fakeVerifierDriver) Dispatch(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
	d.calls = append(d.calls, in)
	return d.dispatchFn(ctx, in)
}

// okResult builds a StatusOK dispatch result carrying the emitted verdict
// JSON plus fixed economics (the fields acceptStructuredVerdict must source
// from the Result — S06 AC-05).
func okResult(emitted string) driver.Result {
	return driver.Result{
		Status:         driver.StatusOK,
		ResultText:     "investigation notes",
		StructuredJSON: json.RawMessage(emitted),
		CostUSD:        0.002,
		CostSource:     "estimated",
		InputTokens:    700,
		OutputTokens:   300,
		ModelID:        "confirmed-model",
		DurationMS:     42,
	}
}

func agenticInput() AgenticInput {
	return AgenticInput{Spec: "spec content", Diff: "diff content", Proof: "proof content", ModelID: "fake/verifier"}
}

// TestRunAgenticPass drives the full structured path end-to-end (the
// reachability artefact): the verifier emits a schema-valid verifier-verdict-v1
// object, it validates, and the typed verdict comes off the object — no prose
// scrape. It also asserts the prompt, payload, and emit schema handed to the
// driver via DispatchInput.
func TestRunAgenticPass(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			if in.Role != driver.RoleVerifier {
				t.Fatalf("expected Role=verifier, got %q", in.Role)
			}
			if !strings.Contains(in.SystemPrompt, "Verifier Role Prompt") {
				t.Error("system prompt should be the verifier role prompt")
			}
			for _, sec := range []string{"## SPEC", "## DIFF", "## PROOF"} {
				if !strings.Contains(in.Payload, sec) {
					t.Errorf("user payload missing %s section", sec)
				}
			}
			// The emit schema is the judgement subset, named verifier-verdict-v1.
			if !strings.Contains(string(in.VerdictSchema), "verifier-verdict-v1") {
				t.Error("emit schema should carry the verifier-verdict-v1 title")
			}
			if !strings.Contains(string(in.VerdictSchema), "INCONCLUSIVE") {
				t.Error("emit schema should constrain the verdict enum")
			}
			return okResult(`{"verdict":"PASS","rationale":"All acceptance checks satisfied."}`), nil
		},
	}

	result, err := RunAgentic(context.Background(), agenticInput(), fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Pass {
		t.Fatalf("expected PASS, got %s (%s)", result.Verdict, result.Rationale)
	}
	if result.Rationale != "All acceptance checks satisfied." {
		t.Errorf("rationale came off the typed object? got %q", result.Rationale)
	}
	if result.CostUSD <= 0 {
		t.Error("expected non-zero Result-sourced cost")
	}
	if result.InputTokens != 700 || result.OutputTokens != 300 {
		t.Errorf("expected token split 700/300, got %d/%d", result.InputTokens, result.OutputTokens)
	}
	if result.DurationMS != 42 {
		t.Errorf("expected DurationMS 42 off the driver Result, got %d", result.DurationMS)
	}
	if result.ModelIDConfirmed != "confirmed-model" {
		t.Errorf("expected ModelIDConfirmed off the driver Result, got %q", result.ModelIDConfirmed)
	}
}

func TestRunAgenticFail(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			return okResult(`{"verdict":"FAIL","rationale":"two problems","violations":[{"gate":"adversarial","description":"AC3 not satisfied"},{"gate":"tests","description":"missing coverage"}]}`), nil
		},
	}
	result, err := RunAgentic(context.Background(), agenticInput(), fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Fail {
		t.Fatalf("expected FAIL, got %s", result.Verdict)
	}
	if len(result.Violations) != 2 {
		t.Fatalf("expected 2 typed violations, got %d (%v)", len(result.Violations), result.Violations)
	}
	if result.Violations[0] != "adversarial: AC3 not satisfied" {
		t.Errorf("violation came off the typed object? got %q", result.Violations[0])
	}
}

func TestRunAgenticBlocked(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			return okResult(`{"verdict":"BLOCKED","rationale":"cannot verify","violations":[{"gate":"spec","description":"AC3 references a non-existent file"}],"routing":"needs_planner"}`), nil
		},
	}
	in := agenticInput()
	in.Proof = ""
	result, err := RunAgentic(context.Background(), in, fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Blocked {
		t.Fatalf("expected BLOCKED, got %s", result.Verdict)
	}
	if result.Routing != "needs_planner" {
		t.Errorf("expected routing needs_planner off the typed object, got %q", result.Routing)
	}
	if len(result.Violations) != 1 {
		t.Errorf("expected 1 violation, got %v", result.Violations)
	}
}

// TestRunAgenticFailWithoutViolationsInconclusive is the fail-closed heart of
// the pilot: the schema requires a FAIL verdict to cite ≥1 violation, so a FAIL
// with none fails validation and resolves to INCONCLUSIVE — a property the old
// HasPrefix("FAIL") scrape could never enforce.
func TestRunAgenticFailWithoutViolationsInconclusive(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			return okResult(`{"verdict":"FAIL","rationale":"vague failure with no cited violations"}`), nil
		},
	}
	result, err := RunAgentic(context.Background(), agenticInput(), fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE (schema-invalid FAIL), got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_verdict_invalid" {
		t.Errorf("expected gate verifier_verdict_invalid, got %s", result.FailedGate)
	}
}

func TestRunAgenticMalformedEmissionInconclusive(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			return okResult(`this is not a JSON object`), nil
		},
	}
	result, err := RunAgentic(context.Background(), agenticInput(), fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE for malformed emission, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_structured_malformed" {
		t.Errorf("expected gate verifier_structured_malformed, got %s", result.FailedGate)
	}
}

func TestRunAgenticBadVerdictEnumInconclusive(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			return okResult(`{"verdict":"MAYBE","rationale":"out-of-enum verdict"}`), nil
		},
	}
	result, err := RunAgentic(context.Background(), agenticInput(), fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE for out-of-enum verdict, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_verdict_invalid" {
		t.Errorf("expected gate verifier_verdict_invalid, got %s", result.FailedGate)
	}
}

// TestRunAgenticStructuredUnsupportedInconclusive is the S06 adaptation of the
// old "verifier driver does not support structured output" case: that
// pre-dispatch type-assert is unrepresentable at the engine layer now — the
// in-process driver fails it closed BEFORE the investigation loop as a
// StatusError/protocol dispatch (inprocess_verify.go), which the engine maps
// to INCONCLUSIVE via the dispatch-error arm. Fail-closed property preserved.
func TestRunAgenticStructuredUnsupportedInconclusive(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindProtocol},
				errors.New(`inprocess: client for "fake/verifier" does not support structured output`)
		},
	}
	result, err := RunAgentic(context.Background(), agenticInput(), fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE for non-structured driver, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_structured_dispatch" {
		t.Errorf("expected gate verifier_structured_dispatch, got %s", result.FailedGate)
	}
}

func TestRunAgenticStructuredDispatchErrorInconclusive(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			return driver.Result{Status: driver.StatusError, ErrKind: "upstream"},
				errors.New("provider 503")
		},
	}
	result, err := RunAgentic(context.Background(), agenticInput(), fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE on dispatch error, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_structured_dispatch" {
		t.Errorf("expected gate verifier_structured_dispatch, got %s", result.FailedGate)
	}
}

// TestRunAgenticTerminalDispatchErrorBlocked proves a terminal driver error
// kind (revoked key, exhausted credits — driver.TerminalErrKind, read from
// Result.ErrKind per S06 AC-03) on the verifier dispatch surfaces as BLOCKED,
// not INCONCLUSIVE: triage maps BLOCKED to Halt, so the run loop cannot burn
// the implementer escalation ladder on an error that can never succeed on
// retry — mirroring the implementer path's terminal-error halt (S09 AC1).
// Both kinds are asserted — the {auth, credits} set is the S04 Coach
// acknowledgement binding (spec R-03).
func TestRunAgenticTerminalDispatchErrorBlocked(t *testing.T) {
	cases := []struct {
		name    string
		errKind string
		kind    model.ErrorKind
	}{
		{"auth_revoked_key", driver.ErrKindAuth, model.KindAuth},
		{"credits_exhausted", driver.ErrKindCredits, model.KindCredits},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fd := &fakeVerifierDriver{
				dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
					return driver.Result{Status: driver.StatusError, ErrKind: tc.errKind, CostUSD: 0.001},
						&model.Error{
							Kind:     tc.kind,
							Status:   401,
							Provider: "openai",
							Model:    "gpt-4o-mini",
							Message:  "credentials rejected",
						}
				},
			}
			result, err := RunAgentic(context.Background(), agenticInput(), fd)
			if err != nil {
				t.Fatalf("RunAgentic: %v", err)
			}
			if result.Verdict != verdict.Blocked {
				t.Fatalf("expected BLOCKED for terminal %s error, got %s (%s)",
					tc.errKind, result.Verdict, result.Rationale)
			}
			if result.FailedGate != "verifier_terminal_error" {
				t.Errorf("expected gate verifier_terminal_error, got %s", result.FailedGate)
			}
			if result.ExitCode() != 2 {
				t.Errorf("expected exit code 2 (BLOCKED), got %d", result.ExitCode())
			}
			if !strings.Contains(strings.ToLower(result.Rationale), tc.errKind) {
				t.Errorf("rationale should name the terminal kind %q, got %q", tc.errKind, result.Rationale)
			}
		})
	}
}

// TestRunAgenticTransientTypedErrorInconclusive pins the boundary: typed but
// NON-terminal driver error kinds (rate limit, upstream 5xx, transient,
// other/protocol) stay INCONCLUSIVE so triage retries/escalates — only the
// terminal kinds {auth, credits} halt as BLOCKED (the transient-continues leg
// of the R-03 regression net).
func TestRunAgenticTransientTypedErrorInconclusive(t *testing.T) {
	for _, errKind := range []string{"rate_limit", "upstream", driver.ErrKindTransient, "other", driver.ErrKindProtocol} {
		fd := &fakeVerifierDriver{
			dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
				return driver.Result{Status: driver.StatusError, ErrKind: errKind},
					errors.New("transient")
			},
		}
		result, err := RunAgentic(context.Background(), agenticInput(), fd)
		if err != nil {
			t.Fatalf("RunAgentic (%s): %v", errKind, err)
		}
		if result.Verdict != verdict.Inconclusive {
			t.Fatalf("expected INCONCLUSIVE for transient %s error, got %s", errKind, result.Verdict)
		}
		if result.FailedGate != "verifier_structured_dispatch" {
			t.Errorf("expected gate verifier_structured_dispatch for %s, got %s", errKind, result.FailedGate)
		}
	}
}

// TestRunAgenticMissingStructuredOutputInconclusive is the old empty-choices
// class under the driver transport: a StatusOK dispatch that carries no
// StructuredJSON fails closed.
func TestRunAgenticMissingStructuredOutputInconclusive(t *testing.T) {
	fd := &fakeVerifierDriver{
		dispatchFn: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
			return driver.Result{Status: driver.StatusOK, ResultText: "prose only"}, nil
		},
	}
	result, err := RunAgentic(context.Background(), agenticInput(), fd)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE on missing structured output, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_structured_dispatch" {
		t.Errorf("expected gate verifier_structured_dispatch, got %s", result.FailedGate)
	}
}
