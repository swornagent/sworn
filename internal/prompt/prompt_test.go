package prompt

import (
	"strings"
	"testing"
)

const oldPlaceholder = `You are an adversarial verifier. You see only the spec, the diff, and the proof.
You never saw the implementer's reasoning. Return exactly one of:
PASS | FAIL: <numbered violations> | BLOCKED: <reason>. Fail closed: absence of
evidence is FAIL, not optimistic PASS.`

func TestVerifier_NonEmpty(t *testing.T) {
	if got := Verifier(); strings.TrimSpace(got) == "" {
		t.Fatal("Verifier() returned empty string — embed may have failed")
	}
}

func TestVerifier_ContainsVerdictContract(t *testing.T) {
	got := Verifier()
	for _, token := range []string{"PASS", "FAIL", "BLOCKED"} {
		if !strings.Contains(got, token) {
			t.Errorf("Verifier() missing verdict token %q", token)
		}
	}
}

// Negative check: the embedded verifier prompt must differ from the old
// inline const that preceded go:embed vendoring. A silent vendoring failure
// (wrong path, empty embed) would pass the positive checks above because
// the old const also contains PASS/FAIL/BLOCKED.
func TestVerifier_NotOldPlaceholder(t *testing.T) {
	got := strings.TrimSpace(Verifier())
	if got == strings.TrimSpace(oldPlaceholder) {
		t.Fatal("Verifier() returned the old inline const — vendored prompt not embedded")
	}
}

func TestVerifier_ContainsInconclusive(t *testing.T) {
	got := Verifier()
	if !strings.Contains(got, "INCONCLUSIVE") {
		t.Errorf("Verifier() missing INCONCLUSIVE token — prompt may not be the real Baton verifier prompt")
	}
}

func TestImplementer_NonEmpty(t *testing.T) {
	if got := Implementer(); strings.TrimSpace(got) == "" {
		t.Fatal("Implementer() returned empty string — embed may have failed")
	}
}

func TestPlanner_NonEmpty(t *testing.T) {
	if got := Planner(); strings.TrimSpace(got) == "" {
		t.Fatal("Planner() returned empty string — embed may have failed")
	}
}

func TestCaptain_NonEmpty(t *testing.T) {
	if got := Captain(); strings.TrimSpace(got) == "" {
		t.Fatal("Captain() returned empty string — embed may have failed")
	}
}

func TestVerifyStateless_NonEmpty(t *testing.T) {
	if got := VerifyStateless(); strings.TrimSpace(got) == "" {
		t.Fatal("VerifyStateless() returned empty string — embed may have failed")
	}
}

func TestVerifyStateless_StatelessMarkers(t *testing.T) {
	got := VerifyStateless()
	markers := []string{
		"no tools",
		"SPEC+DIFF only",
		"verdict-leading",
		"PASS",
		"FAIL",
		"BLOCKED",
		"INCONCLUSIVE",
	}
	for _, m := range markers {
		if !strings.Contains(got, m) {
			t.Errorf("VerifyStateless() missing marker %q", m)
		}
	}
}

func TestVerifyStateless_NotAgenticVerifier(t *testing.T) {
	got := VerifyStateless()
	agenticTokens := []string{
		"walk a worktree",
		"git worktree",
		"git -C",
		"run tests",
		"fresh terminal",
		"Baton verifier",
		"investigating agent",
	}
	for _, tok := range agenticTokens {
		if strings.Contains(got, tok) {
			t.Errorf("VerifyStateless() contains agentic token %q — should be a pure judge, not an investigator", tok)
		}
	}
}

func TestBatonVersion_NonEmpty(t *testing.T) {
	if got := BatonVersion(); got == "" {
		t.Fatal("BatonVersion() returned empty string")
	}
}
func TestPlannerHasPhase2b(t *testing.T) {
	got := Planner()
	headings := []string{
		"Registry check",
		"Design consultation",
		"Architecture conformance",
		"Capture",
	}
	for _, h := range headings {
		if !strings.Contains(got, h) {
			t.Errorf("Planner() missing Phase 2b heading %q", h)
		}
	}
}

func TestPlannerPhase2bDRYGate(t *testing.T) {
	got := Planner()
	if !strings.Contains(got, "docs/decisions.md") {
		t.Errorf("Planner() missing DRY gate reference to docs/decisions.md")
	}
}

func TestPlannerPhase2bFastPath(t *testing.T) {
	got := Planner()
	if !strings.Contains(got, "do not block") {
		t.Errorf("Planner() missing fast-path guard: 'do not block' for missing catalog files")
	}
}

func TestImplementerHasDeviationCheck(t *testing.T) {
	got := Implementer()
	if !strings.Contains(got, "Deviation check") {
		t.Errorf("Implementer() missing 'Deviation check' heading")
	}
}

func TestImplementerHasDependencyDiscipline(t *testing.T) {
	got := Implementer()
	if !strings.Contains(got, "Dependency discipline") {
		t.Errorf("Implementer() missing 'Dependency discipline' heading")
	}
}

func TestVerifierHasCatalogConformance(t *testing.T) {
	got := Verifier()
	if !strings.Contains(got, "Catalog conformance check") {
		t.Errorf("Verifier() missing 'Catalog conformance check' heading")
	}
}