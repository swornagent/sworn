package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/swornagent/sworn/internal/baton/schemas"
	"github.com/swornagent/sworn/internal/config"
)

// @mock-boundary
// Deterministic local model endpoints are intentional test transport only; no
// credentialed provider request is made by these reachability tests.

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
		name      string
		safeMatch string
	}{
		{name: "wrong known identity", safeMatch: `LLM check response identity mismatch`},
		{name: "missing identity", safeMatch: `LLM check response violates llm-check-report-v1`},
		{name: "unknown identity", safeMatch: `LLM check response violates llm-check-report-v1`},
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
			if !strings.Contains(string(output), tt.safeMatch) {
				t.Fatalf("safe contract diagnostic missing: %s", output)
			}
			if strings.Contains(string(output), `\"check\":`) || strings.Contains(string(output), `\"verdict\":\"PASS\"`) {
				t.Fatalf("raw model payload leaked into public output: %s", output)
			}
		})
	}
	if structuredCalls.Load() != int32(len(responses)) {
		t.Fatalf("structured output calls = %d, want %d", structuredCalls.Load(), len(responses))
	}
}

// TestLLMCheckOpenAIResponsesStructuredEnvelopeBinaryReachability drives the
// built public binary through the OpenAI Responses structured-output boundary.
// The child environment is deliberately scrubbed: a future loss of the fake
// endpoint override cannot fall through to a developer credential, proxy, or
// live provider.
func TestLLMCheckOpenAIResponsesStructuredEnvelopeBinaryReachability(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/responses" {
			t.Errorf("Responses path = %q, want /responses", r.URL.Path)
		}
		var request struct {
			Instructions string `json:"instructions"`
			Input        []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"input"`
			Text *struct {
				Format *struct {
					Type   string          `json:"type"`
					Name   string          `json:"name"`
					Schema json.RawMessage `json:"schema"`
					Strict bool            `json:"strict"`
				} `json:"format"`
			} `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Responses request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Instructions == "" || len(request.Input) != 1 || request.Input[0].Role != "user" || request.Input[0].Content == "" {
			t.Errorf("Responses prompt/payload was not preserved: %+v", request)
		}
		if request.Text == nil || request.Text.Format == nil {
			t.Error("Responses request omitted text.format")
		} else {
			if request.Text.Format.Type != "json_schema" || !request.Text.Format.Strict {
				t.Errorf("Responses text.format = %+v, want strict json_schema", request.Text.Format)
			}
			if request.Text.Format.Name != "llm-check-report-v1-openai-envelope" {
				t.Errorf("Responses schema name = %q, want llm-check-report-v1-openai-envelope", request.Text.Format.Name)
			}
			assertLLMCheckOpenAIEnvelope(t, request.Text.Format.Schema)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []any{map[string]any{
				"type": "message",
				"content": []any{map[string]any{
					"type": "output_text",
					"text": `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`,
				}},
			}},
		})
	}))
	defer server.Close()

	root := llmCheckRepoFixture(t)
	configPath := llmCheckConfig(t, "openai/test-model")
	cmd := exec.Command(buildSworn(t), "llm-check", "--type", "ac-satisfaction", "--slice", "S01-test", "--release", "test-release", "--base", "HEAD", "--json")
	cmd.Dir = root
	cmd.Env = hermeticLLMCheckEnv(t, configPath, "SWORN_OPENAI_BASE_URL", server.URL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Responses llm-check failed: %v\n%s", err, output)
	}
	if got := cmd.ProcessState.ExitCode(); got != 0 {
		t.Fatalf("Responses llm-check exit = %d, want 0. output:\n%s", got, output)
	}
	if hits.Load() != 1 {
		t.Fatalf("Responses endpoint hits = %d, want 1", hits.Load())
	}
}

// TestLLMCheckOpenAICompletionsStructuredEnvelopeBinaryReachability drives the
// same public generic check through the legacy chat/completions wire format.
func TestLLMCheckOpenAICompletionsStructuredEnvelopeBinaryReachability(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/chat/completions" {
			t.Errorf("completions path = %q, want /chat/completions", r.URL.Path)
		}
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat *struct {
				Type       string `json:"type"`
				JSONSchema *struct {
					Name   string          `json:"name"`
					Schema json.RawMessage `json:"schema"`
					Strict bool            `json:"strict"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode completions request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[0].Content == "" || request.Messages[1].Role != "user" || request.Messages[1].Content == "" {
			t.Errorf("completions prompt/payload was not preserved: %+v", request.Messages)
		}
		if request.ResponseFormat == nil || request.ResponseFormat.JSONSchema == nil {
			t.Error("completions request omitted response_format.json_schema")
		} else {
			if request.ResponseFormat.Type != "json_schema" || !request.ResponseFormat.JSONSchema.Strict {
				t.Errorf("completions response_format = %+v, want strict json_schema", request.ResponseFormat)
			}
			if request.ResponseFormat.JSONSchema.Name != "llm-check-report-v1-openai-envelope" {
				t.Errorf("completions schema name = %q, want llm-check-report-v1-openai-envelope", request.ResponseFormat.JSONSchema.Name)
			}
			assertLLMCheckOpenAIEnvelope(t, request.ResponseFormat.JSONSchema.Schema)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`},
			}},
		})
	}))
	defer server.Close()

	root := llmCheckRepoFixture(t)
	configPath := llmCheckConfig(t, "openai-completions/test-model")
	cmd := exec.Command(buildSworn(t), "llm-check", "--type", "ac-satisfaction", "--slice", "S01-test", "--release", "test-release", "--base", "HEAD", "--json")
	cmd.Dir = root
	cmd.Env = hermeticLLMCheckEnv(t, configPath, "SWORN_OPENAI_COMPLETIONS_BASE_URL", server.URL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("completions llm-check failed: %v\n%s", err, output)
	}
	if got := cmd.ProcessState.ExitCode(); got != 0 {
		t.Fatalf("completions llm-check exit = %d, want 0. output:\n%s", got, output)
	}
	if hits.Load() != 1 {
		t.Fatalf("completions endpoint hits = %d, want 1", hits.Load())
	}
}

// TestLLMCheckOpenRouterToolStructuredBinaryReachability drives the built
// command through the direct-only OpenRouter forced-tool route. The endpoint
// and key are synthetic so this asserts public wiring without any provider
// dispatch or inherited login state.
func TestLLMCheckOpenRouterToolStructuredBinaryReachability(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/chat/completions" {
			t.Errorf("OpenRouter path = %q, want /chat/completions", r.URL.Path)
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Type     string `json:"type"`
				Function struct {
					Name       string          `json:"name"`
					Parameters json.RawMessage `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
			ToolChoice struct {
				Type     string `json:"type"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tool_choice"`
			ResponseFormat json.RawMessage `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode OpenRouter request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Model != "z-ai/glm-5.2" {
			t.Errorf("OpenRouter model = %q, want z-ai/glm-5.2", request.Model)
		}
		if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[0].Content == "" || request.Messages[1].Role != "user" || request.Messages[1].Content == "" {
			t.Errorf("OpenRouter prompt/payload was not preserved: %+v", request.Messages)
		}
		if len(request.Tools) != 1 || request.Tools[0].Type != "function" || request.Tools[0].Function.Name != "emit_structured_output" {
			t.Errorf("OpenRouter tools = %+v, want one forced emit_structured_output function", request.Tools)
		} else {
			// JSON transport compacts insignificant whitespace while embedding the
			// raw schema value; compare the canonical JSON representation to prove
			// no envelope, projection, or source rewrite reached parameters.
			canonical, err := json.Marshal(json.RawMessage(schemas.LLMCheckReportV1))
			if err != nil {
				t.Fatalf("marshal canonical report as wire JSON: %v", err)
			}
			if !bytes.Equal(request.Tools[0].Function.Parameters, canonical) {
				t.Error("OpenRouter tool parameters did not preserve the canonical report")
			}
		}
		if request.ToolChoice.Type != "function" || request.ToolChoice.Function.Name != "emit_structured_output" {
			t.Errorf("OpenRouter tool_choice = %+v, want forced emit_structured_output", request.ToolChoice)
		}
		if len(request.ResponseFormat) != 0 {
			t.Errorf("OpenRouter request unexpectedly sent response_format: %s", request.ResponseFormat)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"content": "",
					"tool_calls": []any{map[string]any{
						"id":   "call-1",
						"type": "function",
						"function": map[string]any{
							"name":      "emit_structured_output",
							"arguments": `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`,
						},
					}},
				},
			}},
		})
	}))
	defer server.Close()

	root := llmCheckRepoFixture(t)
	configPath := llmCheckConfig(t, "ignored/by-flag")
	cmd := exec.Command(buildSworn(t), "llm-check", "--type", "ac-satisfaction", "--slice", "S01-test", "--release", "test-release", "--model", "openrouter/z-ai/glm-5.2", "--base", "HEAD", "--json")
	cmd.Dir = root
	cmd.Env = append(
		hermeticLLMCheckEnv(t, configPath, "SWORN_OPENROUTER_BASE_URL", server.URL),
		"OPENROUTER_API_KEY=synthetic-openrouter-key",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("OpenRouter llm-check failed: %v", err)
	}
	if got := cmd.ProcessState.ExitCode(); got != 0 {
		t.Fatalf("OpenRouter llm-check exit = %d, want 0", got)
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatal("OpenRouter llm-check did not print a PASS report")
	}
	if hits.Load() != 1 {
		t.Fatalf("OpenRouter endpoint hits = %d, want 1", hits.Load())
	}
}

func TestLLMCheckOpenRouterToolStructuredBinaryRejectsInvalidResponse(t *testing.T) {
	root := llmCheckRepoFixture(t)
	configPath := llmCheckConfig(t, "ignored/by-flag")
	var response map[string]any
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	toolCall := func(name, arguments string) any {
		return map[string]any{
			"id":   "call-1",
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": arguments,
			},
		}
	}
	withCalls := func(calls []any) map[string]any {
		return map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": "", "tool_calls": calls},
			}},
		}
	}

	for _, tt := range []struct {
		name      string
		toolCalls []any
	}{
		{name: "missing tool call"},
		{name: "multiple tool calls", toolCalls: []any{toolCall("emit_structured_output", `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`), toolCall("emit_structured_output", `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`)}},
		{name: "wrong tool name", toolCalls: []any{toolCall("other_function", `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`)}},
		{name: "non-object arguments", toolCalls: []any{toolCall("emit_structured_output", `[]`)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hits.Store(0)
			response = withCalls(tt.toolCalls)
			cmd := exec.Command(buildSworn(t), "llm-check", "--type", "ac-satisfaction", "--slice", "S01-test", "--release", "test-release", "--model", "openrouter/z-ai/glm-5.2", "--base", "HEAD", "--json")
			cmd.Dir = root
			cmd.Env = append(
				hermeticLLMCheckEnv(t, configPath, "SWORN_OPENROUTER_BASE_URL", server.URL),
				"OPENROUTER_API_KEY=synthetic-openrouter-key",
			)
			if _, err := cmd.CombinedOutput(); err == nil {
				t.Fatal("invalid OpenRouter tool response exited 0")
			}
			if hits.Load() != 1 {
				t.Fatalf("OpenRouter endpoint hits = %d, want 1 without retry or fallback", hits.Load())
			}
		})
	}
}

// TestLLMCheckOpenAIEnvelopeBinaryRejectsInvalidCanonicalResponse proves a
// provider-accepted envelope is not semantic acceptance. The built binary must
// retain gate's canonical allOf validation and requested/emitted check equality.
func TestLLMCheckOpenAIEnvelopeBinaryRejectsInvalidCanonicalResponse(t *testing.T) {
	var hits atomic.Int32
	var response string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/responses" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []any{map[string]any{
					"type": "message",
					"content": []any{map[string]any{
						"type": "output_text",
						"text": response,
					}},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": response},
			}},
		})
	}))
	defer server.Close()

	root := llmCheckRepoFixture(t)
	binary := buildSworn(t)
	for _, route := range []struct {
		name       string
		modelID    string
		baseURLKey string
	}{
		{name: "Responses", modelID: "openai/test-model", baseURLKey: "SWORN_OPENAI_BASE_URL"},
		{name: "chat completions", modelID: "openai-completions/test-model", baseURLKey: "SWORN_OPENAI_COMPLETIONS_BASE_URL"},
	} {
		for _, invalid := range []struct {
			name string
			raw  string
		}{
			{name: "PASS with blocking finding", raw: `{"check":"ac-satisfaction","verdict":"PASS","findings":[{"id":"F-01","severity":"high","blocking":true,"title":"blocked","detail":"must fail locally"}]}`},
			{name: "FAIL without blocking finding", raw: `{"check":"ac-satisfaction","verdict":"FAIL","findings":[{"id":"F-01","severity":"low","blocking":false,"title":"not blocked","detail":"must fail locally"}]}`},
			{name: "missing check", raw: `{"verdict":"PASS","findings":[]}`},
			{name: "different check", raw: `{"check":"design-review","verdict":"PASS","findings":[]}`},
		} {
			t.Run(route.name+"/"+invalid.name, func(t *testing.T) {
				hits.Store(0)
				response = invalid.raw
				configPath := llmCheckConfig(t, route.modelID)
				cmd := exec.Command(binary, "llm-check", "--type", "ac-satisfaction", "--slice", "S01-test", "--release", "test-release", "--base", "HEAD", "--json")
				cmd.Dir = root
				cmd.Env = hermeticLLMCheckEnv(t, configPath, route.baseURLKey, server.URL)
				output, err := cmd.CombinedOutput()
				if err == nil {
					t.Fatalf("invalid canonical response exited 0: %s", output)
				}
				if got := cmd.ProcessState.ExitCode(); got != 1 {
					t.Fatalf("invalid canonical response exit = %d, want 1. output:\n%s", got, output)
				}
				if hits.Load() != 1 {
					t.Fatalf("endpoint hits = %d, want 1", hits.Load())
				}
			})
		}
	}
}

// TestLLMCheckProofReceiptBinaryReachability is the public reachability gate
// for S22. It drives the built binary through the direct forced-tool wire,
// records only the strict metadata receipt, and proves neither the synthetic
// credential, endpoint, nor model response reaches public output.
func TestLLMCheckProofReceiptBinaryReachability(t *testing.T) {
	testLLMCheckProofReceiptBinaryReachability(t)
}

func TestLLMCheckProofReceiptLeakCanaries(t *testing.T) {
	testLLMCheckProofReceiptBinaryReachability(t)
}

func testLLMCheckProofReceiptBinaryReachability(t *testing.T) {
	const (
		release = "2026-07-15-baton-v0.16-conformance"
		slice   = "S22-openrouter-tool-structured-output"
		start   = "a09b0e46df465862d00469d4aef2a997442b3d5b"
		modelID = "openrouter/z-ai/glm-5.2"
	)

	root := llmCheckRepoFixture(t)
	releaseDir := filepath.Join(root, "docs", "release", release)
	sliceDir := filepath.Join(releaseDir, slice)
	write := func(path, contents string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(releaseDir, "index.md"), "# S22 fixture\n")
	write(filepath.Join(sliceDir, "spec.json"), `{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "S22-openrouter-tool-structured-output",
  "release": "2026-07-15-baton-v0.16-conformance",
  "user_outcome": "A native receipt records one bounded proof.",
  "covers_needs": ["N-10"],
  "acceptance_criteria": [{"id":"AC-01","text":"WHEN the fixture runs THE SYSTEM SHALL retain only a receipt.","ears_pattern":"event-driven"}],
  "in_scope": [],
  "out_of_scope": [],
  "references": []
}
`)
	write(filepath.Join(sliceDir, "status.json"), proofReceiptS22StatusFixture())
	write(filepath.Join(releaseDir, "S21-openai-structured-envelope", "status.json"), proofReceiptS21StatusFixture())
	write(filepath.Join(sliceDir, "receipts", "attempt-1.json"), `{
  "$schema": "https://swornagent.dev/schemas/llm-check-proof-receipt-v1.json",
  "record_version": 1,
  "release": "2026-07-15-baton-v0.16-conformance",
  "slice_id": "S22-openrouter-tool-structured-output",
  "check_type": "spec-ambiguity",
  "model_id": "openrouter/z-ai/glm-5.2",
  "immutable_start_commit": "a09b0e46df465862d00469d4aef2a997442b3d5b",
  "attempt": 1,
  "attempt_class": "receipt_failure",
  "result": "UNPARSEABLE",
  "process_exit_code": "unavailable"
}
`)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		calls.Add(1)
		var request struct {
			Tools []struct {
				Type     string `json:"type"`
				Function struct {
					Name       string          `json:"name"`
					Parameters json.RawMessage `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
			ToolChoice     json.RawMessage `json:"tool_choice"`
			ResponseFormat json.RawMessage `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error("proof receipt request was not JSON")
			return
		}
		if len(request.Tools) != 1 || request.Tools[0].Type != "function" || request.Tools[0].Function.Name != "emit_structured_output" || len(request.Tools[0].Function.Parameters) == 0 || len(request.ToolChoice) == 0 || len(request.ResponseFormat) != 0 {
			t.Error("proof receipt did not use the direct forced-tool contract")
			return
		}
		arguments := `{"$schema":"https://baton.sawy3r.net/schemas/spec-ambiguity-report-v1.json","schema_version":1,"check":"spec-ambiguity","slice_id":"S22-openrouter-tool-structured-output","release":"2026-07-15-baton-v0.16-conformance","verdict":"PASS","blocking_findings":{},"advisory_findings":{"s22.advisory":{"id":"F-01","severity":"info","title":"advisory","detail":"S22-RESPONSE-CANARY","criterion_id":"AC-01","ambiguity_kind":"vague-language","observable_divergence":"fixture evidence","contract_surface":"verification-evidence","semantic_subject":"fixture","suggested_resolution":"none"}}}`
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{
				"id": "call-1", "type": "function", "function": map[string]any{"name": "emit_structured_output", "arguments": arguments},
			}}}}},
		})
	}))
	defer server.Close()

	configPath := llmCheckConfig(t, modelID)
	cmd := exec.Command(buildSworn(t), "llm-check", "--proof-receipt", "--type", "spec-ambiguity", "--slice", slice, "--release", release, "--model", modelID, "--base", start)
	cmd.Dir = root
	cmd.Env = append(hermeticLLMCheckEnv(t, configPath, "SWORN_OPENROUTER_BASE_URL", server.URL), "OPENROUTER_API_KEY=S22-SYNTHETIC-KEY-CANARY")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("proof receipt command failed with exit %d", cmd.ProcessState.ExitCode())
	}
	if calls.Load() != 1 {
		t.Fatalf("proof receipt requests = %d, want 1", calls.Load())
	}
	for _, forbidden := range []string{"S22-SYNTHETIC-KEY-CANARY", "S22-RESPONSE-CANARY", server.URL} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("public proof receipt output leaked protected data")
		}
	}
	receiptBytes, err := os.ReadFile(filepath.Join(sliceDir, "receipts", "attempt-2.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"S22-SYNTHETIC-KEY-CANARY", "S22-RESPONSE-CANARY", server.URL} {
		if bytes.Contains(receiptBytes, []byte(forbidden)) {
			t.Fatalf("proof receipt leaked protected data")
		}
	}
	var receipt struct {
		Attempt int    `json:"attempt"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Attempt != 2 || receipt.Result != "PASS" {
		t.Fatalf("receipt identity = attempt %d result %q, want attempt 2 PASS", receipt.Attempt, receipt.Result)
	}
}

func TestLLMCheckProofReceiptPreflightRequiresFreshS21VerificationEvidence(t *testing.T) {
	const (
		release = "2026-07-15-baton-v0.16-conformance"
		slice   = "S22-openrouter-tool-structured-output"
		start   = "a09b0e46df465862d00469d4aef2a997442b3d5b"
		modelID = "openrouter/z-ai/glm-5.2"
	)

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()

	for _, tt := range []struct {
		name        string
		prepare     func(t *testing.T, root, sliceDir string)
		base        string
		releaseArg  string
		wantAttempt bool
	}{
		{
			name:    "missing historical attempt one",
			prepare: func(*testing.T, string, string) {},
		},
		{
			name: "mismatched historical model",
			prepare: func(t *testing.T, _ string, sliceDir string) {
				writeProofReceiptFixture(t, filepath.Join(sliceDir, "receipts", "attempt-1.json"), "openrouter/other", 1, "receipt_failure", "UNPARSEABLE", `"unavailable"`)
			},
		},
		{
			name: "historical v0.15 receipt binding",
			prepare: func(t *testing.T, _ string, sliceDir string) {
				path := filepath.Join(sliceDir, "receipts", "attempt-1.json")
				writeProofReceiptFixture(t, path, modelID, 1, "receipt_failure", "UNPARSEABLE", `"unavailable"`)
				replaceProofReceiptFixture(t, path, "2026-07-15-baton-v0.16-conformance", "2026-07-15-baton-v0.15-conformance")
			},
		},
		{
			name: "exhausted second attempt",
			prepare: func(t *testing.T, _ string, sliceDir string) {
				writeProofReceiptFixture(t, filepath.Join(sliceDir, "receipts", "attempt-1.json"), modelID, 1, "receipt_failure", "UNPARSEABLE", `"unavailable"`)
				writeProofReceiptFixture(t, filepath.Join(sliceDir, "receipts", "attempt-2.json"), modelID, 2, "final_verdict", "PASS", "0")
			},
			wantAttempt: true,
		},
		{
			name: "upstream S21 no longer verified",
			prepare: func(t *testing.T, root, _ string) {
				if err := os.WriteFile(filepath.Join(root, "docs", "release", release, "S21-openai-structured-envelope", "status.json"), []byte(`{"state":"planned","verification":{"result":"pending"}}`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "upstream S21 release mismatch",
			prepare: func(t *testing.T, root, _ string) {
				path := filepath.Join(root, "docs", "release", release, "S21-openai-structured-envelope", "status.json")
				replaceProofReceiptFixture(t, path, release, "2026-07-15-baton-v0.15-conformance")
			},
		},
		{
			name: "upstream S21 start mismatch",
			prepare: func(t *testing.T, root, _ string) {
				path := filepath.Join(root, "docs", "release", release, "S21-openai-structured-envelope", "status.json")
				replaceProofReceiptFixture(t, path, "ed0badf68673f0af84834458f07be0792555484f", strings.Repeat("b", 40))
			},
		},
		{
			name: "upstream S21 verdict time missing",
			prepare: func(t *testing.T, root, _ string) {
				path := filepath.Join(root, "docs", "release", release, "S21-openai-structured-envelope", "status.json")
				replaceProofReceiptFixture(t, path, "2026-07-17T09:32:45+10:00", "")
			},
		},
		{
			name: "upstream S21 not fresh context",
			prepare: func(t *testing.T, root, _ string) {
				path := filepath.Join(root, "docs", "release", release, "S21-openai-structured-envelope", "status.json")
				replaceProofReceiptFixture(t, path, `"verifier_was_fresh_context":true`, `"verifier_was_fresh_context":false`)
			},
		},
		{
			name: "S22 authoritative status reference mismatch",
			prepare: func(t *testing.T, _ string, sliceDir string) {
				path := filepath.Join(sliceDir, "status.json")
				replaceProofReceiptFixture(t, path, "240a2ede9a5fd022ae403ced30a6a5f80d918747", strings.Repeat("c", 40))
			},
		},
		{
			name:       "v0.15 invocation release",
			prepare:    func(*testing.T, string, string) {},
			releaseArg: "2026-07-15-baton-v0.15-conformance",
		},
		{
			name: "mismatched explicit base",
			prepare: func(t *testing.T, _ string, sliceDir string) {
				writeProofReceiptFixture(t, filepath.Join(sliceDir, "receipts", "attempt-1.json"), modelID, 1, "receipt_failure", "UNPARSEABLE", `"unavailable"`)
			},
			base: strings.Repeat("b", 40),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hits.Store(0)
			root, sliceDir := proofReceiptCommandFixture(t)
			tt.prepare(t, root, sliceDir)
			oldCwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(oldCwd)
			if err := os.Chdir(root); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SWORN_DIRECT", "1")
			t.Setenv("OPENROUTER_API_KEY", "S22-SYNTHETIC-KEY-CANARY")
			t.Setenv("SWORN_OPENROUTER_BASE_URL", server.URL)
			base := start
			if tt.base != "" {
				base = tt.base
			}
			releaseValue := release
			if tt.releaseArg != "" {
				releaseValue = tt.releaseArg
			}
			if got := cmdLLMCheck([]string{"--proof-receipt", "--type", "spec-ambiguity", "--slice", slice, "--release", releaseValue, "--model", modelID, "--base", base}); got != 2 {
				t.Fatalf("preflight exit = %d, want 2", got)
			}
			if hits.Load() != 0 {
				t.Fatalf("preflight dispatched %d provider requests", hits.Load())
			}
			_, err = os.Stat(filepath.Join(sliceDir, "receipts", "attempt-2.json"))
			if tt.wantAttempt {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatal("preflight created or rewrote a second attempt")
			}
		})
	}
}

func TestLLMCheckProofReceiptPreflightWriteFailureHasZeroDispatch(t *testing.T) {
	root, sliceDir := proofReceiptCommandFixture(t)
	receiptPath := filepath.Join(sliceDir, "receipts", "attempt-1.json")
	writeProofReceiptFixture(t, receiptPath, s22ProofReceiptModel, 1, "receipt_failure", "UNPARSEABLE", `"unavailable"`)
	receiptDir := filepath.Dir(receiptPath)
	if err := os.Chmod(receiptDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(receiptDir, 0o700) })

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer server.Close()
	if got := runProofReceiptCommandFixture(t, root, server.URL); got != 2 {
		t.Fatalf("preflight write failure exit = %d, want 2", got)
	}
	if hits.Load() != 0 {
		t.Fatalf("preflight write failure dispatched %d provider requests", hits.Load())
	}
}

func TestLLMCheckProofReceiptMismatchedBindingHasZeroDispatch(t *testing.T) {
	root, sliceDir := proofReceiptCommandFixture(t)
	receiptPath := filepath.Join(sliceDir, "receipts", "attempt-1.json")
	writeProofReceiptFixture(t, receiptPath, s22ProofReceiptModel, 1, "receipt_failure", "UNPARSEABLE", `"unavailable"`)
	replaceProofReceiptFixture(t, receiptPath, s22ProofReceiptRelease, "2026-07-15-baton-v0.15-conformance")

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer server.Close()
	if got := runProofReceiptCommandFixture(t, root, server.URL); got != 2 {
		t.Fatalf("mismatched binding exit = %d, want 2", got)
	}
	if hits.Load() != 0 {
		t.Fatalf("mismatched binding dispatched %d provider requests", hits.Load())
	}
}

func TestLLMCheckProofReceiptStopsAfterTwoAttempts(t *testing.T) {
	root, sliceDir := proofReceiptCommandFixture(t)
	writeProofReceiptFixture(t, filepath.Join(sliceDir, "receipts", "attempt-1.json"), s22ProofReceiptModel, 1, "receipt_failure", "UNPARSEABLE", `"unavailable"`)
	writeProofReceiptFixture(t, filepath.Join(sliceDir, "receipts", "attempt-2.json"), s22ProofReceiptModel, 2, "upstream", "UNPARSEABLE", "2")

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer server.Close()
	if got := runProofReceiptCommandFixture(t, root, server.URL); got != 2 {
		t.Fatalf("exhausted attempt budget exit = %d, want 2", got)
	}
	if hits.Load() != 0 {
		t.Fatalf("exhausted two-attempt budget dispatched %d third requests", hits.Load())
	}
}

func TestLLMCheckProofReceiptTerminalFailuresUseOneDispatch(t *testing.T) {
	root, sliceDir := proofReceiptCommandFixture(t)
	writeProofReceiptFixture(t, filepath.Join(sliceDir, "receipts", "attempt-1.json"), s22ProofReceiptModel, 1, "receipt_failure", "UNPARSEABLE", `"unavailable"`)

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"S22-TERMINAL-BODY-CANARY"}}`))
	}))
	defer server.Close()
	if got := runProofReceiptCommandFixture(t, root, server.URL); got != 2 {
		t.Fatalf("terminal provider failure exit = %d, want 2", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("terminal provider failure dispatched %d requests, want 1", hits.Load())
	}
	if got := runProofReceiptCommandFixture(t, root, server.URL); got != 2 {
		t.Fatalf("repeated terminal invocation exit = %d, want 2", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("terminal receipt allowed retry; provider requests = %d", hits.Load())
	}
}

func runProofReceiptCommandFixture(t *testing.T, root, baseURL string) int {
	t.Helper()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldCwd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWORN_CONFIG_PATH", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("SWORN_DIRECT", "1")
	t.Setenv("OPENROUTER_API_KEY", "S22-SYNTHETIC-KEY-CANARY")
	t.Setenv("SWORN_OPENROUTER_BASE_URL", baseURL)
	return cmdLLMCheck([]string{
		"--proof-receipt",
		"--type", "spec-ambiguity",
		"--slice", s22ProofReceiptSlice,
		"--release", s22ProofReceiptRelease,
		"--model", s22ProofReceiptModel,
		"--base", s22ProofReceiptStart,
	})
}

func proofReceiptCommandFixture(t *testing.T) (string, string) {
	t.Helper()
	const (
		release = "2026-07-15-baton-v0.16-conformance"
		slice   = "S22-openrouter-tool-structured-output"
		start   = "a09b0e46df465862d00469d4aef2a997442b3d5b"
	)
	root := llmCheckRepoFixture(t)
	releaseDir := filepath.Join(root, "docs", "release", release)
	sliceDir := filepath.Join(releaseDir, slice)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(releaseDir, "index.md"), "# fixture\n")
	write(filepath.Join(sliceDir, "spec.json"), `{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "S22-openrouter-tool-structured-output",
  "release": "2026-07-15-baton-v0.16-conformance",
  "user_outcome": "A native receipt records one bounded proof.",
  "covers_needs": ["N-10"],
  "acceptance_criteria": [{"id":"AC-01","text":"WHEN the fixture runs THE SYSTEM SHALL retain only a receipt.","ears_pattern":"event-driven"}],
  "in_scope": [],
  "out_of_scope": [],
  "references": []
}
`)
	write(filepath.Join(sliceDir, "status.json"), proofReceiptS22StatusFixture())
	write(filepath.Join(releaseDir, "S21-openai-structured-envelope", "status.json"), proofReceiptS21StatusFixture())
	if err := os.MkdirAll(filepath.Join(sliceDir, "receipts"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = start
	return root, sliceDir
}

func proofReceiptS22StatusFixture() string {
	return `{
  "slice_id":"S22-openrouter-tool-structured-output",
  "release":"2026-07-15-baton-v0.16-conformance",
  "start_commit":"a09b0e46df465862d00469d4aef2a997442b3d5b",
  "upstream_gate":{
    "slice_id":"S21-openai-structured-envelope",
    "required_state":"verified",
    "required_verification_result":"pass",
    "authoritative_track_status_commit":"240a2ede9a5fd022ae403ced30a6a5f80d918747",
    "immutable_start_commit":"ed0badf68673f0af84834458f07be0792555484f",
    "verifier_verdict_at":"2026-07-17T09:32:45+10:00"
  }
}`
}

func proofReceiptS21StatusFixture() string {
	return `{
  "slice_id":"S21-openai-structured-envelope",
  "release":"2026-07-15-baton-v0.16-conformance",
  "state":"verified",
  "start_commit":"ed0badf68673f0af84834458f07be0792555484f",
  "verification":{
    "result":"pass",
    "verifier_verdict_at":"2026-07-17T09:32:45+10:00",
    "verifier_was_fresh_context":true
  }
}`
}

func writeProofReceiptFixture(t *testing.T, path, model string, attempt int, class, result, exit string) {
	t.Helper()
	body := `{"$schema":"https://swornagent.dev/schemas/llm-check-proof-receipt-v1.json","record_version":1,"release":"2026-07-15-baton-v0.16-conformance","slice_id":"S22-openrouter-tool-structured-output","check_type":"spec-ambiguity","model_id":"` + model + `","immutable_start_commit":"a09b0e46df465862d00469d4aef2a997442b3d5b","attempt":` + strconv.Itoa(attempt) + `,"attempt_class":"` + class + `","result":"` + result + `","process_exit_code":` + exit + `}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func replaceProofReceiptFixture(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), old, replacement, 1)
	if updated == string(data) {
		t.Fatalf("fixture replacement %q not found in %s", old, path)
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
}

func llmCheckRepoFixture(t *testing.T) string {
	t.Helper()
	root := llmCheckFixture(t)
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "tests@example.invalid"},
		{"config", "user.name", "Sworn tests"},
		{"add", "."},
		{"commit", "-qm", "fixture"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	return root
}

func llmCheckConfig(t *testing.T, modelID string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	contents := `{"version":1,"verifier":{"model":"` + modelID + `"}}`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// hermeticLLMCheckEnv intentionally does not derive from os.Environ(). It
// prevents the binary reachability tests from reading login state, inherited
// provider credentials, or a user-configured proxy. The dead proxy protects
// against a regression that ignores the local fake endpoint override.
func hermeticLLMCheckEnv(t *testing.T, configPath, baseURLKey, baseURL string) []string {
	t.Helper()
	home := t.TempDir()
	xdgConfig := t.TempDir()
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + xdgConfig,
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"SWORN_CONFIG_PATH=" + configPath,
		"SWORN_DIRECT=1",
		"OPENAI_API_KEY=synthetic-openai-key",
		"XAI_API_KEY=synthetic-xai-key",
		"HTTP_PROXY=http://127.0.0.1:1",
		"HTTPS_PROXY=http://127.0.0.1:1",
		"ALL_PROXY=http://127.0.0.1:1",
		"NO_PROXY=127.0.0.1,localhost",
		"no_proxy=127.0.0.1,localhost",
		baseURLKey + "=" + baseURL,
	}
}

func assertLLMCheckOpenAIEnvelope(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("envelope schema is not JSON: %v", err)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Errorf("envelope root = %#v, want sealed object", schema)
	}
	assertJSONRequired(t, schema, "check", "verdict", "findings")
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("envelope properties = %#v, want object", schema["properties"])
	}
	check, ok := properties["check"].(map[string]any)
	if !ok || !jsonStringSetContainsAll(check["enum"], "spec-ambiguity", "design-review", "ac-satisfaction", "security-review", "semantic-coverage", "maintainability-review") {
		t.Errorf("envelope check vocabulary = %#v, want canonical enum", check)
	}
	findings, ok := properties["findings"].(map[string]any)
	if !ok {
		t.Fatalf("envelope findings = %#v, want schema object", properties["findings"])
	}
	items, ok := findings["items"].(map[string]any)
	if !ok {
		t.Fatalf("envelope findings.items = %#v, want object", findings["items"])
	}
	if items["type"] != "object" || items["additionalProperties"] != false {
		t.Errorf("envelope findings.items = %#v, want sealed object", items)
	}
	assertJSONRequired(t, items, "id", "severity", "blocking", "title", "detail")
	assertNoOpenAIStrictForbiddenKeywords(t, schema)
}

func assertJSONRequired(t *testing.T, schema map[string]any, want ...string) {
	t.Helper()
	if !jsonStringSetContainsAll(schema["required"], want...) {
		t.Errorf("required = %#v, want %v", schema["required"], want)
	}
}

func jsonStringSetContainsAll(value any, wants ...string) bool {
	got := map[string]bool{}
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if text, ok := value.(string); ok {
			got[text] = true
		}
	}
	for _, want := range wants {
		if !got[want] {
			return false
		}
	}
	return true
}

func assertNoOpenAIStrictForbiddenKeywords(t *testing.T, value any) {
	t.Helper()
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case "allOf", "if", "then", "else", "not":
				t.Errorf("envelope contains forbidden strict-schema keyword %q", key)
			}
			assertNoOpenAIStrictForbiddenKeywords(t, child)
		}
	case []any:
		for _, child := range node {
			assertNoOpenAIStrictForbiddenKeywords(t, child)
		}
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
	if err := os.WriteFile(configPath, []byte(`{"version":1,"verifier":{"model":"xai/test-model"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(buildSworn(t), "llm-check", "--type", "spec-ambiguity", "--slice", "S01-test", "--release", "test-release", "--base", "HEAD", "--json")
	cmd.Dir = root
	cmd.Env = hermeticLLMCheckEnv(t, configPath, "SWORN_XAI_BASE_URL", server.URL)
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

// TestLLMCheckOpenAIUnsupportedCanonicalSchemaMakesZeroRequests proves the
// dedicated C-02 map report never gets flattened, retried, or sent through the
// generic OpenAI envelope. Both native OpenAI wire formats must reject it in
// the built binary before the fake endpoint observes any request.
func TestLLMCheckOpenAIUnsupportedCanonicalSchemaMakesZeroRequests(t *testing.T) {
	root := llmCheckRepoFixture(t)
	releaseDir := filepath.Join(root, "docs", "release", "test-release")
	sliceDir := filepath.Join(releaseDir, "S01-test")
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.json"), []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "S01-test",
  "release": "test-release",
  "user_outcome": "CANARY-SPEC-MUST-NOT-LEAK",
  "covers_needs": ["N-01"],
  "acceptance_criteria": [{"id":"AC-01","text":"THE SYSTEM SHALL retain the dedicated map report.","ears_pattern":"ubiquitous"}],
  "in_scope": [],
  "out_of_scope": [],
  "references": []
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	for _, tt := range []struct {
		name       string
		modelID    string
		baseURLKey string
	}{
		{name: "Responses", modelID: "openai/test-model", baseURLKey: "SWORN_OPENAI_BASE_URL"},
		{name: "chat completions", modelID: "openai-completions/test-model", baseURLKey: "SWORN_OPENAI_COMPLETIONS_BASE_URL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hits.Store(0)
			configPath := llmCheckConfig(t, tt.modelID)
			cmd := exec.Command(buildSworn(t), "llm-check", "--type", "spec-ambiguity", "--slice", "S01-test", "--release", "test-release", "--base", "HEAD", "--json")
			cmd.Dir = root
			cmd.Env = hermeticLLMCheckEnv(t, configPath, tt.baseURLKey, server.URL)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("unsupported spec-ambiguity report exited 0: %s", output)
			}
			if got := cmd.ProcessState.ExitCode(); got != 2 {
				t.Fatalf("unsupported spec-ambiguity exit = %d, want 2. output:\n%s", got, output)
			}
			if hits.Load() != 0 {
				t.Fatalf("unsupported spec-ambiguity endpoint hits = %d, want 0", hits.Load())
			}
			text := string(output)
			if !strings.Contains(text, "rejected dedicated spec-ambiguity report") {
				t.Fatalf("missing stable local rejection: %s", text)
			}
			for _, leaked := range []string{"CANARY-SPEC", "synthetic-openai-key"} {
				if strings.Contains(text, leaked) {
					t.Fatalf("local rejection leaked %q: %s", leaked, text)
				}
			}
		})
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
