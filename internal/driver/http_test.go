package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemFileCredentialTrimsLineEndingsAndRefusesMalformed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(name string, body []byte) string {
		t.Helper()
		pathValue := filepath.Join(root, name)
		if err := os.WriteFile(pathValue, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return pathValue
	}
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{"exact", "token-value", "token-value"},
		{"trailing-lf", "token-value\n", "token-value"},
		{"trailing-crlf", "token-value\r\n", "token-value"},
		{"trailing-many", "token-value\n\r\n\n", "token-value"},
		{"interior-space", "token value\n", "token value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			secret, err := systemFileCredential(
				context.Background(),
				write(test.name, []byte(test.body)),
			)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			defer clearBytes(secret)
			if string(secret) != test.want {
				t.Fatalf("secret length = %d, want %d", len(secret), len(test.want))
			}
		})
	}
	for _, test := range []struct {
		name string
		body string
	}{
		{"only-line-endings", "\r\n\n"},
		{"interior-lf", "token\nvalue\n"},
		{"interior-cr", "token\rvalue"},
		{"leading-lf", "\ntoken-value"},
		{"tab", "token\tvalue"},
		{"nul", "token\x00value"},
		{"delete", "token\x7fvalue"},
	} {
		t.Run(test.name, func(t *testing.T) {
			secret, err := systemFileCredential(
				context.Background(),
				write(test.name, []byte(test.body)),
			)
			if !IsCode(err, "CREDENTIAL_MALFORMED") || secret != nil {
				t.Fatalf("error = %v, secret returned = %t", err, secret != nil)
			}
		})
	}
	// An absent file stays unavailable, not malformed.
	if _, err := systemFileCredential(
		context.Background(),
		filepath.Join(root, "absent"),
	); !IsCode(err, "CREDENTIAL_UNAVAILABLE") {
		t.Fatalf("absent file error = %v", err)
	}
}

func TestCredentialMalformedClassifiesAsCredentialFailure(t *testing.T) {
	t.Parallel()
	if kind := classifyKind("CREDENTIAL_MALFORMED", false); kind != KindAuthorization {
		t.Fatalf("kind = %q, want %q", kind, KindAuthorization)
	}
	if code := certificationFailureCode(fail("CREDENTIAL_MALFORMED")); code != "certification_credential_failed" {
		t.Fatalf("certification code = %q", code)
	}
	var contractErr *ContractError
	if err := normalizeAdapterError(fail("CREDENTIAL_MALFORMED")); !errors.As(err, &contractErr) ||
		contractErr.Code != "CREDENTIAL_MALFORMED" ||
		contractErr.Kind != KindAuthorization {
		t.Fatalf("normalized = %v", err)
	}
}

func TestHTTPTransportRefusesMalformedCredentialBeforeDial(t *testing.T) {
	t.Parallel()
	for _, secret := range []string{"token\n", "to\r\nken", "token\x00"} {
		config := HTTPProfileConfig{
			Key: "malformed-credential-test", ID: "sworn.malformed-credential-test",
			Version: "1.0.0", Endpoint: "https://provider.test/v1/chat",
			CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
			CredentialRefs: []string{"cred"},
			ResponseBytes:  MaxProviderResponseBytes,
		}
		transport, err := newHTTPTransport(
			config,
			AuthModeBearer,
			func(context.Context, string) ([]byte, error) {
				return []byte(secret), nil
			},
			nil,
			roundTripperFunc(func(*http.Request) (*http.Response, error) {
				t.Error("malformed credential reached the round tripper")
				return nil, errors.New("unreachable")
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		ref := "cred"
		_, err = transport.roundTrip(context.Background(), &ref, providerLimitRequest())
		if !IsCode(err, "CREDENTIAL_MALFORMED") {
			t.Fatalf("error = %v, want CREDENTIAL_MALFORMED", err)
		}
	}
}

func TestHTTPTransportSeparatesPreDialRejectionFromTransportFailure(t *testing.T) {
	t.Parallel()
	ref := "cred"

	t.Run("invalid header value is refused before dial", func(t *testing.T) {
		t.Parallel()
		transport := newProviderLimitTransport(t, roundTripperFunc(
			func(*http.Request) (*http.Response, error) {
				t.Error("invalid header value reached the round tripper")
				return nil, errors.New("unreachable")
			},
		))
		request := providerLimitRequest()
		request.Headers = map[string]string{"X-Provider-Option": "private\nvalue"}
		_, err := transport.roundTrip(context.Background(), &ref, request)
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.Code != "INVALID_PROVIDER_REQUEST" {
			t.Fatalf("error = %v, want INVALID_PROVIDER_REQUEST", err)
		}
		if contractErr.Detail !=
			"request rejected before dial: invalid header field value for X-Provider-Option" ||
			strings.Contains(contractErr.Detail, "private") ||
			validateText(contractErr.Detail, maxProviderErrorDetailBytes, false) != nil {
			t.Fatalf("detail = %q", contractErr.Detail)
		}
		if kind := classifyKind(contractErr.Code, false); kind == KindTransport {
			t.Fatalf("pre-dial rejection classified as %q", kind)
		}
	})

	t.Run("real net/http transport never sees the request", func(t *testing.T) {
		t.Parallel()
		// With no round tripper injected the transport is net/http's own,
		// which is where the undifferentiated refusal used to come from.
		config := newProviderLimitTransport(t, nil).config
		config.CredentialPrefix = "Bearer\n"
		transport, err := newHTTPTransport(
			config,
			AuthModeBearer,
			func(context.Context, string) ([]byte, error) {
				return []byte("secret"), nil
			},
			nil,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = transport.roundTrip(context.Background(), &ref, providerLimitRequest())
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.Code != "INVALID_PROVIDER_REQUEST" ||
			!strings.HasSuffix(contractErr.Detail, " for Authorization") ||
			strings.Contains(contractErr.Detail, "secret") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("dial failure stays a transport failure", func(t *testing.T) {
		t.Parallel()
		transport := newProviderLimitTransport(t, roundTripperFunc(
			func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial tcp: connection refused")
			},
		))
		_, err := transport.roundTrip(context.Background(), &ref, providerLimitRequest())
		if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
			t.Fatalf("error = %v, want PROVIDER_TRANSPORT_FAILED", err)
		}
	})

	t.Run("tab in a header value is still sendable", func(t *testing.T) {
		t.Parallel()
		called := false
		transport := newProviderLimitTransport(t, roundTripperFunc(
			func(*http.Request) (*http.Response, error) {
				called = true
				return statusResponse(http.StatusOK, nil, `{}`), nil
			},
		))
		request := providerLimitRequest()
		request.Headers = map[string]string{"X-Provider-Option": "a\tb"}
		if _, err := transport.roundTrip(context.Background(), &ref, request); err != nil || !called {
			t.Fatalf("error = %v, called = %t", err, called)
		}
	})
}

func TestDoctorFailsMalformedFileCredentialAndPassesTrimmedOne(t *testing.T) {
	config := completeDriverConfigFixture(t)
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	credentialPath := ""
	for _, source := range config.Credentials {
		if source.Key == "gemini-file" {
			credentialPath = source.Reference
		}
	}
	if err := os.MkdirAll(filepath.Dir(credentialPath), 0o700); err != nil {
		t.Fatal(err)
	}
	registry, err := loaded.BuildRegistry(
		[]string{"gemini"},
		DriverFactoryOptions{FileCredentials: systemFileCredential},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		body  string
		state ReadinessState
		code  string
	}{
		{"trailing newline is trimmed", "token-value\n", ReadinessPass, "http_boundary_ready"},
		{"interior newline is malformed", "token\nvalue\n", ReadinessFail, "credential_malformed"},
		{"only line endings is malformed", "\r\n", ReadinessFail, "credential_malformed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(credentialPath, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			report := registry.Doctor(context.Background(), "gemini", "model-gemini")
			if report.State != test.state || report.Code != test.code {
				t.Fatalf("doctor = %s %s, want %s %s",
					report.State, report.Code, test.state, test.code)
			}
			encoded, err := canonicalJSON(report)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte("token")) {
				t.Fatalf("doctor report leaked credential bytes")
			}
		})
	}
	// An absent credential stays outside doctor's verdict, as before.
	if err := os.Remove(credentialPath); err != nil {
		t.Fatal(err)
	}
	report := registry.Doctor(context.Background(), "gemini", "model-gemini")
	if report.State != ReadinessPass || report.Code != "http_boundary_ready" {
		t.Fatalf("absent credential doctor = %s %s", report.State, report.Code)
	}
}

func TestOpenAIAdapterOutputCeilingClampsBothSurfaces(t *testing.T) {
	t.Parallel()
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("secret"), nil
	}
	requestBody := func(t *testing.T, config OpenAIProfileConfig, limit int64) []byte {
		t.Helper()
		adapter, err := NewOpenAIAdapter(config, resolver, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		loop, ok := adapter.(*loopAdapter)
		if !ok {
			t.Fatalf("adapter type = %T", adapter)
		}
		conversation, err := loop.new(
			[]byte(`{}`),
			"exact-model",
			toolDefinitions(ReadWrite),
			Limits{TimeoutMillis: 120_000, OutputBytes: limit},
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()
		request, err := conversation.request()
		if err != nil {
			t.Fatal(err)
		}
		return request.Body
	}
	base := func(api OpenAIAPI, endpoint string) OpenAIProfileConfig {
		config := OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "a-ceiling", ID: "sworn.ceiling", Version: "1.0.0",
				Endpoint:         endpoint,
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"cred"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API: api,
		}
		if api == OpenAIResponsesAPI {
			config.ReasoningEffort = "medium"
		}
		return config
	}
	for _, surface := range []struct {
		name     string
		api      OpenAIAPI
		endpoint string
		field    string
	}{
		{"responses", OpenAIResponsesAPI,
			"https://provider.test/v1/responses", "max_output_tokens"},
		{"chat", OpenAIChatCompletionsAPI,
			"https://provider.test/v1/chat/completions", "max_completion_tokens"},
	} {
		t.Run(surface.name, func(t *testing.T) {
			t.Parallel()
			for _, test := range []struct {
				name    string
				ceiling int64
				limit   int64
				want    string
			}{
				{"absent sends the limit", 0, MaxProviderOutputBytes, "1048576"},
				{"ceiling below the limit clamps", 524_288, MaxProviderOutputBytes, "524288"},
				{"ceiling above the limit keeps the limit", 524_288, 65_536, "65536"},
				{"ceiling equal to the limit", 65_536, 65_536, "65536"},
			} {
				config := base(surface.api, surface.endpoint)
				config.MaxOutputTokens = test.ceiling
				body := requestBody(t, config, test.limit)
				var sent map[string]json.RawMessage
				if err := json.Unmarshal(body, &sent); err != nil {
					t.Fatal(err)
				}
				if string(sent[surface.field]) != test.want {
					t.Fatalf("%s: %s = %s, want %s",
						test.name, surface.field, sent[surface.field], test.want)
				}
			}
		})
	}
}

func TestOpenAIAdapterOutputCeilingConfigValidationAndCanonicalForm(t *testing.T) {
	config := completeDriverConfigFixture(t)
	before, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte("max_output_tokens")) {
		t.Fatalf("absent ceiling leaked into canonical form: %s", before)
	}
	setCeiling := func(value int64) DriverConfig {
		clone := config
		clone.Adapters = append([]DriverAdapterConfig(nil), config.Adapters...)
		for index := range clone.Adapters {
			if clone.Adapters[index].OpenAI != nil &&
				clone.Adapters[index].OpenAI.Key == "a-openai" {
				openAI := cloneOpenAIProfileConfig(*clone.Adapters[index].OpenAI)
				openAI.MaxOutputTokens = value
				clone.Adapters[index].OpenAI = &openAI
			}
		}
		return clone
	}
	body, err := EncodeDriverConfig(setCeiling(524_288))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"max_output_tokens":524288`)) {
		t.Fatalf("ceiling missing from canonical form: %s", body)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ConfigurationDigest() != Digest(body) ||
		loaded.ConfigurationDigest() == Digest(before) {
		t.Fatalf("ceiling digest = %s", loaded.ConfigurationDigest())
	}
	if _, err := EncodeDriverConfig(setCeiling(MaxProviderOutputBytes)); err != nil {
		t.Fatalf("ceiling at the maximum error = %v", err)
	}
	for _, value := range []int64{-1, MaxProviderOutputBytes + 1} {
		if _, err := EncodeDriverConfig(setCeiling(value)); !IsCode(err, "INVALID_DRIVER_CONFIG") {
			t.Fatalf("ceiling %d error = %v", value, err)
		}
	}
	// An explicit zero is not the canonical spelling of absence.
	zero := bytes.Replace(
		body,
		[]byte(`"max_output_tokens":524288`),
		[]byte(`"max_output_tokens":0`),
		1,
	)
	if _, err := DecodeDriverConfig(zero); !IsCode(err, "NONCANONICAL_JSON") {
		t.Fatalf("explicit zero ceiling error = %v", err)
	}
}

func TestOpenAIAdapterReasoningSummaryThreadsToResponsesRequest(t *testing.T) {
	t.Parallel()
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("secret"), nil
	}
	requestBody := func(t *testing.T, config OpenAIProfileConfig) []byte {
		t.Helper()
		adapter, err := NewOpenAIAdapter(config, resolver, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		loop, ok := adapter.(*loopAdapter)
		if !ok {
			t.Fatalf("adapter type = %T", adapter)
		}
		conversation, err := loop.new(
			[]byte(`{}`),
			"exact-model",
			toolDefinitions(ReadWrite),
			Limits{TimeoutMillis: 120_000, OutputBytes: MaxProviderOutputBytes},
		)
		if err != nil {
			t.Fatal(err)
		}
		defer conversation.close()
		request, err := conversation.request()
		if err != nil {
			t.Fatal(err)
		}
		return request.Body
	}
	base := func(stream bool) OpenAIProfileConfig {
		return OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "a-summary", ID: "sworn.summary", Version: "1.0.0",
				Endpoint:         "https://provider.test/v1/responses",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"cred"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:             OpenAIResponsesAPI,
			ReasoningEffort: "high",
			Stream:          stream,
		}
	}
	reasoning := func(t *testing.T, body []byte) map[string]json.RawMessage {
		t.Helper()
		var sent struct {
			Reasoning map[string]json.RawMessage `json:"reasoning"`
		}
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatal(err)
		}
		return sent.Reasoning
	}
	for _, stream := range []bool{false, true} {
		absent := reasoning(t, requestBody(t, base(stream)))
		if string(absent["effort"]) != `"high"` || len(absent) != 1 {
			t.Fatalf("stream=%t absent summary reasoning = %v", stream, absent)
		}
		for _, value := range []string{"auto", "concise", "detailed"} {
			config := base(stream)
			config.ReasoningSummary = value
			sent := reasoning(t, requestBody(t, config))
			if string(sent["effort"]) != `"high"` ||
				string(sent["summary"]) != `"`+value+`"` || len(sent) != 2 {
				t.Fatalf("stream=%t summary %q reasoning = %v", stream, value, sent)
			}
		}
	}
	// The chat surface has no reasoning.summary; the field is refused there
	// exactly as stream is, rather than silently dropped.
	chat := base(false)
	chat.API = OpenAIChatCompletionsAPI
	chat.Endpoint = "https://provider.test/v1/chat/completions"
	chat.ReasoningEffort = ""
	if _, err := NewOpenAIAdapter(chat, resolver, nil, nil, nil); err != nil {
		t.Fatalf("chat without summary error = %v", err)
	}
	chat.ReasoningSummary = "auto"
	if _, err := NewOpenAIAdapter(chat, resolver, nil, nil, nil); !IsCode(err, "INVALID_ADAPTER") {
		t.Fatalf("chat with summary error = %v", err)
	}
	invalid := base(false)
	invalid.ReasoningSummary = "verbose"
	if _, err := NewOpenAIAdapter(invalid, resolver, nil, nil, nil); !IsCode(err, "INVALID_ADAPTER") {
		t.Fatalf("invalid summary error = %v", err)
	}
}

func TestOpenAIAdapterReasoningSummaryConfigValidationAndCanonicalForm(t *testing.T) {
	config := completeDriverConfigFixture(t)
	before, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte("reasoning_summary")) {
		t.Fatalf("absent summary leaked into canonical form: %s", before)
	}
	setSummary := func(key, value string) DriverConfig {
		clone := config
		clone.Adapters = append([]DriverAdapterConfig(nil), config.Adapters...)
		for index := range clone.Adapters {
			if clone.Adapters[index].OpenAI != nil &&
				clone.Adapters[index].OpenAI.Key == key {
				openAI := cloneOpenAIProfileConfig(*clone.Adapters[index].OpenAI)
				openAI.ReasoningSummary = value
				clone.Adapters[index].OpenAI = &openAI
			}
		}
		return clone
	}
	// An explicit empty value is absence: the canonical form and digest are
	// the same bytes as leaving the field out.
	unchanged, err := EncodeDriverConfig(setSummary("a-openai", ""))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, before) || Digest(unchanged) != Digest(before) {
		t.Fatalf("empty summary changed the canonical form: %s", unchanged)
	}
	for _, value := range []string{"auto", "concise", "detailed"} {
		body, err := EncodeDriverConfig(setSummary("a-openai", value))
		if err != nil {
			t.Fatalf("summary %q error = %v", value, err)
		}
		want := []byte(`"reasoning_summary":"` + value + `"`)
		if !bytes.Contains(body, want) {
			t.Fatalf("summary %q missing from canonical form: %s", value, body)
		}
		loaded, err := DecodeDriverConfig(body)
		if err != nil {
			t.Fatalf("summary %q decode error = %v", value, err)
		}
		if loaded.ConfigurationDigest() != Digest(body) ||
			loaded.ConfigurationDigest() == Digest(before) {
			t.Fatalf("summary %q digest = %s", value, loaded.ConfigurationDigest())
		}
	}
	for _, value := range []string{"verbose", "Auto", "auto "} {
		if _, err := EncodeDriverConfig(setSummary("a-openai", value)); !IsCode(err, "INVALID_DRIVER_CONFIG") {
			t.Fatalf("summary %q error = %v", value, err)
		}
	}
	// A chat-completions adapter has no reasoning.summary to carry.
	if _, err := EncodeDriverConfig(setSummary("a-openai-chat", "auto")); !IsCode(err, "INVALID_DRIVER_CONFIG") {
		t.Fatalf("chat summary error = %v", err)
	}
}

// TestHTTPRoundTripCapturesRequestIDOnProviderRefusalsOnly pins
// S4-lane-live-probe C1: a non-2xx response's request id rides the
// returned ContractError for 401, 429 and 503, and no raw header block or
// body ever reaches it - only the bounded, allowlisted extraction does.
func TestHTTPRoundTripCapturesRequestIDOnProviderRefusalsOnly(t *testing.T) {
	t.Parallel()
	ref := "cred"
	for _, test := range []struct {
		name   string
		status int
		header http.Header
		body   string
		code   string
	}{
		{
			"authorization", http.StatusUnauthorized,
			http.Header{"X-Request-Id": {"auth-id"}},
			`{"error":{"message":"bad key"}}`,
			"PROVIDER_AUTHORIZATION_FAILED",
		},
		{
			"limited", http.StatusTooManyRequests,
			http.Header{"Request-Id": {"limited-id"}, "Retry-After": {"5"}},
			`{"error":{"message":"slow down"}}`,
			"PROVIDER_LIMITED",
		},
		{
			"unavailable", http.StatusServiceUnavailable,
			http.Header{"X-Amzn-Requestid": {"unavailable-id"}},
			`{"error":{"message":"down"}}`,
			"PROVIDER_UNAVAILABLE",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			transport := newProviderLimitTransport(t, roundTripperFunc(
				func(*http.Request) (*http.Response, error) {
					return statusResponse(test.status, test.header, test.body), nil
				},
			))
			_, err := transport.roundTrip(
				context.Background(), &ref, providerLimitRequest(),
			)
			var contractErr *ContractError
			if !errors.As(err, &contractErr) || contractErr.Code != test.code {
				t.Fatalf("error = %v, want code %s", err, test.code)
			}
			var wantID string
			for key := range test.header {
				if key == "Retry-After" {
					continue
				}
				wantID = test.header.Get(key)
			}
			if contractErr.RequestID != wantID {
				t.Fatalf("request id = %q, want %q", contractErr.RequestID, wantID)
			}
			if len(contractErr.RequestID) > maxLaneProbeRequestIDBytes {
				t.Fatalf("request id exceeds its bound: %d bytes", len(contractErr.RequestID))
			}
			if strings.ContainsAny(contractErr.RequestID, "\r\n\t") {
				t.Fatalf("request id carried a control character: %q", contractErr.RequestID)
			}
		})
	}
}

// TestHTTPRoundTripHonorsHeaderRequestIDOverBody pins the extractor's
// stated priority: an allowlisted header wins over a body id field when
// both are present.
func TestHTTPRoundTripHonorsHeaderRequestIDOverBody(t *testing.T) {
	t.Parallel()
	ref := "cred"
	transport := newProviderLimitTransport(t, roundTripperFunc(
		func(*http.Request) (*http.Response, error) {
			return statusResponse(
				http.StatusBadRequest,
				http.Header{"X-Request-Id": {"header-id"}},
				`{"error":{"message":"bad request"},"id":"body-id"}`,
			), nil
		},
	))
	_, err := transport.roundTrip(context.Background(), &ref, providerLimitRequest())
	var contractErr *ContractError
	if !errors.As(err, &contractErr) || contractErr.RequestID != "header-id" {
		t.Fatalf("error = %v", err)
	}
}

// TestHTTPRoundTripSuccessCarriesNoRequestIDField pins the scope boundary:
// a successful (2xx) round trip returns only the response body, exactly as
// before - request id extraction on success happens only inside the lane
// probe, which reads the body itself.
func TestHTTPRoundTripSuccessCarriesNoRequestIDField(t *testing.T) {
	t.Parallel()
	ref := "cred"
	transport := newProviderLimitTransport(t, roundTripperFunc(
		func(*http.Request) (*http.Response, error) {
			return statusResponse(
				http.StatusOK,
				http.Header{"X-Request-Id": {"success-id"}},
				`{"id":"resp-id"}`,
			), nil
		},
	))
	body, err := transport.roundTrip(context.Background(), &ref, providerLimitRequest())
	if err != nil || string(body) != `{"id":"resp-id"}` {
		t.Fatalf("body = %s, err = %v", body, err)
	}
}
