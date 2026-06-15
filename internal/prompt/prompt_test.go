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

func TestBatonVersion_NonEmpty(t *testing.T) {
	if got := BatonVersion(); got == "" {
		t.Fatal("BatonVersion() returned empty string")
	}
}