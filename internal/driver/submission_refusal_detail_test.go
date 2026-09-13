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

// floorSummaryFixture and floorDetailFixture are compact, non-empty
// summary/detail text that never self-declares as a probe, so tests in this
// file can compose a submission that reaches the refusal branch under test
// rather than tripping A2 first.
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
		{"bare probe exact (the 2026-09-12 Captain receipt body)", "probe", true, "known_probe_declaration"},
		{"honest probe-ordering prefix does not match bare probe", "Probe ordering is documented in the design's section three, well past the floor.", false, ""},
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
	invocation.RecoveryStepHook = func(context.Context, RecoveryStepKind, *SubmitRefusal) error { return nil }
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
	invocation.RecoveryStepHook = func(context.Context, RecoveryStepKind, *SubmitRefusal) error { return nil }
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
	var lastRefusal *SubmitRefusal
	invocation.RecoveryStepHook = func(_ context.Context, kind RecoveryStepKind, refusal *SubmitRefusal) error {
		if kind != RecoveryStepSubmissionCorrection {
			t.Fatalf("reservation kind = %s", kind)
		}
		reservations++
		lastRefusal = refusal
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

	missingPlan := map[string]any{
		"schema_version": SubmissionSchemaVersion,
		"invocation_id":  invocation.Request.InvocationID,
		"responsibility": string(PlannerProposal),
		"summary":        floorSummaryFixture,
		"detail":         floorDetailFixture,
		// plan omitted: a planner_proposal requires Plan != nil, so this
		// still-refused substantive case (SUBMISSION_SHAPE_MISMATCH)
		// replaces the removed A3 content-floor refusal this fixture used.
	}
	res := executeToolJSON(t, session, "shape-mismatch", "sworn_submit", map[string]any{"submission": missingPlan})
	requireSubmissionRefusalDetail(t, res.Content, "SUBMISSION_SHAPE_MISMATCH", "submit.validate", "plan", "")
	if reservations != 1 {
		t.Fatalf("reservations = %d, want 1", reservations)
	}
	if lastRefusal == nil || lastRefusal.Code != "SUBMISSION_SHAPE_MISMATCH" || lastRefusal.Detail == "" {
		t.Fatalf("reserved refusal = %#v, want the exact SUBMISSION_SHAPE_MISMATCH refusal", lastRefusal)
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

// requireSubmissionRefusalExpected asserts an INVALID_FIELD submit.decode
// refusal names both the engine-known key and the expected wire shape
// (#306).
func requireSubmissionRefusalExpected(t *testing.T, content []byte, wantField, wantExpected string) {
	t.Helper()
	requireSubmissionRefusalDetail(t, content, "INVALID_FIELD", "submit.decode", wantField, "")
	prefix := "error:INVALID_FIELD detail="
	var detail submissionRefusalDetail
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(content), prefix)), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Expected != wantExpected {
		t.Fatalf("content = %q, expected = %q, want %q", content, detail.Expected, wantExpected)
	}
}

// TestToolSubmitNamesWrongTypedKnownFieldAndExpectedShape pins #306: a
// known key carrying the wrong JSON type refuses INVALID_FIELD naming that
// key and the shape the schema wants, at every nesting level, instead of
// naming only the containing object (root not-an-object) or nothing at all
// (the pre-fix fieldless INVALID_SUBMISSION for a detail sent as {path}).
func TestToolSubmitNamesWrongTypedKnownFieldAndExpectedShape(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.RecoveryStepHook = func(context.Context, RecoveryStepKind, *SubmitRefusal) error { return nil }
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

	// The observed r3 refusal: the submission member itself was not an
	// object (a stringified document).
	res := executeToolJSON(t, session, "root-string", "sworn_submit", map[string]any{
		"submission": "{\"schema_version\":\"sworn.submission/v1\"}",
	})
	requireSubmissionRefusalExpected(t, res.Content, "submission", "object")

	// The #307 shape: detail written to a file and sent as a path object.
	detailObject := fullShape()
	detailObject["detail"] = map[string]any{"path": "/tmp/design.md"}
	res = executeToolJSON(t, session, "detail-object", "sworn_submit", map[string]any{"submission": detailObject})
	requireSubmissionRefusalExpected(t, res.Content, "detail", "string")

	summaryArray := fullShape()
	summaryArray["summary"] = []any{"a", "b"}
	res = executeToolJSON(t, session, "summary-array", "sworn_submit", map[string]any{"submission": summaryArray})
	requireSubmissionRefusalExpected(t, res.Content, "summary", "string")

	planString := fullShape()
	planString["plan"] = "/tmp/plan.json"
	res = executeToolJSON(t, session, "plan-string", "sworn_submit", map[string]any{"submission": planString})
	requireSubmissionRefusalExpected(t, res.Content, "plan", "object")

	planByteCount := fullShape()
	planByteCount["plan"] = map[string]any{
		"byte_count": "1",
		"digest":     Digest([]byte("x")),
		"bytes":      base64.StdEncoding.EncodeToString([]byte("x")),
	}
	res = executeToolJSON(t, session, "plan-byte-count", "sworn_submit", map[string]any{"submission": planByteCount})
	requireSubmissionRefusalExpected(t, res.Content, "plan.byte_count", "integer")

	contractsArray := fullShape()
	contractsArray["contracts"] = []any{}
	res = executeToolJSON(t, session, "contracts-array", "sworn_submit", map[string]any{"submission": contractsArray})
	requireSubmissionRefusalExpected(t, res.Content, "contracts", "object")

	contractsEntryDigest := fullShape()
	contractsEntryDigest["contracts"] = map[string]any{
		"docs/x.md": map[string]any{
			"byte_count": 1,
			"digest":     7,
			"bytes":      base64.StdEncoding.EncodeToString([]byte("x")),
		},
	}
	res = executeToolJSON(t, session, "contracts-entry-digest", "sworn_submit", map[string]any{"submission": contractsEntryDigest})
	requireSubmissionRefusalExpected(t, res.Content, "contracts entry.digest", "string")

	decisionOutcome := fullShape()
	decisionOutcome["responsibility"] = string(CaptainReview)
	decisionOutcome["decision"] = map[string]any{"outcome": 1}
	res = executeToolJSON(t, session, "decision-outcome", "sworn_submit", map[string]any{"submission": decisionOutcome})
	requireSubmissionRefusalExpected(t, res.Content, "decision.outcome", "string")

	// An unknown key still names only the containing object, never the
	// worker-authored key, and carries no expected shape.
	unknown := fullShape()
	unknown["bogus_field"] = "x"
	res = executeToolJSON(t, session, "decode-unknown", "sworn_submit", map[string]any{"submission": unknown})
	requireSubmissionRefusalDetail(t, res.Content, "UNKNOWN_FIELD", "submit.decode", "submission", "")
	if strings.Contains(string(res.Content), "expected") {
		t.Fatalf("UNKNOWN_FIELD carried an expected shape: %s", res.Content)
	}

	terminated, terminalErr := session.terminated()
	if terminated || terminalErr != nil {
		t.Fatalf("session terminated after named refusals alone: terminated=%v err=%v", terminated, terminalErr)
	}
}

// TestSubmissionIsUnattachedPointerMatchesObservedBodiesAndAdmitsDocuments
// pins #307's predicate: the two observed "see the file" design bodies
// refuse, a real document that merely opens with the phrase does not once
// it is past the size bound, and honest prose that mentions attachments
// mid-body is never matched.
func TestSubmissionIsUnattachedPointerMatchesObservedBodiesAndAdmitsDocuments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		detail  string
		wantHit bool
	}{
		{"observed attempt 4", "See detail file: full TL;DR mapping A1-A6 to the sections of /tmp/design.md written this turn.", true},
		{"observed attempt 7", "See attached detail: full design TL;DR mapping every acceptance identifier to its section.", true},
		{"mixed case and leading whitespace", "  SEE THE ATTACHED file /home/sworn/design.md for the full design.", true},
		{"attached colon", "Attached: /tmp/design.md", true},
		{"past the size bound is a document", "See attached section headings below.\n" + strings.Repeat("A real design paragraph with substance. ", 120), false},
		{"mid-body mention never matches", "The design keeps the existing seal path. See attached notes for the migration order, which are reproduced in full in section four below.", false},
		{"empty never matches", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hit, bound := submissionIsUnattachedPointer(tc.detail)
			if hit != tc.wantHit {
				t.Fatalf("submissionIsUnattachedPointer(%q) = (%v, %q), want hit=%v", tc.detail, hit, bound, tc.wantHit)
			}
			if hit && bound != "unattached_file_pointer" {
				t.Fatalf("bound = %q", bound)
			}
		})
	}
	if len(strings.Repeat("A real design paragraph with substance. ", 120)) <= submissionPointerMaxBytes {
		t.Fatal("fixture does not exceed submissionPointerMaxBytes")
	}
}

// TestToolSubmitRefusesUnattachedPointerDetailThroughTheLivePath pins #307
// end to end: a pointer body refuses SUBMISSION_UNATTACHED_POINTER through
// the live submit path and never seals.
func TestToolSubmitRefusesUnattachedPointerDetailThroughTheLivePath(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.RecoveryStepHook = func(context.Context, RecoveryStepKind, *SubmitRefusal) error { return nil }
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	pointer := map[string]any{
		"schema_version": SubmissionSchemaVersion,
		"invocation_id":  invocation.Request.InvocationID,
		"responsibility": string(ImplementerDesign),
		"summary":        floorSummaryFixture,
		"detail":         "See attached detail: full design TL;DR mapping every acceptance identifier A1-A6 to /tmp/design.md.",
	}
	res := executeToolJSON(t, session, "pointer", "sworn_submit", map[string]any{"submission": pointer})
	requireSubmissionRefusalDetail(t, res.Content, "SUBMISSION_UNATTACHED_POINTER", "submit.detail_pointer", "detail", "unattached_file_pointer")

	submitted, _ := session.submitted()
	if submitted || session.handoff() != nil {
		t.Fatalf("pointer payload sealed: submitted=%v handoff=%#v", submitted, session.handoff())
	}
}
