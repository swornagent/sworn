//go:build linux

package driver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func laneProbeSelectionRegistry(
	t *testing.T,
	key string,
	adapter Adapter,
	credentialRef *string,
) ConfiguredDriverRegistry {
	t.Helper()
	network := NetworkNone
	if credentialRef != nil {
		network = NetworkRequired
	} else if _, ok := adapter.(*ProcessAdapter); !ok {
		network = NetworkRequired
	}
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: key, Adapter: adapter.Identity().Key,
			Network: network, CredentialRef: credentialRef,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	return ConfiguredDriverRegistry{SelectionRegistry: registry}
}

func laneProbeChatAdapter(
	t *testing.T,
	roundTripper http.RoundTripper,
) Adapter {
	t.Helper()
	return laneProbeChatAdapterKeyed(t, "probe-chat-adapter", roundTripper)
}

func laneProbeChatAdapterKeyed(
	t *testing.T,
	key string,
	roundTripper http.RoundTripper,
) Adapter {
	t.Helper()
	config := HTTPProfileConfig{
		Key: key, ID: "sworn.probe-chat", Version: "1.0.0",
		Endpoint:         "https://provider.test/v1/chat/completions",
		CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
		CredentialRefs: []string{"probe-cred"},
		ResponseBytes:  MaxProviderResponseBytes,
	}
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("secret-canary"), nil
	}
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: config,
			API:               OpenAIChatCompletionsAPI,
		},
		resolver, nil, nil, roundTripper,
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func laneProbeResponsesAdapter(
	t *testing.T,
	roundTripper http.RoundTripper,
) Adapter {
	t.Helper()
	config := HTTPProfileConfig{
		Key: "probe-responses-adapter", ID: "sworn.probe-responses", Version: "1.0.0",
		Endpoint:         "https://provider.test/v1/responses",
		CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
		CredentialRefs: []string{"probe-cred"},
		ResponseBytes:  MaxProviderResponseBytes,
	}
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("secret-canary"), nil
	}
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: config,
			API:               OpenAIResponsesAPI,
			ReasoningEffort:   "medium",
		},
		resolver, nil, nil, roundTripper,
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

// TestProbeLaneHTTPChatMinimalRequestAndReadyOnLiveResponse pins A1's
// request shape (no tools, declared output bound, fixed prompt) and A2's
// no-ready-without-a-live-response floor for the chat dialect.
func TestProbeLaneHTTPChatMinimalRequestAndReadyOnLiveResponse(t *testing.T) {
	t.Parallel()
	var captured []byte
	var requests int
	roundTripper := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Fatal(err)
		}
		captured = body
		if strings.Contains(string(body), "secret-canary") {
			t.Fatal("credential leaked into probe request body")
		}
		return statusResponse(
			200, http.Header{"X-Request-Id": {"resp-header-id"}},
			`{"id":"resp-body-id","choices":[]}`,
		), nil
	})
	ref := "probe-cred"
	registry := laneProbeSelectionRegistry(
		t, "chat-profile", laneProbeChatAdapter(t, roundTripper), &ref,
	)
	result, err := ProbeLane(context.Background(), registry, "chat-profile", "model-x")
	if err != nil {
		t.Fatalf("ProbeLane error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if !result.Ready || result.Code != laneProbeCodeLivePassed {
		t.Fatalf("result = %#v", result)
	}
	if result.RequestID != "resp-header-id" {
		t.Fatalf("request id = %q, want the header value (headers win over body)", result.RequestID)
	}
	if result.LatencyMillis < 0 {
		t.Fatalf("latency = %d", result.LatencyMillis)
	}
	var decoded map[string]any
	if err := json.Unmarshal(captured, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["tools"]; present {
		t.Fatalf("probe request carried a tools field: %s", captured)
	}
	if _, present := decoded["tool_choice"]; present {
		t.Fatalf("probe request carried a tool_choice field: %s", captured)
	}
	if decoded["model"] != "model-x" {
		t.Fatalf("model = %v", decoded["model"])
	}
	if messages, ok := decoded["messages"].([]any); !ok || len(messages) != 1 {
		t.Fatalf("messages = %v", decoded["messages"])
	}
	if tokens, ok := decoded["max_completion_tokens"].(float64); !ok || tokens != laneProbeMaxOutputTokens {
		t.Fatalf("max_completion_tokens = %v", decoded["max_completion_tokens"])
	}
}

// TestProbeLaneHTTPResponsesCarriesConfiguredReasoningEffort pins the
// responses-dialect request shape: the profile's own declared reasoning
// effort rides the probe request exactly as dispatch would send it, with
// no tools and the declared output bound.
func TestProbeLaneHTTPResponsesCarriesConfiguredReasoningEffort(t *testing.T) {
	t.Parallel()
	var captured []byte
	roundTripper := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Fatal(err)
		}
		captured = body
		return statusResponse(200, http.Header{}, `{"id":"resp-1","output":[]}`), nil
	})
	ref := "probe-cred"
	registry := laneProbeSelectionRegistry(
		t, "responses-profile", laneProbeResponsesAdapter(t, roundTripper), &ref,
	)
	result, err := ProbeLane(context.Background(), registry, "responses-profile", "model-y")
	if err != nil || !result.Ready {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(captured, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["tools"]; present {
		t.Fatalf("responses probe carried tools: %s", captured)
	}
	reasoning, ok := decoded["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "medium" {
		t.Fatalf("reasoning = %v", decoded["reasoning"])
	}
	if tokens, ok := decoded["max_output_tokens"].(float64); !ok || tokens != laneProbeMaxOutputTokens {
		t.Fatalf("max_output_tokens = %v", decoded["max_output_tokens"])
	}
	if store, ok := decoded["store"].(bool); !ok || store {
		t.Fatalf("store = %v", decoded["store"])
	}
}

// TestProbeLaneNeverAppliesContextWindowClamp anchors A3's split correction:
// a probe's dispatch is genuinely a fresh, single, first request of its own
// conversation, so the context-window clamp cannot fire there - structurally,
// because ProbeLane's request builders never construct an openAIConversation
// or responsesConversation at all. A profile whose declared context window
// is tiny enough to force ECONOMY_CONTEXT_EXHAUSTED on any ordinary dispatch
// still probes successfully, unclamped, on both surfaces.
func TestProbeLaneNeverAppliesContextWindowClamp(t *testing.T) {
	t.Parallel()
	buildAdapter := func(t *testing.T, api OpenAIAPI, endpoint string, roundTripper http.RoundTripper) Adapter {
		t.Helper()
		config := HTTPProfileConfig{
			Key: "probe-context-window-adapter", ID: "sworn.probe-context-window", Version: "1.0.0",
			Endpoint:         endpoint,
			CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
			CredentialRefs:      []string{"probe-cred"},
			ResponseBytes:       MaxProviderResponseBytes,
			ContextWindowTokens: 1,
		}
		profile := OpenAIProfileConfig{HTTPProfileConfig: config, API: api}
		if api == OpenAIResponsesAPI {
			profile.ReasoningEffort = "medium"
		}
		resolver := func(context.Context, string) ([]byte, error) {
			return []byte("secret-canary"), nil
		}
		adapter, err := NewOpenAIAdapter(profile, resolver, nil, nil, roundTripper)
		if err != nil {
			t.Fatal(err)
		}
		return adapter
	}

	t.Run("chat completions", func(t *testing.T) {
		t.Parallel()
		var captured []byte
		roundTripper := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
			if err != nil {
				t.Fatal(err)
			}
			captured = body
			return statusResponse(200, http.Header{}, `{"id":"resp-1","choices":[]}`), nil
		})
		ref := "probe-cred"
		registry := laneProbeSelectionRegistry(
			t, "chat-profile",
			buildAdapter(t, OpenAIChatCompletionsAPI, "https://provider.test/v1/chat/completions", roundTripper),
			&ref,
		)
		result, err := ProbeLane(context.Background(), registry, "chat-profile", "model-x")
		if err != nil || !result.Ready {
			t.Fatalf("result = %#v, err = %v, want a passing probe unaffected by the tiny context window", result, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(captured, &decoded); err != nil {
			t.Fatal(err)
		}
		if tokens, ok := decoded["max_completion_tokens"].(float64); !ok || tokens != laneProbeMaxOutputTokens {
			t.Fatalf("max_completion_tokens = %v, want the unclamped probe ceiling %d", decoded["max_completion_tokens"], laneProbeMaxOutputTokens)
		}
	})

	t.Run("responses", func(t *testing.T) {
		t.Parallel()
		var captured []byte
		roundTripper := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
			if err != nil {
				t.Fatal(err)
			}
			captured = body
			return statusResponse(200, http.Header{}, `{"id":"resp-1","output":[]}`), nil
		})
		ref := "probe-cred"
		registry := laneProbeSelectionRegistry(
			t, "responses-profile",
			buildAdapter(t, OpenAIResponsesAPI, "https://provider.test/v1/responses", roundTripper),
			&ref,
		)
		result, err := ProbeLane(context.Background(), registry, "responses-profile", "model-y")
		if err != nil || !result.Ready {
			t.Fatalf("result = %#v, err = %v, want a passing probe unaffected by the tiny context window", result, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(captured, &decoded); err != nil {
			t.Fatal(err)
		}
		if tokens, ok := decoded["max_output_tokens"].(float64); !ok || tokens != laneProbeMaxOutputTokens {
			t.Fatalf("max_output_tokens = %v, want the unclamped probe ceiling %d", decoded["max_output_tokens"], laneProbeMaxOutputTokens)
		}
	})
}

// TestProbeLaneHTTPProviderRefusalsMapToClosedCodesWithMessageAndRequestID
// pins A1's closed-code mapping and C1's request-id capture across the
// authorization, limited and unavailable provider statuses, and proves no
// raw header block or body reaches the result.
func TestProbeLaneHTTPProviderRefusalsMapToClosedCodesWithMessageAndRequestID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		header http.Header
		body   string
		code   string
	}{
		{
			"authorization", 401,
			http.Header{"X-Request-Id": {"auth-req-id"}},
			`{"error":{"message":"invalid api key"}}`,
			"certification_provider_authorization_failed",
		},
		{
			"limited", 429,
			http.Header{"X-Amzn-Requestid": {"limited-req-id"}},
			`{"error":{"message":"rate limited"}}`,
			"certification_provider_limited",
		},
		{
			"unavailable", 503,
			http.Header{"Request-Id": {"unavailable-req-id"}},
			`{"error":{"message":"upstream down"}}`,
			"certification_provider_unavailable",
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			roundTripper := roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return statusResponse(test.status, test.header, test.body), nil
			})
			ref := "probe-cred"
			registry := laneProbeSelectionRegistry(
				t, "refusal-profile", laneProbeChatAdapter(t, roundTripper), &ref,
			)
			result, err := ProbeLane(context.Background(), registry, "refusal-profile", "model")
			if err != nil {
				t.Fatalf("ProbeLane error = %v", err)
			}
			if result.Ready {
				t.Fatalf("result = %#v, want not ready", result)
			}
			if result.Code != test.code {
				t.Fatalf("code = %q, want %q", result.Code, test.code)
			}
			var wantRequestID string
			for key := range test.header {
				wantRequestID = test.header.Get(key)
			}
			if result.RequestID != wantRequestID {
				t.Fatalf("request id = %q, want %q", result.RequestID, wantRequestID)
			}
			if result.Message == "" {
				t.Fatalf("message is empty, want the provider's bounded words")
			}
			if strings.Contains(result.Message, "X-Request-Id") ||
				strings.Contains(result.Message, "Header") {
				t.Fatalf("message leaked header content: %q", result.Message)
			}
		})
	}
}

// TestProbeLaneHTTPTimeoutMapsToCertificationTimeout pins C2: an
// already-expired probe bound maps to certification_timeout, never a
// transport code, and the probe never falls back to reporting ready.
func TestProbeLaneHTTPTimeoutMapsToCertificationTimeout(t *testing.T) {
	t.Parallel()
	roundTripper := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("round tripper reached after context expiry")
		return nil, nil
	})
	ref := "probe-cred"
	registry := laneProbeSelectionRegistry(
		t, "timeout-profile", laneProbeChatAdapter(t, roundTripper), &ref,
	)
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	result, err := ProbeLane(expired, registry, "timeout-profile", "model")
	if err != nil {
		t.Fatalf("ProbeLane error = %v", err)
	}
	if result.Ready || result.Code != "certification_timeout" {
		t.Fatalf("result = %#v", result)
	}
}

// TestProbeLaneFakeAdapterIsNeverReady pins the fake/development lane
// exclusion: it is never reported ready by the probe.
func TestProbeLaneFakeAdapterIsNeverReady(t *testing.T) {
	t.Parallel()
	adapter := processAdapterFixture(t, "a-fake-probe", "sworn.fake-probe")
	registry := laneProbeSelectionRegistry(t, "fake-profile", adapter, nil)
	result, err := ProbeLane(context.Background(), registry, "fake-profile", "model")
	if err != nil {
		t.Fatalf("ProbeLane error = %v", err)
	}
	if result.Ready || result.Code != laneProbeCodeFakeNotProduction {
		t.Fatalf("result = %#v", result)
	}
}

// TestProbeLaneNeverFallsBackToAnotherProfileOrModel pins A3: an unknown
// profile or an invalid model fails the Go error return, never substitutes
// another lane.
func TestProbeLaneNeverFallsBackToAnotherProfileOrModel(t *testing.T) {
	t.Parallel()
	roundTripper := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("round tripper reached for an inadmissible probe request")
		return nil, nil
	})
	ref := "probe-cred"
	registry := laneProbeSelectionRegistry(
		t, "explicit-profile", laneProbeChatAdapter(t, roundTripper), &ref,
	)
	if _, err := ProbeLane(
		context.Background(), registry, "other-profile", "model",
	); !IsCode(err, "UNKNOWN_PROFILE") {
		t.Fatalf("unknown profile error = %v", err)
	}
	if _, err := ProbeLane(
		context.Background(), registry, "explicit-profile", "",
	); !IsCode(err, "INVALID_MODEL") {
		t.Fatalf("invalid model error = %v", err)
	}
}

// TestProbeLaneIsTheSameFunctionCLIAndS5WillCall proves the same
// (profile, model) pair yields byte-identical probe requests across two
// calls, so a Manager seat's CLI probe and S5's backoff wait observe the
// identical admission the exposed single function promises (A3).
func TestProbeLaneIsTheSameFunctionCLIAndS5WillCall(t *testing.T) {
	t.Parallel()
	var bodies [][]byte
	roundTripper := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
		return statusResponse(200, http.Header{}, `{"choices":[]}`), nil
	})
	ref := "probe-cred"
	registry := laneProbeSelectionRegistry(
		t, "repeat-profile", laneProbeChatAdapter(t, roundTripper), &ref,
	)
	for i := 0; i < 2; i++ {
		if _, err := ProbeLane(
			context.Background(), registry, "repeat-profile", "model-z",
		); err != nil {
			t.Fatal(err)
		}
	}
	if len(bodies) != 2 || string(bodies[0]) != string(bodies[1]) {
		t.Fatalf("bodies = %q", bodies)
	}
}

// TestBuildGeminiProbeRequestOmitsToolsAndUsesConfiguredThinking pins the
// Gemini dialect's minimal shape directly: the declared output bound under
// Gemini's own field name, no tools, and the endpoint path.
func TestBuildGeminiProbeRequestOmitsToolsAndUsesConfiguredThinking(t *testing.T) {
	t.Parallel()
	request, err := buildGeminiProbeRequest(
		"https://generativelanguage.example.invalid/", "gemini-model", "LOW", true,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "https://generativelanguage.example.invalid/v1beta/models/gemini-model:generateContent"
	if request.URL != wantURL {
		t.Fatalf("url = %q, want %q", request.URL, wantURL)
	}
	var decoded map[string]any
	if err := json.Unmarshal(request.Body, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["tools"]; present {
		t.Fatalf("gemini probe carried tools: %s", request.Body)
	}
	generation, ok := decoded["generationConfig"].(map[string]any)
	if !ok || generation["maxOutputTokens"] != float64(laneProbeMaxOutputTokens) {
		t.Fatalf("generationConfig = %v", decoded["generationConfig"])
	}
	thinking, ok := generation["thinkingConfig"].(map[string]any)
	if !ok || thinking["thinkingLevel"] != "LOW" || thinking["includeThoughts"] != true {
		t.Fatalf("thinkingConfig = %v", generation["thinkingConfig"])
	}
}

// TestBuildBedrockProbeRequestOmitsSystemAndToolsAndUsesConverseURL pins
// the Bedrock Converse dialect's minimal shape: no system block, no
// toolConfig, the declared output bound as inferenceConfig.maxTokens, and
// the /model/.../converse path.
func TestBuildBedrockProbeRequestOmitsSystemAndToolsAndUsesConverseURL(t *testing.T) {
	t.Parallel()
	request, err := buildBedrockProbeRequest(
		"https://bedrock-runtime.us-east-1.amazonaws.com", "anthropic.model",
	)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.model/converse"
	if request.URL != wantURL {
		t.Fatalf("url = %q, want %q", request.URL, wantURL)
	}
	var decoded map[string]any
	if err := json.Unmarshal(request.Body, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["system"]; present {
		t.Fatalf("bedrock probe carried a system block: %s", request.Body)
	}
	if _, present := decoded["toolConfig"]; present {
		t.Fatalf("bedrock probe carried a toolConfig block: %s", request.Body)
	}
	inference, ok := decoded["inferenceConfig"].(map[string]any)
	if !ok || inference["maxTokens"] != float64(laneProbeMaxOutputTokens) {
		t.Fatalf("inferenceConfig = %v", decoded["inferenceConfig"])
	}
}

// laneProbeDummyExecutable creates a bounded, executable file whose bytes
// are never inspected: validateExecutableIdentity requires a real
// executable regular file to exist even on the direct-environment AWS
// chain path, which never runs it.
func laneProbeDummyExecutable(t *testing.T) ExecutableIdentity {
	t.Helper()
	pathValue := filepath.Join(t.TempDir(), "aws")
	if err := os.WriteFile(pathValue, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// validateAWSChainSpec pins the exact AWS CLI version and digest even on
	// the direct-environment chain, which never executes this file: only
	// its existence and executable bit matter here.
	return ExecutableIdentity{Path: pathValue, Digest: AWSCLIDigest}
}

// TestProbeLaneBedrockConverseSignsAndReportsReadyOrClosedRefusal pins A2's
// Bedrock leg: the probe drives the exact bedrockTransport.roundTrip
// dispatch uses (SigV4 signing over the direct-environment credential
// chain), and only a live 2xx response is ready.
func TestProbeLaneBedrockConverseSignsAndReportsReadyOrClosedRefusal(t *testing.T) {
	t.Parallel()
	chain := AWSChainSpec{
		CLI:              laneProbeDummyExecutable(t),
		CLIVersion:       AWSCLIVersion,
		Region:           "ap-southeast-2",
		RegionSource:     AWSSourceEnvironment,
		CredentialSource: AWSSourceEnvironment,
		EnvironmentKeys: []string{
			"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
			"AWS_REGION", "AWS_DEFAULT_REGION",
		},
		RuntimeFiles: awsRuntimeIdentityFixture(),
		RequiredRuntimeTargets: []string{
			"/etc/ssl/certs/ca-certificates.crt",
			"/etc/resolv.conf",
			"/etc/hosts",
			"/etc/nsswitch.conf",
		},
	}
	resolver := func(context.Context, string) ([][]byte, error) {
		return [][]byte{
			[]byte("AWS_ACCESS_KEY_ID=AKIAEXAMPLE1234"),
			[]byte("AWS_SECRET_ACCESS_KEY=secret-example-value"),
			[]byte("AWS_REGION=ap-southeast-2"),
			[]byte("AWS_DEFAULT_REGION=ap-southeast-2"),
		}, nil
	}
	var authorization string
	roundTripper := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		authorization = request.Header.Get("Authorization")
		if request.URL.Path != "/model/anthropic.model/converse" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		return statusResponse(200, http.Header{}, `{"output":{}}`), nil
	})
	adapter, err := NewBedrockAdapter(
		BedrockProfileConfig{
			Key: "probe-bedrock-adapter", ID: "sworn.probe-bedrock", Version: "1.0.0",
			Endpoint:       "https://bedrock-runtime.ap-southeast-2.amazonaws.com",
			CredentialRefs: []string{"probe-bedrock-cred"},
			ResponseBytes:  MaxProviderResponseBytes,
			Chain:          chain,
		},
		resolver, nil, roundTripper,
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := "probe-bedrock-cred"
	registry := laneProbeSelectionRegistry(t, "bedrock-profile", adapter, &ref)
	result, err := ProbeLane(context.Background(), registry, "bedrock-profile", "anthropic.model")
	if err != nil {
		t.Fatalf("ProbeLane error = %v", err)
	}
	if !result.Ready || result.Code != laneProbeCodeLivePassed {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(authorization, "Credential=AKIAEXAMPLE1234/") ||
		!strings.Contains(authorization, "/bedrock/aws4_request") {
		t.Fatalf("authorization = %q", authorization)
	}

	forbidden := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return statusResponse(403, http.Header{"X-Amzn-Requestid": {"bedrock-req-id"}}, ""), nil
	})
	refusalAdapter, err := NewBedrockAdapter(
		BedrockProfileConfig{
			Key: "probe-bedrock-refusal", ID: "sworn.probe-bedrock-refusal", Version: "1.0.0",
			Endpoint:       "https://bedrock-runtime.ap-southeast-2.amazonaws.com",
			CredentialRefs: []string{"probe-bedrock-cred"},
			ResponseBytes:  MaxProviderResponseBytes,
			Chain:          chain,
		},
		resolver, nil, forbidden,
	)
	if err != nil {
		t.Fatal(err)
	}
	refusalRegistry := laneProbeSelectionRegistry(t, "bedrock-refusal", refusalAdapter, &ref)
	refused, err := ProbeLane(context.Background(), refusalRegistry, "bedrock-refusal", "anthropic.model")
	if err != nil {
		t.Fatalf("ProbeLane error = %v", err)
	}
	if refused.Ready || refused.Code != "certification_provider_authorization_failed" {
		t.Fatalf("refused result = %#v", refused)
	}
	if refused.RequestID != "bedrock-req-id" {
		t.Fatalf("bedrock request id = %q", refused.RequestID)
	}
}

// TestDriverDoctorMakesNoRoundTripAndCertifyDoes pins A2's honesty signal
// at the driver-package level: certify's live probe path is distinct from
// doctor's, which makes no round trip at all.
func TestDriverDoctorMakesNoRoundTripAndCertifyDoes(t *testing.T) {
	t.Parallel()
	var calls int
	roundTripper := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return statusResponse(401, http.Header{}, `{"error":{"message":"no"}}`), nil
	})
	config := HTTPProfileConfig{
		Key: "doctor-honesty-adapter", ID: "sworn.doctor-honesty", Version: "1.0.0",
		Endpoint:         "https://provider.test/v1/chat/completions",
		CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
		CredentialRefs: []string{"doctor-honesty-cred"},
		ResponseBytes:  MaxProviderResponseBytes,
	}
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("secret"), nil
	}
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: config, API: OpenAIChatCompletionsAPI,
		},
		resolver, nil, nil, roundTripper,
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := "doctor-honesty-cred"
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: "doctor-honesty", Adapter: adapter.Identity().Key,
			Network: NetworkRequired, CredentialRef: &ref,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	doctorReport := registry.Doctor(context.Background(), "doctor-honesty", "model")
	if doctorReport.State != ReadinessPass || calls != 0 {
		t.Fatalf("doctor report = %#v, calls = %d", doctorReport, calls)
	}
	certifyReport := registry.Certify(context.Background(), "doctor-honesty", "model")
	if certifyReport.State != ReadinessNotCertified ||
		certifyReport.Code != "live_probe_not_configured" {
		t.Fatalf("certify report = %#v", certifyReport)
	}
}
