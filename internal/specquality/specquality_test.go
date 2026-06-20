package specquality

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSpec is a test helper that writes a spec.md file to a slice directory.
func writeSpec(t *testing.T, dir, sliceID, examples, criteria string) {
	t.Helper()
	sliceDir := filepath.Join(dir, sliceID)
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	spec := "# Slice: " + sliceID + "\n\n"
	if criteria != "" {
		spec += "## Acceptance checks\n\n" + criteria + "\n\n"
	}
	if examples != "" {
		spec += "## Acceptance examples\n\n" + examples + "\n"
	}

	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeStatus is a test helper that writes a minimal status.json.
func writeStatus(t *testing.T, dir, sliceID string) {
	t.Helper()
	status := `{
  "$schema": "https://example.com/schemas/baton/slice-status-v1.json",
  "slice_id": "` + sliceID + `",
  "state": "planned",
  "verification": {
    "result": "pending"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, sliceID, "status.json"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRun_Pass_SoundAndComplete(t *testing.T) {
	// A sound+complete example set — every expected output is consistent with
	// the criteria, and mutations would be caught.
	dir := t.TempDir()

	writeSpec(t, dir, "S01-test-slice",
		`- name: "valid-ears"
  input: "release with all EARS-format ACs"
  expected: "sworn lint ac exits 0 and reports all ACs well-formed"

- name: "free-form-ac"
  input: "release with a free-form AC"
  expected: "sworn lint ac exits 1 naming the slice and line"`,
		`- [ ] WHEN a release has only well-formed EARS ACs sworn lint ac SHALL exit 0
- [ ] WHEN a release has a free-form AC sworn lint ac SHALL exit 1 naming the slice and line`)

	writeStatus(t, dir, "S01-test-slice")

	report, err := Run(dir, 0.5)
	if err != nil {
		t.Fatal(err)
	}

	if !report.Passed {
		t.Errorf("expected report to pass, got violations: %+v", report.Results[0].Violations)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	r := report.Results[0]
	if r.Soundness != 1.0 {
		t.Errorf("expected soundness 1.0, got %.2f", r.Soundness)
	}
	if r.Completeness <= 0 {
		t.Errorf("expected completeness > 0, got %.2f", r.Completeness)
	}
	if r.ExampleCount != 2 {
		t.Errorf("expected 2 examples, got %d", r.ExampleCount)
	}
}

func TestRun_Fail_NoExamples(t *testing.T) {
	// A slice with no acceptance examples should fail.
	dir := t.TempDir()

	writeSpec(t, dir, "S01-no-examples",
		"",
		`- [ ] WHEN something happens THE SYSTEM SHALL do something`)

	writeStatus(t, dir, "S01-no-examples")

	report, err := Run(dir, 0.5)
	if err != nil {
		t.Fatal(err)
	}

	if report.Passed {
		t.Error("expected report to fail (no examples), but it passed")
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if len(report.Results[0].Violations) == 0 {
		t.Error("expected violations for missing examples")
	}
}

func TestRun_Fail_LowCompleteness(t *testing.T) {
	// An example set whose expected outputs are too vague should score low
	// on completeness and fall below the threshold.
	dir := t.TempDir()

	writeSpec(t, dir, "S01-vague-examples",
		`- name: "generic-pass"
  input: "any input"
  expected: "works correctly"

- name: "generic-fail"
  input: "any bad input"
  expected: "fails"`,
		`- [ ] WHEN input is valid THE SYSTEM SHALL work`)

	writeStatus(t, dir, "S01-vague-examples")

	report, err := Run(dir, 0.5)
	if err != nil {
		t.Fatal(err)
	}

	// The examples are vague — mutations won't be caught well.
	// Completeness should be low (likely 0 since "works correctly" doesn't
	// contain any keywords for mutation operators to work on).
	if report.Passed {
		t.Error("expected report to fail (low completeness), but it passed")
	}

	hadCompletenessViolation := false
	for _, v := range report.Results[0].Violations {
		if contains(v.Reason, "completeness") && contains(v.Reason, "threshold") {
			hadCompletenessViolation = true
			break
		}
	}
	if !hadCompletenessViolation {
		t.Errorf("expected completeness threshold violation, got violations: %v", report.Results[0].Violations)
	}
}

func TestRun_Fail_UnsoundExpectation(t *testing.T) {
	// An example whose expected output contradicts the criteria (expects
	// failure when criteria only describe a pass case).
	dir := t.TempDir()

	writeSpec(t, dir, "S01-unsound",
		`- name: "contradicts-criteria"
  input: "a valid release"
  expected: "sworn lint ac exits 1 with violations"`,
		`- [ ] WHEN a release is valid sworn lint ac SHALL exit 0`)

	writeStatus(t, dir, "S01-unsound")

	report, err := Run(dir, 0.5)
	if err != nil {
		t.Fatal(err)
	}

	if report.Passed {
		t.Error("expected report to fail (unsound example), but it passed")
	}

	hadSoundnessViolation := false
	for _, v := range report.Results[0].Violations {
		if contains(v.Reason, "expects failure") {
			hadSoundnessViolation = true
			break
		}
	}
	if !hadSoundnessViolation {
		t.Errorf("expected soundness violation, got violations: %v", report.Results[0].Violations)
	}
}

func TestRun_MultipleSlices_MixedResults(t *testing.T) {
	dir := t.TempDir()

	// Sound+complete slice.
	writeSpec(t, dir, "S01-good",
		`- name: "pass-case"
  input: "valid input"
  expected: "sworn lint ac exits 0"`,
		`- [ ] WHEN input is valid sworn lint ac SHALL exit 0`)

	writeStatus(t, dir, "S01-good")

	// Slice with no examples.
	writeSpec(t, dir, "S02-no-examples", "", `- [ ] WHEN something THE SYSTEM SHALL respond`)

	writeStatus(t, dir, "S02-no-examples")

	report, err := Run(dir, 0.5)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}

	// S01 should pass or have only completeness violations.
	r1 := report.Results[0]
	if r1.SliceID != "S01-good" && r1.SliceID != "S02-no-examples" {
		t.Fatalf("unexpected slice order: first is %s", r1.SliceID)
	}

	// At least one slice should have violations (S02-no-examples).
	hasFail := false
	for _, r := range report.Results {
		if len(r.Violations) > 0 {
			hasFail = true
		}
	}
	if !hasFail {
		t.Error("expected at least one slice to have violations")
	}

	// Report should not pass — at least one slice fails.
	if report.Passed {
		t.Error("expected overall report to fail (at least one slice has violations)")
	}
}

func TestRun_EmptyRelease(t *testing.T) {
	dir := t.TempDir()
	// No slice directories at all.

	report, err := Run(dir, 0.5)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(report.Results))
	}
	if !report.Passed {
		t.Error("expected empty release to pass (no violations)")
	}
}

func TestParseExamples_Structured(t *testing.T) {
	spec := `## Acceptance examples

- name: "pass-case"
  input: "valid input"
  expected: "exits 0"

- name: "fail-case"
  input: "invalid input"
  expected: "exits 1"
`
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.md")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	examples := parseExamples(specPath)
	if len(examples) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(examples))
	}
	if examples[0].Name != "pass-case" {
		t.Errorf("expected name 'pass-case', got %q", examples[0].Name)
	}
	if examples[0].Input != "valid input" {
		t.Errorf("expected input 'valid input', got %q", examples[0].Input)
	}
	if examples[0].Expected != "exits 0" {
		t.Errorf("expected expected 'exits 0', got %q", examples[0].Expected)
	}
}

func TestParseExamples_Shorthand(t *testing.T) {
	spec := `## Acceptance examples

- valid release → exits 0
- invalid release → exits 1
`
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.md")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	examples := parseExamples(specPath)
	if len(examples) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(examples))
	}
	if examples[0].Input != "valid release" {
		t.Errorf("expected input 'valid release', got %q", examples[0].Input)
	}
	if examples[0].Expected != "exits 0" {
		t.Errorf("expected expected 'exits 0', got %q", examples[0].Expected)
	}
}

func TestParseExamples_None(t *testing.T) {
	spec := `## Acceptance checks

- [ ] WHEN something happens THE SYSTEM SHALL respond
`
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.md")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	examples := parseExamples(specPath)
	if len(examples) != 0 {
		t.Errorf("expected 0 examples, got %d", len(examples))
	}
}

func TestMutationOperators_FlipExitCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"exits 0", "exits 1"},
		{"exits 1", "exits 0"},
		{"exit 0", "exit 1"},
		{"exit code 0", "exit code 1"},
		{"no numbers here", ""},
	}
	for _, tt := range tests {
		result := mutateFlipExitCode(tt.input)
		if result != tt.expected {
			t.Errorf("mutateFlipExitCode(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestMutationOperators_NegateAssertion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"results in PASS", "results in FAIL"},
		{"ends with FAIL", "ends with PASS"},
		{"no keyword match", ""},
	}
	for _, tt := range tests {
		result := mutateNegateAssertion(tt.input)
		if result != tt.expected {
			t.Errorf("mutateNegateAssertion(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractCommandRefs(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"sworn lint ac exits 0", []string{"sworn lint", "sworn lint ac"}},
		{"runs sworn verify", []string{"sworn verify"}},
		{"no commands here", nil},
		{"sworn specquality and sworn reqvalidate", []string{"sworn reqvalidate", "sworn specquality"}},	}
	for _, tt := range tests {
		result := extractCommandRefs(tt.input)
		if !stringSliceEqual(result, tt.expected) {
			t.Errorf("extractCommandRefs(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestPrint_EmptyReport(t *testing.T) {
	r := Report{Threshold: 0.5, Passed: true}
	output := Print(r)
	if output == "" {
		t.Error("expected non-empty output for empty report")
	}
}

func TestPrintCompact(t *testing.T) {
	r := Report{
		Threshold: 0.5,
		Passed:    true,
		Results: []SliceResult{
			{SliceID: "S01-a", Soundness: 1.0, Completeness: 0.8, ExampleCount: 2},
		},
	}
	output := PrintCompact(r)
	if !contains(output, "PASSED") {
		t.Errorf("expected PASSED in compact output, got: %s", output)
	}
}

// contains reports whether substr is in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

// containsStr is an inline strings.Contains for test helpers.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// stringSliceEqual compares two string slices.
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}