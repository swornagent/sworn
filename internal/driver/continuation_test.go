package driver

import (
	"bytes"
	"context"
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

func TestAPIContinuationModesAreTruthful(t *testing.T) {
	t.Parallel()
	for dialect, expected := range map[providerDialect]ContinuationMode{
		providerDialectOpenAIChat:      ContinuationModeTranscriptReplay,
		providerDialectOpenAIResponses: ContinuationModeOpaqueReplay,
		providerDialectOpenRouterChat:  ContinuationModeOpaqueReplay,
		providerDialectOpaqueChat:      ContinuationModeOpaqueReplay,
		providerDialectGoogleChat:      ContinuationModeOpaqueReplay,
		providerDialectXAIChat:         ContinuationModeTranscriptReplay,
		providerDialectXAIResponses:    ContinuationModeOpaqueReplay,
		providerDialectGemini:          ContinuationModeOpaqueReplay,
		providerDialectBedrockConverse: ContinuationModeOpaqueReplay,
	} {
		if actual := dialect.continuationMode(); actual != expected {
			t.Fatalf("%s mode = %s, want %s", dialect, actual, expected)
		}
	}
	if actual := providerDialect("other").continuationMode(); actual != "" {
		t.Fatalf("unknown dialect mode = %s", actual)
	}
}

func TestOpaqueChatReplaysReasoningAndExactToolCorrelation(t *testing.T) {
	t.Parallel()
	conversation, err := newOpenAIConversation(
		"https://api.example.invalid/chat/completions",
		"deepseek-reasoner",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectOpaqueChat,
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
		Messages []map[string]any `json:"messages"`
		Tools    []openAITool     `json:"tools"`
	}
	if json.Unmarshal(request.Body, &resumed) != nil ||
		len(resumed.Messages) != 4 ||
		resumed.Messages[1]["reasoning_content"] != "opaque-reasoning" ||
		resumed.Messages[2]["tool_call_id"] != "call-1" ||
		resumed.Messages[3]["role"] != "user" {
		t.Fatalf("DeepSeek resumed replay = %s", request.Body)
	}
	hasWrite := false
	for _, tool := range resumed.Tools {
		hasWrite = hasWrite || tool.Function.Name == "Write"
	}
	if !hasWrite {
		t.Fatalf("DeepSeek resumed tools = %s", request.Body)
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
		providerDialectOpenAIChat,
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
		nil,
		false,
		providerDialectOpenAIResponses,
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
		Tools     []responsesTool    `json:"tools"`
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
	if json.Unmarshal(request.Body, &replay) != nil ||
		len(replay.Input) != 6 ||
		!bytes.Equal(replay.Input[1], reasoning) ||
		!bytes.Equal(replay.Input[2], message) ||
		!bytes.Equal(replay.Input[3], function) {
		t.Fatalf("Responses resumed replay = %s", request.Body)
	}
	var implementationItem struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if json.Unmarshal(replay.Input[5], &implementationItem) != nil ||
		implementationItem.Role != "user" ||
		implementationItem.Content != `{"invocation_id":"implementation"}` {
		t.Fatalf("Responses implementation item = %s", replay.Input[5])
	}
	hasWrite := false
	for _, tool := range replay.Tools {
		hasWrite = hasWrite || tool.Name == "Write"
	}
	if !hasWrite {
		t.Fatalf("Responses resumed tools = %s", request.Body)
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
		name    string
		api     OpenAIAPI
		path    string
		effort  string
		efforts []string
		valid   bool
	}{
		{"Responses custom route", OpenAIResponsesAPI, "/deployments/model/responses", "medium", nil, true},
		{"Responses needs effort", OpenAIResponsesAPI, "/v1/responses", "", nil, false},
		{"Chat compatibility", OpenAIChatCompletionsAPI, "/chat/completions", "", nil, true},
		{"Chat disables reasoning", OpenAIChatCompletionsAPI, "/v1/chat/completions", "none", nil, true},
		{"Chat admits generic reasoning", OpenAIChatCompletionsAPI, "/v1/chat/completions", "low", nil, true},
		{"Chat admits declared reasoning", OpenAIChatCompletionsAPI, "/v1/chat/completions", "high", []string{"high", "max"}, true},
		{"Chat admits declared minimal outside global set", OpenAIChatCompletionsAPI, "/v1/chat/completions", "minimal", []string{"minimal"}, true},
		{"Chat rejects undeclared reasoning", OpenAIChatCompletionsAPI, "/v1/chat/completions", "minimal", []string{"low", "medium"}, false},
		{"Chat rejects unsorted declared vocabulary", OpenAIChatCompletionsAPI, "/v1/chat/completions", "high", []string{"max", "high"}, false},
		{"OpenRouter explicit", OpenRouterChatCompletionsAPI, "/v1/chat/completions", "", nil, true},
		{"OpenRouter admits generic effort", OpenRouterChatCompletionsAPI, "/v1/chat/completions", "none", nil, true},
		{"OpenRouter admits declared reasoning", OpenRouterChatCompletionsAPI, "/v1/chat/completions", "high", []string{"high"}, true},
		{"Responses admits declared minimal", OpenAIResponsesAPI, "/v1/responses", "minimal", []string{"minimal"}, true},
		{"Responses rejects undeclared reasoning", OpenAIResponsesAPI, "/v1/responses", "minimal", []string{"low", "medium"}, false},
		{"Unknown dialect", OpenAIAPI("unknown"), "/v1/responses", "medium", nil, false},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			config := OpenAIProfileConfig{
				HTTPProfileConfig: base,
				API:               test.api,
				ReasoningEffort:   test.effort,
				ReasoningEfforts:  append([]string(nil), test.efforts...),
			}
			config.Endpoint = "https://api.example.invalid" + test.path
			if config.valid() != test.valid {
				t.Fatalf("valid = %t, want %t", config.valid(), test.valid)
			}
		})
	}
}

func TestOpenAIConversationCarriesExactDeclaredReasoningEffort(t *testing.T) {
	t.Parallel()
	conversation, err := newOpenAIConversation(
		"https://api.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		providerDialectOpenAIChat,
		"high",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if json.Unmarshal(request.Body, &envelope) != nil ||
		envelope.Model != "exact-model" ||
		envelope.ReasoningEffort != "high" {
		t.Fatalf("chat request = %s", request.Body)
	}
}

func TestOpenRouterDialectIsExplicitDigestBoundAndClosed(t *testing.T) {
	t.Parallel()
	base := HTTPProfileConfig{
		Key: "openrouter", ID: "sworn.openrouter", Version: "1.0.0",
		Endpoint:         "https://openrouter.example.invalid/v1/chat/completions",
		CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
		CredentialRefs: []string{"credential-ref"},
		ResponseBytes:  MaxProviderResponseBytes,
	}
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("unused"), nil
	}
	standard, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: base,
			API:               OpenAIChatCompletionsAPI,
		},
		resolver,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: base,
			API:               OpenRouterChatCompletionsAPI,
		},
		resolver,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if standard.Identity().ConfigurationDigest ==
		router.Identity().ConfigurationDigest {
		t.Fatal("OpenRouter dialect did not change adapter configuration digest")
	}
	routerLoop := router.(*loopAdapter)
	if routerLoop.surface != ProfileSurfaceOpenRouterChat ||
		routerLoop.dialect != providerDialectOpenRouterChat ||
		routerLoop.dialect.continuationMode() != ContinuationModeOpaqueReplay {
		t.Fatalf("OpenRouter adapter = %#v", routerLoop)
	}
	descriptor, err := (DriverAdapterConfig{OpenAI: &OpenAIProfileConfig{
		HTTPProfileConfig: base,
		API:               OpenRouterChatCompletionsAPI,
	}}).descriptor()
	if err != nil || descriptor.surface != ProfileSurfaceOpenRouterChat {
		t.Fatalf("OpenRouter descriptor = %#v, %v", descriptor, err)
	}
	ref := "credential-ref"
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: "openrouter", Adapter: router.Identity().Key,
			Network: NetworkRequired, CredentialRef: &ref,
		}},
		[]Adapter{router},
	)
	if err != nil {
		t.Fatal(err)
	}
	report := registry.Inspect(
		context.Background(),
		"openrouter",
		"openrouter/model",
	)
	if report.Family != ProfileOpenAIHTTP ||
		report.Surface != ProfileSurfaceOpenRouterChat {
		t.Fatalf("OpenRouter certification report = %#v", report)
	}

	conversation, err := newOpenAIConversation(
		base.Endpoint,
		"openrouter/model",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"design"}`),
		providerDialectOpenRouterChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	details := json.RawMessage(
		`[{"type":"reasoning.encrypted","data":"opaque-provider-state","id":"reasoning-1","format":"anthropic-claude-v1","index":0}]`,
	)
	response := append(
		[]byte(`{"choices":[{"message":{"role":"assistant","content":null,"reasoning_details":`),
		details...,
	)
	response = append(
		response,
		[]byte(`,"tool_calls":[{"id":"router-call","type":"function","function":{"name":"Read","arguments":"{\"path\":\"/workspace/a\"}"}}]},"finish_reason":"tool_calls"}]}`)...,
	)
	turn, err := conversation.accept(response)
	if err != nil || len(turn.Calls) != 1 ||
		turn.Calls[0].ID != "router-call" {
		t.Fatalf("OpenRouter turn = %#v, %v", turn, err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "router-call", Name: "Read", Content: []byte("accepted"),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.resume(
		[]byte(`{"invocation_id":"implementation"}`),
		toolDefinitions(ReadWrite),
	); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var replay struct {
		Messages []openAIMessage `json:"messages"`
		Tools    []openAITool    `json:"tools"`
	}
	if json.Unmarshal(request.Body, &replay) != nil ||
		len(replay.Messages) != 4 ||
		!bytes.Equal(replay.Messages[1].ReasoningDetails, details) ||
		replay.Messages[2].ToolCallID != "router-call" ||
		string(replay.Messages[2].Content) != `"accepted"` ||
		replay.Messages[3].Role != "user" {
		t.Fatalf("OpenRouter replay = %s", request.Body)
	}
	hasWrite := false
	for _, tool := range replay.Tools {
		hasWrite = hasWrite || tool.Function.Name == "Write"
	}
	if !hasWrite {
		t.Fatalf("OpenRouter resume tools = %s", request.Body)
	}

	for _, format := range []string{
		"bedrock-openai-responses-v1",
		"meta-responses-v1",
	} {
		formatDetails := bytes.Replace(
			details,
			[]byte("anthropic-claude-v1"),
			[]byte(format),
			1,
		)
		formatResponse := bytes.Replace(response, details, formatDetails, 1)
		formatConversation, formatErr := newOpenAIConversation(
			base.Endpoint,
			"openrouter/model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			providerDialectOpenRouterChat,
			"",
		)
		if formatErr != nil {
			t.Fatal(formatErr)
		}
		formatTurn, formatErr := formatConversation.accept(formatResponse)
		if formatErr != nil || len(formatTurn.Calls) != 1 {
			formatConversation.close()
			t.Fatalf("%s OpenRouter turn = %#v, %v", format, formatTurn, formatErr)
		}
		if formatErr = formatConversation.appendResults([]providerToolResult{{
			ID: "router-call", Name: "Read", Content: []byte("accepted"),
		}}); formatErr != nil {
			formatConversation.close()
			t.Fatal(formatErr)
		}
		formatRequest, formatErr := formatConversation.request()
		formatConversation.close()
		var formatReplay struct {
			Messages []openAIMessage `json:"messages"`
		}
		if formatErr != nil ||
			json.Unmarshal(formatRequest.Body, &formatReplay) != nil ||
			len(formatReplay.Messages) != 3 ||
			!bytes.Equal(
				formatReplay.Messages[1].ReasoningDetails,
				formatDetails,
			) {
			t.Fatalf("%s OpenRouter replay = %s, %v", format, formatRequest.Body, formatErr)
		}
	}

	for _, dialect := range []providerDialect{
		providerDialectOpenAIChat,
		providerDialectOpaqueChat,
	} {
		foreign, foreignErr := newOpenAIConversation(
			base.Endpoint,
			"exact-model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
			dialect,
			"",
		)
		if foreignErr != nil {
			t.Fatal(foreignErr)
		}
		_, acceptErr := foreign.accept(response)
		foreign.close()
		if !IsCode(acceptErr, "CONTINUATION_INVALID") {
			t.Fatalf("%s accepted OpenRouter state: %v", dialect, acceptErr)
		}
	}
	reordered := bytes.Replace(
		response,
		[]byte(`"index":0`),
		[]byte(`"index":1`),
		1,
	)
	fresh, err := newOpenAIConversation(
		base.Endpoint,
		"openrouter/model",
		toolDefinitions(ReadOnly),
		[]byte(`{}`),
		providerDialectOpenRouterChat,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	_, acceptErr := fresh.accept(reordered)
	fresh.close()
	if !IsCode(acceptErr, "CONTINUATION_INVALID") {
		t.Fatalf("reordered OpenRouter state error = %v", acceptErr)
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
	response := []byte(`{
	  "candidates":[{
	    "content":{"role":"model","parts":[
	      {"functionCall":{"id":"provider-a","name":"Read","args":{"path":"/workspace/a"}},"thoughtSignature":"` + signatureA + `"},
	      {"functionCall":{"name":"Read","args":{"path":"/workspace/b"}}}
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
		replay.Contents[1].Parts[1].ThoughtSignature != "" ||
		replay.Contents[2].Role != "user" ||
		replay.Contents[2].Parts[0].FunctionResponse.ID != "provider-a" ||
		replay.Contents[2].Parts[1].FunctionResponse.ID != "" {
		t.Fatalf("Gemini replay = %s", request.Body)
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
		Contents []geminiContent `json:"contents"`
		Tools    []struct {
			FunctionDeclarations []geminiDeclaration `json:"functionDeclarations"`
		} `json:"tools"`
	}
	if json.Unmarshal(request.Body, &resumed) != nil ||
		len(resumed.Contents) != 3 ||
		len(resumed.Contents[2].Parts) != 3 ||
		resumed.Contents[2].Parts[0].FunctionResponse == nil ||
		resumed.Contents[2].Parts[1].FunctionResponse == nil ||
		resumed.Contents[2].Parts[2].Text == nil ||
		*resumed.Contents[2].Parts[2].Text !=
			`{"invocation_id":"implementation"}` {
		t.Fatalf("Gemini resumed replay = %s", request.Body)
	}
	hasWrite := false
	for _, declaration := range resumed.Tools[0].FunctionDeclarations {
		hasWrite = hasWrite || declaration.Name == "Write"
	}
	if !hasWrite {
		t.Fatalf("Gemini resumed tools = %s", request.Body)
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
