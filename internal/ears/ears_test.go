package ears

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassify_Ubiquitous(t *testing.T) {
	tests := []string{
		"THE SYSTEM SHALL display the dashboard.",
		"the system shall display the dashboard.",
		"THE  SYSTEM  SHALL  display the dashboard.",
	}
	for _, tc := range tests {
		if got := Classify(tc); got != PatternUbiquitous {
			t.Errorf("Classify(%q) = %v, want %v", tc, got, PatternUbiquitous)
		}
	}
}

func TestClassify_EventDriven(t *testing.T) {
	tests := []string{
		"WHEN a user clicks save THE SYSTEM SHALL persist the form.",
		"when a user clicks save the system shall persist the form.",
		"WHEN  a user clicks save  THE SYSTEM SHALL persist the form.",
	}
	for _, tc := range tests {
		if got := Classify(tc); got != PatternEventDriven {
			t.Errorf("Classify(%q) = %v, want %v", tc, got, PatternEventDriven)
		}
	}
}

func TestClassify_StateDriven(t *testing.T) {
	tests := []string{
		"WHILE the system is in maintenance mode THE SYSTEM SHALL show a banner.",
		"while the system is in maintenance mode the system shall show a banner.",
	}
	for _, tc := range tests {
		if got := Classify(tc); got != PatternStateDriven {
			t.Errorf("Classify(%q) = %v, want %v", tc, got, PatternStateDriven)
		}
	}
}

func TestClassify_OptionalFeature(t *testing.T) {
	tests := []string{
		"WHERE a premium feature is enabled THE SYSTEM SHALL show the export button.",
		"where a premium feature is enabled the system shall show the export button.",
	}
	for _, tc := range tests {
		if got := Classify(tc); got != PatternOptionalFeature {
			t.Errorf("Classify(%q) = %v, want %v", tc, got, PatternOptionalFeature)
		}
	}
}

func TestClassify_UnwantedBehaviour(t *testing.T) {
	tests := []string{
		"IF the database is unreachable THEN THE SYSTEM SHALL return a 503 error.",
		"if the database is unreachable then the system shall return a 503 error.",
	}
	for _, tc := range tests {
		if got := Classify(tc); got != PatternUnwanted {
			t.Errorf("Classify(%q) = %v, want %v", tc, got, PatternUnwanted)
		}
	}
}

func TestClassify_Complex(t *testing.T) {
	tests := []string{
		"WHEN a user clicks save WHILE the form is valid THE SYSTEM SHALL persist the form.",
		"WHEN a user clicks save IF the form is valid THEN THE SYSTEM SHALL persist the form.",
		"WHERE premium is enabled WHILE the user is authenticated THE SYSTEM SHALL show the export button.",
	}
	for _, tc := range tests {
		if got := Classify(tc); got != PatternComplex {
			t.Errorf("Classify(%q) = %v, want %v", tc, got, PatternComplex)
		}
	}
}

func TestClassify_Note(t *testing.T) {
	tests := []string{
		"NOTE: this is a deliberate non-requirement note.",
		"note: this is a deliberate non-requirement note.",
		"NOTE : this is a deliberate non-requirement note.",
	}
	for _, tc := range tests {
		if got := Classify(tc); got != PatternNote {
			t.Errorf("Classify(%q) = %v, want %v", tc, got, PatternNote)
		}
	}
}

func TestClassify_FreeForm(t *testing.T) {
	tests := []string{
		"The system should display the dashboard.",
		"Display the dashboard when the user logs in.",
		"Make sure the form is saved.",
		"THE SYSTEM MAY display a banner.",
		"WHEN something happens, the system should do a thing.",
		"IF the database is unreachable THE SYSTEM SHALL return a 503 error.", // missing THEN
		"THEN THE SYSTEM SHALL return a 503 error.",                           // THEN without IF
	}
	for _, tc := range tests {
		if got := Classify(tc); got != PatternNone {
			t.Errorf("Classify(%q) = %v, want %v", tc, got, PatternNone)
		}
	}
}
func TestClassify_UnwantedRequiresThen(t *testing.T) {
	// IF without THEN is not a valid unwanted-behaviour pattern.
	// It should not classify as PatternUnwanted. Since it has a SHALL clause
	// but no valid precondition (IF alone without THEN doesn't count), it
	// should be ubiquitous (no valid preconditions counted).
	tc := "IF the database is unreachable THE SYSTEM SHALL return a 503 error."
	got := Classify(tc)
	if got == PatternUnwanted {
		t.Errorf("Classify(%q) = %v, should not be PatternUnwanted without THEN", tc, got)
	}
}

// writeFixture creates a minimal release directory tree for testing.
func writeFixture(t *testing.T, mods ...func(dir string)) string {
	t.Helper()
	dir := t.TempDir()

	// S01-test-slice with well-formed EARS ACs.
	sliceDir := filepath.Join(dir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)
	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist the form.
- [ ] WHILE the system is in maintenance mode THE SYSTEM SHALL show a banner.
- [ ] WHERE a premium feature is enabled THE SYSTEM SHALL show the export button.
- [ ] IF the database is unreachable THEN THE SYSTEM SHALL return a 503 error.
- [ ] WHEN a user clicks save WHILE the form is valid THE SYSTEM SHALL persist the form.
- [ ] NOTE: this is a deliberate non-requirement note.

## Required tests

- **Unit**: internal/ears/ears_test.go
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)

	for _, mod := range mods {
		mod(dir)
	}

	return dir
}

func TestValidate_AllPatterns(t *testing.T) {
	dir := writeFixture(t)
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if report.HasViolations() {
		t.Fatalf("expected 0 violations, got %d:\n%s",
			len(report.Violations), violationsString(report.Violations))
	}
	if report.TotalACs != 6 {
		t.Errorf("expected 6 ACs (excluding NOTE), got %d", report.TotalACs)
	}
	if report.TotalNotes != 1 {
		t.Errorf("expected 1 NOTE, got %d", report.TotalNotes)
	}
	// Check distribution.
	if report.Dist[PatternUbiquitous] != 1 {
		t.Errorf("expected 1 ubiquitous, got %d", report.Dist[PatternUbiquitous])
	}
	if report.Dist[PatternEventDriven] != 1 {
		t.Errorf("expected 1 event-driven, got %d", report.Dist[PatternEventDriven])
	}
	if report.Dist[PatternStateDriven] != 1 {
		t.Errorf("expected 1 state-driven, got %d", report.Dist[PatternStateDriven])
	}
	if report.Dist[PatternOptionalFeature] != 1 {
		t.Errorf("expected 1 optional-feature, got %d", report.Dist[PatternOptionalFeature])
	}
	if report.Dist[PatternUnwanted] != 1 {
		t.Errorf("expected 1 unwanted, got %d", report.Dist[PatternUnwanted])
	}
	if report.Dist[PatternComplex] != 1 {
		t.Errorf("expected 1 complex, got %d", report.Dist[PatternComplex])
	}
}

func TestValidate_FreeFormViolation(t *testing.T) {
	dir := writeFixture(t, func(dir string) {
		spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] The system should display the dashboard when the user logs in.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist the form.

## Required tests

- **Unit**: some test
`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "spec.md"), []byte(spec), 0644)
	})
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !report.HasViolations() {
		t.Fatal("expected violations, got none")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(report.Violations))
	}
	v := report.Violations[0]
	if v.SliceID != "S01-test-slice" {
		t.Errorf("violation slice id: %q, want S01-test-slice", v.SliceID)
	}
	if !strings.Contains(v.Text, "The system should display") {
		t.Errorf("violation text: %q", v.Text)
	}
}

func TestValidate_NoteExcluded(t *testing.T) {
	dir := writeFixture(t, func(dir string) {
		spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] NOTE: this is a deliberate non-requirement note.
- [ ] NOTE: another note that would be free-form if not for the escape.

## Required tests

- **Unit**: some test
`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "spec.md"), []byte(spec), 0644)
	})
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if report.HasViolations() {
		t.Fatalf("expected 0 violations (NOTEs excluded), got %d", len(report.Violations))
	}
	if report.TotalACs != 1 {
		t.Errorf("expected 1 AC (excluding 2 NOTEs), got %d", report.TotalACs)
	}
	if report.TotalNotes != 2 {
		t.Errorf("expected 2 NOTEs, got %d", report.TotalNotes)
	}
}

func TestValidate_MultipleSlices(t *testing.T) {
	dir := writeFixture(t, func(dir string) {
		// Add a second slice.
		sliceDir2 := filepath.Join(dir, "S02-test-slice")
		os.MkdirAll(sliceDir2, 0755)
		spec2 := `---
title: S02-test-slice
---

# Slice: S02-test-slice

## User outcome

Test outcome 2.

## Acceptance checks

- [ ] WHEN a user logs in THE SYSTEM SHALL show the dashboard.
- [ ] Make sure the form is saved.

## Required tests

- **Unit**: some test
`
		os.WriteFile(filepath.Join(sliceDir2, "spec.md"), []byte(spec2), 0644)
	})
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(report.Violations) != 1 {
		t.Fatalf("expected 1 violation (from S02), got %d", len(report.Violations))
	}
	if report.Violations[0].SliceID != "S02-test-slice" {
		t.Errorf("violation slice id: %q, want S02-test-slice", report.Violations[0].SliceID)
	}
}

func TestValidate_MultiLineAC(t *testing.T) {
	// An AC that spans multiple lines (continuation indentation) should be
	// joined and classified as a single AC.
	dir := writeFixture(t, func(dir string) {
		spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a slice's spec.md contains an acceptance check matching no EARS pattern, THE
      SYSTEM SHALL exit non-zero from sworn ears <release> and name the slice + the line.
- [ ] THE SYSTEM SHALL display the dashboard.

## Required tests

- **Unit**: some test
`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "spec.md"), []byte(spec), 0644)
	})
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if report.HasViolations() {
		t.Fatalf("expected 0 violations (multi-line AC should join), got %d:\n%s",
			len(report.Violations), violationsString(report.Violations))
	}
	if report.TotalACs != 2 {
		t.Errorf("expected 2 ACs, got %d", report.TotalACs)
	}
	// The first AC should be event-driven (WHEN ... THE SYSTEM SHALL ...).
	if report.Dist[PatternEventDriven] != 1 {
		t.Errorf("expected 1 event-driven, got %d", report.Dist[PatternEventDriven])
	}
	if report.Dist[PatternUbiquitous] != 1 {
		t.Errorf("expected 1 ubiquitous, got %d", report.Dist[PatternUbiquitous])
	}
}
func TestValidate_SkipsNonSliceDirs(t *testing.T) {
	dir := writeFixture(t, func(dir string) {
		// Add a screenshots directory (not a slice).
		os.MkdirAll(filepath.Join(dir, "screenshots"), 0755)
		// Add a non-spec file in a non-slice dir.
		os.WriteFile(filepath.Join(dir, "screenshots", "readme.md"), []byte("not a spec"), 0644)
	})
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Should not error or produce violations from the screenshots dir.
	if report.HasViolations() {
		t.Fatalf("expected 0 violations, got %d", len(report.Violations))
	}
}

func TestValidate_EmptyRelease(t *testing.T) {
	dir := t.TempDir()
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if report.TotalACs != 0 {
		t.Errorf("expected 0 ACs for empty release, got %d", report.TotalACs)
	}
	if report.HasViolations() {
		t.Errorf("expected 0 violations for empty release, got %d", len(report.Violations))
	}
}

func TestPrint_NonEmpty(t *testing.T) {
	dir := writeFixture(t)
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	out := Print(report)
	if !strings.Contains(out, "EARS Acceptance-Criteria Validation") {
		t.Error("Print output missing title")
	}
	if !strings.Contains(out, "Pattern distribution") {
		t.Error("Print output missing pattern distribution")
	}
	if !strings.Contains(out, "Per-slice breakdown") {
		t.Error("Print output missing per-slice breakdown")
	}
	if !strings.Contains(out, "Violations: none") {
		t.Error("Print output should show 'Violations: none' for clean release")
	}
}

func TestPrint_WithViolations(t *testing.T) {
	dir := writeFixture(t, func(dir string) {
		spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] Make sure the form is saved.

## Required tests

- **Unit**: some test
`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "spec.md"), []byte(spec), 0644)
	})
	report, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	out := Print(report)
	if !strings.Contains(out, "Violations (1 free-form ACs)") {
		t.Errorf("Print output should show 1 violation, got:\n%s", out)
	}
	if !strings.Contains(out, "S01-test-slice") {
		t.Error("Print output should name the violating slice")
	}
}

func TestIsSliceID(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"S01-rtm-spine", true},
		{"S02-ears-ac-format", true},
		{"S10-no-mock-boundary", true},
		{"screenshots", false},
		{".git", false},
		{"intake.md", false},
		{"T1-fidelity-core", false},
	}
	for _, tc := range tests {
		if got := isSliceID(tc.s); got != tc.want {
			t.Errorf("isSliceID(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate: got %q", got)
	}
	if got := truncate("hello world foo bar", 10); got != "hello w..." {
		t.Errorf("truncate: got %q", got)
	}
}

// violationsString renders violations for test error messages.
func violationsString(vs []Violation) string {
	var b []byte
	for _, v := range vs {
		b = append(b, v.String()...)
		b = append(b, '\n')
	}
	return string(b)
}
