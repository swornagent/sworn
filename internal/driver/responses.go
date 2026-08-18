package driver

import "encoding/json"

type responsesConversation struct {
	endpoint        string
	model           string
	dialect         providerDialect
	reasoningEffort string
	enableThinking  *bool
	stream          bool
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
	enableThinking *bool,
	stream bool,
	dialect providerDialect,
) (*responsesConversation, error) {
	if validateEndpoint(endpoint) != nil ||
		validateText(model, 500, false) != nil ||
		reasoningEffort == "" ||
		(dialect != providerDialectOpenAIResponses &&
			dialect != providerDialectXAIResponses) {
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
		dialect:         dialect,
		reasoningEffort: reasoningEffort,
		enableThinking:  enableThinking,
		stream:          stream,
		tools:           tools,
		input:           []json.RawMessage{initial},
		ledger:          newContinuationLedger(),
	}, nil
}

func responsesTools(
	definitions []providerToolDefinition,
) ([]responsesTool, error) {
	if len(definitions) == 0 || len(definitions) > MaxToolCalls {
		return nil, failContinuation("continuation.responses.tool_count_out_of_bounds")
	}
	tools := make([]responsesTool, len(definitions))
	for index, definition := range definitions {
		if !providerKeyPattern.MatchString(definition.Name) ||
			len(definition.InputSchema) == 0 ||
			len(definition.InputSchema) > MaxToolArgumentBytes {
			return nil, failContinuation("continuation.responses.invalid_tool_definition")
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
		return providerRequest{}, failContinuation("continuation.responses.request_pending_tool_calls")
	}
	body, err := json.Marshal(struct {
		Model             string             `json:"model"`
		Input             []json.RawMessage  `json:"input"`
		Tools             []responsesTool    `json:"tools"`
		ToolChoice        string             `json:"tool_choice"`
		ParallelToolCalls bool               `json:"parallel_tool_calls"`
		Reasoning         responsesReasoning `json:"reasoning"`
		EnableThinking    *bool              `json:"enable_thinking,omitempty"`
		Store             bool               `json:"store"`
		Stream            bool               `json:"stream"`
	}{
		Model:             conversation.model,
		Input:             conversation.input,
		Tools:             conversation.tools,
		ToolChoice:        "auto",
		ParallelToolCalls: true,
		Reasoning:         responsesReasoning{Effort: conversation.reasoningEffort},
		EnableThinking:    conversation.enableThinking,
		Store:             false,
		Stream:            conversation.stream,
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
		Stream:      conversation.stream,
	}, nil
}

func (conversation *responsesConversation) accept(
	body []byte,
) (providerTurn, error) {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxProviderResponseBytes {
		liveStream.driverError("accept-site-1", nil)
		return providerTurn{}, failContinuation("continuation.responses.accept_invalid_state_or_body_size")
	}
	value, err := decodeStrict(body, MaxProviderResponseBytes)
	if err != nil {
		return providerTurn{}, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		liveStream.driverError("accept-site-2", nil)
		return providerTurn{}, failContinuation("continuation.responses.accept_root_not_object")
	}
	if providerError, present := root["error"]; present && providerError != nil {
		return providerTurn{}, fail("PROVIDER_ERROR")
	}
	var effort *string
	// The Responses root is deliberately not closed-object validated (it
	// carries many provider-specific fields), so reasoning.effort is read
	// leniently: a malformed measurement is ignored rather than failing the
	// run, in line with "measurement never becomes a gate".
	if reasoningValue, present := root["reasoning"]; present &&
		reasoningValue != nil {
		if reasoning, reasoningOK := reasoningValue.(map[string]any); reasoningOK {
			if effortValue, present := reasoning["effort"]; present &&
				effortValue != nil {
				if effortText, effortOK := effortValue.(string); effortOK &&
					validateText(effortText, 128, false) == nil {
					effort = &effortText
				}
			}
		}
	}
	status, statusOK := root["status"].(string)
	if !statusOK {
		return providerTurn{}, failContinuation("continuation.responses.accept_status_missing")
	}
	if status != "completed" {
		if status != "incomplete" || !responsesTruncated(root) {
			return providerTurn{}, fail("MISSING_SUBMISSION")
		}
		// The provider hit its output-token ceiling: report the explicit
		// named failure carrying the provider's own finish reason instead of
		// an empty-looking success.
		reason := "max_output_tokens"
		turn := providerTurn{
			ReasoningEffort: effort,
			FinishReason:    &reason,
			Truncated:       true,
		}
		if usageValue, present := root["usage"]; present && usageValue != nil {
			usage, usageErr := responsesUsage(usageValue, conversation.dialect)
			if usageErr != nil {
				return providerTurn{}, usageErr
			}
			turn.Usage = usage
		}
		return turn, nil
	}
	output, ok := root["output"].([]any)
	if !ok || len(output) == 0 ||
		len(output) > MaxToolCalls+MaxContinuationSteps {
		liveStream.driverError("accept-site-3", nil)
		return providerTurn{}, failContinuation("continuation.responses.accept_output_invalid_or_out_of_bounds")
	}
	var rawResponse struct {
		Output []json.RawMessage `json:"output"`
	}
	if json.Unmarshal(body, &rawResponse) != nil ||
		len(rawResponse.Output) != len(output) {
		liveStream.driverError("accept-site-4", nil)
		return providerTurn{}, failContinuation("continuation.responses.accept_output_raw_unmarshal_mismatch")
	}
	calls := make([]providerToolCall, 0, len(output))
	retainedFields := make([]opaqueField, 0, len(output))
	for index := range output {
		item, itemOK := output[index].(map[string]any)
		itemType, typeOK := item["type"].(string)
		if !itemOK || !typeOK {
			liveStream.driverError("accept-site-5", nil)
			return providerTurn{}, failContinuation("continuation.responses.accept_item_not_object_or_missing_type")
		}
		switch itemType {
		case "reasoning":
			if validateResponsesReasoningItem(item) != nil {
				liveStream.driverError("accept-site-6", nil)
				return providerTurn{}, failContinuation("continuation.responses.accept_reasoning_invalid")
			}
		case "message":
			if !validResponsesMessageItem(item) {
				liveStream.driverError("accept-site-7", nil)
				return providerTurn{}, failContinuation("continuation.responses.accept_message_invalid")
			}
		case "function_call":
			call, callErr := responsesFunctionCall(conversation.ledger, item)
			if callErr != nil {
				return providerTurn{}, callErr
			}
			calls = append(calls, call)
		default:
			liveStream.driverError("accept-site-8", nil)
			return providerTurn{}, failContinuation("continuation.responses.accept_unknown_item_type")
		}
		retainedFields = append(retainedFields, opaqueField{
			kind: opaqueText,
			body: rawResponse.Output[index],
		})
	}
	retained, err := conversation.ledger.retain(retainedFields...)
	if err != nil {
		return providerTurn{}, err
	}
	for _, item := range retained {
		conversation.input = append(conversation.input, item)
	}
	conversation.pending = append([]providerToolCall(nil), calls...)
	turn := providerTurn{Calls: calls, Prose: len(calls) == 0}
	if effort != nil {
		turn.ReasoningEffort = effort
	}
	if usageValue, present := root["usage"]; present && usageValue != nil {
		usage, usageErr := responsesUsage(usageValue, conversation.dialect)
		if usageErr != nil {
			return providerTurn{}, usageErr
		}
		turn.Usage = usage
	}
	return turn, nil
}

// responsesTruncated reports whether an incomplete Responses response was cut
// by the provider's output-token ceiling (incomplete_details.reason
// "max_output_tokens"). The root object is not closed-object validated, so
// the detail is read leniently; any other incompletion keeps the prior
// MISSING_SUBMISSION behavior.
func responsesTruncated(root map[string]any) bool {
	details, present := root["incomplete_details"]
	if !present {
		return false
	}
	detailsObject, ok := details.(map[string]any)
	if !ok {
		return false
	}
	reason, ok := detailsObject["reason"].(string)
	return ok && reason == "max_output_tokens"
}

// responsesUsage parses the Responses usage object, surfacing the cached-token
// detail (input_tokens_details.cached_tokens) as cache reads instead of
// discarding it. The details object carries provider-specific breakdown fields
// (audio, image, text tokens), so only cached_tokens is extracted and a
// malformed detail never fails the run. On the xAI responses dialect the
// recorded vendor usage decorations are admitted at this position and
// tolerated-and-ignored, so the normalized accounting still reads only the
// standard fields.
func responsesUsage(value any, dialect providerDialect) (*Usage, error) {
	// x_details is Qwen's provider-specific usage annex on the responses
	// flavour; it is tolerated and ignored rather than failing the turn.
	optional := []string{"input_tokens_details", "output_tokens_details", "x_details"}
	if dialect == providerDialectXAIResponses {
		optional = append(optional,
			"num_sources_used",
			"num_server_side_tools_used",
			"cost_in_usd_ticks",
			"context_details",
		)
	}
	usage, err := closedObject(
		value,
		[]string{"input_tokens", "output_tokens", "total_tokens"},
		optional,
	)
	if err != nil {
		return nil, fail("INVALID_USAGE")
	}
	input, inputOK := safeJSONInt(usage["input_tokens"])
	outputTokens, outputOK := safeJSONInt(usage["output_tokens"])
	if !inputOK || !outputOK {
		return nil, fail("INVALID_USAGE")
	}
	result := &Usage{InputTokens: input, OutputTokens: outputTokens}
	if detailsValue, present := usage["input_tokens_details"]; present &&
		detailsValue != nil {
		details, detailsOK := detailsValue.(map[string]any)
		if detailsOK {
			if cachedValue, present := details["cached_tokens"]; present {
				cached, cachedOK := safeJSONInt(cachedValue)
				if cachedOK {
					result.CacheReadTokens = &cached
				}
			}
		}
	}
	return result, nil
}

func (conversation *responsesConversation) appendInstruction(body []byte) error {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxOpaqueFieldBytes ||
		!validOpaqueText(body) {
		return failContinuation("continuation.responses.append_instruction_invalid")
	}
	message, err := json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: string(body)})
	if err != nil || len(message) > MaxOpaqueFieldBytes {
		clearBytes(message)
		return failContinuation("continuation.responses.append_instruction_encode_failed")
	}
	conversation.input = append(conversation.input, message)
	return nil
}

func validateResponsesReasoningItem(item map[string]any) error {
	if item["type"] != "reasoning" {
		return failContinuation("continuation.responses.reasoning_type_mismatch")
	}
	if encrypted, encryptedOK := item["encrypted_content"].(string); encryptedOK {
		if len(encrypted) == 0 || len(encrypted) > MaxOpaqueFieldBytes ||
			!validOpaqueText([]byte(encrypted)) {
			return failContinuation("continuation.responses.reasoning_encrypted_content_invalid")
		}
		return nil
	}
	// DeepSeek's Responses dialect carries plaintext reasoning as
	// content parts of type reasoning_text instead of encrypted_content;
	// Qwen's carries a summary list of summary_text parts with null content.
	if parts, partsOK := item["content"].([]any); partsOK && len(parts) > 0 {
		for _, rawPart := range parts {
			part, partOK := rawPart.(map[string]any)
			if !partOK || part["type"] != "reasoning_text" {
				return failContinuation("continuation.responses.reasoning_part_type_mismatch")
			}
			text, textOK := part["text"].(string)
			if !textOK || len(text) > MaxOpaqueFieldBytes ||
				!validOpaqueText([]byte(text)) {
				return failContinuation("continuation.responses.reasoning_part_text_invalid")
			}
		}
		return nil
	}
	summary, summaryOK := item["summary"].([]any)
	if !summaryOK || len(summary) == 0 {
		return failContinuation("continuation.responses.reasoning_summary_missing")
	}
	for _, rawPart := range summary {
		part, partOK := rawPart.(map[string]any)
		if !partOK || part["type"] != "summary_text" {
			return failContinuation("continuation.responses.reasoning_summary_part_type_mismatch")
		}
		text, textOK := part["text"].(string)
		if !textOK || len(text) > MaxOpaqueFieldBytes ||
			!validOpaqueText([]byte(text)) {
			return failContinuation("continuation.responses.reasoning_summary_text_invalid")
		}
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
	if item["type"] != "function_call" {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.item_type_mismatch")
	}
	callID, callIDOK := item["call_id"].(string)
	if !callIDOK {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.missing_call_id")
	}
	name, nameOK := item["name"].(string)
	if !nameOK {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.missing_name")
	}
	argumentsText, argumentsOK := item["arguments"].(string)
	if !argumentsOK {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.missing_arguments")
	}
	status, hasStatus := item["status"]
	if hasStatus && status != "completed" {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.status_incomplete")
	}
	if err := ledger.correlate(callID); err != nil {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.correlate_reuse")
	}
	if !providerKeyPattern.MatchString(name) {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.invalid_name_pattern")
	}
	if len(argumentsText) == 0 || len(argumentsText) > MaxToolArgumentBytes {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.arguments_length_out_of_bounds")
	}
	arguments := []byte(argumentsText)
	if _, err := decodeStrict(arguments, MaxToolArgumentBytes); err != nil {
		return providerToolCall{}, failContinuation("continuation.toolcall_decode.arguments_json_invalid")
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
		return failContinuation("continuation.responses.append_results_pending_mismatch")
	}
	for index, result := range results {
		expected := conversation.pending[index]
		if result.ID != expected.ID || result.Name != expected.Name ||
			len(result.Content) > MaxToolResultBytes ||
			!validOpaqueText(result.Content) {
			return failContinuation("continuation.responses.append_results_mismatch_or_invalid")
		}
		item, err := json.Marshal(responsesFunctionOutput{
			Type:   "function_call_output",
			CallID: result.ID,
			Output: string(result.Content),
		})
		if err != nil || len(item) > MaxOpaqueFieldBytes {
			return failContinuation("continuation.responses.append_results_marshal_failed")
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
		return failContinuation("continuation.responses.resume_invalid_state_or_tail")
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
		return failContinuation("continuation.responses.resume_marshal_failed")
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
	conversation.dialect = ""
	conversation.reasoningEffort = ""
	conversation.input = nil
	conversation.pending = nil
	conversation.tools = nil
}

func (conversation *responsesConversation) declaredReasoningEffort() string {
	if conversation == nil {
		return ""
	}
	return conversation.reasoningEffort
}

func clearResponsesTools(tools []responsesTool) {
	for index := range tools {
		clearBytes(tools[index].Parameters)
	}
}
