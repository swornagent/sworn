package gate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/model"
)

// The live LLM-check tests.
//
// Everything else in this package is mocked, and mocks cannot prove the one thing
// that matters most here: THAT A REAL MODEL CAN SATISFY THE CONTRACT WE PUBLISHED.
//
// Baton v0.12.0 changed the check response shape — `severity` (impact) split from
// `blocking` (disposition), verdict DERIVED from the findings — and parseLLMResponse
// now validates against llm-check-report-v1 and FAILS CLOSED on violation. Sensible,
// except that no model had ever been asked to produce that shape. If models do not
// reliably emit `blocking`, every check returns a contract-violation finding and the
// gate is RED FOR EVERYONE, ALWAYS. A fail-closed gate whose failure mode is
// "everything fails" must be proved able to PASS.
//
// These run only under SWORN_LIVE_TESTS=1 with a real key, out of the PR gate — a
// live model call is non-deterministic, and a flaky gate gets muted. A muted guard is
// a decoration.

// liveVerifier builds a real model client, or skips.
func liveVerifier(t *testing.T) (model.Verifier, string) {
	t.Helper()
	if os.Getenv("SWORN_LIVE_TESTS") != "1" {
		t.Skip("live test requires SWORN_LIVE_TESTS=1")
	}
	id := os.Getenv("SWORN_LIVE_MODEL")
	if id == "" {
		id = "openai/gpt-4.1-mini"
	}
	if model.ProviderKey(strings.SplitN(id, "/", 2)[0]) == "" {
		t.Skipf("live test requires a key for %s (canonical env var, or credentials.json)", id)
	}
	v, err := model.FromEnv(id)
	if err != nil {
		t.Skipf("live model %s unavailable: %v", id, err)
	}
	return v, id
}

// securityDiff carries two deliberately MEDIUM-severity issues (per the
// security-review prompt's scale: "information leak, weak crypto, unsafe defaults").
// Medium is the interesting band — it is the one the stakes keying moves.
const securityDiff = `diff --git a/refund.go b/refund.go
--- a/refund.go
+++ b/refund.go
@@ -10,6 +10,7 @@ func HandleRefund(w http.ResponseWriter, r *http.Request) {
 	user := session.User(r)
+	log.Printf("processing refund for %s (%s)", user.Email, user.PhoneNumber)
 	amount := parseAmount(r)
 	if err := payments.Refund(user.ID, amount); err != nil {
 		http.Error(w, err.Error(), 500)
@@ -30,5 +31,6 @@ func NewRouter() http.Handler {
 	r := chi.NewRouter()
+	r.Use(cors.Handler(cors.Options{AllowedOrigins: []string{"*"}}))
 	r.Post("/refund", HandleRefund)
 	return r
 }
`

// liveRepo builds a temp repo with a slice and a RATIFIED project-context record at
// the requested stakes, so RunLLMCheck's project.Resolve finds a real declaration.
func liveRepo(t *testing.T, highStakes bool) string {
	t.Helper()
	root := t.TempDir()

	must := func(p, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	must(filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n") // RepoRootFrom walks up for .git

	stakes := `{"production": false, "real_users": false, "sensitive_data": []}`
	context := "a Go CLI, used only by its own author"
	if highStakes {
		stakes = `{"production": true, "real_users": true, "sensitive_data": ["pii", "financial"]}`
		context = "a Go payments backend serving live paying customers"
	}
	must(filepath.Join(root, ".sworn", "project.json"), `{
  "context": "`+context+`",
  "stakes": `+stakes+`,
  "ratification": {"ratified": true, "by": "live-test"}
}`)

	sliceDir := filepath.Join(root, "docs", "release", "live", "S01-live")
	must(filepath.Join(sliceDir, "spec.md"), "# Slice: S01-live\n\n## Acceptance checks\n\n- [ ] WHEN a refund is requested, THE SYSTEM SHALL process it.\n")
	must(filepath.Join(sliceDir, "status.json"), `{"slice_id":"S01-live","state":"implemented"}`)
	return sliceDir
}

// TestLive_SecurityReviewSatisfiesTheContract is the test that de-risks what we
// shipped blind: can a real model actually produce llm-check-report-v1?
//
// parseLLMResponse appends a blocking F-00 "violates llm-check-report-v1" finding on
// any schema violation. If that finding appears here, then EVERY llm-check in EVERY
// repo is failing closed on a contract no model can satisfy — the gate would be red
// for every user, always.
func TestLive_SecurityReviewSatisfiesTheContract(t *testing.T) {
	verifier, id := liveVerifier(t)
	sliceDir := liveRepo(t, true)

	report, err := RunLLMCheck(context.Background(), CheckSecurityReview, sliceDir, securityDiff, verifier)
	if err != nil {
		t.Fatalf("RunLLMCheck: %v", err)
	}

	t.Logf("model=%s verdict=%s findings=%d", id, report.Verdict, len(report.Findings))
	for _, f := range report.Findings {
		blocking := "nil"
		if f.Blocking != nil {
			blocking = map[bool]string{true: "true", false: "false"}[*f.Blocking]
		}
		t.Logf("  [%s severity=%s blocking=%s] %s", f.ID, f.Severity, blocking, f.Title)
	}

	for _, f := range report.Findings {
		if f.ID == "F-00" && strings.Contains(f.Title, "llm-check-report-v1") {
			t.Fatalf("THE PUBLISHED CONTRACT IS UNSATISFIABLE by %s.\n"+
				"parseLLMResponse rejected the model's response, so every llm-check in every "+
				"repo fails closed on a shape no model produces — the gate is red for everyone.\n"+
				"detail: %s\nraw: %.600s", id, f.Detail, report.RawResponse)
		}
	}

	// Every finding must carry the v0.12.0 disposition field, not just a severity.
	for _, f := range report.Findings {
		if f.Blocking == nil {
			t.Errorf("finding %s (%s) has no `blocking` field — the model is emitting the "+
				"pre-v0.12.0 shape, and severity alone cannot decide the verdict", f.ID, f.Title)
		}
	}

	// A diff this bad must produce SOMETHING. Silence would mean the payload never
	// reached the model.
	if len(report.Findings) == 0 {
		t.Errorf("no findings on a diff that logs PII and opens CORS to * — "+
			"the check is not seeing the diff.\nraw: %.400s", report.RawResponse)
	}
}

// TestLive_StakesRaiseTheBar proves the composition that mocks structurally cannot:
// sworn renders the stakes (unit-tested), THE MODEL grades against them (untested
// until now), and sworn honours `blocking` (unit-tested). Only the middle was unproven
// — and it is the entire point of the stakes work.
//
// The assertion is MONOTONICITY, not an exact verdict: the same diff, graded at high
// stakes, must be at least as strict as at low stakes. That is the actual contract
// ("stakes raise the bar"), and it is robust to the model variance that would make an
// exact-verdict assertion flaky — a flaky guard gets muted, and a muted guard is a
// decoration.
func TestLive_StakesRaiseTheBar(t *testing.T) {
	verifier, id := liveVerifier(t)

	run := func(highStakes bool) *LLMCheckReport {
		t.Helper()
		report, err := RunLLMCheck(context.Background(), CheckSecurityReview,
			liveRepo(t, highStakes), securityDiff, verifier)
		if err != nil {
			t.Fatalf("RunLLMCheck(highStakes=%v): %v", highStakes, err)
		}
		return report
	}

	low := run(false)
	high := run(true)

	describe := func(label string, r *LLMCheckReport) {
		t.Logf("%s stakes (%s): verdict=%s blocks=%v", label, id, r.Verdict, r.HasViolations())
		for _, f := range r.Findings {
			b := "nil"
			if f.Blocking != nil {
				b = map[bool]string{true: "true", false: "false"}[*f.Blocking]
			}
			t.Logf("    severity=%-8s blocking=%-5s %s", f.Severity, b, f.Title)
		}
	}
	describe("LOW", low)
	describe("HIGH", high)

	if low.HasViolations() && !high.HasViolations() {
		lowJSON, _ := json.Marshal(low.Findings)
		highJSON, _ := json.Marshal(high.Findings)
		t.Fatalf("STAKES INVERTED: the same diff BLOCKS at low stakes and PASSES at high stakes.\n"+
			"High stakes must be at least as strict — a system serving real customers with "+
			"financial data cannot be graded more leniently than a personal CLI.\n"+
			"low:  %s\nhigh: %s", lowJSON, highJSON)
	}

	// Observational, not asserted: whether a medium finding actually flips to blocking
	// at high stakes is the model's judgement against the published prompt. Log it so a
	// human can see whether the stakes keying is doing real work, without making the
	// gate hostage to model variance.
	if !low.HasViolations() && high.HasViolations() {
		t.Logf("STAKES KEYING FIRED: the same diff is advisory at low stakes and BLOCKING at high stakes.")
	}
}
