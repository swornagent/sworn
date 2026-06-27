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

// Capabilities returns CapVerify | CapChat — the Anthropic driver supports both
// single-shot verification and multi-turn chat (S10-agentic-chat-anthropic).
func (a *Anthropic) Capabilities() Capability { return CapVerify | CapChat }

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
		// Fallback: a non-HTTP error (DNS failure, TLS handshake, connection
		// refused, etc.). This error is not a *model.Error — IsTransient in
		// errors.go returns true for unknown error types (see IsTransient:
		// "unknown errors are assumed transient"), so the caller's retry
		// policy will treat this as transient and retry. We preserve the
		// original error message rather than wrapping with NewProviderError.
		return "", 0, fmt.Errorf("model: anthropic dispatch: %w", err)
	}

	// Extract the first text block. SwornAgent uses single-shot verify calls
	// (no tools); the only content we care about is type "text".
	for _, block := range msg.Content {
		if block.Type == "text" {
			cost := ComputeCost(a.Model, int(msg.Usage.InputTokens), int(msg.Usage.OutputTokens))
			return block.Text, cost, nil
		}
	}
	return "", 0, fmt.Errorf("model: no text content in Anthropic response")
}

// Chat sends a multi-message conversation to the Anthropic Messages API.
// System messages are extracted and sent via the System parameter; user and
// assistant messages are mapped to the Messages array. Tool definitions are
// accepted for interface compatibility but not passed to the API (Anthropic
// tool-use is deferred — see S10 spec out-of-scope).
//
// The returned ChatResponse carries the first text block as content, actual
// token counts in Usage.InputTokens / Usage.OutputTokens, and a computed
// CostUSD from the Pricing table (not always 0).
func (a *Anthropic) Chat(ctx context.Context, messages []ChatMessage, tools []ToolDef) (*ChatResponse, error) {
	// Separate system messages from user/assistant messages.
	var systemBlocks []anthropic.TextBlockParam
	var msgParams []anthropic.MessageParam

	for _, m := range messages {
		switch m.Role {
		case "system":
			systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: m.Content})
		case "user":
			msgParams = append(msgParams,
				anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			msgParams = append(msgParams,
				anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
			// Other roles (tool, etc.) are silently skipped — Anthropic
			// tool-use is deferred (S10 out-of-scope).
		}
	}

	msg, err := a.Client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.Model),
		MaxTokens: a.MaxTokens,
		System:    systemBlocks,
		Messages:  msgParams,
	})
	if err != nil {
		if code, ok := anthropicStatusCode(err); ok {
			return nil, NewProviderError(code, "anthropic", a.Model, nil)
		}
		return nil, fmt.Errorf("model: anthropic chat dispatch: %w", err)
	}

	// Extract the first text block.
	for _, block := range msg.Content {
		if block.Type == "text" {
			inputTokens := int(msg.Usage.InputTokens)
			outputTokens := int(msg.Usage.OutputTokens)
			return &ChatResponse{
				Choices: []struct {
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
						}{Content: block.Text},
						FinishReason: string(msg.StopReason),
					},
				},
				Usage: &UsageBlock{
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
					TotalTokens:  inputTokens + outputTokens,
				},
				CostUSD: ComputeCost(a.Model, inputTokens, outputTokens),
			}, nil
		}
	}
	return nil, fmt.Errorf("model: no text content in Anthropic chat response")
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
