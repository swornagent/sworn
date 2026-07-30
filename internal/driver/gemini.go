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

type geminiFunctionResponse struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Response struct {
		Output string `json:"output"`
		Failed bool   `json:"failed"`
	} `json:"response"`
}

type geminiDeclaration struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	ParametersJSONSchema json.RawMessage `json:"parametersJsonSchema"`
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
	transport, err := newHTTPTransport(config, resolver, probe, roundTripper)
	if err != nil {
		return nil, err
	}
	factory := func(
		prompt []byte,
		model string,
		tools []providerToolDefinition,
	) (providerConversation, error) {
		return newGeminiConversation(config.Endpoint, model, tools, prompt)
	}
	configuration := struct {
		HTTPProfileConfig
		Family ProfileFamily
	}{transport.config, ProfileGemini}
	return newLoopAdapter(
		config.Key, config.ID, config.Version, ProfileGemini,
		"", providerDialectGemini, configuration, factory, transport,
	)
}

func newGeminiConversation(
	baseURL, model string,
	definitions []providerToolDefinition,
	prompt []byte,
) (*geminiConversation, error) {
	if validateEndpoint(baseURL) != nil || validateText(model, 500, false) != nil {
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
		ledger: newContinuationLedger(),
	}, nil
}

func geminiDeclarations(
	definitions []providerToolDefinition,
) ([]geminiDeclaration, error) {
	if len(definitions) == 0 || len(definitions) > MaxToolCalls {
		return nil, fail("CONTINUATION_INVALID")
	}
	declarations := make([]geminiDeclaration, len(definitions))
	for index, definition := range definitions {
		if !providerKeyPattern.MatchString(definition.Name) ||
			len(definition.InputSchema) == 0 ||
			len(definition.InputSchema) > MaxToolArgumentBytes {
			return nil, fail("CONTINUATION_INVALID")
		}
		declarations[index] = geminiDeclaration{
			Name: definition.Name, Description: definition.Description,
			ParametersJSONSchema: append([]byte(nil), definition.InputSchema...),
		}
	}
	return declarations, nil
}

func (conversation *geminiConversation) request() (providerRequest, error) {
	if conversation == nil || len(conversation.pending) != 0 {
		return providerRequest{}, fail("CONTINUATION_INVALID")
	}
	body, err := json.Marshal(struct {
		Contents []geminiContent `json:"contents"`
		Tools    []struct {
			FunctionDeclarations []geminiDeclaration `json:"functionDeclarations"`
		} `json:"tools"`
		GenerationConfig struct {
			CandidateCount int `json:"candidateCount"`
		} `json:"generationConfig"`
	}{
		Contents: conversation.contents,
		Tools: []struct {
			FunctionDeclarations []geminiDeclaration `json:"functionDeclarations"`
		}{{FunctionDeclarations: conversation.tools}},
		GenerationConfig: struct {
			CandidateCount int `json:"candidateCount"`
		}{CandidateCount: 1},
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
		return providerTurn{}, fail("CONTINUATION_INVALID")
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
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	candidates, ok := root["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	candidate, err := closedObject(
		candidates[0],
		[]string{"content", "finishReason"},
		[]string{
			"index", "safetyRatings", "citationMetadata", "tokenCount",
			"finishMessage",
		},
	)
	if err != nil || candidate["finishReason"] != "STOP" {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	content, err := closedObject(candidate["content"], []string{"role", "parts"}, nil)
	if err != nil || content["role"] != "model" {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	rawParts, ok := content["parts"].([]any)
	if !ok || len(rawParts) == 0 || len(rawParts) > MaxToolCalls {
		return providerTurn{}, fail("CONTINUATION_INVALID")
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
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		_, hasText := part["text"]
		functionValue, hasCall := part["functionCall"]
		if hasText == hasCall {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		decoded := geminiPart{}
		if hasText {
			text, ok := part["text"].(string)
			if !ok || !validOpaqueText([]byte(text)) {
				return providerTurn{}, fail("CONTINUATION_INVALID")
			}
			decoded.Text = &text
		} else {
			function, functionErr := closedObject(
				functionValue,
				[]string{"name", "args"},
				[]string{"id"},
			)
			if functionErr != nil {
				return providerTurn{}, fail("CONTINUATION_INVALID")
			}
			name, nameOK := function["name"].(string)
			if !nameOK || !providerKeyPattern.MatchString(name) {
				return providerTurn{}, fail("CONTINUATION_INVALID")
			}
			arguments, marshalErr := canonicalJSON(function["args"])
			if marshalErr != nil || len(arguments) > MaxToolArgumentBytes {
				return providerTurn{}, fail("CONTINUATION_INVALID")
			}
			providerID, _ := function["id"].(string)
			internalID := providerID
			if internalID == "" {
				internalID = "gemini-" + itoa(conversation.step+1) + "-" + itoa(index+1)
			}
			if conversation.ledger.correlate(internalID) != nil {
				return providerTurn{}, fail("CONTINUATION_INVALID")
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
				return providerTurn{}, fail("CONTINUATION_INVALID")
			}
			opaque = append(opaque, opaqueField{kind: opaqueBase64, body: []byte(signature)})
			decoded.ThoughtSignature = signature
		}
		if hasCall && len(calls) == 1 &&
			geminiThoughtSignatureRequired(conversation.model) &&
			decoded.ThoughtSignature == "" {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		parts = append(parts, decoded)
	}
	if len(calls) == 0 {
		return providerTurn{}, fail("MISSING_SUBMISSION")
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
	turn := providerTurn{Calls: calls}
	if usage, present := root["usageMetadata"]; present {
		metadata, metadataErr := closedObject(
			usage,
			nil,
			[]string{
				"promptTokenCount", "candidatesTokenCount", "totalTokenCount",
				"cachedContentTokenCount", "thoughtsTokenCount", "toolUsePromptTokenCount",
				"promptTokensDetails", "candidatesTokensDetails", "cacheTokensDetails",
				"toolUsePromptTokensDetails", "serviceTier",
			},
		)
		if metadataErr != nil {
			return providerTurn{}, fail("INVALID_USAGE")
		}
		input, inputOK := safeJSONInt(metadata["promptTokenCount"])
		output, outputOK := safeJSONInt(metadata["candidatesTokenCount"])
		if !inputOK || !outputOK {
			return providerTurn{}, fail("INVALID_USAGE")
		}
		turn.Usage = &Usage{InputTokens: input, OutputTokens: output}
	}
	return turn, nil
}

func (conversation *geminiConversation) appendResults(results []providerToolResult) error {
	if conversation == nil || len(results) != len(conversation.pending) {
		return fail("CONTINUATION_INVALID")
	}
	parts := make([]geminiPart, len(results))
	for index, result := range results {
		expected := conversation.pending[index]
		if result.ID != expected.call.ID || result.Name != expected.call.Name ||
			len(result.Content) > MaxToolResultBytes || !validOpaqueText(result.Content) {
			return fail("CONTINUATION_INVALID")
		}
		response := &geminiFunctionResponse{
			ID: expected.providerID, Name: expected.call.Name,
		}
		response.Response.Output = string(result.Content)
		response.Response.Failed = result.Failed
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
		return fail("CONTINUATION_INVALID")
	}
	last := &conversation.contents[len(conversation.contents)-1]
	assistant := conversation.contents[len(conversation.contents)-2]
	if last.Role != "user" || len(last.Parts) == 0 ||
		assistant.Role != "model" {
		return fail("CONTINUATION_INVALID")
	}
	calls := make([]*geminiFunctionCall, 0, len(assistant.Parts))
	for index := range assistant.Parts {
		if assistant.Parts[index].FunctionCall != nil {
			calls = append(calls, assistant.Parts[index].FunctionCall)
		}
	}
	if len(calls) != len(last.Parts) {
		return fail("CONTINUATION_INVALID")
	}
	for index, part := range last.Parts {
		if part.FunctionResponse == nil || part.Text != nil ||
			part.FunctionCall != nil || part.ThoughtSignature != "" ||
			part.FunctionResponse.ID != calls[index].ID ||
			part.FunctionResponse.Name != calls[index].Name ||
			!validOpaqueText(
				[]byte(part.FunctionResponse.Response.Output),
			) {
			return fail("CONTINUATION_INVALID")
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
				entry.FunctionResponse.Response.Output = ""
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
		clearBytes(declarations[index].ParametersJSONSchema)
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

func geminiThoughtSignatureRequired(model string) bool {
	normalized := strings.ToLower(model)
	return normalized == "gemini-3" ||
		strings.HasPrefix(normalized, "gemini-3-") ||
		strings.HasPrefix(normalized, "gemini-3.")
}
