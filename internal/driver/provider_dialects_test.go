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
		receipt, err := NormalizeUsage(turn.Usage, nil)
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
		receipt, err := NormalizeUsage(turn.Usage, nil)
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
// captures the container replayed on the second request; the xAI vendor
// carries the recorded usage decorations on every response.
type dialectCertRoundTripper struct {
	codec     dialectCodec
	vendor    dialectVendor
	container json.RawMessage
	usage     json.RawMessage
	strict    []byte

	requests          int
	invocationID      string
	replayedContainer json.RawMessage
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
				transport.replayedContainer = dialectReplayedContainer(body)
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

// dialectSubmissionArguments builds a valid implementer_design submission for
// the exact certification invocation ID, shaped exactly as the permission
// admits it (summary and detail only).
func dialectSubmissionArguments(invocationID string) string {
	submission := Submission{
		SchemaVersion:  SubmissionSchemaVersion,
		InvocationID:   invocationID,
		Responsibility: ImplementerDesign,
		Summary:        "Recorded provider-dialect certification probe.",
		Detail:         "The fixture-serving round tripper replayed the recorded wire dialect.\n",
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
