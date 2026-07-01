package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestChatMessageAlwaysEmitsContent is the S27 reachability test for the
// universal agent-loop serialization bug: when an assistant turn carries only
// tool_calls and no prose (Content == ""), the request must still include the
// "content" field. With `json:"content,omitempty"` the field was dropped, and
// providers rejected it — DeepSeek: "missing field content"; OpenAI: "content:
// expected a string, got null" — killing every multi-turn implement session.
func TestChatMessageAlwaysEmitsContent(t *testing.T) {
	// An assistant tool-call turn with empty text content — the exact shape
	// that broke the loop.
	msg := ChatMessage{
		Role:    "assistant",
		Content: "",
		ToolCalls: []ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: FunctionCall{
				Name:      "read_file",
				Arguments: `{"path":"x.go"}`,
			},
		}},
	}

	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal ChatMessage: %v", err)
	}
	got := string(b)

	// The field must be present and must be an empty string (not omitted, not null).
	if !strings.Contains(got, `"content":""`) {
		t.Fatalf("assistant tool-call turn dropped the content field — providers reject this.\n got: %s", got)
	}

	// A plain assistant text turn must still round-trip its content.
	textMsg := ChatMessage{Role: "assistant", Content: "done"}
	tb, err := json.Marshal(textMsg)
	if err != nil {
		t.Fatalf("marshal text ChatMessage: %v", err)
	}
	if !strings.Contains(string(tb), `"content":"done"`) {
		t.Fatalf("text turn lost its content: %s", string(tb))
	}
}
