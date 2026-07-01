package reqverify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeVerifier returns a canned reply for model dispatch.
type fakeVerifier struct {
	reply string
	cost  float64
}

func (f fakeVerifier) Verify(context.Context, string, string) (string, float64, int64, int64, error) {
	return f.reply, f.cost, 0, 0, nil
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

// --- parseGrades tests ---

func TestParseGrades_AllPass(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-test", Index: 2, Content: "WHEN Y THE SYSTEM SHALL do Z."},
	}
	reply := `Some analysis preamble.

## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): PASS`

	grades, err := parseGrades(reply, acs)
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

func TestParseGrades_MixedPassFail(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-test", Index: 2, Content: "WHEN Y THE SYSTEM SHALL do Z and also do W."},
	}
	reply := `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — singular [bundles two distinct actions]`

	grades, err := parseGrades(reply, acs)
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
}

func TestParseGrades_AmbiguousBreach(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-test", Index: 2, Content: "THE SYSTEM SHALL display the data appropriately."},
	}
	reply := `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — ambiguous [could mean any format]`

	grades, err := parseGrades(reply, acs)
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
	if grades[1].Violation.Characteristic != CharAmbiguous {
		t.Errorf("AC 2: want characteristic 'ambiguous', got %q", grades[1].Violation.Characteristic)
	}
}

func TestParseGrades_IncompleteBreach(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL display the dashboard."},
		{SliceID: "S01-test", Index: 2, Content: "THE SYSTEM SHALL notify the user."},
	}
	reply := `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — incomplete [lacks trigger condition]`

	grades, err := parseGrades(reply, acs)
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
	if grades[1].Violation.Characteristic != "incomplete" {
		t.Errorf("AC 2: want characteristic 'complete', got %q", grades[1].Violation.Characteristic)
	}
}

func TestParseGrades_MissingResultsBlocks(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
	}
	reply := `Some analysis without a RESULTS section.`

	_, err := parseGrades(reply, acs)
	if err == nil {
		t.Fatal("want error for missing RESULTS section, got nil")
	}
}

func TestParseGrades_FailClosedOnMissingAC(t *testing.T) {
	acs := []AC{
		{SliceID: "S01-test", Index: 1, Content: "THE SYSTEM SHALL do X."},
		{SliceID: "S01-test", Index: 2, Content: "THE SYSTEM SHALL do Y."},
	}
	reply := `## RESULTS

AC 1 (S01-test): PASS`

	grades, err := parseGrades(reply, acs)
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

	reply := `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): PASS`

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

func TestRun_WithViolations(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL do X.
- [ ] WHEN Y THE SYSTEM SHALL do Z and also do W.
`)

	reply := `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — singular [bundles two actions]`

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

	reply := `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — ambiguous [could mean any format]`

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

	reply := `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — incomplete [lacks trigger condition]`

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

	report, err := Run(context.Background(), releaseDir, fakeVerifier{reply: `## RESULTS

AC 1 (S01-test): PASS`}, sampleSystemPrompt)
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

func (errorVerifier) Verify(context.Context, string, string) (string, float64, int64, int64, error) {
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
