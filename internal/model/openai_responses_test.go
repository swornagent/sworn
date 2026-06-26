package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// responsesRoundTripHandler mimics /v1/responses for a reasoning model
// performing one tool call then a final text response.
func responsesRoundTripHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			t.Errorf("expected /responses path, got %s", r.URL.Path)
		}

		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		// Inspect request — these are the assertions built into the handler.
		// The caller sets the server URL, so we validate the first request and
		// return either a function_call or text response based on what the
		// caller sends (determined by whether tools are in the request).

		w.Header().Set("Content-Type", "application/json")

		if len(req.Tools) > 0 && req.Input[len(req.Input)-1].Role == "user" {
			// First turn with tools → return a function_call.
			json.NewEncoder(w).Encode(responsesAPIResponse{
				ID: "resp-toolcall",
				Output: []responsesOutput{
					{
						Type:      "function_call",
						CallID:    "call_abc123",
						Name:      "bash",
						Arguments: `{"command":"echo hello"}`,
					},
				},
				Usage: &responsesUsage{
					InputTokens:  100,
					OutputTokens: 50,
					TotalTokens:  150,
				},
			})
			return
		}

		// Final turn — return text.
		json.NewEncoder(w).Encode(responsesAPIResponse{
			ID: "resp-final",
			Output: []responsesOutput{
				{
					Type: "message",
					Role: "assistant",
					Content: []responsesContentItem{
						{Type: "output_text", Text: "PASS - all checks pass"},
					},
				},
			},
			Usage: &responsesUsage{
				InputTokens:  200,
				OutputTokens: 30,
				TotalTokens:  230,
			},
		})
	}
}

func TestOpenAIResponses_Verify(t *testing.T) {
	srv := httptest.NewServer(responsesRoundTripHandler(t))
	t.Cleanup(srv.Close)

	c := &OpenAIResponses{
		BaseURL:         srv.URL,
		Model:           "gpt-5.5",
		APIKey:          "test-key",
		ReasoningEffort: "medium",
	}

	text, cost, err := c.Verify(context.Background(), "You are a tester.", "Verify this.")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if text != "PASS - all checks pass" {
		t.Errorf("unexpected text: %q", text)
	}
	if cost == 0 {
		t.Error("expected non-zero cost for known model")
	}
}

func TestOpenAIResponses_Chat_ToolCallRoundTrip(t *testing.T) {
	srv := httptest.NewServer(responsesRoundTripHandler(t))
	t.Cleanup(srv.Close)

	c := &OpenAIResponses{
		BaseURL:         srv.URL,
		Model:           "gpt-5.5",
		APIKey:          "test-key",
		ReasoningEffort: "medium",
	}

	tools := []ToolDef{
		{
			Name:        "bash",
			Description: "Run a shell command.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
		},
	}

	messages := []ChatMessage{
		{Role: "system", Content: "You are a tester."},
		{Role: "user", Content: "Run a command."},
	}

	resp, err := c.Chat(context.Background(), messages, tools)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	if len(resp.Choices[0].Message.ToolCalls) == 0 {
		t.Fatal("expected tool calls in first response")
	}
	tc := resp.Choices[0].Message.ToolCalls[0]
	if tc.Function.Name != "bash" {
		t.Errorf("expected tool name 'bash', got %q", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"command":"echo hello"}` {
		t.Errorf("unexpected arguments: %s", tc.Function.Arguments)
	}
}

func TestOpenAIResponses_RequestShape(t *testing.T) {
	// Validate that the request omits temperature and includes reasoning_effort.
	var capturedReq responsesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			t.Fatalf("decode: %v", err)
		}
		json.NewEncoder(w).Encode(responsesAPIResponse{
			ID: "test",
			Output: []responsesOutput{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := &OpenAIResponses{
		BaseURL:         srv.URL,
		Model:           "gpt-5.5-pro",
		APIKey:          "test-key",
		ReasoningEffort: "high",
	}

	_, _, err := c.Verify(context.Background(), "sys", "hello")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if capturedReq.Reasoning == nil {
		t.Error("expected reasoning config in request")
	} else if capturedReq.Reasoning.Effort != "high" {
		t.Errorf("expected reasoning effort 'high', got %q", capturedReq.Reasoning.Effort)
	}

	// Temperature must NOT be present. We can't directly check absence on
	// the typed struct, but the struct doesn't have a Temperature field,
	// so it can't be serialized. This is the structural guarantee.

	// Instructions should come from system prompt.
	if capturedReq.Instructions != "sys" {
		t.Errorf("expected instructions 'sys', got %q", capturedReq.Instructions)
	}

	if len(capturedReq.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(capturedReq.Input))
	}
	if capturedReq.Input[0].Role != "user" {
		t.Errorf("expected user role, got %q", capturedReq.Input[0].Role)
	}
	if capturedReq.Input[0].Content != "hello" {
		t.Errorf("expected content 'hello', got %q", capturedReq.Input[0].Content)
	}
}

func TestOpenAIResponses_WebSearchTool(t *testing.T) {
	// When UseWebSearch is true, the request should include web_search_preview.
	var capturedReq responsesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			t.Fatalf("decode: %v", err)
		}
		json.NewEncoder(w).Encode(responsesAPIResponse{
			ID: "test",
			Output: []responsesOutput{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := &OpenAIResponses{
		BaseURL:         srv.URL,
		Model:           "gpt-5.5",
		APIKey:          "test-key",
		ReasoningEffort: "medium",
		UseWebSearch:    true,
	}

	_, _, err := c.Verify(context.Background(), "", "search test")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	found := false
	for _, tool := range capturedReq.Tools {
		if tool.Type == "web_search_preview" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected web_search_preview tool in request when UseWebSearch=true")
	}
}

func TestOpenAIResponses_WebSearchTool_Off(t *testing.T) {
	var capturedReq responsesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			t.Fatalf("decode: %v", err)
		}
		json.NewEncoder(w).Encode(responsesAPIResponse{
			ID: "test",
			Output: []responsesOutput{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := &OpenAIResponses{
		BaseURL:         srv.URL,
		Model:           "gpt-5.5",
		APIKey:          "test-key",
		ReasoningEffort: "medium",
		UseWebSearch:    false,
	}

	_, _, err := c.Verify(context.Background(), "", "search test")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	for _, tool := range capturedReq.Tools {
		if tool.Type == "web_search_preview" {
			t.Error("web_search_preview tool should NOT be present when UseWebSearch=false")
		}
	}
}

func TestOpenAIResponses_Chat_MultiTurnConversion(t *testing.T) {
	// Verify that ChatMessage → responses input conversion handles
	// system/user/assistant/tool messages correctly.
	var capturedReq responsesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			t.Fatalf("decode: %v", err)
		}
		json.NewEncoder(w).Encode(responsesAPIResponse{
			ID: "test",
			Output: []responsesOutput{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "done"}}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := &OpenAIResponses{
		BaseURL:         srv.URL,
		Model:           "gpt-5.5",
		APIKey:          "test-key",
		ReasoningEffort: "medium",
	}

	tcID := "call_1"
	messages := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "run a command"},
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "bash", Arguments: `{"command":"ls"}`}},
		}},
		{Role: "tool", Content: "file.txt", ToolCallID: &tcID},
	}

	_, err := c.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	// Check instructions (system message)
	if capturedReq.Instructions != "You are helpful." {
		t.Errorf("instructions = %q, want 'You are helpful.'", capturedReq.Instructions)
	}

	// Check input items
	if len(capturedReq.Input) != 3 {
		t.Fatalf("expected 3 input items, got %d: %+v", len(capturedReq.Input), capturedReq.Input)
	}
	// Item 0: user
	if capturedReq.Input[0].Role != "user" || capturedReq.Input[0].Content != "run a command" {
		t.Errorf("input[0]: %+v", capturedReq.Input[0])
	}
	// Item 1: function_call
	if capturedReq.Input[1].Type != "function_call" || capturedReq.Input[1].Name != "bash" {
		t.Errorf("input[1]: %+v", capturedReq.Input[1])
	}
	// Item 2: function_call_output
	if capturedReq.Input[2].Type != "function_call_output" || capturedReq.Input[2].Output != "file.txt" {
		t.Errorf("input[2]: %+v", capturedReq.Input[2])
	}
}

func TestOpenAIResponses_NewClient_Registration(t *testing.T) {
	// openai-responses prefix in NewClient should return *OpenAIResponses.
	v, err := NewClient("openai-responses/gpt-5.5", ProviderConfig{
		OpenAIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, ok := v.(*OpenAIResponses); !ok {
		t.Errorf("expected *OpenAIResponses, got %T", v)
	}
}

func TestNewOpenAIResponses_ReasoningDefault(t *testing.T) {
	// Default reasoning_effort is "medium" when env is unset.
	os.Unsetenv("SWORN_OPENAI_RESPONSES_REASONING_EFFORT")
	os.Unsetenv("SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH")
	c, err := NewOpenAIResponses("gpt-5.5", "test-key")
	if err != nil {
		t.Fatalf("NewOpenAIResponses: %v", err)
	}
	if c.ReasoningEffort != "medium" {
		t.Errorf("default effort = %q, want 'medium'", c.ReasoningEffort)
	}
	if c.UseWebSearch {
		t.Error("UseWebSearch should default to false")
	}
}

func TestNewOpenAIResponses_EnvOverrides(t *testing.T) {
	os.Setenv("SWORN_OPENAI_RESPONSES_REASONING_EFFORT", "low")
	os.Setenv("SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH", "1")
	t.Cleanup(func() {
		os.Unsetenv("SWORN_OPENAI_RESPONSES_REASONING_EFFORT")
		os.Unsetenv("SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH")
	})

	c, err := NewOpenAIResponses("gpt-5.5", "test-key")
	if err != nil {
		t.Fatalf("NewOpenAIResponses: %v", err)
	}
	if c.ReasoningEffort != "low" {
		t.Errorf("effort = %q, want 'low'", c.ReasoningEffort)
	}
	if !c.UseWebSearch {
		t.Error("UseWebSearch should be true when SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH=1")
	}
}

func TestOpenAIResponses_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
	}))
	t.Cleanup(srv.Close)

	c := &OpenAIResponses{
		BaseURL: srv.URL,
		Model:   "gpt-5.5",
		APIKey:  "test-key",
	}

	_, _, err := c.Verify(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	me, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *model.Error, got %T: %v", err, err)
	}
	if me.Status != 429 {
		t.Errorf("status = %d, want 429", me.Status)
	}
}

func TestConvertMessages_SystemPreservation(t *testing.T) {
	instructions, input := convertMessages([]ChatMessage{
		{Role: "system", Content: "Primary instruction."},
		{Role: "system", Content: "Extra — should be dropped."},
		{Role: "user", Content: "hello"},
	})

	if instructions != "Primary instruction." {
		t.Errorf("instructions = %q", instructions)
	}
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}
	if input[0].Role != "user" {
		t.Errorf("expected user, got %q", input[0].Role)
	}
}

func TestExtractOutputText(t *testing.T) {
	output := []responsesOutput{
		{Type: "reasoning", Content: []responsesContentItem{{Type: "reasoning_text", Text: "thinking..."}}},
		{Type: "function_call", CallID: "c1", Name: "bash", Arguments: `{}`},
		{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "final answer"}}},
	}
	text := extractOutputText(output)
	if text != "final answer" {
		t.Errorf("extracted = %q, want 'final answer'", text)
	}
}

func TestConvertToChatResponse(t *testing.T) {
	output := []responsesOutput{
		{Type: "function_call", CallID: "call_1", Name: "bash", Arguments: `{"cmd":"ls"}`},
		{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "done"}}},
	}
	cr := convertToChatResponse(output, &responsesUsage{
		InputTokens: 10, OutputTokens: 20, TotalTokens: 30,
	})

	if len(cr.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(cr.Choices))
	}
	c := cr.Choices[0]
	if c.Message.Content != "done" {
		t.Errorf("content = %q, want 'done'", c.Message.Content)
	}
	if len(c.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(c.Message.ToolCalls))
	}
	if c.Message.ToolCalls[0].Function.Name != "bash" {
		t.Errorf("tool name = %q, want 'bash'", c.Message.ToolCalls[0].Function.Name)
	}
	if cr.Usage.PromptTokens != 10 || cr.Usage.CompletionTokens != 20 {
		t.Errorf("usage mapping: prompt=%d completion=%d", cr.Usage.PromptTokens, cr.Usage.CompletionTokens)
	}
}
