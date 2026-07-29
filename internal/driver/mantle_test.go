//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBedrockMantleAPIKeyUsesOpenAIChatCodecAndSharedToolLoop(t *testing.T) {
	invocationID := "mantle-api-key"
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
			request.Header.Get("Authorization") != "Bearer mantle-secret-canary" ||
			request.Header.Get("X-Amz-Date") != "" {
			t.Errorf("Mantle API-key request path=%s headers=%#v", request.URL.Path, request.Header)
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
			t.Errorf("Mantle Chat Completions request = %s", body)
		}
		if turn == 1 {
			writeJSONResponse(t, writer, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"tool_calls": []any{openAIToolCallFixture(
							"mantle-read",
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
			envelope.Messages[2].ToolCallID != "mantle-read" ||
			string(envelope.Messages[2].Content) != `"bounded input"` {
			t.Errorf("Mantle continuation = %s", body)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"mantle-submit",
			"sworn_submit",
			submitArguments,
			5,
			7,
		))
	}))
	defer server.Close()

	var returnedSecret []byte
	adapter, err := NewBedrockMantleAdapter(
		BedrockMantleProfileConfig{
			Key: "mantle-api", ID: "sworn.bedrock.mantle.api", Version: "1.0.0",
			Endpoint:       server.URL + "/v1/chat/completions",
			CredentialRefs: []string{"credential-ref"},
			ResponseBytes:  MaxProviderResponseBytes,
			AuthMode:       BedrockMantleAPIKey,
		},
		func(context.Context, string) ([]byte, error) {
			returnedSecret = []byte("mantle-secret-canary")
			return returnedSecret, nil
		},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	invocation := productionInvocationFixture(
		t,
		adapter,
		ProfileBedrock,
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
			"Mantle API-key observation=%#v requests=%d error=%v",
			observation,
			requests.Load(),
			err,
		)
	}
	if !bytes.Equal(returnedSecret, make([]byte, len(returnedSecret))) {
		t.Fatalf("Mantle API key was not cleared: %q", returnedSecret)
	}
	report := profileReportFixture(t, adapter, "mantle-api")
	if report.Family != ProfileBedrock ||
		report.Surface != ProfileSurfaceBedrockMantleChat {
		t.Fatalf("Mantle API-key report = %#v", report)
	}
	body, _ := json.Marshal(observation)
	if bytes.Contains(body, []byte("mantle-secret-canary")) {
		t.Fatalf("Mantle secret escaped observation: %s", body)
	}
}

func TestBedrockMantleAWSModeSignsChatCompletionsWithoutFallback(t *testing.T) {
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
	invocationID := "mantle-aws"
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
			!strings.Contains(authorization, "Credential=AKIAMANTLE12345/") ||
			!strings.Contains(authorization, "/bedrock-mantle/aws4_request") ||
			strings.Contains(authorization, "/bedrock/aws4_request") ||
			request.Header.Get("X-Amz-Content-Sha256") == "" ||
			request.Header.Get("Api-Key") != "" ||
			!bytes.Contains(body, []byte(`"messages"`)) ||
			!bytes.Contains(body, []byte(`"tools"`)) {
			t.Errorf(
				"Mantle AWS request path=%s auth=%q headers=%#v body=%s",
				request.URL.Path,
				authorization,
				request.Header,
				body,
			)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"mantle-aws-submit",
			"sworn_submit",
			submissionToolArguments(t, submission),
			13,
			17,
		))
	}))
	defer server.Close()

	var environment [][]byte
	adapter, err := NewBedrockMantleAdapter(
		BedrockMantleProfileConfig{
			Key: "mantle-aws", ID: "sworn.bedrock.mantle.aws", Version: "1.0.0",
			Endpoint:       server.URL + "/v1/chat/completions",
			CredentialRefs: []string{"credential-ref"},
			ResponseBytes:  MaxProviderResponseBytes,
			AuthMode:       BedrockMantleAWS,
			Chain:          &chain,
		},
		nil,
		func(context.Context, string) ([][]byte, error) {
			environment = [][]byte{
				[]byte("AWS_ACCESS_KEY_ID=AKIAMANTLE12345"),
				[]byte("AWS_SECRET_ACCESS_KEY=mantle-aws-secret"),
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
	expiration := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	transport.runAWS = func(
		_ context.Context,
		_ AWSChainSpec,
		_ [][]byte,
		arguments ...string,
	) ([]byte, error) {
		if strings.Join(arguments, " ") ==
			"configure export-credentials --format process" {
			return []byte(
				`{"Version":1,"AccessKeyId":"AKIAMANTLE12345","SecretAccessKey":"mantle-aws-secret","Expiration":"` +
					expiration + `"}`,
			), nil
		}
		return []byte(awsEnvironmentTable), nil
	}
	invocation := productionInvocationFixture(
		t,
		adapter,
		ProfileBedrock,
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
			"Mantle AWS observation=%#v requests=%d error=%v",
			observation,
			requests.Load(),
			err,
		)
	}
	for _, entry := range environment {
		if !bytes.Equal(entry, make([]byte, len(entry))) {
			t.Fatalf("Mantle AWS environment not cleared: %q", entry)
		}
	}
	report := profileReportFixture(t, adapter, "mantle-aws")
	if report.Family != ProfileBedrock ||
		report.Surface != ProfileSurfaceBedrockMantleChat {
		t.Fatalf("Mantle AWS report = %#v", report)
	}
	observationBody, _ := json.Marshal(observation)
	for _, secret := range []string{"AKIAMANTLE12345", "mantle-aws-secret"} {
		if bytes.Contains(observationBody, []byte(secret)) {
			t.Fatalf("Mantle AWS secret escaped observation: %s", observationBody)
		}
	}

	if _, err := NewBedrockMantleAdapter(
		BedrockMantleProfileConfig{
			Key: "invalid-api", ID: "sworn.invalid.api", Version: "1.0.0",
			Endpoint:       server.URL + "/v1/chat/completions",
			CredentialRefs: []string{"credential-ref"},
			ResponseBytes:  MaxProviderResponseBytes,
			AuthMode:       BedrockMantleAPIKey,
		},
		func(context.Context, string) ([]byte, error) { return nil, nil },
		func(context.Context, string) ([][]byte, error) { return nil, nil },
		nil,
		nil,
	); !IsCode(err, "INVALID_ADAPTER") {
		t.Fatalf("API-key auth accepted AWS fallback: %v", err)
	}
	if _, err := NewBedrockMantleAdapter(
		BedrockMantleProfileConfig{
			Key: "invalid-aws", ID: "sworn.invalid.aws", Version: "1.0.0",
			Endpoint:       server.URL + "/v1/chat/completions",
			CredentialRefs: []string{"credential-ref"},
			ResponseBytes:  MaxProviderResponseBytes,
			AuthMode:       BedrockMantleAWS,
			Chain:          &chain,
		},
		func(context.Context, string) ([]byte, error) { return nil, nil },
		func(context.Context, string) ([][]byte, error) { return nil, nil },
		nil,
		nil,
	); !IsCode(err, "INVALID_ADAPTER") {
		t.Fatalf("AWS auth accepted API-key fallback: %v", err)
	}
	if _, err := NewBedrockMantleAdapter(
		BedrockMantleProfileConfig{
			Key: "invalid-endpoint", ID: "sworn.invalid.endpoint", Version: "1.0.0",
			Endpoint: server.URL, CredentialRefs: []string{"credential-ref"},
			ResponseBytes: MaxProviderResponseBytes,
			AuthMode:      BedrockMantleAPIKey,
		},
		func(context.Context, string) ([]byte, error) { return nil, nil },
		nil,
		nil,
		nil,
	); !IsCode(err, "INVALID_ADAPTER") {
		t.Fatalf("Mantle base URL was accepted as the POST endpoint: %v", err)
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
