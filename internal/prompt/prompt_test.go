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

func TestCaptain_ResolveDirtyWorktree(t *testing.T) {
	got := Captain()
	// v0.5.0 Captain has a design-review function that checks for
	// drift and stale state before code is written. The specific
	// "resolve-dirty-worktree" function name was a Sworn-native
	// addition (S36) not present in canonical baton v0.5.0.
	if !strings.Contains(got, "design-review") {
		t.Errorf("Captain() missing design-review function")
	}
	if !strings.Contains(got, "Step 1 — Drift detection") {
		t.Errorf("Captain() missing drift detection step")
	}
}
func TestBatonVersion_NonEmpty(t *testing.T) {
	if got := BatonVersion(); got == "" {
		t.Fatal("BatonVersion() returned empty string")
	}
}
func TestPlannerHasPhase2b(t *testing.T) {
	got := Planner()
	// v0.5.0 planner uses "Phase 2 — Discovery" with Layers 1-6
	// instead of the Sworn-specific Phase 2b headings.
	headings := []string{
		"Phase 2 — Discovery",
		"Phase 3 — Propose decomposition",
		"Phase 3b — Group slices into tracks",
		"Phase 6 — Handoff",
	}
	for _, h := range headings {
		if !strings.Contains(got, h) {
			t.Errorf("Planner() missing heading %q", h)
		}
	}
}

func TestPlannerPhase2bDRYGate(t *testing.T) {
	got := Planner()
	// v0.5.0 planner has Phase 3b for track grouping
	// (Sworn-specific "docs/decisions.md DRY gate" is not present).
	if !strings.Contains(got, "Phase 3b — Group slices into tracks") {
		t.Errorf("Planner() missing Phase 3b heading")
	}
}

func TestPlannerPhase2bFastPath(t *testing.T) {
	got := Planner()
	// v0.5.0 planner has a hard-constraint stop guard.
	if !strings.Contains(got, "stop and force a") {
		t.Errorf("Planner() missing stop guard")
	}
}

func TestImplementerHasDeviationCheck(t *testing.T) {
	got := Implementer()
	// v0.5.0 implementer has "Required reading at session start"
	// and fresh-context boundary as pre-flight discipline.
	// (Sworn-specific "Deviation check" and "Worktree cleanliness gate"
	// headings are not in canonical baton v0.5.0.)
	if !strings.Contains(got, "Required reading at session start") {
		t.Errorf("Implementer() missing 'Required reading at session start' heading")
	}
}

func TestImplementerHasDependencyDiscipline(t *testing.T) {
	got := Implementer()
	// v0.5.0 implementer has explicit hard constraints and
	// "What you must never do" section.
	if !strings.Contains(got, "What you must never do") {
		t.Errorf("Implementer() missing 'What you must never do' section")
	}
}

func TestVerifierHasCatalogConformance(t *testing.T) {
	got := Verifier()
	// v0.5.0 verifier Gate 6 is "Design conformance (Rule 9, Layer 1)".
	// (Sworn-specific "Catalog conformance check" is not in canonical baton.)
	if !strings.Contains(got, "Gate 6 — Design conformance") {
		t.Errorf("Verifier() missing 'Gate 6 — Design conformance' heading")
	}
}

// --- Baton protocol embed tests (S21-canonical-baton) ---

func TestBatonRulesNonEmpty(t *testing.T) {
	got, err := Baton("rules.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 100 {
		t.Errorf("Baton(\"rules.md\") returned %d bytes, want > 100", len(got))
	}
}

func TestBatonAllKeys(t *testing.T) {
	all := BatonAll()
	required := []string{"rules.md", "track-mode.md", "session-discipline.md", "brainstorm-patterns.md", "README.md", "VERSION.txt"}
	for _, k := range required {
		if v, ok := all[k]; !ok {
			t.Errorf("BatonAll() missing key %q", k)
		} else if strings.TrimSpace(v) == "" {
			t.Errorf("BatonAll()[%q] is empty", k)
		}
	}
}

func TestBatonRulesHasAllTen(t *testing.T) {
	got, err := Baton("rules.md")
	if err != nil {
		t.Fatal(err)
	}
	mustContain := []string{
		"Requirements Fidelity",
		"Design Fidelity",
		"Customer Journey Validation",
	}
	for _, token := range mustContain {
		if !strings.Contains(got, token) {
			t.Errorf("Baton(\"rules.md\") missing rule name %q — stale seven-rule set?", token)
		}
	}
}

func TestBatonMissingFile(t *testing.T) {
	_, err := Baton("nonexistent.md")
	if err == nil {
		t.Fatal("Baton(\"nonexistent.md\") returned nil error, want error")
	}
}

// --- S27-public-readiness-scrub tests ---

func TestEmbeddedPromptsPublicSafe(t *testing.T) {
	prompts := map[string]string{
		"Captain":     Captain(),
		"Implementer": Implementer(),
		"Verifier":    Verifier(),
		"Planner":     Planner(),
	}
	banned := []string{
		"coach" + "-loop",
		"--" + "auto-ack",
		"approved" + "-ack",
		"captain" + "-route",
		"[[" + "project_",
		"[[" + "feedback_",
		"S21 stall",
	}
	for name, text := range prompts {
		for _, token := range banned {
			if strings.Contains(text, token) {
				t.Errorf("%s() contains banned token %q", name, token)
			}
		}
	}
}

func TestCaptainKeepsRoleVocab(t *testing.T) {
	got := Captain()
	if !strings.Contains(got, "Captain") {
		t.Errorf("Captain() does not contain 'Captain' — role vocab must be retained")
	}
	if !strings.Contains(got, "Coach") {
		t.Errorf("Captain() does not contain 'Coach' — role vocab must be retained")
	}
}
