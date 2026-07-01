package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
)

type fakeVerifier struct {
	reply string
	cost  float64
}

func (f fakeVerifier) Verify(context.Context, string, string) (string, float64, int64, int64, error) {
	return f.reply, f.cost, 0, 0, nil
}

// capturingVerifier records the system prompt it is handed by verify.RunFirstPass.
// Now unused by RunFirstPass (deterministic-only) but kept for RunAgentic tests.
type capturingVerifier struct {
	reply          string
	cost           float64
	capturedPrompt string
}

func (c *capturingVerifier) Verify(_ context.Context, systemPrompt, _ string) (string, float64, int64, int64, error) {
	c.capturedPrompt = systemPrompt
	return c.reply, c.cost, 0, 0, nil
}

// verify_test.go — tests for verify.RunFirstPass (deterministic first-pass gate)
// and boundary-mock detection.
// writeTmp writes a temp file for test use and returns its path.
func writeTmp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- verify.RunFirstPass tests ---
func TestFirstPass_Pass(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("diff content"), 0644)

	res := RunFirstPass(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
	})
	if res.Verdict != verdict.Pass {
		t.Errorf("expected PASS, got %s", res.Verdict)
	}
	if res.CostUSD != 0 {
		t.Errorf("expected cost 0 (deterministic), got %f", res.CostUSD)
	}
}

func TestFirstPass_PassDoesNotWriteState(t *testing.T) {
	// RunFirstPass returns a verdict.Result but has no side effects —
	// it does not write to status.json or advance any state machine.
	// Only the agentic verifier (RunAgentic) drives state transitions.
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("diff content"), 0644)

	res := RunFirstPass(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
	})
	if res.Verdict != verdict.Pass {
		t.Errorf("expected PASS, got %s", res.Verdict)
	}
	// No status.json write occurs — the function signature takes no statusPath.
}

func TestFirstPass_Fail_ModelReplyIgnored(t *testing.T) {
	// RunFirstPass is deterministic-only. Even when a Verifier is provided
	// that would return FAIL, RunFirstPass ignores it and checks only
	// structural properties. Spec+diff are valid → PASS.
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("diff content"), 0644)

	fv := fakeVerifier{reply: "FAIL:\n1. missing coverage\n2. wrong implementation", cost: 0.02}
	res := RunFirstPass(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
		Model:    "test/model",
		Verifier: fv,
	})
	if res.Verdict != verdict.Pass {
		t.Errorf("RunFirstPass should ignore model and return deterministic PASS, got %s", res.Verdict)
	}
}
func TestFirstPass_Blocked_EmptySpec(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte(""), 0644) // empty spec
	os.WriteFile(diff, []byte("diff"), 0644)

	res := RunFirstPass(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
	})
	if res.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED, got %s", res.Verdict)
	}
}

func TestFirstPass_Blocked_EmptyDiff(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte(""), 0644) // empty diff

	res := RunFirstPass(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
	})
	if res.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED, got %s", res.Verdict)
	}
}

func TestVerifyRun_Blocked_MissingFile(t *testing.T) {
	res := RunFirstPass(context.Background(), Input{
		SpecPath: "/nonexistent/spec.md",
		DiffPath: "/nonexistent/diff.patch",
	})
	if res.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED, got %s", res.Verdict)
	}
}

// --- Proof-bundle first-pass gate (Rule 6 fail-closed) ---
// A missing, empty, or unparseable proof must never upgrade to PASS, and an
// absent proof BLOCKs when the caller requires one (the standalone CLI).
func TestFirstPass_ProofGate(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("diff content"), 0644)

	cases := []struct {
		name          string
		proofPath     string
		proofRequired bool
		want          verdict.Verdict
	}{
		{"missing_proof_file", filepath.Join(dir, "nonexistent-proof.md"), false, verdict.Blocked},
		{"empty_proof", writeTmp(t, "proof.md", "   \n"), false, verdict.Blocked},
		{"malformed_json_proof", writeTmp(t, "proof.json", "{not json"), false, verdict.Blocked},
		{"required_but_absent", "", true, verdict.Blocked},
		{"valid_md_proof", writeTmp(t, "proof.md", "# Proof\nScope: x"), true, verdict.Pass},
		{"valid_json_proof", writeTmp(t, "proof.json", `{"scope":"x"}`), true, verdict.Pass},
		// Not supplied and not required: callers that own their own absence
		// gate (RunSlice) or measure spec/diff structure only (bench).
		{"not_supplied_not_required", "", false, verdict.Pass},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := RunFirstPass(context.Background(), Input{
				SpecPath:      spec,
				DiffPath:      diff,
				ProofPath:     tc.proofPath,
				ProofRequired: tc.proofRequired,
			})
			if res.Verdict != tc.want {
				t.Fatalf("expected %s, got %s (%s)", tc.want, res.Verdict, res.Rationale)
			}
			if tc.want == verdict.Blocked {
				if res.FailedGate != "first_pass:proof" {
					t.Errorf("expected gate first_pass:proof, got %s", res.FailedGate)
				}
				if res.ExitCode() != 2 {
					t.Errorf("expected exit code 2 (BLOCKED), got %d", res.ExitCode())
				}
			}
		})
	}
}

// NOTE (ADR-0011 Step 3): TestParseVerdict* removed — the prose HasPrefix
// verdict scrape they covered is deleted. The structured authoring path is
// covered by the TestRunAgentic* cases in verify_agentic_test.go, which drive
// ChatStructured and assert schema-validated, fail-closed acceptance.

func TestBuildPayload(t *testing.T) {
	p := buildPayload("spec", "diff", "")
	if !strings.Contains(p, "## SPEC\nspec") {
		t.Error("payload should include SPEC section")
	}
	if !strings.Contains(p, "## DIFF\ndiff") {
		t.Error("payload should include DIFF section")
	}
	if strings.Contains(p, "## PROOF") {
		t.Error("empty proof should not add PROOF section")
	}

	p2 := buildPayload("spec", "diff", "proof")
	if !strings.Contains(p2, "## PROOF\nproof") {
		t.Error("non-empty proof should add PROOF section")
	}
}

func TestFirstPass_OpenDeferrals(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("+	db := mockDB // db mock\n+	auth := mockAuth // auth mock"), 0644)

	res := RunFirstPass(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
		OpenDeferrals: []state.Deferral{
			{Item: "db mock for integration tests"},
			{Item: "auth stub for test isolation"},
		},
	})
	if res.Verdict != verdict.Pass {
		t.Errorf("expected PASS with declared mocks, got %s", res.Verdict)
	}
	// Declared mocks should appear in the rationale.
	if !strings.Contains(res.Rationale, "declared boundary mock") {
		t.Errorf("declared mocks should appear in rationale, got: %q", res.Rationale)
	}
}

func TestFirstPass_UndeclaredMockFails(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("+	db := mockDB"), 0644)

	res := RunFirstPass(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
	})
	if res.Verdict != verdict.Fail {
		t.Errorf("expected FAIL for undeclared db mock, got %s", res.Verdict)
	}
	if res.FailedGate != "boundary_mock" {
		t.Errorf("expected boundary_mock gate, got %s", res.FailedGate)
	}
}

// --- S11: Agentic verifier tests ---
// (these live in verify_agentic_test.go)

// --- S10: Boundary-mock detection tests ---

func TestCheckBoundaryMocks_UndeclaredDbMockFails(t *testing.T) {
	diff := "+func TestDB(t *testing.T) {\n+	db := mockDB\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared mock, got %d", len(report.UndeclaredMocks))
	}
}

func TestCheckBoundaryMocks_DeclaredDbMockPasses(t *testing.T) {
	diff := "+func TestDB(t *testing.T) {\n+	db := mockDB\n+}"
	report := CheckBoundaryMocks(diff, []state.Deferral{{Item: "db mock for integration tests"}})
	if len(report.UndeclaredMocks) != 0 {
		t.Fatalf("want 0 undeclared, got %d", len(report.UndeclaredMocks))
	}
	if len(report.DeclaredMocks) != 1 {
		t.Fatalf("want 1 declared, got %d", len(report.DeclaredMocks))
	}
}

// AC-05: a mock declared via a real object-form deferral (the boundary+mock
// keyword living in the Why field, fired's actual shape) is recognised as
// declared — the matcher reads the structured Item/Why fields, not a flat string.
func TestCheckBoundaryMocks_DeclaredViaObjectWhyPasses(t *testing.T) {
	diff := "+func TestDB(t *testing.T) {\n+	db := mockDB\n+}"
	report := CheckBoundaryMocks(diff, []state.Deferral{{
		Why:             "db mock used for integration tests until the real harness lands",
		Tracking:        "#123",
		Acknowledgement: "Brad (Coach)",
	}})
	if len(report.UndeclaredMocks) != 0 {
		t.Fatalf("AC-05: declared-via-Why mock must not be flagged, got %d undeclared", len(report.UndeclaredMocks))
	}
	if len(report.DeclaredMocks) != 1 {
		t.Fatalf("AC-05: want 1 declared, got %d", len(report.DeclaredMocks))
	}
}

// AC-05 (strictness): a boundary+mock keyword that appears ONLY in Tracking or
// Acknowledgement (IDs/URLs) must NOT declare the mock — those fields are not
// matched, so an undeclared boundary mock still fails closed. This keeps
// enforcement at least as strict as the prior free-form []string match (Rule 10).
func TestCheckBoundaryMocks_KeywordInTrackingDoesNotOverDeclare(t *testing.T) {
	diff := "+func TestDB(t *testing.T) {\n+	db := mockDB\n+}"
	report := CheckBoundaryMocks(diff, []state.Deferral{{
		Item:            "unrelated deferral",
		Why:             "an unrelated reason with no boundary or test-double keyword",
		Tracking:        "https://issues/db-mock-tracking",
		Acknowledgement: "db mock acknowledged by someone",
	}})
	if len(report.DeclaredMocks) != 0 {
		t.Fatalf("AC-05: keyword in Tracking/Acknowledgement must not over-declare, got %d declared", len(report.DeclaredMocks))
	}
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("AC-05: undeclared boundary mock must still fail closed, got %d undeclared", len(report.UndeclaredMocks))
	}
}

func TestCheckBoundaryMocks_NonBoundaryMockNotFlagged(t *testing.T) {
	diff := "+func TestMock(t *testing.T) {\n+	m := mockHTTPClient\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 0 {
		t.Fatalf("non-boundary mock should not be flagged, got %d", len(report.UndeclaredMocks))
	}
}

func TestCheckBoundaryMocks_AuthMockUndeclaredFails(t *testing.T) {
	diff := "+func TestAuth(t *testing.T) {\n+	auth := mockAuth\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared auth mock, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "auth" {
		t.Fatalf("want auth boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_EntitlementMockUndeclaredFails(t *testing.T) {
	diff := "+func TestEntitlement(t *testing.T) {\n+	ent := mockEntitle\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared entitlement mock, got %d", len(report.UndeclaredMocks))
	}
}

func TestCheckBoundaryMocks_FakeDbDetected(t *testing.T) {
	diff := "+func TestDB(t *testing.T) {\n+	var db fakeDB\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared fake for db boundary, got %d", len(report.UndeclaredMocks))
	}
}

func TestCheckBoundaryMocks_EmptyDiffReturnsEmpty(t *testing.T) {
	report := CheckBoundaryMocks("", nil)
	if len(report.UndeclaredMocks) != 0 {
		t.Fatalf("empty diff should return empty, got %d", len(report.UndeclaredMocks))
	}
}

func TestCheckBoundaryMocks_MultipleBoundaryMocksAllFlagged(t *testing.T) {
	diff := "+var db mockDB\n+var auth mockAuth\n+var ent mockEntitle\n"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 3 {
		t.Fatalf("want 3 undeclared, got %d", len(report.UndeclaredMocks))
	}
}

func TestCheckBoundaryMocks_StubAuthDetected(t *testing.T) {
	diff := "+func TestAuth(t *testing.T) {\n+	var auth stubAuth\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared stub for auth boundary, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "auth" {
		t.Fatalf("want auth boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_StubDbDetected(t *testing.T) {
	diff := "+func TestDB(t *testing.T) {\n+	var db stubDB\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared stub for db boundary, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "db" {
		t.Fatalf("want db boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_CreditsEntitlementBoundary(t *testing.T) {
	diff := "+func TestCredits(t *testing.T) {\n+	mockCredits := mock.New(ctrl)\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared mock for credits boundary, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "entitlement" {
		t.Fatalf("want entitlement boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_KeylessEntitlementBoundary(t *testing.T) {
	diff := "+func TestKeyless(t *testing.T) {\n+	mockKeyless := mock.New(ctrl)\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared mock for keyless boundary, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "entitlement" {
		t.Fatalf("want entitlement boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_ClaudePBillingBoundary(t *testing.T) {
	// claude -p on the same line as a mock to trigger detection.
	diff := "+\tmockExec := mock.New(ctrl) // mock for claude -p billing call\n"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared mock for claude -p boundary, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "entitlement" {
		t.Fatalf("want entitlement boundary for claude -p, got %s", report.UndeclaredMocks[0].Boundary)
	}
}
