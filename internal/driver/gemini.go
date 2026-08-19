package driver

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

type geminiConversation struct {
	baseURL  string
	model    string
	tools    []geminiDeclaration
	contents []geminiContent
	pending  []geminiPending
	ledger   *continuationLedger
	step     int
	// maxOutputTokens and thinkingLevel are the operator-controlled request
	// knobs. maxOutputTokens comes from the invocation limits (omitted when
	// zero, e.g. automation paths); thinkingLevel is adapter configuration
	// (omitted when unset, never a hardcoded default).
	maxOutputTokens int64
	thinkingLevel   string
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             *string                 `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
}

type geminiFunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// geminiFunctionResponse carries the fixed content-independent single-key
// envelope {"result": "<the exact string>"}: a tool result whose text is
// itself valid JSON still rides as text, never parsed and embedded
// structured. The provider's functionResponse part carries only name and the
// envelope (no id), matching the recorded n3/n6 wire shape.
type geminiFunctionResponse struct {
	Name     string `json:"name"`
	Response struct {
		Result string `json:"result"`
	} `json:"response"`
}

type geminiDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// geminiThinkingConfig and geminiGenerationConfig render the recorded
// generationConfig shape: maxOutputTokens then thinkingConfig, each omitted
// when the operator left the knob unset.
type geminiThinkingConfig struct {
	ThinkingLevel string `json:"thinkingLevel"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int64                 `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiPending struct {
	call       providerToolCall
	providerID string
}

func NewGeminiAdapter(
	config HTTPProfileConfig,
	resolver HeaderCredentialResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (Adapter, error) {
	transport, err := newHTTPTransport(
		config,
		AuthModeBearer,
		resolver,
		probe,
		roundTripper,
	)
	if err != nil {
		return nil, err
	}
	if !validThinkingLevel(config.ThinkingLevel) ||
		config.InputTokensPerMinute < 0 {
		return nil, fail("INVALID_ADAPTER")
	}
	factory := func(
		prompt []byte,
		model string,
		tools []providerToolDefinition,
		limits Limits,
	) (providerConversation, error) {
		return newGeminiConversation(
			config.Endpoint, model, tools, prompt,
			limits.OutputBytes, config.ThinkingLevel,
		)
	}
	configuration := struct {
		HTTPProfileConfig
		Family ProfileFamily
	}{transport.config, ProfileGemini}
	adapter, err := newLoopAdapter(
		config.Key, config.ID, config.Version, ProfileGemini,
		"", providerDialectGemini, configuration, factory, transport,
	)
	if err != nil {
		return nil, err
	}
	adapter.pacingCap = config.InputTokensPerMinute
	return adapter, nil
}

func newGeminiConversation(
	baseURL, model string,
	definitions []providerToolDefinition,
	prompt []byte,
	maxOutputTokens int64,
	thinkingLevel string,
) (*geminiConversation, error) {
	if validateEndpoint(baseURL) != nil ||
		validateText(model, 500, false) != nil ||
		maxOutputTokens < 0 || maxOutputTokens > MaxProviderOutputBytes ||
		!validThinkingLevel(thinkingLevel) {
		return nil, fail("INVALID_ADAPTER")
	}
	declarations, err := geminiDeclarations(definitions)
	if err != nil {
		return nil, err
	}
	promptText := string(prompt)
	return &geminiConversation{
		baseURL: baseURL, model: model, tools: declarations,
		contents: []geminiContent{{
			Role: "user", Parts: []geminiPart{{Text: &promptText}},
		}},
		ledger:          newContinuationLedger(),
		maxOutputTokens: maxOutputTokens,
		thinkingLevel:   thinkingLevel,
	}, nil
}

// validThinkingLevel admits the closed operator vocabulary. An empty value
// means the thinkingConfig knob is unset and nothing is emitted; every other
// value fails closed so a typo never reaches the wire as a hardcoded default.
func validThinkingLevel(value string) bool {
	switch value {
	case "", "LOW", "MEDIUM", "HIGH":
		return true
	default:
		return false
	}
}

func geminiDeclarations(
	definitions []providerToolDefinition,
) ([]geminiDeclaration, error) {
	if len(definitions) == 0 || len(definitions) > MaxToolCalls {
		return nil, failContinuation("continuation.gemini.tool_count_out_of_bounds")
	}
	declarations := make([]geminiDeclaration, len(definitions))
	for index, definition := range definitions {
		if !providerKeyPattern.MatchString(definition.Name) ||
			len(definition.InputSchema) == 0 ||
			len(definition.InputSchema) > MaxToolArgumentBytes {
			return nil, failContinuation("continuation.gemini.invalid_tool_definition")
		}
		parameters, err := geminiParameterSchema(definition.InputSchema)
		if err != nil {
			return nil, err
		}
		declarations[index] = geminiDeclaration{
			Name: definition.Name, Description: definition.Description,
			Parameters: parameters,
		}
	}
	return declarations, nil
}

// geminiParameterSchema renders a tool input schema in the generateContent
// Schema subset: the API rejects whole requests over JSON Schema keywords
// absent from its Schema proto, and every sworn tool pins
// additionalProperties. The keyword is advisory to the model here - the
// closed shapes stay enforced host-side at submission and argument
// validation - so it is dropped at schema nodes only, never inside
// properties, where a tool argument may legitimately carry that name.
func geminiParameterSchema(raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, failContinuation("continuation.gemini.parameter_schema_unmarshal_failed")
	}
	stripUnsupportedSchemaKeywords(schema)
	rendered, err := json.Marshal(schema)
	if err != nil || len(rendered) > MaxToolArgumentBytes {
		return nil, failContinuation("continuation.gemini.parameter_schema_marshal_failed")
	}
	return rendered, nil
}

func stripUnsupportedSchemaKeywords(schema map[string]any) {
	delete(schema, "additionalProperties")
	if properties, ok := schema["properties"].(map[string]any); ok {
		for _, value := range properties {
			if child, ok := value.(map[string]any); ok {
				stripUnsupportedSchemaKeywords(child)
			}
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		stripUnsupportedSchemaKeywords(items)
	}
}

func (conversation *geminiConversation) request() (providerRequest, error) {
	if conversation == nil || len(conversation.pending) != 0 {
		return providerRequest{}, failContinuation("continuation.gemini.request_pending_tool_calls")
	}
	generation := geminiGenerationConfig{
		MaxOutputTokens: conversation.maxOutputTokens,
	}
	if conversation.thinkingLevel != "" {
		generation.ThinkingConfig = &geminiThinkingConfig{
			ThinkingLevel: conversation.thinkingLevel,
		}
	}
	body, err := json.Marshal(struct {
		Contents []geminiContent `json:"contents"`
		Tools    []struct {
			FunctionDeclarations []geminiDeclaration `json:"functionDeclarations"`
		} `json:"tools"`
		GenerationConfig geminiGenerationConfig `json:"generationConfig"`
	}{
		Contents: conversation.contents,
		Tools: []struct {
			FunctionDeclarations []geminiDeclaration `json:"functionDeclarations"`
		}{{FunctionDeclarations: conversation.tools}},
		GenerationConfig: generation,
	})
	if err != nil || len(body) > MaxProviderRequestBytes {
		clearBytes(body)
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	endpoint := strings.TrimSuffix(conversation.baseURL, "/") +
		"/v1beta/models/" + url.PathEscape(conversation.model) + ":generateContent"
	return providerRequest{
		Method: "POST", URL: endpoint, ContentType: "application/json", Body: body,
	}, nil
}

func (conversation *geminiConversation) accept(body []byte) (providerTurn, error) {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxProviderResponseBytes {
		return providerTurn{}, failContinuation("continuation.gemini.accept_invalid_state_or_body_size")
	}
	value, err := decodeStrict(body, MaxProviderResponseBytes)
	if err != nil {
		return providerTurn{}, err
	}
	root, err := closedObject(
		value,
		[]string{"candidates"},
		[]string{"usageMetadata", "modelVersion", "responseId", "promptFeedback"},
	)
	if err != nil {
		return providerTurn{}, failContinuation("continuation.gemini.accept_root_invalid")
	}
	candidates, ok := root["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		return providerTurn{}, failContinuation("continuation.gemini.accept_candidates_invalid")
	}
	candidate, err := closedObject(
		candidates[0],
		[]string{"content", "finishReason"},
		[]string{
			"index", "safetyRatings", "citationMetadata", "tokenCount",
			"finishMessage",
		},
	)
	if err != nil {
		return providerTurn{}, failContinuation("continuation.gemini.accept_candidate_invalid")
	}
	finishReason, finishOK := candidate["finishReason"].(string)
	if !finishOK {
		return providerTurn{}, failContinuation("continuation.gemini.accept_finish_reason_missing")
	}
	if finishReason != "STOP" {
		if finishReason != "MAX_TOKENS" {
			return providerTurn{}, failContinuation("continuation.gemini.accept_finish_reason_unsupported")
		}
		// The provider hit its output-token ceiling: report the explicit
		// named failure carrying the provider's own finish reason instead of
		// an empty-looking success.
		turn := providerTurn{
			FinishReason: &finishReason,
			Truncated:    true,
		}
		if usage, present := root["usageMetadata"]; present {
			parsed, usageErr := geminiUsage(usage)
			if usageErr != nil {
				return providerTurn{}, usageErr
			}
			turn.Usage = parsed
		}
		return turn, nil
	}
	content, err := closedObject(candidate["content"], []string{"role", "parts"}, nil)
	if err != nil || content["role"] != "model" {
		return providerTurn{}, failContinuation("continuation.gemini.accept_content_invalid")
	}
	rawParts, ok := content["parts"].([]any)
	if !ok || len(rawParts) == 0 || len(rawParts) > MaxToolCalls {
		return providerTurn{}, failContinuation("continuation.gemini.accept_parts_invalid")
	}
	parts := make([]geminiPart, 0, len(rawParts))
	var calls []providerToolCall
	var pending []geminiPending
	var opaque []opaqueField
	for index, rawPart := range rawParts {
		part, partErr := closedObject(
			rawPart,
			nil,
			[]string{"text", "functionCall", "thoughtSignature"},
		)
		if partErr != nil {
			return providerTurn{}, failContinuation("continuation.gemini.accept_part_object_invalid")
		}
		_, hasText := part["text"]
		functionValue, hasCall := part["functionCall"]
		if hasText == hasCall {
			return providerTurn{}, failContinuation("continuation.gemini.accept_part_ambiguous")
		}
		decoded := geminiPart{}
		if hasText {
			text, ok := part["text"].(string)
			if !ok || !validOpaqueText([]byte(text)) {
				return providerTurn{}, failContinuation("continuation.gemini.accept_part_text_invalid")
			}
			decoded.Text = &text
		} else {
			function, functionErr := closedObject(
				functionValue,
				[]string{"name", "args"},
				[]string{"id"},
			)
			if functionErr != nil {
				return providerTurn{}, failContinuation("continuation.gemini.accept_function_call_invalid")
			}
			name, nameOK := function["name"].(string)
			if !nameOK || !providerKeyPattern.MatchString(name) {
				return providerTurn{}, failContinuation("continuation.gemini.accept_function_name_invalid")
			}
			arguments, marshalErr := canonicalJSON(function["args"])
			if marshalErr != nil || len(arguments) > MaxToolArgumentBytes {
				return providerTurn{}, failContinuation("continuation.gemini.accept_function_args_invalid")
			}
			providerID, _ := function["id"].(string)
			internalID := providerID
			if internalID == "" {
				internalID = "gemini-" + itoa(conversation.step+1) + "-" + itoa(index+1)
			}
			if conversation.ledger.correlate(internalID) != nil {
				return providerTurn{}, failContinuation("continuation.gemini.accept_function_correlate_failed")
			}
			call := providerToolCall{
				ID: internalID, Name: name,
				Arguments: append([]byte(nil), arguments...),
			}
			decoded.FunctionCall = &geminiFunctionCall{
				ID: providerID, Name: name,
				Args: append([]byte(nil), arguments...),
			}
			calls = append(calls, call)
			pending = append(pending, geminiPending{call: call, providerID: providerID})
		}
		if signatureValue, present := part["thoughtSignature"]; present {
			signature, ok := signatureValue.(string)
			if !ok || signature == "" {
				return providerTurn{}, failContinuation("continuation.gemini.accept_thought_signature_invalid")
			}
			opaque = append(opaque, opaqueField{kind: opaqueBase64, body: []byte(signature)})
			decoded.ThoughtSignature = signature
		}
		if hasCall && len(calls) == 1 &&
			geminiThoughtSignatureRequired(conversation.model) &&
			decoded.ThoughtSignature == "" {
			return providerTurn{}, failContinuation("continuation.gemini.accept_thought_signature_missing")
		}
		parts = append(parts, decoded)
	}
	if len(opaque) > 0 {
		retained, retainErr := conversation.ledger.retain(opaque...)
		if retainErr != nil {
			return providerTurn{}, retainErr
		}
		retainedIndex := 0
		for index := range parts {
			if parts[index].ThoughtSignature != "" {
				parts[index].ThoughtSignature = string(retained[retainedIndex])
				retainedIndex++
			}
		}
	}
	conversation.contents = append(conversation.contents, geminiContent{Role: "model", Parts: parts})
	conversation.pending = pending
	conversation.step++
	turn := providerTurn{Calls: calls, Prose: len(calls) == 0}
	if usage, present := root["usageMetadata"]; present {
		parsed, usageErr := geminiUsage(usage)
		if usageErr != nil {
			return providerTurn{}, usageErr
		}
		turn.Usage = parsed
	}
	return turn, nil
}

// geminiUsage parses usageMetadata, surfacing cached-content tokens as cache
// reads instead of discarding them. Gemini reports only the read side (there
// is no write vocabulary), so CacheWriteTokens stays nil. The per-modality
// cacheTokensDetails breakdown is summed only when the total
// cachedContentTokenCount is absent.
func geminiUsage(value any) (*Usage, error) {
	metadata, err := closedObject(
		value,
		nil,
		[]string{
			"promptTokenCount", "candidatesTokenCount", "totalTokenCount",
			"cachedContentTokenCount", "thoughtsTokenCount", "toolUsePromptTokenCount",
			"promptTokensDetails", "candidatesTokensDetails", "cacheTokensDetails",
			"toolUsePromptTokensDetails", "serviceTier",
		},
	)
	if err != nil {
		return nil, fail("INVALID_USAGE")
	}
	input, inputOK := safeJSONInt(metadata["promptTokenCount"])
	output, outputOK := safeJSONInt(metadata["candidatesTokenCount"])
	if !inputOK || !outputOK {
		return nil, fail("INVALID_USAGE")
	}
	result := &Usage{InputTokens: input, OutputTokens: output}
	// thoughtsTokenCount is the reasoning side of the native usage
	// vocabulary, admitted on the same path cache reads ride: it lands on
	// the driver result and is summed across turns exactly like cache reads.
	if value, present := metadata["thoughtsTokenCount"]; present {
		reasoning, reasoningOK := safeJSONInt(value)
		if !reasoningOK {
			return nil, fail("INVALID_USAGE")
		}
		result.ReasoningTokens = &reasoning
	}
	if _, present := metadata["cachedContentTokenCount"]; present {
		read, readOK := safeJSONInt(metadata["cachedContentTokenCount"])
		if !readOK {
			return nil, fail("INVALID_USAGE")
		}
		result.CacheReadTokens = &read
		return result, nil
	}
	detailsValue, present := metadata["cacheTokensDetails"]
	if !present {
		return result, nil
	}
	details, detailsOK := detailsValue.([]any)
	if !detailsOK {
		return nil, fail("INVALID_USAGE")
	}
	var total int64
	for _, rawDetail := range details {
		detail, detailErr := closedObject(
			rawDetail,
			nil,
			[]string{"modality", "tokenCount"},
		)
		if detailErr != nil {
			return nil, fail("INVALID_USAGE")
		}
		count, countOK := safeJSONInt(detail["tokenCount"])
		if !countOK || total > MaxSafeInteger-count {
			return nil, fail("INVALID_USAGE")
		}
		total += count
	}
	if total != 0 || len(details) != 0 {
		result.CacheReadTokens = &total
	}
	return result, nil
}

func (conversation *geminiConversation) appendInstruction(body []byte) error {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxOpaqueFieldBytes ||
		!validOpaqueText(body) {
		return failContinuation("continuation.gemini.append_instruction_invalid")
	}
	text := string(body)
	conversation.contents = append(conversation.contents, geminiContent{
		Role: "user",
		Parts: []geminiPart{
			{Text: &text},
		},
	})
	return nil
}

func (conversation *geminiConversation) appendResults(results []providerToolResult) error {
	if conversation == nil || len(results) != len(conversation.pending) {
		return failContinuation("continuation.gemini.append_results_pending_mismatch")
	}
	parts := make([]geminiPart, len(results))
	for index, result := range results {
		expected := conversation.pending[index]
		if result.ID != expected.call.ID || result.Name != expected.call.Name ||
			len(result.Content) > MaxToolResultBytes || !validOpaqueText(result.Content) {
			return failContinuation("continuation.gemini.append_results_mismatch_or_invalid")
		}
		response := &geminiFunctionResponse{Name: expected.call.Name}
		response.Response.Result = string(result.Content)
		parts[index].FunctionResponse = response
	}
	conversation.contents = append(conversation.contents, geminiContent{Role: "user", Parts: parts})
	conversation.pending = nil
	return nil
}

func (conversation *geminiConversation) resume(
	prompt []byte,
	definitions []providerToolDefinition,
) error {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(prompt) == 0 || len(prompt) > MaxProviderRequestBytes ||
		len(conversation.contents) < 3 {
		return failContinuation("continuation.gemini.resume_invalid_state")
	}
	last := &conversation.contents[len(conversation.contents)-1]
	assistant := conversation.contents[len(conversation.contents)-2]
	if last.Role != "user" || len(last.Parts) == 0 ||
		assistant.Role != "model" {
		return failContinuation("continuation.gemini.resume_role_mismatch")
	}
	calls := make([]*geminiFunctionCall, 0, len(assistant.Parts))
	for index := range assistant.Parts {
		if assistant.Parts[index].FunctionCall != nil {
			calls = append(calls, assistant.Parts[index].FunctionCall)
		}
	}
	if len(calls) != len(last.Parts) {
		return failContinuation("continuation.gemini.resume_call_count_mismatch")
	}
	for index, part := range last.Parts {
		if part.FunctionResponse == nil || part.Text != nil ||
			part.FunctionCall != nil || part.ThoughtSignature != "" ||
			part.FunctionResponse.Name != calls[index].Name ||
			!validOpaqueText(
				[]byte(part.FunctionResponse.Response.Result),
			) {
			return failContinuation("continuation.gemini.resume_part_invalid")
		}
	}
	declarations, err := geminiDeclarations(definitions)
	if err != nil {
		return err
	}
	text := string(prompt)
	last.Parts = append(last.Parts, geminiPart{Text: &text})
	clearGeminiDeclarations(conversation.tools)
	conversation.tools = declarations
	return nil
}

func (conversation *geminiConversation) close() {
	if conversation == nil {
		return
	}
	conversation.ledger.Close()
	for content := range conversation.contents {
		for part := range conversation.contents[content].Parts {
			entry := &conversation.contents[content].Parts[part]
			if entry.Text != nil {
				*entry.Text = ""
			}
			entry.ThoughtSignature = ""
			if entry.FunctionCall != nil {
				clearBytes(entry.FunctionCall.Args)
			}
			if entry.FunctionResponse != nil {
				entry.FunctionResponse.Response.Result = ""
			}
		}
	}
	clearGeminiDeclarations(conversation.tools)
	conversation.baseURL = ""
	conversation.model = ""
	conversation.contents = nil
	conversation.pending = nil
	conversation.tools = nil
	conversation.step = 0
}

func clearGeminiDeclarations(declarations []geminiDeclaration) {
	for index := range declarations {
		clearBytes(declarations[index].Parameters)
	}
}

func safeJSONInt(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Int64()
	return parsed, err == nil && parsed >= 0
}

func canonicalBase64(value string) bool {
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil {
		return false
	}
	defer clearBytes(decoded)
	return base64.StdEncoding.EncodeToString(decoded) == value
}

// declaredReasoningEffort is honest absence for the Gemini dialect, which
// has no reasoning-effort request vocabulary.
func (*geminiConversation) declaredReasoningEffort() string { return "" }

func geminiThoughtSignatureRequired(model string) bool {
	normalized := strings.ToLower(model)
	return normalized == "gemini-3" ||
		strings.HasPrefix(normalized, "gemini-3-") ||
		strings.HasPrefix(normalized, "gemini-3.")
}
