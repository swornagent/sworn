package driver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// floorSummaryFixture and floorDetailFixture clear both content floors
// (MinSubmissionSummaryFloorBytes, MinSubmissionDetailFloorBytes) without
// self-declaring as a probe, so tests in this file can compose a submission
// that reaches the refusal branch under test rather than tripping A2 or A3
// first.
const (
	floorSummaryFixture = "Compact responsibility summary padded so this fixture clears the submission content floor for its dedicated A1/A2/A3 refusal-detail regression coverage across every named branch."
	floorDetailFixture  = "Bounded detail padded so this fixture clears the detail content floor for its dedicated A1/A2/A3 refusal-detail regression coverage across every named branch tested here, well past the two-hundred-byte bound.\n"
)

// requireSubmissionRefusalDetail asserts a failed tool result's content
// carries exactly the "error:<code> detail=<envelope>" shape A1 requires,
// with the envelope naming check/field/bound.
func requireSubmissionRefusalDetail(
	t *testing.T,
	content []byte,
	wantCode, wantCheck, wantField, wantBound string,
) {
	t.Helper()
	prefix := "error:" + wantCode + " detail="
	text := string(content)
	if !strings.HasPrefix(text, prefix) {
		t.Fatalf("content = %q, want prefix %q", text, prefix)
	}
	var detail submissionRefusalDetail
	if err := json.Unmarshal([]byte(strings.TrimPrefix(text, prefix)), &detail); err != nil {
		t.Fatalf("content %q did not decode a submissionRefusalDetail: %v", text, err)
	}
	if detail.Check != wantCheck || detail.Field != wantField || detail.Bound != wantBound {
		t.Fatalf(
			"content = %q, detail = %#v, want check=%s field=%s bound=%s",
			text, detail, wantCheck, wantField, wantBound,
		)
	}
}

func planMember(t *testing.T) map[string]any {
	t.Helper()
	body := validPlanBytes()
	return map[string]any{
		"byte_count": int64(len(body)),
		"digest":     Digest(body),
		"bytes":      base64.StdEncoding.EncodeToString(body),
	}
}

// TestSubmissionDeclaresProbeMatchesObservedPayloadsAndAdmitsHonestWork pins
// A2: each real-world probe payload native-lane-honesty observed is caught
// (verbatim and padded past the floor), the contract's own self-label and
// submit-surface phrases are caught, and honest work - including work that
// quotes the same strings mid-document, and the over-match fixture the
// prior design wrongly caught - is admitted.
func TestSubmissionDeclaresProbeMatchesObservedPayloadsAndAdmitsHonestWork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		field   string
		wantHit bool
		bound   string
	}{
		{"bare test exact", "test", true, "known_probe_declaration"},
		{"bare test mixed case and padding trims", "  TEST  ", true, "known_probe_declaration"},
		{"known probe sentence verbatim", "probe: minimal submission to isolate field validation", true, "known_probe_declaration"},
		{
			"known probe sentence padded past the floor still self-declares",
			"isolation test summary padded to reasonable length to avoid floor issues" +
				strings.Repeat(" and then some extra padding text appended after it.", 3),
			true, "known_probe_declaration",
		},
		{"self label colon prefix", "Probe: ad-hoc reconnaissance of the submit surface, not real work at all.", true, "self_label_prefix"},
		{"self label dash prefix", "probe - quick isolated check of the field validator's own behaviour today.", true, "self_label_prefix"},
		{"submit surface phrase", "This is a test of the submit surface, not real work of any kind today.", true, "submit_surface_declaration"},
		{"this is a probe phrase", "This is a probe of the validation path, nothing more than that today.", true, "submit_surface_declaration"},
		{
			"honest test coverage prefix does not match bare test",
			"Test coverage padded well past the one-hundred-twenty-byte submission content floor for its own dedicated regression coverage.",
			false, "",
		},
		{"over-match fixture stays admitted", "Recorded provider-dialect certification probe.", false, ""},
		{
			"mid-document quote does not self-trigger",
			"This design documents that a field equal to \"test\" or a field beginning \"probe:\" self-declares under A2, which is exactly the behaviour this fixture proves by staying admitted here.",
			false, "",
		},
		{"empty field never declares", "", false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			declared, bound := submissionDeclaresProbe(tc.field)
			if declared != tc.wantHit || bound != tc.bound {
				t.Fatalf(
					"submissionDeclaresProbe(%q) = (%v, %q), want (%v, %q)",
					tc.field, declared, bound, tc.wantHit, tc.bound,
				)
			}
		})
	}
}

// TestSubmissionFloorCheckRefusesBelowFloorAndExemptsNonFlooredResponsibilities
// pins A3: each of the five floored responsibilities refuses a summary or
// detail under its own byte floor and names which, while
// captain_plan_review and assembly_verification stay exempt even when both
// fields are tiny.
func TestSubmissionFloorCheckRefusesBelowFloorAndExemptsNonFlooredResponsibilities(t *testing.T) {
	t.Parallel()
	longSummary := strings.Repeat("s", MinSubmissionSummaryFloorBytes)
	longDetail := strings.Repeat("d", MinSubmissionDetailFloorBytes)
	shortSummary := strings.Repeat("s", MinSubmissionSummaryFloorBytes-1)
	shortDetail := strings.Repeat("d", MinSubmissionDetailFloorBytes-1)

	tests := []struct {
		name           string
		responsibility Responsibility
		summary        string
		detail         string
		wantCode       string
		wantField      string
		wantBound      string
	}{
		{"planner below summary floor", PlannerProposal, shortSummary, longDetail, "SUBMISSION_BELOW_FLOOR", "summary", "min_submission_summary_floor_bytes"},
		{"planner below detail floor", PlannerProposal, longSummary, shortDetail, "SUBMISSION_BELOW_FLOOR", "detail", "min_submission_detail_floor_bytes"},
		{"planner exactly at floor admitted", PlannerProposal, longSummary, longDetail, "", "", ""},
		{"assembly verification exempt even when tiny", AssemblyVerification, "x", "y", "", "", ""},
		{"captain plan review exempt even when tiny", CaptainPlanReview, "x", "y", "", "", ""},
		{"implementer design below summary floor", ImplementerDesign, shortSummary, longDetail, "SUBMISSION_BELOW_FLOOR", "summary", "min_submission_summary_floor_bytes"},
		{"captain review below detail floor", CaptainReview, longSummary, shortDetail, "SUBMISSION_BELOW_FLOOR", "detail", "min_submission_detail_floor_bytes"},
		{"work verification below summary floor first", WorkVerification, shortSummary, shortDetail, "SUBMISSION_BELOW_FLOOR", "summary", "min_submission_summary_floor_bytes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := submissionFloorCheck(Submission{
				Responsibility: tc.responsibility,
				Summary:        tc.summary,
				Detail:         tc.detail,
			})
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("submissionFloorCheck(%s) = %v, want nil", tc.name, err)
				}
				return
			}
			var contractErr *ContractError
			if !errors.As(err, &contractErr) || contractErr.Code != tc.wantCode {
				t.Fatalf("submissionFloorCheck(%s) = %v, want code %s", tc.name, err, tc.wantCode)
			}
			var detail submissionRefusalDetail
			if json.Unmarshal([]byte(contractErr.Detail), &detail) != nil ||
				detail.Check != "submit.content_floor" ||
				detail.Field != tc.wantField || detail.Bound != tc.wantBound {
				t.Fatalf(
					"submissionFloorCheck(%s) detail = %s, want field=%s bound=%s",
					tc.name, contractErr.Detail, tc.wantField, tc.wantBound,
				)
			}
		})
	}
}

// TestSubmitEncodeDetailAttachesBoundOnlyForResourceLimit pins Captain
// correction C2: the max_submission_bytes bound is named only when the
// wrapped EncodeSubmission error is actually RESOURCE_LIMIT.
func TestSubmitEncodeDetailAttachesBoundOnlyForResourceLimit(t *testing.T) {
	t.Parallel()
	limited := submitEncodeDetail(&ContractError{Code: "RESOURCE_LIMIT"})
	var contractErr *ContractError
	if !errors.As(limited, &contractErr) || contractErr.Code != "RESOURCE_LIMIT" {
		t.Fatalf("submitEncodeDetail(RESOURCE_LIMIT) = %v", limited)
	}
	var detail submissionRefusalDetail
	if json.Unmarshal([]byte(contractErr.Detail), &detail) != nil ||
		detail.Check != "submit.encode" || detail.Bound != "max_submission_bytes" || detail.Field != "" {
		t.Fatalf("submitEncodeDetail(RESOURCE_LIMIT) detail = %s", contractErr.Detail)
	}

	other := submitEncodeDetail(&ContractError{Code: "INVALID_SUBMISSION"})
	if !errors.As(other, &contractErr) || contractErr.Code != "INVALID_SUBMISSION" {
		t.Fatalf("submitEncodeDetail(INVALID_SUBMISSION) = %v", other)
	}
	var otherDetail submissionRefusalDetail
	if json.Unmarshal([]byte(contractErr.Detail), &otherDetail) != nil ||
		otherDetail.Check != "submit.encode" || otherDetail.Bound != "" {
		t.Fatalf("submitEncodeDetail(INVALID_SUBMISSION) detail = %s, want no bound", contractErr.Detail)
	}
}

// TestTruncateSubmissionScopeLintPathsBoundsEncodedBytesAndMarksTruncation
// pins submit.plan_scope_lint's Paths bound: a short list rides untouched,
// and a list whose encoding would exceed maxSubmissionScopeLintDetailBytes
// is cut and ends with the truncation marker rather than losing everything.
func TestTruncateSubmissionScopeLintPathsBoundsEncodedBytesAndMarksTruncation(t *testing.T) {
	t.Parallel()
	short := []string{"internal/driver", "internal/baton"}
	if got := truncateSubmissionScopeLintPaths(short); len(got) != len(short) {
		t.Fatalf("short list truncated unexpectedly: %v", got)
	}
	var many []string
	for i := 0; i < 200; i++ {
		many = append(many, fmt.Sprintf("internal/package/number/%03d/leaf", i))
	}
	truncated := truncateSubmissionScopeLintPaths(many)
	encoded, err := json.Marshal(truncated)
	if err != nil || len(encoded) > maxSubmissionScopeLintDetailBytes {
		t.Fatalf("truncated paths exceed bound: len=%d err=%v", len(encoded), err)
	}
	if len(truncated) == 0 || truncated[len(truncated)-1] != submissionScopeLintTruncationMarker {
		t.Fatalf("truncated paths missing marker: %v", truncated)
	}
	if len(truncated) >= len(many) {
		t.Fatalf("truncation did not drop any paths: got %d, want < %d", len(truncated), len(many))
	}
}

// TestToolSubmitNamesEveryRefusalBranch pins A1 end to end: every refusal
// branch inside toolSession.submit's chain names its check and (where the
// design calls for one) its field or bound, over one live session's actual
// tool results - not a unit-level construction of the error.
func TestToolSubmitNamesEveryRefusalBranch(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.RecoveryStepHook = func(context.Context, RecoveryStepKind) error { return nil }
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	fullShape := func() map[string]any {
		return map[string]any{
			"schema_version": SubmissionSchemaVersion,
			"invocation_id":  invocation.Request.InvocationID,
			"responsibility": string(PlannerProposal),
			"summary":        floorSummaryFixture,
			"detail":         floorDetailFixture,
		}
	}

	// submit.arguments_decode, reached only by calling submit directly:
	// execute()'s own empty/oversized guard (tools.go) forecloses these two
	// decodeStrict sites from the ordinary tool-call route, since it applies
	// the identical MaxToolArgumentBytes bound one layer earlier. They are
	// still part of the shared submit() chain A1 names, so they are pinned
	// here at the function itself.
	if _, err := session.submit(context.Background(), nil); !IsCode(err, "MISSING_JSON") {
		t.Fatalf("empty arguments = %v, want MISSING_JSON", err)
	} else {
		var contractErr *ContractError
		errors.As(err, &contractErr)
		var detail submissionRefusalDetail
		if json.Unmarshal([]byte(contractErr.Detail), &detail) != nil ||
			detail.Check != "submit.arguments_decode" || detail.Field != "" || detail.Bound != "" {
			t.Fatalf("MISSING_JSON detail = %s", contractErr.Detail)
		}
	}
	oversized := make([]byte, MaxToolArgumentBytes+1)
	for i := range oversized {
		oversized[i] = 'a'
	}
	if _, err := session.submit(context.Background(), oversized); !IsCode(err, "RESOURCE_LIMIT") {
		t.Fatalf("oversized arguments = %v, want RESOURCE_LIMIT", err)
	} else {
		var contractErr *ContractError
		errors.As(err, &contractErr)
		var detail submissionRefusalDetail
		if json.Unmarshal([]byte(contractErr.Detail), &detail) != nil ||
			detail.Check != "submit.arguments_decode" || detail.Bound != "max_tool_argument_bytes" {
			t.Fatalf("RESOURCE_LIMIT detail = %s", contractErr.Detail)
		}
	}
	// submit.arguments_decode reached through the ordinary execute() route
	// with malformed-but-admitted-size JSON.
	res := session.execute(context.Background(), providerToolCall{
		ID: "bad-json", Name: "sworn_submit", Arguments: []byte(`{"submission":`),
	})
	requireSubmissionRefusalDetail(t, res.Content, "INVALID_JSON", "submit.arguments_decode", "", "")

	// submit.root_object
	res = executeToolJSON(t, session, "root-missing", "sworn_submit", map[string]any{})
	requireSubmissionRefusalDetail(t, res.Content, "MISSING_FIELD", "submit.root_object", "submission", "")

	res = executeToolJSON(t, session, "root-unknown", "sworn_submit", map[string]any{
		"submission": map[string]any{}, "extra": 1,
	})
	requireSubmissionRefusalDetail(t, res.Content, "UNKNOWN_FIELD", "submit.root_object", "submission", "")

	// submit.decode
	res = executeToolJSON(t, session, "decode-missing", "sworn_submit", map[string]any{
		"submission": map[string]any{},
	})
	requireSubmissionRefusalDetail(t, res.Content, "MISSING_FIELD", "submit.decode", "schema_version", "")

	unknownSubmission := fullShape()
	unknownSubmission["bogus_field"] = "x"
	res = executeToolJSON(t, session, "decode-unknown", "sworn_submit", map[string]any{
		"submission": unknownSubmission,
	})
	requireSubmissionRefusalDetail(t, res.Content, "UNKNOWN_FIELD", "submit.decode", "submission", "")

	// submit.exact_bytes_path: a plan member naming both path and bytes.
	exactBytesConflict := fullShape()
	exactBytesConflict["plan"] = map[string]any{
		"byte_count": 1,
		"digest":     Digest([]byte("x")),
		"path":       "/tmp/x",
		"bytes":      base64.StdEncoding.EncodeToString([]byte("x")),
	}
	res = executeToolJSON(t, session, "exact-bytes-conflict", "sworn_submit", map[string]any{
		"submission": exactBytesConflict,
	})
	requireSubmissionRefusalDetail(t, res.Content, "INVALID_EXACT_BYTES", "submit.exact_bytes_path", "plan", "")

	// submit.validate: INVALID_VERSION
	badVersion := fullShape()
	badVersion["schema_version"] = "wrong"
	res = executeToolJSON(t, session, "bad-version", "sworn_submit", map[string]any{"submission": badVersion})
	requireSubmissionRefusalDetail(t, res.Content, "INVALID_VERSION", "submit.validate", "schema_version", "")

	// submit.validate: SUBMISSION_SHAPE_MISMATCH naming the first violated
	// member (plan, absent for a planner_proposal).
	res = executeToolJSON(t, session, "shape-mismatch", "sworn_submit", map[string]any{"submission": fullShape()})
	requireSubmissionRefusalDetail(t, res.Content, "SUBMISSION_SHAPE_MISMATCH", "submit.validate", "plan", "")

	// submit.permission_validate: invocation_id disagrees with the bound
	// permission descriptor.
	wrongInvocation := fullShape()
	wrongInvocation["invocation_id"] = "some-other-invocation"
	wrongInvocation["plan"] = planMember(t)
	res = executeToolJSON(t, session, "wrong-invocation", "sworn_submit", map[string]any{
		"submission": wrongInvocation,
	})
	requireSubmissionRefusalDetail(t, res.Content, "SUBMISSION_BINDING_MISMATCH", "submit.permission_validate", "invocation_id", "")

	// submit.yield_first_required: a fresh session's plan bytes must yield
	// first.
	freshPlan := fullShape()
	freshPlan["plan"] = planMember(t)
	res = executeToolJSON(t, session, "yield-first", "sworn_submit", map[string]any{"submission": freshPlan})
	requireSubmissionRefusalDetail(t, res.Content, "YIELD_FIRST_REQUIRED", "submit.yield_first_required", "", "")

	terminated, terminalErr := session.terminated()
	if terminated || terminalErr != nil {
		t.Fatalf("session terminated after named refusals alone: terminated=%v err=%v", terminated, terminalErr)
	}
}

// TestToolSubmitRefusesSelfDeclaredProbePlainAndPaddedPastTheFloor pins A2
// end to end: a bare "test" summary and the padded evidence string both
// refuse as SUBMISSION_DECLARED_PROBE through the live submit path, and
// neither ever seals.
func TestToolSubmitRefusesSelfDeclaredProbePlainAndPaddedPastTheFloor(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.RecoveryStepHook = func(context.Context, RecoveryStepKind) error { return nil }
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	bareTest := map[string]any{
		"schema_version": SubmissionSchemaVersion,
		"invocation_id":  invocation.Request.InvocationID,
		"responsibility": string(PlannerProposal),
		"summary":        "test",
		"detail":         floorDetailFixture,
	}
	res := executeToolJSON(t, session, "bare-test", "sworn_submit", map[string]any{"submission": bareTest})
	requireSubmissionRefusalDetail(t, res.Content, "SUBMISSION_DECLARED_PROBE", "submit.probe_declaration", "summary", "known_probe_declaration")

	padded := map[string]any{
		"schema_version": SubmissionSchemaVersion,
		"invocation_id":  invocation.Request.InvocationID,
		"responsibility": string(PlannerProposal),
		"summary": "isolation test summary padded to reasonable length to avoid floor issues" +
			strings.Repeat(" and then some extra padding text appended after it.", 3),
		"detail": floorDetailFixture,
	}
	res = executeToolJSON(t, session, "padded-probe", "sworn_submit", map[string]any{"submission": padded})
	requireSubmissionRefusalDetail(t, res.Content, "SUBMISSION_DECLARED_PROBE", "submit.probe_declaration", "summary", "known_probe_declaration")

	submitted, _ := session.submitted()
	if submitted || session.handoff() != nil {
		t.Fatalf("probe payload sealed: submitted=%v handoff=%#v", submitted, session.handoff())
	}
}

// TestNamedSubmissionRefusalCostsOneCorrectionNoTryAndCorrectedFollowUpSucceeds
// pins A4: a named refusal (here, A3's floor) accounts exactly one bounded
// correction, keeps the session alive without consuming a dispatch try, and
// a worker acting on the returned field/bound name succeeds on its next
// submission.
func TestNamedSubmissionRefusalCostsOneCorrectionNoTryAndCorrectedFollowUpSucceeds(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	reservations := 0
	invocation.RecoveryStepHook = func(_ context.Context, kind RecoveryStepKind) error {
		if kind != RecoveryStepSubmissionCorrection {
			t.Fatalf("reservation kind = %s", kind)
		}
		reservations++
		return nil
	}
	invocation.recoverableInput = &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "Continue with the approved planner turn.",
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	belowFloor := map[string]any{
		"schema_version": SubmissionSchemaVersion,
		"invocation_id":  invocation.Request.InvocationID,
		"responsibility": string(PlannerProposal),
		"summary":        "short",
		"detail":         "short",
		"plan":           planMember(t),
	}
	res := executeToolJSON(t, session, "below-floor", "sworn_submit", map[string]any{"submission": belowFloor})
	requireSubmissionRefusalDetail(t, res.Content, "SUBMISSION_BELOW_FLOOR", "submit.content_floor", "summary", "min_submission_summary_floor_bytes")
	if reservations != 1 {
		t.Fatalf("reservations = %d, want 1", reservations)
	}
	terminated, terminalErr := session.terminated()
	if terminated || terminalErr != nil {
		t.Fatalf("session terminated after one named refusal: terminated=%v err=%v", terminated, terminalErr)
	}

	corrected := map[string]any{
		"schema_version": SubmissionSchemaVersion,
		"invocation_id":  invocation.Request.InvocationID,
		"responsibility": string(PlannerProposal),
		"summary":        floorSummaryFixture,
		"detail":         floorDetailFixture,
		"plan":           planMember(t),
	}
	res = executeToolJSON(t, session, "corrected", "sworn_submit", map[string]any{"submission": corrected})
	submitted, submitErr := session.submitted()
	if res.Failed || !submitted || submitErr != nil || session.handoff() == nil {
		t.Fatalf("corrected follow-up = %#v, submitted=%v, error=%v", res, submitted, submitErr)
	}
	if reservations != 1 {
		t.Fatalf("reservations after success = %d, want 1", reservations)
	}
}
