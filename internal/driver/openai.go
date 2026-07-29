package driver

import (
	"encoding/json"
	"net/http"
)

type openAIConversation struct {
	endpoint string
	model    string
	deepSeek bool
	tools    []openAITool
	messages []openAIMessage
	pending  []providerToolCall
	ledger   *continuationLedger
}

type openAIMessage struct {
	Role             string           `json:"role"`
	Content          json.RawMessage  `json:"content,omitempty"`
	ReasoningContent json.RawMessage  `json:"reasoning_content,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

func NewOpenAICompatibleAdapter(
	config HTTPProfileConfig,
	resolver HeaderCredentialResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (Adapter, error) {
	return newOpenAIAdapter(config, resolver, probe, roundTripper, false)
}

func NewDeepSeekAdapter(
	config HTTPProfileConfig,
	resolver HeaderCredentialResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (Adapter, error) {
	return newOpenAIAdapter(config, resolver, probe, roundTripper, true)
}

func newOpenAIAdapter(
	config HTTPProfileConfig,
	resolver HeaderCredentialResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
	deepSeek bool,
) (Adapter, error) {
	transport, err := newHTTPTransport(config, resolver, probe, roundTripper)
	if err != nil {
		return nil, err
	}
	family := ProfileOpenAIHTTP
	if deepSeek {
		family = ProfileDeepSeek
	}
	factory := func(
		prompt []byte,
		model string,
		tools []providerToolDefinition,
	) (providerConversation, error) {
		return newOpenAIConversation(config.Endpoint, model, tools, prompt, deepSeek)
	}
	configuration := struct {
		HTTPProfileConfig
		Family ProfileFamily
	}{transport.config, family}
	return newLoopAdapter(
		config.Key,
		config.ID,
		config.Version,
		family,
		"",
		configuration,
		factory,
		transport,
	)
}

func newOpenAIConversation(
	endpoint, model string,
	definitions []providerToolDefinition,
	prompt []byte,
	deepSeek bool,
) (*openAIConversation, error) {
	if validateEndpoint(endpoint) != nil || validateText(model, 500, false) != nil {
		return nil, fail("INVALID_ADAPTER")
	}
	tools := make([]openAITool, len(definitions))
	for index, definition := range definitions {
		tools[index].Type = "function"
		tools[index].Function.Name = definition.Name
		tools[index].Function.Description = definition.Description
		tools[index].Function.Parameters = append([]byte(nil), definition.InputSchema...)
	}
	content, _ := json.Marshal(string(prompt))
	return &openAIConversation{
		endpoint: endpoint,
		model:    model,
		deepSeek: deepSeek,
		tools:    tools,
		messages: []openAIMessage{{Role: "user", Content: content}},
		ledger:   newContinuationLedger(),
	}, nil
}

func (conversation *openAIConversation) request() (providerRequest, error) {
	if conversation == nil || len(conversation.pending) != 0 {
		return providerRequest{}, fail("CONTINUATION_INVALID")
	}
	body, err := json.Marshal(struct {
		Model      string          `json:"model"`
		Messages   []openAIMessage `json:"messages"`
		Tools      []openAITool    `json:"tools"`
		ToolChoice string          `json:"tool_choice"`
		Stream     bool            `json:"stream"`
	}{
		Model: conversation.model, Messages: conversation.messages,
		Tools: conversation.tools, ToolChoice: "auto", Stream: false,
	})
	if err != nil || len(body) > MaxProviderRequestBytes {
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	return providerRequest{
		Method: "POST", URL: conversation.endpoint,
		ContentType: "application/json", Body: body,
	}, nil
}

func (conversation *openAIConversation) accept(body []byte) (providerTurn, error) {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxProviderResponseBytes {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	value, err := decodeStrict(body, MaxProviderResponseBytes)
	if err != nil {
		return providerTurn{}, err
	}
	root, err := closedObject(value, nil, []string{
		"id", "object", "created", "model", "choices", "usage", "error",
		"system_fingerprint", "service_tier",
	})
	if err != nil {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if providerError, present := root["error"]; present && providerError != nil {
		return providerTurn{}, fail("PROVIDER_ERROR")
	}
	rawChoices, ok := root["choices"].([]any)
	if !ok || len(rawChoices) != 1 {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	choice, err := closedObject(
		rawChoices[0],
		[]string{"message", "finish_reason"},
		[]string{"index", "logprobs"},
	)
	if err != nil || choice["finish_reason"] != "tool_calls" {
		return providerTurn{}, fail("MISSING_SUBMISSION")
	}
	rawMessage, err := closedObject(
		choice["message"],
		[]string{"role", "content", "tool_calls"},
		[]string{"reasoning", "reasoning_content", "refusal", "annotations"},
	)
	if err != nil || rawMessage["role"] != "assistant" {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if refusal, present := rawMessage["refusal"]; present && refusal != nil {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if annotations, present := rawMessage["annotations"]; present {
		array, ok := annotations.([]any)
		if !ok || len(array) != 0 {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
	}
	if reasoningValue, present := rawMessage["reasoning"]; present &&
		reasoningValue != nil {
		reasoning, ok := reasoningValue.(string)
		if !ok || len(reasoning) > MaxOpaqueFieldBytes ||
			!validOpaqueText([]byte(reasoning)) {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
	}
	rawToolCalls, ok := rawMessage["tool_calls"].([]any)
	if !ok || len(rawToolCalls) == 0 || len(rawToolCalls) > MaxToolCalls {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	message := openAIMessage{Role: "assistant"}
	switch content := rawMessage["content"].(type) {
	case nil:
		message.Content = json.RawMessage("null")
	case string:
		if !validOpaqueText([]byte(content)) {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		message.Content, _ = json.Marshal(content)
	default:
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if reasoningValue, present := rawMessage["reasoning_content"]; present &&
		reasoningValue != nil {
		if !conversation.deepSeek {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		reasoning, ok := reasoningValue.(string)
		if !ok {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		retained, err := conversation.ledger.retain(opaqueField{
			kind: opaqueText,
			body: []byte(reasoning),
		})
		if err != nil {
			return providerTurn{}, err
		}
		message.ReasoningContent, _ = json.Marshal(string(retained[0]))
	}
	calls := make([]providerToolCall, 0, len(rawToolCalls))
	for _, rawToolCall := range rawToolCalls {
		toolCall, toolErr := closedObject(
			rawToolCall,
			[]string{"id", "type", "function"},
			[]string{"index"},
		)
		function, functionErr := closedObject(
			toolCall["function"],
			[]string{"name", "arguments"},
			nil,
		)
		id, idOK := toolCall["id"].(string)
		name, nameOK := function["name"].(string)
		argumentsText, argumentsOK := function["arguments"].(string)
		if toolErr != nil || functionErr != nil ||
			toolCall["type"] != "function" || !idOK || !nameOK || !argumentsOK ||
			conversation.ledger.correlate(id) != nil ||
			!providerKeyPattern.MatchString(name) ||
			len(argumentsText) == 0 ||
			len(argumentsText) > MaxToolArgumentBytes {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		arguments := []byte(argumentsText)
		if _, err := decodeStrict(arguments, MaxToolArgumentBytes); err != nil {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		calls = append(calls, providerToolCall{
			ID: id, Name: name,
			Arguments: append([]byte(nil), arguments...),
		})
		encodedArguments, _ := json.Marshal(argumentsText)
		message.ToolCalls = append(message.ToolCalls, openAIToolCall{
			ID: id, Type: "function",
			Function: openAIFunction{
				Name: name, Arguments: encodedArguments,
			},
		})
	}
	conversation.messages = append(conversation.messages, message)
	conversation.pending = append([]providerToolCall(nil), calls...)
	turn := providerTurn{Calls: calls}
	if usageValue, present := root["usage"]; present && usageValue != nil {
		usage, usageErr := closedObject(
			usageValue,
			[]string{"prompt_tokens", "completion_tokens"},
			[]string{
				"total_tokens", "prompt_tokens_details", "completion_tokens_details",
				"prompt_cache_hit_tokens", "prompt_cache_miss_tokens",
			},
		)
		input, inputOK := safeJSONInt(usage["prompt_tokens"])
		output, outputOK := safeJSONInt(usage["completion_tokens"])
		if usageErr != nil || !inputOK || !outputOK {
			return providerTurn{}, fail("INVALID_USAGE")
		}
		turn.Usage = &Usage{
			InputTokens: input, OutputTokens: output,
		}
	}
	return turn, nil
}

func (conversation *openAIConversation) appendResults(results []providerToolResult) error {
	if conversation == nil || len(results) != len(conversation.pending) {
		return fail("CONTINUATION_INVALID")
	}
	for index, result := range results {
		expected := conversation.pending[index]
		if result.ID != expected.ID || result.Name != expected.Name ||
			len(result.Content) > MaxToolResultBytes || !validOpaqueText(result.Content) {
			return fail("CONTINUATION_INVALID")
		}
		content, _ := json.Marshal(string(result.Content))
		conversation.messages = append(conversation.messages, openAIMessage{
			Role: "tool", ToolCallID: result.ID, Content: content,
		})
	}
	conversation.pending = nil
	return nil
}

func (conversation *openAIConversation) close() {
	if conversation == nil {
		return
	}
	conversation.ledger.Close()
	for index := range conversation.messages {
		clearBytes(conversation.messages[index].Content)
		clearBytes(conversation.messages[index].ReasoningContent)
		for tool := range conversation.messages[index].ToolCalls {
			clearBytes(conversation.messages[index].ToolCalls[tool].Function.Arguments)
		}
	}
	conversation.messages = nil
	conversation.pending = nil
	conversation.tools = nil
}
