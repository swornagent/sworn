package gate

import (
	"context"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/project"
)

// --- mock model verifier ---

// mockVerifier implements model.Verifier with canned responses.
type mockVerifier struct {
	text    string
	costUSD float64
	err     error
}

func (m *mockVerifier) Verify(_ context.Context, _, _ string) (string, float64, int64, int64, error) {
	return m.text, m.costUSD, 0, 0, m.err
}

// --- prompt building tests ---

func TestBuildUserPayload(t *testing.T) {
	spec := "# Slice: S01-test\n\n## Acceptance checks\n\n- [ ] AC 1\n- [ ] AC 2\n"
	diff := "diff --git a/foo.go b/foo.go\n+added line\n"

	payload := buildUserPayload(project.Resolved{Context: "a Go project"}, spec, diff)

	// Must contain the spec content.
	if !strings.Contains(payload, spec) {
		t.Error("user payload missing spec content")
	}
	// Must contain the diff content.
	if !strings.Contains(payload, diff) {
		t.Error("user payload missing diff content")
	}
	// Must contain the separator.
	if !strings.Contains(payload, "--- GIT DIFF ---") {
		t.Error("user payload missing diff separator")
	}
}

func TestBuildUserPayload_EmptyDiff(t *testing.T) {
	spec := "# Slice\n\n## Acceptance checks\n\n- [ ] AC 1\n"
	payload := buildUserPayload(project.Resolved{Context: "a Go project"}, spec, "")
	if !strings.Contains(payload, "(no diff available") {
		t.Error("user payload missing empty-diff message")
	}
}

// --- response parsing tests ---

func TestParseLLMResponse_Pass(t *testing.T) {
	raw := `{"verdict": "PASS", "findings": []}`
	result, err := parseLLMResponse(raw)
	if err != nil {
		t.Fatalf("parseLLMResponse: %v", err)
	}
	if result.Verdict != "PASS" {
		t.Errorf("expected PASS, got %s", result.Verdict)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestParseLLMResponse_Fail(t *testing.T) {
	raw := `{"verdict": "FAIL", "findings": [{"id": "F-01", "severity": "high", "blocking": true, "title": "Missing error handling", "detail": "The code does not handle error case X"}]}`
	result, err := parseLLMResponse(raw)
	if err != nil {
		t.Fatalf("parseLLMResponse: %v", err)
	}
	if result.Verdict != "FAIL" {
		t.Errorf("expected FAIL, got %s", result.Verdict)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].ID != "F-01" {
		t.Errorf("expected F-01, got %s", result.Findings[0].ID)
	}
	if result.Findings[0].Severity != "high" {
		t.Errorf("expected high severity, got %s", result.Findings[0].Severity)
	}
}

func TestParseLLMResponse_MarkdownFence(t *testing.T) {
	raw := "```json\n{\"verdict\": \"PASS\", \"findings\": []}\n```"
	result, err := parseLLMResponse(raw)
	if err != nil {
		t.Fatalf("parseLLMResponse: %v", err)
	}
	if result.Verdict != "PASS" {
		t.Errorf("expected PASS, got %s", result.Verdict)
	}
}

func TestParseLLMResponse_ProseWrapping(t *testing.T) {
	raw := "Here is my analysis:\n\n{\"verdict\": \"FAIL\", \"findings\": [{\"id\": \"F-01\", \"severity\": \"medium\", \"blocking\": false, \"title\": \"Weak hash\", \"detail\": \"Uses MD5\"}]}\n\nThat's all."
	result, err := parseLLMResponse(raw)
	if err != nil {
		t.Fatalf("parseLLMResponse: %v", err)
	}
	if result.Verdict != "FAIL" {
		t.Errorf("expected FAIL, got %s", result.Verdict)
	}
}

func TestParseLLMResponse_UnknownVerdict(t *testing.T) {
	raw := `{"verdict": "UNCLEAR", "findings": []}`
	result, err := parseLLMResponse(raw)
	if err != nil {
		t.Fatalf("parseLLMResponse: %v", err)
	}
	// Unknown verdict → fail closed.
	if result.Verdict != "FAIL" {
		t.Errorf("expected FAIL (fail-closed), got %s", result.Verdict)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 info finding for unknown verdict, got %d", len(result.Findings))
	}
}

func TestParseLLMResponse_InvalidJSON(t *testing.T) {
	raw := `not json at all`
	_, err := parseLLMResponse(raw)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- RunLLMCheck integration tests ---

func TestRunLLMCheck_Pass(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test/spec.md": `---
title: S01-test
---

# Slice: S01-test

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN called, THE SYSTEM SHALL respond with data.
`,
		"S01-test/status.json": `{"slice_id": "S01-test", "state": "verified"}`,
	})

	sliceDir := dir + "/S01-test"
	mock := &mockVerifier{
		text: `{"verdict": "PASS", "findings": []}`,
	}

	report, err := RunLLMCheck(context.Background(), CheckACSatisfaction, sliceDir, "mock diff", mock)
	if err != nil {
		t.Fatalf("RunLLMCheck: %v", err)
	}
	if report.Verdict != "PASS" {
		t.Errorf("expected PASS, got %s", report.Verdict)
	}
	if report.HasViolations() {
		t.Error("expected no violations")
	}
	if report.CheckType != CheckACSatisfaction {
		t.Errorf("expected check type %s, got %s", CheckACSatisfaction, report.CheckType)
	}
}

func TestRunLLMCheck_Fail(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test/spec.md": `---
title: S01-test
---

# Slice: S01-test

## Acceptance checks

- [ ] WHEN called, THE SYSTEM SHALL respond.
`,
		"S01-test/status.json": `{"slice_id": "S01-test", "state": "verified"}`,
	})

	sliceDir := dir + "/S01-test"
	mock := &mockVerifier{
		text: `{"verdict": "FAIL", "findings": [{"id": "F-01", "severity": "high", "blocking": true, "title": "AC not satisfied", "detail": "The code does not handle the response case"}]}`,
	}

	report, err := RunLLMCheck(context.Background(), CheckACSatisfaction, sliceDir, "mock diff", mock)
	if err != nil {
		t.Fatalf("RunLLMCheck: %v", err)
	}
	if report.Verdict != "FAIL" {
		t.Errorf("expected FAIL, got %s", report.Verdict)
	}
	if !report.HasViolations() {
		t.Error("expected violations")
	}
}

func TestRunLLMCheck_AllCheckTypes(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test/spec.md": `---
title: S01-test
---

# Slice: S01-test

## Acceptance checks

- [ ] WHEN called, THE SYSTEM SHALL respond.
`,
		"S01-test/status.json": `{"slice_id": "S01-test", "state": "verified"}`,
	})

	sliceDir := dir + "/S01-test"

	checkTypes := []CheckType{
		CheckACSatisfaction,
		CheckSpecAmbiguity,
		CheckDesignReview,
		CheckSecurityReview,
		CheckSemanticCoverage,
		CheckMaintainabilityReview,
	}

	for _, ct := range checkTypes {
		t.Run(string(ct), func(t *testing.T) {
			mock := &mockVerifier{
				text: `{"verdict": "PASS", "findings": []}`,
			}
			report, err := RunLLMCheck(context.Background(), ct, sliceDir, "mock diff", mock)
			if err != nil {
				t.Fatalf("RunLLMCheck(%s): %v", ct, err)
			}
			if report.CheckType != ct {
				t.Errorf("expected check type %s, got %s", ct, report.CheckType)
			}
		})
	}
}

func TestRunLLMCheck_InvalidType(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test/spec.md":     "# spec",
		"S01-test/status.json": `{}`,
	})

	_, err := RunLLMCheck(context.Background(), "bogus-type", dir+"/S01-test", "", &mockVerifier{})
	if err == nil {
		t.Error("expected error for invalid check type")
	}
}

func TestRunLLMCheck_MissingSpec(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test/status.json": `{}`,
	})

	_, err := RunLLMCheck(context.Background(), CheckACSatisfaction, dir+"/S01-test", "", &mockVerifier{})
	if err == nil {
		t.Error("expected error for missing spec.md")
	}
}

func TestRunLLMCheck_UnparseableResponse(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test/spec.md": `---
title: S01-test
---

# Slice: S01-test

## Acceptance checks

- [ ] AC 1
`,
		"S01-test/status.json": `{}`,
	})

	sliceDir := dir + "/S01-test"
	mock := &mockVerifier{
		text: "I'm sorry, I cannot provide a JSON response right now.",
	}

	report, err := RunLLMCheck(context.Background(), CheckACSatisfaction, sliceDir, "mock diff", mock)
	if err != nil {
		t.Fatalf("RunLLMCheck: %v", err)
	}
	// Tolerant parse: should fail closed.
	if report.Verdict != "FAIL" {
		t.Errorf("expected FAIL for unparseable response, got %s", report.Verdict)
	}
}

func TestRunLLMCheck_SecuritySeverities(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test/spec.md": `---
title: S01-test
---

## Acceptance checks

- [ ] AC 1
`,
		"S01-test/status.json": `{}`,
	})

	sliceDir := dir + "/S01-test"
	mock := &mockVerifier{
		text: `{"verdict": "FAIL", "findings": [
			{"id": "F-01", "severity": "critical", "blocking": true, "title": "RCE via input", "detail": "..."},
			{"id": "F-02", "severity": "high", "blocking": true, "title": "SQL injection", "detail": "..."},
			{"id": "F-03", "severity": "medium", "blocking": false, "title": "Info leak", "detail": "..."},
			{"id": "F-04", "severity": "low", "blocking": false, "title": "Weak cipher", "detail": "..."}
		]}`,
	}

	report, err := RunLLMCheck(context.Background(), CheckSecurityReview, sliceDir, "mock diff", mock)
	if err != nil {
		t.Fatalf("RunLLMCheck: %v", err)
	}
	if len(report.Findings) != 4 {
		t.Fatalf("expected 4 findings, got %d", len(report.Findings))
	}
	if !report.HasViolations() {
		t.Error("expected violations for critical/high/medium findings")
	}
}

// --- print/render tests ---

func TestPrintLLMCheck_Pass(t *testing.T) {
	r := &LLMCheckReport{
		CheckType: CheckACSatisfaction,
		Slice:     "S01-test",
		Release:   "test-release",
		Verdict:   "PASS",
		Findings:  nil,
	}
	out := PrintLLMCheck(r)
	if !strings.Contains(out, "PASS") {
		t.Errorf("expected PASS in output: %s", out)
	}
	if !strings.Contains(out, "LLM CHECK") {
		t.Error("missing LLM CHECK banner")
	}
}

func TestPrintLLMCheck_Fail(t *testing.T) {
	r := &LLMCheckReport{
		CheckType: CheckSpecAmbiguity,
		Slice:     "S01-test",
		Release:   "test-release",
		Verdict:   "FAIL",
		Findings: []LLMFinding{
			{ID: "F-01", Severity: "FAIL", Title: "Vague AC", Detail: "AC lacks concrete terms"},
		},
	}
	out := PrintLLMCheck(r)
	if !strings.Contains(out, "FAIL") {
		t.Errorf("expected FAIL in output: %s", out)
	}
	if !strings.Contains(out, "NOT PASSED") {
		t.Error("missing NOT PASSED")
	}
	if !strings.Contains(out, "Vague AC") {
		t.Error("missing finding title")
	}
}

func TestJSONLLMCheck(t *testing.T) {
	r := &LLMCheckReport{
		CheckType: CheckACSatisfaction,
		Slice:     "S01-test",
		Release:   "test-release",
		Verdict:   "PASS",
	}
	out := JSONLLMCheck(r)
	if !strings.Contains(out, "\"verdict\"") {
		t.Error("JSON output missing verdict")
	}
	if !strings.Contains(out, "\"check_type\"") {
		t.Error("JSON output missing check_type")
	}
}

// --- helpers ---

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"plain json", `{"a": 1}`, `{"a": 1}`},
		{"fenced", "```json\n{\"a\": 1}\n```", `{"a": 1}`},
		{"fenced no lang", "```\n{\"a\": 1}\n```", `{"a": 1}`},
		{"prose wrap", "Here is result:\n{\"a\": 1}\nDone.", `{"a": 1}`},
		{"no braces", "just text", "just text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.raw)
			if got != tt.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestValidCheckTypes(t *testing.T) {
	expected := []CheckType{
		CheckACSatisfaction,
		CheckSpecAmbiguity,
		CheckDesignReview,
		CheckSecurityReview,
		CheckSemanticCoverage,
		CheckMaintainabilityReview,
	}
	if len(ValidCheckTypes) != len(expected) {
		t.Errorf("expected %d valid check types, got %d", len(expected), len(ValidCheckTypes))
	}
	for _, ct := range expected {
		if !ValidCheckTypes[ct] {
			t.Errorf("missing check type %s in ValidCheckTypes", ct)
		}
	}
}

func TestLLMCheckReport_HasViolations(t *testing.T) {
	tests := []struct {
		name   string
		report LLMCheckReport
		want   bool
	}{
		{"pass empty", LLMCheckReport{Verdict: "PASS"}, false},
		{"fail with findings", LLMCheckReport{Verdict: "FAIL", Findings: []LLMFinding{{Severity: "FAIL"}}}, true},
		{"warn only", LLMCheckReport{Verdict: "PASS", Findings: []LLMFinding{{Severity: "WARN"}}}, false},
		{"info only", LLMCheckReport{Verdict: "PASS", Findings: []LLMFinding{{Severity: "INFO"}}}, false},
		{"fail verdict no findings", LLMCheckReport{Verdict: "FAIL"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.report.HasViolations()
			if got != tt.want {
				t.Errorf("HasViolations() = %v, want %v", got, tt.want)
			}
		})
	}
}
