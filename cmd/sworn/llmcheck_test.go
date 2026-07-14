package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/config"
)

// isolateModelConfig points config.Load() at a path that does not exist and clears
// the model env vars, so a test asserts against a genuinely unconfigured setup.
//
// Without this the test would read the DEVELOPER'S real ~/.config/sworn/config.json,
// find a verifier model, and then "pass" for the wrong reason — exiting 2 because the
// provider API key is missing rather than because no model is configured. A test whose
// assertion is broader than its claim is not a test, it is a coincidence.
func isolateModelConfig(t *testing.T) {
	t.Helper()
	t.Setenv("SWORN_CONFIG_PATH", filepath.Join(t.TempDir(), "does-not-exist.json"))
	t.Setenv("SWORN_VERIFIER_MODEL", "")
	t.Setenv("SWORN_MODEL", "") // dropped — set here to prove it is not consulted
}

// llmCheckFixture builds a release dir with one slice, so llm-check gets past its
// path resolution and reaches the model-resolution step.
func llmCheckFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sliceDir := filepath.Join(dir, "docs", "release", "test-release", "S01-test")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}
	must := func(p, content string) {
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(dir, "docs", "release", "test-release", "index.md"), "---\ntitle: Test\n---\n")
	must(filepath.Join(sliceDir, "spec.md"), "# Slice\n\n## Acceptance checks\n\n- [ ] THE SYSTEM SHALL work.\n")
	must(filepath.Join(sliceDir, "status.json"), `{"slice_id":"S01-test","state":"implemented"}`)
	return dir
}

// TestLLMCheck_NoModelConfigured — with nothing configured anywhere, llm-check must
// exit 2 (configuration error) rather than proceed.
func TestLLMCheck_NoModelConfigured(t *testing.T) {
	isolateModelConfig(t)
	dir := llmCheckFixture(t)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdLLMCheck([]string{"--type", "ac-satisfaction", "--slice", "S01-test", "--release", "test-release"})
	if exit != 2 {
		t.Errorf("exit = %d, want 2 when no model is configured anywhere", exit)
	}
}

// TestLLMCheck_ResolvesFromConfigFile is the regression guard for the actual defect.
//
// llm-check was the only model-using command that resolved env-only
// (--model > $SWORN_MODEL) and ignored config.json — so a FULLY CONFIGURED setup
// still got "no model configured", and it read a different env var from every
// sibling. Surfaced dogfooding a design-review: the supplementary
// `sworn llm-check -type design-review` could not run despite a configured loop.
//
// A config file with a verifier model must now get PAST model resolution. It will
// still fail later (no API key in the test env), but the point is that it no longer
// fails AT resolution — which is the bug.
func TestLLMCheck_ResolvesFromConfigFile(t *testing.T) {
	isolateModelConfig(t)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1,"verifier":{"model":"openai/gpt-4.1"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWORN_CONFIG_PATH", cfgPath)

	dir := llmCheckFixture(t)
	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	// This must drive cmdLLMCheck, not re-implement its resolution. Asserting on the
	// EXIT CODE alone cannot distinguish "no model configured" (the bug) from "model
	// setup failed, no API key" (expected here) — both exit 2. So assert on what the
	// command SAYS: with a verifier model in config.json it must never claim that no
	// model is configured.
	stderr := captureStderr(t, func() {
		cmdLLMCheck([]string{"--type", "ac-satisfaction", "--slice", "S01-test", "--release", "test-release"})
	})

	if strings.Contains(stderr, "not configured") || strings.Contains(stderr, "no model configured") {
		t.Errorf("llm-check reported the model as unconfigured despite config.json declaring\n"+
			"verifier.model = openai/gpt-4.1 — it is ignoring config.json.\nstderr: %s", stderr)
	}
}

// TestLLMCheck_FlagBeatsConfig pins the precedence llm-check now shares with
// reqverify, verify and the loop: flag > config.json. There is no env layer —
// a per-role env var was a second source of truth, and drift between the two is
// exactly what made llm-check unrunnable on a fully-configured setup.
func TestLLMCheck_FlagBeatsConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1,"verifier":{"model":"from/config"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{name: "flag wins", flag: "from/flag", env: "from/env", want: "from/flag"},
		{name: "config is the source; env is IGNORED", flag: "", env: "from/env", want: "from/config"},
		{name: "config with no env", flag: "", env: "", want: "from/config"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SWORN_VERIFIER_MODEL", tc.env)
			if got := resolvedModelForTest(t, tc.flag, cfgPath); got != tc.want {
				t.Errorf("resolved %q, want %q", got, tc.want)
			}
		})
	}
}

// resolvedModelForTest exercises the SAME resolution path cmdLLMCheck now uses:
// config.Load() (honouring $SWORN_CONFIG_PATH) then config.ResolveVerifierModel.
func resolvedModelForTest(t *testing.T, flag, cfgPath string) string {
	t.Helper()
	t.Setenv("SWORN_CONFIG_PATH", cfgPath)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	got, err := config.ResolveVerifierModel(flag, cfg)
	if err != nil {
		t.Fatalf("ResolveVerifierModel: %v", err)
	}
	return got
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what it wrote.
// Rule 11: os.Stderr is process-global, so the original is restored unconditionally.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	w.Close()
	return <-done
}
