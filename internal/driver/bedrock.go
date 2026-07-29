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
	) (providerConversation, error) {
		return newBedrockConversation(config, model, tools, prompt)
	}
	return newLoopAdapter(
		config.Key, config.ID, config.Version, ProfileBedrock,
		ProfileSurfaceBedrockRuntimeConverse,
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
) (*bedrockConversation, error) {
	if validateText(model, 500, false) != nil || validateEndpoint(config.Endpoint) != nil {
		return nil, fail("INVALID_ADAPTER")
	}
	tools := make([]bedrockTool, len(definitions))
	for index, definition := range definitions {
		tools[index].ToolSpec.Name = definition.Name
		tools[index].ToolSpec.Description = definition.Description
		tools[index].ToolSpec.InputSchema.JSON =
			append([]byte(nil), definition.InputSchema...)
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
		system:   "Use only the supplied Sworn workspace tools and terminate with sworn_submit.",
		tools:    tools, messages: []json.RawMessage{initialMessage},
		allowCachePoint:   config.AllowCachePoint,
		allowGuardContent: config.AllowGuardContent,
		ledger:            newContinuationLedger(),
	}, nil
}

func (conversation *bedrockConversation) request() (providerRequest, error) {
	if conversation == nil || len(conversation.pending) != 0 {
		return providerRequest{}, fail("CONTINUATION_INVALID")
	}
	body, err := json.Marshal(struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		Messages   []json.RawMessage `json:"messages"`
		ToolConfig struct {
			Tools []bedrockTool `json:"tools"`
		} `json:"toolConfig"`
	}{
		System: []struct {
			Text string `json:"text"`
		}{{Text: conversation.system}},
		Messages: conversation.messages,
		ToolConfig: struct {
			Tools []bedrockTool `json:"tools"`
		}{Tools: conversation.tools},
	})
	if err != nil || len(body) > MaxProviderRequestBytes {
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
		return providerTurn{}, fail("CONTINUATION_INVALID")
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
	if err != nil || root["stopReason"] != "tool_use" {
		return providerTurn{}, fail("PROVIDER_ERROR")
	}
	output, err := closedObject(root["output"], []string{"message"}, nil)
	if err != nil {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	message, err := closedObject(output["message"], []string{"role", "content"}, nil)
	if err != nil || message["role"] != "assistant" {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) == 0 || len(content) > MaxToolCalls {
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	var calls []providerToolCall
	var opaque []opaqueField
	for _, rawBlock := range content {
		block, ok := rawBlock.(map[string]any)
		if !ok || len(block) != 1 {
			return providerTurn{}, fail("CONTINUATION_INVALID")
		}
		for name, blockValue := range block {
			switch name {
			case "text":
				text, valid := blockValue.(string)
				if !valid || !validOpaqueText([]byte(text)) {
					return providerTurn{}, fail("CONTINUATION_INVALID")
				}
			case "toolUse":
				toolUse, toolErr := closedObject(
					blockValue,
					[]string{"toolUseId", "name", "input"},
					nil,
				)
				if toolErr != nil {
					return providerTurn{}, fail("CONTINUATION_INVALID")
				}
				id, idOK := toolUse["toolUseId"].(string)
				toolName, nameOK := toolUse["name"].(string)
				arguments, marshalErr := canonicalJSON(toolUse["input"])
				if !idOK || !nameOK || marshalErr != nil ||
					conversation.ledger.correlate(id) != nil ||
					!providerKeyPattern.MatchString(toolName) ||
					len(arguments) > MaxToolArgumentBytes {
					return providerTurn{}, fail("CONTINUATION_INVALID")
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
					return providerTurn{}, fail("CONTINUATION_INVALID")
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
						return providerTurn{}, fail("CONTINUATION_INVALID")
					}
					opaque = append(opaque, opaqueField{
						kind: opaqueBase64, body: []byte(signature),
					})
				} else {
					redacted, valid := reasoning["redactedContent"].(string)
					if !valid || redacted == "" {
						return providerTurn{}, fail("CONTINUATION_INVALID")
					}
					opaque = append(opaque, opaqueField{
						kind: opaqueBase64, body: []byte(redacted),
					})
				}
			case "cachePoint":
				cache, cacheErr := closedObject(blockValue, []string{"type"}, nil)
				if !conversation.allowCachePoint || cacheErr != nil ||
					cache["type"] != "default" {
					return providerTurn{}, fail("CONTINUATION_INVALID")
				}
			case "guardContent":
				if !conversation.allowGuardContent {
					return providerTurn{}, fail("CONTINUATION_INVALID")
				}
				guard, valid := blockValue.(map[string]any)
				if !valid || len(guard) != 1 {
					return providerTurn{}, fail("CONTINUATION_INVALID")
				}
				for guardKind, guardValue := range guard {
					if guardKind != "text" && guardKind != "image" {
						return providerTurn{}, fail("CONTINUATION_INVALID")
					}
					canonical, canonicalErr := canonicalJSON(guardValue)
					if canonicalErr != nil || len(canonical) > MaxOpaqueFieldBytes {
						return providerTurn{}, fail("CONTINUATION_INVALID")
					}
					clearBytes(canonical)
				}
			default:
				return providerTurn{}, fail("CONTINUATION_INVALID")
			}
		}
	}
	if len(calls) == 0 {
		return providerTurn{}, fail("CONTINUATION_INVALID")
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
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	if len(rawResponse.Output.Message) > MaxOpaqueStepBytes {
		clearBytes(rawResponse.Output.Message)
		return providerTurn{}, fail("CONTINUATION_INVALID")
	}
	conversation.messages = append(
		conversation.messages,
		append(json.RawMessage(nil), rawResponse.Output.Message...),
	)
	conversation.pending = append([]providerToolCall(nil), calls...)
	turn := providerTurn{Calls: calls}
	if usageValue, present := root["usage"]; present {
		usage, usageErr := closedObject(
			usageValue,
			[]string{"inputTokens", "outputTokens", "totalTokens"},
			[]string{"cacheReadInputTokens", "cacheWriteInputTokens"},
		)
		input, inputOK := safeJSONInt(usage["inputTokens"])
		outputTokens, outputOK := safeJSONInt(usage["outputTokens"])
		if usageErr != nil || !inputOK || !outputOK {
			return providerTurn{}, fail("INVALID_USAGE")
		}
		turn.Usage = &Usage{InputTokens: input, OutputTokens: outputTokens}
	}
	return turn, nil
}

func (conversation *bedrockConversation) appendResults(results []providerToolResult) error {
	if conversation == nil || len(results) != len(conversation.pending) {
		return fail("CONTINUATION_INVALID")
	}
	content := make([]map[string]any, len(results))
	for index, result := range results {
		expected := conversation.pending[index]
		if result.ID != expected.ID || result.Name != expected.Name ||
			len(result.Content) > MaxToolResultBytes || !validOpaqueText(result.Content) {
			return fail("CONTINUATION_INVALID")
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
		return fail("CONTINUATION_INVALID")
	}
	conversation.messages = append(conversation.messages, message)
	conversation.pending = nil
	return nil
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
	conversation.messages = nil
	conversation.pending = nil
	conversation.tools = nil
}

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
		clearBytes(body)
		return nil, fail("PROVIDER_ERROR")
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
			return ReadinessFail, "live_probe_failed"
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
	if canonicalURI == "" {
		canonicalURI = "/"
	}
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

func hmacSHA256(key, body []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}
