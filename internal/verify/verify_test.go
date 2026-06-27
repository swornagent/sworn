package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/verdict"
)

type fakeVerifier struct {
	reply string
	cost  float64
}

func (f fakeVerifier) Verify(context.Context, string, string) (string, float64, error) {
	return f.reply, f.cost, nil
}

// capturingVerifier records the system prompt it is handed by verify.Run.
type capturingVerifier struct {
	reply          string
	cost           float64
	capturedPrompt string
}

func (c *capturingVerifier) Verify(_ context.Context, systemPrompt, _ string) (string, float64, error) {
	c.capturedPrompt = systemPrompt
	return c.reply, c.cost, nil
}

// verify_test.go — tests for verify.Run (stateless judge) and boundary-mock detection.

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

// --- verify.Run tests ---
func TestVerifyRun_Pass(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("diff content"), 0644)

	fv := fakeVerifier{reply: "PASS\n\nlooks good", cost: 0.01}
	res := Run(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
		Model:    "test/model",
		Verifier: fv,
	})
	if res.Verdict != verdict.Pass {
		t.Errorf("expected PASS, got %s", res.Verdict)
	}
	if res.CostUSD != 0.01 {
		t.Errorf("expected cost 0.01, got %f", res.CostUSD)
	}
}

func TestVerifyRun_Fail(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("diff content"), 0644)

	fv := fakeVerifier{reply: "FAIL:\n1. missing coverage\n2. wrong implementation", cost: 0.02}
	res := Run(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
		Model:    "test/model",
		Verifier: fv,
	})
	if res.Verdict != verdict.Fail {
		t.Errorf("expected FAIL, got %s", res.Verdict)
	}
}

func TestVerifyRun_Blocked_EmptySpec(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte(""), 0644) // empty spec
	os.WriteFile(diff, []byte("diff"), 0644)

	res := Run(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
	})
	if res.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED, got %s", res.Verdict)
	}
}

func TestVerifyRun_Blocked_EmptyDiff(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte(""), 0644) // empty diff

	res := Run(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
	})
	if res.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED, got %s", res.Verdict)
	}
}

func TestVerifyRun_Blocked_MissingFile(t *testing.T) {
	res := Run(context.Background(), Input{
		SpecPath: "/nonexistent/spec.md",
		DiffPath: "/nonexistent/diff.patch",
	})
	if res.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED, got %s", res.Verdict)
	}
}

func TestParseVerdictPass(t *testing.T) {
	cases := []string{
		"PASS",
		"PASS\n\nwith details",
		"```\nPASS",
		"**PASS**",
		"  PASS  ",
	}
	for _, c := range cases {
		r := parseVerdict(c, 0)
		if r.Verdict != verdict.Pass {
			t.Errorf("parseVerdict(%q): expected PASS, got %s", c, r.Verdict)
		}
	}
}

func TestParseVerdictFail(t *testing.T) {
	cases := []string{
		"FAIL: something wrong",
		"FAIL",
		"**FAIL**: missing tests",
	}
	for _, c := range cases {
		r := parseVerdict(c, 0)
		if r.Verdict != verdict.Fail {
			t.Errorf("parseVerdict(%q): expected FAIL, got %s", c, r.Verdict)
		}
	}
}

func TestParseVerdictBlocked(t *testing.T) {
	cases := []string{
		"BLOCKED: spec ambiguous",
		"BLOCKED",
	}
	for _, c := range cases {
		r := parseVerdict(c, 0)
		if r.Verdict != verdict.Blocked {
			t.Errorf("parseVerdict(%q): expected BLOCKED, got %s", c, r.Verdict)
		}
	}
}

func TestParseVerdictInconclusive(t *testing.T) {
	r := parseVerdict("INCONCLUSIVE: dev server unreachable", 0)
	if r.Verdict != verdict.Inconclusive {
		t.Errorf("expected INCONCLUSIVE, got %s", r.Verdict)
	}
}

func TestParseVerdictUnparseableBlocks(t *testing.T) {
	r := parseVerdict("Here is a detailed analysis", 0)
	if r.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED for unparseable, got %s", r.Verdict)
	}
	if r.FailedGate != "unparseable_verdict" {
		t.Errorf("expected unparseable_verdict, got %s", r.FailedGate)
	}
}

func TestSystemPromptIsStatelessJudge(t *testing.T) {
	cv := &capturingVerifier{reply: "PASS", cost: 0.01}
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("diff"), 0644)
	Run(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
		Model:    "test/model",
		Verifier: cv,
	})
	if !strings.Contains(cv.capturedPrompt, "SPEC+DIFF") {
		t.Error("system prompt should contain SPEC+DIFF clue")
	}
	if strings.Contains(cv.capturedPrompt, "Verifier Role Prompt") {
		t.Error("stateless judge should NOT use the full verifier role prompt")
	}
}

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

func TestVerifyRun_OpenDeferrals(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("+	db := mockDB // db mock\n+	auth := mockAuth // auth mock"), 0644)

	fv := fakeVerifier{reply: "PASS", cost: 0.01}
	res := Run(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
		Model:    "test/model",
		Verifier: fv,
		OpenDeferrals: []string{
			"db mock for integration tests",
			"auth stub for test isolation",
		},
	})
	if res.Verdict != verdict.Pass {
		t.Errorf("expected PASS with declared mocks, got %s", res.Verdict)
	}
	// Declared mocks should appear in the rationale.
	if !strings.Contains(res.Rationale, "Declared boundary mock") {
		t.Error("declared mocks should appear in rationale")
	}
}

func TestVerifyRun_UndeclaredMockBlocks(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	os.WriteFile(spec, []byte("# spec"), 0644)
	os.WriteFile(diff, []byte("+	db := mockDB"), 0644)

	fv := fakeVerifier{reply: "PASS", cost: 0}
	res := Run(context.Background(), Input{
		SpecPath: spec,
		DiffPath: diff,
		Model:    "test/model",
		Verifier: fv,
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
	report := CheckBoundaryMocks(diff, []string{"db mock for integration tests"})
	if len(report.UndeclaredMocks) != 0 {
		t.Fatalf("want 0 undeclared, got %d", len(report.UndeclaredMocks))
	}
	if len(report.DeclaredMocks) != 1 {
		t.Fatalf("want 1 declared, got %d", len(report.DeclaredMocks))
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