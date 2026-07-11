package implement

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/reqverify"
	"github.com/swornagent/sworn/internal/state"
)

// ---------------------------------------------------------------------------
// Fake verifier for reqverify tests
// ---------------------------------------------------------------------------

// fakeVerifier returns a canned STRUCTURED reply (S02: the DoR grading call is
// schema-constrained — Verify carries the emit schema and returns the
// reqverify-results JSON object, not a `## RESULTS` prose section). For passing
// tests every AC grades PASS; failing tests include a FAIL grade.
type fakeVerifier struct {
	reply string
}

func (f fakeVerifier) Verify(_ context.Context, _, _ string, _ []byte) (string, float64, int64, int64, error) {
	return f.reply, 0.0, 0, 0, nil
}

// gradeRec is one record in the structured reqverify-results emission.
type gradeRec struct {
	SliceID        string `json:"slice_id"`
	ACIndex        int    `json:"ac_index"`
	Status         string `json:"status"`
	Characteristic string `json:"characteristic,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func gradesReply(recs []gradeRec) string {
	b, _ := json.Marshal(map[string]any{"results": recs})
	return string(b)
}

// passingReply returns a structured reply that grades every AC as PASS.
func passingReply(acs []reqverify.AC) string {
	recs := make([]gradeRec, 0, len(acs))
	for _, ac := range acs {
		recs = append(recs, gradeRec{SliceID: ac.SliceID, ACIndex: ac.Index, Status: "PASS"})
	}
	return gradesReply(recs)
}

// failingReply returns a structured reply with one FAIL grade for the given slice.
func failingReply(acs []reqverify.AC, sliceID string) string {
	recs := make([]gradeRec, 0, len(acs))
	for _, ac := range acs {
		if ac.SliceID == sliceID {
			recs = append(recs, gradeRec{SliceID: ac.SliceID, ACIndex: ac.Index, Status: "FAIL", Characteristic: "ambiguous", Reason: "Contains multiple interpretations"})
		} else {
			recs = append(recs, gradeRec{SliceID: ac.SliceID, ACIndex: ac.Index, Status: "PASS"})
		}
	}
	return gradesReply(recs)
}

// unsupportedVerifier models a model with no structured-output capability —
// drives the AC-03 declared-deferral arm of CheckDoR.
type unsupportedVerifier struct{}

func (unsupportedVerifier) Verify(_ context.Context, _, _ string, _ []byte) (string, float64, int64, int64, error) {
	return "", 0, 0, 0, reqverify.ErrStructuredUnsupported
}

// ---------------------------------------------------------------------------// Release directory fixture builder
// ---------------------------------------------------------------------------

// writeRTMFixture creates a minimal release directory that passes the RTM
// traceability check. Tests can apply modifiers to introduce breaks.
func writeRTMFixture(t *testing.T, mods ...func(dir string)) string {
	t.Helper()
	dir := t.TempDir()

	// intake.md with needs and a release goal.
	intake := `---
title: Test intake
---

# Release Intake: test-release

## Release goal

The release goal text for testing.

## Needs

- N-01: First need for testing
- N-02: Second need for testing

## Other section

Some content.
`
	os.WriteFile(filepath.Join(dir, "intake.md"), []byte(intake), 0644)

	// index.md with release benefit and slices.
	index := `---
title: Test board
tracks:
  - id: T1-test
    slices: [S06-target-slice]
    worktree_branch: track/test/T1-test
---

# Board

## Release summary

- **Goal**: the release goal from index
- **Target version / integration branch**: release/v0.1.0

## Release benefit

The release delivers value to users.

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| S06-target-slice | T1 | test outcome | planned | human | [spec](./S06-target-slice/spec.md) | — |
`
	os.WriteFile(filepath.Join(dir, "index.md"), []byte(index), 0644)

	// S06-target-slice with spec.md and status.json (full trace).
	sliceDir := filepath.Join(dir, "S06-target-slice")
	os.MkdirAll(sliceDir, 0755)

	spec := `---
title: S06-target-slice
---

# Slice: S06-target-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a release has a need, THE SYSTEM SHALL link it to N-01.
- [ ] WHEN a test runs, THE SYSTEM SHALL verify N-02.

## Required tests

- **Unit**: internal/implement/ready_test.go
- **Integration**: exercise the command end-to-end
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)

	status := `{
  "slice_id": "S06-target-slice",
  "release": "test-release",
  "track": "T1-test",
  "state": "planned",
  "release_benefit": "The release delivers value to users.",
  "verification": {"result": "pending"}
}`
	os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644)
	for _, mod := range mods {
		mod(dir)
	}

	return dir
}

// rtmWithOrphanedAC makes the target slice's AC cite a non-existent need.
func rtmWithOrphanedAC(dir string) {
	sliceDir := filepath.Join(dir, "S06-target-slice")
	spec := `---
title: S06-target-slice
---

# Slice: S06-target-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN something happens, THE SYSTEM SHALL do something (N-999).

## Required tests

- **Unit**: internal/implement/ready_test.go
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)
}

// ---------------------------------------------------------------------------
// Validation fixture helpers
// ---------------------------------------------------------------------------

// writeValidationRecord creates a status.json with a validation record for
// the target slice. If humanRatified is false, the record is model-only.
func writeValidationRecord(t *testing.T, releaseDir, sliceID string, humanRatified bool) {
	t.Helper()
	v := state.ValidationRecord{
		PositiveScenarios: []string{"User clicks save, form persists."},
		NegativeScenarios: []string{"User clicks save while offline, system shows error."},
		BenefitHypothesis: "Saves user time by persisting forms.",
	}
	if humanRatified {
		v.HumanRatified = true
		v.RatifiedBy = "test-user"
		v.RatifiedAt = "2026-06-16T12:00:00Z"
	}

	dir := filepath.Join(releaseDir, sliceID)
	s := state.Status{
		Schema:       "https://example.com/schemas/baton/slice-status-v1.json",
		SliceID:      sliceID,
		Release:      "test-release",
		Track:        "T1-test",
		State:        state.Planned,
		Validation:   v,
		Verification: state.Verification{Result: "pending"},
	}
	if err := state.Write(filepath.Join(dir, "status.json"), &s); err != nil {
		t.Fatal(err)
	}
}

// fv is a convenience fake verifier that passes all ACs.
var fvPass = fakeVerifier{}

// fvFail is a fake verifier that produces a FAIL for the target slice.
var fvFail = fakeVerifier{}

func init() {
	// We can't pre-build the reply here because we don't know the ACs yet.
	// We'll create them in each test by calling writeFixture.
}

// makeReleaseDir creates a full release directory with all necessary artefacts
// for the CheckDoR test. Returns the release dir path and a fake verifier that
// matches the test's needs.
func makeReleaseDir(t *testing.T, withValidation bool, failReqlify bool) (releaseDir string, verifier reqverify.Verifier) {
	t.Helper()
	releaseDir = writeRTMFixture(t)

	// Write validation record if requested.
	if withValidation {
		writeValidationRecord(t, releaseDir, "S06-target-slice", true)
	} else {
		writeValidationRecord(t, releaseDir, "S06-target-slice", false)
	}

	// Create verifier.
	var v fakeVerifier
	if failReqlify {
		// We need to know the ACs to build a failing reply. Let's build it dynamically.
		// For convenience, the failing verifier is set up in the test itself.
		v = fakeVerifier{}
	} else {
		// Build passing reply from the fixture's ACs
		acs := []reqverify.AC{
			{SliceID: "S06-target-slice", Index: 1, Content: "WHEN a release has a need, THE SYSTEM SHALL link it to N-01."},
			{SliceID: "S06-target-slice", Index: 2, Content: "WHEN a test runs, THE SYSTEM SHALL verify N-02."},
		}
		v = fakeVerifier{reply: passingReply(acs)}
	}

	return releaseDir, v
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCheckDoR_AllPass(t *testing.T) {
	releaseDir, v := makeReleaseDir(t, true, false)

	result, err := CheckDoR(context.Background(), releaseDir, "S06-target-slice", v)
	if err != nil {
		t.Fatalf("CheckDoR: unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected DoR to pass, got:\n%s", DoRErrorSummary(result))
	}
	if !result.RTMPassed {
		t.Error("RTM should pass in fully-traced fixture")
	}
	if !result.ReqverifyPassed {
		t.Error("reqverify should pass with passing verifier")
	}
	if !result.ReqvalidatePassed {
		t.Error("reqvalidate should pass with human-ratified validation")
	}
}

func TestCheckDoR_RTMFailure(t *testing.T) {
	// Create a fixture with a broken trace.
	releaseDir := writeRTMFixture(t, rtmWithOrphanedAC)

	// Add validation (passing)
	writeValidationRecord(t, releaseDir, "S06-target-slice", true)

	// Create passing verifier for reqverify
	acs := []reqverify.AC{
		{SliceID: "S06-target-slice", Index: 1, Content: "WHEN something happens, THE SYSTEM SHALL do something (N-999)."},
	}
	v := fakeVerifier{reply: passingReply(acs)}

	result, err := CheckDoR(context.Background(), releaseDir, "S06-target-slice", v)
	if err != nil {
		t.Fatalf("CheckDoR: unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected DoR to fail due to RTM violations, but it passed")
	}
	if result.RTMPassed {
		t.Fatal("expected RTM to fail, but it passed")
	}
	if len(result.RTMFailures) == 0 {
		t.Fatal("expected at least one RTM failure")
	}

	summary := DoRErrorSummary(result)
	if !strings.Contains(summary, "RTM") {
		t.Errorf("DoRErrorSummary should mention RTM, got:\n%s", summary)
	}
}

func TestCheckDoR_ReqverifyFailure(t *testing.T) {
	releaseDir, _ := makeReleaseDir(t, true, false)

	// Build a failing verifier
	acs := []reqverify.AC{
		{SliceID: "S06-target-slice", Index: 1, Content: "WHEN a release has a need, THE SYSTEM SHALL link it to N-01."},
		{SliceID: "S06-target-slice", Index: 2, Content: "WHEN a test runs, THE SYSTEM SHALL verify N-02."},
	}
	v := fakeVerifier{reply: failingReply(acs, "S06-target-slice")}

	result, err := CheckDoR(context.Background(), releaseDir, "S06-target-slice", v)
	if err != nil {
		t.Fatalf("CheckDoR: unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected DoR to fail due to reqverify violations, but it passed")
	}
	if result.ReqverifyPassed {
		t.Fatal("expected reqverify to fail, but it passed")
	}
	if len(result.ReqverifyFailures) == 0 {
		t.Fatal("expected at least one reqverify failure")
	}

	summary := DoRErrorSummary(result)
	if !strings.Contains(summary, "Requirements verification") {
		t.Errorf("DoRErrorSummary should mention requirements verification, got:\n%s", summary)
	}
}

func TestCheckDoR_ReqvalidateFailure(t *testing.T) {
	// Create fixture WITHOUT human-ratified validation.
	releaseDir, v := makeReleaseDir(t, false, false)

	result, err := CheckDoR(context.Background(), releaseDir, "S06-target-slice", v)
	if err != nil {
		t.Fatalf("CheckDoR: unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected DoR to fail due to reqvalidate violations, but it passed")
	}
	if result.ReqvalidatePassed {
		t.Fatal("expected reqvalidate to fail, but it passed")
	}
	if len(result.ReqvalidateFailures) == 0 {
		t.Fatal("expected at least one reqvalidate failure")
	}

	summary := DoRErrorSummary(result)
	if !strings.Contains(summary, "Requirements validation") {
		t.Errorf("DoRErrorSummary should mention requirements validation, got:\n%s", summary)
	}
}

func TestCheckDoR_FailClosedNoVerifier(t *testing.T) {
	releaseDir := writeRTMFixture(t)

	// We need to ensure the slice dir has status.json with validation to
	// isolate the verifier-nil test.
	os.MkdirAll(filepath.Join(releaseDir, "S06-target-slice"), 0755)
	writeValidationRecord(t, releaseDir, "S06-target-slice", true)

	result, err := CheckDoR(context.Background(), releaseDir, "S06-target-slice", nil)
	if err != nil {
		t.Fatalf("CheckDoR: unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected DoR to fail closed with nil verifier, but it passed")
	}
	if result.ReqverifyPassed {
		t.Fatal("expected reqverify to fail with nil verifier, but it passed")
	}
}

// TestCheckDoR_CapabilityAbsentDeferral is AC-03 at the DoR integration point
// (ready.go CheckDoR -> reqverify.Run): a verifier whose model cannot emit
// structured output routes through the "not evaluated" arm as a DECLARED Rule 2
// deferral — DoR fails closed (ReqverifyPassed=false) with a capability-naming
// reason, never a silent pass and never a hard error.
func TestCheckDoR_CapabilityAbsentDeferral(t *testing.T) {
	releaseDir := writeRTMFixture(t)
	os.MkdirAll(filepath.Join(releaseDir, "S06-target-slice"), 0755)
	writeValidationRecord(t, releaseDir, "S06-target-slice", true)

	result, err := CheckDoR(context.Background(), releaseDir, "S06-target-slice", unsupportedVerifier{})
	if err != nil {
		t.Fatalf("capability-absent must be a deferral, not a CheckDoR error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected DoR to fail closed on a capability-absent model, but it passed")
	}
	if result.ReqverifyPassed {
		t.Fatal("expected reqverify to fail closed (not evaluated) on capability-absent, but it passed")
	}
	joined := strings.Join(result.ReqverifyFailures, " ")
	if !strings.Contains(joined, "structured-output capability") {
		t.Errorf("reqverify failure should name the missing capability, got: %q", joined)
	}
}

func TestCheckDoR_FailClosedOnUnreadableDir(t *testing.T) {
	_, err := CheckDoR(context.Background(), "/nonexistent/release", "S06-target-slice", fvPass)
	if err == nil {
		t.Fatal("expected error for nonexistent release directory, got nil")
	}
	if !strings.Contains(err.Error(), "dor") {
		t.Errorf("expected error prefix 'dor', got: %v", err)
	}
}

func TestDoRErrorSummary_NilResult(t *testing.T) {
	if s := DoRErrorSummary(nil); s != "" {
		t.Errorf("expected empty string for nil result, got: %q", s)
	}
}

func TestDoRErrorSummary_PassingResult(t *testing.T) {
	r := &DoRResult{Passed: true, RTMPassed: true, ReqverifyPassed: true, ReqvalidatePassed: true}
	if s := DoRErrorSummary(r); s != "" {
		t.Errorf("expected empty string for passing result, got: %q", s)
	}
}

func TestDoRErrorSummary_AllFailing(t *testing.T) {
	r := &DoRResult{
		Passed:              false,
		RTMPassed:           false,
		ReqverifyPassed:     false,
		ReqvalidatePassed:   false,
		RTMFailures:         []string{"orphaned need N-01"},
		ReqverifyFailures:   []string{"ambiguous (AC 1): contains multiple meanings"},
		ReqvalidateFailures: []string{"human ratification missing"},
	}
	s := DoRErrorSummary(r)
	if !strings.Contains(s, "RTM") {
		t.Error("summary should contain RTM section")
	}
	if !strings.Contains(s, "Requirements verification") {
		t.Error("summary should contain Requirements verification section")
	}
	if !strings.Contains(s, "Requirements validation") {
		t.Error("summary should contain Requirements validation section")
	}
	if !strings.Contains(s, "orphaned need") {
		t.Error("summary should contain RTM failure details")
	}
	if !strings.Contains(s, "ambiguous") {
		t.Error("summary should contain reqverify failure details")
	}
	if !strings.Contains(s, "human ratification") {
		t.Error("summary should contain reqvalidate failure details")
	}
}
