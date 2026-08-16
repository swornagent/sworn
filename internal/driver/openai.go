package driver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
)

type openAIConversation struct {
	endpoint        string
	model           string
	dialect         providerDialect
	reasoningEffort string
	tools           []openAITool
	messages        []openAIMessage
	pending         []providerToolCall
	ledger          *continuationLedger
}

type openAIMessage struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content,omitempty"`
	ReasoningContent json.RawMessage `json:"reasoning_content,omitempty"`
	ReasoningDetails json.RawMessage `json:"reasoning_details,omitempty"`
	// ExtraContent carries the Google chat dialect's per-message opaque
	// thought-signature container verbatim. It exists only on the Google
	// dialect's assistant-message position, is never parsed for meaning, and
	// is replayed byte-exact on every subsequent request; close() clears it
	// with the rest of the message bytes.
	ExtraContent json.RawMessage  `json:"extra_content,omitempty"`
	ToolCalls    []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID   string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
	// ExtraContent carries the Google chat dialect's per-call opaque
	// thought-signature container verbatim at exactly this tool call's
	// position (fixture g4). It exists only on the Google dialect's
	// per-call position, is never parsed for meaning, and is replayed
	// byte-exact inside the same tool call on every subsequent request;
	// close() clears it with the rest of the message bytes.
	ExtraContent json.RawMessage `json:"extra_content,omitempty"`
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

type OpenAIAPI string

const (
	OpenAIResponsesAPI           OpenAIAPI = "responses"
	OpenAIChatCompletionsAPI     OpenAIAPI = "chat_completions"
	OpenRouterChatCompletionsAPI OpenAIAPI = "openrouter_chat_completions"
)

func (api OpenAIAPI) valid() bool {
	switch api {
	case OpenAIResponsesAPI,
		OpenAIChatCompletionsAPI,
		OpenRouterChatCompletionsAPI:
		return true
	default:
		return false
	}
}

type OpenAIProfileConfig struct {
	HTTPProfileConfig
	API              OpenAIAPI `json:"api"`
	ReasoningEffort  string    `json:"reasoning_effort,omitempty"`
	ReasoningEfforts []string  `json:"reasoning_efforts,omitempty"`
	// Stream enables SSE streaming on the responses flavour: events render
	// live while the terminal event's embedded response object feeds the
	// exact non-streaming validation path.
	Stream bool `json:"stream,omitempty"`
	// EnableThinking is Qwen's thinking toggle on the responses flavour,
	// carried verbatim when set.
	EnableThinking  *bool         `json:"enable_thinking,omitempty"`
	Preset          string        `json:"preset,omitempty"`
	AuthMode        AuthMode      `json:"auth_mode,omitempty"`
	Chain           *AWSChainSpec `json:"chain,omitempty"`
	OpaqueReasoning *bool         `json:"opaque_reasoning,omitempty"`
	// ThoughtSignature selects the Google chat dialect: the provider's
	// per-message opaque thought-signature container is admitted at the
	// assistant-message position, retained, and replayed byte-exact, and any
	// container that cannot be placed fails closed. Chat completions only.
	ThoughtSignature *bool `json:"thought_signature,omitempty"`
	// VendorUsage selects the xAI dialects (chat completions and responses):
	// the recorded vendor usage decorations are admitted at the usage
	// position, tolerated-and-ignored, so accounting still reads only the
	// standard token fields.
	VendorUsage *bool `json:"vendor_usage,omitempty"`
}

// effectiveAuth returns the admission-time authentication mode of a unified
// OpenAI-compatible adapter. Omission keeps the legacy deterministic default:
// a configured AWS chain means SigV4, everything else means bearer. The
// explicit none mode is the only way to obtain a credential-less adapter.
func (config OpenAIProfileConfig) effectiveAuth() AuthMode {
	if config.AuthMode != "" {
		return config.AuthMode
	}
	if config.Chain != nil {
		return AuthModeAWSSigV4
	}
	return AuthModeBearer
}

// validOpenAIAuth is fail-closed over the authentication surface: bearer and
// SigV4 require credential references, SigV4 requires an AWS chain and forbids
// header auth, and none requires the absence of every credential artifact.
func validOpenAIAuth(config *OpenAIProfileConfig) bool {
	if config == nil {
		return false
	}
	switch config.effectiveAuth() {
	case AuthModeNone:
		return len(config.CredentialRefs) == 0 &&
			config.CredentialHeader == "" &&
			config.CredentialPrefix == "" &&
			config.Chain == nil
	case AuthModeBearer:
		return len(config.CredentialRefs) > 0 && config.Chain == nil
	case AuthModeAWSSigV4:
		return len(config.CredentialRefs) > 0 &&
			config.Chain != nil &&
			config.CredentialHeader == "" &&
			config.CredentialPrefix == ""
	default:
		return false
	}
}

func NewOpenAIAdapter(
	config OpenAIProfileConfig,
	resolver HeaderCredentialResolver,
	awsResolver AWSRuntimeResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (Adapter, error) {
	if !config.valid() || !validOpenAIAuth(&config) {
		return nil, fail("INVALID_ADAPTER")
	}
	var transport providerTransport
	switch config.effectiveAuth() {
	case AuthModeNone:
		httpTransport, err := newHTTPTransport(
			config.HTTPProfileConfig,
			AuthModeNone,
			nil,
			probe,
			roundTripper,
		)
		if err != nil {
			return nil, err
		}
		config.CredentialRefs = append(
			[]string(nil),
			httpTransport.config.CredentialRefs...,
		)
		transport = httpTransport
	case AuthModeBearer:
		httpTransport, err := newHTTPTransport(
			config.HTTPProfileConfig,
			AuthModeBearer,
			resolver,
			probe,
			roundTripper,
		)
		if err != nil {
			return nil, err
		}
		config.HTTPProfileConfig = httpTransport.config
		transport = httpTransport
	case AuthModeAWSSigV4:
		bedrockTransport, err := newBedrockTransport(
			BedrockProfileConfig{
				Key:            config.Key,
				ID:             config.ID,
				Version:        config.Version,
				Endpoint:       config.Endpoint,
				CredentialRefs: append([]string(nil), config.CredentialRefs...),
				ResponseBytes:  config.ResponseBytes,
				Chain:          *config.Chain,
			},
			// The transport-level signing surface stays internal: the
			// adapter's reported surface remains the OpenAI chat surface
			// while Bedrock Mantle signs the Chat Completions payload.
			ProfileSurfaceBedrockMantleChat,
			awsResolver,
			probe,
			roundTripper,
		)
		if err != nil {
			return nil, err
		}
		chain := bedrockTransport.config.Chain
		config.Chain = &chain
		config.CredentialRefs = append(
			[]string(nil),
			bedrockTransport.config.CredentialRefs...,
		)
		transport = bedrockTransport
	default:
		return nil, fail("INVALID_ADAPTER")
	}
	var factory providerConversationFactory
	surface := ProfileSurfaceOpenAIChat
	dialect := providerDialectOpenAIChat
	if config.OpaqueReasoning != nil && *config.OpaqueReasoning {
		dialect = providerDialectOpaqueChat
	}
	switch config.API {
	case OpenAIResponsesAPI:
		surface = ProfileSurfaceOpenAIResponses
		dialect = providerDialectOpenAIResponses
		if config.VendorUsage != nil && *config.VendorUsage {
			dialect = providerDialectXAIResponses
		}
		factory = func(
			prompt []byte,
			model string,
			tools []providerToolDefinition,
		) (providerConversation, error) {
			return newResponsesConversation(
				config.Endpoint,
				model,
				tools,
				prompt,
				config.ReasoningEffort,
				config.EnableThinking,
				config.Stream,
				dialect,
			)
		}
	case OpenAIChatCompletionsAPI, OpenRouterChatCompletionsAPI:
		if config.API == OpenRouterChatCompletionsAPI {
			surface = ProfileSurfaceOpenRouterChat
			dialect = providerDialectOpenRouterChat
		}
		if config.ThoughtSignature != nil && *config.ThoughtSignature {
			dialect = providerDialectGoogleChat
		}
		if config.VendorUsage != nil && *config.VendorUsage {
			dialect = providerDialectXAIChat
		}
		factory = func(
			prompt []byte,
			model string,
			tools []providerToolDefinition,
		) (providerConversation, error) {
			return newOpenAIConversation(
				config.Endpoint,
				model,
				tools,
				prompt,
				dialect,
				config.ReasoningEffort,
			)
		}
	default:
		return nil, fail("INVALID_ADAPTER")
	}
	configuration := struct {
		OpenAIProfileConfig
		Family ProfileFamily
	}{config, ProfileOpenAIHTTP}
	return newLoopAdapter(
		config.Key,
		config.ID,
		config.Version,
		ProfileOpenAIHTTP,
		surface,
		dialect,
		configuration,
		factory,
		transport,
	)
}

func (config OpenAIProfileConfig) valid() bool {
	parsed, err := url.Parse(config.Endpoint)
	if validateEndpoint(config.Endpoint) != nil ||
		err != nil || parsed.RawQuery != "" ||
		!validReasoningEfforts(config.ReasoningEfforts) {
		return false
	}
	// The vendor dialects are an explicit one-per-adapter choice: a profile
	// cannot enable two vendor dialects at once, and each flag is admitted
	// only on the API surface that surface defines (ThoughtSignature is
	// chat completions only; VendorUsage is chat completions or responses;
	// the OpenRouter and OpaqueReasoning dialects never combine with them).
	thoughtSignature := config.ThoughtSignature != nil && *config.ThoughtSignature
	vendorUsage := config.VendorUsage != nil && *config.VendorUsage
	opaqueReasoning := config.OpaqueReasoning != nil && *config.OpaqueReasoning
	if (thoughtSignature && vendorUsage) ||
		(opaqueReasoning && (thoughtSignature || vendorUsage)) {
		return false
	}
	// Streaming is a responses-flavour capability only; the chat flavours
	// keep the exact non-streaming request shape.
	switch config.API {
	case OpenAIChatCompletionsAPI, OpenRouterChatCompletionsAPI:
		if thoughtSignature && config.API != OpenAIChatCompletionsAPI {
			return false
		}
		if vendorUsage && config.API == OpenRouterChatCompletionsAPI {
			return false
		}
		return !config.Stream &&
			(config.ReasoningEffort == "" ||
				config.declaresReasoningEffort(config.ReasoningEffort))
	case OpenAIResponsesAPI:
		if thoughtSignature {
			return false
		}
		return config.ReasoningEffort != "" &&
			config.declaresReasoningEffort(config.ReasoningEffort)
	default:
		return false
	}
}

// declaresReasoningEffort reports whether value is admitted by this profile.
// A declared per-profile vocabulary wins; when none is declared the global
// backward-compatibility set is the vocabulary.
func (config OpenAIProfileConfig) declaresReasoningEffort(value string) bool {
	if len(config.ReasoningEfforts) != 0 {
		return slices.Contains(config.ReasoningEfforts, value)
	}
	return validOpenAIReasoningEffort(value)
}

// validReasoningEfforts requires a canonical per-profile vocabulary: values
// are non-empty bounded text, strictly increasing, and never duplicated, so
// two profiles declaring the same capability canonicalize identically.
func validReasoningEfforts(values []string) bool {
	for index, value := range values {
		if value == "" ||
			validateText(value, 128, false) != nil ||
			(index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}

func newOpenAIConversation(
	endpoint, model string,
	definitions []providerToolDefinition,
	prompt []byte,
	dialect providerDialect,
	reasoningEffort string,
) (*openAIConversation, error) {
	if validateEndpoint(endpoint) != nil ||
		validateText(model, 500, false) != nil ||
		(dialect != providerDialectOpenAIChat &&
			dialect != providerDialectOpenRouterChat &&
			dialect != providerDialectOpaqueChat &&
			dialect != providerDialectGoogleChat &&
			dialect != providerDialectXAIChat) {
		return nil, fail("INVALID_ADAPTER")
	}
	tools, err := openAITools(definitions)
	if err != nil {
		return nil, err
	}
	content, _ := json.Marshal(string(prompt))
	return &openAIConversation{
		endpoint:        endpoint,
		model:           model,
		dialect:         dialect,
		reasoningEffort: reasoningEffort,
		tools:           tools,
		messages:        []openAIMessage{{Role: "user", Content: content}},
		ledger:          newContinuationLedger(),
	}, nil
}

func openAITools(definitions []providerToolDefinition) ([]openAITool, error) {
	if len(definitions) == 0 || len(definitions) > MaxToolCalls {
		return nil, fail("CONTINUATION_INVALID")
	}
	tools := make([]openAITool, len(definitions))
	for index, definition := range definitions {
		if !providerKeyPattern.MatchString(definition.Name) ||
			len(definition.InputSchema) == 0 ||
			len(definition.InputSchema) > MaxToolArgumentBytes {
			return nil, fail("CONTINUATION_INVALID")
		}
		tools[index].Type = "function"
		tools[index].Function.Name = definition.Name
		tools[index].Function.Description = definition.Description
		tools[index].Function.Parameters =
			append([]byte(nil), definition.InputSchema...)
	}
	return tools, nil
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
		Effort     string          `json:"reasoning_effort,omitempty"`
	}{
		Model: conversation.model, Messages: conversation.messages,
		Tools: conversation.tools, ToolChoice: "auto", Stream: false,
		Effort: conversation.reasoningEffort,
	})
	if err != nil || len(body) > MaxProviderRequestBytes {
		clearBytes(body)
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	return providerRequest{
		Method: "POST", URL: conversation.endpoint,
		ContentType: "application/json", Body: body,
	}, nil
}

func validOpenAIReasoningEffort(value string) bool {
	switch value {
	case "", "none", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func (conversation *openAIConversation) declaredReasoningEffort() string {
	if conversation == nil {
		return ""
	}
	return conversation.reasoningEffort
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
		"system_fingerprint", "service_tier", "reasoning_effort",
	})
	if err != nil {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if providerError, present := root["error"]; present && providerError != nil {
		return providerTurn{}, fail("PROVIDER_ERROR")
	}
	var effort *string
	if effortValue, present := root["reasoning_effort"]; present &&
		effortValue != nil {
		effortText, ok := effortValue.(string)
		if !ok || validateText(effortText, 128, false) != nil {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		effort = &effortText
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
	if err != nil {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	finishReason, finishOK := choice["finish_reason"].(string)
	if !finishOK {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if finishReason != "tool_calls" && finishReason != "stop" {
		if finishReason != "length" {
			return providerTurn{}, fail("MISSING_SUBMISSION")
		}
		// The provider hit its output-token ceiling: report the explicit
		// named failure carrying the provider's own finish reason instead of
		// an empty-looking success.
		turn := providerTurn{
			ReasoningEffort: effort,
			FinishReason:    &finishReason,
			Truncated:       true,
		}
		if usageValue, present := root["usage"]; present && usageValue != nil {
			usage, usageErr := openAIUsage(usageValue, conversation.dialect)
			if usageErr != nil {
				return providerTurn{}, usageErr
			}
			turn.Usage = usage
		}
		return turn, nil
	}
	// The assistant-message allowlist is closed. Under the Google chat
	// dialect exactly two structural positions are admitted beyond the
	// common set: the message-level extra_content (per-message thought
	// signature) and, on a tool-call-only turn, the content field itself may
	// be absent entirely (fixture g4). Everywhere else extra_content stays an
	// unknown field and fails exactly as it fails today.
	messageRequired := []string{"role", "content"}
	messageOptional := []string{
		"tool_calls",
		"reasoning", "reasoning_content", "reasoning_details",
		"refusal", "annotations",
	}
	if conversation.dialect == providerDialectGoogleChat {
		messageRequired = []string{"role"}
		messageOptional = append(messageOptional, "content", "extra_content")
	}
	rawMessage, err := closedObject(
		choice["message"],
		messageRequired,
		messageOptional,
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
	rawToolCalls := []any(nil)
	if rawToolCallsValue, present := rawMessage["tool_calls"]; present &&
		rawToolCallsValue != nil {
		var ok bool
		rawToolCalls, ok = rawToolCallsValue.([]any)
		if !ok || len(rawToolCalls) > MaxToolCalls {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
	}
	if (finishReason == "tool_calls") != (len(rawToolCalls) > 0) {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	message := openAIMessage{Role: "assistant"}
	switch content := rawMessage["content"].(type) {
	case nil:
		// Under the Google dialect an absent content field stays absent: the
		// recorded g4 tool-call-only assistant message carries no content key,
		// so `json:"content,omitempty"` reproduces that shape on the replayed
		// request instead of a "null" the provider never sent.
		if conversation.dialect != providerDialectGoogleChat {
			message.Content = json.RawMessage("null")
		}
	case string:
		if !validOpaqueText([]byte(content)) {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		message.Content, _ = json.Marshal(content)
	default:
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	// Content-optionality is scoped to the recorded tool-call-only turn: an
	// assistant message with neither content nor tool calls would otherwise
	// decode as an empty prose nudge, so it fails CONTINUATION_INVALID exactly
	// as it does on every other dialect.
	if conversation.dialect == providerDialectGoogleChat &&
		len(message.Content) == 0 && len(rawToolCalls) == 0 {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if reasoningValue, present := rawMessage["reasoning_content"]; present &&
		reasoningValue != nil {
		if conversation.dialect != providerDialectOpaqueChat {
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
	if detailsValue, present := rawMessage["reasoning_details"]; present &&
		detailsValue != nil {
		if conversation.dialect != providerDialectOpenRouterChat {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		var raw struct {
			Choices []struct {
				Message struct {
					ReasoningDetails json.RawMessage `json:"reasoning_details"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(body, &raw) != nil ||
			len(raw.Choices) != 1 ||
			validateOpenRouterReasoningDetails(
				detailsValue,
				raw.Choices[0].Message.ReasoningDetails,
			) != nil {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		retained, retainErr := conversation.ledger.retain(opaqueField{
			kind: opaqueText,
			body: raw.Choices[0].Message.ReasoningDetails,
		})
		if retainErr != nil {
			return providerTurn{}, retainErr
		}
		message.ReasoningDetails =
			append(json.RawMessage(nil), retained[0]...)
	}
	// Google's thought signature is the model's reasoning continuity: it is
	// retained as opaque state and replayed byte-exact on every subsequent
	// turn. Replay is mandatory, never best-effort - the recorded probe shows
	// the model silently contradicts its own prior turn when the signature is
	// absent - so a container that is missing or cannot be retained or placed
	// fails closed with CONTINUATION_STATE_UNPLAYABLE rather than re-requesting
	// without it.
	if conversation.dialect == providerDialectGoogleChat {
		// On a prose turn the per-message thought signature is mandatory (the
		// recorded g5 response carries it at the message level). On a
		// tool-call-only turn the signature rides per call (fixture g4), so
		// the message-level container is not required there - but when the
		// provider does carry it, it is still admitted, retained, and replayed
		// byte-exact, never dropped.
		extraValue, present := rawMessage["extra_content"]
		if len(rawToolCalls) == 0 && (!present || extraValue == nil) {
			return providerTurn{}, fail("CONTINUATION_STATE_UNPLAYABLE")
		}
		if present && extraValue != nil {
			var raw struct {
				Choices []struct {
					Message struct {
						ExtraContent json.RawMessage `json:"extra_content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if json.Unmarshal(body, &raw) != nil || len(raw.Choices) != 1 ||
				validateGoogleExtraContent(
					extraValue,
					raw.Choices[0].Message.ExtraContent,
				) != nil {
				return providerTurn{}, fail("CONTINUATION_STATE_UNPLAYABLE")
			}
			retained, retainErr := conversation.ledger.retain(opaqueField{
				kind: opaqueText,
				body: raw.Choices[0].Message.ExtraContent,
			})
			if retainErr != nil {
				return providerTurn{}, fail("CONTINUATION_STATE_UNPLAYABLE")
			}
			message.ExtraContent = append(json.RawMessage(nil), retained[0]...)
		}
	}
	calls := make([]providerToolCall, 0, len(rawToolCalls))
	// Under the Google dialect each tool call may carry its own opaque
	// thought-signature container at the per-call position (fixture g4). A
	// present container must match the recorded closed shape, is retained as
	// opaque continuation state keyed to its call, and is replayed byte-exact
	// inside the same call on the following request; a container that cannot
	// be validated or retained fails the whole turn closed.
	type perCallState struct {
		extra json.RawMessage
	}
	perCall := make([]perCallState, len(rawToolCalls))
	perCallCount := 0
	for index, rawToolCall := range rawToolCalls {
		toolCallOptional := []string{"index"}
		if conversation.dialect == providerDialectGoogleChat {
			toolCallOptional = append(toolCallOptional, "extra_content")
		}
		toolCall, toolErr := closedObject(
			rawToolCall,
			[]string{"id", "type", "function"},
			toolCallOptional,
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
		if conversation.dialect == providerDialectGoogleChat {
			// A present per-call extra_content must be a playable container:
			// null is not the recorded shape, so it fails closed rather than
			// being silently dropped from the replay.
			if extraValue, present := toolCall["extra_content"]; present {
				raw := googlePerCallContainerFromResponse(body, index)
				if validateGoogleExtraContent(extraValue, raw) != nil {
					return providerTurn{}, fail("CONTINUATION_STATE_UNPLAYABLE")
				}
				perCall[index].extra = raw
				perCallCount++
			}
		}
		encodedArguments, _ := json.Marshal(argumentsText)
		message.ToolCalls = append(message.ToolCalls, openAIToolCall{
			ID: id, Type: "function",
			Function: openAIFunction{
				Name: name, Arguments: encodedArguments,
			},
		})
	}
	if perCallCount != 0 {
		// All per-call containers of one turn are retained in a single ledger
		// step, bounded by the same opaque-field budgets as every other
		// retained vendor field; an over-budget turn fails closed.
		fields := make([]opaqueField, 0, perCallCount)
		for _, state := range perCall {
			if len(state.extra) != 0 {
				fields = append(fields, opaqueField{
					kind: opaqueText, body: state.extra,
				})
			}
		}
		retained, retainErr := conversation.ledger.retain(fields...)
		if retainErr != nil {
			return providerTurn{}, fail("CONTINUATION_STATE_UNPLAYABLE")
		}
		retainIndex := 0
		for index := range message.ToolCalls {
			if len(perCall[index].extra) != 0 {
				message.ToolCalls[index].ExtraContent =
					append(json.RawMessage(nil), retained[retainIndex]...)
				retainIndex++
			}
		}
	}
	conversation.messages = append(conversation.messages, message)
	conversation.pending = append([]providerToolCall(nil), calls...)
	turn := providerTurn{Calls: calls, Prose: len(calls) == 0}
	if effort != nil {
		turn.ReasoningEffort = effort
	}
	if usageValue, present := root["usage"]; present && usageValue != nil {
		usage, usageErr := openAIUsage(usageValue, conversation.dialect)
		if usageErr != nil {
			return providerTurn{}, usageErr
		}
		turn.Usage = usage
	}
	return turn, nil
}

// openAIUsage parses the chat-completions usage object into the normalized
// turn accounting. The OpenAI cache vocabulary (prompt_cache_hit_tokens ->
// read, prompt_cache_miss_tokens -> write) is surfaced instead of discarded;
// each side stays nil when the provider omits it. On the xAI chat dialect the
// recorded vendor usage decorations are admitted at this position and
// tolerated-and-ignored, so the normalized accounting still reads only the
// standard fields.
func openAIUsage(value any, dialect providerDialect) (*Usage, error) {
	optional := []string{
		"total_tokens", "prompt_tokens_details", "completion_tokens_details",
		"prompt_cache_hit_tokens", "prompt_cache_miss_tokens",
	}
	if dialect == providerDialectXAIChat {
		optional = append(optional,
			"num_sources_used",
			"num_server_side_tools_used",
			"cost_in_usd_ticks",
			"context_details",
		)
	}
	usage, err := closedObject(
		value,
		[]string{"prompt_tokens", "completion_tokens"},
		optional,
	)
	if err != nil {
		return nil, fail("INVALID_USAGE")
	}
	input, inputOK := safeJSONInt(usage["prompt_tokens"])
	output, outputOK := safeJSONInt(usage["completion_tokens"])
	if !inputOK || !outputOK {
		return nil, fail("INVALID_USAGE")
	}
	result := &Usage{InputTokens: input, OutputTokens: output}
	if _, present := usage["prompt_cache_hit_tokens"]; present {
		read, readOK := safeJSONInt(usage["prompt_cache_hit_tokens"])
		if !readOK {
			return nil, fail("INVALID_USAGE")
		}
		result.CacheReadTokens = &read
	}
	if _, present := usage["prompt_cache_miss_tokens"]; present {
		write, writeOK := safeJSONInt(usage["prompt_cache_miss_tokens"])
		if !writeOK {
			return nil, fail("INVALID_USAGE")
		}
		result.CacheWriteTokens = &write
	}
	return result, nil
}

func (conversation *openAIConversation) appendInstruction(body []byte) error {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(body) == 0 || len(body) > MaxOpaqueFieldBytes ||
		!validOpaqueText(body) {
		return fail("CONTINUATION_INVALID")
	}
	content, err := json.Marshal(string(body))
	if err != nil || len(content) > MaxOpaqueFieldBytes {
		clearBytes(content)
		return fail("CONTINUATION_INVALID")
	}
	conversation.messages = append(conversation.messages, openAIMessage{
		Role: "user", Content: content,
	})
	return nil
}

func validateOpenRouterReasoningDetails(value any, raw json.RawMessage) error {
	details, ok := value.([]any)
	if !ok || len(details) == 0 || len(details) > MaxContinuationSteps ||
		len(raw) == 0 || len(raw) > MaxOpaqueFieldBytes ||
		!validOpaqueText(raw) {
		return fail("CONTINUATION_INVALID")
	}
	for index, rawDetail := range details {
		detail, err := closedObject(
			rawDetail,
			[]string{"type", "format"},
			[]string{
				"id", "index", "summary", "data", "text", "signature",
			},
		)
		if err != nil {
			return fail("CONTINUATION_INVALID")
		}
		detailType, typeOK := detail["type"].(string)
		format, formatOK := detail["format"].(string)
		if !typeOK || !formatOK ||
			validateText(format, 128, false) != nil {
			return fail("CONTINUATION_INVALID")
		}
		switch format {
		case "unknown",
			"openai-responses-v1",
			"azure-openai-responses-v1",
			"bedrock-openai-responses-v1",
			"xai-responses-v1",
			"meta-responses-v1",
			"anthropic-claude-v1",
			"google-gemini-v1":
		default:
			return fail("CONTINUATION_INVALID")
		}
		if id, present := detail["id"]; present && id != nil {
			idText, idOK := id.(string)
			if !idOK || validateText(idText, MaxCorrelationIDBytes, false) != nil {
				return fail("CONTINUATION_INVALID")
			}
		}
		if itemIndex, present := detail["index"]; present {
			parsed, parsedOK := safeJSONInt(itemIndex)
			if !parsedOK || parsed != int64(index) {
				return fail("CONTINUATION_INVALID")
			}
		}
		requiredField := ""
		switch detailType {
		case "reasoning.summary":
			requiredField = "summary"
		case "reasoning.encrypted":
			requiredField = "data"
		case "reasoning.text":
			requiredField = "text"
		default:
			return fail("CONTINUATION_INVALID")
		}
		for _, field := range []string{"summary", "data", "text"} {
			fieldValue, present := detail[field]
			if field == requiredField {
				text, textOK := fieldValue.(string)
				if !present || !textOK || len(text) > MaxOpaqueFieldBytes ||
					!validOpaqueText([]byte(text)) {
					return fail("CONTINUATION_INVALID")
				}
			} else if present {
				return fail("CONTINUATION_INVALID")
			}
		}
		if signature, present := detail["signature"]; present &&
			signature != nil {
			signatureText, signatureOK := signature.(string)
			if detailType != "reasoning.text" || !signatureOK ||
				len(signatureText) > MaxOpaqueFieldBytes ||
				!validOpaqueText([]byte(signatureText)) {
				return fail("CONTINUATION_INVALID")
			}
		} else if present && detailType != "reasoning.text" {
			return fail("CONTINUATION_INVALID")
		}
	}
	return nil
}

// validateGoogleExtraContent admits exactly the recorded Google
// extra_content container at the assistant-message position: a bounded JSON
// object whose field set is exactly {google:{thought_signature}}, with the
// signature a non-empty bounded valid opaque string and the raw container
// within the opaque field budget. The extension point is defined by the
// recorded exchange, never a wildcard.
func validateGoogleExtraContent(value any, raw json.RawMessage) error {
	if value == nil || len(raw) == 0 || len(raw) > MaxOpaqueFieldBytes ||
		!validOpaqueText(raw) {
		return fail("CONTINUATION_INVALID")
	}
	container, ok := value.(map[string]any)
	if !ok || len(container) != 1 {
		return fail("CONTINUATION_INVALID")
	}
	google, ok := container["google"].(map[string]any)
	if !ok || len(google) != 1 {
		return fail("CONTINUATION_INVALID")
	}
	signature, ok := google["thought_signature"].(string)
	if !ok || signature == "" || len(signature) > MaxOpaqueFieldBytes ||
		!validOpaqueText([]byte(signature)) {
		return fail("CONTINUATION_INVALID")
	}
	return nil
}

// googlePerCallContainerFromResponse extracts the raw extra_content container
// bytes of one tool call from a Google chat-completions response, byte-for-byte
// (the same targeted json.Unmarshal pattern the message-level container uses,
// extended one level into tool_calls[index]).
func googlePerCallContainerFromResponse(body []byte, index int) json.RawMessage {
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
		index < 0 || index >= len(raw.Choices[0].Message.ToolCalls) {
		return nil
	}
	return append(
		json.RawMessage(nil),
		raw.Choices[0].Message.ToolCalls[index].ExtraContent...,
	)
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

func (conversation *openAIConversation) resume(
	prompt []byte,
	definitions []providerToolDefinition,
) error {
	if conversation == nil || len(conversation.pending) != 0 ||
		len(prompt) == 0 || len(prompt) > MaxProviderRequestBytes ||
		!validOpenAIResumeTail(conversation.messages) {
		return fail("CONTINUATION_INVALID")
	}
	tools, err := openAITools(definitions)
	if err != nil {
		return err
	}
	content, err := json.Marshal(string(prompt))
	if err != nil || len(content) > MaxProviderRequestBytes {
		clearBytes(content)
		clearOpenAITools(tools)
		return fail("CONTINUATION_INVALID")
	}
	clearOpenAITools(conversation.tools)
	conversation.tools = tools
	conversation.messages = append(conversation.messages, openAIMessage{
		Role: "user", Content: content,
	})
	return nil
}

func validOpenAIResumeTail(messages []openAIMessage) bool {
	if len(messages) < 3 {
		return false
	}
	assistant := messages[len(messages)-2]
	result := messages[len(messages)-1]
	if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 ||
		result.Role != "tool" || result.ToolCallID == "" ||
		result.ToolCallID != assistant.ToolCalls[0].ID ||
		len(result.ToolCalls) != 0 ||
		len(result.ReasoningContent) != 0 ||
		len(result.ReasoningDetails) != 0 {
		return false
	}
	var content string
	return json.Unmarshal(result.Content, &content) == nil &&
		validOpaqueText([]byte(content))
}

func (conversation *openAIConversation) close() {
	if conversation == nil {
		return
	}
	conversation.ledger.Close()
	for index := range conversation.messages {
		clearBytes(conversation.messages[index].Content)
		clearBytes(conversation.messages[index].ReasoningContent)
		clearBytes(conversation.messages[index].ReasoningDetails)
		clearBytes(conversation.messages[index].ExtraContent)
		for tool := range conversation.messages[index].ToolCalls {
			clearBytes(conversation.messages[index].ToolCalls[tool].Function.Arguments)
			clearBytes(conversation.messages[index].ToolCalls[tool].ExtraContent)
		}
	}
	clearOpenAITools(conversation.tools)
	conversation.endpoint = ""
	conversation.model = ""
	conversation.dialect = ""
	conversation.reasoningEffort = ""
	conversation.messages = nil
	conversation.pending = nil
	conversation.tools = nil
}

func clearOpenAITools(tools []openAITool) {
	for index := range tools {
		clearBytes(tools[index].Function.Parameters)
	}
}
