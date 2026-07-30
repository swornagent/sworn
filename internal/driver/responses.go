package driver

import "encoding/json"

type responsesConversation struct {
	endpoint        string
	model           string
	reasoningEffort string
	tools           []responsesTool
	input           []json.RawMessage
	pending         []providerToolCall
	ledger          *continuationLedger
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

type responsesReasoning struct {
	Effort string `json:"effort"`
}

type responsesFunctionOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

func newResponsesConversation(
	endpoint, model string,
	definitions []providerToolDefinition,
	prompt []byte,
	reasoningEffort string,
) (*responsesConversation, error) {
	if validateEndpoint(endpoint) != nil ||
		validateText(model, 500, false) != nil ||
		reasoningEffort == "" ||
		!validOpenAIReasoningEffort(reasoningEffort) {
		return nil, fail("INVALID_ADAPTER")
	}
	tools, err := responsesTools(definitions)
	if err != nil {
		return nil, err
	}
	initial, err := json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: string(prompt)})
	if err != nil || len(initial) > MaxOpaqueFieldBytes {
		return nil, fail("RESOURCE_LIMIT")
	}
	return &responsesConversation{
		endpoint:        endpoint,
		model:           model,
		reasoningEffort: reasoningEffort,
		tools:           tools,
		input:           []json.RawMessage{initial},
		ledger:          newContinuationLedger(),
	}, nil
}

func responsesTools(
	definitions []providerToolDefinition,
) ([]responsesTool, error) {
	if len(definitions) == 0 || len(definitions) > MaxToolCalls {
		return nil, fail("CONTINUATION_INVALID")
	}
	tools := make([]responsesTool, len(definitions))
	for index, definition := range definitions {
		if !providerKeyPattern.MatchString(definition.Name) ||
			len(definition.InputSchema) == 0 ||
			len(definition.InputSchema) > MaxToolArgumentBytes {
			return nil, fail("CONTINUATION_INVALID")
		}
		tools[index] = responsesTool{
			Type:        "function",
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  append([]byte(nil), definition.InputSchema...),
			Strict:      false,
		}
	}
	return tools, nil
}

func (conversation *responsesConversation) request() (providerRequest, error) {
	if conversation == nil || len(conversation.pending) != 0 {
		return providerRequest{}, fail("CONTINUATION_INVALID")
	}
	body, err := json.Marshal(struct {
		Model             string             `json:"model"`
		Input             []json.RawMessage  `json:"input"`
		Tools             []responsesTool    `json:"tools"`
		ToolChoice        string             `json:"tool_choice"`
		ParallelToolCalls bool               `json:"parallel_tool_calls"`
		Reasoning         responsesReasoning `json:"reasoning"`
		Store             bool               `json:"store"`
		Stream            bool               `json:"stream"`
	}{
		Model:             conversation.model,
		Input:             conversation.input,
		Tools:             conversation.tools,
		ToolChoice:        "auto",
		ParallelToolCalls: true,
		Reasoning:         responsesReasoning{Effort: conversation.reasoningEffort},
		Store:             false,
		Stream:            false,
	})
	if err != nil || len(body) > MaxProviderRequestBytes {
		clearBytes(body)
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	return providerRequest{
		Method:      "POST",
		URL:         conversation.endpoint,
		ContentType: "application/json",
		Body:        body,
	}, nil
}

func (conversation *responsesConversation) accept(
	body []byte,
) (providerTurn, error) {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxProviderResponseBytes {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	value, err := decodeStrict(body, MaxProviderResponseBytes)
	if err != nil {
		return providerTurn{}, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if providerError, present := root["error"]; present && providerError != nil {
		return providerTurn{}, fail("PROVIDER_ERROR")
	}
	if root["status"] != "completed" {
		return providerTurn{}, fail("MISSING_SUBMISSION")
	}
	output, ok := root["output"].([]any)
	if !ok || len(output) == 0 ||
		len(output) > MaxToolCalls+MaxContinuationSteps {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	var rawResponse struct {
		Output []json.RawMessage `json:"output"`
	}
	if json.Unmarshal(body, &rawResponse) != nil ||
		len(rawResponse.Output) != len(output) {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	calls := make([]providerToolCall, 0, len(output))
	retainedFields := make([]opaqueField, 0, len(output))
	for index := range output {
		item, itemOK := output[index].(map[string]any)
		itemType, typeOK := item["type"].(string)
		if !itemOK || !typeOK {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		switch itemType {
		case "reasoning":
			if validateResponsesReasoningItem(item) != nil {
				return providerTurn{}, fail("CONTINUATION_INVALID")
			}
		case "message":
			if !validResponsesMessageItem(item) {
				return providerTurn{}, fail("CONTINUATION_INVALID")
			}
		case "function_call":
			call, callErr := responsesFunctionCall(conversation.ledger, item)
			if callErr != nil {
				return providerTurn{}, callErr
			}
			calls = append(calls, call)
		default:
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		retainedFields = append(retainedFields, opaqueField{
			kind: opaqueText,
			body: rawResponse.Output[index],
		})
	}
	if len(calls) == 0 {
		return providerTurn{}, fail("MISSING_SUBMISSION")
	}
	retained, err := conversation.ledger.retain(retainedFields...)
	if err != nil {
		return providerTurn{}, err
	}
	for _, item := range retained {
		conversation.input = append(conversation.input, item)
	}
	conversation.pending = append([]providerToolCall(nil), calls...)
	turn := providerTurn{Calls: calls}
	if usageValue, present := root["usage"]; present && usageValue != nil {
		usage, usageErr := closedObject(
			usageValue,
			[]string{"input_tokens", "output_tokens", "total_tokens"},
			[]string{"input_tokens_details", "output_tokens_details"},
		)
		input, inputOK := safeJSONInt(usage["input_tokens"])
		outputTokens, outputOK := safeJSONInt(usage["output_tokens"])
		if usageErr != nil || !inputOK || !outputOK {
			return providerTurn{}, fail("INVALID_USAGE")
		}
		turn.Usage = &Usage{
			InputTokens:  input,
			OutputTokens: outputTokens,
		}
	}
	return turn, nil
}

func validateResponsesReasoningItem(item map[string]any) error {
	encrypted, encryptedOK := item["encrypted_content"].(string)
	if item["type"] != "reasoning" || !encryptedOK ||
		len(encrypted) == 0 || len(encrypted) > MaxOpaqueFieldBytes ||
		!validOpaqueText([]byte(encrypted)) {
		return fail("CONTINUATION_INVALID")
	}
	return nil
}

func validResponsesMessageItem(item map[string]any) bool {
	status, hasStatus := item["status"]
	return item["type"] == "message" &&
		item["role"] == "assistant" &&
		(!hasStatus || status == "completed")
}

func responsesFunctionCall(
	ledger *continuationLedger,
	item map[string]any,
) (providerToolCall, error) {
	callID, callIDOK := item["call_id"].(string)
	name, nameOK := item["name"].(string)
	argumentsText, argumentsOK := item["arguments"].(string)
	status, hasStatus := item["status"]
	if item["type"] != "function_call" ||
		!callIDOK || !nameOK || !argumentsOK ||
		(hasStatus && status != "completed") ||
		ledger.correlate(callID) != nil ||
		!providerKeyPattern.MatchString(name) ||
		len(argumentsText) == 0 ||
		len(argumentsText) > MaxToolArgumentBytes {
		return providerToolCall{}, fail("CONTINUATION_INVALID")
	}
	arguments := []byte(argumentsText)
	if _, err := decodeStrict(arguments, MaxToolArgumentBytes); err != nil {
		return providerToolCall{}, fail("CONTINUATION_INVALID")
	}
	return providerToolCall{
		ID:        callID,
		Name:      name,
		Arguments: append([]byte(nil), arguments...),
	}, nil
}

func (conversation *responsesConversation) appendResults(
	results []providerToolResult,
) error {
	if conversation == nil || len(results) != len(conversation.pending) {
		return fail("CONTINUATION_INVALID")
	}
	for index, result := range results {
		expected := conversation.pending[index]
		if result.ID != expected.ID || result.Name != expected.Name ||
			len(result.Content) > MaxToolResultBytes ||
			!validOpaqueText(result.Content) {
			return fail("CONTINUATION_INVALID")
		}
		item, err := json.Marshal(responsesFunctionOutput{
			Type:   "function_call_output",
			CallID: result.ID,
			Output: string(result.Content),
		})
		if err != nil || len(item) > MaxOpaqueFieldBytes {
			return fail("CONTINUATION_INVALID")
		}
		conversation.input = append(conversation.input, item)
	}
	conversation.pending = nil
	return nil
}

func (conversation *responsesConversation) resume(
	prompt []byte,
	definitions []providerToolDefinition,
) error {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(prompt) == 0 || len(prompt) > MaxProviderRequestBytes ||
		!validResponsesResumeTail(conversation.input) {
		return fail("CONTINUATION_INVALID")
	}
	tools, err := responsesTools(definitions)
	if err != nil {
		return err
	}
	item, err := json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: string(prompt)})
	if err != nil || len(item) > MaxProviderRequestBytes {
		clearBytes(item)
		clearResponsesTools(tools)
		return fail("CONTINUATION_INVALID")
	}
	clearResponsesTools(conversation.tools)
	conversation.tools = tools
	conversation.input = append(conversation.input, item)
	return nil
}

func validResponsesResumeTail(input []json.RawMessage) bool {
	if len(input) < 3 {
		return false
	}
	value, err := decodeStrict(input[len(input)-1], MaxOpaqueFieldBytes)
	if err != nil {
		return false
	}
	result, err := closedObject(
		value,
		[]string{"type", "call_id", "output"},
		nil,
	)
	callID, callIDOK := result["call_id"].(string)
	output, outputOK := result["output"].(string)
	if err != nil || result["type"] != "function_call_output" ||
		!callIDOK ||
		validateText(callID, MaxCorrelationIDBytes, false) != nil ||
		!outputOK || !validOpaqueText([]byte(output)) {
		return false
	}
	for index := len(input) - 2; index > 0; index-- {
		var call struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
			Name   string `json:"name"`
		}
		if json.Unmarshal(input[index], &call) == nil &&
			call.Type == "function_call" &&
			call.CallID == callID &&
			providerKeyPattern.MatchString(call.Name) {
			return true
		}
	}
	return false
}

func (conversation *responsesConversation) close() {
	if conversation == nil {
		return
	}
	conversation.ledger.Close()
	for _, item := range conversation.input {
		clearBytes(item)
	}
	clearResponsesTools(conversation.tools)
	conversation.endpoint = ""
	conversation.model = ""
	conversation.reasoningEffort = ""
	conversation.input = nil
	conversation.pending = nil
	conversation.tools = nil
}

func clearResponsesTools(tools []responsesTool) {
	for index := range tools {
		clearBytes(tools[index].Parameters)
	}
}
