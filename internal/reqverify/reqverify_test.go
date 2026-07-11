package reqverify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeVerifier returns a canned structured reply for the schema-constrained
// model dispatch (S02: Verify now carries the emit schema and returns the
// structured JSON, not prose).
type fakeVerifier struct {
	reply string
	cost  float64
}

func (f fakeVerifier) Verify(context.Context, string, string, []byte) (string, float64, int64, int64, error) {
	return f.reply, f.cost, 0, 0, nil
}

// unsupportedVerifier models a model with no structured-output capability: its
// Verify returns ErrStructuredUnsupported, driving the AC-03 declared-deferral
// arm.
type unsupportedVerifier struct{}

func (unsupportedVerifier) Verify(context.Context, string, string, []byte) (string, float64, int64, int64, error) {
	return "", 0, 0, 0, ErrStructuredUnsupported
}

// gradesReply builds a structured reqverify-results emission (the shape the
// model emits against reqverifyResultsSchema) from per-AC records — the
// structured replacement for the old `## RESULTS` prose fixtures.
func gradesReply(recs ...resultRecord) string {
	b, _ := json.Marshal(resultsEnvelope{Results: recs})
	return string(b)
}

// writeFixture creates a slice spec.md under a temp release directory.
func writeFixture(t *testing.T, releaseDir, sliceID, spec string) {
	t.Helper()
	dir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sampleSystemPrompt = "You are a requirements quality verifier."

// --- parseACs tests ---

func TestParseACs_ExtractsCheckboxLines(t *testing.T) {
	spec := `---
title: S01-test
---

# Slice: S01-test

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist.
- [ ] IF the input is invalid THEN THE SYSTEM SHALL show an error.

## Required tests

None.`

	acs := parseACs(spec, "S01-test")
	if len(acs) != 3 {
		t.Fatalf("want 3 ACs, got %d", len(acs))
	}
	for i, ac := range acs {
		if ac.Index != i+1 {
			t.Errorf("AC %d: want Index %d, got %d", i, i+1, ac.Index)
		}
		if ac.SliceID != "S01-test" {
			t.Errorf("AC %d: want SliceID S01-test, got %s", i, ac.SliceID)
		}
	}
}

func TestParseACs_SkipsNonCheckboxLines(t *testing.T) {
	spec := `---
title: S02-test
---

# Slice: S02-test

## Acceptance checks

- [ ] WHEN x THE SYSTEM SHALL y.
Some random text that isn't a checkbox.
- [ ] THE SYSTEM SHALL z.

## Required tests

- [ ] Some other checkbox outside the section.`

	acs := parseACs(spec, "S02-test")
	if len(acs) != 2 {
		t.Fatalf("want 2 ACs, got %d", len(acs))
	}
}

func TestParseACs_StopsAtNextHeading(t *testing.T) {
	spec := `## Acceptance checks

- [ ] AC one.

## Some other section

- [ ] This should NOT be included.`

	acs := parseACs(spec, "S03-test")
	if len(acs) != 1 {
		t.Fatalf("want 1 AC, got %d", len(acs))
	}
}

func TestParseACs_CaseInsensitiveHeader(t *testing.T) {
	spec := `## ACCEPTANCE CHECKS

- [ ] THE SYSTEM SHALL do something.`

	acs := parseACs(spec, "S04-test")
	if len(acs) != 1 {
		t.Fatalf("want 1 AC, got %d", len(acs))
	}
}

func TestParseACs_EmptyChecksSection(t *testing.T) {
	spec := `## Acceptance checks

## Required tests`

	acs := parseACs(spec, "S05-test")
	if len(acs) != 0 {
		t.Fatalf("want 0 ACs, got %d", len(acs))
	}
}

// --- extractACs tests ---

func TestExtractACs_ReadsAllSlices(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-first", `## Acceptance checks

- [ ] AC one.
- [ ] AC two.
`)
	writeFixture(t, releaseDir, "S02-second", `## Acceptance checks

- [ ] AC three.
`)
	writeFixture(t, releaseDir, "S03-third", `## Acceptance checks

`)

	acs, err := extractACs(releaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(acs) != 3 {
		t.Fatalf("want 3 ACs, got %d", len(acs))
	}
}

func TestExtractACs_SkipsNonSliceDirs(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] AC one.
`)
	os.MkdirAll(filepath.Join(releaseDir, ".hidden"), 0o755)
	os.MkdirAll(filepath.Join(releaseDir, "assets"), 0o755)

	acs, err := extractACs(releaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(acs) != 1 {
		t.Fatalf("want 1 AC, got %d", len(acs))
	}
}

// --- buildPayload tests ---

func TestBuildPayload_FormatsCorrectly(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-first", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-first", Index: 2, Content: "WHEN Y THE SYSTEM SHALL do Z."},
		{SliceID: "S02-second", Index: 1, Content: "THE SYSTEM SHALL do W."},
	}
	payload := buildPayload(acs)

	if !strings.Contains(payload, "### Slice: S01-first") {
		t.Errorf("payload missing S01-first header")
	}
	if !strings.Contains(payload, "### Slice: S02-second") {
		t.Errorf("payload missing S02-second header")
	}
	if !strings.Contains(payload, "AC 1: THE SYSTEM SHALL do X.") {
		t.Errorf("payload missing AC 1 content")
	}
	if !strings.Contains(payload, "AC 2: WHEN Y THE SYSTEM SHALL do Z.") {
		t.Errorf("payload missing AC 2 content")
	}
}

// --- parseStructuredGrades tests (S02: structured object, not `## RESULTS`) ---

func TestParseStructuredGrades_AllPass(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-test", Index: 2, Content: "WHEN Y THE SYSTEM SHALL do Z."},
	}
	reply := gradesReply(
		resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"},
		resultRecord{SliceID: "S01-test", ACIndex: 2, Status: "PASS"},
	)

	grades, err := parseStructuredGrades(reply, acs)
	if err != nil {
		t.Fatal(err)
	}
	if len(grades) != 2 {
		t.Fatalf("want 2 grades, got %d", len(grades))
	}
	for i, g := range grades {
		if !g.Passed {
			t.Errorf("grade %d: want PASS, got FAIL", i)
		}
	}
}

func TestParseStructuredGrades_MixedPassFail(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-test", Index: 2, Content: "WHEN Y THE SYSTEM SHALL do Z and also do W."},
	}
	reply := gradesReply(
		resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"},
		resultRecord{SliceID: "S01-test", ACIndex: 2, Status: "FAIL", Characteristic: "singular", Reason: "bundles two distinct actions"},
	)

	grades, err := parseStructuredGrades(reply, acs)
	if err != nil {
		t.Fatal(err)
	}
	if len(grades) != 2 {
		t.Fatalf("want 2 grades, got %d", len(grades))
	}
	if !grades[0].Passed {
		t.Errorf("AC 1: want PASS")
	}
	if grades[1].Passed {
		t.Errorf("AC 2: want FAIL")
	}
	if grades[1].Violation == nil {
		t.Fatal("AC 2: want Violation, got nil")
	}
	if grades[1].Violation.Characteristic != CharSingular {
		t.Errorf("AC 2: want characteristic 'singular', got %q", grades[1].Violation.Characteristic)
	}
	if grades[1].Violation.Reason != "bundles two distinct actions" {
		t.Errorf("AC 2: reason = %q", grades[1].Violation.Reason)
	}
}

func TestParseStructuredGrades_AmbiguousBreach(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-test", Index: 2, Content: "THE SYSTEM SHALL display the data appropriately."},
	}
	reply := gradesReply(
		resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"},
		resultRecord{SliceID: "S01-test", ACIndex: 2, Status: "FAIL", Characteristic: "ambiguous", Reason: "could mean any format"},
	)

	grades, err := parseStructuredGrades(reply, acs)
	if err != nil {
		t.Fatal(err)
	}
	if grades[1].Violation == nil || grades[1].Violation.Characteristic != CharAmbiguous {
		t.Errorf("AC 2: want characteristic 'ambiguous', got %+v", grades[1].Violation)
	}
}

func TestParseStructuredGrades_IncompleteBreach(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL display the dashboard."},
		{SliceID: "S01-test", Index: 2, Content: "THE SYSTEM SHALL notify the user."},
	}
	reply := gradesReply(
		resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"},
		resultRecord{SliceID: "S01-test", ACIndex: 2, Status: "FAIL", Characteristic: "incomplete", Reason: "lacks trigger condition"},
	)

	grades, err := parseStructuredGrades(reply, acs)
	if err != nil {
		t.Fatal(err)
	}
	if grades[1].Violation == nil || grades[1].Violation.Characteristic != "incomplete" {
		t.Errorf("AC 2: want characteristic 'incomplete', got %+v", grades[1].Violation)
	}
}

// TestParseStructuredGrades_MalformedBlocks is the structured equivalent of the
// old "missing ## RESULTS section" BLOCK: an emission that is not a
// results-bearing object fails the lightweight validate and blocks.
func TestParseStructuredGrades_MalformedBlocks(t *testing.T) {
	acs := []AC{{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."}}

	cases := map[string]string{
		"not an object":     `"just a string"`,
		"no results key":    `{"analysis":"some prose"}`,
		"results not array": `{"results":"nope"}`,
		"bad status":        `{"results":[{"slice_id":"S01-test","ac_index":1,"status":"MAYBE"}]}`,
		"non-positive idx":  `{"results":[{"slice_id":"S01-test","ac_index":0,"status":"PASS"}]}`,
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseStructuredGrades(reply, acs); err == nil {
				t.Fatalf("want BLOCK error for %s, got nil", name)
			}
		})
	}
}

func TestParseStructuredGrades_FailClosedOnMissingAC(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-test", Index: 2, Content: "THE SYSTEM SHALL do Y."},
	}
	// AC 2 absent from a well-formed results array — fail-closed FAIL, not a BLOCK.
	reply := gradesReply(resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"})

	grades, err := parseStructuredGrades(reply, acs)
	if err != nil {
		t.Fatal(err) // missing AC is not a parse error — it's a failing grade
	}
	if len(grades) != 2 {
		t.Fatalf("want 2 grades, got %d", len(grades))
	}
	if !grades[0].Passed {
		t.Errorf("AC 1: want PASS")
	}
	if grades[1].Passed {
		t.Errorf("AC 2 (missing from response): want FAIL (fail-closed), got PASS")
	}
}

// --- Run integration tests ---

func TestRun_AllPass(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist the form.
`)

	reply := gradesReply(
		resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"},
		resultRecord{SliceID: "S01-test", ACIndex: 2, Status: "PASS"},
	)

	report, err := Run(context.Background(), releaseDir, fakeVerifier{reply: reply}, sampleSystemPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasViolations() {
		t.Fatalf("want no violations, got %d", len(report.Violations))
	}
	if report.PassedACs != 2 {
		t.Errorf("want 2 passed, got %d", report.PassedACs)
	}
	if !report.FreshContext {
		t.Errorf("want FreshContext=true")
	}
}

// TestRun_StructuredNoProseResults is AC-02: the Grok DoR failure now passes.
// The model emits a valid structured results object with NO `## RESULTS` prose
// anywhere — the gate parses the structured object, not a prose section.
func TestRun_StructuredNoProseResults(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
`)

	reply := gradesReply(resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"})
	if strings.Contains(reply, "## RESULTS") {
		t.Fatal("fixture precondition: structured reply must not contain a `## RESULTS` prose section")
	}

	report, err := Run(context.Background(), releaseDir, fakeVerifier{reply: reply}, sampleSystemPrompt)
	if err != nil {
		t.Fatalf("Run over a prose-marker-less structured reply must PASS, got: %v", err)
	}
	if report.HasViolations() || report.PassedACs != 1 {
		t.Errorf("want 1 passed / 0 violations, got passed=%d violations=%d", report.PassedACs, len(report.Violations))
	}
}

// TestRun_CapabilityAbsentDeferred is AC-03 for the DoR gate: a model that
// cannot emit structured output yields a DECLARED Rule 2 deferral
// (report.Deferred), never a hard error and never a silent pass.
func TestRun_CapabilityAbsentDeferred(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
`)

	report, err := Run(context.Background(), releaseDir, unsupportedVerifier{}, sampleSystemPrompt)
	if err != nil {
		t.Fatalf("capability-absent must be a deferral, not an error: %v", err)
	}
	if !report.Deferred {
		t.Fatal("want report.Deferred=true for a capability-absent model")
	}
	if !strings.Contains(report.DeferredReason, "structured-output capability") {
		t.Errorf("DeferredReason should name the missing capability, got: %q", report.DeferredReason)
	}
}

func TestRun_WithViolations(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL do X.
- [ ] WHEN Y THE SYSTEM SHALL do Z and also do W.
`)

	reply := gradesReply(
		resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"},
		resultRecord{SliceID: "S01-test", ACIndex: 2, Status: "FAIL", Characteristic: "singular", Reason: "bundles two actions"},
	)

	report, err := Run(context.Background(), releaseDir, fakeVerifier{reply: reply}, sampleSystemPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasViolations() {
		t.Fatal("want violations, got none")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("want 1 violation, got %d", len(report.Violations))
	}
	if report.Violations[0].Characteristic != CharSingular {
		t.Errorf("want characteristic 'singular', got %q", report.Violations[0].Characteristic)
	}
	if report.Violations[0].SliceID != "S01-test" {
		t.Errorf("want slice S01-test, got %s", report.Violations[0].SliceID)
	}
	if report.FailedACs != 1 {
		t.Errorf("want 1 failed, got %d", report.FailedACs)
	}
}

func TestRun_AmbiguousViolation(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL do X.
- [ ] THE SYSTEM SHALL display the data appropriately.
`)

	reply := gradesReply(
		resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"},
		resultRecord{SliceID: "S01-test", ACIndex: 2, Status: "FAIL", Characteristic: "ambiguous", Reason: "could mean any format"},
	)

	report, err := Run(context.Background(), releaseDir, fakeVerifier{reply: reply}, sampleSystemPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasViolations() {
		t.Fatal("want violations, got none")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("want 1 violation, got %d", len(report.Violations))
	}
	if report.Violations[0].Characteristic != CharAmbiguous {
		t.Errorf("want characteristic ambiguous, got %q", report.Violations[0].Characteristic)
	}
	if report.FailedACs != 1 {
		t.Errorf("want 1 failed, got %d", report.FailedACs)
	}
}

func TestRun_IncompleteViolation(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] THE SYSTEM SHALL notify the user.
`)

	reply := gradesReply(
		resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"},
		resultRecord{SliceID: "S01-test", ACIndex: 2, Status: "FAIL", Characteristic: "incomplete", Reason: "lacks trigger condition"},
	)

	report, err := Run(context.Background(), releaseDir, fakeVerifier{reply: reply}, sampleSystemPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasViolations() {
		t.Fatal("want violations, got none")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("want 1 violation, got %d", len(report.Violations))
	}
	if report.Violations[0].Characteristic != "incomplete" {
		t.Errorf("want characteristic complete, got %q", report.Violations[0].Characteristic)
	}
	if report.FailedACs != 1 {
		t.Errorf("want 1 failed, got %d", report.FailedACs)
	}
}

func TestRun_NoACsFailsClosed(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

## Required tests

None.`)

	report, err := Run(context.Background(), releaseDir, fakeVerifier{reply: ""}, sampleSystemPrompt)
	if err == nil {
		t.Fatal("want error for release with no evaluable ACs (fail closed), got nil")
	}
	if !strings.Contains(err.Error(), "no evaluable acceptance criteria") {
		t.Errorf("want 'no evaluable acceptance criteria' error, got %v", err)
	}
	if report.TotalACs != 0 {
		t.Errorf("want 0 total ACs, got %d", report.TotalACs)
	}
}

// writeSpecJSONFixture creates a slice spec.json (spec-v1 record) under a
// temp release directory.
func writeSpecJSONFixture(t *testing.T, releaseDir, sliceID, specJSON string) {
	t.Helper()
	dir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtractACs_PrefersSpecJSON(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeSpecJSONFixture(t, releaseDir, "S01-test", `{
  "schema_version": 1,
  "slice_id": "S01-test",
  "release": "test-release",
  "acceptance_criteria": [
    {"id": "AC-1", "type": "ubiquitous", "text": "THE SYSTEM SHALL do X."},
    {"id": "AC-2", "type": "event-driven", "ears_keyword": "When", "text": "WHEN Y THE SYSTEM SHALL do Z."}
  ]
}`)
	// A stale spec.md alongside spec.json must NOT win.
	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] stale markdown AC that must be ignored
`)

	acs, err := extractACs(releaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(acs) != 2 {
		t.Fatalf("want 2 ACs from spec.json, got %d", len(acs))
	}
	if acs[0].Content != "THE SYSTEM SHALL do X." || acs[0].Index != 1 {
		t.Errorf("unexpected first AC: %+v", acs[0])
	}
	if acs[1].SliceID != "S01-test" || acs[1].Index != 2 {
		t.Errorf("unexpected second AC: %+v", acs[1])
	}
}

func TestExtractACs_MalformedSpecJSONFailsClosed(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeSpecJSONFixture(t, releaseDir, "S01-test", `{not json`)

	if _, err := extractACs(releaseDir); err == nil {
		t.Fatal("want error for malformed spec.json, got nil")
	}
}

func TestRun_SpecJSONDispatchesModel(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeSpecJSONFixture(t, releaseDir, "S01-test", `{
  "schema_version": 1,
  "slice_id": "S01-test",
  "release": "test-release",
  "acceptance_criteria": [
    {"id": "AC-1", "type": "ubiquitous", "text": "THE SYSTEM SHALL do X."}
  ]
}`)

	report, err := Run(context.Background(), releaseDir,
		fakeVerifier{reply: gradesReply(resultRecord{SliceID: "S01-test", ACIndex: 1, Status: "PASS"})},
		sampleSystemPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalACs != 1 || report.PassedACs != 1 {
		t.Errorf("want 1 AC graded PASS, got total=%d passed=%d", report.TotalACs, report.PassedACs)
	}
}

func TestRun_ModelErrorBlocks(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL do X.
`)

	_, err := Run(context.Background(), releaseDir, errorVerifier{}, sampleSystemPrompt)
	if err == nil {
		t.Fatal("want error from model dispatch, got nil")
	}
}

// errorVerifier returns an error on dispatch, simulating a model failure.
type errorVerifier struct{}

func (errorVerifier) Verify(context.Context, string, string, []byte) (string, float64, int64, int64, error) {
	return "", 0, 0, 0, fmt.Errorf("model unavailable")
}

// --- Print / PrintCompact tests ---

func TestPrint_Formatting(t *testing.T) {
	report := Report{
		Grades: []Grade{
			{SliceID: "S01-test", ACIndex: 1, ACContent: "AC one", Passed: true},
			{SliceID: "S02-test", ACIndex: 1, ACContent: "AC two", Passed: false,
				Violation: &Violation{SliceID: "S02-test", ACIndex: 1, ACContent: "AC two",
					Characteristic: CharAmbiguous, Reason: "could mean X or Y"}},
		},
		Violations: []Violation{
			{SliceID: "S02-test", ACIndex: 1, ACContent: "AC two",
				Characteristic: CharAmbiguous, Reason: "could mean X or Y"},
		},
		TotalACs:     2,
		PassedACs:    1,
		FailedACs:    1,
		FreshContext: true,
	}

	output := Print(report)
	if !strings.Contains(output, "Violations:") {
		t.Errorf("print missing Violations section")
	}
	if !strings.Contains(output, "Per-AC grades:") {
		t.Errorf("print missing Per-AC grades section")
	}
	if !strings.Contains(output, "fresh-context") {
		t.Errorf("print missing fresh-context indicator")
	}
}

func TestPrintCompact_Passed(t *testing.T) {
	output := PrintCompact(Report{TotalACs: 3, PassedACs: 3, FailedACs: 0})
	if !strings.Contains(output, "PASSED") {
		t.Errorf("want PASSED, got: %s", output)
	}
}

func TestPrintCompact_Failed(t *testing.T) {
	output := PrintCompact(Report{TotalACs: 3, PassedACs: 2, FailedACs: 1,
		Violations: []Violation{{SliceID: "S01", ACIndex: 1}}})
	if !strings.Contains(output, "FAILED") {
		t.Errorf("want FAILED, got: %s", output)
	}
}

func TestPrintCompact_NoACs(t *testing.T) {
	output := PrintCompact(Report{TotalACs: 0})
	if !strings.Contains(output, "no acceptance criteria") {
		t.Errorf("want 'no acceptance criteria', got: %s", output)
	}
}
