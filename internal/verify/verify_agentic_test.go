package verify

import (
	"context"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/verdict"
)

// fakeAgent is a test fake implementing agent.Agent.
type fakeAgent struct {
	chatFn func(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error)
}

func (f *fakeAgent) Chat(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
	return f.chatFn(ctx, messages, tools)
}

func chatResponse(content string) *model.ChatResponse {
	return &model.ChatResponse{
		Choices: []struct {
			Message struct {
				Content   string          `json:"content"`
				ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Message: struct {
					Content   string          `json:"content"`
					ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
				}{
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: &model.UsageBlock{TotalTokens: 1000},
	}
}
func TestRunAgenticPass(t *testing.T) {
	fa := &fakeAgent{
		chatFn: func(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
			// Verify the system prompt is the verifier role prompt.
			if len(messages) < 2 {
				t.Fatal("expected at least system + user messages")
			}
			if messages[0].Role != "system" {
				t.Errorf("expected system role, got %s", messages[0].Role)
			}
			// The system prompt should contain the verifier role prompt.
			if !strings.Contains(messages[0].Content, "Verifier Role Prompt") {
				t.Error("system prompt should contain verifier role prompt")
			}
			// The user message should contain SPEC + DIFF + PROOF.
			if !strings.Contains(messages[1].Content, "## SPEC") {
				t.Error("user message should contain SPEC section")
			}
			if !strings.Contains(messages[1].Content, "## DIFF") {
				t.Error("user message should contain DIFF section")
			}
			if !strings.Contains(messages[1].Content, "## PROOF") {
				t.Error("user message should contain PROOF section")
			}
			// No tools should be requested.
			if tools != nil {
				t.Error("agentic verifier should not receive tools")
			}
			return chatResponse("PASS\n\nAll acceptance checks satisfied."), nil
		},
	}

	result, err := RunAgentic(context.Background(), "spec content", "diff content", "proof content", fa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Pass {
		t.Errorf("expected PASS, got %s", result.Verdict)
	}
	if result.CostUSD <= 0 {
		t.Error("expected non-zero cost (usage-based)")
	}
}

func TestRunAgenticFail(t *testing.T) {
	fa := &fakeAgent{
		chatFn: func(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
			return chatResponse("FAIL:\n1. missing test coverage\n2. spec AC3 not satisfied"), nil
		},
	}

	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", fa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Fail {
		t.Errorf("expected FAIL, got %s", result.Verdict)
	}
	if result.FailedGate != "adversarial" {
		t.Errorf("expected FailedGate adversarial, got %s", result.FailedGate)
	}
}

func TestRunAgenticBlocked(t *testing.T) {
	fa := &fakeAgent{
		chatFn: func(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
			return chatResponse("BLOCKED: spec acceptance check 3 references non-existent file"), nil
		},
	}

	result, err := RunAgentic(context.Background(), "spec", "diff", "", fa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED, got %s", result.Verdict)
	}
}

func TestRunAgenticUnparseableBlocks(t *testing.T) {
	fa := &fakeAgent{
		chatFn: func(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
			return chatResponse("Here is a detailed analysis of the code..."), nil
		},
	}

	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", fa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED for unparseable output, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_agentic_unparseable" {
		t.Errorf("expected FailedGate verifier_agentic_unparseable, got %s", result.FailedGate)
	}
}

func TestRunAgenticEmptyChoicesBlocks(t *testing.T) {
	fa := &fakeAgent{
		chatFn: func(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
			return &model.ChatResponse{}, nil
		},
	}

	result, err := RunAgentic(context.Background(), "spec", "diff", "proof", fa)
	if err != nil {
		t.Fatalf("RunAgentic: %v", err)
	}
	if result.Verdict != verdict.Blocked {
		t.Errorf("expected BLOCKED for empty choices, got %s", result.Verdict)
	}
	if result.FailedGate != "verifier_agentic_dispatch" {
		t.Errorf("expected FailedGate verifier_agentic_dispatch, got %s", result.FailedGate)
	}
}