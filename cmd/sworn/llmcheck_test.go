package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

// TestGenericCheckIdentityBinaryReachability drives the built CLI through the
// structured-output boundary. Missing, unknown, and wrong schema-valid generic
// identities must not be relabelled as the requested check or accepted as PASS.
func TestGenericCheckIdentityBinaryReachability(t *testing.T) {
	var structuredCalls atomic.Int32
	responses := []string{
		`{"check":"design-review","verdict":"PASS","findings":[]}`,
		`{"verdict":"PASS","findings":[]}`,
		`{"check":"unknown-check","verdict":"PASS","findings":[]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request struct {
			ResponseFormat json.RawMessage `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode model request: %v", err)
		}
		if len(request.ResponseFormat) == 0 {
			t.Error("generic check did not use schema-constrained response_format")
		}
		call := structuredCalls.Add(1) - 1
		if int(call) >= len(responses) {
			t.Errorf("unexpected structured model call %d", call+1)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": responses[call]}}},
		})
	}))
	defer server.Close()

	root := llmCheckFixture(t)
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version":1,"verifier":{"model":"openai-completions/test-model"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildSworn(t)
	for _, tt := range []struct {
		name          string
		rawCheckMatch string
	}{
		{name: "wrong known identity", rawCheckMatch: `\"check\":\"design-review\"`},
		{name: "missing identity", rawCheckMatch: `\"verdict\":\"PASS\"`},
		{name: "unknown identity", rawCheckMatch: `\"check\":\"unknown-check\"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, "llm-check", "--type", "ac-satisfaction", "--slice", "S01-test", "--release", "test-release", "--base", "HEAD", "--json")
			cmd.Dir = root
			cmd.Env = append(os.Environ(),
				"HOME="+t.TempDir(),
				"SWORN_CONFIG_PATH="+configPath,
				"SWORN_DIRECT=1",
				"OPENAI_API_KEY=test-key",
				"SWORN_OPENAI_COMPLETIONS_BASE_URL="+server.URL,
			)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("invalid emitted check exited 0; want non-zero. output:\n%s", output)
			}
			if got := cmd.ProcessState.ExitCode(); got != 1 {
				t.Fatalf("invalid emitted check exit = %d, want 1. output:\n%s", got, output)
			}
			if !strings.Contains(string(output), tt.rawCheckMatch) {
				t.Fatalf("raw model identity was lost or relabelled: %s", output)
			}
		})
	}
	if structuredCalls.Load() != int32(len(responses)) {
		t.Fatalf("structured output calls = %d, want %d", structuredCalls.Load(), len(responses))
	}
}

// TestSpecAmbiguityTypedReferencesBinaryReachability proves the public command
// reaches the dedicated C-02 resolver and model schema boundary. The handler
// receives exactly the explicit typed artifacts, never an unreferenced canary.
func TestSpecAmbiguityTypedReferencesBinaryReachability(t *testing.T) {
	root := llmCheckFixture(t)
	if output, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	releaseDir := filepath.Join(root, "docs", "release", "test-release")
	write := func(path, contents string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(releaseDir, "S01-test", "spec.json"), `{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "S01-test",
  "release": "test-release",
  "user_outcome": "The planner receives explicit typed artifacts.",
  "covers_needs": ["N-01"],
  "acceptance_criteria": [{"id":"AC-01","text":"THE SYSTEM SHALL resolve typed references.","ears_pattern":"ubiquitous"}],
  "in_scope": [],
  "out_of_scope": [],
  "references": [
    {"kind":"file","path":"docs/reference.txt"},
    {"kind":"contract","contract_id":"C-01"},
    {"kind":"slice","slice_id":"S02-sibling"}
  ]
}
`)
	write(filepath.Join(root, "docs", "reference.txt"), "explicit file reference\n")
	write(filepath.Join(root, "private-canary.txt"), "MUST-NOT-LEAK")
	write(filepath.Join(releaseDir, "contracts.json"), `{
  "$schema":"https://baton.sawy3r.net/schemas/contracts-v1.json",
  "release":"test-release",
  "contracts":[{"id":"C-01","kind":"schema-version","surface":"fixture","shape":"fixture","owner":"S01-test"}]
}
`)
	write(filepath.Join(releaseDir, "S02-sibling", "spec.json"), `{
  "$schema":"https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id":"S02-sibling",
  "release":"test-release",
  "user_outcome":"A sibling slice is explicit evidence.",
  "covers_needs":["N-01"],
  "acceptance_criteria":[{"id":"AC-01","text":"THE SYSTEM SHALL be a valid sibling.","ears_pattern":"ubiquitous"}],
  "in_scope":[],
  "out_of_scope":[],
  "references":[]
}
`)

	var mu sync.Mutex
	var receivedPayload string
	var structuredCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat json.RawMessage `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode model request: %v", err)
		}
		if len(request.ResponseFormat) == 0 || len(request.Messages) != 2 {
			t.Errorf("dedicated check did not use the two-message structured boundary: %+v", request)
		}
		structuredCalls.Add(1)
		if len(request.Messages) >= 2 {
			mu.Lock()
			receivedPayload = request.Messages[1].Content
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": `{"$schema":"https://baton.sawy3r.net/schemas/spec-ambiguity-report-v1.json","schema_version":1,"check":"spec-ambiguity","slice_id":"S01-test","release":"test-release","verdict":"PASS","blocking_findings":{},"advisory_findings":{}}`}}},
		})
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version":1,"verifier":{"model":"openai-completions/test-model"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(buildSworn(t), "llm-check", "--type", "spec-ambiguity", "--slice", "S01-test", "--release", "test-release", "--base", "HEAD", "--json")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"SWORN_CONFIG_PATH="+configPath,
		"SWORN_DIRECT=1",
		"OPENAI_API_KEY=test-key",
		"SWORN_OPENAI_COMPLETIONS_BASE_URL="+server.URL,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("spec-ambiguity command failed: %v\n%s", err, output)
	}
	if structuredCalls.Load() != 1 {
		t.Fatalf("structured output calls = %d, want 1", structuredCalls.Load())
	}
	if !strings.Contains(string(output), "blocking_findings") || strings.Contains(string(output), `"findings"`) {
		t.Fatalf("public output did not preserve the dedicated ambiguity report: %s", output)
	}
	mu.Lock()
	payload := receivedPayload
	mu.Unlock()
	for _, want := range []string{
		"--- ARTIFACT docs/reference.txt ---\nexplicit file reference\n",
		"--- ARTIFACT docs/release/test-release/S02-sibling/spec.json ---",
		"--- ARTIFACT docs/release/test-release/contracts.json ---",
	} {
		if !strings.Contains(payload, want) {
			t.Fatalf("model payload missing %q:\n%s", want, payload)
		}
	}
	if strings.Contains(payload, "MUST-NOT-LEAK") || strings.Contains(payload, "private-canary") {
		t.Fatalf("model payload leaked an unreferenced canary:\n%s", payload)
	}
}

func TestGenericMaintainabilityReviewRetiredWithoutDispatch(t *testing.T) {
	root := llmCheckFixture(t)
	before := fixtureTreeSnapshot(t, root)
	var modelCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cmd := exec.Command(buildSworn(t), "llm-check", "--type", "maintainability-review", "--slice", "S01-test", "--release", "test-release", "--model", "openai-completions/test-model", "--base", "definitely-not-a-ref")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"SWORN_CONFIG_PATH="+filepath.Join(t.TempDir(), "missing-config.json"),
		"SWORN_DIRECT=1",
		"OPENAI_API_KEY=test-key",
		"SWORN_OPENAI_COMPLETIONS_BASE_URL="+server.URL,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("retired maintainability command exited 0: %s", output)
	}
	if got := cmd.ProcessState.ExitCode(); got != 64 {
		t.Fatalf("retired maintainability exit = %d, want 64. output:\n%s", got, output)
	}
	if !strings.Contains(string(output), "use sworn maintainability review") {
		t.Fatalf("retired maintainability guidance missing: %s", output)
	}
	if modelCalls.Load() != 0 {
		t.Fatalf("retired maintainability dispatched %d model calls, want 0", modelCalls.Load())
	}
	if after := fixtureTreeSnapshot(t, root); after != before {
		t.Fatalf("retired maintainability mutated the fixture tree\nbefore: %q\nafter:  %q", before, after)
	}
}

func fixtureTreeSnapshot(t *testing.T, root string) string {
	t.Helper()
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel)+"\x00"+string(contents))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return strings.Join(files, "\n")
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
