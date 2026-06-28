package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/swornagent/sworn/internal/account"
) // OAI dispatches a single /chat/completions call to an OpenAI-compatible
// endpoint. It implements Verifier via stdlib net/http + encoding/json
// (zero third-party dependencies per AGENTS.md).
//
// Normalisation strategy (spec Risk #1, Captain pin 4): json.Decode into a
// struct with only the fields SwornAgent needs. Unknown fields in the response
// are silently ignored — this is the normalisation. Missing or unparseable
// response is an error, mapped to BLOCKED by the caller.
//
// No logging of API keys, request bodies, or response payloads — per project
// AGENTS.md Security rule (spec Risk #2, Captain pin 3).
type OAI struct {
	BaseURL string // e.g. https://api.openai.com/v1
	Model   string // e.g. gpt-4.1
	APIKey  string
	Client  *http.Client // nil means http.DefaultClient
}

// ToolDef describes a tool the model may call. Name, Description, and the
// JSON Schema for Parameters are the wire format in the /chat/completions
// tools array. ToolDef is defined in the model package (the wire-format
// owner); agent tools provide their definition via Schema() model.ToolDef
// to avoid hand-editing JSON schema on both sides of the boundary.
//
// JSON serialisation uses the OpenAI-compliant nested format:
// {"type":"function","function":{"name":"...","description":"...","parameters":{...}}}
// MarshalJSON handles this so callers construct ToolDef flat.
type ToolDef struct {
	Name        string          `json:"-"`
	Description string          `json:"-"`
	Parameters  json.RawMessage `json:"-"`
}

// ToolFunction is the nested function object in an OpenAI tool definition.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// MarshalJSON serialises ToolDef in the OpenAI-compliant format with
// "type": "function" and the nested "function" object. This is required
// by OpenRouter (and is the canonical OpenAI API format); direct OpenAI
// accepts both shapes but OpenRouter strictly validates.
func (td ToolDef) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string       `json:"type"`
		Function ToolFunction `json:"function"`
	}{
		Type: "function",
		Function: ToolFunction{
			Name:        td.Name,
			Description: td.Description,
			Parameters:  td.Parameters,
		},
	})
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []ToolDef     `json:"tools,omitempty"`
}

// ChatMessage is a single message in a /chat/completions conversation.
// Exported so callers (agent package) can build message history.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"` // EVAL FIX 2026-06-28: omitempty dropped 'content' on tool-only assistant turns → OpenAI "content: got null" / DeepSeek "missing field content". Always emit (incl "").
	Name       string     `json:"name,omitempty"`
	ToolCallID *string    `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ChatResponse contains only the fields SwornAgent needs. Other fields from
// the provider's response are silently ignored (normalisation per Risk #1).
type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage   *UsageBlock `json:"usage"`
	CostUSD float64     `json:"-"` // computed by driver from Usage × pricing
}

// ToolCall is a single tool invocation the model requests in a response.
// Exported so the agent package can reconstruct message history.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall is the function name and arguments within a ToolCall.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// UsageBlock carries token usage from the API response.
type UsageBlock struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// InputTokens / OutputTokens are provider-agnostic aliases for drivers
	// whose native response shape uses different field names (e.g. Anthropic's
	// input_tokens / output_tokens). OAI-derived drivers populate both sets;
	// native drivers populate only InputTokens/OutputTokens.
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// modelPricing maps a model ID to USD per 1M tokens. A model not in the table
// gets a zero cost. Expand as needed; S10 (benchmark) will make this
// data-driven.
//
// Prices sourced from public API pricing pages (USD).
var modelPricing = map[string]struct {
	promptCostPer1M     float64
	completionCostPer1M float64
}{
	"gpt-4.1":      {2.00, 8.00},
	"gpt-4.1-mini": {0.30, 0.80},
	"gpt-4.1-nano": {0.10, 0.40},
	"gpt-4o":       {2.50, 10.00},
	"gpt-4o-mini":  {0.15, 0.60},
	"o4-mini":      {1.10, 4.40},
	"o3":           {10.00, 40.00},
	"o3-mini":      {1.10, 4.40},
	// gpt-5.x reasoning models (responses API pricing, USD per 1M tokens).
	// Preliminary — confirm with https://openai.com/api/pricing/.
	"gpt-5.5":       {1.25, 10.00},
	"gpt-5.5-pro":   {2.50, 20.00},
	"gpt-5.3-codex": {3.00, 12.00},
}

// Verify sends the system prompt + user payload to /chat/completions.
// On any HTTP error, timeout, or unparseable response it returns an error
// (not a panic) — the caller (verify.Run) maps errors to BLOCKED, fulfilling
// spec AC4.
// Capabilities returns CapVerify | CapChat — the OAI driver supports both
// single-shot verification and multi-turn chat.
func (c *OAI) Capabilities() Capability { return CapVerify | CapChat }

func (c *OAI) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error) {
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPayload},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqBody); err != nil {
		return "", 0, fmt.Errorf("model: marshal request: %w", err)
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", 0, fmt.Errorf("model: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("model: dispatch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("model: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		me := NewProviderError(resp.StatusCode, "openai", c.Model, body)
		// 402 Payment Required — insufficient credits (Coach ack pin C).
		// Never silently downgrade to a direct provider call.
		if resp.StatusCode == http.StatusPaymentRequired {
			me.Err = account.ErrInsufficientCredits
		}
		return "", 0, me
	}

	var cr ChatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", 0, fmt.Errorf("model: unmarshal response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", 0, fmt.Errorf("model: empty choices in response")
	}

	cost := computeCost(c.Model, cr.Usage)
	return cr.Choices[0].Message.Content, cost, nil
}

// Chat sends a multi-message conversation (possibly with tool definitions// and tool-call history) to /chat/completions. It returns the full
// ChatResponse so the caller can inspect tool_calls and finish_reason.
// Cost is the sum of all Chat calls in the loop — tracked by the caller.
//
// No logging of message content — per AGENTS.md Security. The message
// history may contain file contents and command output.
func (c *OAI) Chat(ctx context.Context, messages []ChatMessage, tools []ToolDef) (*ChatResponse, error) {
	reqBody := chatRequest{
		Model:    c.Model,
		Messages: messages,
		Tools:    tools,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqBody); err != nil {
		return nil, fmt.Errorf("model: marshal request: %w", err)
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("model: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("model: dispatch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("model: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		me := NewProviderError(resp.StatusCode, "openai", c.Model, body)
		// 402 Payment Required — insufficient credits (Coach ack pin C).
		if resp.StatusCode == http.StatusPaymentRequired {
			me.Err = account.ErrInsufficientCredits
		}
		return nil, me
	}

	var cr ChatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, fmt.Errorf("model: unmarshal response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return nil, fmt.Errorf("model: empty choices in response")
	}

	return &cr, nil
}
func computeCost(model string, usage *UsageBlock) float64 {
	p, ok := modelPricing[model]
	if !ok || usage == nil {
		return 0
	}
	promptCost := float64(usage.PromptTokens) / 1_000_000 * p.promptCostPer1M
	completionCost := float64(usage.CompletionTokens) / 1_000_000 * p.completionCostPer1M
	return promptCost + completionCost
}

func trimBody(b []byte, max int) string {
	s := string(b)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
