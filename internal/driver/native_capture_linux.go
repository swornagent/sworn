//go:build linux

package driver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type nativeProviderEvidence struct {
	RequestDigest string
	ToolDigest    string
}

// nativeProviderCapture is an adapter-owned, loopback-only fake provider. It
// records the actual first request emitted by the pinned CLI; callers cannot
// choose its endpoint, token, response, or evidence.
type nativeProviderCapture struct {
	family           ProfileFamily
	model            string
	access           WorkspaceAccess
	address          string
	prefix           string
	token            []byte
	brokerCapability []byte
	listener         net.Listener
	server           *http.Server
	captured         chan nativeProviderEvidence
	once             sync.Once
}

func newNativeProviderCapture(
	family ProfileFamily,
	model string,
	access WorkspaceAccess,
) (*nativeProviderCapture, error) {
	if (family != ProfileCodex && family != ProfileClaude) ||
		validateText(model, 500, false) != nil ||
		(access != ReadOnly && access != ReadWrite) {
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, fail("ENDPOINT_UNAVAILABLE")
	}
	prefix := base64.RawURLEncoding.EncodeToString(random[:16])
	token := []byte(base64.RawURLEncoding.EncodeToString(random[16:]))
	clearBytes(random)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		clearBytes(token)
		return nil, fail("ENDPOINT_UNAVAILABLE")
	}
	capture := &nativeProviderCapture{
		family: family, model: model, access: access,
		address: listener.Addr().String(), prefix: prefix,
		token: token, listener: listener,
		captured: make(chan nativeProviderEvidence, 1),
	}
	capture.server = &http.Server{
		Handler:           capture,
		ReadHeaderTimeout: 2 * time.Second,
		IdleTimeout:       2 * time.Second,
		MaxHeaderBytes:    16_384,
	}
	go func() { _ = capture.server.Serve(listener) }()
	return capture, nil
}

func (capture *nativeProviderCapture) BaseURL() string {
	if capture.family == ProfileCodex {
		return "http://" + capture.address + "/" + capture.prefix + "/v1"
	}
	return "http://" + capture.address + "/" + capture.prefix
}

func (capture *nativeProviderCapture) endpointDigest() string {
	return Digest([]byte(capture.BaseURL()))
}

func (capture *nativeProviderCapture) bearer() []byte {
	return append([]byte(nil), capture.token...)
}

func (capture *nativeProviderCapture) bindBrokerCapability(capability []byte) {
	capture.brokerCapability = append(
		capture.brokerCapability[:0],
		capability...,
	)
}

func (capture *nativeProviderCapture) expectedPath() string {
	if capture.family == ProfileCodex {
		return "/" + capture.prefix + "/v1/responses"
	}
	return "/" + capture.prefix + "/v1/messages"
}

func (capture *nativeProviderCapture) Captured() <-chan nativeProviderEvidence {
	return capture.captured
}

func (capture *nativeProviderCapture) Close() {
	if capture == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = capture.server.Shutdown(ctx)
	_ = capture.listener.Close()
	clearBytes(capture.token)
	clearBytes(capture.brokerCapability)
	capture.token = nil
	capture.brokerCapability = nil
}

func (capture *nativeProviderCapture) ServeHTTP(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodPost ||
		request.Host != capture.address ||
		request.URL.Path != capture.expectedPath() ||
		request.URL.RawQuery != capture.expectedQuery() {
		writeNativeCaptureError(writer, capture.family, http.StatusBadRequest)
		return
	}
	if !capture.authorized(request) {
		writeNativeCaptureError(writer, capture.family, http.StatusBadRequest)
		return
	}
	first := false
	capture.once.Do(func() { first = true })
	if !first {
		writeNativeCaptureError(writer, capture.family, http.StatusConflict)
		return
	}
	if !strings.HasPrefix(
		strings.ToLower(request.Header.Get("Content-Type")),
		"application/json",
	) {
		writeNativeCaptureError(writer, capture.family, http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(
		request.Body,
		MaxProviderRequestBytes+1,
	))
	if err != nil || len(body) == 0 || len(body) > MaxProviderRequestBytes {
		clearBytes(body)
		writeNativeCaptureError(writer, capture.family, http.StatusBadRequest)
		return
	}
	defer clearBytes(body)
	if containsCapability(body, capture.token) ||
		containsCapability(body, capture.brokerCapability) {
		writeNativeCaptureError(writer, capture.family, http.StatusBadRequest)
		return
	}
	toolDigest, err := validateNativeProviderRequest(
		body,
		capture.family,
		capture.model,
		capture.access,
	)
	if err != nil {
		writeNativeCaptureError(writer, capture.family, http.StatusBadRequest)
		return
	}
	evidence := nativeProviderEvidence{
		RequestDigest: Digest(body),
		ToolDigest:    toolDigest,
	}
	capture.captured <- evidence
	writeNativeCaptureError(writer, capture.family, http.StatusServiceUnavailable)
}

func (capture *nativeProviderCapture) expectedQuery() string {
	if capture.family == ProfileClaude {
		return "beta=true"
	}
	return ""
}

func (capture *nativeProviderCapture) authorized(request *http.Request) bool {
	expected := string(capture.token)
	authorization := request.Header.Get("Authorization")
	switch capture.family {
	case ProfileCodex:
		return authorization == "Bearer "+expected
	case ProfileClaude:
		return authorization == "Bearer "+expected ||
			request.Header.Get("X-Api-Key") == expected
	default:
		return false
	}
}

func writeNativeCaptureError(
	writer http.ResponseWriter,
	family ProfileFamily,
	status int,
) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	if family == ProfileClaude {
		_, _ = writer.Write([]byte(
			`{"type":"error","error":{"type":"overloaded_error","message":"capture complete"}}`,
		))
		return
	}
	_, _ = writer.Write([]byte(
		`{"error":{"message":"capture complete","type":"server_error","code":"capture_complete"}}`,
	))
}

func validateNativeProviderRequest(
	body []byte,
	family ProfileFamily,
	model string,
	access WorkspaceAccess,
) (string, error) {
	value, err := decodeStrict(body, MaxProviderRequestBytes)
	if err != nil {
		return "", fail("NATIVE_SURFACE_INVALID")
	}
	root, ok := value.(map[string]any)
	if !ok || root["model"] != model {
		return "", fail("NATIVE_SURFACE_INVALID")
	}
	rawTools, ok := root["tools"].([]any)
	if !ok {
		return "", fail("NATIVE_SURFACE_INVALID")
	}
	definitions := toolDefinitions(access)
	switch family {
	case ProfileCodex:
		if root["tool_choice"] != "auto" ||
			root["parallel_tool_calls"] != false ||
			!exactCodexProviderTools(rawTools, definitions) {
			return "", fail("NATIVE_SURFACE_INVALID")
		}
	case ProfileClaude:
		if !exactClaudeProviderTools(rawTools, definitions) {
			return "", fail("NATIVE_SURFACE_INVALID")
		}
	default:
		return "", fail("NATIVE_SURFACE_INVALID")
	}
	return nativeToolSurfaceDigest(access), nil
}

func exactClaudeProviderTools(
	rawTools []any,
	definitions []providerToolDefinition,
) bool {
	if len(rawTools) != len(definitions)+1 {
		return false
	}
	expected := make(map[string]providerToolDefinition, len(definitions)+1)
	for _, definition := range definitions {
		expected["mcp__sworn__"+definition.Name] = definition
	}
	expected["StructuredOutput"] = providerToolDefinition{
		Name:        "StructuredOutput",
		Description: "Use this tool to return your final response in the requested structured format. You MUST call this tool exactly once at the end of your response to provide the structured output.",
		InputSchema: json.RawMessage(nativeOutputSchemaJSON),
	}
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok ||
			!closedNativeProviderTool(
				tool,
				[]string{"name", "description", "input_schema"},
				nil,
			) {
			return false
		}
		name, nameOK := tool["name"].(string)
		definition, present := expected[name]
		if !nameOK || !present ||
			tool["description"] != definition.Description ||
			!nativeProviderSchemaMatches(
				tool["input_schema"],
				definition.InputSchema,
				false,
			) {
			return false
		}
		delete(expected, name)
	}
	return len(expected) == 0
}

func exactCodexProviderTools(
	rawTools []any,
	definitions []providerToolDefinition,
) bool {
	inert := codexInertProviderTools()
	if len(rawTools) != len(inert)+1 {
		return false
	}
	namespaceSeen := false
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		switch tool["type"] {
		case "function":
			if !closedNativeProviderTool(
				tool,
				[]string{
					"type", "name", "description", "strict", "parameters",
				},
				nil,
			) ||
				tool["strict"] != false {
				return false
			}
			name, nameOK := tool["name"].(string)
			definition, present := inert[name]
			if !nameOK || !present ||
				tool["description"] != definition.Description ||
				!nativeProviderSchemaMatches(
					tool["parameters"],
					definition.InputSchema,
					false,
				) {
				return false
			}
			delete(inert, name)
		case "namespace":
			if namespaceSeen ||
				!closedNativeProviderTool(
					tool,
					[]string{"type", "name", "description", "tools"},
					nil,
				) ||
				tool["name"] != "mcp__sworn" ||
				tool["description"] != "Tools in the mcp__sworn namespace." ||
				!exactCodexNamespaceTools(tool["tools"], definitions) {
				return false
			}
			namespaceSeen = true
		default:
			return false
		}
	}
	return namespaceSeen && len(inert) == 0
}

func exactCodexNamespaceTools(
	value any,
	definitions []providerToolDefinition,
) bool {
	rawTools, ok := value.([]any)
	if !ok || len(rawTools) != len(definitions) {
		return false
	}
	expected := make(map[string]providerToolDefinition, len(definitions))
	for _, definition := range definitions {
		expected[definition.Name] = definition
	}
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok ||
			!closedNativeProviderTool(
				tool,
				[]string{
					"type", "name", "description", "strict", "parameters",
				},
				nil,
			) ||
			tool["type"] != "function" ||
			tool["strict"] != false {
			return false
		}
		name, nameOK := tool["name"].(string)
		definition, present := expected[name]
		if !nameOK || !present ||
			tool["description"] != definition.Description ||
			!nativeProviderSchemaMatches(
				tool["parameters"],
				definition.InputSchema,
				true,
			) {
			return false
		}
		delete(expected, name)
	}
	return len(expected) == 0
}

func nativeProviderSchemaMatches(
	actual any,
	expectedJSON json.RawMessage,
	codexNormalize bool,
) bool {
	expected, err := decodeStrict(expectedJSON, MaxToolArgumentBytes)
	if err != nil {
		return false
	}
	if codexNormalize {
		normalizeCodexProviderSchema(expected)
	}
	actualBody, actualErr := canonicalJSON(actual)
	expectedBody, expectedErr := canonicalJSON(expected)
	matches := actualErr == nil && expectedErr == nil &&
		bytes.Equal(actualBody, expectedBody)
	clearBytes(actualBody)
	clearBytes(expectedBody)
	return matches
}

func normalizeCodexProviderSchema(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if typed["type"] == "object" {
			if _, present := typed["properties"]; !present {
				typed["properties"] = map[string]any{}
			}
		}
		for _, child := range typed {
			normalizeCodexProviderSchema(child)
		}
	case []any:
		for _, child := range typed {
			normalizeCodexProviderSchema(child)
		}
	}
}

func codexInertProviderTools() map[string]providerToolDefinition {
	return map[string]providerToolDefinition{
		"list_mcp_resources": {
			Name:        "list_mcp_resources",
			Description: "Lists resources provided by MCP servers. Resources allow servers to share data that provides context to language models, such as files, database schemas, or application-specific information. Prefer resources over web search when possible.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"cursor":{"type":"string","description":"Opaque cursor from a previous list_mcp_resources call; omit for the first page."},"server":{"type":"string","description":"MCP server name. Omit to list resources from every configured server."}},"additionalProperties":false}`),
		},
		"list_mcp_resource_templates": {
			Name:        "list_mcp_resource_templates",
			Description: "Lists resource templates provided by MCP servers. Parameterized resource templates allow servers to share data that takes parameters and provides context to language models, such as files, database schemas, or application-specific information. Prefer resource templates over web search when possible.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"cursor":{"type":"string","description":"Opaque cursor from a previous list_mcp_resource_templates call; omit for the first page."},"server":{"type":"string","description":"MCP server name. Omit to list resource templates from every configured server."}},"additionalProperties":false}`),
		},
		"read_mcp_resource": {
			Name:        "read_mcp_resource",
			Description: "Read a specific resource from an MCP server given the server name and resource URI.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"server":{"type":"string","description":"MCP server name exactly as configured. Must match the 'server' field returned by list_mcp_resources."},"uri":{"type":"string","description":"Resource URI to read. Must be one of the URIs returned by list_mcp_resources."}},"required":["server","uri"],"additionalProperties":false}`),
		},
		"update_plan": {
			Name:        "update_plan",
			Description: "Updates the task plan.\nProvide an optional explanation and a list of plan items, each with a step and status.\nAt most one step can be in_progress at a time.\n",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"explanation":{"type":"string","description":"Optional explanation for this plan update."},"plan":{"type":"array","description":"The list of steps","items":{"type":"object","properties":{"status":{"type":"string","description":"Step status.","enum":["pending","in_progress","completed"]},"step":{"type":"string","description":"Task step text."}},"required":["step","status"],"additionalProperties":false}}},"required":["plan"],"additionalProperties":false}`),
		},
	}
}

func closedNativeProviderTool(
	tool map[string]any,
	required []string,
	optional []string,
) bool {
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, name := range required {
		allowed[name] = struct{}{}
		if _, present := tool[name]; !present {
			return false
		}
	}
	for _, name := range optional {
		allowed[name] = struct{}{}
	}
	for name := range tool {
		if _, present := allowed[name]; !present {
			return false
		}
	}
	return true
}

func nativeProfileDigest(profile ProfileConfig) string {
	body, err := canonicalJSON(profile)
	if err != nil {
		return ""
	}
	return Digest(body)
}

func validateNativeSurfaceCertificate(
	certificate nativeSurfaceCertificate,
	invocation Invocation,
	config NativeAdapterConfig,
) error {
	if certificate.Family != config.Family ||
		certificate.ProfileDigest != nativeProfileDigest(
			invocation.Selected.Profile,
		) ||
		certificate.Model != invocation.Selected.Model ||
		certificate.AdapterConfigDigest !=
			invocation.Selected.Adapter.ConfigurationDigest ||
		certificate.ExecutableDigest != config.CLI.Digest ||
		certificate.CLIVersion != config.CLIVersion ||
		certificate.ToolDigest != nativeToolSurfaceDigest(ReadWrite) ||
		!digestPattern.MatchString(certificate.CaptureEvidenceDigest) ||
		certificate.Protocol == "" ||
		certificate.ClientName == "" ||
		certificate.ClientVersion == "" ||
		!digestPattern.MatchString(certificate.InitializeDigest) ||
		!digestPattern.MatchString(certificate.NotificationDigest) ||
		!digestPattern.MatchString(certificate.ListDigest) {
		return fail("NATIVE_NOT_CERTIFIED")
	}
	return nil
}

func nativeCaptureCredentialBody(
	capture *nativeProviderCapture,
) ([]byte, error) {
	if capture == nil {
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	token := string(capture.token)
	switch capture.family {
	case ProfileCodex:
		return []byte(`{}`), nil
	case ProfileClaude:
		return json.Marshal(map[string]any{
			"claudeAiOauth": map[string]any{
				"accessToken":      token,
				"refreshToken":     token,
				"expiresAt":        int64(8_000_000_000_000_000),
				"scopes":           []string{"user:inference"},
				"subscriptionType": "max",
			},
		})
	default:
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
}
