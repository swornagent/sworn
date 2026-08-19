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
			delay := providerRetryDelay(response.Header.Get("Retry-After"), body)
			clearBytes(body)
			return nil, &ContractError{Code: "PROVIDER_LIMITED", RetryAfter: delay}
		}
		clearBytes(body)
		return nil, providerHTTPStatusError(response.StatusCode)
	}
	return body, nil
}

func providerHTTPStatusError(statusCode int) error {
	switch {
	case statusCode == http.StatusUnauthorized ||
		statusCode == http.StatusForbidden:
		return fail("PROVIDER_AUTHORIZATION_FAILED")
	case statusCode == http.StatusTooManyRequests:
		return fail("PROVIDER_LIMITED")
	case statusCode >= 400 && statusCode < 500:
		return fail("PROVIDER_REQUEST_REJECTED")
	case statusCode >= 500 && statusCode < 600:
		return fail("PROVIDER_UNAVAILABLE")
	default:
		return fail("PROVIDER_ERROR")
	}
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
