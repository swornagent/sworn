package verify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/verdict"
)

// chatOnlyAgent implements agent.Agent (Chat) but NOT model.StructuredOutput.
// Used to prove RunAgentic fails closed (INCONCLUSIVE) when the verifier driver
// cannot emit a schema-constrained verdict (ADR-0011).
type chatOnlyAgent struct{}

func (chatOnlyAgent) Chat(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
	return chatResponse(`{"verdict":"PASS","rationale":"ignored"}`), nil
}

// structuredAgent implements BOTH agent.Agent and model.StructuredOutput — the
// shape of a real structured-capable driver (e.g. *model.OAI). RunAgentic
// type-asserts model.StructuredOutput and drives ChatStructured.
type structuredAgent struct {
	structuredFn func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error)
}

func (s *structuredAgent) Chat(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
	// RunAgentic must use ChatStructured, never Chat, on a structured driver.
	return nil, errors.New("Chat must not be called on the structured verifier path")
}

func (s *structuredAgent) ChatStructured(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
	return s.structuredFn(ctx, messages, schema)
}

func chatResponse(content string) *model.ChatResponse {
	return &model.ChatResponse{
		Choices: []struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Message: struct {
					Content   string           `json:"content"`
					ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
				}{
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: &model.UsageBlock{TotalTokens: 1000, PromptTokens: 700, CompletionTokens: 300},
	}
}

// TestRunAgenticPass drives the full structured path end-to-end (the
// reachability artefact): the verifier emits a schema-valid verifier-verdict-v1
// object, it validates, and the typed verdict comes off the object — no prose
// scrape. It also asserts the messages and the emit schema handed to the driver.
func TestRunAgenticPass(t *testing.T) {
	sa := &structuredAgent{
		structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
			if len(messages) < 2 || messages[0].Role != "system" {
				t.Fatalf("expected system + user messages, got %+v", messages)
			}
			if !strings.Contains(messages[0].Content, "Verifier Role Prompt") {
				t.Error("system prompt should be the verifier role prompt")
			}
			for _, sec := range []string{"## SPEC", "## DIFF", "## PROOF"} {
				if !strings.Contains(messages[1].Content, sec) {
					t.Errorf("user payload missing %s section", sec)
				}
			}
			// The emit schema is the judgement subset, named verifier-verdict-v1.
			if !strings.Contains(string(schema), "verifier-verdict-v1") {
				t.Error("emit schema should carry the verifier-verdict-v1 title")
			}
			if !strings.Contains(string(schema), "INCONCLUSIVE") {
				t.Error("emit schema should constrain the verdict enum")
			}
			return chatResponse(`{"verdict":"PASS","rationale":"All acceptance checks satisfied."}`), nil
		},
	}

	result, err := RunAgentic(context.Background(), "spec content", "diff content", "proof content", sa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Pass {
		t.Fatalf("expected PASS, got %s (%s)", result.Verdict, result.Rationale)
	}
	if result.Rationale != "All acceptance checks satisfied." {
		t.Errorf("rationale came off the typed object? got %q", result.Rationale)
	}
	if result.CostUSD <= 0 {
		t.Error("expected non-zero usage-based cost")
	}
	if result.InputTokens != 700 || result.OutputTokens != 300 {
		t.Errorf("expected token split 700/300, got %d/%d", result.InputTokens, result.OutputTokens)
	}
}

func TestRunAgenticFail(t *testing.T) {
	sa := &structuredAgent{
		structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
			return chatResponse(`{"verdict":"FAIL","rationale":"two problems","violations":[{"gate":"adversarial","description":"AC3 not satisfied"},{"gate":"tests","description":"missing coverage"}]}`), nil
		},
	}
	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", sa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Fail {
		t.Fatalf("expected FAIL, got %s", result.Verdict)
	}
	if len(result.Violations) != 2 {
		t.Fatalf("expected 2 typed violations, got %d (%v)", len(result.Violations), result.Violations)
	}
	if result.Violations[0] != "adversarial: AC3 not satisfied" {
		t.Errorf("violation came off the typed object? got %q", result.Violations[0])
	}
}

func TestRunAgenticBlocked(t *testing.T) {
	sa := &structuredAgent{
		structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
			return chatResponse(`{"verdict":"BLOCKED","rationale":"cannot verify","violations":[{"gate":"spec","description":"AC3 references a non-existent file"}],"routing":"needs_planner"}`), nil
		},
	}
	result, err := RunAgentic(context.Background(), "spec", "diff", "", sa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Blocked {
		t.Fatalf("expected BLOCKED, got %s", result.Verdict)
	}
	if result.Routing != "needs_planner" {
		t.Errorf("expected routing needs_planner off the typed object, got %q", result.Routing)
	}
	if len(result.Violations) != 1 {
		t.Errorf("expected 1 violation, got %v", result.Violations)
	}
}

// TestRunAgenticFailWithoutViolationsInconclusive is the fail-closed heart of
// the pilot: the schema requires a FAIL verdict to cite ≥1 violation, so a FAIL
// with none fails validation and resolves to INCONCLUSIVE — a property the old
// HasPrefix("FAIL") scrape could never enforce.
func TestRunAgenticFailWithoutViolationsInconclusive(t *testing.T) {
	sa := &structuredAgent{
		structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
			return chatResponse(`{"verdict":"FAIL","rationale":"vague failure with no cited violations"}`), nil
		},
	}
	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", sa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE (schema-invalid FAIL), got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_verdict_invalid" {
		t.Errorf("expected gate verifier_verdict_invalid, got %s", result.FailedGate)
	}
}

func TestRunAgenticMalformedEmissionInconclusive(t *testing.T) {
	sa := &structuredAgent{
		structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
			return chatResponse(`this is not a JSON object`), nil
		},
	}
	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", sa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE for malformed emission, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_structured_malformed" {
		t.Errorf("expected gate verifier_structured_malformed, got %s", result.FailedGate)
	}
}

func TestRunAgenticBadVerdictEnumInconclusive(t *testing.T) {
	sa := &structuredAgent{
		structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
			return chatResponse(`{"verdict":"MAYBE","rationale":"out-of-enum verdict"}`), nil
		},
	}
	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", sa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE for out-of-enum verdict, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_verdict_invalid" {
		t.Errorf("expected gate verifier_verdict_invalid, got %s", result.FailedGate)
	}
}

// TestRunAgenticNonStructuredAgentInconclusive proves a verifier driver that
// cannot emit structured output is not trusted to a prose verdict — fail closed.
func TestRunAgenticNonStructuredAgentInconclusive(t *testing.T) {
	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", chatOnlyAgent{})
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE for non-structured driver, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_structured_unsupported" {
		t.Errorf("expected gate verifier_structured_unsupported, got %s", result.FailedGate)
	}
}

func TestRunAgenticStructuredDispatchErrorInconclusive(t *testing.T) {
	sa := &structuredAgent{
		structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
			return nil, errors.New("provider 503")
		},
	}
	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", sa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE on dispatch error, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_structured_dispatch" {
		t.Errorf("expected gate verifier_structured_dispatch, got %s", result.FailedGate)
	}
}

// TestRunAgenticTerminalDispatchErrorBlocked proves a terminal provider error
// (revoked key, exhausted credits — model.IsTerminal) on the verifier dispatch
// surfaces as BLOCKED, not INCONCLUSIVE: triage maps BLOCKED to Halt, so the
// run loop cannot burn the implementer escalation ladder on an error that can
// never succeed on retry — mirroring the implementer path's terminal-error
// halt (S09 AC1).
func TestRunAgenticTerminalDispatchErrorBlocked(t *testing.T) {
	cases := []struct {
		name string
		kind model.ErrorKind
	}{
		{"auth_revoked_key", model.KindAuth},
		{"credits_exhausted", model.KindCredits},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sa := &structuredAgent{
				structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
					return nil, &model.Error{
						Kind:     tc.kind,
						Status:   401,
						Provider: "openai",
						Model:    "gpt-4o-mini",
						Message:  "credentials rejected",
					}
				},
			}
			result, err := RunAgentic(context.Background(), "spec", "diff", "proof", sa)
			if err != nil {
				t.Fatalf("RunAgentic: %v", err)
			}
			if result.Verdict != verdict.Blocked {
				t.Fatalf("expected BLOCKED for terminal %s error, got %s (%s)",
					tc.kind, result.Verdict, result.Rationale)
			}
			if result.FailedGate != "verifier_terminal_error" {
				t.Errorf("expected gate verifier_terminal_error, got %s", result.FailedGate)
			}
			if result.ExitCode() != 2 {
				t.Errorf("expected exit code 2 (BLOCKED), got %d", result.ExitCode())
			}
			if !strings.Contains(strings.ToLower(result.Rationale), tc.kind.String()) {
				t.Errorf("rationale should name the terminal kind %q, got %q", tc.kind, result.Rationale)
			}
		})
	}
}

// TestRunAgenticTransientTypedErrorInconclusive pins the boundary: typed but
// NON-terminal provider errors (rate limit, upstream 5xx) stay INCONCLUSIVE so
// triage retries/escalates — only terminal kinds halt as BLOCKED.
func TestRunAgenticTransientTypedErrorInconclusive(t *testing.T) {
	for _, kind := range []model.ErrorKind{model.KindRateLimit, model.KindUpstream, model.KindTransient, model.KindOther} {
		sa := &structuredAgent{
			structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
				return nil, &model.Error{Kind: kind, Provider: "openai", Message: "transient"}
			},
		}
		result, err := RunAgentic(context.Background(), "spec", "diff", "proof", sa)
		if err != nil {
			t.Fatalf("RunAgentic (%s): %v", kind, err)
		}
		if result.Verdict != verdict.Inconclusive {
			t.Fatalf("expected INCONCLUSIVE for transient %s error, got %s", kind, result.Verdict)
		}
		if result.FailedGate != "verifier_structured_dispatch" {
			t.Errorf("expected gate verifier_structured_dispatch for %s, got %s", kind, result.FailedGate)
		}
	}
}

func TestRunAgenticEmptyChoicesInconclusive(t *testing.T) {
	sa := &structuredAgent{
		structuredFn: func(ctx context.Context, messages []model.ChatMessage, schema []byte) (*model.ChatResponse, error) {
			return &model.ChatResponse{}, nil
		},
	}
	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", sa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Inconclusive {
		t.Fatalf("expected INCONCLUSIVE on empty choices, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_structured_dispatch" {
		t.Errorf("expected gate verifier_structured_dispatch, got %s", result.FailedGate)
	}
}
