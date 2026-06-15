package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OAI dispatches a single /chat/completions call to an OpenAI-compatible
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

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse contains only the fields SwornAgent needs. Other fields from
// the provider's response are silently ignored (normalisation per Risk #1).
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *usageBlock `json:"usage"`
}

type usageBlock struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
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
}

// Verify sends the system prompt + user payload to /chat/completions.
// On any HTTP error, timeout, or unparseable response it returns an error
// (not a panic) — the caller (verify.Run) maps errors to BLOCKED, fulfilling
// spec AC4.
func (c *OAI) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error) {
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
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
		return "", 0, fmt.Errorf("model: HTTP %d: %s", resp.StatusCode, trimBody(body, 200))
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", 0, fmt.Errorf("model: unmarshal response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", 0, fmt.Errorf("model: empty choices in response")
	}

	cost := computeCost(c.Model, cr.Usage)
	return cr.Choices[0].Message.Content, cost, nil
}

func computeCost(model string, usage *usageBlock) float64 {
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
