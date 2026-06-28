// Package agent defines the agentic tool loop: a model that can request
// tool operations (Read, Write, Edit, Bash, Grep, Glob) and the Run loop
// that executes them within a workspace-confined sandbox until the model
// produces a final text response or a turn cap is hit.
//
// The verifier does NOT use this package; it stays single-shot via
// model.Verifier. The agentic loop is the implementer's engine (S06).
//
// No logging of message history, file contents, or tool outputs — per
// AGENTS.md Security. The message history may contain sensitive workspace
// data, and the same discipline as internal/model applies here.
package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/model"
)

// ErrMaxTurns is returned by Run when the agent loop exhausts its turn cap
// without producing a terminal text response. Callers (the scheduler worker)
// detect it via errors.Is to escalate to a PAGE event rather than treating
// it as a generic failure.
var ErrMaxTurns = errors.New("agent: max turns exhausted")

// MaxTurnsSentinel is the substring the worker/router checks for in error
// messages returned by RunSlice to detect max-turns exhaustion without
// importing the run package (which would create an import cycle).
const MaxTurnsSentinel = "RunSlice: max turns exhausted:"// Agent is a model that can carry a multi-turn conversation with tool calls.
// The model.Verifier interface (single-shot) is separate; the implementer
// engine (S06) consumes Agent.
type Agent interface {
	// Chat sends the full message history plus tool definitions to the model.
	// The returned ChatResponse may contain text content or tool_calls.
	Chat(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error)
}

// Config controls the Run loop behaviour.
type Config struct {
	// MaxTurns is the maximum number of model turns before forced termination.
	// Default 25 if <= 0.
	MaxTurns int

	// MaxOutputBytes is the maximum stdout/stderr captured from a Bash command,
	// and the maximum file content returned from a Read tool. Content beyond
	// the cap is truncated with a marker so the model knows output was capped.
	// Default 100_000 (100KB) if <= 0.
	MaxOutputBytes int
}

const (
	defaultMaxTurns       = 25
	defaultMaxOutputBytes = 100_000
)

// Message is one turn's message in the conversation history. It is the agent
// package's own type so callers don't hand-craft model.ChatMessage fields.
type Message struct {
	Role    string // "system", "user", "assistant", "tool"
	Content string
	// ToolCallID identifies which tool call this message responds to (role=tool).
	ToolCallID string
	// ToolCalls are tool invocations the model requested (role=assistant).
	ToolCalls []ToolCall
}

// ToolCall is a single tool invocation the model requested.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON-encoded arguments
}

// Run drives the agentic loop: send the prompt, execute tool calls, feed
// results back, repeat until the model produces text or the turn cap is hit.
//
// Returns the final text response, the total cost, and the full message
// history (useful for the implementer's proof bundle).
func Run(ctx context.Context, a Agent, systemPrompt, userPrompt string, workspaceRoot string, cfg Config) (string, float64, []Message, error) {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = defaultMaxTurns
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = defaultMaxOutputBytes
	}

	tools := allToolDefs()
	exec := &executor{
		root:      workspaceRoot,
		maxOutput: cfg.MaxOutputBytes,
	}

	// Build initial messages
	history := []model.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	var agentMessages []Message
	agentMessages = append(agentMessages, Message{Role: "system", Content: systemPrompt})
	agentMessages = append(agentMessages, Message{Role: "user", Content: userPrompt})

	var totalCost float64

	for turn := 0; turn < cfg.MaxTurns; turn++ {
		resp, err := a.Chat(ctx, history, tools)
		if err != nil {
			return "", totalCost, agentMessages, fmt.Errorf("agent: turn %d: %w", turn, err)
		}
		if len(resp.Choices) == 0 {
			return "", totalCost, agentMessages, fmt.Errorf("agent: turn %d: empty choices", turn)
		}

		choice := resp.Choices[0]
		totalCost += computeCost(resp.Usage)

		msg := choice.Message

		// If the model produced no tool calls, the turn is terminal.
		// Content may be empty (e.g., gpt-oss-class reasoning models stop
		// silently after finishing tool work). We return the accumulated
		// content; downstream verification judges the actual diff/tests, not
		// the agent's prose.
		if len(msg.ToolCalls) == 0 {
			history = append(history, model.ChatMessage{
				Role:    "assistant",
				Content: msg.Content,
			})
			agentMessages = append(agentMessages, Message{
				Role:    "assistant",
				Content: msg.Content,
			})
			return msg.Content, totalCost, agentMessages, nil
		}

		// If the model requested tool calls, execute them and continue.
		var agentTCs []ToolCall
		var modelTCs []model.ToolCall
		for _, tc := range msg.ToolCalls {
			agentTCs = append(agentTCs, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
			modelTCs = append(modelTCs, tc)
		}
		history = append(history, model.ChatMessage{
			Role:      "assistant",
			Content:   msg.Content,
			ToolCalls: modelTCs,
		})
		agentMessages = append(agentMessages, Message{
			Role:      "assistant",
			Content:   msg.Content,
			ToolCalls: agentTCs,
		})

		// Execute each tool call and append results
		for _, tc := range msg.ToolCalls {
			result := exec.run(tc.Function.Name, tc.Function.Arguments)
			tcID := tc.ID // capture loop variable
			history = append(history, model.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: &tcID,
			})
			agentMessages = append(agentMessages, Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", totalCost, agentMessages, fmt.Errorf("%w: turn cap (%d) reached with no text response", ErrMaxTurns, cfg.MaxTurns)}

// computeCost is a local passthrough for testability. FakeAgent usage is nil
// (cost 0). Real Chat responses include usage. For accurate model-specific
// pricing, the model package's pricing table is authoritative (S10).
func computeCost(usage *model.UsageBlock) float64 {
	if usage == nil {
		return 0
	}
	// Nominal cost estimate (~$2/1M tokens). Real model prices are in
	// model.modelPricing. S10 (benchmark) will make this data-driven.
	return float64(usage.TotalTokens) * 0.000002 // ~$2/1M tokens
}
