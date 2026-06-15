package verify

import (
	"context"
	"os"
	"path/filepath"
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

func TestRun_GarbledVerdictBlocks(t *testing.T) {	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "looks good to me!"}, // no PASS/FAIL/BLOCKED prefix
	}
	if got := Run(context.Background(), in); got.Verdict != verdict.Blocked {
		t.Fatalf("want BLOCKED on unparseable, got %s", got.Verdict)
	}
}
