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
	reply        string
	cost         float64
	capturedPrompt string
}

func (c *capturingVerifier) Verify(_ context.Context, systemPrompt, _ string) (string, float64, error) {
	c.capturedPrompt = systemPrompt
	return c.reply, c.cost, nil
}
func writeTmp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRun_PassExitsZero(t *testing.T) {
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "PASS - meets the spec", cost: 0.01},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Pass || got.ExitCode() != 0 {
		t.Fatalf("want PASS/0, got %s/%d", got.Verdict, got.ExitCode())
	}
}

func TestRun_MissingSpecBlocks(t *testing.T) {
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "   "), // empty -> first-pass blocks
		DiffPath: writeTmp(t, "c.diff", "+ x"),
		Verifier: fakeVerifier{reply: "PASS"},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Blocked || got.ExitCode() != 2 {
		t.Fatalf("want BLOCKED/2, got %s/%d", got.Verdict, got.ExitCode())
	}
}

func TestRun_UnconfiguredModelFailsClosed(t *testing.T) {
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		// no Verifier -> Unconfigured -> BLOCKED
	}
	if got := Run(context.Background(), in); got.Verdict != verdict.Blocked {
		t.Fatalf("want BLOCKED, got %s", got.Verdict)
	}
}

func TestRun_MissingFileBlocks(t *testing.T) {
	in := Input{
		SpecPath: filepath.Join(t.TempDir(), "does-not-exist.md"),
		DiffPath: writeTmp(t, "c.diff", "+ x"),
		Verifier: fakeVerifier{reply: "PASS"},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Blocked || got.FailedGate != "first_pass:spec" {
		t.Fatalf("want BLOCKED/first_pass:spec, got %s/%s", got.Verdict, got.FailedGate)
	}
}

func TestRun_GarbledVerdictBlocks(t *testing.T) {
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "looks good to me!"}, // no PASS/FAIL/BLOCKED prefix
	}
	if got := Run(context.Background(), in); got.Verdict != verdict.Blocked {
		t.Fatalf("want BLOCKED on unparseable, got %s", got.Verdict)
	}
}

// --- S02: tolerant verdict parser ---

func TestParseVerdict_MarkdownEmphasis(t *testing.T) {
	// **FAIL** (markdown bold) must resolve to FAIL.
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "**FAIL** — spec clause 3 not met", cost: 0.01},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Fail || got.ExitCode() != 1 {
		t.Fatalf("want FAIL/1, got %s/%d", got.Verdict, got.ExitCode())
	}
}

func TestParseVerdict_LeadingBlankLines(t *testing.T) {
	// One or more leading blank lines before PASS must still resolve to PASS.
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "\n\n\nPASS — all checks green", cost: 0.01},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Pass || got.ExitCode() != 0 {
		t.Fatalf("want PASS/0, got %s/%d", got.Verdict, got.ExitCode())
	}
}

func TestParseVerdict_LeadingFence(t *testing.T) {
	// A bare ``` fence line before the verdict must be skipped.
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "```\nPASS — verifier confirms", cost: 0.01},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Pass || got.ExitCode() != 0 {
		t.Fatalf("want PASS/0, got %s/%d", got.Verdict, got.ExitCode())
	}
}

func TestParseVerdict_ToolCallLeakBlocks(t *testing.T) {
	// <tool_call name="Bash"> as first non-empty line must BLOCK, never parse
	// as a verdict.
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: `<tool_call name="Bash">
{"command": "cat spec.md"}
</tool_call>`, cost: 0.01},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Blocked || got.FailedGate != "unparseable_verdict" {
		t.Fatalf("want BLOCKED/unparseable_verdict, got %s/%s", got.Verdict, got.FailedGate)
	}
}

func TestParseVerdict_ProsePreambleBlocks(t *testing.T) {
	// Investigative prose before the verdict must BLOCK — no false PASS.
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "Verifying slice S03 — checking acceptance criteria now…", cost: 0.01},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Blocked || got.FailedGate != "unparseable_verdict" {
		t.Fatalf("want BLOCKED/unparseable_verdict, got %s/%s", got.Verdict, got.FailedGate)
	}
}

// TestRun_SystemPromptIsStateless validates that verify.Run passes the
// stateless judge prompt (VerifyStateless) to the model, NOT the agentic
// verifier role prompt (Verifier).
func TestRun_SystemPromptIsStateless(t *testing.T) {	cv := &capturingVerifier{reply: "PASS - looks good", cost: 0.01}
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: cv,
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Pass {
		t.Fatalf("want PASS, got %s", got.Verdict)
	}

	prompt := cv.capturedPrompt
	// Must contain stateless markers.
	for _, want := range []string{"no tools", "SPEC+DIFF only", "verdict-leading"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt missing stateless marker %q", want)
		}
	}
	// Must NOT contain agentic verifier instructions.
	for _, forbidden := range []string{"worktree", "git -C", "fresh terminal", "Baton verifier"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("system prompt contains agentic token %q — should use stateless prompt, not verifier.md", forbidden)
		}
	}
}
// --- S10: No-mock-boundary tests ---

func TestCheckBoundaryMocks_UndeclaredDbMockFails(t *testing.T) {
	diff := "+func TestSomething(t *testing.T) {\n+	db := &mockDB{}\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared mock, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "db" {
		t.Fatalf("want db boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
	if len(report.DeclaredMocks) != 0 {
		t.Fatalf("want 0 declared mocks, got %d", len(report.DeclaredMocks))
	}
}

func TestCheckBoundaryMocks_DeclaredDbMockPasses(t *testing.T) {
	diff := "+func TestSomething(t *testing.T) {\n+	db := &mockDB{}\n+}"
	deferrals := []string{"db mock for integration tests - S10 boundary"}
	report := CheckBoundaryMocks(diff, deferrals)
	if len(report.UndeclaredMocks) != 0 {
		t.Fatalf("want 0 undeclared mocks, got %d", len(report.UndeclaredMocks))
	}
	if len(report.DeclaredMocks) != 1 {
		t.Fatalf("want 1 declared mock, got %d", len(report.DeclaredMocks))
	}
	if report.DeclaredMocks[0].Boundary != "db" {
		t.Fatalf("want db boundary, got %s", report.DeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_NonBoundaryMockNotFlagged(t *testing.T) {
	diff := "+func TestSomething(t *testing.T) {\n+	calc := newMockCalculator()\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 0 {
		t.Fatalf("want 0 undeclared mocks for non-boundary, got %d: %v", len(report.UndeclaredMocks), report.UndeclaredMocks)
	}
	if len(report.DeclaredMocks) != 0 {
		t.Fatalf("want 0 declared mocks for non-boundary, got %d", len(report.DeclaredMocks))
	}
}

func TestCheckBoundaryMocks_AuthMockUndeclaredFails(t *testing.T) {
	diff := "+func TestAuth(t *testing.T) {\n+	auth := &mockAuthService{}\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared mock for auth boundary, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "auth" {
		t.Fatalf("want auth boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_EntitlementMockUndeclaredFails(t *testing.T) {
	diff := "+func TestPremium(t *testing.T) {\n+	premium := &mockPremiumService{}\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared mock for entitlement boundary, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "entitlement" {
		t.Fatalf("want entitlement boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_FakeDbDetected(t *testing.T) {
	diff := "+func TestSomething(t *testing.T) {\n+	db := fakeDB{}\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 1 {
		t.Fatalf("want 1 undeclared mock for fake DB, got %d", len(report.UndeclaredMocks))
	}
	if report.UndeclaredMocks[0].Boundary != "db" {
		t.Fatalf("want db boundary, got %s", report.UndeclaredMocks[0].Boundary)
	}
}

func TestCheckBoundaryMocks_EmptyDiffReturnsEmpty(t *testing.T) {
	report := CheckBoundaryMocks("", nil)
	if len(report.UndeclaredMocks) != 0 {
		t.Fatalf("want 0 undeclared mocks for empty diff, got %d", len(report.UndeclaredMocks))
	}
	if len(report.DeclaredMocks) != 0 {
		t.Fatalf("want 0 declared mocks for empty diff, got %d", len(report.DeclaredMocks))
	}
}

func TestCheckBoundaryMocks_MultipleBoundaryMocksAllFlagged(t *testing.T) {
	diff := "+func TestBoth(t *testing.T) {\n+	db := &mockDB{}\n+	auth := newMockAuth()\n+}"
	report := CheckBoundaryMocks(diff, nil)
	if len(report.UndeclaredMocks) != 2 {
		t.Fatalf("want 2 undeclared mocks, got %d", len(report.UndeclaredMocks))
	}
	boundaries := make(map[string]bool)
	for _, m := range report.UndeclaredMocks {
		boundaries[m.Boundary] = true
	}
	if !boundaries["db"] {
		t.Fatal("expected db boundary mock")
	}
	if !boundaries["auth"] {
		t.Fatal("expected auth boundary mock")
	}
}

func TestRun_UndeclaredBoundaryMockFailsClosed(t *testing.T) {
	diff := "+func Test(t *testing.T) {\n+	db := &mockDB{}\n+}"
	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", diff),
		Verifier: fakeVerifier{reply: "PASS - would pass if reached", cost: 0.01},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Fail {
		t.Fatalf("want FAIL on undeclared boundary mock, got %s", got.Verdict)
	}
	if got.FailedGate != "boundary_mock" {
		t.Fatalf("want failed_gate=boundary_mock, got %s", got.FailedGate)
	}
	if got.ExitCode() != 1 {
		t.Fatalf("want exit code 1, got %d", got.ExitCode())
	}
}

func TestRun_DeclaredBoundaryMockAllowed(t *testing.T) {
	diff := "+func Test(t *testing.T) {\n+	db := &mockDB{}\n+}"
	in := Input{
		SpecPath:      writeTmp(t, "spec.md", "must do X"),
		DiffPath:      writeTmp(t, "c.diff", diff),
		Verifier:      fakeVerifier{reply: "PASS - all gates green", cost: 0.01},
		OpenDeferrals: []string{"db mock for integration tests - S10 boundary"},
	}
	got := Run(context.Background(), in)
	if got.Verdict != verdict.Pass {
		t.Fatalf("want PASS with declared deferral, got %s", got.Verdict)
	}
}

func TestCheckBoundaryMocks_StubAuthDetected(t *testing.T) {
	diff := "+func TestAuth(t *testing.T) {\n+	authStub := &stubAuth{}\n+}"
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
