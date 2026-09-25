package driver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// laneProbeBound is the probe's own bound, distinct from any invocation
// timeout: long enough to observe a provider's own admission timeout
// (~30s class) rather than cutting first, and equal to MaxProviderRetryDelay
// and the dispatch wall-clock class.
const laneProbeBound = 120 * time.Second

// laneProbeMaxOutputTokens is the declared output ceiling of the probe's
// one minimal request: small enough to bound cost, large enough that a
// provider never refuses the value itself.
const laneProbeMaxOutputTokens = 16

// laneProbePrompt is the probe's fixed literal request text. It carries no
// repository content.
const laneProbePrompt = "sworn lane probe"

const (
	laneProbeCodeLivePassed        = "live_probe_passed"
	laneProbeCodeFakeNotProduction = "fake_not_production"
	laneProbeCodeNotProbeable      = "profile_not_probeable"
)

// LaneProbeResult is the closed, secret-free outcome of one lane probe.
// Message and RequestID are the provider's own bounded words, already
// normalized to single-line, control-free text by the same extraction the
// dispatch boundary uses; a provider refusal never reaches Go's error
// return, only this typed field.
type LaneProbeResult struct {
	Profile             string         `json:"profile"`
	Model               string         `json:"model"`
	Family              ProfileFamily  `json:"family"`
	Surface             ProfileSurface `json:"surface,omitempty"`
	AdapterID           string         `json:"adapter_id"`
	AdapterVersion      string         `json:"adapter_version"`
	ConfigurationDigest string         `json:"configuration_digest"`
	Ready               bool           `json:"ready"`
	Code                string         `json:"code"`
	Message             string         `json:"message,omitempty"`
	RequestID           string         `json:"request_id,omitempty"`
	LatencyMillis       int64          `json:"latency_ms"`
}

// ProbeLane sends one minimal, bounded live request to an explicitly named
// configured profile and model, proving the lane is admitting requests
// right now. It runs no agent loop, submits no repository byte, and never
// runs a submission-contract check: it proves admission, not the
// conversation contract that certify proves.
//
// It is exposed as one function so the S5 provider-stall backoff and a
// Manager seat issue the identical request. There is no fallback: an
// unknown profile or an invalid model fails the Go error return, never
// substitutes another lane. A provider refusal is never a Go error: it
// rides the typed, closed Code field of the returned result so a caller can
// wait on it or report it without treating "the provider said no" the same
// as "the probe request was malformed".
func ProbeLane(
	ctx context.Context,
	registry ConfiguredDriverRegistry,
	profile string,
	model string,
) (LaneProbeResult, error) {
	if ctx == nil {
		return LaneProbeResult{}, fail("INVALID_CHECK_REQUEST")
	}
	selected, err := registry.ResolveSelection(
		ModelSelection{Profile: profile, Model: model},
	)
	if err != nil {
		return LaneProbeResult{}, err
	}
	result := LaneProbeResult{
		Profile:             profile,
		Model:               model,
		AdapterID:           selected.Adapter.ID,
		AdapterVersion:      selected.Adapter.Version,
		ConfigurationDigest: selected.Adapter.ConfigurationDigest,
	}
	probeCtx, cancel := context.WithTimeout(ctx, laneProbeBound)
	defer cancel()
	start := time.Now()
	switch adapter := selected.adapter.(type) {
	case *loopAdapter:
		result.Family = adapter.family
		result.Surface = adapter.surface
		result.Ready, result.Code, result.Message, result.RequestID =
			probeLoopAdapter(probeCtx, adapter, selected.Profile.CredentialRef, model)
	case *nativeAdapter:
		result.Family = adapter.profileFamily()
		result.Ready, result.Code =
			probeNativeAdapter(probeCtx, adapter, selected.Profile.CredentialRef)
	case *ProcessAdapter:
		result.Family = ProfileFake
		result.Ready, result.Code = false, laneProbeCodeFakeNotProduction
	default:
		result.Ready, result.Code = false, laneProbeCodeNotProbeable
	}
	result.LatencyMillis = time.Since(start).Milliseconds()
	return result, nil
}

// probeLoopAdapter drives an HTTP or Bedrock lane through the exact
// transport dispatch uses (httpTransport.roundTrip or
// bedrockTransport.roundTrip): the same endpoint-authority, credential, and
// redirect-refusal checks apply, and only a live 2xx response is ready. It
// issues exactly one round trip: no pacing, no retry.
func probeLoopAdapter(
	ctx context.Context,
	adapter *loopAdapter,
	ref *string,
	model string,
) (ready bool, code string, message string, requestID string) {
	if adapter == nil {
		return false, laneProbeCodeNotProbeable, "", ""
	}
	endpoint, err := laneProbeEndpoint(adapter.transport)
	if err != nil {
		return false, certificationFailureCode(err), "", ""
	}
	thinkingLevel, includeThoughts := laneProbeGeminiConfig(adapter.transport)
	request, err := buildLaneProbeRequest(
		adapter.dialect, endpoint, model,
		adapter.reasoningEffort, thinkingLevel, includeThoughts,
	)
	if err != nil {
		return false, certificationFailureCode(err), "", ""
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, certificationFailureCode(ctxErr), "", ""
	}
	captureCtx, headerRequestID := laneProbeCaptureRequestID(ctx)
	body, roundTripErr := adapter.transport.roundTrip(captureCtx, ref, request)
	if roundTripErr != nil {
		var contractErr *ContractError
		if errors.As(roundTripErr, &contractErr) {
			return false, certificationFailureCode(roundTripErr),
				contractErr.Detail, contractErr.RequestID
		}
		return false, certificationFailureCode(roundTripErr), "", ""
	}
	requestID = *headerRequestID
	if requestID == "" {
		requestID = laneProbeRequestIDFromBody(body)
	}
	clearBytes(body)
	return true, laneProbeCodeLivePassed, "", requestID
}

// probeNativeAdapter reuses the exact bounded, read-only checks
// certification's honest native reporting uses (nativeVersion identity plus
// nativeCredentialLivenessCheck), with one deliberate inversion: where
// dispatch admission and certify treat an unevaluated credential as
// "unproven, not refused", the probe reports it as not ready. A probe that
// cannot positively prove the lane live must say so, not shrug and pass.
func probeNativeAdapter(
	ctx context.Context,
	adapter *nativeAdapter,
	ref *string,
) (bool, string) {
	if adapter == nil || ref == nil {
		return false, "credential_reference_missing"
	}
	if _, admitted := adapter.refs[*ref]; !admitted {
		return false, "credential_reference_unknown"
	}
	body, versionErr := nativeVersion(ctx, adapter.config)
	exact := versionErr == nil && string(body) == adapter.config.VersionOutput+"\n"
	clearBytes(body)
	if ctx.Err() != nil {
		return false, "certification_timeout"
	}
	if !exact {
		return false, "native_version_changed"
	}
	pathValue, resolveErr := adapter.resolve(ctx, *ref)
	if resolveErr != nil {
		return false, "native_credential_preflight_unevaluated"
	}
	stale, evaluated := nativeCredentialLivenessCheck(
		adapter.config.Family, pathValue, adapter.config.MaxCredentialBytes,
	)
	if stale {
		return false, "native_credential_stale"
	}
	if !evaluated {
		return false, "native_credential_preflight_unevaluated"
	}
	return true, laneProbeCodeLivePassed
}

func laneProbeEndpoint(transport providerTransport) (string, error) {
	switch typed := transport.(type) {
	case *httpTransport:
		return typed.config.Endpoint, nil
	case *bedrockTransport:
		return typed.config.Endpoint, nil
	default:
		return "", fail("INVALID_ADAPTER")
	}
}

// laneProbeGeminiConfig reads the Gemini thinking knobs straight from the
// transport's own admitted configuration; every other dialect's transport
// carries no such knob and returns the honest zero value.
func laneProbeGeminiConfig(transport providerTransport) (thinkingLevel string, includeThoughts bool) {
	if typed, ok := transport.(*httpTransport); ok {
		return typed.config.ThinkingLevel, typed.config.IncludeThoughts
	}
	return "", false
}

// buildLaneProbeRequest renders the probe's one minimal request per wire
// dialect: no tools, no tool_choice, the fixed literal prompt, and the
// declared output ceiling under each dialect's own existing field name.
func buildLaneProbeRequest(
	dialect providerDialect,
	endpoint string,
	model string,
	reasoningEffort string,
	thinkingLevel string,
	includeThoughts bool,
) (providerRequest, error) {
	switch dialect {
	case providerDialectOpenAIResponses, providerDialectXAIResponses:
		return buildResponsesProbeRequest(endpoint, model, reasoningEffort)
	case providerDialectGemini:
		return buildGeminiProbeRequest(endpoint, model, thinkingLevel, includeThoughts)
	case providerDialectBedrockConverse:
		return buildBedrockProbeRequest(endpoint, model)
	case providerDialectOpenAIChat, providerDialectOpenRouterChat,
		providerDialectOpaqueChat, providerDialectGoogleChat,
		providerDialectXAIChat:
		return buildChatProbeRequest(endpoint, model, reasoningEffort, dialect)
	default:
		return providerRequest{}, fail("INVALID_ADAPTER")
	}
}

type laneProbeResponsesInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type laneProbeResponsesReasoning struct {
	Effort string `json:"effort"`
}

func buildResponsesProbeRequest(
	endpoint, model, reasoningEffort string,
) (providerRequest, error) {
	if validateEndpoint(endpoint) != nil || validateText(model, 500, false) != nil {
		return providerRequest{}, fail("INVALID_ADAPTER")
	}
	input := []laneProbeResponsesInputMessage{
		{Role: "user", Content: laneProbePrompt},
	}
	reasoning := laneProbeResponsesReasoning{Effort: reasoningEffort}
	payload := struct {
		Model           string                           `json:"model"`
		Input           []laneProbeResponsesInputMessage `json:"input"`
		Reasoning       laneProbeResponsesReasoning      `json:"reasoning"`
		Store           bool                             `json:"store"`
		Stream          bool                             `json:"stream"`
		MaxOutputTokens int64                            `json:"max_output_tokens"`
	}{
		Model:           model,
		Input:           input,
		Reasoning:       reasoning,
		Store:           false,
		Stream:          false,
		MaxOutputTokens: laneProbeMaxOutputTokens,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	return providerRequest{
		Method: "POST", URL: endpoint, ContentType: "application/json", Body: body,
	}, nil
}

type laneProbeChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func buildChatProbeRequest(
	endpoint, model, reasoningEffort string,
	dialect providerDialect,
) (providerRequest, error) {
	if validateEndpoint(endpoint) != nil || validateText(model, 500, false) != nil {
		return providerRequest{}, fail("INVALID_ADAPTER")
	}
	messages := []laneProbeChatMessage{{Role: "user", Content: laneProbePrompt}}
	payload := struct {
		Model               string                 `json:"model"`
		Messages            []laneProbeChatMessage `json:"messages"`
		Effort              string                 `json:"reasoning_effort,omitempty"`
		MaxCompletionTokens int64                  `json:"max_completion_tokens,omitempty"`
	}{
		Model:    model,
		Messages: messages,
		Effort:   reasoningEffort,
	}
	// The xAI chat surface stays deliberately unwired for this field: no
	// recorded fixture carries it for that dialect, and the dispatch
	// conversation never emits it there either (openai.go request()).
	if dialect != providerDialectXAIChat {
		payload.MaxCompletionTokens = laneProbeMaxOutputTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	return providerRequest{
		Method: "POST", URL: endpoint, ContentType: "application/json", Body: body,
	}, nil
}

func buildGeminiProbeRequest(
	baseURL, model, thinkingLevel string,
	includeThoughts bool,
) (providerRequest, error) {
	if validateEndpoint(baseURL) != nil || validateText(model, 500, false) != nil {
		return providerRequest{}, fail("INVALID_ADAPTER")
	}
	prompt := laneProbePrompt
	generation := geminiGenerationConfig{MaxOutputTokens: laneProbeMaxOutputTokens}
	if thinkingLevel != "" || includeThoughts {
		thinking := &geminiThinkingConfig{}
		if thinkingLevel != "" {
			thinking.ThinkingLevel = thinkingLevel
		}
		if includeThoughts {
			thinking.IncludeThoughts = true
		}
		generation.ThinkingConfig = thinking
	}
	body, err := json.Marshal(struct {
		Contents         []geminiContent        `json:"contents"`
		GenerationConfig geminiGenerationConfig `json:"generationConfig"`
	}{
		Contents: []geminiContent{{
			Role:  "user",
			Parts: []geminiPart{{Text: &prompt}},
		}},
		GenerationConfig: generation,
	})
	if err != nil {
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/v1beta/models/" + url.PathEscape(model) + ":generateContent"
	return providerRequest{
		Method: "POST", URL: endpoint, ContentType: "application/json", Body: body,
	}, nil
}

type laneProbeBedrockInferenceConfig struct {
	MaxTokens int64 `json:"maxTokens,omitempty"`
}

func buildBedrockProbeRequest(endpoint, model string) (providerRequest, error) {
	if validateEndpoint(endpoint) != nil || validateText(model, 500, false) != nil {
		return providerRequest{}, fail("INVALID_ADAPTER")
	}
	messages := []map[string]any{{
		"role":    "user",
		"content": []map[string]string{{"text": laneProbePrompt}},
	}}
	inference := laneProbeBedrockInferenceConfig{MaxTokens: laneProbeMaxOutputTokens}
	body, err := json.Marshal(struct {
		Messages        []map[string]any                `json:"messages"`
		InferenceConfig laneProbeBedrockInferenceConfig `json:"inferenceConfig"`
	}{
		Messages:        messages,
		InferenceConfig: inference,
	})
	if err != nil {
		return providerRequest{}, fail("RESOURCE_LIMIT")
	}
	probeURL := strings.TrimSuffix(endpoint, "/") +
		"/model/" + url.PathEscape(model) + "/converse"
	return providerRequest{
		Method: "POST", URL: probeURL, ContentType: "application/json", Body: body,
	}, nil
}

// laneProbeRequestIDHeaders is the closed, case-insensitive allowlist of
// response headers a provider's request id may ride on. http.Header.Get is
// itself case-insensitive, so listing the canonical spelling is enough.
var laneProbeRequestIDHeaders = []string{
	"X-Request-Id", "Request-Id", "X-Amzn-Requestid", "X-Amz-Request-Id",
}

const maxLaneProbeRequestIDBytes = 128

// laneProbeRequestIDCaptureKey lets the probe read the header-based request
// id off a live 2xx transport response without adding a return value every
// other roundTrip caller (dispatch, doctor, certify) would have to plumb:
// roundTrip writes to the pointer only when this key is present in ctx.
type laneProbeRequestIDCaptureKey struct{}

func laneProbeCaptureRequestID(ctx context.Context) (context.Context, *string) {
	captured := new(string)
	return context.WithValue(ctx, laneProbeRequestIDCaptureKey{}, captured), captured
}

func laneProbeRequestIDCapture(ctx context.Context) *string {
	captured, _ := ctx.Value(laneProbeRequestIDCaptureKey{}).(*string)
	return captured
}

func laneProbeRequestIDFromHeader(header http.Header) string {
	if header == nil {
		return ""
	}
	for _, name := range laneProbeRequestIDHeaders {
		if id := normalizeLaneProbeRequestID(header.Get(name)); id != "" {
			return id
		}
	}
	return ""
}

func laneProbeRequestIDFromBody(body []byte) string {
	var envelope struct {
		ID         string `json:"id"`
		ResponseID string `json:"responseId"`
		RequestID  string `json:"requestId"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	for _, candidate := range []string{envelope.ID, envelope.ResponseID, envelope.RequestID} {
		if id := normalizeLaneProbeRequestID(candidate); id != "" {
			return id
		}
	}
	return ""
}

// normalizeLaneProbeRequestID admits a bounded, single-line, control-free
// request id. Any control byte disqualifies the whole candidate rather than
// being stripped from it: a real request id never carries one, so a value
// that does is treated as honest absence, not sanitized into something the
// provider never sent.
func normalizeLaneProbeRequestID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !utf8.ValidString(trimmed) {
		return ""
	}
	for _, r := range trimmed {
		if r <= 0x1f || (r >= 0x7f && r <= 0x9f) {
			return ""
		}
	}
	if len(trimmed) <= maxLaneProbeRequestIDBytes {
		return trimmed
	}
	bounded := trimmed[:maxLaneProbeRequestIDBytes]
	for len(bounded) > 0 && !utf8.ValidString(bounded) {
		bounded = bounded[:len(bounded)-1]
	}
	return bounded
}
