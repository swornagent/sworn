package driver

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type HeaderCredentialResolver func(context.Context, string) ([]byte, error)
type ProfileLiveProbe func(context.Context, string, string) error

type HTTPProfileConfig struct {
	Key              string   `json:"key"`
	ID               string   `json:"id"`
	Version          string   `json:"version"`
	Endpoint         string   `json:"endpoint"`
	CredentialHeader string   `json:"credential_header"`
	CredentialPrefix string   `json:"credential_prefix"`
	CredentialRefs   []string `json:"credential_refs"`
	ResponseBytes    int      `json:"response_bytes"`
}

type httpTransport struct {
	config    HTTPProfileConfig
	client    *http.Client
	resolve   HeaderCredentialResolver
	liveProbe ProfileLiveProbe
	refs      map[string]struct{}
}

func newHTTPTransport(
	config HTTPProfileConfig,
	resolver HeaderCredentialResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (*httpTransport, error) {
	if !providerKeyPattern.MatchString(config.Key) ||
		!driverIdentityPattern.MatchString(config.ID) ||
		!versionPattern.MatchString(config.Version) ||
		validateEndpoint(config.Endpoint) != nil ||
		!httpToken(config.CredentialHeader) ||
		len(config.CredentialPrefix) > 64 ||
		config.ResponseBytes < 1 || config.ResponseBytes > MaxProviderResponseBytes ||
		len(config.CredentialRefs) == 0 || resolver == nil {
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
	if transport == nil || ref == nil {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	if _, admitted := transport.refs[*ref]; !admitted {
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	if request.Method != http.MethodPost ||
		validateEndpoint(request.URL) != nil ||
		!sameEndpointAuthority(transport.config.Endpoint, request.URL) ||
		request.ContentType != "application/json" ||
		len(request.Body) == 0 || len(request.Body) > MaxProviderRequestBytes {
		return nil, fail("INVALID_PROVIDER_REQUEST")
	}
	secret, err := transport.resolve(ctx, *ref)
	if err != nil || len(secret) == 0 || len(secret) > 65_536 {
		clearBytes(secret)
		return nil, fail("CREDENTIAL_UNAVAILABLE")
	}
	defer clearBytes(secret)
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
	httpRequest.Header.Set(
		transport.config.CredentialHeader,
		transport.config.CredentialPrefix+string(secret),
	)
	response, err := transport.client.Do(httpRequest)
	if err != nil {
		if isContextError(ctx.Err()) {
			return nil, ctx.Err()
		}
		return nil, fail("PROVIDER_TRANSPORT_FAILED")
	}
	defer response.Body.Close()
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
	if transport == nil || ref == nil {
		return ReadinessNotCertified, "credential_reference_missing"
	}
	if _, ok := transport.refs[*ref]; !ok {
		return ReadinessNotCertified, "credential_reference_unknown"
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
		if err := transport.liveProbe(ctx, *ref, model); err != nil {
			return ReadinessFail, certificationFailureCode(err)
		}
		return ReadinessPass, "live_probe_passed"
	default:
		return ReadinessFail, "check_kind_invalid"
	}
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
