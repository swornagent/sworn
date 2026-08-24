package driver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type BedrockProfileConfig struct {
	Key               string       `json:"key"`
	ID                string       `json:"id"`
	Version           string       `json:"version"`
	Endpoint          string       `json:"endpoint"`
	CredentialRefs    []string     `json:"credential_refs"`
	ResponseBytes     int          `json:"response_bytes"`
	Chain             AWSChainSpec `json:"chain"`
	AllowCachePoint   bool         `json:"allow_cache_point"`
	AllowGuardContent bool         `json:"allow_guard_content"`
}

type bedrockTransport struct {
	config    BedrockProfileConfig
	surface   ProfileSurface
	resolve   AWSRuntimeResolver
	liveProbe ProfileLiveProbe
	client    *http.Client
	refs      map[string]struct{}
	runAWS    awsCommandRunner
}

type bedrockConversation struct {
	endpoint          string
	model             string
	allowCachePoint   bool
	allowGuardContent bool
	system            string
	tools             []bedrockTool
	messages          []json.RawMessage
	pending           []providerToolCall
	ledger            *continuationLedger
	// maxOutputTokens is the operator-declared Limits.OutputBytes bound,
	// emitted as inferenceConfig.maxTokens on the Converse surface. Zero
	// omits the inferenceConfig block entirely.
	maxOutputTokens int64
}

type bedrockTool struct {
	ToolSpec struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			JSON json.RawMessage `json:"json"`
		} `json:"inputSchema"`
	} `json:"toolSpec"`
}

func NewBedrockAdapter(
	config BedrockProfileConfig,
	resolver AWSRuntimeResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (Adapter, error) {
	transport, err := newBedrockTransport(
		config,
		ProfileSurfaceBedrockRuntimeConverse,
		resolver,
		probe,
		roundTripper,
	)
	if err != nil {
		return nil, err
	}
	config = transport.config
	factory := func(
		prompt []byte,
		model string,
		tools []providerToolDefinition,
		limits Limits,
	) (providerConversation, error) {
		return newBedrockConversation(
			config,
			model,
			tools,
			prompt,
			limits.OutputBytes,
		)
	}
	return newLoopAdapter(
		config.Key, config.ID, config.Version, ProfileBedrock,
		ProfileSurfaceBedrockRuntimeConverse,
		providerDialectBedrockConverse,
		config, factory, transport,
	)
}

func newBedrockTransport(
	config BedrockProfileConfig,
	surface ProfileSurface,
	resolver AWSRuntimeResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (*bedrockTransport, error) {
	if !providerKeyPattern.MatchString(config.Key) ||
		!driverIdentityPattern.MatchString(config.ID) ||
		!versionPattern.MatchString(config.Version) ||
		validateEndpoint(config.Endpoint) != nil ||
		config.ResponseBytes < 1 || config.ResponseBytes > MaxProviderResponseBytes ||
		len(config.CredentialRefs) == 0 || resolver == nil ||
		validateAWSChainSpec(config.Chain) != nil ||
		bedrockSigningService(surface) == "" {
		return nil, fail("INVALID_ADAPTER")
	}
	refs := make(map[string]struct{}, len(config.CredentialRefs))
	for _, ref := range config.CredentialRefs {
		if !providerKeyPattern.MatchString(ref) {
			return nil, fail("INVALID_CREDENTIAL_REFERENCE")
		}
		if _, duplicate := refs[ref]; duplicate {
			return nil, fail("INVALID_CREDENTIAL_REFERENCE")
		}
		refs[ref] = struct{}{}
	}
	sort.Strings(config.CredentialRefs)
	config.Chain.EnvironmentKeys = normalizedAWSEnvironmentKeys(config.Chain.EnvironmentKeys)
	sort.Slice(config.Chain.RuntimeFiles, func(left, right int) bool {
		return config.Chain.RuntimeFiles[left].Target <
			config.Chain.RuntimeFiles[right].Target
	})
	sort.Strings(config.Chain.RequiredRuntimeTargets)
	if roundTripper == nil {
		roundTripper = http.DefaultTransport.(*http.Transport).Clone()
	}
	transport := &bedrockTransport{
		config: config, surface: surface,
		resolve: resolver, liveProbe: probe, refs: refs,
		runAWS: execAWSCommand,
		client: &http.Client{
			Transport: roundTripper,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return fail("HTTP_REDIRECT_REFUSED")
			},
		},
	}
	return transport, nil
}

func newBedrockConversation(
	config BedrockProfileConfig,
	model string,
	definitions []providerToolDefinition,
	prompt []byte,
	maxOutputTokens ...int64,
) (*bedrockConversation, error) {
	if validateText(model, 500, false) != nil || validateEndpoint(config.Endpoint) != nil {
		return nil, fail("INVALID_ADAPTER")
	}
	outputLimit, err := optionalOutputLimit(maxOutputTokens)
	if err != nil {
		return nil, err
	}
	tools, err := bedrockTools(definitions)
	if err != nil {
		return nil, err
	}
	system, err := bedrockTerminalInstruction(definitions)
	if err != nil {
		return nil, err
	}
	initialMessage, err := json.Marshal(map[string]any{
		"role": "user",
		"content": []map[string]string{{
			"text": string(prompt),
		}},
	})
	if err != nil || len(initialMessage) > MaxProviderRequestBytes {
		return nil, fail("RESOURCE_LIMIT")
	}
	return &bedrockConversation{
		endpoint: strings.TrimSuffix(config.Endpoint, "/"),
		model:    model,
		system:   system,
		tools:    tools, messages: []json.RawMessage{initialMessage},
		allowCachePoint:   config.AllowCachePoint,
		allowGuardContent: config.AllowGuardContent,
		ledger:            newContinuationLedger(),
		maxOutputTokens:   outputLimit,
	}, nil
}

func bedrockTerminalInstruction(
	definitions []providerToolDefinition,
) (string, error) {
	var terminals []string
	for _, definition := range definitions {
		switch definition.Name {
		case "sworn_submit", "sworn_yield",
			"sworn_recovery_decide", "sworn_advisory_respond":
			terminals = append(terminals, definition.Name)
		}
	}
	sort.Strings(terminals)
	valid := len(terminals) == 1 &&
		(terminals[0] == "sworn_recovery_decide" ||
			terminals[0] == "sworn_advisory_respond")
	valid = valid || len(terminals) == 2 &&
		terminals[0] == "sworn_submit" &&
		terminals[1] == "sworn_yield"
	if !valid {
		return "", failContinuation("continuation.bedrock.invalid_terminal_definition")
	}
	return "Use only the supplied Sworn tools and terminate with exactly one call to " +
		strings.Join(terminals, " or ") + ".", nil
}

func bedrockTools(
	definitions []providerToolDefinition,
) ([]bedrockTool, error) {
	if len(definitions) == 0 || len(definitions) > MaxToolCalls {
		return nil, failContinuation("continuation.bedrock.tool_count_out_of_bounds")
	}
	tools := make([]bedrockTool, len(definitions))
	for index, definition := range definitions {
		if !providerKeyPattern.MatchString(definition.Name) ||
			len(definition.InputSchema) == 0 ||
			len(definition.InputSchema) > MaxToolArgumentBytes {
			return nil, failContinuation("continuation.bedrock.invalid_tool_definition")
		}
		tools[index].ToolSpec.Name = definition.Name
		tools[index].ToolSpec.Description = definition.Description
		tools[index].ToolSpec.InputSchema.JSON =
			append([]byte(nil), definition.InputSchema...)
	}
	return tools, nil
}

func (conversation *bedrockConversation) request() (providerRequest, error) {
	if conversation == nil || len(conversation.pending) != 0 {
		return providerRequest{}, failContinuation("continuation.bedrock.request_pending_tool_calls")
	}
	// Limits.OutputBytes is emitted as inferenceConfig.maxTokens on the
	// Converse surface. The block rides as a pointer so an unset limit
	// leaves the request byte-identical to today's shape.
	type inferenceConfig struct {
		MaxTokens int64 `json:"maxTokens,omitempty"`
	}
	var inference *inferenceConfig
	if conversation.maxOutputTokens > 0 {
		inference = &inferenceConfig{MaxTokens: conversation.maxOutputTokens}
	}
	body, err := json.Marshal(struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		Messages   []json.RawMessage `json:"messages"`
		ToolConfig struct {
			Tools []bedrockTool `json:"tools"`
		} `json:"toolConfig"`
		InferenceConfig *inferenceConfig `json:"inferenceConfig,omitempty"`
	}{
		System: []struct {
			Text string `json:"text"`
		}{{Text: conversation.system}},
		Messages: conversation.messages,
		ToolConfig: struct {
			Tools []bedrockTool `json:"tools"`
		}{Tools: conversation.tools},
		InferenceConfig: inference,
	})
	if err != nil || len(body) > MaxProviderRequestBytes {
		clearBytes(body)
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	endpoint := conversation.endpoint + "/model/" +
		url.PathEscape(conversation.model) + "/converse"
	return providerRequest{
		Method: "POST", URL: endpoint, ContentType: "application/json", Body: body,
	}, nil
}

func (conversation *bedrockConversation) accept(body []byte) (providerTurn, error) {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxProviderResponseBytes {
		return providerTurn{}, failContinuation("continuation.bedrock.accept_invalid_state_or_body_size")
	}
	value, err := decodeStrict(body, MaxProviderResponseBytes)
	if err != nil {
		return providerTurn{}, err
	}
	root, err := closedObject(
		value,
		[]string{"output", "stopReason"},
		[]string{
			"usage", "metrics", "additionalModelResponseFields", "trace",
			"performanceConfig", "serviceTier",
		},
	)
	if err != nil {
		return providerTurn{}, failContinuation("continuation.bedrock.accept_root_invalid")
	}
	stopReason, stopOK := root["stopReason"].(string)
	if !stopOK {
		return providerTurn{}, failContinuation("continuation.bedrock.accept_stop_reason_missing")
	}
	if stopReason != "tool_use" && stopReason != "end_turn" {
		if stopReason != "max_tokens" {
			return providerTurn{}, fail("MISSING_SUBMISSION")
		}
		// The provider hit its output-token ceiling: report the explicit
		// named failure carrying the provider's own finish reason instead of
		// an empty-looking success.
		turn := providerTurn{
			FinishReason: &stopReason,
			Truncated:    true,
		}
		if usageValue, present := root["usage"]; present {
			usage, usageErr := bedrockUsage(usageValue)
			if usageErr != nil {
				return providerTurn{}, usageErr
			}
			turn.Usage = usage
		}
		return turn, nil
	}
	output, err := closedObject(root["output"], []string{"message"}, nil)
	if err != nil {
		return providerTurn{}, failContinuation("continuation.bedrock.accept_output_invalid")
	}
	message, err := closedObject(output["message"], []string{"role", "content"}, nil)
	if err != nil || message["role"] != "assistant" {
		return providerTurn{}, failContinuation("continuation.bedrock.accept_message_invalid")
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) == 0 || len(content) > MaxToolCalls {
		return providerTurn{}, failContinuation("continuation.bedrock.accept_content_invalid")
	}
	var calls []providerToolCall
	var opaque []opaqueField
	for _, rawBlock := range content {
		block, ok := rawBlock.(map[string]any)
		if !ok || len(block) != 1 {
			return providerTurn{}, failContinuation("continuation.bedrock.accept_block_invalid")
		}
		for name, blockValue := range block {
			switch name {
			case "text":
				text, valid := blockValue.(string)
				if !valid || !validOpaqueText([]byte(text)) {
					return providerTurn{}, failContinuation("continuation.bedrock.accept_text_invalid")
				}
			case "toolUse":
				toolUse, toolErr := closedObject(
					blockValue,
					[]string{"toolUseId", "name", "input"},
					nil,
				)
				if toolErr != nil {
					return providerTurn{}, failContinuation("continuation.bedrock.accept_tool_use_invalid")
				}
				id, idOK := toolUse["toolUseId"].(string)
				toolName, nameOK := toolUse["name"].(string)
				arguments, marshalErr := canonicalJSON(toolUse["input"])
				if !idOK || !nameOK || marshalErr != nil ||
					conversation.ledger.correlate(id) != nil ||
					!providerKeyPattern.MatchString(toolName) ||
					len(arguments) > MaxToolArgumentBytes {
					return providerTurn{}, failContinuation("continuation.bedrock.accept_tool_use_fields_invalid")
				}
				calls = append(calls, providerToolCall{
					ID: id, Name: toolName, Arguments: arguments,
				})
			case "reasoningContent":
				reasoning, reasoningErr := closedObject(
					blockValue,
					nil,
					[]string{"reasoningText", "redactedContent"},
				)
				if reasoningErr != nil || len(reasoning) != 1 {
					return providerTurn{}, failContinuation("continuation.bedrock.accept_reasoning_invalid")
				}
				if textValue, present := reasoning["reasoningText"]; present {
					reasoningText, textErr := closedObject(
						textValue,
						[]string{"text", "signature"},
						nil,
					)
					text, textOK := reasoningText["text"].(string)
					signature, signatureOK := reasoningText["signature"].(string)
					if textErr != nil || !textOK || !signatureOK ||
						!validOpaqueText([]byte(text)) || signature == "" {
						return providerTurn{}, failContinuation("continuation.bedrock.accept_reasoning_text_invalid")
					}
					opaque = append(opaque, opaqueField{
						kind: opaqueBase64, body: []byte(signature),
					})
				} else {
					redacted, valid := reasoning["redactedContent"].(string)
					if !valid || redacted == "" {
						return providerTurn{}, failContinuation("continuation.bedrock.accept_redacted_content_invalid")
					}
					opaque = append(opaque, opaqueField{
						kind: opaqueBase64, body: []byte(redacted),
					})
				}
			case "cachePoint":
				cache, cacheErr := closedObject(blockValue, []string{"type"}, nil)
				if !conversation.allowCachePoint || cacheErr != nil ||
					cache["type"] != "default" {
					return providerTurn{}, failContinuation("continuation.bedrock.accept_cache_point_invalid")
				}
			case "guardContent":
				if !conversation.allowGuardContent {
					return providerTurn{}, failContinuation("continuation.bedrock.accept_guard_content_not_allowed")
				}
				guard, valid := blockValue.(map[string]any)
				if !valid || len(guard) != 1 {
					return providerTurn{}, failContinuation("continuation.bedrock.accept_guard_content_invalid")
				}
				for guardKind, guardValue := range guard {
					if guardKind != "text" && guardKind != "image" {
						return providerTurn{}, failContinuation("continuation.bedrock.accept_guard_content_kind_invalid")
					}
					canonical, canonicalErr := canonicalJSON(guardValue)
					if canonicalErr != nil || len(canonical) > MaxOpaqueFieldBytes {
						return providerTurn{}, failContinuation("continuation.bedrock.accept_guard_content_payload_invalid")
					}
					clearBytes(canonical)
				}
			default:
				return providerTurn{}, failContinuation("continuation.bedrock.accept_unknown_block_type")
			}
		}
	}
	if (stopReason == "tool_use") != (len(calls) > 0) {
		return providerTurn{}, failContinuation("continuation.bedrock.accept_stop_reason_mismatch")
	}
	if len(opaque) > 0 {
		if _, err := conversation.ledger.retain(opaque...); err != nil {
			return providerTurn{}, err
		}
	}
	var rawResponse struct {
		Output struct {
			Message json.RawMessage `json:"message"`
		} `json:"output"`
	}
	if json.Unmarshal(body, &rawResponse) != nil || len(rawResponse.Output.Message) == 0 {
		return providerTurn{}, failContinuation("continuation.bedrock.accept_raw_unmarshal_failed")
	}
	if len(rawResponse.Output.Message) > MaxOpaqueStepBytes {
		clearBytes(rawResponse.Output.Message)
		return providerTurn{}, failContinuation("continuation.bedrock.accept_raw_message_overflow")
	}
	conversation.messages = append(
		conversation.messages,
		append(json.RawMessage(nil), rawResponse.Output.Message...),
	)
	conversation.pending = append([]providerToolCall(nil), calls...)
	turn := providerTurn{Calls: calls, Prose: len(calls) == 0}
	if usageValue, present := root["usage"]; present {
		usage, usageErr := bedrockUsage(usageValue)
		if usageErr != nil {
			return providerTurn{}, usageErr
		}
		turn.Usage = usage
	}
	return turn, nil
}

// bedrockUsage parses the Converse usage object, surfacing the cache pair
// (cacheReadInputTokens -> read, cacheWriteInputTokens -> write) instead of
// discarding it. Each side stays nil when the provider omits it.
func bedrockUsage(value any) (*Usage, error) {
	usage, err := closedObject(
		value,
		[]string{"inputTokens", "outputTokens", "totalTokens"},
		[]string{
			"cacheReadInputTokens", "cacheWriteInputTokens",
			"serverToolUsage",
		},
	)
	if err != nil {
		return nil, fail("INVALID_USAGE")
	}
	input, inputOK := safeJSONInt(usage["inputTokens"])
	outputTokens, outputOK := safeJSONInt(usage["outputTokens"])
	if !inputOK || !outputOK {
		return nil, fail("INVALID_USAGE")
	}
	result := &Usage{InputTokens: input, OutputTokens: outputTokens}
	if _, present := usage["cacheReadInputTokens"]; present {
		read, readOK := safeJSONInt(usage["cacheReadInputTokens"])
		if !readOK {
			return nil, fail("INVALID_USAGE")
		}
		result.CacheReadTokens = &read
	}
	if _, present := usage["cacheWriteInputTokens"]; present {
		write, writeOK := safeJSONInt(usage["cacheWriteInputTokens"])
		if !writeOK {
			return nil, fail("INVALID_USAGE")
		}
		result.CacheWriteTokens = &write
	}
	return result, nil
}

func (conversation *bedrockConversation) appendInstruction(body []byte) error {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxOpaqueFieldBytes ||
		!validOpaqueText(body) {
		return failContinuation("continuation.bedrock.append_instruction_invalid")
	}
	message, err := json.Marshal(map[string]any{
		"role": "user",
		"content": []map[string]string{
			{"text": string(body)},
		},
	})
	if err != nil || len(message) > MaxOpaqueStepBytes {
		clearBytes(message)
		return failContinuation("continuation.bedrock.append_instruction_marshal_failed")
	}
	conversation.messages = append(conversation.messages, message)
	return nil
}

func (conversation *bedrockConversation) appendResults(results []providerToolResult) error {
	if conversation == nil || len(results) != len(conversation.pending) {
		return failContinuation("continuation.bedrock.append_results_pending_mismatch")
	}
	content := make([]map[string]any, len(results))
	for index, result := range results {
		expected := conversation.pending[index]
		if result.ID != expected.ID || result.Name != expected.Name ||
			len(result.Content) > MaxToolResultBytes || !validOpaqueText(result.Content) {
			return failContinuation("continuation.bedrock.append_results_mismatch_or_invalid")
		}
		status := "success"
		if result.Failed {
			status = "error"
		}
		content[index] = map[string]any{"toolResult": map[string]any{
			"toolUseId": result.ID,
			"content":   []map[string]string{{"text": string(result.Content)}},
			"status":    status,
		}}
	}
	message, err := json.Marshal(map[string]any{"role": "user", "content": content})
	if err != nil || len(message) > MaxOpaqueStepBytes {
		return failContinuation("continuation.bedrock.append_results_marshal_failed")
	}
	conversation.messages = append(conversation.messages, message)
	conversation.pending = nil
	return nil
}

func (conversation *bedrockConversation) resume(
	prompt []byte,
	definitions []providerToolDefinition,
) error {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(prompt) == 0 || len(prompt) > MaxProviderRequestBytes ||
		len(conversation.messages) < 3 {
		return failContinuation("continuation.bedrock.resume_invalid_state")
	}
	last := conversation.messages[len(conversation.messages)-1]
	value, err := decodeStrict(last, MaxOpaqueStepBytes)
	if err != nil {
		return failContinuation("continuation.bedrock.resume_decode_last_failed")
	}
	message, err := closedObject(
		value,
		[]string{"role", "content"},
		nil,
	)
	content, ok := message["content"].([]any)
	if err != nil || message["role"] != "user" ||
		!ok || len(content) == 0 {
		return failContinuation("continuation.bedrock.resume_message_invalid")
	}
	toolUseIDs, err := bedrockToolUseIDs(
		conversation.messages[len(conversation.messages)-2],
	)
	if err != nil || len(toolUseIDs) != len(content) {
		return failContinuation("continuation.bedrock.resume_tool_use_count_mismatch")
	}
	for index, rawBlock := range content {
		block, blockOK := rawBlock.(map[string]any)
		if !blockOK || len(block) != 1 {
			return failContinuation("continuation.bedrock.resume_block_invalid")
		}
		toolResult, present := block["toolResult"]
		result, resultErr := closedObject(
			toolResult,
			[]string{"toolUseId", "content", "status"},
			nil,
		)
		id, idOK := result["toolUseId"].(string)
		status, statusOK := result["status"].(string)
		resultContent, contentOK := result["content"].([]any)
		if !present || resultErr != nil || !idOK ||
			validateText(id, MaxCorrelationIDBytes, false) != nil ||
			id != toolUseIDs[index] ||
			!statusOK || (status != "success" && status != "error") ||
			!contentOK || len(resultContent) != 1 {
			return failContinuation("continuation.bedrock.resume_tool_result_invalid")
		}
		textBlock, textErr := closedObject(
			resultContent[0],
			[]string{"text"},
			nil,
		)
		text, textOK := textBlock["text"].(string)
		if textErr != nil || !textOK ||
			!validOpaqueText([]byte(text)) {
			return failContinuation("continuation.bedrock.resume_text_invalid")
		}
	}
	content = append(content, map[string]string{"text": string(prompt)})
	resumed, err := json.Marshal(map[string]any{
		"role": "user", "content": content,
	})
	if err != nil || len(resumed) > MaxOpaqueStepBytes {
		clearBytes(resumed)
		return failContinuation("continuation.bedrock.resume_marshal_failed")
	}
	tools, err := bedrockTools(definitions)
	if err != nil {
		clearBytes(resumed)
		return err
	}
	clearBytes(last)
	clearBedrockTools(conversation.tools)
	conversation.messages[len(conversation.messages)-1] = resumed
	conversation.tools = tools
	return nil
}

func bedrockToolUseIDs(messageBody json.RawMessage) ([]string, error) {
	value, err := decodeStrict(messageBody, MaxOpaqueStepBytes)
	if err != nil {
		return nil, err
	}
	message, err := closedObject(
		value,
		[]string{"role", "content"},
		nil,
	)
	content, ok := message["content"].([]any)
	if err != nil || message["role"] != "assistant" || !ok {
		return nil, failContinuation("continuation.bedrock.tool_use_ids_content_invalid")
	}
	var ids []string
	for _, rawBlock := range content {
		block, blockOK := rawBlock.(map[string]any)
		if !blockOK || len(block) != 1 {
			return nil, failContinuation("continuation.bedrock.tool_use_ids_block_invalid")
		}
		toolUseValue, present := block["toolUse"]
		if !present {
			continue
		}
		toolUse, toolUseErr := closedObject(
			toolUseValue,
			[]string{"toolUseId", "name", "input"},
			nil,
		)
		id, idOK := toolUse["toolUseId"].(string)
		if toolUseErr != nil || !idOK ||
			validateText(id, MaxCorrelationIDBytes, false) != nil {
			return nil, failContinuation("continuation.bedrock.tool_use_ids_field_invalid")
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, failContinuation("continuation.bedrock.tool_use_ids_none_found")
	}
	return ids, nil
}

func (conversation *bedrockConversation) close() {
	if conversation == nil {
		return
	}
	conversation.ledger.Close()
	conversation.system = ""
	for _, message := range conversation.messages {
		clearBytes(message)
	}
	clearBedrockTools(conversation.tools)
	conversation.endpoint = ""
	conversation.model = ""
	conversation.messages = nil
	conversation.pending = nil
	conversation.tools = nil
}

func clearBedrockTools(tools []bedrockTool) {
	for index := range tools {
		clearBytes(tools[index].ToolSpec.InputSchema.JSON)
	}
}

// declaredReasoningEffort is honest absence for the Converse dialect, which
// has no reasoning-effort request vocabulary.
func (*bedrockConversation) declaredReasoningEffort() string { return "" }

func (transport *bedrockTransport) roundTrip(
	ctx context.Context,
	ref *string,
	request providerRequest,
) ([]byte, error) {
	if transport == nil || ref == nil {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	if _, admitted := transport.refs[*ref]; !admitted {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	if request.Method != http.MethodPost ||
		request.ContentType != "application/json" ||
		validateEndpoint(request.URL) != nil ||
		!sameEndpointAuthority(transport.config.Endpoint, request.URL) ||
		len(request.Body) == 0 || len(request.Body) > MaxProviderRequestBytes {
		return nil, fail("INVALID_PROVIDER_REQUEST")
	}
	environment, err := transport.resolve(ctx, *ref)
	if err != nil {
		clearEnvironment(environment)
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	snapshot, credentials, err := resolveAWSChain(
		ctx,
		transport.config.Chain,
		environment,
		transport.runAWS,
	)
	if err != nil {
		return nil, err
	}
	defer credentials.Close()
	if snapshot.Region != transport.config.Chain.Region {
		return nil, fail("AWS_NOT_CERTIFIED")
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		request.URL,
		bytes.NewReader(request.Body),
	)
	if err != nil {
		return nil, fail("INVALID_PROVIDER_REQUEST")
	}
	httpRequest.Header.Set("Content-Type", request.ContentType)
	service := bedrockSigningService(transport.surface)
	if service == "" {
		return nil, fail("AWS_SIGNING_FAILED")
	}
	if err := signAWSRequest(
		httpRequest,
		request.Body,
		credentials,
		snapshot.Region,
		service,
		time.Now().UTC(),
	); err != nil {
		return nil, err
	}
	response, err := transport.client.Do(httpRequest)
	if err != nil {
		if isContextError(ctx.Err()) {
			return nil, ctx.Err()
		}
		return nil, fail("PROVIDER_TRANSPORT_FAILED")
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(
		response.Body,
		int64(transport.config.ResponseBytes)+1,
	))
	if readErr != nil || len(body) > transport.config.ResponseBytes {
		clearBytes(body)
		return nil, fail("OUTPUT_OVERFLOW")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		// Bedrock's error envelope is not the {"error":{"message":...}} shape
		// the detail extractor anchors, so no message is read here: the
		// stable code stays the only fact, and a bedrock 429 keeps
		// HardLimit false so it stays on today's default-paced path.
		clearBytes(body)
		return nil, providerHTTPStatusError(response.StatusCode, "")
	}
	return body, nil
}

func bedrockSigningService(surface ProfileSurface) string {
	switch surface {
	case ProfileSurfaceBedrockRuntimeConverse:
		return "bedrock"
	case ProfileSurfaceBedrockMantleChat:
		return "bedrock-mantle"
	default:
		return ""
	}
}

func (transport *bedrockTransport) check(
	ctx context.Context,
	kind profileCheckKind,
	ref *string,
	model string,
) (ReadinessState, string) {
	if transport == nil || ref == nil {
		return ReadinessNotCertified, "credential_reference_missing"
	}
	if _, admitted := transport.refs[*ref]; !admitted {
		return ReadinessNotCertified, "credential_reference_unknown"
	}
	directEnvironment := directAWSEnvironmentSpec(transport.config.Chain)
	if !directEnvironment {
		closure, err := openAWSClosure(transport.config.Chain)
		if err != nil {
			return ReadinessNotCertified, "aws_cli_closure_changed"
		}
		closeNativeFiles(closure)
	}
	switch kind {
	case checkInspect:
		return ReadinessPass, "aws_configuration_exact"
	case checkDoctor:
		if directEnvironment {
			return ReadinessPass, "aws_environment_ready"
		}
		if transport.runAWS == nil {
			return ReadinessNotCertified, "aws_cli_runner_missing"
		}
		body, runErr := transport.runAWS(
			ctx,
			transport.config.Chain,
			nil,
			"--version",
		)
		matches := runErr == nil && awsVersionMatches(body)
		clearBytes(body)
		if !matches {
			return ReadinessNotCertified, "aws_cli_version_changed"
		}
		return ReadinessPass, "aws_cli_ready"
	case checkCertify:
		if transport.liveProbe == nil {
			return ReadinessNotCertified, "live_probe_not_configured"
		}
		if err := transport.liveProbe(ctx, *ref, model); err != nil {
			return ReadinessFail, certificationFailureCode(err)
		}
		return ReadinessPass, "live_probe_passed"
	default:
		return ReadinessFail, "check_kind_invalid"
	}
}

func signAWSRequest(
	request *http.Request,
	body []byte,
	credentials *awsCredentials,
	region, service string,
	now time.Time,
) error {
	if request == nil || credentials == nil ||
		len(credentials.accessKeyID) == 0 || len(credentials.secretAccessKey) == 0 ||
		validateText(region, 128, false) != nil ||
		validateText(service, 64, false) != nil ||
		request.URL.RawQuery != "" {
		return fail("AWS_SIGNING_FAILED")
	}
	payloadSum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(payloadSum[:])
	dateTime := now.UTC().Format("20060102T150405Z")
	date := now.UTC().Format("20060102")
	request.Header.Set("X-Amz-Date", dateTime)
	request.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if len(credentials.sessionToken) != 0 {
		request.Header.Set("X-Amz-Security-Token", string(credentials.sessionToken))
	}
	headers := map[string]string{
		"content-type":         strings.TrimSpace(request.Header.Get("Content-Type")),
		"host":                 request.URL.Host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           dateTime,
	}
	if len(credentials.sessionToken) != 0 {
		headers["x-amz-security-token"] = string(credentials.sessionToken)
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.Join(strings.Fields(headers[name]), " "))
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(names, ";")
	canonicalURI := request.URL.EscapedPath()
	canonicalURI = awsCanonicalURI(canonicalURI)
	canonicalRequest := strings.Join([]string{
		request.Method,
		canonicalURI,
		"",
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")
	canonicalSum := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + region + "/" + service + "/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + dateTime + "\n" + scope + "\n" +
		hex.EncodeToString(canonicalSum[:])
	secretPrefix := append([]byte("AWS4"), credentials.secretAccessKey...)
	dateKey := hmacSHA256(secretPrefix, []byte(date))
	clearBytes(secretPrefix)
	regionKey := hmacSHA256(dateKey, []byte(region))
	clearBytes(dateKey)
	serviceKey := hmacSHA256(regionKey, []byte(service))
	clearBytes(regionKey)
	signingKey := hmacSHA256(serviceKey, []byte("aws4_request"))
	clearBytes(serviceKey)
	signature := hmacSHA256(signingKey, []byte(stringToSign))
	clearBytes(signingKey)
	request.Header.Set(
		"Authorization",
		"AWS4-HMAC-SHA256 Credential="+string(credentials.accessKeyID)+"/"+scope+
			", SignedHeaders="+signedHeaders+
			", Signature="+hex.EncodeToString(signature),
	)
	clearBytes(signature)
	return nil
}

func awsCanonicalURI(value string) string {
	if value == "" {
		return "/"
	}
	const hexadecimal = "0123456789ABCDEF"
	var encoded strings.Builder
	encoded.Grow(len(value))
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '/' ||
			character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '.' ||
			character == '_' || character == '~' {
			encoded.WriteByte(character)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hexadecimal[character>>4])
		encoded.WriteByte(hexadecimal[character&0x0f])
	}
	return encoded.String()
}

func hmacSHA256(key, body []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}
