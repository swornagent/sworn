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

// TestProbeLaneNeverCallsAnUnrequestedProfilesTransport pins A3's no-
// fallback promise at the registry level: a registry admitting two
// explicit lanes calls only the one the caller named, never the other,
// whichever order they were registered in.
func TestProbeLaneNeverCallsAnUnrequestedProfilesTransport(t *testing.T) {
	t.Parallel()
	var calledA, calledB int
	roundTripperA := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calledA++
		return statusResponse(200, http.Header{}, `{"choices":[]}`), nil
	})
	roundTripperB := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calledB++
		return statusResponse(200, http.Header{}, `{"choices":[]}`), nil
	})
	adapterA := laneProbeChatAdapterKeyed(t, "probe-lane-a-adapter", roundTripperA)
	adapterB := laneProbeChatAdapterKeyed(t, "probe-lane-b-adapter", roundTripperB)
	refA, refB := "probe-cred", "probe-cred"
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{
			{Key: "lane-a", Adapter: adapterA.Identity().Key, Network: NetworkRequired, CredentialRef: &refA},
			{Key: "lane-b", Adapter: adapterB.Identity().Key, Network: NetworkRequired, CredentialRef: &refB},
		},
		[]Adapter{adapterA, adapterB},
	)
	if err != nil {
		t.Fatal(err)
	}
	configured := ConfiguredDriverRegistry{SelectionRegistry: registry}
	result, err := ProbeLane(context.Background(), configured, "lane-a", "model")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready || calledA != 1 || calledB != 0 {
		t.Fatalf("result = %#v, calledA = %d, calledB = %d", result, calledA, calledB)
	}
	result, err = ProbeLane(context.Background(), configured, "lane-b", "model")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready || calledA != 1 || calledB != 1 {
		t.Fatalf("result = %#v, calledA = %d, calledB = %d", result, calledA, calledB)
	}
}

// TestProbeLaneAdapterIdentityRidesTheResultWithNoSecretOrPathBytes proves
// the exposed single-function result stays closed and secret-free, so a
// Manager seat's CLI probe and S5's backoff wait can log or persist it
// directly.
func TestProbeLaneAdapterIdentityRidesTheResultWithNoSecretOrPathBytes(t *testing.T) {
	t.Parallel()
	// laneProbeChatAdapter's resolver always returns this exact secret.
	const secret = "secret-canary"
	roundTripper := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return statusResponse(200, http.Header{}, `{"choices":[]}`), nil
	})
	adapter := laneProbeChatAdapter(t, roundTripper)
	ref := "probe-cred"
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: "identity-lane", Adapter: adapter.Identity().Key,
			Network: NetworkRequired, CredentialRef: &ref,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	configured := ConfiguredDriverRegistry{SelectionRegistry: registry}
	result, err := ProbeLane(context.Background(), configured, "identity-lane", "model")
	if err != nil || !result.Ready {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if result.AdapterID != adapter.Identity().ID ||
		result.AdapterVersion != adapter.Identity().Version ||
		result.ConfigurationDigest != adapter.Identity().ConfigurationDigest {
		t.Fatalf("identity = %#v, want %#v", result, adapter.Identity())
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytesContains(encoded, []byte(secret)) || bytesContains(encoded, []byte(ref)) {
		t.Fatalf("probe result leaked private data: %s", encoded)
	}
}
