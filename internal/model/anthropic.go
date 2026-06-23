package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic dispatches verification calls to the Anthropic Messages API
// using the official anthropic-sdk-go (v1.51.1). It implements Verifier.
//
// OAI-import segregation (Pin 1): this file imports only the Anthropic SDK
// types — never internal/model/oai.go or any OAI struct types. The two
// drivers share the model.Error taxonomy via this package but have zero
// import overlap.
type Anthropic struct {
	Client    *anthropic.Client
	Model     string
	MaxTokens int64
}

// NewAnthropic constructs an Anthropic driver. apiKey must be non-empty.
// The SDK client is initialised with the explicit key (option.WithAPIKey)
// so it does not fall through to the env-var credential chain.
func NewAnthropic(modelID, apiKey string) (*Anthropic, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("model: missing Anthropic API key")
	}
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Anthropic{
		Client:    &client,
		Model:     modelID,
		MaxTokens: 8192,
	}, nil
}

// Verify sends the system prompt as a system message and userPayload as a
// single user turn to the Anthropic Messages API. It returns the text from
// the first text content block, the compute cost in USD, or an error.
func (a *Anthropic) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error) {
	msg, err := a.Client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.Model),
		MaxTokens: a.MaxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPayload)),
		},
	})
	if err != nil {
		// The anthropic-sdk-go returns *apierror.Error (internal package) on
		// HTTP errors. We cannot import that package directly, so we extract
		// the HTTP status code from the formatted error string, then route
		// through NewProviderError so the caller's ClassifyHTTP / IsTerminal
		// / IsTransient logic works unchanged.
		if code, ok := anthropicStatusCode(err); ok {
			return "", 0, NewProviderError(code, "anthropic", a.Model, nil)
		}
		return "", 0, fmt.Errorf("model: anthropic dispatch: %w", err)
	}

	// Extract the first text block. SwornAgent uses single-shot verify calls
	// (no tools); the only content we care about is type "text".
	for _, block := range msg.Content {
		if block.Type == "text" {
			cost := computeAnthropicCost(a.Model, msg.Usage)
			return block.Text, cost, nil
		}
	}
	return "", 0, fmt.Errorf("model: no text content in Anthropic response")
}

// anthropicStatusCode extracts the HTTP status code from an anthropic-sdk-go
// error. The SDK's internal *apierror.Error formats as:
//
//	'<METHOD> "<URL>": <CODE> <TEXT> [(Request-ID: <ID>)] <JSON>'
//
// We find the `": ` token, then parse the three-digit status code that
// follows. Returns (0, false) when the string does not match this shape.
func anthropicStatusCode(err error) (int, bool) {
	s := err.Error()
	idx := strings.Index(s, `": `)
	if idx < 0 {
		return 0, false
	}
	rest := s[idx+3:]
	end := strings.IndexByte(rest, ' ')
	if end < 0 || end > 5 { // status code is 3 digits + optional
		return 0, false
	}
	code, err2 := strconv.Atoi(rest[:end])
	if err2 != nil {
		return 0, false
	}
	return code, true
}

// anthropicPricing maps model IDs to USD per 1M tokens.
// Prices sourced from Anthropic's public pricing page (2026-06-23 snapshot).
// Unknown claude-* models get zero cost (same posture as OAI).
var anthropicPricing = map[string]struct {
	inputPricePer1M  float64
	outputPricePer1M float64
}{
	"claude-opus-4-8":   {15.00, 75.00},
	"claude-sonnet-4-6": {3.00, 15.00},
	"claude-haiku-4-5":  {1.00, 5.00},
}

// computeAnthropicCost returns the USD cost for a verify call from token
// counts. Returns 0 for unknown models (the caller still received a verdict).
func computeAnthropicCost(model string, usage anthropic.Usage) float64 {
	p, ok := anthropicPricing[model]
	if !ok {
		return 0
	}
	inputCost := float64(usage.InputTokens) / 1_000_000 * p.inputPricePer1M
	outputCost := float64(usage.OutputTokens) / 1_000_000 * p.outputPricePer1M
	return inputCost + outputCost
}
