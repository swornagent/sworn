package gate

import (
	"context"
	"strings"
	"testing"
)

func boolp(b bool) *bool { return &b }

// TestHasViolations_SecurityFailOpen is the regression guard for sworn#103.
//
// The defect: HasViolations() string-matched severity == "FAIL". security-review
// grades critical/high/medium/low and never emits "FAIL", so the loop was dead
// code for that check and blocking degraded to `r.Verdict != "PASS"` — the
// model's own self-assessment. A critical RCE finding alongside a self-declared
// PASS passed the gate green.
//
// This is the shape the defect actually takes: the security check, the
// critical/high vocabulary, and a model that says PASS anyway.
func TestHasViolations_SecurityFailOpen(t *testing.T) {
	for _, sev := range []string{"critical", "high"} {
		t.Run(sev, func(t *testing.T) {
			r := &LLMCheckReport{
				CheckType: CheckSecurityReview,
				Verdict:   "PASS", // model self-declares a pass
				Findings: []LLMFinding{{
					ID: "F-01", Severity: sev,
					Title:  "Remote code execution via unsanitised exec()",
					Detail: "User input flows into os/exec without validation.",
				}},
			}
			if !r.HasViolations() {
				t.Errorf("FAIL-OPEN: severity %q + verdict PASS did not block.\n"+
					"a %s security finding must fail the check regardless of the "+
					"model's stated verdict — trusting verdict alone is self-certification",
					sev, sev)
			}
		})
	}
}

// TestHasViolations_AdvisorySecurityFindingsDoNotBlock keeps the fix from
// over-correcting into a gate that blocks on every observation.
func TestHasViolations_AdvisorySecurityFindingsDoNotBlock(t *testing.T) {
	for _, sev := range []string{"medium", "low", "info"} {
		t.Run(sev, func(t *testing.T) {
			r := &LLMCheckReport{
				CheckType: CheckSecurityReview,
				Verdict:   "PASS",
				Findings:  []LLMFinding{{ID: "F-01", Severity: sev, Title: "t", Detail: "d"}},
			}
			if r.HasViolations() {
				t.Errorf("severity %q is advisory and must not block a PASS", sev)
			}
		})
	}
}

// TestHasViolations_LegacyVocabularyStillBlocks — the other five checks grade
// FAIL/WARN/INFO and must keep working unchanged.
func TestHasViolations_LegacyVocabularyStillBlocks(t *testing.T) {
	blocking := &LLMCheckReport{
		CheckType: CheckACSatisfaction,
		Verdict:   "PASS",
		Findings:  []LLMFinding{{ID: "F-01", Severity: "FAIL", Title: "t", Detail: "d"}},
	}
	if !blocking.HasViolations() {
		t.Error("severity FAIL must block even when the model says PASS")
	}

	advisory := &LLMCheckReport{
		CheckType: CheckACSatisfaction,
		Verdict:   "PASS",
		Findings: []LLMFinding{
			{ID: "F-01", Severity: "WARN", Title: "t", Detail: "d"},
			{ID: "F-02", Severity: "INFO", Title: "t", Detail: "d"},
		},
	}
	if advisory.HasViolations() {
		t.Error("WARN/INFO are advisory and must not block a PASS")
	}
}

// TestHasViolations_BlockingEscalatesButCannotDeEscalate pins the asymmetry:
// Baton v0.12.0's `blocking` flag may promote a finding to blocking, but a model
// may not wave a critical finding through by claiming blocking: false.
func TestHasViolations_BlockingEscalatesButCannotDeEscalate(t *testing.T) {
	escalated := &LLMCheckReport{
		CheckType: CheckSecurityReview,
		Verdict:   "FAIL",
		Findings:  []LLMFinding{{ID: "F-01", Severity: "medium", Blocking: boolp(true), Title: "t", Detail: "d"}},
	}
	if !escalated.HasViolations() {
		t.Error("blocking:true must escalate a medium finding into a block")
	}

	// A critical finding claiming blocking:false violates the security-review
	// prompt's own rule (critical always blocks). Fail closed, do not believe it.
	deEscalated := &LLMCheckReport{
		CheckType: CheckSecurityReview,
		Verdict:   "PASS",
		Findings:  []LLMFinding{{ID: "F-01", Severity: "critical", Blocking: boolp(false), Title: "t", Detail: "d"}},
	}
	if !deEscalated.HasViolations() {
		t.Error("blocking:false must NOT de-escalate a critical finding — that would " +
			"reopen the fail-open through a different door")
	}
}

// TestHasViolations_UnknownSeverityFailsClosed — an ungradeable finding is not a
// pass (Rule 7: absence of evidence is FAIL).
func TestHasViolations_UnknownSeverityFailsClosed(t *testing.T) {
	for _, sev := range []string{"moderate", "sev1", ""} {
		r := &LLMCheckReport{
			CheckType: CheckSecurityReview,
			Verdict:   "PASS",
			Findings:  []LLMFinding{{ID: "F-01", Severity: sev, Title: "t", Detail: "d"}},
		}
		if !r.HasViolations() {
			t.Errorf("unrecognised severity %q must fail closed, not pass", sev)
		}
	}
}

// TestHasViolations_ModelFailVerdictAlwaysBlocks — a model that declares FAIL is
// believed even with no findings. The verdict can only ever add a block.
func TestHasViolations_ModelFailVerdictAlwaysBlocks(t *testing.T) {
	r := &LLMCheckReport{CheckType: CheckSpecAmbiguity, Verdict: "FAIL", Findings: nil}
	if !r.HasViolations() {
		t.Error("a FAIL verdict must block even with no findings")
	}
}

// TestCheckIdentityMismatchFailsClosed keeps a model from borrowing the
// requested label. Missing and unknown values fail schema validation; a wrong
// known check reaches the explicit requested/emitted equality gate.
func TestCheckIdentityMismatchFailsClosed(t *testing.T) {
	dir := fixture(t, map[string]string{
		"S01-test/spec.md": "# Slice: S01-test\n\n## Acceptance checks\n\n- [ ] THE SYSTEM SHALL fail closed.\n",
	})

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "wrong known identity",
			raw:  `{"check":"design-review","verdict":"PASS","findings":[]}`,
			want: "identity mismatch",
		},
		{
			name: "missing identity",
			raw:  `{"verdict":"PASS","findings":[]}`,
			want: "llm-check-report-v1",
		},
		{
			name: "unknown identity",
			raw:  `{"check":"unknown-check","verdict":"PASS","findings":[]}`,
			want: "llm-check-report-v1",
		},
		{
			name: "duplicate identity",
			raw:  `{"check":"ac-satisfaction","check":"design-review","verdict":"PASS","findings":[]}`,
			want: "Unparseable model response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVerifier{text: tt.raw}
			report, err := RunLLMCheck(context.Background(), CheckACSatisfaction, dir+"/S01-test", "", mock)
			if err != nil {
				t.Fatalf("RunLLMCheck: %v", err)
			}
			if !report.HasViolations() || report.Verdict != "FAIL" {
				t.Fatalf("identity failure must block, got %+v", report)
			}
			if !strings.Contains(report.RawResponse, `"verdict":"PASS"`) {
				t.Fatalf("raw model response was lost: %q", report.RawResponse)
			}
			var details []string
			for _, finding := range report.Findings {
				details = append(details, finding.Title+" "+finding.Detail)
			}
			if !strings.Contains(strings.Join(details, "\n"), tt.want) {
				t.Fatalf("failure detail = %q, want %q", strings.Join(details, "\n"), tt.want)
			}
			if mock.structuredCalls != 1 || mock.verifyCalls != 0 {
				t.Fatalf("structured/raw calls = %d/%d, want 1/0", mock.structuredCalls, mock.verifyCalls)
			}
		})
	}
}
