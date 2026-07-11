package inprocess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/model"
)

// --- test scaffolding -------------------------------------------------------

// turn is one scripted provider response: an HTTP status and a raw JSON body.
type turn struct {
	status int
	body   string
}

// script serves scripted /chat/completions responses in order and records
// every raw request body so tests can inspect the exact bytes the driver put
// on the wire (AC-03).
type script struct {
	mu     sync.Mutex
	turns  []turn
	bodies [][]byte
}

func (s *script) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		i := len(s.bodies) - 1
		s.mu.Unlock()

		// Keep wall-clock duration measurably non-zero (AC-01 DurationMS).
		time.Sleep(2 * time.Millisecond)

		if i >= len(s.turns) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"message":"script exhausted"}}`)
			return
		}
		t := s.turns[i]
		if t.status == 0 {
			t.status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(t.status)
		fmt.Fprint(w, t.body)
	}
}

// requestCount returns how many requests the server has seen.
func (s *script) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

// structuredRequestCount counts requests carrying a response_format field —
// i.e. ChatStructured calls (the chat identity's strict json_schema path).
func (s *script) structuredRequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, b := range s.bodies {
		if bytes.Contains(b, []byte(`"response_format"`)) {
			n++
		}
	}
	return n
}

// toolCallTurn scripts an assistant turn that requests one bash tool call
// with EMPTY content — the tool-only shape behind the S27 content-omitempty
// regression (AC-03).
func toolCallTurn() turn {
	return turn{body: `{
		"model": "gpt-test",
		"choices": [{
			"message": {
				"content": "",
				"tool_calls": [{
					"id": "tc-1",
					"type": "function",
					"function": {"name": "bash", "arguments": "{\"command\":\"echo hi\"}"}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 11, "completion_tokens": 5, "total_tokens": 16}
	}`}
}

// textTurn scripts a terminal assistant turn (no tool calls).
func textTurn(content string) turn {
	return turn{body: `{
		"model": "gpt-test",
		"choices": [{"message": {"content": ` + jsonString(content) + `}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 13, "completion_tokens": 7, "total_tokens": 20}
	}`}
}

// pricedTextTurn scripts a terminal assistant turn (no tool calls) whose
// "model" field is a real, pricing-registry-known model ID rather than the
// synthetic "gpt-test" — used to exercise AC-02's pricing-table happy path
// (TestInprocessImplementerPricingTable) distinctly from the fail-closed
// "unknown" path the other tests' unpriced "gpt-test" model exercises.
func pricedTextTurn(model, content string) turn {
	return turn{body: `{
		"model": ` + jsonString(model) + `,
		"choices": [{"message": {"content": ` + jsonString(content) + `}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 13, "completion_tokens": 7, "total_tokens": 20}
	}`}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// providerErrorTurn scripts a classified provider failure.
func providerErrorTurn(status int, message string) turn {
	return turn{status: status, body: `{"error":{"message":` + jsonString(message) + `}}`}
}

var verdictSchema = []byte(`{
	"$id": "https://baton.sawy3r.net/schemas/verifier-verdict-v1.json",
	"type": "object",
	"properties": {"verdict": {"type": "string"}, "reason": {"type": "string"}},
	"required": ["verdict", "reason"]
}`)

// tempWorktree creates a real git working tree so AssertWorktree (Rule 11)
// passes for the dispatch under test.
func tempWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v (%s)", dir, err, out)
	}
	return dir
}

// testDriver returns the chat identity wired to the scripted server via the
// injected client factory — no dispatch ever leaves the process.
func testDriver(srvURL string) *InProcess {
	d := NewOAIChat(model.ProviderConfig{})
	d.maxTurns = 8
	d.newClient = func(modelID string, pcfg model.ProviderConfig) (model.Verifier, error) {
		return &model.OAI{
			BaseURL:    srvURL,
			Model:      "gpt-test",
			APIKey:     "test-key",
			Structured: model.StructuredResponseFormat,
		}, nil
	}
	return d
}

func dispatchInput(role driver.Role, worktree string) driver.DispatchInput {
	in := driver.DispatchInput{
		Role:         role,
		ModelID:      "openai/gpt-test",
		SystemPrompt: "You are the " + string(role) + ".",
		Payload:      "Do the thing.",
		WorktreeRoot: worktree,
		Timeout:      30 * time.Second,
	}
	if role == driver.RoleVerifier {
		in.StructuredSchema = verdictSchema
	}
	return in
}

// --- identity + contract shape ----------------------------------------------

func TestInprocessIdentities(t *testing.T) {
	pcfg := model.ProviderConfig{}
	chat := NewOAIChat(pcfg)
	if got := chat.Name(); got != "oai-inprocess" {
		t.Errorf("NewOAIChat().Name() = %q, want %q", got, "oai-inprocess")
	}
	responses := NewOAIResponses(pcfg)
	if got := responses.Name(); got != "oai-responses-inprocess" {
		t.Errorf("NewOAIResponses().Name() = %q, want %q", got, "oai-responses-inprocess")
	}
	for _, d := range []*InProcess{chat, responses} {
		roles := d.Roles()
		if !roles.Has(driver.RoleImplementer) || !roles.Has(driver.RoleVerifier) {
			t.Errorf("%s: Roles() = %v, want implementer+verifier", d.Name(), roles)
		}
		// S06 D2: the in-process identities declare captain — the
		// captain-family calls are single tool-less judgement dispatches
		// served by dispatchCaptain (never the tool loop).
		if !roles.Has(driver.RoleCaptain) {
			t.Errorf("%s: Roles() must declare captain (S06 D2)", d.Name())
		}
	}
	// Both identities satisfy the contract.
	var _ driver.Driver = chat
	var _ driver.Driver = responses
}

// --- AC-01: implementer runs the multi-turn tool loop ------------------------

func TestInprocessImplementer(t *testing.T) {
	s := &script{turns: []turn{toolCallTurn(), textTurn("done: hi printed")}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	d := testDriver(srv.URL)
	res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, tempWorktree(t)))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Status != driver.StatusOK {
		t.Fatalf("Status = %q, want %q (ErrKind %q)", res.Status, driver.StatusOK, res.ErrKind)
	}
	if res.ResultText != "done: hi printed" {
		t.Errorf("ResultText = %q, want the loop's final text", res.ResultText)
	}
	if s.requestCount() != 2 {
		t.Errorf("server saw %d requests, want 2 (tool-call turn + terminal turn)", s.requestCount())
	}
	// Token accumulation across BOTH turns (11+13 prompt, 5+7 completion).
	if res.InputTokens != 24 {
		t.Errorf("InputTokens = %d, want 24", res.InputTokens)
	}
	if res.OutputTokens != 12 {
		t.Errorf("OutputTokens = %d, want 12", res.OutputTokens)
	}
	if res.DurationMS <= 0 {
		t.Errorf("DurationMS = %d, want > 0", res.DurationMS)
	}
	if res.ModelID != "gpt-test" {
		t.Errorf("ModelID = %q, want provider-confirmed %q", res.ModelID, "gpt-test")
	}
	// "gpt-test" is a synthetic test model with no pricing-registry entry —
	// this exercises AC-04's fail-closed path (S08 replaces the old flat
	// "estimated" nominal-rate stand-in, Coach ack pin 5): no guessed rate,
	// CostUSD=0, CostSource=unknown, plus a stderr warning naming the model.
	if res.CostUSD != 0 || res.CostSource != driver.CostSourceUnknown {
		t.Errorf("CostUSD/CostSource = %v/%q, want 0/%q (no pricing entry for the synthetic test model)", res.CostUSD, res.CostSource, driver.CostSourceUnknown)
	}
}

// TestInprocessImplementerPricingTable proves AC-02's happy path: when the
// CONFIRMED response model-id resolves in the unified pricing registry,
// CostUSD is computed from the true accumulated token split and
// CostSource=pricing-table — not a flat nominal estimate.
func TestInprocessImplementerPricingTable(t *testing.T) {
	s := &script{turns: []turn{pricedTextTurn("gpt-4o-mini", "done")}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	d := testDriver(srv.URL)
	res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, tempWorktree(t)))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Status != driver.StatusOK {
		t.Fatalf("Status = %q, want %q (ErrKind %q)", res.Status, driver.StatusOK, res.ErrKind)
	}
	if res.ModelID != "gpt-4o-mini" {
		t.Errorf("ModelID = %q, want provider-confirmed %q", res.ModelID, "gpt-4o-mini")
	}
	if res.CostSource != driver.CostSourcePricingTable {
		t.Errorf("CostSource = %q, want %q", res.CostSource, driver.CostSourcePricingTable)
	}
	// gpt-4o-mini: $0.15/$0.60 per 1M. 13 prompt + 7 completion tokens (one
	// textTurn-shaped turn, see pricedTextTurn).
	wantCost := float64(13)/1_000_000*0.15 + float64(7)/1_000_000*0.60
	if res.CostUSD < wantCost-0.0000001 || res.CostUSD > wantCost+0.0000001 {
		t.Errorf("CostUSD = %v, want ~%v (computed from real tokens via the pricing registry)", res.CostUSD, wantCost)
	}
}

// --- AC-02: verifier = tool loop, then exactly one ChatStructured verdict ----

func TestInprocessVerifier(t *testing.T) {
	verdict := `{"verdict":"PASS","reason":"tests re-run green"}`
	s := &script{turns: []turn{
		toolCallTurn(),
		textTurn("investigation complete"),
		textTurn(verdict), // response to the single ChatStructured call
	}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	d := testDriver(srv.URL)
	res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleVerifier, tempWorktree(t)))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Status != driver.StatusOK {
		t.Fatalf("Status = %q, want ok (ErrKind %q)", res.Status, res.ErrKind)
	}
	if string(res.StructuredJSON) != verdict {
		t.Errorf("StructuredJSON = %s, want the emitted verdict unmodified", res.StructuredJSON)
	}
	if res.ResultText != "investigation complete" {
		t.Errorf("ResultText = %q, want the investigation loop's final text", res.ResultText)
	}
	if got := s.structuredRequestCount(); got != 1 {
		t.Errorf("structured (response_format) requests = %d, want exactly 1", got)
	}
	if s.requestCount() != 3 {
		t.Errorf("server saw %d requests, want 3 (2 loop turns + 1 verdict call)", s.requestCount())
	}
	// The verdict call replays the accumulated transcript: its messages must
	// include the tool result from the investigation loop.
	s.mu.Lock()
	last := s.bodies[len(s.bodies)-1]
	s.mu.Unlock()
	if !bytes.Contains(last, []byte(`"role":"tool"`)) {
		t.Errorf("verdict call does not carry the investigation transcript (no tool message in request body)")
	}
}

// --- AC-03: tool-only turns still serialize a present content field ----------

func TestInprocessContentAlwaysPresent(t *testing.T) {
	s := &script{turns: []turn{toolCallTurn(), textTurn("done")}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	d := testDriver(srv.URL)
	if _, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, tempWorktree(t))); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// The SECOND request replays the first turn's tool-only assistant message
	// (empty content) back to the model. Inspect its raw body: the assistant
	// message must still carry a present "content" key (S27 regression guard).
	s.mu.Lock()
	if len(s.bodies) < 2 {
		s.mu.Unlock()
		t.Fatalf("server saw %d requests, want 2", len(s.bodies))
	}
	raw := s.bodies[1]
	s.mu.Unlock()

	var req struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal raw request body: %v", err)
	}
	found := false
	for _, msg := range req.Messages {
		if _, isToolCall := msg["tool_calls"]; !isToolCall {
			continue
		}
		found = true
		content, present := msg["content"]
		if !present {
			t.Errorf("tool-only assistant message serialized WITHOUT a content field (S27 content-omitempty regression): %s", raw)
		} else if string(content) != `""` {
			t.Errorf("tool-only assistant message content = %s, want present empty string", content)
		}
	}
	if !found {
		t.Fatalf("no tool-call assistant message found in replayed request body: %s", raw)
	}
}

// --- AC-04: max-turns → transient; structured-emission failure → protocol ----

func TestInprocessMaxTurnsTransient(t *testing.T) {
	// Every turn requests another tool call; the cap (2) acts as circuit
	// breaker and the exit maps to a retryable error, never a panic.
	s := &script{turns: []turn{toolCallTurn(), toolCallTurn()}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	d := testDriver(srv.URL)
	d.maxTurns = 2
	res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, tempWorktree(t)))
	if err == nil {
		t.Fatalf("Dispatch returned nil error, want max-turns failure")
	}
	if !errors.Is(err, agent.ErrMaxTurns) {
		t.Errorf("err = %v, want errors.Is(err, agent.ErrMaxTurns)", err)
	}
	if res.Status != driver.StatusError {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if res.ErrKind != driver.ErrKindTransient {
		t.Errorf("ErrKind = %q, want %q (retryable)", res.ErrKind, driver.ErrKindTransient)
	}
}

func TestInprocessVerdictEmissionProtocol(t *testing.T) {
	cases := []struct {
		name string
		last turn
	}{
		{"empty choices", turn{body: `{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1}}`}},
		{"content not a JSON object", textTurn("PASS — looks good to me")},
		{"empty content", textTurn("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &script{turns: []turn{textTurn("investigation complete"), tc.last}}
			srv := httptest.NewServer(s.handler())
			defer srv.Close()

			d := testDriver(srv.URL)
			res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleVerifier, tempWorktree(t)))
			if err == nil {
				t.Fatalf("Dispatch returned nil error, want structured-emission failure")
			}
			if res.Status != driver.StatusError {
				t.Errorf("Status = %q, want error", res.Status)
			}
			if res.ErrKind != driver.ErrKindProtocol {
				t.Errorf("ErrKind = %q, want %q", res.ErrKind, driver.ErrKindProtocol)
			}
			if len(res.StructuredJSON) != 0 {
				t.Errorf("StructuredJSON = %s, want empty — never a fabricated verdict", res.StructuredJSON)
			}
		})
	}
}

// --- Coach ack pins 1+2: terminal provider errors preserve *model.Error ------

func TestInprocessTerminalErrorsPreserveModelError(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		wantErrKind string
	}{
		{"auth 401", http.StatusUnauthorized, driver.ErrKindAuth},
		{"credits 402", http.StatusPaymentRequired, driver.ErrKindCredits},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &script{turns: []turn{providerErrorTurn(tc.status, "provider rejected the dispatch")}}
			srv := httptest.NewServer(s.handler())
			defer srv.Close()

			d := testDriver(srv.URL)
			res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, tempWorktree(t)))
			if err == nil {
				t.Fatalf("Dispatch returned nil error, want provider failure")
			}
			if res.Status != driver.StatusError || res.ErrKind != tc.wantErrKind {
				t.Errorf("Status/ErrKind = %q/%q, want error/%q", res.Status, res.ErrKind, tc.wantErrKind)
			}
			// Pin 1 option (a): the underlying *model.Error stays in the
			// returned chain so the engine's model.IsTerminal terminal-halt
			// keeps firing on the pre-S06 path.
			if !model.IsTerminal(err) {
				t.Errorf("model.IsTerminal(err) = false, want true — fail-fast signal lost (Coach ack pin 1)")
			}
		})
	}
}

// --- Coach ack pin 6: classified provider error on the verdict call keeps its
// real ErrKind rather than folding into protocol ------------------------------

func TestInprocessVerdictProviderErrorKeepsKind(t *testing.T) {
	s := &script{turns: []turn{
		textTurn("investigation complete"),
		providerErrorTurn(http.StatusUnauthorized, "key revoked mid-run"),
	}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	d := testDriver(srv.URL)
	res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleVerifier, tempWorktree(t)))
	if err == nil {
		t.Fatalf("Dispatch returned nil error, want auth failure")
	}
	if res.ErrKind != driver.ErrKindAuth {
		t.Errorf("ErrKind = %q, want %q — a transport auth failure is not a protocol failure (Coach ack pin 6)", res.ErrKind, driver.ErrKindAuth)
	}
	if !model.IsTerminal(err) {
		t.Errorf("model.IsTerminal(err) = false, want true")
	}
}

// --- D7 fail-closed guards ----------------------------------------------------

func TestInprocessConfigGuards(t *testing.T) {
	srvNever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("guarded dispatch must not reach the provider")
	}))
	defer srvNever.Close()

	t.Run("empty WorktreeRoot", func(t *testing.T) {
		d := testDriver(srvNever.URL)
		res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, ""))
		if err == nil || res.ErrKind != driver.ErrKindConfig {
			t.Errorf("Result/err = %+v/%v, want config error", res, err)
		}
	})
	t.Run("not a git worktree", func(t *testing.T) {
		d := testDriver(srvNever.URL)
		res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, t.TempDir()))
		if err == nil || res.ErrKind != driver.ErrKindConfig {
			t.Errorf("Result/err = %+v/%v, want config error", res, err)
		}
	})
	t.Run("model resolution failure", func(t *testing.T) {
		d := NewOAIChat(model.ProviderConfig{})
		d.newClient = func(string, model.ProviderConfig) (model.Verifier, error) {
			return nil, fmt.Errorf("unknown provider")
		}
		res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, tempWorktree(t)))
		if err == nil || res.ErrKind != driver.ErrKindConfig {
			t.Errorf("Result/err = %+v/%v, want config error", res, err)
		}
	})
	t.Run("chat-incapable client", func(t *testing.T) {
		d := NewOAIChat(model.ProviderConfig{})
		d.newClient = func(string, model.ProviderConfig) (model.Verifier, error) {
			return model.Unconfigured{}, nil // Verifier only, no Chat
		}
		res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleImplementer, tempWorktree(t)))
		if err == nil || res.ErrKind != driver.ErrKindConfig {
			t.Errorf("Result/err = %+v/%v, want config error", res, err)
		}
	})
}

// chatOnly can chat but cannot emit structured output — the verifier path
// must reject it by construction, before the investigation loop runs.
type chatOnly struct{}

func (chatOnly) Verify(context.Context, string, string) (string, float64, int64, int64, error) {
	return "", 0, 0, 0, nil
}

func (chatOnly) Chat(context.Context, []model.ChatMessage, []model.ToolDef) (*model.ChatResponse, error) {
	return nil, fmt.Errorf("chatOnly.Chat must not be called: the StructuredOutput assert runs first")
}

func TestInprocessVerifierRequiresStructuredOutput(t *testing.T) {
	d := NewOAIChat(model.ProviderConfig{})
	d.newClient = func(string, model.ProviderConfig) (model.Verifier, error) {
		return chatOnly{}, nil
	}
	res, err := d.Dispatch(context.Background(), dispatchInput(driver.RoleVerifier, tempWorktree(t)))
	if err == nil || res.ErrKind != driver.ErrKindProtocol {
		t.Errorf("Result/err = %+v/%v, want protocol error (can chat, cannot emit a verdict)", res, err)
	}
}

// --- S02: schema-constrained captain dispatch (design TL;DR / reqverify DoR) --

// captainSchema is a minimal sworn-local emit schema for the structured captain
// tests — strict-mode-subset (no minLength/pattern/format).
var captainSchema = []byte(`{
	"title": "captain-emit",
	"type": "object",
	"additionalProperties": false,
	"properties": {"summary": {"type": "string"}},
	"required": ["summary"]
}`)

func captainStructuredInput(worktree string) driver.DispatchInput {
	return driver.DispatchInput{
		Role:             driver.RoleCaptain,
		ModelID:          "openai/gpt-test",
		SystemPrompt:     "You are the captain.",
		Payload:          "Emit the structured object.",
		WorktreeRoot:     worktree,
		Timeout:          30 * time.Second,
		StructuredSchema: captainSchema,
	}
}

// TestInprocessCaptainStructured proves the S02 driver-contract change: a
// captain dispatch carrying StructuredSchema emits via ChatStructured — exactly
// ONE call (no investigation loop, unlike the verifier) — and returns the JSON
// unmodified in Result.StructuredJSON. This is the seam the design + DoR gates
// ride (AC-01/AC-02).
func TestInprocessCaptainStructured(t *testing.T) {
	emission := `{"summary":"the design is coherent and in scope"}`
	s := &script{turns: []turn{textTurn(emission)}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	d := testDriver(srv.URL)
	res, err := d.Dispatch(context.Background(), captainStructuredInput(tempWorktree(t)))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Status != driver.StatusOK {
		t.Fatalf("Status = %q, want ok (ErrKind %q)", res.Status, res.ErrKind)
	}
	if string(res.StructuredJSON) != emission {
		t.Errorf("StructuredJSON = %s, want the emission unmodified", res.StructuredJSON)
	}
	if res.ResultText != emission {
		t.Errorf("ResultText = %q, want the emission (prose-only callers still see content)", res.ResultText)
	}
	if got := s.structuredRequestCount(); got != 1 {
		t.Errorf("structured (response_format) requests = %d, want exactly 1", got)
	}
	if s.requestCount() != 1 {
		t.Errorf("server saw %d requests, want 1 — a captain structured dispatch is one-shot (no investigation loop)", s.requestCount())
	}
}

// TestInprocessCaptainProseUnchanged proves the nil-schema captain path is the
// unchanged prose Chat call (S02 D1: prose path unchanged when schema is nil).
func TestInprocessCaptainProseUnchanged(t *testing.T) {
	s := &script{turns: []turn{textTurn("prose judgement, no schema")}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	d := testDriver(srv.URL)
	in := captainStructuredInput(tempWorktree(t))
	in.StructuredSchema = nil // prose path
	res, err := d.Dispatch(context.Background(), in)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Status != driver.StatusOK || res.ResultText != "prose judgement, no schema" {
		t.Errorf("Status/ResultText = %q/%q, want ok/prose", res.Status, res.ResultText)
	}
	if len(res.StructuredJSON) != 0 {
		t.Errorf("StructuredJSON = %s, want empty on the prose path", res.StructuredJSON)
	}
	if got := s.structuredRequestCount(); got != 0 {
		t.Errorf("structured requests = %d, want 0 on the prose path", got)
	}
}

// TestInprocessCaptainStructuredRequiresStructuredOutput is AC-03 at the driver
// seam: a captain schema-constrained dispatch to a client that can chat but
// cannot emit structured output fails closed with ErrKindUnsupported — a
// DECLARED Rule 2 deferral the gate records, distinct from ErrKindProtocol (a
// structured-emission failure).
func TestInprocessCaptainStructuredRequiresStructuredOutput(t *testing.T) {
	d := NewOAIChat(model.ProviderConfig{})
	d.newClient = func(string, model.ProviderConfig) (model.Verifier, error) {
		return chatOnly{}, nil
	}
	res, err := d.Dispatch(context.Background(), captainStructuredInput(tempWorktree(t)))
	if err == nil || res.ErrKind != driver.ErrKindUnsupported {
		t.Errorf("Result/err = %+v/%v, want unsupported error (can chat, cannot emit structured output)", res, err)
	}
	if len(res.StructuredJSON) != 0 {
		t.Errorf("StructuredJSON = %s, want empty — never a fabricated emission", res.StructuredJSON)
	}
}
