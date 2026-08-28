package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func providerBalanceProbeTransport(
	t *testing.T,
	balanceProbe *BalanceProbeConfig,
	roundTripper http.RoundTripper,
) *httpTransport {
	t.Helper()
	config := HTTPProfileConfig{
		Key: "balance-probe-transport", ID: "sworn.balance-probe-transport",
		Version:          "1.0.0",
		Endpoint:         "https://provider.test/v1/chat",
		CredentialHeader: "Authorization",
		CredentialPrefix: "Bearer ",
		CredentialRefs:   []string{"cred"},
		ResponseBytes:    MaxProviderResponseBytes,
		BalanceProbe:     balanceProbe,
	}
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("secret"), nil
	}
	transport, err := newHTTPTransport(config, AuthModeBearer, resolver, nil, roundTripper)
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func providerBalanceProbeTestSelection(transport *httpTransport, ref *string) SelectedProfile {
	adapter := &loopAdapter{
		identity: AdapterIdentity{
			Key: "balance-probe-adapter", ID: "sworn.balance-probe", Version: "1.0.0",
			ConfigurationDigest: "sha256:" + string(bytes.Repeat([]byte("e"), 64)),
		},
		transport: transport,
	}
	return SelectedProfile{
		Profile: ProfileConfig{
			Key: "balance-probe-profile", Adapter: adapter.identity.Key,
			Network: NetworkRequired, CredentialRef: ref,
		},
		Adapter: adapter.identity,
		Model:   "balance-probe-model",
		adapter: adapter,
	}
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		Header:     make(http.Header),
	}
}

// TestProbeProviderBalanceRefusesPositiveHardExhaustion pins A3(b)'s
// named-refusal half: a strictly-decoded true exhaustion field refuses
// PROVIDER_LIMITED with HardLimit set, classifying to KindHardExhaustion.
func TestProbeProviderBalanceRefusesPositiveHardExhaustion(t *testing.T) {
	transport := providerBalanceProbeTransport(
		t,
		&BalanceProbeConfig{
			Endpoint: "https://provider.test/v1/balance", ExhaustedField: "exhausted",
			CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
		},
		roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.String() != "https://provider.test/v1/balance" ||
				request.Method != http.MethodGet {
				t.Fatalf("unexpected balance request: %s %s", request.Method, request.URL)
			}
			return jsonResponse(200, `{"exhausted":true}`), nil
		}),
	)
	ref := "cred"
	selected := providerBalanceProbeTestSelection(transport, &ref)
	eventBody, err := ProbeProviderBalance(context.Background(), selected, "run-balance-exhausted")
	var contractErr *ContractError
	if !errors.As(err, &contractErr) || contractErr.Code != "PROVIDER_LIMITED" ||
		!contractErr.HardLimit {
		t.Fatalf("balance probe error = %v, want PROVIDER_LIMITED HardLimit", err)
	}
	if classifyKind(contractErr.Code, contractErr.HardLimit) != KindHardExhaustion {
		t.Fatalf("balance probe Kind = %v, want KindHardExhaustion",
			classifyKind(contractErr.Code, contractErr.HardLimit))
	}
	event := decodeProviderBalanceProbeEvent(t, eventBody)
	if event.Outcome != nativeAdmissionProbeRefused || event.RunID != "run-balance-exhausted" {
		t.Fatalf("probe event = %#v, want a refused outcome", event)
	}
}

// TestProbeProviderBalancePassesNegativeExhaustion pins the healthy path.
func TestProbeProviderBalancePassesNegativeExhaustion(t *testing.T) {
	transport := providerBalanceProbeTransport(
		t,
		&BalanceProbeConfig{
			Endpoint: "https://provider.test/v1/balance", ExhaustedField: "exhausted",
		},
		roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(200, `{"exhausted":false}`), nil
		}),
	)
	ref := "cred"
	selected := providerBalanceProbeTestSelection(transport, &ref)
	eventBody, err := ProbeProviderBalance(context.Background(), selected, "run-balance-live")
	if err != nil {
		t.Fatalf("live-balance probe refused: %v", err)
	}
	event := decodeProviderBalanceProbeEvent(t, eventBody)
	if event.Outcome != nativeAdmissionProbePassed {
		t.Fatalf("probe event = %#v, want passed", event)
	}
}

// TestProbeProviderBalanceDegradesToUnevaluable pins the honesty floor: every
// infra, status, or decode failure admits, never refuses.
func TestProbeProviderBalanceDegradesToUnevaluable(t *testing.T) {
	cases := []struct {
		name      string
		transport func(t *testing.T) *httpTransport
	}{
		{
			name: "transport failure",
			transport: func(t *testing.T) *httpTransport {
				return providerBalanceProbeTransport(
					t,
					&BalanceProbeConfig{
						Endpoint: "https://provider.test/v1/balance", ExhaustedField: "exhausted",
					},
					roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return nil, errors.New("dial refused")
					}),
				)
			},
		},
		{
			name: "non-2xx status",
			transport: func(t *testing.T) *httpTransport {
				return providerBalanceProbeTransport(
					t,
					&BalanceProbeConfig{
						Endpoint: "https://provider.test/v1/balance", ExhaustedField: "exhausted",
					},
					roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return jsonResponse(500, `{"exhausted":true}`), nil
					}),
				)
			},
		},
		{
			name: "malformed body",
			transport: func(t *testing.T) *httpTransport {
				return providerBalanceProbeTransport(
					t,
					&BalanceProbeConfig{
						Endpoint: "https://provider.test/v1/balance", ExhaustedField: "exhausted",
					},
					roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return jsonResponse(200, `not json`), nil
					}),
				)
			},
		},
		{
			name: "missing field",
			transport: func(t *testing.T) *httpTransport {
				return providerBalanceProbeTransport(
					t,
					&BalanceProbeConfig{
						Endpoint: "https://provider.test/v1/balance", ExhaustedField: "exhausted",
					},
					roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return jsonResponse(200, `{"other":true}`), nil
					}),
				)
			},
		},
		{
			name: "wrong-typed field",
			transport: func(t *testing.T) *httpTransport {
				return providerBalanceProbeTransport(
					t,
					&BalanceProbeConfig{
						Endpoint: "https://provider.test/v1/balance", ExhaustedField: "exhausted",
					},
					roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return jsonResponse(200, `{"exhausted":"yes"}`), nil
					}),
				)
			},
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			ref := "cred"
			selected := providerBalanceProbeTestSelection(test.transport(t), &ref)
			eventBody, err := ProbeProviderBalance(context.Background(), selected, "run-balance-degrade")
			if err != nil {
				t.Fatalf("%s: probe refused instead of degrading: %v", test.name, err)
			}
			event := decodeProviderBalanceProbeEvent(t, eventBody)
			if event.Outcome != nativeAdmissionProbeUnevaluable {
				t.Fatalf("%s: probe event = %#v, want unevaluable", test.name, event)
			}
		})
	}
}

// TestProbeProviderBalanceIsANoOpWhenUnconfigured pins the applicability
// gate: no in-tree adapter sets BalanceProbe, so this stays a complete no-op
// today.
func TestProbeProviderBalanceIsANoOpWhenUnconfigured(t *testing.T) {
	transport := providerBalanceProbeTransport(t, nil, roundTripperFunc(
		func(*http.Request) (*http.Response, error) {
			t.Fatal("unconfigured balance probe must never make a request")
			return nil, nil
		},
	))
	ref := "cred"
	selected := providerBalanceProbeTestSelection(transport, &ref)
	body, err := ProbeProviderBalance(context.Background(), selected, "run-unconfigured")
	if body != nil || err != nil {
		t.Fatalf("unconfigured balance probe = (%v, %v), want (nil, nil)", body, err)
	}
}

// TestProbeProviderBalanceIsANoOpForNonHTTPAdapters pins the type-assertion
// applicability gate.
func TestProbeProviderBalanceIsANoOpForNonHTTPAdapters(t *testing.T) {
	adapter := processAdapterFixture(t, "a-fake", "sworn.fake-balance-probe")
	selected := SelectedProfile{
		Profile: ProfileConfig{Key: "fake-profile", Adapter: adapter.Identity().Key, Network: NetworkNone},
		Adapter: adapter.Identity(),
		Model:   "fake-model",
		adapter: adapter,
	}
	body, err := ProbeProviderBalance(context.Background(), selected, "run-fake")
	if body != nil || err != nil {
		t.Fatalf("non-HTTP balance probe = (%v, %v), want (nil, nil)", body, err)
	}
}

func decodeProviderBalanceProbeEvent(t *testing.T, body []byte) ProviderBalanceProbeEvent {
	t.Helper()
	var event ProviderBalanceProbeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatalf("decode probe event: %v", err)
	}
	return event
}
