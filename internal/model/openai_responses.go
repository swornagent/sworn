package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/account"
)

// OpenAIResponses dispatches verification and agent calls to OpenAI's
// /v1/responses endpoint (reasoning + tools + built-in web_search).
// It implements both Verifier (single-shot verify) and the agent.Agent
// interface (multi-turn Chat with tool calls).
//
// ChatMessage → responses input mapping (design decision D1):
// - system messages (first only) → top-level "instructions" field
// - user/assistant messages → "input" items with role+content
// - tool calls → "function_call" input items
// - tool results → "function_call_output" input items
//
// Normalisation strategy: json.Decode into a struct with only the fields
// SwornAgent needs. Unknown fields in the response are silently ignored.
// Missing or unparseable response is an error.
//
// No logging of API keys, request bodies, or response payloads — per
// AGENTS.md Security.
type OpenAIResponses struct {
	BaseURL         string // e.g. https://api.openai.com/v1
	Model           string // e.g. gpt-5.5
	APIKey          string
	Client          *http.Client // nil means http.DefaultClient
	ReasoningEffort string       // "low", "medium", "high" (default "medium")
	UseWebSearch    bool         // include built-in web_search tool
}

// Capabilities returns CapVerify | CapChat — the OpenAIResponses driver
// supports both single-shot verification and multi-turn chat via /v1/responses.
func (o *OpenAIResponses) Capabilities() Capability { return CapVerify | CapChat }

// NewOpenAIResponses constructs an OpenAIResponses driver.
// apiKey must be non-empty. ReasoningEffort defaults to "medium" if empty.
// UseWebSearch defaults to false unless SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH
// is set.
func NewOpenAIResponses(modelID, apiKey string) (*OpenAIResponses, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("model: missing OpenAI API key for responses provider")
	}
	effort := os.Getenv("SWORN_OPENAI_RESPONSES_REASONING_EFFORT")
	if effort == "" {
		effort = "medium"
	}
	useWebSearch := os.Getenv("SWORN_OPENAI_RESPONSES_USE_WEB_SEARCH") == "1"
	return &OpenAIResponses{
		BaseURL:         "https://api.openai.com/v1",
		Model:           modelID,
		APIKey:          apiKey,
		ReasoningEffort: effort,
		UseWebSearch:    useWebSearch,
	}, nil
}

// ---------------------------------------------------------------------------
// Types for the /v1/responses request/response shapes
// ---------------------------------------------------------------------------

// responsesRequest is the top-level request body for POST /v1/responses.
type responsesRequest struct {
	Model        string              `json:"model"`
	Input        []responsesInput    `json:"input"`
	Instructions string              `json:"instructions,omitempty"`
	Reasoning    *reasoningConfig    `json:"reasoning,omitempty"`
	Tools        []responsesToolItem `json:"tools,omitempty"`
}

type responsesInput struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Type    string `json:"type,omitempty"`
	// For function_call items
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// For function_call_output items
	Output string `json:"output,omitempty"`
}

type reasoningConfig struct {
	Effort string `json:"effort"`
}

// responsesToolItem can represent either a function tool or a built-in tool.
type responsesToolItem struct {
	Type      string            `json:"type"`
	Function  *ToolFunction     `json:"function,omitempty"`
	Name      string            `json:"name,omitempty"`
	WebSearch *webSearchPreview `json:"web_search_preview,omitempty"`
}

type webSearchPreview struct{}

// responsesOutput is a single output item from the /v1/responses response.
type responsesOutput struct {
	Type      string                 `json:"type"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Role      string                 `json:"role,omitempty"`
	Content   []responsesContentItem `json:"content,omitempty"`
}

type responsesContentItem struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

// responsesAPIResponse is the top-level response from POST /v1/responses.
type responsesAPIResponse struct {
	ID     string            `json:"id"`
	Output []responsesOutput `json:"output"`
	Usage  *responsesUsage   `json:"usage"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ---------------------------------------------------------------------------
// Verifier interface implementation (single-shot verify call)
// ---------------------------------------------------------------------------

// Verify sends the system prompt + user payload to /v1/responses.
// On any HTTP error, timeout, or unparseable response it returns an error
// (not a panic) — the caller (verify.Run) maps errors to BLOCKED.
func (c *OpenAIResponses) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error) {
	input := []responsesInput{}
	if userPayload != "" {
		input = append(input, responsesInput{Role: "user", Content: userPayload})
	}

	reqBody := c.buildRequest(systemPrompt, input, nil)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqBody); err != nil {
		return "", 0, fmt.Errorf("model: marshal responses request: %w", err)
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", 0, fmt.Errorf("model: build responses request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("model: responses dispatch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("model: read responses response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		me := NewProviderError(resp.StatusCode, "openai-responses", c.Model, body)
		if resp.StatusCode == http.StatusPaymentRequired {
			me.Err = account.ErrInsufficientCredits
		}
		return "", 0, me
	}

	var ar responsesAPIResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return "", 0, fmt.Errorf("model: unmarshal responses response: %w", err)
	}

	text := extractOutputText(ar.Output)
	usage := convertUsage(ar.Usage)
	cost := computeCost(c.Model, usage)
	return text, cost, nil
}

// ---------------------------------------------------------------------------
// Chat interface implementation (multi-turn agent loop)
// ---------------------------------------------------------------------------

// Chat sends the full message history plus tool definitions to /v1/responses.
// Returns the full response including tool_calls for the agent loop.
func (c *OpenAIResponses) Chat(ctx context.Context, messages []ChatMessage, tools []ToolDef) (*ChatResponse, error) {
	instructions, input := convertMessages(messages)
	reqBody := c.buildRequest(instructions, input, tools)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqBody); err != nil {
		return nil, fmt.Errorf("model: marshal responses request: %w", err)
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("model: build responses request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("model: responses dispatch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("model: read responses response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		me := NewProviderError(resp.StatusCode, "openai-responses", c.Model, body)
		if resp.StatusCode == http.StatusPaymentRequired {
			me.Err = account.ErrInsufficientCredits
		}
		return nil, me
	}

	// Parse into responses shape, then convert to ChatResponse for the agent loop.
	var ar responsesAPIResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("model: unmarshal responses response: %w", err)
	}

	return convertToChatResponse(ar.Output, ar.Usage), nil
}

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

// buildRequest constructs the /v1/responses request body.
// Temperature is intentionally omitted (design decision D2).
// reasoning_effort is included when configured.
// Built-in web_search is included when UseWebSearch is true.
func (c *OpenAIResponses) buildRequest(instructions string, input []responsesInput, tools []ToolDef) responsesRequest {
	req := responsesRequest{
		Model:        c.Model,
		Input:        input,
		Instructions: instructions,
		Reasoning:    &reasoningConfig{Effort: c.ReasoningEffort},
	}

	// Convert ToolDefs to responses-format tool items.
	for _, td := range tools {
		funcName := td.Name
		funcDesc := td.Description
		funcParams := td.Parameters
		req.Tools = append(req.Tools, responsesToolItem{
			Type: "function",
			Name: funcName,
			Function: &ToolFunction{
				Name:        funcName,
				Description: funcDesc,
				Parameters:  funcParams,
			},
		})
	}

	// Add built-in web_search tool when opted in (AC3).
	if c.UseWebSearch {
		req.Tools = append(req.Tools, responsesToolItem{
			Type:      "web_search_preview",
			WebSearch: &webSearchPreview{},
		})
	}

	return req
}

// ---------------------------------------------------------------------------
// Message conversion (ChatMessage → responses input items)
// ---------------------------------------------------------------------------

// convertMessages splits ChatMessage history into instructions (first system
// message) and input items (all subsequent messages). This implements design
// decision D1.
func convertMessages(messages []ChatMessage) (instructions string, input []responsesInput) {
	seenSystem := false
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if !seenSystem {
				instructions = msg.Content
				seenSystem = true
			}
			// Subsequent system messages are dropped — /v1/responses only
			// supports a single instructions field.

		case "user":
			input = append(input, responsesInput{
				Role:    "user",
				Content: msg.Content,
			})

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Emit each tool call as a function_call input item.
				for _, tc := range msg.ToolCalls {
					input = append(input, responsesInput{
						Type:      "function_call",
						CallID:    tc.ID,
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					})
				}
			} else {
				input = append(input, responsesInput{
					Role:    "assistant",
					Content: msg.Content,
				})
			}

		case "tool":
			tcID := ""
			if msg.ToolCallID != nil {
				tcID = *msg.ToolCallID
			}
			input = append(input, responsesInput{
				Type:   "function_call_output",
				CallID: tcID,
				Output: msg.Content,
			})
		}
	}
	return instructions, input
}

// ---------------------------------------------------------------------------
// Response parsing (responses output items → ChatResponse)
// ---------------------------------------------------------------------------

// convertToChatResponse converts /v1/responses output items into the
// canonical ChatResponse shape that the agent loop expects.
func convertToChatResponse(output []responsesOutput, usage *responsesUsage) *ChatResponse {
	cr := &ChatResponse{
		Usage: convertUsage(usage),
	}

	var textContent string
	var toolCalls []ToolCall

	for _, item := range output {
		switch item.Type {
		case "message":
			// Final text output.
			for _, ci := range item.Content {
				if ci.Type == "output_text" {
					textContent = ci.Text
				}
			}

		case "function_call":
			toolCalls = append(toolCalls, ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})

			// reasoning, web_search_call, etc. are ignored —
			// they don't map to ChatResponse fields.
		}
	}

	// Build a single choice with the extracted content.
	cr.Choices = []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}{
		{
			Message: struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls,omitempty"`
			}{
				Content:   textContent,
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason(output),
		},
	}

	return cr
}

// extractOutputText returns the text from the final "message" output item,
// used by the Verify path (single-shot).
func extractOutputText(output []responsesOutput) string {
	for _, item := range output {
		if item.Type == "message" {
			for _, ci := range item.Content {
				if ci.Type == "output_text" {
					return ci.Text
				}
			}
		}
	}
	return ""
}

// finishReason maps the presence of output items to a finish_reason string.
func finishReason(output []responsesOutput) string {
	for _, item := range output {
		if item.Type == "message" {
			return "stop"
		}
		if item.Type == "function_call" {
			return "tool_calls"
		}
	}
	return "stop"
}

// convertUsage maps /v1/responses usage field names to the canonical UsageBlock
// (input_tokens → PromptTokens, output_tokens → CompletionTokens).
func convertUsage(u *responsesUsage) *UsageBlock {
	if u == nil {
		return nil
	}
	return &UsageBlock{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
	}
}
