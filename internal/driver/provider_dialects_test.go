package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// providerDialectFixture loads one recorded wire fixture under
// internal/driver/testdata/provider_dialects. These are the actual recorded
// exchanges the slice contract cites as acceptance evidence (captured live
// against the real endpoints on 2026-08-15/16), never shapes derived from
// documented formats.
func providerDialectFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(
		filepath.Join("testdata", "provider_dialects", name),
	)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// googleContainerFromResponse extracts the raw extra_content container bytes
// from a recorded Gemini chat-completions response, byte-for-byte.
func googleContainerFromResponse(t *testing.T, body []byte) json.RawMessage {
	t.Helper()
	var raw struct {
		Choices []struct {
			Message struct {
				ExtraContent json.RawMessage `json:"extra_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(body, &raw) != nil || len(raw.Choices) != 1 ||
		len(raw.Choices[0].Message.ExtraContent) == 0 {
		t.Fatal("invalid Gemini chat fixture: missing extra_content")
	}
	return append(
		json.RawMessage(nil),
		raw.Choices[0].Message.ExtraContent...,
	)
}

// googlePerCallContainerFromFixture extracts the raw extra_content container
// bytes of one tool call from a recorded Gemini chat-completions response,
// byte-for-byte (the per-call position fixture g4 defines).
func googlePerCallContainerFromFixture(
	t *testing.T,
	body []byte,
	index int,
) json.RawMessage {
	t.Helper()
	var raw struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					ExtraContent json.RawMessage `json:"extra_content"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(body, &raw) != nil || len(raw.Choices) != 1 ||
		index < 0 || index >= len(raw.Choices[0].Message.ToolCalls) ||
		len(raw.Choices[0].Message.ToolCalls[index].ExtraContent) == 0 {
		t.Fatal("invalid Gemini chat fixture: missing per-call extra_content")
	}
	return append(
		json.RawMessage(nil),
		raw.Choices[0].Message.ToolCalls[index].ExtraContent...,
	)
}

// grokUsageFromResponse extracts the raw usage block bytes from the recorded
// Grok responses response, byte-for-byte.
func grokUsageFromResponse(t *testing.T, body []byte) json.RawMessage {
	t.Helper()
	var raw struct {
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(body, &raw) != nil || len(raw.Usage) == 0 {
		t.Fatal("invalid Grok responses fixture: missing usage")
	}
	return append(json.RawMessage(nil), raw.Usage...)
}

// A1: the recorded gemini-3.7-flash exchange decodes, the opaque
// thought-signature container is retained, and the next request in the same
// invocation replays it byte-exact in the position the provider requires.
func TestGoogleChatDecodesRetainsAndReplaysThoughtSignature(t *testing.T) {
	t.Parallel()
	conversation, err := newOpenAIConversation(
		"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		"gemini-3.7-flash",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectGoogleChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()

	response := providerDialectFixture(
		t,
		"gemini_chat_response_thought_signature.json",
	)
	container := googleContainerFromResponse(t, response)

	turn, err := conversation.accept(response)
	if err != nil || !turn.Prose || len(turn.Calls) != 0 ||
		turn.Usage == nil || turn.Usage.InputTokens != 16 ||
		turn.Usage.OutputTokens != 2 {
		t.Fatalf("Gemini Google-chat turn = %#v, %v", turn, err)
	}
	if len(conversation.messages) != 2 ||
		!bytes.Equal(conversation.messages[1].ExtraContent, container) {
		t.Fatalf(
			"retained extra_content = %q, want %q",
			conversation.messages[1].ExtraContent,
			container,
		)
	}

	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var replay struct {
		Messages []struct {
			ExtraContent json.RawMessage `json:"extra_content"`
		} `json:"messages"`
	}
	if json.Unmarshal(request.Body, &replay) != nil || len(replay.Messages) != 2 ||
		!bytes.Equal(replay.Messages[1].ExtraContent, container) {
		t.Fatalf(
			"replayed request = %s, container = %q",
			request.Body,
			replay.Messages[1].ExtraContent,
		)
	}

	// The recorded second-turn request carries the same container at the same
	// assistant-message position. The recordings differ only in JSON
	// whitespace (FIXTURE 1 compact, FIXTURE 2 spaced); the byte-exact replay
	// guarantee is against the exact bytes the conversation received, so the
	// fixture comparison is semantic while the replay comparison above is
	// byte-for-byte.
	recordedRequest := providerDialectFixture(
		t,
		"gemini_chat_request_replayed_signature.json",
	)
	var recorded struct {
		Messages []struct {
			ExtraContent json.RawMessage `json:"extra_content"`
		} `json:"messages"`
	}
	if json.Unmarshal(recordedRequest, &recorded) != nil ||
		len(recorded.Messages) != 3 {
		t.Fatal("invalid recorded Gemini request fixture")
	}
	var recordedContainer, replayedContainer any
	if json.Unmarshal(recorded.Messages[1].ExtraContent, &recordedContainer) != nil ||
		json.Unmarshal(container, &replayedContainer) != nil {
		t.Fatal("container fixtures are not JSON")
	}
	containerBody, _ := json.Marshal(replayedContainer)
	recordedBody, _ := json.Marshal(recordedContainer)
	if !bytes.Equal(containerBody, recordedBody) {
		t.Fatalf("recorded replay mismatch: %q vs %q", recordedBody, containerBody)
	}

	// Replay persists on every subsequent turn (prose nudge included).
	if err := conversation.appendInstruction([]byte(providerProseNudge)); err != nil {
		t.Fatal(err)
	}
	request, err = conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var nudged struct {
		Messages []struct {
			ExtraContent json.RawMessage `json:"extra_content"`
		} `json:"messages"`
	}
	if json.Unmarshal(request.Body, &nudged) != nil || len(nudged.Messages) != 3 ||
		!bytes.Equal(nudged.Messages[1].ExtraContent, container) {
		t.Fatalf("post-nudge replay = %s", request.Body)
	}
}

// A1 also requires replay after tool-result turns and after resume; the
// recorded container bytes are exercised through a tool-calling turn to prove
// the position survives appendResults and resume unchanged.
func TestGoogleChatReplaysSignatureAfterToolResultsAndResume(t *testing.T) {
	t.Parallel()
	conversation, err := newOpenAIConversation(
		"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		"gemini-3.7-flash",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectGoogleChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()

	container := googleContainerFromResponse(
		t,
		providerDialectFixture(
			t,
			"gemini_chat_response_thought_signature.json",
		),
	)
	response := []byte(
		`{"choices":[{"message":{"role":"assistant","content":null,"extra_content":` +
			string(container) +
			`,"tool_calls":[{"id":"tool-1","type":"function","function":{"name":"Read","arguments":"{\"path\":\"/workspace/a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
	)
	turn, err := conversation.accept(response)
	if err != nil || len(turn.Calls) != 1 ||
		turn.Calls[0].ID != "tool-1" {
		t.Fatalf("tool-calling Google turn = %#v, %v", turn, err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "tool-1", Name: "Read", Content: []byte("file body"),
	}}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var afterResults struct {
		Messages []struct {
			ExtraContent json.RawMessage `json:"extra_content"`
		} `json:"messages"`
	}
	if json.Unmarshal(request.Body, &afterResults) != nil ||
		len(afterResults.Messages) != 3 ||
		!bytes.Equal(afterResults.Messages[1].ExtraContent, container) {
		t.Fatalf("post-tool-result replay = %s", request.Body)
	}
	if err := conversation.resume(
		[]byte(`{"invocation_id":"implementation"}`),
		toolDefinitions(ReadWrite),
	); err != nil {
		t.Fatal(err)
	}
	request, err = conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var resumed struct {
		Messages []struct {
			ExtraContent json.RawMessage `json:"extra_content"`
		} `json:"messages"`
	}
	if json.Unmarshal(request.Body, &resumed) != nil ||
		len(resumed.Messages) != 4 ||
		!bytes.Equal(resumed.Messages[1].ExtraContent, container) {
		t.Fatalf("post-resume replay = %s", request.Body)
	}
}

// A1+A2: the recorded g4 tool-call-only turn decodes under the Google dialect
// with the content field absent, the per-call thought-signature container is
// retained keyed to its call, and the following request replays it byte-exact
// inside the same tool call with content still absent.
func TestGoogleChatToolCallOnlyTurnDecodesAndReplaysPerCallSignature(t *testing.T) {
	t.Parallel()
	conversation, err := newOpenAIConversation(
		"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		"gemini-3.7-flash",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectGoogleChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()

	response := providerDialectFixture(
		t,
		"gemini_chat_response_tool_call_thought_signature.json",
	)
	container := googlePerCallContainerFromFixture(t, response, 0)

	turn, err := conversation.accept(response)
	if err != nil || turn.Prose || len(turn.Calls) != 1 ||
		turn.Calls[0].ID != "call_2835749" || turn.Calls[0].Name != "probe" ||
		turn.Usage == nil || turn.Usage.InputTokens != 47 ||
		turn.Usage.OutputTokens != 12 {
		t.Fatalf("Google tool-call-only turn = %#v, %v", turn, err)
	}
	if len(conversation.messages) != 2 {
		t.Fatalf("messages = %d", len(conversation.messages))
	}
	assistant := conversation.messages[1]
	if len(assistant.Content) != 0 {
		t.Fatalf("content must be absent, got %q", assistant.Content)
	}
	if len(assistant.ExtraContent) != 0 {
		t.Fatalf(
			"message-level extra_content must be absent on a tool-call turn, got %q",
			assistant.ExtraContent,
		)
	}
	if len(assistant.ToolCalls) != 1 ||
		!bytes.Equal(assistant.ToolCalls[0].ExtraContent, container) {
		t.Fatalf(
			"per-call retained = %q, want %q",
			assistant.ToolCalls[0].ExtraContent,
			container,
		)
	}

	// The replayed request is built after the tool result is appended, exactly
	// as the recorded g5 request carries the result beside the replay.
	if err := conversation.appendResults([]providerToolResult{{
		ID: "call_2835749", Name: "probe", Content: []byte("probe result: 49"),
	}}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var replay struct {
		Messages []struct {
			Content   json.RawMessage `json:"content"`
			ToolCalls []struct {
				ExtraContent json.RawMessage `json:"extra_content"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if json.Unmarshal(request.Body, &replay) != nil || len(replay.Messages) != 3 {
		t.Fatalf("replayed request = %s", request.Body)
	}
	if len(replay.Messages[1].Content) != 0 {
		t.Fatalf(
			"replayed assistant content = %q, want absent",
			replay.Messages[1].Content,
		)
	}
	if len(replay.Messages[1].ToolCalls) != 1 ||
		!bytes.Equal(
			replay.Messages[1].ToolCalls[0].ExtraContent,
			container,
		) {
		t.Fatalf(
			"replayed per-call = %q, want %q",
			replay.Messages[1].ToolCalls[0].ExtraContent,
			container,
		)
	}
}

// A1: content-optionality is dialect-scoped. The same content-absent
// tool-call-only fixture decodes under the Google dialect and fails exactly as
// it does today on strict chat.
func TestGoogleToolCallContentOptionalIsDialectScoped(t *testing.T) {
	t.Parallel()
	response := providerDialectFixture(
		t,
		"gemini_chat_response_tool_call_thought_signature.json",
	)
	newChat := func(t *testing.T, dialect providerDialect) *openAIConversation {
		t.Helper()
		conversation, err := newOpenAIConversation(
			"https://provider.example.invalid/v1/chat/completions",
			"gemini-3.7-flash",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			dialect,
			"",
		)
		if err != nil {
			t.Fatal(err)
		}
		return conversation
	}

	t.Run("strict chat still requires content", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectOpenAIChat)
		defer conversation.close()
		if _, err := conversation.accept(response); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("strict chat error = %v", err)
		}
	})
	t.Run("google chat decodes the content-absent turn", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectGoogleChat)
		defer conversation.close()
		if _, err := conversation.accept(response); err != nil {
			t.Fatalf("google chat error = %v", err)
		}
	})
}

// A1 invariant guard: under the Google dialect content-optionality is scoped
// to the recorded tool-call-only turn, so an assistant message with neither
// content nor tool calls still fails CONTINUATION_INVALID exactly as it does
// today on every other dialect.
func TestGoogleChatEmptyAssistantMessageFails(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"content absent":                `{"choices":[{"message":{"role":"assistant"},"finish_reason":"stop"}]}`,
		"content null":                  `{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}]}`,
		"content absent with signature": `{"choices":[{"message":{"role":"assistant","extra_content":{"google":{"thought_signature":"sig"}}},"finish_reason":"stop"}]}`,
		"content null with signature":   `{"choices":[{"message":{"role":"assistant","content":null,"extra_content":{"google":{"thought_signature":"sig"}}},"finish_reason":"stop"}]}`,
	}
	for name, body := range cases {
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			conversation, err := newOpenAIConversation(
				"https://provider.example.invalid/v1/chat/completions",
				"gemini-3.7-flash",
				toolDefinitions(ReadOnly),
				[]byte(`{}`),
				providerDialectGoogleChat,
				"",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer conversation.close()
			if _, err := conversation.accept([]byte(body)); !IsCode(
				err,
				"CONTINUATION_INVALID",
			) {
				t.Fatalf("%s error = %v", name, err)
			}
		})
	}
}

// A2: the recorded g5 request replays the g4 assistant message byte-exact
// (per-call container in place, content absent) beside the tool result, and
// the recorded g5 response - the provider's coherent continuation - decodes as
// a prose turn carrying the message-level signature.
func TestGoogleChatPerCallSignatureRoundTripMatchesRecordedExchange(t *testing.T) {
	t.Parallel()
	conversation, err := newOpenAIConversation(
		"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		"gemini-3.7-flash",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectGoogleChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()

	g4 := providerDialectFixture(
		t,
		"gemini_chat_response_tool_call_thought_signature.json",
	)
	if _, err := conversation.accept(g4); err != nil {
		t.Fatal(err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "call_2835749", Name: "probe", Content: []byte("probe result: 49"),
	}}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}

	g5Request := providerDialectFixture(
		t,
		"gemini_chat_request_replayed_tool_call_signature.json",
	)
	var built, recorded struct {
		Messages []struct {
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			ToolCalls []struct {
				ID           string          `json:"id"`
				ExtraContent json.RawMessage `json:"extra_content"`
				Function     struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if json.Unmarshal(request.Body, &built) != nil ||
		json.Unmarshal(g5Request, &recorded) != nil {
		t.Fatal("invalid built or recorded request")
	}
	if len(built.Messages) != 3 || len(recorded.Messages) != 3 {
		t.Fatalf(
			"built messages = %d, recorded messages = %d",
			len(built.Messages),
			len(recorded.Messages),
		)
	}
	builtAssistant := built.Messages[1]
	recordedAssistant := recorded.Messages[1]
	if builtAssistant.Role != "assistant" ||
		recordedAssistant.Role != "assistant" {
		t.Fatalf("assistant messages = %#v / %#v", builtAssistant, recordedAssistant)
	}
	if len(builtAssistant.Content) != 0 || len(recordedAssistant.Content) != 0 {
		t.Fatalf(
			"content must be absent on the tool-call turn: built %q recorded %q",
			builtAssistant.Content,
			recordedAssistant.Content,
		)
	}
	if len(builtAssistant.ToolCalls) != 1 ||
		len(recordedAssistant.ToolCalls) != 1 ||
		!bytes.Equal(
			builtAssistant.ToolCalls[0].ExtraContent,
			recordedAssistant.ToolCalls[0].ExtraContent,
		) {
		t.Fatalf(
			"per-call replay mismatch: built %q recorded %q",
			builtAssistant.ToolCalls[0].ExtraContent,
			recordedAssistant.ToolCalls[0].ExtraContent,
		)
	}

	// The recorded g5 response - the provider continuing coherently after the
	// replayed per-call shape - decodes as a prose turn with the message-level
	// signature retained.
	continuation, err := newOpenAIConversation(
		"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		"gemini-3.7-flash",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectGoogleChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.close()
	g5Response := providerDialectFixture(
		t,
		"gemini_chat_response_replayed_tool_call_signature.json",
	)
	turn, err := continuation.accept(g5Response)
	if err != nil || !turn.Prose || len(turn.Calls) != 0 ||
		turn.Usage == nil || turn.Usage.InputTokens != 104 ||
		turn.Usage.OutputTokens != 22 {
		t.Fatalf("g5 continuation turn = %#v, %v", turn, err)
	}
	if len(continuation.messages) != 2 ||
		len(continuation.messages[1].Content) == 0 ||
		len(continuation.messages[1].ExtraContent) == 0 {
		t.Fatalf(
			"g5 continuation assistant = %#v",
			continuation.messages[1],
		)
	}
}

// A3: per-call replay is mandatory and fails closed. A per-call container
// that is structurally unplaceable or cannot be retained yields the labelled
// CONTINUATION_STATE_UNPLAYABLE code; there is no path that re-requests a tool
// call without its retained container.
func TestGoogleChatPerCallSignatureFailsClosed(t *testing.T) {
	t.Parallel()
	newConversation := func(t *testing.T) *openAIConversation {
		t.Helper()
		conversation, err := newOpenAIConversation(
			"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
			"gemini-3.7-flash",
			toolDefinitions(ReadOnly),
			[]byte(`{"prompt":"bounded"}`),
			providerDialectGoogleChat,
			"",
		)
		if err != nil {
			t.Fatal(err)
		}
		return conversation
	}

	t.Run("structurally unplaceable per-call containers", func(t *testing.T) {
		t.Parallel()
		cases := map[string]string{
			"not an object":        `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":"sig","id":"c1","type":"function","function":{"name":"probe","arguments":"{\"value\":7}"}}]}}]}`,
			"null container":       `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":null,"id":"c1","type":"function","function":{"name":"probe","arguments":"{\"value\":7}"}}]}}]}`,
			"empty container":      `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":{},"id":"c1","type":"function","function":{"name":"probe","arguments":"{\"value\":7}"}}]}}]}`,
			"unknown inner field":  `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":{"google":{"thought_signature":"sig","unknown_inner":true}},"id":"c1","type":"function","function":{"name":"probe","arguments":"{\"value\":7}"}}]}}]}`,
			"unknown vendor field": `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":{"other":{"thought_signature":"sig"}},"id":"c1","type":"function","function":{"name":"probe","arguments":"{\"value\":7}"}}]}}]}`,
			"missing signature":    `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":{"google":{}},"id":"c1","type":"function","function":{"name":"probe","arguments":"{\"value\":7}"}}]}}]}`,
			"empty signature":      `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":{"google":{"thought_signature":""}},"id":"c1","type":"function","function":{"name":"probe","arguments":"{\"value\":7}"}}]}}]}`,
			"non-string signature": `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":{"google":{"thought_signature":42}},"id":"c1","type":"function","function":{"name":"probe","arguments":"{\"value\":7}"}}]}}]}`,
		}
		for name, body := range cases {
			body := body
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				conversation := newConversation(t)
				defer conversation.close()
				if _, err := conversation.accept([]byte(body)); !IsCode(
					err,
					"CONTINUATION_STATE_UNPLAYABLE",
				) {
					t.Fatalf("%s error = %v", name, err)
				}
			})
		}
	})

	t.Run("over budget per-call step exhaustion", func(t *testing.T) {
		t.Parallel()
		conversation := newConversation(t)
		defer conversation.close()
		for step := 0; step < MaxContinuationSteps; step++ {
			if _, err := conversation.ledger.retain(opaqueField{
				kind: opaqueText,
				body: []byte("x"),
			}); err != nil {
				t.Fatalf("fill retain %d = %v", step, err)
			}
		}
		response := providerDialectFixture(
			t,
			"gemini_chat_response_tool_call_thought_signature.json",
		)
		if _, err := conversation.accept(response); !IsCode(
			err,
			"CONTINUATION_STATE_UNPLAYABLE",
		) {
			t.Fatalf("over-budget error = %v", err)
		}
	})
}

// A4: the per-call extra_content position is an explicit allowlist gated by
// dialect AND structural position; every other position stays closed and
// fails with the same error it fails with today.
func TestProviderDialectsRejectPerCallExtraContentOutsideTheNamedPosition(t *testing.T) {
	t.Parallel()
	newChat := func(t *testing.T, dialect providerDialect) *openAIConversation {
		t.Helper()
		conversation, err := newOpenAIConversation(
			"https://provider.example.invalid/v1/chat/completions",
			"model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			dialect,
			"",
		)
		if err != nil {
			t.Fatal(err)
		}
		return conversation
	}
	perCallContainer := `{"google":{"thought_signature":"sig"}}`

	t.Run("strict chat rejects per-call extra_content", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectOpenAIChat)
		defer conversation.close()
		body := `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"extra_content":` +
			perCallContainer +
			`,"id":"c1","type":"function","function":{"name":"Read","arguments":"{\"path\":\"/a\"}"}}]}}]}`
		if _, err := conversation.accept([]byte(body)); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("strict chat error = %v", err)
		}
	})
	t.Run("google rejects extra_content inside the function object", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectGoogleChat)
		defer conversation.close()
		body := `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"Read","arguments":"{}","extra_content":` +
			perCallContainer + `}}]}}]}`
		if _, err := conversation.accept([]byte(body)); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("google function-position error = %v", err)
		}
	})
	t.Run("google rejects extra_content on the choice object", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectGoogleChat)
		defer conversation.close()
		body := `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"Read","arguments":"{}"}}]},"extra_content":` +
			perCallContainer + `}]}`
		if _, err := conversation.accept([]byte(body)); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("google choice-position error = %v", err)
		}
	})
	t.Run("google rejects extra_content at the response root", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectGoogleChat)
		defer conversation.close()
		body := `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"Read","arguments":"{}"}}]}}],"extra_content":` +
			perCallContainer + `}`
		if _, err := conversation.accept([]byte(body)); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("google root-position error = %v", err)
		}
	})
	t.Run("per-call allowlist rejects an unknown sibling field", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectGoogleChat)
		defer conversation.close()
		body := `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"extra_content":` +
			perCallContainer +
			`,"unknown_sibling":1,"id":"c1","type":"function","function":{"name":"Read","arguments":"{}"}}]}}]}`
		if _, err := conversation.accept([]byte(body)); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("google unknown sibling error = %v", err)
		}
	})
	t.Run("responses surface rejects the chat per-call shape", func(t *testing.T) {
		t.Parallel()
		conversation, err := newResponsesConversation(
			"https://provider.example.invalid/v1/responses",
			"model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			"medium",
			nil,
			false,
			providerDialectOpenAIResponses,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()
		response := providerDialectFixture(
			t,
			"gemini_chat_response_tool_call_thought_signature.json",
		)
		if _, err := conversation.accept(response); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("responses surface error = %v", err)
		}
	})
}

// A3: nothing from a per-call container crosses the trust boundary either. A
// full Dispatcher invocation replays a canary per-call container on the wire,
// and the observation, submission, seal, and encoded result contain none of
// it; close() zeroes the per-call bytes.
func TestOpaquePerCallVendorStateNeverCrossesTrustBoundary(t *testing.T) {
	canary := "A5-PER-CALL-CANARY-OPAQUE-SIGNATURE-0123456789abcdef"
	perCall := json.RawMessage(`{"google":{"thought_signature":"` + canary + `"}}`)
	transport := &dialectCertRoundTripper{
		codec:            dialectCodecChat,
		vendor:           dialectVendorGoogle,
		perCallContainer: perCall,
	}
	trueFlag := true
	adapter, err := providerDialectAdapter(t, OpenAIProfileConfig{
		HTTPProfileConfig: HTTPProfileConfig{
			Key: "boundary-dialect", ID: "sworn.boundary.dialect",
			Version:  "1.0.0",
			Endpoint: "https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		},
		API:              OpenAIChatCompletionsAPI,
		ThoughtSignature: &trueFlag,
	}, transport)
	if err != nil {
		t.Fatal(err)
	}
	ref := "dialect-cert-credential"
	selected := SelectedProfile{
		Profile: ProfileConfig{
			Key: "boundary-dialect", Adapter: adapter.Identity().Key,
			Network: NetworkRequired, AuthMode: AuthModeBearer,
			CredentialRef: &ref,
		},
		Adapter: adapter.Identity(),
		Model:   "gemini-3.7-flash",
		adapter: adapter,
	}
	invocation := providerDialectCertificationInvocation(t, selected)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil ||
		observation.TransportStatus != Completed {
		t.Fatalf("boundary observation = %#v, %v", observation, err)
	}
	observationBody, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(observationBody, []byte(canary)) {
		t.Fatalf("canary escaped into observation: %s", observationBody)
	}
	if bytes.Contains(observation.Handoff.SubmissionBytes, []byte(canary)) {
		t.Fatalf(
			"canary escaped into submission: %s",
			observation.Handoff.SubmissionBytes,
		)
	}
	if bytes.Contains(observation.Handoff.SealBytes, []byte(canary)) {
		t.Fatalf("canary escaped into seal: %s", observation.Handoff.SealBytes)
	}
	var resultUsage *Usage
	if observation.Usage.InputTokens != nil {
		resultUsage = &Usage{
			InputTokens:  *observation.Usage.InputTokens,
			OutputTokens: *observation.Usage.OutputTokens,
		}
	}
	result, err := EncodeResult(Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    invocation.Request.InvocationID,
		AdapterID:       adapter.Identity().ID,
		AdapterVersion:  adapter.Identity().Version,
		ObservedModel:   selected.Model,
		DurationMillis:  1,
		TransportStatus: Completed,
		Usage:           resultUsage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result, []byte(canary)) {
		t.Fatalf("canary escaped into result: %s", result)
	}
	// The canary did reach the wire: it was replayed byte-exact inside the
	// tool call on the second request, proving per-call opaque retention
	// while staying out of every external surface.
	if len(transport.requestBodies) < 2 ||
		len(transport.replayedPerCall) != 1 ||
		!bytes.Equal(transport.replayedPerCall[0], perCall) {
		t.Fatalf(
			"canary per-call replay = %d requests, %v",
			len(transport.requestBodies),
			transport.replayedPerCall,
		)
	}

	// close() zeroes the retained per-call bytes alongside the rest of the
	// message state.
	conversation, err := newOpenAIConversation(
		"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		"gemini-3.7-flash",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectGoogleChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.accept([]byte(
		`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"extra_content":` +
			string(perCall) +
			`,"id":"c1","type":"function","function":{"name":"Read","arguments":"{}"}}]}}]}`,
	)); err != nil {
		t.Fatal(err)
	}
	retained := conversation.messages[1].ToolCalls[0].ExtraContent
	if !bytes.Equal(retained, perCall) {
		t.Fatalf("retained = %q", retained)
	}
	conversation.close()
	for index := range retained {
		if retained[index] != 0 {
			t.Fatalf("per-call byte %d not cleared after close", index)
		}
	}
}

// A2: replay is mandatory and fails closed. A container that is over budget
// or structurally unplaceable yields the labelled CONTINUATION_STATE_UNPLAYABLE
// code (mapped to certification_response_contract_failed); there is no path
// that re-requests without the retained signature.
func TestGoogleChatFailsClosedWhenSignatureCannotBeReplayed(t *testing.T) {
	t.Parallel()
	newConversation := func(t *testing.T) *openAIConversation {
		t.Helper()
		conversation, err := newOpenAIConversation(
			"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
			"gemini-3.7-flash",
			toolDefinitions(ReadOnly),
			[]byte(`{"prompt":"bounded"}`),
			providerDialectGoogleChat,
			"",
		)
		if err != nil {
			t.Fatal(err)
		}
		return conversation
	}

	t.Run("structurally unplaceable containers", func(t *testing.T) {
		t.Parallel()
		cases := map[string]string{
			"not an object":         `{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":"signature"},"finish_reason":"stop"}]}`,
			"null container":        `{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":null},"finish_reason":"stop"}]}`,
			"empty container":       `{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":{}},"finish_reason":"stop"}]}`,
			"unknown inner field":   `{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":{"google":{"thought_signature":"sig","unknown_inner":true}}},"finish_reason":"stop"}]}`,
			"unknown vendor field":  `{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":{"other":{"thought_signature":"sig"}}},"finish_reason":"stop"}]}`,
			"missing signature":     `{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":{"google":{}}},"finish_reason":"stop"}]}`,
			"empty signature":       `{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":{"google":{"thought_signature":""}}},"finish_reason":"stop"}]}`,
			"non-string signature":  `{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":{"google":{"thought_signature":42}}},"finish_reason":"stop"}]}`,
			"missing extra_content": `{"choices":[{"message":{"role":"assistant","content":"chosen."},"finish_reason":"stop"}]}`,
		}
		for name, body := range cases {
			body := body
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				conversation := newConversation(t)
				defer conversation.close()
				if _, err := conversation.accept([]byte(body)); !IsCode(
					err,
					"CONTINUATION_STATE_UNPLAYABLE",
				) {
					t.Fatalf("%s error = %v", name, err)
				}
			})
		}
	})

	t.Run("over budget step exhaustion", func(t *testing.T) {
		t.Parallel()
		conversation := newConversation(t)
		defer conversation.close()
		// The response-size cap keeps a single container within the opaque
		// field budget, so the over-budget surface is the ledger's step
		// budget: when the continuation ledger can no longer retain, the
		// dispatch fails closed instead of dropping the signature.
		for step := 0; step < MaxContinuationSteps; step++ {
			if _, err := conversation.ledger.retain(opaqueField{
				kind: opaqueText,
				body: []byte("x"),
			}); err != nil {
				t.Fatalf("fill retain %d = %v", step, err)
			}
		}
		response := providerDialectFixture(
			t,
			"gemini_chat_response_thought_signature.json",
		)
		if _, err := conversation.accept(response); !IsCode(
			err,
			"CONTINUATION_STATE_UNPLAYABLE",
		) {
			t.Fatalf("over-budget error = %v", err)
		}
	})
}

// A3: the recorded grok-4.6 usage decorations decode on both the responses
// and chat-completions surfaces, and the normalized accounting reads only the
// standard token fields.
func TestXAIUsageDecorationsDecodeAndStayOutOfAccounting(t *testing.T) {
	t.Parallel()
	recorded := providerDialectFixture(
		t,
		"grok_responses_response_usage.json",
	)

	t.Run("responses surface", func(t *testing.T) {
		t.Parallel()
		conversation, err := newResponsesConversation(
			"https://api.x.ai.example.invalid/v1/responses",
			"grok-4.6",
			toolDefinitions(ReadOnly),
			[]byte(`{"prompt":"bounded"}`),
			"high",
			nil,
			false,
			providerDialectXAIResponses,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()
		turn, err := conversation.accept(recorded)
		if err != nil || !turn.Prose || len(turn.Calls) != 0 ||
			turn.Usage == nil || turn.Usage.InputTokens != 209 ||
			turn.Usage.OutputTokens != 43 ||
			turn.Usage.CacheReadTokens == nil ||
			*turn.Usage.CacheReadTokens != 128 {
			t.Fatalf("Grok responses turn = %#v, %v", turn, err)
		}
		receipt, err := NormalizeUsage(turn.Usage, nil, "sworn.test")
		if err != nil || receipt.CostStatus != UsageUnavailable ||
			receipt.CostMicroUnits != nil || receipt.Source != nil {
			t.Fatalf("Grok accounting = %#v, %v", receipt, err)
		}
	})

	t.Run("chat surface", func(t *testing.T) {
		t.Parallel()
		conversation, err := newOpenAIConversation(
			"https://api.x.ai.example.invalid/v1/chat/completions",
			"grok-4.6",
			toolDefinitions(ReadOnly),
			[]byte(`{"prompt":"bounded"}`),
			providerDialectXAIChat,
			"",
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()
		// The recorded decoration names (num_sources_used,
		// num_server_side_tools_used, cost_in_usd_ticks, context_details)
		// appear verbatim; the chat surface reads its own standard token
		// vocabulary (prompt_tokens/completion_tokens + the OpenAI cache
		// pair), so the accounting still comes only from standard fields.
		chatUsage := json.RawMessage(
			`{"prompt_tokens":209,"completion_tokens":43,"total_tokens":252,"prompt_cache_hit_tokens":128,"num_sources_used":0,"num_server_side_tools_used":0,"cost_in_usd_ticks":4840000,"context_details":{"input_tokens":209,"output_tokens":43}}`,
		)
		response := []byte(
			`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"Read","arguments":"{\"path\":\"/workspace/a\"}"}}]},"finish_reason":"tool_calls"}],"usage":` +
				string(chatUsage) + `}`,
		)
		turn, err := conversation.accept(response)
		if err != nil || len(turn.Calls) != 1 ||
			turn.Usage == nil || turn.Usage.InputTokens != 209 ||
			turn.Usage.OutputTokens != 43 ||
			turn.Usage.CacheReadTokens == nil ||
			*turn.Usage.CacheReadTokens != 128 {
			t.Fatalf("Grok chat turn = %#v, %v", turn, err)
		}
		receipt, err := NormalizeUsage(turn.Usage, nil, "sworn.test")
		if err != nil || receipt.CostStatus != UsageUnavailable ||
			receipt.CostMicroUnits != nil || receipt.Source != nil {
			t.Fatalf("Grok chat accounting = %#v, %v", receipt, err)
		}
	})
}

// A4: every admission is gated by dialect AND structural position; the
// extension points are an explicit allowlist, never a global tolerance.
func TestProviderDialectsAreExplicitAllowlistsNotGlobalTolerance(t *testing.T) {
	t.Parallel()
	geminiResponse := providerDialectFixture(
		t,
		"gemini_chat_response_thought_signature.json",
	)
	grokResponse := providerDialectFixture(
		t,
		"grok_responses_response_usage.json",
	)
	// The chat usage block carries the recorded decoration names beside the
	// chat surface's own standard token vocabulary, so the strict-dialect
	// rejection below is specifically about the decorations, not a
	// vocabulary mismatch.
	chatUsage := json.RawMessage(
		`{"prompt_tokens":209,"completion_tokens":43,"total_tokens":252,"prompt_cache_hit_tokens":128,"num_sources_used":0,"num_server_side_tools_used":0,"cost_in_usd_ticks":4840000,"context_details":{"input_tokens":209,"output_tokens":43}}`,
	)
	chatUsageResponse := []byte(
		`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"Read","arguments":"{\"path\":\"/workspace/a\"}"}}]},"finish_reason":"tool_calls"}],"usage":` +
			string(chatUsage) + `}`,
	)

	newChat := func(t *testing.T, dialect providerDialect) *openAIConversation {
		t.Helper()
		conversation, err := newOpenAIConversation(
			"https://provider.example.invalid/v1/chat/completions",
			"model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			dialect,
			"",
		)
		if err != nil {
			t.Fatal(err)
		}
		return conversation
	}

	t.Run("strict chat rejects extra_content", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectOpenAIChat)
		defer conversation.close()
		if _, err := conversation.accept(geminiResponse); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("strict chat error = %v", err)
		}
	})
	t.Run("strict chat rejects vendor usage decorations", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectOpenAIChat)
		defer conversation.close()
		if _, err := conversation.accept(chatUsageResponse); !IsCode(
			err,
			"INVALID_USAGE",
		) {
			t.Fatalf("strict chat usage error = %v", err)
		}
	})
	t.Run("strict responses rejects vendor usage decorations", func(t *testing.T) {
		t.Parallel()
		conversation, err := newResponsesConversation(
			"https://provider.example.invalid/v1/responses",
			"model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			"medium",
			nil,
			false,
			providerDialectOpenAIResponses,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()
		if _, err := conversation.accept(grokResponse); !IsCode(
			err,
			"INVALID_USAGE",
		) {
			t.Fatalf("strict responses usage error = %v", err)
		}
	})
	t.Run("xai chat rejects usage beyond the named set", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectXAIChat)
		defer conversation.close()
		mutated := bytes.Replace(
			chatUsageResponse,
			[]byte(`"cost_in_usd_ticks":4840000`),
			[]byte(`"cost_in_usd_ticks":4840000,"unlisted_vendor_field":1`),
			1,
		)
		if _, err := conversation.accept(mutated); !IsCode(
			err,
			"INVALID_USAGE",
		) {
			t.Fatalf("xai chat unknown usage error = %v", err)
		}
	})
	t.Run("xai responses rejects usage beyond the named set", func(t *testing.T) {
		t.Parallel()
		conversation, err := newResponsesConversation(
			"https://provider.example.invalid/v1/responses",
			"model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			"medium",
			nil,
			false,
			providerDialectXAIResponses,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()
		mutated := bytes.Replace(
			grokResponse,
			[]byte(`"cost_in_usd_ticks":4840000`),
			[]byte(`"cost_in_usd_ticks":4840000,"unlisted_vendor_field":1`),
			1,
		)
		if _, err := conversation.accept(mutated); !IsCode(
			err,
			"INVALID_USAGE",
		) {
			t.Fatalf("xai responses unknown usage error = %v", err)
		}
	})
	t.Run("google chat rejects unknown outer message fields", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectGoogleChat)
		defer conversation.close()
		malformed := []byte(
			`{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":{"google":{"thought_signature":"sig"}},"unknown_outer_field":true},"finish_reason":"stop"}]}`,
		)
		if _, err := conversation.accept(malformed); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("google chat unknown outer error = %v", err)
		}
	})
	t.Run("google chat rejects unknown inner container fields", func(t *testing.T) {
		t.Parallel()
		conversation := newChat(t, providerDialectGoogleChat)
		defer conversation.close()
		malformed := []byte(
			`{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":{"google":{"thought_signature":"sig","unknown_inner":true}}},"finish_reason":"stop"}]}`,
		)
		if _, err := conversation.accept(malformed); !IsCode(
			err,
			"CONTINUATION_STATE_UNPLAYABLE",
		) {
			t.Fatalf("google chat unknown inner error = %v", err)
		}
	})
	t.Run("google fixture is not a responses extension", func(t *testing.T) {
		t.Parallel()
		conversation, err := newResponsesConversation(
			"https://provider.example.invalid/v1/responses",
			"model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			"medium",
			nil,
			false,
			providerDialectOpenAIResponses,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()
		if _, err := conversation.accept(geminiResponse); !IsCode(
			err,
			"CONTINUATION_INVALID",
		) {
			t.Fatalf("responses accepted google fixture: %v", err)
		}
	})
}

// dialectCodec distinguishes the two wire surfaces the dialects extend.
type dialectCodec int

const (
	dialectCodecChat dialectCodec = iota + 1
	dialectCodecResponses
)

// dialectVendor selects which recorded vendor dialect the fixture round
// tripper exercises on the wire.
type dialectVendor int

const (
	dialectVendorGoogle dialectVendor = iota + 1
	dialectVendorXAI
	// dialectVendorStrict serves the recorded fixture verbatim to a strict
	// adapter, pinning the certification failure code the provider produces
	// today.
	dialectVendorStrict
)

// dialectCertRoundTripper is a fixture-serving http.RoundTripper that scripts
// a certification conversation: turn one reads the certification input, turn
// two submits a valid implementer_design sworn_submit. The Google vendor
// carries the recorded extra_content container on every assistant message and
// captures the container replayed on the second request; when perCallContainer
// is set the Google vendor instead emits the recorded tool-call-only shape
// (fixture g4): the assistant message omits content and carries each tool
// call's extra_content inside the call, captured per call on replay. The xAI
// vendor carries the recorded usage decorations on every response.
type dialectCertRoundTripper struct {
	codec            dialectCodec
	vendor           dialectVendor
	container        json.RawMessage
	perCallContainer json.RawMessage
	usage            json.RawMessage
	strict           []byte

	requests          int
	invocationID      string
	replayedContainer json.RawMessage
	replayedPerCall   []json.RawMessage
	requestBodies     [][]byte
	servedResponses   [][]byte
}

func (transport *dialectCertRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	body, err := io.ReadAll(io.LimitReader(
		request.Body,
		int64(MaxProviderRequestBytes)+1,
	))
	if err != nil {
		return nil, err
	}
	transport.requestBodies = append(
		transport.requestBodies,
		append([]byte(nil), body...),
	)
	transport.requests++
	if transport.requests == 1 {
		transport.invocationID = dialectRequestInvocationID(
			transport.codec,
			body,
		)
	}
	var response *http.Response
	if transport.vendor == dialectVendorStrict {
		response = w8HTTPResponse(transport.strict)
	} else {
		switch transport.requests {
		case 1:
			callID := "dialect-read-1"
			arguments := `{"path":"/sworn/inputs/certification/request.json"}`
			response = w8HTTPResponse(transport.turn(callID, "Read", arguments))
		case 2:
			if transport.codec == dialectCodecChat {
				if transport.perCallContainer != nil {
					transport.replayedPerCall =
						dialectReplayedPerCallContainers(body)
				} else {
					transport.replayedContainer = dialectReplayedContainer(body)
				}
			}
			callID := "dialect-submit-1"
			arguments := dialectSubmissionArguments(transport.invocationID)
			response = w8HTTPResponse(transport.turn(callID, "sworn_submit", arguments))
		default:
			return nil, fail("TRANSPORT_FAILURE")
		}
	}
	served, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return nil, readErr
	}
	transport.servedResponses = append(
		transport.servedResponses,
		append([]byte(nil), served...),
	)
	response.Body = io.NopCloser(bytes.NewReader(served))
	return response, nil
}

// turn builds one provider response carrying a tool call plus the vendor's
// recorded decorations on the selected codec.
func (transport *dialectCertRoundTripper) turn(
	callID string,
	name string,
	arguments string,
) []byte {
	if transport.perCallContainer != nil {
		return transport.perCallTurn(callID, name, arguments)
	}
	if transport.codec == dialectCodecResponses {
		argumentsJSON, _ := json.Marshal(arguments)
		body := `{"id":"response-cert-1","object":"response","status":"completed","error":null,"output":[` +
			`{"type":"reasoning","id":"reasoning-cert-1","status":"completed","summary":[{"type":"summary_text","text":"certification probe"}]},` +
			`{"type":"function_call","id":` + strconv.Quote(callID) +
			`,"call_id":` + strconv.Quote(callID) +
			`,"name":` + strconv.Quote(name) +
			`,"arguments":` + string(argumentsJSON) + `,"status":"completed"}` +
			`],"usage":` + string(transport.usage) + `}`
		return []byte(body)
	}
	argumentsJSON, _ := json.Marshal(arguments)
	var builder strings.Builder
	builder.WriteString(
		`{"choices":[{"message":{"role":"assistant","content":null,`,
	)
	if len(transport.container) != 0 {
		builder.WriteString(`"extra_content":`)
		builder.Write(transport.container)
		builder.WriteByte(',')
	}
	builder.WriteString(`"tool_calls":[{"id":`)
	builder.WriteString(strconv.Quote(callID))
	builder.WriteString(`,"type":"function","function":{"name":`)
	builder.WriteString(strconv.Quote(name))
	builder.WriteString(`,"arguments":`)
	builder.Write(argumentsJSON)
	builder.WriteString(`}}]},"finish_reason":"tool_calls"}],`)
	if len(transport.usage) != 0 {
		builder.WriteString(`"usage":`)
		builder.Write(transport.usage)
		builder.WriteByte(',')
	}
	builder.WriteString(
		`"id":"dialect-cert-1","object":"chat.completion","created":1,"model":"certification"}`,
	)
	return []byte(builder.String())
}

// perCallTurn builds the recorded g4 tool-call-only shape: the assistant
// message carries no content field at all, and each tool call carries its own
// extra_content container at the per-call position.
func (transport *dialectCertRoundTripper) perCallTurn(
	callID string,
	name string,
	arguments string,
) []byte {
	argumentsJSON, _ := json.Marshal(arguments)
	var builder strings.Builder
	builder.WriteString(
		`{"choices":[{"finish_reason":"tool_calls","index":0,"message":{"role":"assistant","tool_calls":[{"extra_content":`,
	)
	builder.Write(transport.perCallContainer)
	builder.WriteString(`,"function":{"name":`)
	builder.WriteString(strconv.Quote(name))
	builder.WriteString(`,"arguments":`)
	builder.Write(argumentsJSON)
	builder.WriteString(`},"id":`)
	builder.WriteString(strconv.Quote(callID))
	builder.WriteString(`,"type":"function"}]}}],`)
	if len(transport.usage) != 0 {
		builder.WriteString(`"usage":`)
		builder.Write(transport.usage)
		builder.WriteByte(',')
	}
	builder.WriteString(
		`"id":"dialect-cert-1","object":"chat.completion","created":1,"model":"certification"}`,
	)
	return []byte(builder.String())
}

// dialectRequestInvocationID extracts the certification invocation ID from a
// request's leading user prompt envelope on either codec. The prompt is a
// JSON string whose decoded body is the model-prompt envelope, so the string
// is unwrapped before the envelope is read.
func dialectRequestInvocationID(codec dialectCodec, body []byte) string {
	var promptString string
	switch codec {
	case dialectCodecChat:
		var envelope struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &envelope) != nil ||
			len(envelope.Messages) == 0 ||
			json.Unmarshal(envelope.Messages[0].Content, &promptString) != nil {
			return ""
		}
	case dialectCodecResponses:
		var envelope struct {
			Input []json.RawMessage `json:"input"`
		}
		if json.Unmarshal(body, &envelope) != nil ||
			len(envelope.Input) == 0 {
			return ""
		}
		var item struct {
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(envelope.Input[0], &item) != nil ||
			json.Unmarshal(item.Content, &promptString) != nil {
			return ""
		}
	default:
		return ""
	}
	var prompt map[string]any
	if json.Unmarshal([]byte(promptString), &prompt) != nil {
		return ""
	}
	id, _ := prompt["invocation_id"].(string)
	return id
}

// dialectReplayedContainer captures the raw extra_content container replayed
// on the second chat request's assistant message.
func dialectReplayedContainer(body []byte) json.RawMessage {
	var envelope struct {
		Messages []struct {
			ExtraContent json.RawMessage `json:"extra_content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Messages) < 2 {
		return nil
	}
	return append(
		json.RawMessage(nil),
		envelope.Messages[1].ExtraContent...,
	)
}

// dialectReplayedPerCallContainers captures, in order, the per-call
// extra_content containers replayed inside the tool calls of the second chat
// request's assistant message.
func dialectReplayedPerCallContainers(body []byte) []json.RawMessage {
	var envelope struct {
		Messages []struct {
			ToolCalls []struct {
				ExtraContent json.RawMessage `json:"extra_content"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Messages) < 2 {
		return nil
	}
	containers := make(
		[]json.RawMessage,
		0,
		len(envelope.Messages[1].ToolCalls),
	)
	for _, call := range envelope.Messages[1].ToolCalls {
		containers = append(containers, append(
			json.RawMessage(nil),
			call.ExtraContent...,
		))
	}
	return containers
}

// dialectSubmissionArguments builds a valid implementer_design submission for
// the exact certification invocation ID, shaped exactly as the permission
// admits it (summary and detail only).
func dialectSubmissionArguments(invocationID string) string {
	submission := Submission{
		SchemaVersion:  SubmissionSchemaVersion,
		InvocationID:   invocationID,
		Responsibility: ImplementerDesign,
		Summary:        "Recorded provider-dialect certification probe. Padded past the submission content floor so this fixture clears A3 without losing its own certification identity or wording.",
		Detail:         "The fixture-serving round tripper replayed the recorded wire dialect. Padded past the submission detail content floor so this fixture clears A3 while still asserting the exact replayed dialect behaviour under test.\n",
	}
	body, err := EncodeSubmission(submission)
	if err != nil {
		return ""
	}
	var value any
	if json.Unmarshal(body, &value) != nil {
		return ""
	}
	arguments, err := json.Marshal(map[string]any{"submission": value})
	if err != nil {
		return ""
	}
	return string(arguments)
}

// providerDialectCertificationInvocation builds one certification-shaped
// invocation (the exact driver-certify request the production live probe
// runs) for an adapter-selected profile.
func providerDialectCertificationInvocation(
	t *testing.T,
	selected SelectedProfile,
) Invocation {
	t.Helper()
	workspace := t.TempDir()
	instruction := []byte(
		`{"instruction":"Exercise the configured model and terminate with one valid implementer_design sworn_submit call. Do not make product changes.","schema_version":"` +
			driverCertificationSchemaVersion + `"}`,
	)
	input := Input{
		Name:   "driver-certification",
		Path:   "certification/request.json",
		Digest: Digest(instruction),
	}
	request, err := NewRequest(
		"driver-certify-provider-dialects",
		RoleImplementer,
		selected.Profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: ReadWrite},
		[]Input{input},
		true,
		Limits{TimeoutMillis: 120_000, OutputBytes: MaxProviderOutputBytes},
	)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		ContainmentReadWrite,
		ImplementerDesign,
	)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request:       request,
		HostWorkspace: workspace,
		Selected:      selected,
		Permission:    permission,
		Inputs: []InputContent{{
			Input: input,
			Bytes: instruction,
		}},
		RecoveryStepHook: func(context.Context, RecoveryStepKind) error {
			return nil
		},
	}
}

// providerDialectAdapter builds a unified OpenAI adapter carrying the vendor
// dialect flags, a fixture round tripper, and a live probe that runs a full
// certification-shaped Dispatcher invocation against that adapter.
func providerDialectAdapter(
	t *testing.T,
	config OpenAIProfileConfig,
	roundTripper *dialectCertRoundTripper,
) (Adapter, error) {
	t.Helper()
	ref := "dialect-cert-credential"
	config.CredentialRefs = []string{ref}
	config.CredentialHeader = "Authorization"
	config.CredentialPrefix = "Bearer "
	config.ResponseBytes = MaxProviderResponseBytes
	var probe ProfileLiveProbe
	var adapter Adapter
	probe = func(ctx context.Context, refValue string, model string) error {
		refCopy := refValue
		selected := SelectedProfile{
			Profile: ProfileConfig{
				Key:           "live-certification",
				Adapter:       adapter.Identity().Key,
				Network:       NetworkRequired,
				AuthMode:      AuthModeBearer,
				CredentialRef: &refCopy,
			},
			Adapter: adapter.Identity(),
			Model:   model,
			adapter: adapter,
		}
		invocation := providerDialectCertificationInvocation(t, selected)
		_, err := (Dispatcher{}).Invoke(context.Background(), invocation)
		return err
	}
	var err error
	adapter, err = NewOpenAIAdapter(
		config,
		func(context.Context, string) ([]byte, error) {
			return []byte("dialect-cert-secret"), nil
		},
		nil,
		probe,
		roundTripper,
	)
	return adapter, err
}

// A6: certification passes for a Google chat-completions adapter with
// gemini-3.7-flash and an xAI responses adapter with grok-4.6 when replayed
// against the recorded fixtures, and the strict-dialect regression fixtures
// pin the certification failure codes these providers produce today.
func TestCertificationReplaysRecordedProviderDialects(t *testing.T) {
	geminiResponse := providerDialectFixture(
		t,
		"gemini_chat_response_thought_signature.json",
	)
	container := googleContainerFromResponse(t, geminiResponse)
	grokResponse := providerDialectFixture(
		t,
		"grok_responses_response_usage.json",
	)
	usageBlock := grokUsageFromResponse(t, grokResponse)
	trueFlag := true

	t.Run("google chat gemini-3.7-flash certifies", func(t *testing.T) {
		googleTransport := &dialectCertRoundTripper{
			codec:     dialectCodecChat,
			vendor:    dialectVendorGoogle,
			container: container,
		}
		adapter, err := providerDialectAdapter(t, OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "google-dialect", ID: "sworn.google.dialect",
				Version:  "1.0.0",
				Endpoint: "https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
			},
			API:              OpenAIChatCompletionsAPI,
			ThoughtSignature: &trueFlag,
		}, googleTransport)
		if err != nil {
			t.Fatal(err)
		}
		ref := "dialect-cert-credential"
		registry, err := NewSelectionRegistry(
			[]ProfileConfig{{
				Key: "google-dialect", Adapter: adapter.Identity().Key,
				Network: NetworkRequired, CredentialRef: &ref,
			}},
			[]Adapter{adapter},
		)
		if err != nil {
			t.Fatal(err)
		}
		report := registry.Certify(
			context.Background(),
			"google-dialect",
			"gemini-3.7-flash",
		)
		if report.State != ReadinessPass || report.Code != "live_probe_passed" {
			t.Fatalf("google certification report = %#v", report)
		}
		// The second request replayed the recorded container byte-exact at
		// the assistant-message position the provider requires.
		if !bytes.Equal(googleTransport.replayedContainer, container) {
			t.Fatalf(
				"certification replayed container = %q, want %q",
				googleTransport.replayedContainer,
				container,
			)
		}
	})

	t.Run("google chat tool-call turn certifies with per-call containers", func(t *testing.T) {
		perCall := json.RawMessage(
			`{"google":{"thought_signature":"PER-CALL-CERT-SIGNATURE-` +
				"8f3a5c2e9b1d7f4a6c8e0b2d" + `"}}`,
		)
		toolCallTransport := &dialectCertRoundTripper{
			codec:            dialectCodecChat,
			vendor:           dialectVendorGoogle,
			perCallContainer: perCall,
		}
		adapter, err := providerDialectAdapter(t, OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "google-toolcall-dialect", ID: "sworn.google.toolcall",
				Version:  "1.0.0",
				Endpoint: "https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
			},
			API:              OpenAIChatCompletionsAPI,
			ThoughtSignature: &trueFlag,
		}, toolCallTransport)
		if err != nil {
			t.Fatal(err)
		}
		ref := "dialect-cert-credential"
		registry, err := NewSelectionRegistry(
			[]ProfileConfig{{
				Key: "google-toolcall-dialect", Adapter: adapter.Identity().Key,
				Network: NetworkRequired, CredentialRef: &ref,
			}},
			[]Adapter{adapter},
		)
		if err != nil {
			t.Fatal(err)
		}
		report := registry.Certify(
			context.Background(),
			"google-toolcall-dialect",
			"gemini-3.7-flash",
		)
		if report.State != ReadinessPass || report.Code != "live_probe_passed" {
			t.Fatalf("google tool-call certification report = %#v", report)
		}
		// The second request replayed the per-call container byte-exact inside
		// the same tool call (the g4/g5 shape), content still absent.
		if len(toolCallTransport.replayedPerCall) != 1 ||
			!bytes.Equal(toolCallTransport.replayedPerCall[0], perCall) {
			t.Fatalf(
				"certification replayed per-call = %v, want %q",
				toolCallTransport.replayedPerCall,
				perCall,
			)
		}
	})

	t.Run("xai responses grok-4.6 certifies", func(t *testing.T) {
		xaiTransport := &dialectCertRoundTripper{
			codec:  dialectCodecResponses,
			vendor: dialectVendorXAI,
			usage:  usageBlock,
		}
		adapter, err := providerDialectAdapter(t, OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "xai-dialect", ID: "sworn.xai.dialect",
				Version:  "1.0.0",
				Endpoint: "https://api.x.ai.example.invalid/v1/responses",
			},
			API:             OpenAIResponsesAPI,
			ReasoningEffort: "high",
			VendorUsage:     &trueFlag,
		}, xaiTransport)
		if err != nil {
			t.Fatal(err)
		}
		ref := "dialect-cert-credential"
		registry, err := NewSelectionRegistry(
			[]ProfileConfig{{
				Key: "xai-dialect", Adapter: adapter.Identity().Key,
				Network: NetworkRequired, CredentialRef: &ref,
			}},
			[]Adapter{adapter},
		)
		if err != nil {
			t.Fatal(err)
		}
		report := registry.Certify(
			context.Background(),
			"xai-dialect",
			"grok-4.6",
		)
		if report.State != ReadinessPass || report.Code != "live_probe_passed" {
			t.Fatalf("xai certification report = %#v", report)
		}
	})

	t.Run("strict chat pins certification_response_contract_failed", func(t *testing.T) {
		strictTransport := &dialectCertRoundTripper{
			codec:  dialectCodecChat,
			vendor: dialectVendorStrict,
			strict: geminiResponse,
		}
		adapter, err := providerDialectAdapter(t, OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "strict-chat-dialect", ID: "sworn.strict.chat",
				Version:  "1.0.0",
				Endpoint: "https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
			},
			API: OpenAIChatCompletionsAPI,
		}, strictTransport)
		if err != nil {
			t.Fatal(err)
		}
		ref := "dialect-cert-credential"
		registry, err := NewSelectionRegistry(
			[]ProfileConfig{{
				Key: "strict-chat-dialect", Adapter: adapter.Identity().Key,
				Network: NetworkRequired, CredentialRef: &ref,
			}},
			[]Adapter{adapter},
		)
		if err != nil {
			t.Fatal(err)
		}
		report := registry.Certify(
			context.Background(),
			"strict-chat-dialect",
			"gemini-3.7-flash",
		)
		if report.State != ReadinessFail ||
			report.Code != "certification_response_contract_failed" {
			t.Fatalf("strict chat report = %#v", report)
		}
	})

	t.Run("strict chat pins certification_response_contract_failed on the g4 tool-call turn", func(t *testing.T) {
		// The recorded g4 tool-call-only message has no content field at all:
		// strict chat fails at the assistant-message closed-object check
		// (MISSING_FIELD on content), so the certification failure this gap
		// produces today is pinned and cannot silently return.
		g4 := providerDialectFixture(
			t,
			"gemini_chat_response_tool_call_thought_signature.json",
		)
		strictTransport := &dialectCertRoundTripper{
			codec:  dialectCodecChat,
			vendor: dialectVendorStrict,
			strict: g4,
		}
		adapter, err := providerDialectAdapter(t, OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "strict-g4-dialect", ID: "sworn.strict.g4",
				Version:  "1.0.0",
				Endpoint: "https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
			},
			API: OpenAIChatCompletionsAPI,
		}, strictTransport)
		if err != nil {
			t.Fatal(err)
		}
		ref := "dialect-cert-credential"
		registry, err := NewSelectionRegistry(
			[]ProfileConfig{{
				Key: "strict-g4-dialect", Adapter: adapter.Identity().Key,
				Network: NetworkRequired, CredentialRef: &ref,
			}},
			[]Adapter{adapter},
		)
		if err != nil {
			t.Fatal(err)
		}
		report := registry.Certify(
			context.Background(),
			"strict-g4-dialect",
			"gemini-3.7-flash",
		)
		if report.State != ReadinessFail ||
			report.Code != "certification_response_contract_failed" {
			t.Fatalf("strict g4 report = %#v", report)
		}
	})

	t.Run("strict responses pins certification_usage_failed", func(t *testing.T) {
		strictTransport := &dialectCertRoundTripper{
			codec:  dialectCodecResponses,
			vendor: dialectVendorStrict,
			strict: grokResponse,
		}
		adapter, err := providerDialectAdapter(t, OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "strict-responses-dialect", ID: "sworn.strict.responses",
				Version:  "1.0.0",
				Endpoint: "https://api.x.ai.example.invalid/v1/responses",
			},
			API:             OpenAIResponsesAPI,
			ReasoningEffort: "high",
		}, strictTransport)
		if err != nil {
			t.Fatal(err)
		}
		ref := "dialect-cert-credential"
		registry, err := NewSelectionRegistry(
			[]ProfileConfig{{
				Key: "strict-responses-dialect", Adapter: adapter.Identity().Key,
				Network: NetworkRequired, CredentialRef: &ref,
			}},
			[]Adapter{adapter},
		)
		if err != nil {
			t.Fatal(err)
		}
		report := registry.Certify(
			context.Background(),
			"strict-responses-dialect",
			"grok-4.6",
		)
		if report.State != ReadinessFail ||
			report.Code != "certification_usage_failed" {
			t.Fatalf("strict responses report = %#v", report)
		}
	})
}

// A5: nothing from an extension container crosses the trust boundary. A full
// Dispatcher invocation replays a canary signature on the wire, and the
// observation, submission, usage receipt, and encoded result contain none of
// it; close() zeroes the retained message bytes.
func TestOpaqueVendorStateNeverCrossesTrustBoundary(t *testing.T) {
	canary := "A5-CANARY-OPAQUE-SIGNATURE-9f8e7d6c5b4a3210"
	container := json.RawMessage(
		`{"google":{"thought_signature":"` + canary + `"}}`,
	)
	transport := &dialectCertRoundTripper{
		codec:     dialectCodecChat,
		vendor:    dialectVendorGoogle,
		container: container,
	}
	trueFlag := true
	adapter, err := providerDialectAdapter(t, OpenAIProfileConfig{
		HTTPProfileConfig: HTTPProfileConfig{
			Key: "boundary-dialect", ID: "sworn.boundary.dialect",
			Version:  "1.0.0",
			Endpoint: "https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		},
		API:              OpenAIChatCompletionsAPI,
		ThoughtSignature: &trueFlag,
	}, transport)
	if err != nil {
		t.Fatal(err)
	}
	ref := "dialect-cert-credential"
	selected := SelectedProfile{
		Profile: ProfileConfig{
			Key: "boundary-dialect", Adapter: adapter.Identity().Key,
			Network: NetworkRequired, AuthMode: AuthModeBearer,
			CredentialRef: &ref,
		},
		Adapter: adapter.Identity(),
		Model:   "gemini-3.7-flash",
		adapter: adapter,
	}
	invocation := providerDialectCertificationInvocation(t, selected)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil ||
		observation.TransportStatus != Completed {
		t.Fatalf("boundary observation = %#v, %v", observation, err)
	}
	observationBody, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(observationBody, []byte(canary)) {
		t.Fatalf("canary escaped into observation: %s", observationBody)
	}
	if bytes.Contains(observation.Handoff.SubmissionBytes, []byte(canary)) {
		t.Fatalf(
			"canary escaped into submission: %s",
			observation.Handoff.SubmissionBytes,
		)
	}
	if bytes.Contains(observation.Handoff.SealBytes, []byte(canary)) {
		t.Fatalf("canary escaped into seal: %s", observation.Handoff.SealBytes)
	}
	var resultUsage *Usage
	if observation.Usage.InputTokens != nil {
		resultUsage = &Usage{
			InputTokens:  *observation.Usage.InputTokens,
			OutputTokens: *observation.Usage.OutputTokens,
		}
	}
	result, err := EncodeResult(Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    invocation.Request.InvocationID,
		AdapterID:       adapter.Identity().ID,
		AdapterVersion:  adapter.Identity().Version,
		ObservedModel:   selected.Model,
		DurationMillis:  1,
		TransportStatus: Completed,
		Usage:           resultUsage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result, []byte(canary)) {
		t.Fatalf("canary escaped into result: %s", result)
	}
	// The canary did reach the wire: it was replayed byte-exact on the second
	// request's assistant message, proving opaque retention while staying out
	// of every external surface.
	if len(transport.requestBodies) < 2 ||
		!bytes.Contains(transport.requestBodies[1], []byte(canary)) {
		t.Fatalf("canary was not replayed on the wire: %d requests",
			len(transport.requestBodies))
	}

	// close() zeroes the retained container bytes alongside the rest of the
	// message state.
	conversation, err := newOpenAIConversation(
		"https://generativelanguage.example.invalid/v1beta/openai/chat/completions",
		"gemini-3.7-flash",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectGoogleChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.accept([]byte(
		`{"choices":[{"message":{"role":"assistant","content":"chosen.","extra_content":` +
			string(container) + `},"finish_reason":"stop"}]}`,
	)); err != nil {
		t.Fatal(err)
	}
	retained := conversation.messages[1].ExtraContent
	if !bytes.Equal(retained, container) {
		t.Fatalf("retained = %q", retained)
	}
	conversation.close()
	for index := range retained {
		if retained[index] != 0 {
			t.Fatalf("container byte %d not cleared after close", index)
		}
	}
}

// Config plumbing: the vendor dialect flags select the exact wire dialect on
// each API surface, preset inheritance carries them into adapters, invalid
// combinations are rejected at admission, and omission keeps canonical JSON
// untouched.
func TestProviderDialectConfigSelectionAndPresetInheritance(t *testing.T) {
	t.Parallel()
	trueFlag, falseFlag := true, false
	base := HTTPProfileConfig{
		Key: "dialect-config", ID: "sworn.dialect.config",
		Version:          "1.0.0",
		Endpoint:         "https://provider.example.invalid/v1/chat/completions",
		CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
		CredentialRefs: []string{"dialect-ref"},
		ResponseBytes:  MaxProviderResponseBytes,
	}
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("secret"), nil
	}

	t.Run("dialect selection per surface", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name    string
			api     OpenAIAPI
			effort  string
			flags   func(config *OpenAIProfileConfig)
			dialect providerDialect
			surface ProfileSurface
		}{
			{"google chat", OpenAIChatCompletionsAPI, "", func(c *OpenAIProfileConfig) {
				c.ThoughtSignature = &trueFlag
			}, providerDialectGoogleChat, ProfileSurfaceOpenAIChat},
			{"xai chat", OpenAIChatCompletionsAPI, "", func(c *OpenAIProfileConfig) {
				c.VendorUsage = &trueFlag
			}, providerDialectXAIChat, ProfileSurfaceOpenAIChat},
			{"xai responses", OpenAIResponsesAPI, "high", func(c *OpenAIProfileConfig) {
				c.VendorUsage = &trueFlag
			}, providerDialectXAIResponses, ProfileSurfaceOpenAIResponses},
			{"explicit false google", OpenAIChatCompletionsAPI, "", func(c *OpenAIProfileConfig) {
				c.ThoughtSignature = &falseFlag
			}, providerDialectOpenAIChat, ProfileSurfaceOpenAIChat},
		}
		for _, row := range rows {
			row := row
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				config := OpenAIProfileConfig{
					HTTPProfileConfig: base,
					API:               row.api,
					ReasoningEffort:   row.effort,
				}
				row.flags(&config)
				adapter, err := NewOpenAIAdapter(
					config,
					resolver,
					nil,
					nil,
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}
				loop := adapter.(*loopAdapter)
				if loop.dialect != row.dialect ||
					loop.surface != row.surface ||
					loop.dialect.continuationMode() == "" {
					t.Fatalf("adapter dialect=%q surface=%q", loop.dialect, loop.surface)
				}
			})
		}
	})

	t.Run("invalid combinations rejected", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name   string
			api    OpenAIAPI
			effort string
			flags  func(config *OpenAIProfileConfig)
		}{
			{"thought signature on responses", OpenAIResponsesAPI, "high", func(c *OpenAIProfileConfig) {
				c.ThoughtSignature = &trueFlag
			}},
			{"thought signature on openrouter", OpenRouterChatCompletionsAPI, "", func(c *OpenAIProfileConfig) {
				c.ThoughtSignature = &trueFlag
			}},
			{"vendor usage on openrouter", OpenRouterChatCompletionsAPI, "", func(c *OpenAIProfileConfig) {
				c.VendorUsage = &trueFlag
			}},
			{"thought signature and vendor usage", OpenAIChatCompletionsAPI, "", func(c *OpenAIProfileConfig) {
				c.ThoughtSignature = &trueFlag
				c.VendorUsage = &trueFlag
			}},
			{"opaque reasoning and thought signature", OpenAIChatCompletionsAPI, "", func(c *OpenAIProfileConfig) {
				c.OpaqueReasoning = &trueFlag
				c.ThoughtSignature = &trueFlag
			}},
			{"opaque reasoning and vendor usage", OpenAIChatCompletionsAPI, "", func(c *OpenAIProfileConfig) {
				c.OpaqueReasoning = &trueFlag
				c.VendorUsage = &trueFlag
			}},
		}
		for _, row := range rows {
			row := row
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				config := OpenAIProfileConfig{
					HTTPProfileConfig: base,
					API:               row.api,
					ReasoningEffort:   row.effort,
				}
				row.flags(&config)
				if _, err := NewOpenAIAdapter(
					config,
					resolver,
					nil,
					nil,
					nil,
				); !IsCode(err, "INVALID_ADAPTER") {
					t.Fatalf("adapter error = %v", err)
				}
			})
		}
	})

	t.Run("preset inheritance", func(t *testing.T) {
		t.Parallel()
		preset := DriverPreset{
			Key: "dialect-preset", API: OpenAIChatCompletionsAPI,
			BaseURL:          "https://provider.example.invalid/v1/chat/completions",
			Auth:             AuthModeBearer,
			CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
			ResponseBytes:    MaxProviderResponseBytes,
			ThoughtSignature: true,
			VendorUsage:      false,
		}
		config := DriverAdapterConfig{OpenAI: &OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "dialect-adapter", ID: "sworn.dialect.adapter",
				Version:        "1.0.0",
				CredentialRefs: []string{"dialect-ref"},
			},
			Preset: "dialect-preset",
		}}
		resolved, err := resolvePreset(
			config,
			map[string]DriverPreset{"dialect-preset": preset},
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.OpenAI == nil ||
			resolved.OpenAI.ThoughtSignature == nil ||
			!*resolved.OpenAI.ThoughtSignature ||
			resolved.OpenAI.VendorUsage == nil ||
			*resolved.OpenAI.VendorUsage {
			t.Fatalf("preset flags not inherited: %#v", resolved.OpenAI)
		}
		// Omission of both flags stays omitted from canonical JSON, so
		// untouched documents keep their exact configuration digest.
		plain := OpenAIProfileConfig{HTTPProfileConfig: base, API: OpenAIChatCompletionsAPI}
		plainBody, err := canonicalJSON(plain)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(plainBody, []byte("thought_signature")) ||
			bytes.Contains(plainBody, []byte("vendor_usage")) {
			t.Fatalf("nil flags leaked into canonical JSON: %s", plainBody)
		}
	})
}

// A2 also checks the recorded silent-degradation probe into the fixture set
// as the reason replay is mandatory: the same second-turn request with the
// signature absent is what made the model answer confidently and wrongly.
func TestRecordedProbeDocumentsSilentDegradationWithoutSignature(t *testing.T) {
	t.Parallel()
	replayed := providerDialectFixture(
		t,
		"gemini_chat_request_replayed_signature.json",
	)
	without := providerDialectFixture(
		t,
		"gemini_chat_request_without_signature.json",
	)
	var withMessages, withoutMessages struct {
		Messages []struct {
			Content      string          `json:"content"`
			ExtraContent json.RawMessage `json:"extra_content"`
			Role         string          `json:"role"`
		} `json:"messages"`
	}
	if json.Unmarshal(replayed, &withMessages) != nil ||
		json.Unmarshal(without, &withoutMessages) != nil {
		t.Fatal("invalid recorded probe fixtures")
	}
	if len(withMessages.Messages) != 3 ||
		len(withoutMessages.Messages) != 3 ||
		withMessages.Messages[1].Role != "assistant" ||
		withoutMessages.Messages[1].Role != "assistant" {
		t.Fatal("recorded probe is not the two-turn exchange")
	}
	if len(withMessages.Messages[1].ExtraContent) == 0 {
		t.Fatal("replayed probe must carry the thought signature")
	}
	if len(withoutMessages.Messages[1].ExtraContent) != 0 {
		t.Fatal("silent-degradation probe must omit the thought signature")
	}
	if withMessages.Messages[1].Content != withoutMessages.Messages[1].Content {
		t.Fatal("the two probe requests differ beyond the signature")
	}
}

// A1: recorded multi-call fixtures decode, execute in order, and return
// correlated results in the following request on both the Responses and Chat
// surfaces, and resume succeeds over the multi-call tail.
func TestParallelToolCallsRecordedFixturesRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("chat surface recorded multi-call fixture", func(t *testing.T) {
		t.Parallel()
		conversation, err := newOpenAIConversation(
			"https://api.openai.example.invalid/v1/chat/completions",
			"gpt-4o",
			toolDefinitions(ReadOnly),
			[]byte(`{"prompt":"bounded"}`),
			providerDialectOpenAIChat,
			"",
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()

		response := providerDialectFixture(t, "multicall_chat_response.json")
		turn, err := conversation.accept(response)
		if err != nil || len(turn.Calls) != 3 ||
			turn.Calls[0].ID != "call_multicall_chat_1" || turn.Calls[0].Name != "Read" ||
			turn.Calls[1].ID != "call_multicall_chat_2" || turn.Calls[1].Name != "Read" ||
			turn.Calls[2].ID != "call_multicall_chat_3" || turn.Calls[2].Name != "Read" ||
			turn.Usage == nil || turn.Usage.InputTokens != 42 || turn.Usage.OutputTokens != 38 {
			t.Fatalf("chat multi-call turn = %#v, %v", turn, err)
		}

		results := []providerToolResult{
			{ID: "call_multicall_chat_1", Name: "Read", Content: []byte("content of a")},
			{ID: "call_multicall_chat_2", Name: "Read", Content: []byte("content of b")},
			{ID: "call_multicall_chat_3", Name: "Read", Content: []byte("content of c")},
		}
		if err := conversation.appendResults(results); err != nil {
			t.Fatal(err)
		}

		request, err := conversation.request()
		if err != nil {
			t.Fatal(err)
		}
		var req struct {
			Messages []openAIMessage `json:"messages"`
		}
		if json.Unmarshal(request.Body, &req) != nil || len(req.Messages) != 5 {
			t.Fatalf("replayed request = %s", request.Body)
		}
		for i, expected := range results {
			toolMsg := req.Messages[2+i]
			if toolMsg.Role != "tool" || toolMsg.ToolCallID != expected.ID {
				t.Fatalf("tool message %d = %#v, want ID %s", i, toolMsg, expected.ID)
			}
			var content string
			if json.Unmarshal(toolMsg.Content, &content) != nil || content != string(expected.Content) {
				t.Fatalf("tool message %d content = %s, want %s", i, toolMsg.Content, expected.Content)
			}
		}

		if err := conversation.resume([]byte(`{"resume":"prompt"}`), toolDefinitions(ReadWrite)); err != nil {
			t.Fatalf("resume failed over 3-call tail: %v", err)
		}
		if len(conversation.messages) != 6 || conversation.messages[5].Role != "user" {
			t.Fatalf("messages after resume = %#v", conversation.messages)
		}
	})

	t.Run("responses surface recorded multi-call fixture", func(t *testing.T) {
		t.Parallel()
		conversation, err := newResponsesConversation(
			"https://api.openai.example.invalid/v1/responses",
			"gpt-4o",
			toolDefinitions(ReadOnly),
			[]byte(`{"prompt":"bounded"}`),
			"medium",
			nil,
			false,
			providerDialectOpenAIResponses,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()

		response := providerDialectFixture(t, "multicall_responses_response.json")
		turn, err := conversation.accept(response)
		if err != nil || len(turn.Calls) != 3 ||
			turn.Calls[0].ID != "call_multicall_resp_1" || turn.Calls[0].Name != "Read" ||
			turn.Calls[1].ID != "call_multicall_resp_2" || turn.Calls[1].Name != "Read" ||
			turn.Calls[2].ID != "call_multicall_resp_3" || turn.Calls[2].Name != "Read" ||
			turn.Usage == nil || turn.Usage.InputTokens != 52 || turn.Usage.OutputTokens != 44 {
			t.Fatalf("responses multi-call turn = %#v, %v", turn, err)
		}

		results := []providerToolResult{
			{ID: "call_multicall_resp_1", Name: "Read", Content: []byte("content of a")},
			{ID: "call_multicall_resp_2", Name: "Read", Content: []byte("content of b")},
			{ID: "call_multicall_resp_3", Name: "Read", Content: []byte("content of c")},
		}
		if err := conversation.appendResults(results); err != nil {
			t.Fatal(err)
		}

		request, err := conversation.request()
		if err != nil {
			t.Fatal(err)
		}
		var req struct {
			Input []json.RawMessage `json:"input"`
		}
		if json.Unmarshal(request.Body, &req) != nil || len(req.Input) != 8 {
			t.Fatalf("responses replayed request = %s (input len %d)", request.Body, len(req.Input))
		}

		if err := conversation.resume([]byte(`{"resume":"prompt"}`), toolDefinitions(ReadWrite)); err != nil {
			t.Fatalf("responses resume failed over 3-call tail: %v", err)
		}
		if len(conversation.input) != 9 {
			t.Fatalf("responses input after resume = %d items", len(conversation.input))
		}
	})
}

// A1: the responses usage parser extracts reasoning tokens from the same
// payload the live renderer prints (stream.go), so the recorded grok fixture
// that carries output_tokens_details.reasoning_tokens:42 lands on the
// receipt instead of dying on stderr. Today-before-the-fix this test fails:
// the receipt dropped the 42.
func TestResponsesUsageSurfacesReasoningTokensFromRecordedWire(t *testing.T) {
	t.Parallel()
	recorded := providerDialectFixture(
		t,
		"grok_responses_response_usage.json",
	)
	conversation, err := newResponsesConversation(
		"https://api.x.ai.example.invalid/v1/responses",
		"grok-4.6",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		"high",
		nil,
		false,
		providerDialectXAIResponses,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	turn, err := conversation.accept(recorded)
	if err != nil || turn.Usage == nil ||
		turn.Usage.ReasoningTokens == nil ||
		*turn.Usage.ReasoningTokens != 42 {
		t.Fatalf("grok responses reasoning = %#v, %v", turn.Usage, err)
	}
	receipt, err := NormalizeUsage(turn.Usage, nil, "sworn.test")
	if err != nil || receipt.ReasoningTokens == nil ||
		*receipt.ReasoningTokens != 42 ||
		receipt.CacheReadTokens == nil ||
		*receipt.CacheReadTokens != 128 {
		t.Fatalf("grok responses receipt = %#v, %v", receipt, err)
	}
}

// A1: the chat-completions details objects carry the reasoning and cached
// sides of the standard vocabulary; both now reach the receipt from a
// recorded-shape fixture, leniently (a malformed detail never fails the run).
func TestOpenAIChatUsageSurfacesDetailsFromRecordedWire(t *testing.T) {
	t.Parallel()
	recorded := providerDialectFixture(
		t,
		"openai_chat_usage_details.json",
	)
	conversation, err := newOpenAIConversation(
		"https://api.openai.example.invalid/v1/chat/completions",
		"gpt-4.1",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectOpenAIChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	turn, err := conversation.accept(recorded)
	if err != nil || turn.Usage == nil ||
		turn.Usage.CacheReadTokens == nil ||
		*turn.Usage.CacheReadTokens != 4 ||
		turn.Usage.ReasoningTokens == nil ||
		*turn.Usage.ReasoningTokens != 3 {
		t.Fatalf("chat details turn = %#v, %v", turn.Usage, err)
	}
	receipt, err := NormalizeUsage(turn.Usage, nil, "sworn.test")
	if err != nil || receipt.CacheReadTokens == nil ||
		*receipt.CacheReadTokens != 4 ||
		receipt.ReasoningTokens == nil ||
		*receipt.ReasoningTokens != 3 {
		t.Fatalf("chat details receipt = %#v, %v", receipt, err)
	}

	// A malformed detail object is ignored, never a failed run.
	malformed := []byte(
		`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"prompt_tokens_details":{"cached_tokens":"bogus"},"completion_tokens_details":{"reasoning_tokens":["bogus"]}}}`,
	)
	turn, err = conversation.accept(malformed)
	if err != nil || turn.Usage == nil ||
		turn.Usage.CacheReadTokens != nil ||
		turn.Usage.ReasoningTokens != nil {
		t.Fatalf("malformed details = %#v, %v", turn.Usage, err)
	}
}
