package reqvalidate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)

// writeStatus creates a slice's status.json under a temp release directory
// with the given validation record.
func writeStatus(t *testing.T, releaseDir, sliceID string, v state.ValidationRecord) {
	t.Helper()
	dir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := state.Status{
		Schema:     "https://example.com/schemas/baton/slice-status-v1.json",
		SliceID:    sliceID,
		Release:    "test-release",
		Track:      "T1-test",
		State:      state.Planned,
		Validation: v,
	}
	if err := state.Write(filepath.Join(dir, "status.json"), &s); err != nil {
		t.Fatal(err)
	}
}

// --- validateSlice tests ---

func TestValidateSlice_MissingRecordFails(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Write a status.json with no Validation record (zero value).
	writeStatus(t, releaseDir, "S01-test", state.ValidationRecord{})

	violations := validateSlice(releaseDir, "S01-test")
	if len(violations) == 0 {
		t.Fatal("want violations for missing validation record, got none")
	}
	// Should fail at least on human_ratified and missing scenarios.
	if !containsReason(violations, "human_ratified") {
		t.Errorf("want violation about human ratification, got: %v", violations)
	}
	if !containsReason(violations, "positive scenarios") {
		t.Errorf("want violation about missing positive scenarios, got: %v", violations)
	}
	if !containsReason(violations, "negative/exception scenarios") {
		t.Errorf("want violation about missing negative scenarios, got: %v", violations)
	}
	if !containsReason(violations, "benefit/alignment hypothesis") {
		t.Errorf("want violation about missing benefit hypothesis, got: %v", violations)
	}
}

func TestValidateSlice_ModelOnlyNoRatification(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Model-authored but no human ratification.
	v := state.ValidationRecord{
		PositiveScenarios: []string{"User clicks save, form persists."},
		NegativeScenarios: []string{"User clicks save while offline, system shows error."},
		BenefitHypothesis: "Saves user time by persisting forms.",
		// HumanRatified is false by default.
	}
	writeStatus(t, releaseDir, "S01-test", v)

	violations := validateSlice(releaseDir, "S01-test")
	if len(violations) == 0 {
		t.Fatal("want violations for model-only validation (no human ratification), got none")
	}
	if !containsReason(violations, "human_ratified") {
		t.Errorf("want violation about human ratification, got: %v", violations)
	}
}

func TestValidateSlice_PositiveWithoutNegativeFails(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Has positive scenarios but no negative scenarios.
	v := state.ValidationRecord{
		HumanRatified:     true,
		RatifiedBy:        "test-user",
		RatifiedAt:        "2026-06-16T12:00:00Z",
		PositiveScenarios: []string{"User clicks save, form persists."},
		BenefitHypothesis: "Saves user time by persisting forms.",
		// NegativeScenarios is empty.
	}
	writeStatus(t, releaseDir, "S01-test", v)

	violations := validateSlice(releaseDir, "S01-test")
	if len(violations) == 0 {
		t.Fatal("want violations for missing negative scenarios, got none")
	}
	if !containsReason(violations, "negative/exception scenarios") {
		t.Errorf("want violation about missing negative scenarios, got: %v", violations)
	}
}

func TestValidateSlice_NegativeWithoutPositiveFails(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Has negative scenarios but no positive scenarios.
	v := state.ValidationRecord{
		HumanRatified:     true,
		RatifiedBy:        "test-user",
		RatifiedAt:        "2026-06-16T12:00:00Z",
		NegativeScenarios: []string{"User clicks save while offline, system shows error."},
		BenefitHypothesis: "Saves user time by persisting forms.",
		// PositiveScenarios is empty.
	}
	writeStatus(t, releaseDir, "S01-test", v)

	violations := validateSlice(releaseDir, "S01-test")
	if len(violations) == 0 {
		t.Fatal("want violations for missing positive scenarios, got none")
	}
	if !containsReason(violations, "positive scenarios") {
		t.Errorf("want violation about missing positive scenarios, got: %v", violations)
	}
}

func TestValidateSlice_MissingBenefitHypothesisFails(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Has everything except benefit hypothesis.
	v := state.ValidationRecord{
		HumanRatified:     true,
		RatifiedBy:        "test-user",
		RatifiedAt:        "2026-06-16T12:00:00Z",
		PositiveScenarios: []string{"User clicks save, form persists."},
		NegativeScenarios: []string{"User clicks save while offline, system shows error."},
		// BenefitHypothesis is empty.
	}
	writeStatus(t, releaseDir, "S01-test", v)

	violations := validateSlice(releaseDir, "S01-test")
	if len(violations) == 0 {
		t.Fatal("want violations for missing benefit hypothesis, got none")
	}
	if !containsReason(violations, "benefit/alignment hypothesis") {
		t.Errorf("want violation about missing benefit hypothesis, got: %v", violations)
	}
}

func TestValidateSlice_CompleteRatifiedRecordPasses(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	v := state.ValidationRecord{
		HumanRatified:     true,
		RatifiedBy:        "test-human",
		RatifiedAt:        "2026-06-16T12:00:00Z",
		PositiveScenarios: []string{"User clicks save, form persists."},
		NegativeScenarios: []string{"User clicks save while offline, system shows error."},
		BenefitHypothesis: "Saves user time by persisting forms, reducing abandonment.",
	}
	writeStatus(t, releaseDir, "S01-test", v)

	violations := validateSlice(releaseDir, "S01-test")
	if len(violations) != 0 {
		t.Fatalf("want no violations for complete ratified record, got: %v", violations)
	}
}

func TestValidateSlice_MissingStatusJSONFails(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Create slice directory but no status.json.
	sliceDir := filepath.Join(releaseDir, "S01-test")
	os.MkdirAll(sliceDir, 0o755)

	violations := validateSlice(releaseDir, "S01-test")
	if len(violations) == 0 {
		t.Fatal("want violations for missing status.json, got none")
	}
}

// --- Run tests ---

func TestRun_AllSlicesValidated(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Two slices — both fully validated.
	v := state.ValidationRecord{
		HumanRatified:     true,
		RatifiedBy:        "test-human",
		RatifiedAt:        "2026-06-16T12:00:00Z",
		PositiveScenarios: []string{"Positive scenario"},
		NegativeScenarios: []string{"Negative scenario"},
		BenefitHypothesis: "Benefit hypothesis",
	}
	writeStatus(t, releaseDir, "S01-first", v)
	writeStatus(t, releaseDir, "S02-second", v)

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasViolations() {
		t.Fatalf("want no violations, got %d", len(report.Violations))
	}
	if report.TotalSlices != 2 {
		t.Errorf("want 2 total slices, got %d", report.TotalSlices)
	}
	if report.ValidatedSlices != 2 {
		t.Errorf("want 2 validated slices, got %d", report.ValidatedSlices)
	}
}

func TestRun_MixedValidationResults(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// S01 — fully validated.
	v1 := state.ValidationRecord{
		HumanRatified:     true,
		RatifiedBy:        "test-human",
		RatifiedAt:        "2026-06-16T12:00:00Z",
		PositiveScenarios: []string{"Positive scenario"},
		NegativeScenarios: []string{"Negative scenario"},
		BenefitHypothesis: "Benefit hypothesis",
	}
	writeStatus(t, releaseDir, "S01-pass", v1)

	// S02 — missing ratification.
	v2 := state.ValidationRecord{
		PositiveScenarios: []string{"Positive scenario"},
		NegativeScenarios: []string{"Negative scenario"},
		BenefitHypothesis: "Benefit hypothesis",
		// HumanRatified is false.
	}
	writeStatus(t, releaseDir, "S02-no-ratify", v2)

	// S03 — missing negative scenarios.
	v3 := state.ValidationRecord{
		HumanRatified:     true,
		RatifiedBy:        "test-human",
		RatifiedAt:        "2026-06-16T12:00:00Z",
		PositiveScenarios: []string{"Positive scenario"},
		BenefitHypothesis: "Benefit hypothesis",
		// NegativeScenarios is empty.
	}
	writeStatus(t, releaseDir, "S03-no-negative", v3)

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasViolations() {
		t.Fatal("want violations, got none")
	}
	if report.TotalSlices != 3 {
		t.Errorf("want 3 total slices, got %d", report.TotalSlices)
	}
	if report.ValidatedSlices != 1 {
		t.Errorf("want 1 validated slice, got %d", report.ValidatedSlices)
	}
	if report.FailedSlices != 2 {
		t.Errorf("want 2 failed slices, got %d", report.FailedSlices)
	}
}

func TestRun_NoSlicesPasses(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasViolations() {
		t.Fatal("want no violations for empty release")
	}
	if report.TotalSlices != 0 {
		t.Errorf("want 0 total slices, got %d", report.TotalSlices)
	}
}

func TestRun_SkipsNonSliceDirs(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Create a valid slice and some non-slice directories.
	v := state.ValidationRecord{
		HumanRatified:     true,
		RatifiedBy:        "test-human",
		RatifiedAt:        "2026-06-16T12:00:00Z",
		PositiveScenarios: []string{"Positive scenario"},
		NegativeScenarios: []string{"Negative scenario"},
		BenefitHypothesis: "Benefit hypothesis",
	}
	writeStatus(t, releaseDir, "S01-test", v)
	os.MkdirAll(filepath.Join(releaseDir, "assets"), 0o755)
	os.MkdirAll(filepath.Join(releaseDir, ".hidden"), 0o755)

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasViolations() {
		t.Fatalf("want no violations, got %d", len(report.Violations))
	}
	if report.TotalSlices != 1 {
		t.Errorf("want 1 total slice, got %d", report.TotalSlices)
	}
}

// --- Print / PrintCompact tests ---

func TestPrint_Formatting(t *testing.T) {
	report := Report{
		Violations: []Violation{
			{SliceID: "S01-test", Reason: "validation record missing human ratification"},
			{SliceID: "S02-test", Reason: "validation record has no negative scenarios"},
		},
		TotalSlices:     3,
		ValidatedSlices: 1,
		FailedSlices:    2,
	}

	output := Print(report)
	if !strings.Contains(output, "Violations:") {
		t.Errorf("print missing Violations section")
	}
	if !strings.Contains(output, "Per-slice results:") {
		t.Errorf("print missing Per-slice results section")
	}
	if !strings.Contains(output, "S01-test") {
		t.Errorf("print missing S01-test reference")
	}
	if !strings.Contains(output, "S02-test") {
		t.Errorf("print missing S02-test reference")
	}
}

func TestPrintCompact_Passed(t *testing.T) {
	output := PrintCompact(Report{TotalSlices: 3, ValidatedSlices: 3, FailedSlices: 0})
	if !strings.Contains(output, "PASSED") {
		t.Errorf("want PASSED, got: %s", output)
	}
}

func TestPrintCompact_Failed(t *testing.T) {
	output := PrintCompact(Report{TotalSlices: 3, ValidatedSlices: 1, FailedSlices: 2,
		Violations: []Violation{{SliceID: "S01-test", Reason: "missing ratification"}}})
	if !strings.Contains(output, "FAILED") {
		t.Errorf("want FAILED, got: %s", output)
	}
}

func TestPrintCompact_NoSlices(t *testing.T) {
	output := PrintCompact(Report{TotalSlices: 0})
	if !strings.Contains(output, "no slices to validate") {
		t.Errorf("want 'no slices to validate', got: %s", output)
	}
}

// --- helpers ---

func containsReason(violations []Violation, substr string) bool {
	for _, v := range violations {
		if strings.Contains(v.Reason, substr) {
			return true
		}
	}
	return false
}
