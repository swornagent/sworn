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

// TestRun_SystemPromptIsStateless validates that verify.Run passes the
// stateless judge prompt (VerifyStateless) to the model, NOT the agentic
// verifier role prompt (Verifier).
func TestRun_SystemPromptIsStateless(t *testing.T) {
	cv := &capturingVerifier{reply: "PASS - looks good", cost: 0.01}
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