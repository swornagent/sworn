package model

import "testing"

// TestReasoningCapableModel covers the family classification used to decide
// whether to send the Responses `reasoning` parameter.
func TestReasoningCapableModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"gpt-4o", false},
		{"gpt-4.1", false},
		{"gpt-4o-2024-08-06", false},
		{"openai/gpt-4o", false},
		{"gpt-5.6", true},
		{"gpt-5.5-pro", true},
		{"openai/gpt-5.6", true},
		{"o3", true},
		{"o1-mini", true},
		{"o4-mini", true},
		{"openai/o3", true},
	}
	for _, c := range cases {
		if got := reasoningCapableModel(c.model); got != c.want {
			t.Errorf("reasoningCapableModel(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

// TestBuildRequest_OmitsReasoningForChatModels is the finding-H regression
// (2026-07-13 dogfood): sending reasoning.effort to gpt-4o — the default
// escalation model — returned a 400 "Unsupported parameter: 'reasoning.effort'"
// and killed the escalation chain at turn 0. buildRequest must omit the
// reasoning field for chat models and keep it for reasoning models.
//
// Mutation proof: make buildRequest set Reasoning unconditionally and the
// gpt-4o assertion below goes red.
func TestBuildRequest_OmitsReasoningForChatModels(t *testing.T) {
	chat := &OpenAIResponses{Model: "gpt-4o", ReasoningEffort: "medium"}
	if req := chat.buildRequest("inst", nil, nil); req.Reasoning != nil {
		t.Errorf("gpt-4o: expected reasoning omitted, got %+v", req.Reasoning)
	}

	reasoning := &OpenAIResponses{Model: "gpt-5.6", ReasoningEffort: "medium"}
	if req := reasoning.buildRequest("inst", nil, nil); req.Reasoning == nil {
		t.Error("gpt-5.6: expected reasoning included, got nil")
	} else if req.Reasoning.Effort != "medium" {
		t.Errorf("gpt-5.6: reasoning effort = %q, want medium", req.Reasoning.Effort)
	}
}
