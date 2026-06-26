package tui

import "testing"

func TestGateResultRenderInline_AllClean(t *testing.T) {
	g := GateResult{
		TraceVerdict: "PASS",
		CoveragePct:  "8/8",
		DesignCount:  0,
		MockStatus:   "clean",
		LLMResult:    "PASS",
	}
	line := g.RenderInline()
	// Should contain all five indicators.
	for _, want := range []string{"T:", "C:", "D:", "M:", "L:"} {
		if !contains(line, want) {
			t.Errorf("expected %q in render, got: %s", want, line)
		}
	}
	if g.HasFailures() {
		t.Error("all-clean gate should not have failures")
	}
	if !g.IsClean() {
		t.Error("all-clean gate should be clean")
	}
}

func TestGateResultRenderInline_AllFailures(t *testing.T) {
	g := GateResult{
		TraceVerdict: "FAIL",
		CoveragePct:  "3/8",
		DesignCount:  5,
		MockStatus:   "flagged",
		LLMResult:    "FAIL",
	}
	if !g.HasFailures() {
		t.Error("all-fail gate should have failures")
	}
	if g.IsClean() {
		t.Error("all-fail gate should not be clean")
	}
	line := g.RenderInline()
	if line == "" {
		t.Error("expected non-empty render")
	}
}

func TestGateResultRenderInline_Empty(t *testing.T) {
	g := GateResult{DesignCount: -1}
	line := g.RenderInline()
	if !contains(line, "no gates") {
		t.Errorf("expected 'no gates' for empty result, got: %s", line)
	}
}

func TestGateResultRenderInline_Partial(t *testing.T) {
	g := GateResult{
		TraceVerdict: "PASS",
		CoveragePct:  "5/10",
		DesignCount:  0,
		MockStatus:   "clean",
	}
	line := g.RenderInline()
	// Should have trace, coverage, design, mock — no LLM.
	if !contains(line, "T:") {
		t.Errorf("expected T: in partial render, got: %s", line)
	}
	if !contains(line, "C:") {
		t.Errorf("expected C: in partial render, got: %s", line)
	}
	if contains(line, "L:") {
		t.Errorf("unexpected L: in partial render (LLM not set): %s", line)
	}
	// Coverage partial should not be a hard failure.
	if g.HasFailures() {
		t.Error("partial coverage only should not be a hard failure")
	}
	// But it should not be strictly clean either (warnings).
	if g.IsClean() {
		t.Error("partial coverage should not be clean")
	}
}

func TestGateResultRenderInline_DesignViolationsOnly(t *testing.T) {
	g := GateResult{
		TraceVerdict: "PASS",
		CoveragePct:  "8/8",
		DesignCount:  3,
		MockStatus:   "clean",
	}
	if !g.HasFailures() {
		t.Error("design violations should be failures")
	}
	line := g.RenderInline()
	if !contains(line, "3") {
		t.Errorf("expected design count 3 in render, got: %s", line)
	}
}

func TestGateResultRenderInline_MockFlagged(t *testing.T) {
	g := GateResult{
		TraceVerdict: "PASS",
		CoveragePct:  "8/8",
		DesignCount:  0,
		MockStatus:   "flagged",
	}
	if !g.HasFailures() {
		t.Error("flagged mock should be a failure")
	}
}

func TestGateResultRenderInline_LLMCheckOnly(t *testing.T) {
	g := GateResult{
		LLMResult: "PASS",
	}
	line := g.RenderInline()
	if !contains(line, "L:") {
		t.Errorf("expected L: for LLM-only result, got: %s", line)
	}
	if contains(line, "T:") {
		t.Errorf("unexpected T: in LLM-only result: %s", line)
	}
}

func TestGateResult_DesignCountDefault(t *testing.T) {
	g := GateResult{}
	if g.DesignCount != 0 {
		t.Error("zero-value GateResult should have DesignCount 0")
	}
}

func TestIsPartialCoverage(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"8/8", false},
		{"0/0", false},
		{"8/10", true},
		{"0/5", true},
		{"5", false},
		{"", false},
		{"abc", false},
	}
	for _, tt := range tests {
		got := isPartialCoverage(tt.s)
		if got != tt.want {
			t.Errorf("isPartialCoverage(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
