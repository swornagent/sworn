package driver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestContinuationLedgerBoundsCanonicalOpaqueStateAndClears(t *testing.T) {
	t.Parallel()
	ledger := newContinuationLedger()
	signature := []byte(base64.StdEncoding.EncodeToString([]byte("signed-state")))
	retained, err := ledger.retain(
		opaqueField{kind: opaqueText, body: []byte("reasoning\nstate")},
		opaqueField{kind: opaqueBase64, body: signature},
	)
	if err != nil || len(retained) != 2 ||
		!bytes.Equal(retained[1], signature) {
		t.Fatalf("retain = %q, %v", retained, err)
	}
	if err := ledger.correlate("call-1"); err != nil {
		t.Fatal(err)
	}
	if err := ledger.correlate("call-1"); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("duplicate correlation error = %v", err)
	}
	if _, err := ledger.retain(opaqueField{
		kind: opaqueBase64, body: []byte("not canonical"),
	}); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("noncanonical signature error = %v", err)
	}
	if _, err := ledger.retain(opaqueField{
		kind: opaqueText, body: bytes.Repeat([]byte("x"), MaxOpaqueFieldBytes+1),
	}); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("oversize error = %v", err)
	}
	ledger.Close()
	for _, field := range retained {
		if !bytes.Equal(field, make([]byte, len(field))) {
			t.Fatalf("retained field not cleared: %q", field)
		}
	}
	if _, err := ledger.retain(opaqueField{kind: opaqueText, body: []byte("late")}); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("post-close error = %v", err)
	}
}

func TestDeepSeekReplaysReasoningAndExactToolCorrelation(t *testing.T) {
	t.Parallel()
	conversation, err := newOpenAIConversation(
		"https://api.example.invalid/chat/completions",
		"deepseek-reasoner",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		true,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	first := []byte(`{
	  "choices":[{
	    "message":{"role":"assistant","content":null,"reasoning_content":"opaque-reasoning",
	      "tool_calls":[{"id":"call-1","type":"function","function":{"name":"Read","arguments":"{\"path\":\"/workspace/a.txt\"}"}}]},
	    "finish_reason":"tool_calls"}],
	  "usage":{"prompt_tokens":2,"completion_tokens":3}
	}`)
	turn, err := conversation.accept(first)
	if err != nil || len(turn.Calls) != 1 ||
		string(turn.Calls[0].Arguments) != `{"path":"/workspace/a.txt"}` ||
		turn.Usage.InputTokens != 2 || turn.Usage.OutputTokens != 3 {
		t.Fatalf("first turn = %#v, %v", turn, err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "call-1", Name: "Read", Content: []byte("file body"),
	}}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var replay struct {
		Messages []map[string]any `json:"messages"`
	}
	if json.Unmarshal(request.Body, &replay) != nil || len(replay.Messages) != 3 ||
		replay.Messages[1]["reasoning_content"] != "opaque-reasoning" ||
		replay.Messages[2]["tool_call_id"] != "call-1" {
		t.Fatalf("DeepSeek replay = %s", request.Body)
	}
	duplicate := []byte(`{"choices":[{"message":{"role":"assistant","content":null,"reasoning_content":"again","tool_calls":[{"id":"call-1","type":"function","function":{"name":"Read","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
	if _, err := conversation.accept(duplicate); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("duplicate id error = %v", err)
	}
}

func TestOpenAIProfileRejectsUnexpectedReasoningState(t *testing.T) {
	t.Parallel()
	conversation, err := newOpenAIConversation(
		"https://api.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadOnly),
		[]byte(`{}`),
		false,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	response := []byte(`{"choices":[{"message":{"role":"assistant","content":null,"reasoning_content":"provider-extension","tool_calls":[{"id":"call","type":"function","function":{"name":"Read","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
	if _, err := conversation.accept(response); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("unexpected reasoning error = %v", err)
	}
	unknown := []byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call","type":"function","function":{"name":"Read","arguments":"{}","provider_extension":"forbidden"}}]},"finish_reason":"tool_calls"}]}`)
	if _, err := conversation.accept(unknown); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("unknown continuation field error = %v", err)
	}
}

func TestResponsesReplaysEncryptedReasoningAndExactToolCorrelation(t *testing.T) {
	t.Parallel()
	conversation, err := newResponsesConversation(
		"https://api.example.invalid/v1/responses",
		"gpt-5.6-sol",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		"medium",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	reasoning := json.RawMessage(
		`{"type":"reasoning","id":"reasoning-1","status":"completed","summary":[{"type":"summary_text","text":"provider-owned"}],"content":[{"type":"reasoning_text","text":"provider-owned"}],"encrypted_content":"opaque-encrypted-reasoning"}`,
	)
	message := json.RawMessage(
		`{"type":"message","id":"message-1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"provider-owned"}]}`,
	)
	function := json.RawMessage(
		`{"type":"function_call","call_id":"call-1","name":"Read","arguments":"{\"path\":\"/workspace/a.txt\"}","status":"completed","caller":"provider-owned","namespace":"provider-owned"}`,
	)
	response := append(
		[]byte(`{"id":"response-1","object":"response","status":"completed","output":[`),
		reasoning...,
	)
	response = append(response, ',')
	response = append(response, message...)
	response = append(response, ',')
	response = append(response, function...)
	response = append(
		response,
		[]byte(`],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5},"future_metadata":{"ignored":true}}`)...,
	)
	turn, err := conversation.accept(response)
	if err != nil || len(turn.Calls) != 1 ||
		turn.Calls[0].ID != "call-1" ||
		turn.Calls[0].Name != "Read" ||
		string(turn.Calls[0].Arguments) != `{"path":"/workspace/a.txt"}` ||
		turn.Usage == nil ||
		turn.Usage.InputTokens != 2 ||
		turn.Usage.OutputTokens != 3 {
		t.Fatalf("turn = %#v, %v", turn, err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "call-1", Name: "Read", Content: []byte("file body"),
	}}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var replay struct {
		Store     *bool              `json:"store"`
		Reasoning responsesReasoning `json:"reasoning"`
		Input     []json.RawMessage  `json:"input"`
	}
	if json.Unmarshal(request.Body, &replay) != nil ||
		replay.Store == nil || *replay.Store ||
		replay.Reasoning.Effort != "medium" ||
		len(replay.Input) != 5 ||
		!bytes.Equal(replay.Input[1], reasoning) ||
		!bytes.Equal(replay.Input[2], message) ||
		!bytes.Equal(replay.Input[3], function) {
		t.Fatalf("Responses replay = %s", request.Body)
	}
	var result responsesFunctionOutput
	if json.Unmarshal(replay.Input[4], &result) != nil ||
		result.Type != "function_call_output" ||
		result.CallID != "call-1" ||
		result.Output != "file body" {
		t.Fatalf("Responses tool result = %s", replay.Input[4])
	}
	duplicate := []byte(
		`{"status":"completed","output":[{"type":"function_call","id":"function-2","call_id":"call-1","name":"Read","arguments":"{}","status":"completed"}]}`,
	)
	if _, err := conversation.accept(duplicate); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("duplicate call error = %v", err)
	}
}

func TestOpenAIAPIConfigurationIsExplicit(t *testing.T) {
	t.Parallel()
	base := HTTPProfileConfig{
		Key: "openai", ID: "sworn.openai", Version: "1.0.0",
		CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
		CredentialRefs: []string{"credential-ref"},
		ResponseBytes:  MaxProviderResponseBytes,
	}
	for _, test := range []struct {
		name   string
		api    OpenAIAPI
		path   string
		effort string
		valid  bool
	}{
		{"Responses custom route", OpenAIResponsesAPI, "/deployments/model/responses", "medium", true},
		{"Responses needs effort", OpenAIResponsesAPI, "/v1/responses", "", false},
		{"Chat compatibility", OpenAIChatCompletionsAPI, "/chat/completions", "", true},
		{"Chat disables reasoning", OpenAIChatCompletionsAPI, "/v1/chat/completions", "none", true},
		{"Chat rejects reasoning", OpenAIChatCompletionsAPI, "/v1/chat/completions", "low", false},
		{"Unknown dialect", OpenAIAPI("unknown"), "/v1/responses", "medium", false},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			config := OpenAIProfileConfig{
				HTTPProfileConfig: base,
				API:               test.api,
				ReasoningEffort:   test.effort,
			}
			config.Endpoint = "https://api.example.invalid" + test.path
			if config.valid() != test.valid {
				t.Fatalf("valid = %t, want %t", config.valid(), test.valid)
			}
		})
	}
}

func TestGeminiReplaysThoughtSignaturesAndParallelCorrelationInWireOrder(t *testing.T) {
	t.Parallel()
	conversation, err := newGeminiConversation(
		"https://generativelanguage.example.invalid",
		"gemini-3-pro",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	signatureA := base64.StdEncoding.EncodeToString([]byte("thought-a"))
	signatureB := base64.StdEncoding.EncodeToString([]byte("thought-b"))
	response := []byte(`{
	  "candidates":[{
	    "content":{"role":"model","parts":[
	      {"functionCall":{"id":"provider-a","name":"Read","args":{"path":"/workspace/a"}},"thoughtSignature":"` + signatureA + `"},
	      {"functionCall":{"name":"Read","args":{"path":"/workspace/b"}},"thoughtSignature":"` + signatureB + `"}
	    ]},
	    "finishReason":"STOP"
	  }],
	  "usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":5,"totalTokenCount":9}
	}`)
	turn, err := conversation.accept(response)
	if err != nil || len(turn.Calls) != 2 ||
		turn.Calls[0].ID != "provider-a" ||
		!strings.HasPrefix(turn.Calls[1].ID, "gemini-1-") ||
		turn.Usage.InputTokens != 4 || turn.Usage.OutputTokens != 5 {
		t.Fatalf("Gemini turn = %#v, %v", turn, err)
	}
	if err := conversation.appendResults([]providerToolResult{
		{ID: turn.Calls[0].ID, Name: "Read", Content: []byte("a")},
		{ID: turn.Calls[1].ID, Name: "Read", Content: []byte("b")},
	}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var replay struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				ThoughtSignature string `json:"thoughtSignature"`
				FunctionResponse *struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"functionResponse"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if json.Unmarshal(request.Body, &replay) != nil || len(replay.Contents) != 3 ||
		replay.Contents[1].Role != "model" ||
		replay.Contents[1].Parts[0].ThoughtSignature != signatureA ||
		replay.Contents[1].Parts[1].ThoughtSignature != signatureB ||
		replay.Contents[2].Role != "user" ||
		replay.Contents[2].Parts[0].FunctionResponse.ID != "provider-a" ||
		replay.Contents[2].Parts[1].FunctionResponse.ID != "" {
		t.Fatalf("Gemini replay = %s", request.Body)
	}
	bad := bytes.Replace(response, []byte(signatureA), []byte("not-base64"), 1)
	fresh, _ := newGeminiConversation(
		"https://generativelanguage.example.invalid", "gemini-3-pro",
		toolDefinitions(ReadOnly), []byte(`{}`),
	)
	defer fresh.close()
	if _, err := fresh.accept(bad); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("bad thought signature error = %v", err)
	}
	missing := bytes.Replace(
		response,
		[]byte(`,"thoughtSignature":"`+signatureA+`"`),
		nil,
		1,
	)
	unsigned, _ := newGeminiConversation(
		"https://generativelanguage.example.invalid", "gemini-3-pro",
		toolDefinitions(ReadOnly), []byte(`{}`),
	)
	defer unsigned.close()
	if _, err := unsigned.accept(missing); !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("missing required thought signature error = %v", err)
	}
}
