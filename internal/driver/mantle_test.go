//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAIChatBearerUsesOpenAIChatCodecAndSharedToolLoop(t *testing.T) {
	invocationID := "openai-chat-bearer"
	submission := submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	)
	submitArguments := submissionToolArguments(t, submission)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		turn := requests.Add(1)
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Error(err)
			return
		}
		if request.URL.Path != "/v1/chat/completions" ||
			request.Header.Get("Authorization") != "Bearer openai-secret-canary" ||
			request.Header.Get("X-Amz-Date") != "" {
			t.Errorf(
				"OpenAI chat bearer request path=%s headers=%#v",
				request.URL.Path,
				request.Header,
			)
		}
		var envelope struct {
			Model    string          `json:"model"`
			Messages []openAIMessage `json:"messages"`
			Tools    []openAITool    `json:"tools"`
		}
		if json.Unmarshal(body, &envelope) != nil ||
			envelope.Model != "exact-model" ||
			len(envelope.Messages) == 0 ||
			len(envelope.Tools) == 0 {
			t.Errorf("OpenAI Chat Completions request = %s", body)
		}
		if turn == 1 {
			writeJSONResponse(t, writer, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"reasoning": "bounded response-only reasoning",
						"tool_calls": []any{openAIToolCallFixture(
							"openai-chat-read",
							"Read",
							`{"path":"/workspace/input.txt"}`,
						)},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{
					"prompt_tokens": 2, "completion_tokens": 3,
				},
			})
			return
		}
		if len(envelope.Messages) != 3 ||
			envelope.Messages[2].Role != "tool" ||
			envelope.Messages[2].ToolCallID != "openai-chat-read" ||
			string(envelope.Messages[2].Content) != `"bounded input"` ||
			bytes.Contains(body, []byte(`"reasoning"`)) {
			t.Errorf("OpenAI chat continuation = %s", body)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"openai-chat-submit",
			"sworn_submit",
			submitArguments,
			5,
			7,
		))
	}))
	defer server.Close()

	var returnedSecret []byte
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-chat", ID: "sworn.openai.chat", Version: "1.0.0",
				Endpoint:         server.URL + "/v1/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API: OpenAIChatCompletionsAPI,
		},
		func(context.Context, string) ([]byte, error) {
			returnedSecret = []byte("openai-secret-canary")
			return returnedSecret, nil
		},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	loop := adapter.(*loopAdapter)
	if loop.dialect != providerDialectOpenAIChat ||
		loop.dialect.continuationMode() != ContinuationModeTranscriptReplay ||
		loop.surface != ProfileSurfaceOpenAIChat {
		t.Fatalf("OpenAI chat continuation identity = %#v", loop)
	}
	invocation := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	if err := osWriteProviderFixture(
		invocation.HostWorkspace,
		"input.txt",
		"bounded input",
	); err != nil {
		t.Fatal(err)
	}
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil ||
		requests.Load() != 2 ||
		observation.Usage.InputTokens == nil ||
		*observation.Usage.InputTokens != 7 ||
		observation.Usage.OutputTokens == nil ||
		*observation.Usage.OutputTokens != 10 {
		t.Fatalf(
			"OpenAI chat bearer observation=%#v requests=%d error=%v",
			observation,
			requests.Load(),
			err,
		)
	}
	if !bytes.Equal(returnedSecret, make([]byte, len(returnedSecret))) {
		t.Fatalf("OpenAI chat key was not cleared: %q", returnedSecret)
	}
	report := profileReportFixture(t, adapter, "openai-chat")
	if report.Family != ProfileOpenAIHTTP ||
		report.Surface != ProfileSurfaceOpenAIChat {
		t.Fatalf("OpenAI chat bearer report = %#v", report)
	}
	body, _ := json.Marshal(observation)
	if bytes.Contains(body, []byte("openai-secret-canary")) {
		t.Fatalf("OpenAI chat secret escaped observation: %s", body)
	}
}

func TestOpenAIChatAWSSigV4SignsChatCompletionsWithoutFallback(t *testing.T) {
	root := t.TempDir()
	awsPath := filepath.Join(root, "aws")
	if err := os.WriteFile(awsPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runtimeFiles := driverRuntimeFilesFixture(t, root)
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	chain := AWSChainSpec{
		CLI:              ExecutableIdentity{Path: awsPath, Digest: AWSCLIDigest},
		CLIVersion:       AWSCLIVersion,
		Region:           "ap-southeast-2",
		RegionSource:     AWSSourceEnvironment,
		CredentialSource: AWSSourceEnvironment,
		EnvironmentKeys: []string{
			"AWS_ACCESS_KEY_ID", "AWS_DEFAULT_REGION",
			"AWS_REGION", "AWS_SECRET_ACCESS_KEY",
		},
		RuntimeFiles:           runtimeFiles,
		RequiredRuntimeTargets: required,
	}
	invocationID := "openai-chat-aws"
	submission := submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		requests.Add(1)
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Error(err)
			return
		}
		authorization := request.Header.Get("Authorization")
		if request.URL.Path != "/v1/chat/completions" ||
			request.Header.Get("X-Amz-Date") == "" ||
			!strings.Contains(authorization, "Credential=AKIAOPENAI12345/") ||
			!strings.Contains(authorization, "/bedrock-mantle/aws4_request") ||
			strings.Contains(authorization, "/bedrock/aws4_request") ||
			request.Header.Get("X-Amz-Content-Sha256") == "" ||
			request.Header.Get("Api-Key") != "" ||
			!bytes.Contains(body, []byte(`"messages"`)) ||
			!bytes.Contains(body, []byte(`"tools"`)) {
			t.Errorf(
				"OpenAI chat AWS request path=%s auth=%q headers=%#v body=%s",
				request.URL.Path,
				authorization,
				request.Header,
				body,
			)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"openai-chat-aws-submit",
			"sworn_submit",
			submissionToolArguments(t, submission),
			13,
			17,
		))
	}))
	defer server.Close()

	var environment [][]byte
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-chat-aws", ID: "sworn.openai.chat.aws",
				Version:        "1.0.0",
				Endpoint:       server.URL + "/v1/chat/completions",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:      OpenAIChatCompletionsAPI,
			AuthMode: AuthModeAWSSigV4,
			Chain:    &chain,
		},
		nil,
		func(context.Context, string) ([][]byte, error) {
			environment = [][]byte{
				[]byte("AWS_ACCESS_KEY_ID=AKIAOPENAI12345"),
				[]byte("AWS_SECRET_ACCESS_KEY=openai-chat-aws-secret"),
				[]byte("AWS_REGION=ap-southeast-2"),
				[]byte("AWS_DEFAULT_REGION=ap-southeast-2"),
			}
			return environment, nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	transport := adapter.(*loopAdapter).transport.(*bedrockTransport)
	transport.runAWS = func(
		_ context.Context,
		_ AWSChainSpec,
		_ [][]byte,
		_ ...string,
	) ([]byte, error) {
		return nil, errors.New("AWS CLI must not run for direct environment credentials")
	}
	invocation := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil ||
		requests.Load() != 1 ||
		observation.Usage.InputTokens == nil ||
		*observation.Usage.InputTokens != 13 ||
		observation.Usage.OutputTokens == nil ||
		*observation.Usage.OutputTokens != 17 {
		t.Fatalf(
			"OpenAI chat AWS observation=%#v requests=%d error=%v",
			observation,
			requests.Load(),
			err,
		)
	}
	for _, entry := range environment {
		if !bytes.Equal(entry, make([]byte, len(entry))) {
			t.Fatalf("OpenAI chat AWS environment not cleared: %q", entry)
		}
	}
	loop := adapter.(*loopAdapter)
	if loop.surface != ProfileSurfaceOpenAIChat {
		t.Fatalf("OpenAI chat AWS reported surface = %#v", loop)
	}
	report := profileReportFixture(t, adapter, "openai-chat-aws")
	if report.Family != ProfileOpenAIHTTP ||
		report.Surface != ProfileSurfaceOpenAIChat {
		t.Fatalf("OpenAI chat AWS report = %#v", report)
	}
	observationBody, _ := json.Marshal(observation)
	for _, secret := range []string{"AKIAOPENAI12345", "openai-chat-aws-secret"} {
		if bytes.Contains(observationBody, []byte(secret)) {
			t.Fatalf("OpenAI chat AWS secret escaped observation: %s", observationBody)
		}
	}

	if _, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "invalid-bearer", ID: "sworn.invalid.bearer", Version: "1.0.0",
				Endpoint:       server.URL + "/v1/chat/completions",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:      OpenAIChatCompletionsAPI,
			AuthMode: AuthModeBearer,
			Chain:    &chain,
		},
		func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
		nil,
		nil,
		nil,
	); !IsCode(err, "INVALID_ADAPTER") {
		t.Fatalf("bearer auth accepted an AWS chain: %v", err)
	}
	if _, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "invalid-aws", ID: "sworn.invalid.aws", Version: "1.0.0",
				Endpoint:       server.URL + "/v1/chat/completions",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:      OpenAIChatCompletionsAPI,
			AuthMode: AuthModeAWSSigV4,
		},
		nil,
		func(context.Context, string) ([][]byte, error) { return nil, nil },
		nil,
		nil,
	); !IsCode(err, "INVALID_ADAPTER") {
		t.Fatalf("SigV4 auth accepted a missing AWS chain: %v", err)
	}
	if _, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "invalid-endpoint", ID: "sworn.invalid.endpoint", Version: "1.0.0",
				Endpoint:       server.URL + "?api-version=1",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:      OpenAIChatCompletionsAPI,
			AuthMode: AuthModeBearer,
		},
		func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
		nil,
		nil,
		nil,
	); !IsCode(err, "INVALID_ADAPTER") {
		t.Fatalf("query-string endpoint was accepted as the POST endpoint: %v", err)
	}
}

func TestOpenAIChatEndpointAdmissionRestsOnURLShapeNotPathSuffix(t *testing.T) {
	for _, test := range []struct {
		name     string
		endpoint string
		valid    bool
	}{
		{"DeepSeek-style chat path without v1", "https://api.example.invalid/chat/completions", true},
		{"Gemini-style v1beta openai path", "https://generativelanguage.example.invalid/v1beta/openai/chat/completions", true},
		{"Canonical v1 chat path", "https://api.example.invalid/v1/chat/completions", true},
		{"Bare host is a valid exact POST URL", "https://api.example.invalid", true},
		{"Query string is rejected", "https://api.example.invalid/chat/completions?key=value", false},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			adapter, err := NewOpenAIAdapter(
				OpenAIProfileConfig{
					HTTPProfileConfig: HTTPProfileConfig{
						Key: "openai-paths", ID: "sworn.openai.paths",
						Version: "1.0.0", Endpoint: test.endpoint,
						CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
						CredentialRefs: []string{"credential-ref"},
						ResponseBytes:  MaxProviderResponseBytes,
					},
					API: OpenAIChatCompletionsAPI,
				},
				func(context.Context, string) ([]byte, error) {
					return []byte("secret"), nil
				},
				nil,
				nil,
				nil,
			)
			if test.valid && err != nil {
				t.Fatalf("endpoint %q rejected: %v", test.endpoint, err)
			}
			if !test.valid && !IsCode(err, "INVALID_ADAPTER") {
				t.Fatalf("endpoint %q error = %v, want INVALID_ADAPTER", test.endpoint, err)
			}
			if adapter != nil {
				loop := adapter.(*loopAdapter)
				if loop.dialect != providerDialectOpenAIChat ||
					loop.surface != ProfileSurfaceOpenAIChat {
					t.Fatalf("OpenAI chat path surface = %#v", loop)
				}
			}
		})
	}
}

func profileReportFixture(
	t *testing.T,
	adapter Adapter,
	profile string,
) ProfileReport {
	t.Helper()
	ref := "credential-ref"
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: profile, Adapter: adapter.Identity().Key,
			Network: NetworkRequired, CredentialRef: &ref,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry.Inspect(context.Background(), profile, "exact-model")
}
