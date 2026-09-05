package driver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"syscall"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

// checkEvidencePlanFixture builds one legacy inline baton.plan/v2 body
// (declared.ContractPath == "", so resolveVerifierContractBinding reads
// declared.Checks directly, no contract file needed) declaring exactly
// checks for slice S1.
func checkEvidencePlanFixture(checks []string) []byte {
	encoded := "["
	for index, check := range checks {
		if index > 0 {
			encoded += ","
		}
		encoded += `"` + check + `"`
	}
	encoded += "]"
	return []byte("```baton-plan-v2\n" + `{
  "schema_version": "baton.plan/v2",
  "release": "fixture",
  "revision": 1,
  "previous_plan": null,
  "repository": "fixture/repo",
  "target_ref": "refs/heads/main",
  "approval_ref": "fixture://approval/fixture/1",
  "tracks": [
    {
      "id": "T1",
      "depends_on": [],
      "slices": [
        {
          "id": "S1",
          "outcome": "Deliver S1.",
          "scope": {"include": ["one.txt"], "exclude": []},
          "acceptance": [{"id": "A-S1", "text": "S1 is exact."}],
          "checks": ` + encoded + `,
          "constraints": ["deterministic"],
          "depends_on": [],
          "consumes": []
        }
      ]
    }
  ]
}` + "\n```\n# Plan\n")
}

// checkEvidenceInvocationFixture builds one WorkVerification, read-only
// toolSession invocation whose work-context.json and plan.md Inputs carry
// slice S1 with declaredChecks - the exact shape resolveVerifierContractBinding
// reads (production_dispatch.go's own currentPlanBinding/productionInputContents
// idiom, reproduced here without any internal/runtime dependency).
func checkEvidenceInvocationFixture(t *testing.T, declaredChecks []string) Invocation {
	t.Helper()
	contextBody := []byte(`{"release":"fixture","slice":"S1","attempt":1}`)
	planBody := checkEvidencePlanFixture(declaredChecks)
	workContextInput := Input{
		Name: "work-context", Path: "work-context.json", Digest: Digest(contextBody),
	}
	planInput := Input{Name: "plan", Path: "baton/plan.md", Digest: Digest(planBody)}
	request, err := NewRequest(
		"invocation-check-evidence",
		RoleVerifier,
		"fake-profile",
		"selected-model",
		Workspace{Path: GuestWorkspacePath, Access: ReadOnly},
		[]Input{workContextInput, planInput},
		true,
		Limits{TimeoutMillis: 60_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &memoryAdapter{identity: AdapterIdentity{
		Key:                 "fake-adapter",
		ID:                  FakeDriverID,
		Version:             FakeDriverVersion,
		ConfigurationDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}}
	selected := SelectedProfile{
		Profile: ProfileConfig{
			Key: request.Profile, Adapter: adapter.identity.Key, Network: NetworkNone,
		},
		Adapter: adapter.identity, Model: request.Model, adapter: adapter,
	}
	permission, err := NewSubmissionPermission(
		request, selected, ContainmentReadOnly, WorkVerification,
	)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request:       request,
		HostWorkspace: t.TempDir(),
		Selected:      selected,
		Permission:    permission,
		Inputs: []InputContent{
			{Input: workContextInput, Bytes: contextBody},
			{Input: planInput, Bytes: planBody},
		},
		// A no-op success hook so an in-turn correction's real refusal code
		// is visible in this test instead of being masked by
		// RECOVERY_STEP_REFUSED (reserveRecoveryStep refuses outright with a
		// nil hook).
		RecoveryStepHook: func(context.Context, RecoveryStepKind) error { return nil },
	}
}

func workVerificationSubmitArguments(
	t *testing.T, invocationID string, outcome DecisionOutcome,
) map[string]any {
	t.Helper()
	checkBytes, err := NewCheckBytes([]byte{0x00, 0xff, '\n'})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := NewDecision(outcome)
	if err != nil {
		t.Fatal(err)
	}
	submission := Submission{
		SchemaVersion:  SubmissionSchemaVersion,
		InvocationID:   invocationID,
		Responsibility: WorkVerification,
		Summary:        "Compact WorkVerification summary padded well past the submission content floor so this fixture clears validation independent of the check-evidence behavior under test in this file.",
		Detail:         "Bounded LF-only WorkVerification detail padded well past the detail content floor so this fixture clears submission validation independent of the check-evidence behavior actually under test in this file, which concerns only the checks field.\n",
		Checks:         checkBytes,
		Decision:       decision,
	}
	return map[string]any{"submission": submission}
}

// TestWorkVerificationCheckEvidenceIsDriverAuthored pins S5-A3 evidence (b):
// the driver, not the model, authors submission.Checks. session.bash's own
// glue (tools.go) is a two-line pass-through to recordCheckEvidence, already
// exercised against the real sandbox by the pre-existing tools_linux_test.go
// corpus; this test drives recordCheckEvidence itself with the exact
// (output, exit code, error) shape runToolBash returns, so it proves the
// accumulator and the submit-time manifest construction on any host,
// including one without a trusted containment binary. One recorded call
// covers the declared check (wrapped in an ordinary redirect-and-tail form
// the real sandbox pushes workers toward) and one is arbitrary; the sealed
// submission's Checks, decoded through baton.ParseCheckResults, shows the
// wrapped call covering the declared check with outcome pass and the
// arbitrary call recorded verbatim - never the placeholder bytes
// {0x00, 0xff, '\n'} the model itself submitted.
func TestWorkVerificationCheckEvidenceIsDriverAuthored(t *testing.T) {
	declared := "echo covered-check"
	invocation := checkEvidenceInvocationFixture(t, []string{declared})
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	wrapped := "echo covered-check > /tmp/covered.log 2>&1; tail -c 4000 /tmp/covered.log"
	arbitrary := "echo an-arbitrary-call"
	session.recordCheckEvidence(wrapped, []byte("covered-check\n"), 0, nil)
	session.recordCheckEvidence(arbitrary, []byte("an-arbitrary-call\n"), 0, nil)

	arguments := workVerificationSubmitArguments(t, invocation.Request.InvocationID, DecisionPass)
	result := executeToolJSON(t, session, "submit", "sworn_submit", arguments)
	if result.Failed {
		t.Fatalf("submit failed: %s", result.Content)
	}

	submission, err := DecodeSubmission(session.submission)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Checks == nil {
		t.Fatal("submission.Checks is nil")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(submission.Checks.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	results, err := baton.ParseCheckResults(raw)
	if err != nil {
		t.Fatalf("driver-built manifest does not parse: %v", err)
	}
	if len(results.Entries) != 2 {
		t.Fatalf("entries = %#v, want 2", results.Entries)
	}
	var sawCovering, sawArbitrary bool
	for _, entry := range results.Entries {
		if entry.Check == wrapped {
			if entry.Outcome != baton.CheckOutcomePass ||
				!baton.CheckCommandCovers(declared, entry.Check) {
				t.Fatalf("covering entry = %#v", entry)
			}
			sawCovering = true
		}
		if entry.Check == arbitrary {
			if entry.Outcome != baton.CheckOutcomePass {
				t.Fatalf("arbitrary entry = %#v", entry)
			}
			sawArbitrary = true
		}
	}
	if !sawCovering || !sawArbitrary {
		t.Fatalf("entries = %#v, want both the wrapped and arbitrary calls recorded verbatim", results.Entries)
	}
}

// TestWorkVerificationCheckEvidenceIncompleteRefusesPassInTurn pins S5-A3
// evidence (c): a pass claim with no recorded pass covering a declared check
// is refused in-turn (session stays alive, one bounded correction is
// accounted), naming the uncovered check; the identical situation under a
// fail decision is accepted unchecked (point 4: non-pass verdicts stay
// sayable without producing provenance).
func TestWorkVerificationCheckEvidenceIncompleteRefusesPassInTurn(t *testing.T) {
	declared := "echo covered-check"

	t.Run("pass with no covering entry is refused in-turn", func(t *testing.T) {
		invocation := checkEvidenceInvocationFixture(t, []string{declared})
		session, err := newToolSession(invocation)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		before := session.submitCorrections
		arguments := workVerificationSubmitArguments(t, invocation.Request.InvocationID, DecisionPass)
		result := executeToolJSON(t, session, "submit", "sworn_submit", arguments)
		if !result.Failed || !strings.Contains(string(result.Content), "CHECK_EVIDENCE_INCOMPLETE") {
			t.Fatalf("refusal = %#v, want CHECK_EVIDENCE_INCOMPLETE", result)
		}
		if session.submitCorrections != before+1 {
			t.Fatalf("submitCorrections = %d, want %d", session.submitCorrections, before+1)
		}
		if terminal, _ := session.terminated(); terminal {
			t.Fatal("session terminated on an in-turn correction")
		}

		// The worker can now run the declared check and resubmit inside the
		// same turn (recordCheckEvidence stands in for session.bash's own
		// pass-through, see TestWorkVerificationCheckEvidenceIsDriverAuthored).
		session.recordCheckEvidence(declared, []byte("covered-check\n"), 0, nil)
		retry := executeToolJSON(t, session, "submit-2", "sworn_submit", arguments)
		if retry.Failed {
			t.Fatalf("resubmit after running the declared check failed: %s", retry.Content)
		}
	})

	t.Run("fail decision bypasses the completeness gate", func(t *testing.T) {
		invocation := checkEvidenceInvocationFixture(t, []string{declared})
		session, err := newToolSession(invocation)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		arguments := workVerificationSubmitArguments(t, invocation.Request.InvocationID, DecisionFail)
		result := executeToolJSON(t, session, "submit", "sworn_submit", arguments)
		if result.Failed {
			t.Fatalf("fail-decision submit was refused: %s", result.Content)
		}
	})
}

// TestCheckResultOutcomeClassifiesExactlyWhatWasObserved pins the outcome
// doc comment on CheckResultEntry: a nonzero exit with no harness error is
// fail (the command ran and said no), a context deadline is timeout, an
// OUTPUT_OVERFLOW is overflow, and any other harness-side ContractError is
// fail with its code as the diagnostic - never a bare, uninformative string.
// For PROCESS_START_FAILED carrying a valid sandbox_start.* Detail envelope,
// it preserves the key=value structured diagnostic.
func TestCheckResultOutcomeClassifiesExactlyWhatWasObserved(t *testing.T) {
	if outcome, diagnostic := checkResultOutcome(0, nil); outcome != baton.CheckOutcomePass || diagnostic != "" {
		t.Fatalf("pass = (%q, %q)", outcome, diagnostic)
	}
	if outcome, diagnostic := checkResultOutcome(3, nil); outcome != baton.CheckOutcomeFail || diagnostic == "" {
		t.Fatalf("nonzero exit = (%q, %q)", outcome, diagnostic)
	}
	if outcome, _ := checkResultOutcome(0, context.DeadlineExceeded); outcome != baton.CheckOutcomeTimeout {
		t.Fatalf("deadline exceeded outcome = %q", outcome)
	}
	if outcome, diagnostic := checkResultOutcome(0, fail("OUTPUT_OVERFLOW")); outcome != baton.CheckOutcomeOverflow ||
		diagnostic != "OUTPUT_OVERFLOW" {
		t.Fatalf("overflow = (%q, %q)", outcome, diagnostic)
	}
	if outcome, diagnostic := checkResultOutcome(0, fail("PROCESS_TREE_NOT_QUIESCENT")); outcome != baton.CheckOutcomeFail ||
		diagnostic != "PROCESS_TREE_NOT_QUIESCENT" {
		t.Fatalf("harness error = (%q, %q)", outcome, diagnostic)
	}
	if outcome, diagnostic := checkResultOutcome(0, fail("PROCESS_START_FAILED")); outcome != baton.CheckOutcomeFail ||
		diagnostic != "PROCESS_START_FAILED" {
		t.Fatalf("bare PROCESS_START_FAILED = (%q, %q)", outcome, diagnostic)
	}
	startErr := failSandboxStart("sandbox_start.bwrap_exec_start", syscall.EACCES)
	wantDiag := `PROCESS_START_FAILED detail={"check":"sandbox_start.bwrap_exec_start","cause":"permission denied"}`
	if outcome, diagnostic := checkResultOutcome(0, startErr); outcome != baton.CheckOutcomeFail ||
		diagnostic != wantDiag {
		t.Fatalf("sandbox start failure = (%q, %q), want (%q, %q)", outcome, diagnostic, baton.CheckOutcomeFail, wantDiag)
	}
	invalidDetailErr := &ContractError{Code: "PROCESS_START_FAILED", Detail: `{"unknown":"field"}`}
	if outcome, diagnostic := checkResultOutcome(0, invalidDetailErr); outcome != baton.CheckOutcomeFail ||
		diagnostic != "PROCESS_START_FAILED" {
		t.Fatalf("invalid detail PROCESS_START_FAILED = (%q, %q)", outcome, diagnostic)
	}
}

// TestWorkVerificationCheckEvidenceIncompleteCarriesSandboxDetail pins S4-A1:
// a CHECK_EVIDENCE_INCOMPLETE refusal additively names the sandbox_start.*
// check and bounded cause when the declared check's last recorded attempt
// failed with PROCESS_START_FAILED.
func TestWorkVerificationCheckEvidenceIncompleteCarriesSandboxDetail(t *testing.T) {
	declared := "echo covered-check"

	extractDetail := func(t *testing.T, content []byte) submissionRefusalDetail {
		t.Helper()
		const prefix = "error:CHECK_EVIDENCE_INCOMPLETE detail="
		text := string(content)
		if !strings.HasPrefix(text, prefix) {
			t.Fatalf("content %q does not have prefix %q", text, prefix)
		}
		var detail submissionRefusalDetail
		if err := json.Unmarshal([]byte(strings.TrimPrefix(text, prefix)), &detail); err != nil {
			t.Fatalf("failed to decode submissionRefusalDetail: %v", err)
		}
		return detail
	}

	t.Run("last attempt was PROCESS_START_FAILED with errno cause", func(t *testing.T) {
		invocation := checkEvidenceInvocationFixture(t, []string{declared})
		session, err := newToolSession(invocation)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		startErr := failSandboxStart("sandbox_start.bwrap_exec_start", syscall.EACCES)
		session.recordCheckEvidence(declared+" || true", nil, 0, startErr)

		arguments := workVerificationSubmitArguments(t, invocation.Request.InvocationID, DecisionPass)
		result := executeToolJSON(t, session, "submit", "sworn_submit", arguments)
		if !result.Failed {
			t.Fatal("submit succeeded, want refusal")
		}
		detail := extractDetail(t, result.Content)
		if detail.Check != "submit.check_evidence_incomplete" ||
			detail.Field != "checks" ||
			detail.Bound != "check_command_covers" ||
			len(detail.Paths) != 1 || detail.Paths[0] != declared {
			t.Fatalf("unexpected standard refusal fields: %#v", detail)
		}
		if detail.SandboxCheck != "sandbox_start.bwrap_exec_start" {
			t.Fatalf("sandbox_check = %q, want sandbox_start.bwrap_exec_start", detail.SandboxCheck)
		}
		if detail.SandboxCause != "permission denied" {
			t.Fatalf("sandbox_cause = %q, want permission denied", detail.SandboxCause)
		}
	})

	t.Run("last attempt was PROCESS_START_FAILED with engine cause", func(t *testing.T) {
		invocation := checkEvidenceInvocationFixture(t, []string{declared})
		session, err := newToolSession(invocation)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		startErr := failSandboxStartCause("sandbox_start.process_group_handshake_read", "leader exit 1")
		session.recordCheckEvidence(declared, nil, 0, startErr)

		arguments := workVerificationSubmitArguments(t, invocation.Request.InvocationID, DecisionPass)
		result := executeToolJSON(t, session, "submit", "sworn_submit", arguments)
		if !result.Failed {
			t.Fatal("submit succeeded, want refusal")
		}
		detail := extractDetail(t, result.Content)
		if detail.SandboxCheck != "sandbox_start.process_group_handshake_read" {
			t.Fatalf("sandbox_check = %q, want sandbox_start.process_group_handshake_read", detail.SandboxCheck)
		}
		if detail.SandboxCause != "leader exit 1" {
			t.Fatalf("sandbox_cause = %q, want leader exit 1", detail.SandboxCause)
		}
	})

	t.Run("prior attempt was sandbox failure but last attempt was nonzero exit", func(t *testing.T) {
		invocation := checkEvidenceInvocationFixture(t, []string{declared})
		session, err := newToolSession(invocation)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		startErr := failSandboxStart("sandbox_start.bwrap_exec_start", syscall.EPERM)
		session.recordCheckEvidence(declared, nil, 0, startErr)
		session.recordCheckEvidence(declared, []byte("command failed"), 1, nil)

		arguments := workVerificationSubmitArguments(t, invocation.Request.InvocationID, DecisionPass)
		result := executeToolJSON(t, session, "submit", "sworn_submit", arguments)
		if !result.Failed {
			t.Fatal("submit succeeded, want refusal")
		}
		detail := extractDetail(t, result.Content)
		if detail.SandboxCheck != "" || detail.SandboxCause != "" {
			t.Fatalf("sandbox detail = (%q, %q), want empty because last attempt was exit 1", detail.SandboxCheck, detail.SandboxCause)
		}
	})

	t.Run("no attempt recorded omits sandbox detail", func(t *testing.T) {
		invocation := checkEvidenceInvocationFixture(t, []string{declared})
		session, err := newToolSession(invocation)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()

		arguments := workVerificationSubmitArguments(t, invocation.Request.InvocationID, DecisionPass)
		result := executeToolJSON(t, session, "submit", "sworn_submit", arguments)
		if !result.Failed {
			t.Fatal("submit succeeded, want refusal")
		}
		detail := extractDetail(t, result.Content)
		if detail.SandboxCheck != "" || detail.SandboxCause != "" {
			t.Fatalf("sandbox detail = (%q, %q), want empty because no attempt recorded", detail.SandboxCheck, detail.SandboxCause)
		}
	})
}

// TestTruncateCheckCommandKeepsTheHeadAndStaysWithinBound pins correction 1:
// a script longer than baton.MaxCheckCommandBytes is truncated (never left
// to poison EncodeCheckResults with an opaque failure), keeping the head so
// CheckCommandCovers's prefix match still holds against a short declared
// check.
func TestTruncateCheckCommandKeepsTheHeadAndStaysWithinBound(t *testing.T) {
	declared := "echo covered-check"
	long := declared + strings.Repeat(" x", 2_000)
	truncated := truncateCheckCommand(long)
	if len(truncated) > baton.MaxCheckCommandBytes {
		t.Fatalf("truncated length = %d, want <= %d", len(truncated), baton.MaxCheckCommandBytes)
	}
	if !strings.HasSuffix(truncated, checkCommandTruncationMarker) {
		t.Fatalf("truncated = %q, want the truncation marker", truncated)
	}
	if !baton.CheckCommandCovers(declared, truncated) {
		t.Fatalf("truncated command %q no longer covers %q", truncated, declared)
	}
}

// TestAppendCheckEvidenceLockedEvictsOldestFirst pins correction 6/point 6: a
// session that records past checkEvidenceEntryLimit calls keeps its most
// recent entries, never dropping the tail - the declared checks a verifier
// runs are conventionally its last calls.
func TestAppendCheckEvidenceLockedEvictsOldestFirst(t *testing.T) {
	session := &toolSession{}
	total := checkEvidenceEntryLimit + 10
	for index := 0; index < total; index++ {
		session.recordCheckEvidence(
			"echo call-"+itoa(index), []byte("output\n"), 0, nil,
		)
	}
	entries := session.snapshotCheckEvidence()
	if len(entries) != checkEvidenceEntryLimit {
		t.Fatalf("entries = %d, want %d", len(entries), checkEvidenceEntryLimit)
	}
	if entries[0].Check != "echo call-10" {
		t.Fatalf("oldest surviving entry = %q, want echo call-10 (the first 10 evicted)", entries[0].Check)
	}
	last := "echo call-" + itoa(total-1)
	if entries[len(entries)-1].Check != last {
		t.Fatalf("newest entry = %q, want %q", entries[len(entries)-1].Check, last)
	}
}
