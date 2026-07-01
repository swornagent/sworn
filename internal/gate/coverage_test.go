package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- unit: RunCoverage with fixture ---

func TestRunCoverage_FullCoverage_Go(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] THE SYSTEM SHALL validate user input fields
- [ ] WHEN a request is received, THE SYSTEM SHALL respond with status 200
- [ ] THE SYSTEM SHALL compute and return the portfolio total
`,
	})

	// Test the extraction + matching logic directly with a mock
	// rather than requiring git diff context.
	// Use the parseAcceptanceChecks + bestMatch functions directly.
	sliceDir := filepath.Join(dir, "S01-test-slice")
	spec, err := os.ReadFile(filepath.Join(sliceDir, "spec.md"))
	if err != nil {
		t.Fatal(err)
	}
	acs := parseAcceptanceChecks(string(spec))
	if len(acs) != 3 {
		t.Fatalf("expected 3 ACs, got %d", len(acs))
	}

	// Simulate test functions.
	tests := []testFunc{
		{Name: "TestValidateInputFields", File: "validate_test.go", Line: 12},
		{Name: "TestRequestResponds200", File: "handler_test.go", Line: 34},
		{Name: "TestPortfolioTotal", File: "portfolio_test.go", Line: 56},
	}

	for i, ac := range acs {
		best, _ := bestMatch(ac, tests)
		if best == nil {
			t.Errorf("AC-%02d %q: expected match, got none", i+1, truncate(ac, 60))
		}
	}
}

func TestRunCoverage_UncoveredACs(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] THE SYSTEM SHALL validate user input fields
- [ ] WHEN a request is received, THE SYSTEM SHALL respond with status 200
- [ ] THE SYSTEM SHALL compute and return the portfolio total
`,
	})

	sliceDir := filepath.Join(dir, "S01-test-slice")
	spec, err := os.ReadFile(filepath.Join(sliceDir, "spec.md"))
	if err != nil {
		t.Fatal(err)
	}
	acs := parseAcceptanceChecks(string(spec))

	// Only one matching test — two ACs uncovered.
	tests := []testFunc{
		{Name: "TestValidateInputFields", File: "validate_test.go", Line: 12},
	}

	covered := 0
	for _, ac := range acs {
		best, _ := bestMatch(ac, tests)
		if best != nil {
			covered++
		}
	}
	if covered != 1 {
		t.Errorf("expected 1 covered AC, got %d", covered)
	}
}

func TestRunCoverage_TypeScriptPatterns(t *testing.T) {
	// Test that TS patterns are recognised.
	tf := filepath.Join(t.TempDir(), "component.test.ts")
	content := `
it('renders the button with correct label', () => {});
test('handles click events on submit', () => {});
describe('PortfolioPage', () => {
  it('displays total value', () => {});
});
`
	os.WriteFile(tf, []byte(content), 0644)

	tests, err := extractTestFuncs(tf)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 3 {
		t.Errorf("expected 3 TS test funcs, got %d: %v", len(tests), tests)
	}

	names := map[string]bool{}
	for _, tf := range tests {
		names[tf.Name] = true
	}
	if !names["renders the button with correct label"] {
		t.Error("missing first it() name")
	}
	if !names["handles click events on submit"] {
		t.Error("missing test() name")
	}
	if !names["displays total value"] {
		t.Error("missing nested it() name")
	}
}

func TestRunCoverage_GoPatterns(t *testing.T) {
	tf := filepath.Join(t.TempDir(), "handler_test.go")
	content := `
package handler_test

import "testing"

func TestHandlerResponds200(t *testing.T) {}

func BenchmarkHandlerLatency(b *testing.B) {}

func (s *Suite) TestWithReceiver(t *testing.T) {}
`
	os.WriteFile(tf, []byte(content), 0644)

	tests, err := extractTestFuncs(tf)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tf := range tests {
		names[tf.Name] = true
	}
	if !names["TestHandlerResponds200"] {
		t.Error("missing TestHandlerResponds200")
	}
	if !names["BenchmarkHandlerLatency"] {
		t.Error("missing BenchmarkHandlerLatency")
	}
	if !names["TestWithReceiver"] {
		t.Error("missing TestWithReceiver")
	}
}

func TestRunCoverage_PythonPatterns(t *testing.T) {
	tf := filepath.Join(t.TempDir(), "test_portfolio.py")
	content := `
import pytest

def test_calculate_returns():
    pass

def test_empty_portfolio():
    pass
`
	os.WriteFile(tf, []byte(content), 0644)

	tests, err := extractTestFuncs(tf)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tf := range tests {
		names[tf.Name] = true
	}
	if !names["test_calculate_returns"] {
		t.Error("missing test_calculate_returns")
	}
	if !names["test_empty_portfolio"] {
		t.Error("missing test_empty_portfolio")
	}
}

// --- keyword matching ---

func TestMatchScore(t *testing.T) {
	// AC: "THE SYSTEM SHALL validate user input fields"
	ac := "THE SYSTEM SHALL validate user input fields"

	// Test name should score high.
	s1 := matchScore(ac, "TestValidateInputFields")
	if s1 < 1 {
		t.Errorf("expected score >= 1 for TestValidateInputFields, got %d", s1)
	}

	// Unrelated test should score 0.
	s2 := matchScore(ac, "TestSomethingCompletelyDifferent")
	if s2 > 0 {
		t.Errorf("expected score 0 for unrelated test, got %d", s2)
	}

	// TS-style name with words from the AC.
	s3 := matchScore(ac, "validates the user input fields correctly")
	if s3 < 1 {
		t.Errorf("expected score >= 1 for TS-style name, got %d", s3)
	}
}

func TestTokenise(t *testing.T) {
	toks := tokenise("THE SYSTEM SHALL validate user input fields")
	if !toks["user"] || !toks["input"] || !toks["fields"] {
		t.Errorf("missing expected tokens: %v", toks)
	}
	// Stop words excluded.
	if toks["the"] || toks["shall"] || toks["system"] {
		t.Errorf("stop words not excluded: %v", toks)
	}
}

// --- test file detection ---

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"internal/gate/coverage_test.go", true},
		{"internal/gate/coverage.go", false},
		{"src/component.test.ts", true},
		{"src/component.test.tsx", true},
		{"src/component.spec.ts", true},
		{"src/component.spec.tsx", true},
		{"src/component.ts", false},
		{"tests/test_portfolio.py", true},
		{"src/portfolio.py", false},
	}
	for _, tt := range tests {
		got := isTestFile(tt.path)
		if got != tt.want {
			t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// --- bestMatch returns candidates ---

func TestBestMatch_Candidates(t *testing.T) {
	ac := "THE SYSTEM SHALL validate user input fields"
	tests := []testFunc{
		{Name: "TestValidateInputFields", File: "validate_test.go", Line: 12},
		{Name: "TestUserInput", File: "input_test.go", Line: 8},
		{Name: "TestSomethingElse", File: "other_test.go", Line: 1},
	}

	best, cands := bestMatch(ac, tests)
	if best == nil {
		t.Fatal("expected a match")
	}
	if best.Name != "TestValidateInputFields" {
		t.Errorf("expected TestValidateInputFields as best, got %s", best.Name)
	}
	if len(cands) < 2 {
		t.Errorf("expected at least 2 candidates, got %d", len(cands))
	}
}

func TestBestMatch_NoMatch(t *testing.T) {
	ac := "THE SYSTEM SHALL compute orbital mechanics"
	tests := []testFunc{
		{Name: "TestValidateInputFields", File: "validate_test.go", Line: 12},
	}
	best, cands := bestMatch(ac, tests)
	if best != nil {
		t.Errorf("expected no match, got %s", best.Name)
	}
	if len(cands) != 0 {
		t.Errorf("expected no candidates, got %v", cands)
	}
}

// --- CoverageReport helpers ---

func TestCoverageReport_HasViolations(t *testing.T) {
	r := &CoverageReport{Uncovered: 0}
	if r.HasViolations() {
		t.Error("expected no violations")
	}
	r.Uncovered = 1
	if !r.HasViolations() {
		t.Error("expected violations")
	}
}

// --- PrintCoverage ---

func TestPrintCoverage_Pass(t *testing.T) {
	r := &CoverageReport{
		Slice:    "S66-lint-coverage",
		Release:  "test-release",
		TotalACs: 2,
		Covered:  2,
		Verdict:  "PASS",
		Entries: []CoverageEntry{
			{ACID: "AC-01", ACText: "shall validate input", MatchedTest: "TestValidateInput", MatchFile: "validate_test.go", MatchLine: 12},
			{ACID: "AC-02", ACText: "shall respond 200", MatchedTest: "TestRespond200", MatchFile: "handler_test.go", MatchLine: 34},
		},
	}
	out := PrintCoverage(r)
	if !strings.Contains(out, "PASS") {
		t.Error("PrintCoverage missing PASS")
	}
	if !strings.Contains(out, "COVERAGE") {
		t.Error("PrintCoverage missing banner")
	}
}

func TestPrintCoverage_Fail(t *testing.T) {
	r := &CoverageReport{
		Slice:     "S66-lint-coverage",
		Release:   "test-release",
		TotalACs:  2,
		Covered:   1,
		Uncovered: 1,
		Verdict:   "FAIL",
		Entries: []CoverageEntry{
			{ACID: "AC-01", ACText: "shall validate input", MatchedTest: "TestValidateInput", MatchFile: "validate_test.go", MatchLine: 12},
			{ACID: "AC-02", ACText: "shall compute orbital mechanics", Candidates: []string{"TestValidateInput (validate_test.go:12 score=1)"}},
		},
	}
	out := PrintCoverage(r)
	if !strings.Contains(out, "FAIL") {
		t.Error("PrintCoverage missing FAIL")
	}
}

func TestJSONCoverage(t *testing.T) {
	r := &CoverageReport{
		Slice:    "S66",
		Verdict:  "PASS",
		TotalACs: 0,
	}
	out := JSONCoverage(r)
	if !strings.Contains(out, `"verdict"`) {
		t.Error("JSONCoverage missing verdict")
	}
}

// --- BaseRefForSlice ---

func TestBaseRefForSlice(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "start_commit": "abc123def"
}`,
	})
	sliceDir := filepath.Join(dir, "S01-test-slice")
	ref, err := BaseRefForSlice(sliceDir, "test-release")
	if err != nil {
		t.Fatal(err)
	}
	if ref != "abc123def" {
		t.Errorf("expected abc123def, got %s", ref)
	}
}

func TestBaseRefForSlice_Fallback(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice"
}`,
	})
	sliceDir := filepath.Join(dir, "S01-test-slice")
	ref, err := BaseRefForSlice(sliceDir, "test-release")
	if err != nil {
		t.Fatal(err)
	}
	if ref != "release-wt/test-release" {
		t.Errorf("expected release-wt/test-release, got %s", ref)
	}
}
