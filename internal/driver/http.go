package driver

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

type HeaderCredentialResolver func(context.Context, string) ([]byte, error)
type ProfileLiveProbe func(context.Context, string, string) error

// AuthMode is the explicit per-profile authentication surface of a network
// adapter. It is closed to bearer, AWS SigV4, and none. Omission never yields
// none: a credential-less profile without an explicit mode fails admission.
type AuthMode string

const (
	AuthModeBearer   AuthMode = "bearer"
	AuthModeAWSSigV4 AuthMode = "aws_sigv4"
	AuthModeNone     AuthMode = "none"
)

func (mode AuthMode) valid() bool {
	return mode == AuthModeBearer ||
		mode == AuthModeAWSSigV4 ||
		mode == AuthModeNone
}

const (
	maxConfigVariables    = 16
	maxConfigVariableSize = 128
)

var configVariableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,31}$`)

type HTTPProfileConfig struct {
	Key              string   `json:"key"`
	ID               string   `json:"id"`
	Version          string   `json:"version"`
	Endpoint         string   `json:"endpoint"`
	CredentialHeader string   `json:"credential_header"`
	CredentialPrefix string   `json:"credential_prefix"`
	CredentialRefs   []string `json:"credential_refs"`
	ResponseBytes    int      `json:"response_bytes"`
	// ThinkingLevel is the Gemini adapter's operator-chosen thinking knob
	// (closed {LOW, MEDIUM, HIGH}; empty omits thinkingConfig). OpenAI and
	// Bedrock adapters ignore the field.
	ThinkingLevel string `json:"thinking_level,omitempty"`
	// InputTokensPerMinute is the operator's statement of the provider's
	// per-model input-token quota (the provider counts cached tokens
	// against it). When set, a conversation is paced at the wall — delayed
	// only when the sliding minute plus the next request would cross it.
	// Zero disables proactive pacing; reactive 429 pacing always applies.
	InputTokensPerMinute int64 `json:"input_tokens_per_minute,omitempty"`
	// Stream enables the Gemini adapter's streamGenerateContent mode: SSE
	// deltas render live while the reconstructed terminal response object
	// feeds the exact non-streaming accept path. OpenAI and Bedrock adapters
	// ignore the field, and an OpenAI profile's own shallower "stream" key
	// keeps binding its responses-flavour knob unchanged.
	Stream bool `json:"stream,omitempty"`
	// IncludeThoughts asks Gemini for thought content in the provider's own
	// include_thoughts vocabulary. OpenAI and Bedrock adapters ignore the
	// field. Omission changes nothing: no thinkingConfig is emitted unless a
	// thinking knob is set.
	IncludeThoughts bool `json:"include_thoughts,omitempty"`
	// BalanceProbe is the additive, operator-configured admission-time
	// balance/quota probe surface (S5-preflight-probes A3(b)). It carries
	// no in-tree consumer today: no shipped adapter configuration sets it,
	// and ProbeProviderBalance no-ops whenever it is nil, so its presence
	// changes nothing for today's fleet's canonical bytes or behavior.
	BalanceProbe *BalanceProbeConfig `json:"balance_probe,omitempty"`
}

// BalanceProbeConfig names the cheap balance/quota endpoint an
// operator-declared HTTP profile may probe at dispatch admission, distinct
// from the profile's own model endpoint. Endpoint is validated on the same
// grounds as the profile endpoint; ExhaustedField is the top-level JSON
// object field name a strict decode of the response reads as the positive
// hard-exhaustion boolean; CredentialHeader/CredentialPrefix, when set,
// carry the profile's bound credential onto the probe request under this
// probe's own header shape (which may differ from the transport's own auth
// header).
type BalanceProbeConfig struct {
	Endpoint         string `json:"endpoint"`
	CredentialHeader string `json:"credential_header,omitempty"`
	CredentialPrefix string `json:"credential_prefix,omitempty"`
	ExhaustedField   string `json:"exhausted_field"`
}

type httpTransport struct {
	config    HTTPProfileConfig
	auth      AuthMode
	client    *http.Client
	resolve   HeaderCredentialResolver
	liveProbe ProfileLiveProbe
	refs      map[string]struct{}
}

func newHTTPTransport(
	config HTTPProfileConfig,
	auth AuthMode,
	resolver HeaderCredentialResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (*httpTransport, error) {
	if !providerKeyPattern.MatchString(config.Key) ||
		!driverIdentityPattern.MatchString(config.ID) ||
		!versionPattern.MatchString(config.Version) ||
		validateEndpoint(config.Endpoint) != nil ||
		config.ResponseBytes < 1 || config.ResponseBytes > MaxProviderResponseBytes {
		return nil, fail("INVALID_ADAPTER")
	}
	switch auth {
	case AuthModeNone:
		if resolver != nil || len(config.CredentialRefs) != 0 ||
			config.CredentialHeader != "" || config.CredentialPrefix != "" {
			return nil, fail("INVALID_ADAPTER")
		}
	case AuthModeBearer:
		if resolver == nil || len(config.CredentialRefs) == 0 ||
			!httpToken(config.CredentialHeader) ||
			len(config.CredentialPrefix) > 64 {
			return nil, fail("INVALID_ADAPTER")
		}
	default:
		return nil, fail("INVALID_ADAPTER")
	}
	if config.BalanceProbe != nil {
		probe := config.BalanceProbe
		if validateEndpoint(probe.Endpoint) != nil ||
			probe.ExhaustedField == "" || len(probe.ExhaustedField) > 128 ||
			(probe.CredentialHeader != "" && !httpToken(probe.CredentialHeader)) ||
			len(probe.CredentialPrefix) > 64 {
			return nil, fail("INVALID_ADAPTER")
		}
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
	if roundTripper == nil {
		roundTripper = http.DefaultTransport.(*http.Transport).Clone()
	}
	return &httpTransport{
		config: config,
		auth:   auth,
		client: &http.Client{
			Transport: roundTripper,
			Timeout:   0,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return fail("HTTP_REDIRECT_REFUSED")
			},
		},
		resolve:   resolver,
		liveProbe: probe,
		refs:      refs,
	}, nil
}

func (transport *httpTransport) roundTrip(
	ctx context.Context,
	ref *string,
	request providerRequest,
) ([]byte, error) {
	if transport == nil {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	if transport.auth == AuthModeNone {
		if ref != nil {
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		}
	} else {
		if ref == nil {
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		}
		if _, admitted := transport.refs[*ref]; !admitted {
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		}
	}
	if request.Method != http.MethodPost ||
		validateEndpoint(request.URL) != nil ||
		!sameEndpointAuthority(transport.config.Endpoint, request.URL) ||
		request.ContentType != "application/json" ||
		len(request.Body) == 0 || len(request.Body) > MaxProviderRequestBytes {
		return nil, fail("INVALID_PROVIDER_REQUEST")
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		request.Method,
		request.URL,
		bytes.NewReader(request.Body),
	)
	if err != nil {
		return nil, fail("INVALID_PROVIDER_REQUEST")
	}
	httpRequest.Header.Set("Content-Type", request.ContentType)
	for name, value := range request.Headers {
		if !httpToken(name) || len(value) > 4_096 {
			return nil, fail("INVALID_PROVIDER_REQUEST")
		}
		httpRequest.Header.Set(name, value)
	}
	if transport.auth == AuthModeBearer {
		secret, err := transport.resolve(ctx, *ref)
		if err != nil || len(secret) == 0 || len(secret) > 65_536 {
			clearBytes(secret)
			return nil, fail("CREDENTIAL_UNAVAILABLE")
		}
		defer clearBytes(secret)
		httpRequest.Header.Set(
			transport.config.CredentialHeader,
			transport.config.CredentialPrefix+string(secret),
		)
	}
	if request.Stream {
		httpRequest.Header.Set("Accept", "text/event-stream")
	}
	response, err := transport.client.Do(httpRequest)
	if err != nil {
		if isContextError(ctx.Err()) {
			return nil, ctx.Err()
		}
		return nil, fail("PROVIDER_TRANSPORT_FAILED")
	}
	defer response.Body.Close()
	if request.Stream && response.StatusCode >= 200 && response.StatusCode <= 299 {
		if request.StreamFormat == geminiStreamFormat {
			body, streamErr := readGeminiStream(
				response.Body,
				transport.config.ResponseBytes,
				request.StreamModel,
			)
			if streamErr != nil {
				if isContextError(ctx.Err()) {
					return nil, ctx.Err()
				}
				return nil, streamErr
			}
			return body, nil
		}
		body, streamErr := readStreamedResponse(
			response.Body,
			transport.config.ResponseBytes,
		)
		if streamErr != nil {
			if isContextError(ctx.Err()) {
				return nil, ctx.Err()
			}
			return nil, streamErr
		}
		return body, nil
	}
	reader := io.LimitReader(response.Body, int64(transport.config.ResponseBytes)+1)
	body, readErr := io.ReadAll(reader)
	if readErr != nil {
		clearBytes(body)
		return nil, fail("PROVIDER_TRANSPORT_FAILED")
	}
	if len(body) > transport.config.ResponseBytes {
		clearBytes(body)
		return nil, fail("OUTPUT_OVERFLOW")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		if response.StatusCode == http.StatusTooManyRequests {
			retryAfter := response.Header.Get("Retry-After")
			message := providerErrorDetail(body)
			hard := providerLimitHard(retryAfter, body)
			delay := providerRetryDelay(retryAfter, body)
			clearBytes(body)
			return nil, &ContractError{
				Code:       "PROVIDER_LIMITED",
				Detail:     message,
				RetryAfter: delay,
				HardLimit:  hard,
			}
		}
		detail := providerErrorDetail(body)
		clearBytes(body)
		return nil, providerHTTPStatusError(response.StatusCode, detail)
	}
	return body, nil
}

// providerHTTPStatusError maps a non-2xx status to the stable provider
// status vocabulary. detail is the bounded, normalized status-envelope
// message the caller extracted ("" when none); it rides only the provider
// status codes, and the dispatcher boundary re-validates it before it can
// persist or render.
func providerHTTPStatusError(statusCode int, detail string) error {
	code := ""
	switch {
	case statusCode == http.StatusUnauthorized ||
		statusCode == http.StatusForbidden:
		code = "PROVIDER_AUTHORIZATION_FAILED"
	case statusCode == http.StatusTooManyRequests:
		code = "PROVIDER_LIMITED"
	case statusCode >= 400 && statusCode < 500:
		code = "PROVIDER_REQUEST_REJECTED"
	case statusCode >= 500 && statusCode < 600:
		code = "PROVIDER_UNAVAILABLE"
	default:
		code = "PROVIDER_ERROR"
	}
	return &ContractError{Code: code, Detail: detail}
}

func (transport *httpTransport) check(
	ctx context.Context,
	kind profileCheckKind,
	ref *string,
	model string,
) (ReadinessState, string) {
	if transport == nil {
		return ReadinessNotCertified, "credential_reference_missing"
	}
	if transport.auth == AuthModeNone {
		if ref != nil {
			return ReadinessNotCertified, "credential_reference_unknown"
		}
	} else {
		if ref == nil {
			return ReadinessNotCertified, "credential_reference_missing"
		}
		if _, ok := transport.refs[*ref]; !ok {
			return ReadinessNotCertified, "credential_reference_unknown"
		}
	}
	switch kind {
	case checkInspect:
		return ReadinessPass, "http_configuration_exact"
	case checkDoctor:
		return ReadinessPass, "http_boundary_ready"
	case checkCertify:
		if transport.liveProbe == nil {
			return ReadinessNotCertified, "live_probe_not_configured"
		}
		if err := transport.liveProbe(ctx, refValue(ref), model); err != nil {
			return ReadinessFail, certificationFailureCode(err)
		}
		return ReadinessPass, "live_probe_passed"
	default:
		return ReadinessFail, "check_kind_invalid"
	}
}

func refValue(ref *string) string {
	if ref == nil {
		return ""
	}
	return *ref
}

func validateEndpoint(value string) error {
	if len(value) == 0 || len(value) > 4_096 ||
		strings.ContainsAny(value, "\r\n\t") {
		return fail("INVALID_ENDPOINT")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Host == "" ||
		parsed.Fragment != "" {
		return fail("INVALID_ENDPOINT")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := parsed.Hostname()
	if parsed.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback()) {
		return nil
	}
	return fail("INVALID_ENDPOINT")
}

func sameEndpointAuthority(configured, requested string) bool {
	left, leftErr := url.Parse(configured)
	right, rightErr := url.Parse(requested)
	return leftErr == nil && rightErr == nil &&
		left.Scheme == right.Scheme &&
		strings.EqualFold(left.Host, right.Host)
}

func httpToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character <= 32 || character >= 127 ||
			strings.ContainsRune("()<>@,;:\\\"/[]?={}", character) {
			return false
		}
	}
	return true
}

func providerTimeout(parent context.Context, milliseconds int64) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(milliseconds)*time.Millisecond)
}

// validateConfigVariables bounds the declared endpoint-template variables of a
// driver document. Values are non-empty bounded text and never carry braces,
// so substitution is total and terminating.
func validateConfigVariables(variables map[string]string) error {
	if len(variables) > maxConfigVariables {
		return fail("RESOURCE_LIMIT")
	}
	for name, value := range variables {
		if !configVariableNamePattern.MatchString(name) ||
			value == "" || len(value) > maxConfigVariableSize ||
			strings.ContainsAny(value, "{}") ||
			containsControlCharacter(value) {
			return fail("INVALID_DRIVER_CONFIG")
		}
	}
	return nil
}

// resolveEndpointTemplate substitutes declared variables into a bounded
// endpoint template exactly once at admission. Each named placeholder appears
// at most once, every placeholder must be declared, and the result is then
// admitted on the ordinary endpoint grounds by the caller.
func resolveEndpointTemplate(
	template string,
	variables map[string]string,
) (string, error) {
	if len(template) == 0 || len(template) > 4096 ||
		strings.ContainsAny(template, "\r\n\t") {
		return "", fail("INVALID_ENDPOINT")
	}
	if !strings.Contains(template, "{") {
		return template, nil
	}
	seen := make(map[string]struct{})
	resolved := template
	for {
		start := strings.IndexByte(resolved, '{')
		if start < 0 {
			return resolved, nil
		}
		end := strings.IndexByte(resolved[start:], '}')
		if end < 0 {
			return "", fail("INVALID_ENDPOINT")
		}
		end += start
		name := resolved[start+1 : end]
		if !configVariableNamePattern.MatchString(name) {
			return "", fail("INVALID_ENDPOINT")
		}
		if _, duplicate := seen[name]; duplicate {
			return "", fail("INVALID_ENDPOINT")
		}
		seen[name] = struct{}{}
		value, declared := variables[name]
		if !declared {
			return "", fail("INVALID_ENDPOINT")
		}
		resolved = resolved[:start] + value + resolved[end+1:]
	}
}
