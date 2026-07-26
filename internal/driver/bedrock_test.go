//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBedrockConverseReplaysCompleteAdmittedAssistantMessage(t *testing.T) {
	t.Parallel()
	config := BedrockProfileConfig{
		Endpoint:        "https://bedrock-runtime.us-east-1.amazonaws.com",
		AllowCachePoint: true, AllowGuardContent: true,
	}
	conversation, err := newBedrockConversation(
		config,
		"exact-model",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	signature := base64.StdEncoding.EncodeToString([]byte("reasoning-signature"))
	response := []byte(`{
	  "output":{"message":{"role":"assistant","content":[
	    {"reasoningContent":{"reasoningText":{"text":"private reasoning","signature":"` + signature + `"}}},
	    {"cachePoint":{"type":"default"}},
	    {"guardContent":{"text":{"text":"guarded"}}},
	    {"toolUse":{"toolUseId":"tool-1","name":"Read","input":{"path":"/workspace/a.txt"}}}
	  ]}},
	  "stopReason":"tool_use",
	  "usage":{"inputTokens":17,"outputTokens":19,"totalTokens":36}
	}`)
	turn, err := conversation.accept(response)
	if err != nil || len(turn.Calls) != 1 ||
		turn.Calls[0].ID != "tool-1" ||
		turn.Calls[0].Name != "Read" ||
		turn.Usage == nil || turn.Usage.InputTokens != 17 ||
		turn.Usage.OutputTokens != 19 {
		t.Fatalf("turn = %#v, error=%v", turn, err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "tool-1", Name: "Read", Content: []byte("file body"),
	}}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var replay struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if json.Unmarshal(request.Body, &replay) != nil || len(replay.Messages) != 3 ||
		!bytes.Contains(replay.Messages[1], []byte(`"signature":"`+signature+`"`)) ||
		!bytes.Contains(replay.Messages[1], []byte(`"cachePoint"`)) ||
		!bytes.Contains(replay.Messages[1], []byte(`"guardContent"`)) ||
		!bytes.Contains(replay.Messages[2], []byte(`"toolUseId":"tool-1"`)) {
		t.Fatalf("Bedrock replay = %s", request.Body)
	}

	for name, invalid := range map[string][]byte{
		"unknown union":     []byte(`{"output":{"message":{"role":"assistant","content":[{"providerExtension":{}}]}},"stopReason":"tool_use"}`),
		"missing signature": []byte(`{"output":{"message":{"role":"assistant","content":[{"reasoningContent":{"reasoningText":{"text":"x"}}},{"toolUse":{"toolUseId":"x","name":"Read","input":{}}}]}},"stopReason":"tool_use"}`),
		"unknown stop":      []byte(`{"output":{"message":{"role":"assistant","content":[{"toolUse":{"toolUseId":"x","name":"Read","input":{}}}]}},"stopReason":"end_turn"}`),
	} {
		fresh, freshErr := newBedrockConversation(
			config,
			"exact-model",
			toolDefinitions(ReadOnly),
			[]byte(`{}`),
		)
		if freshErr != nil {
			t.Fatal(freshErr)
		}
		_, acceptErr := fresh.accept(invalid)
		fresh.close()
		if acceptErr == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestBedrockStandardChainFakeServerSignsWithoutPersistingSecrets(t *testing.T) {
	const awsPath = "/usr/local/aws-cli/v2/2.35.9/dist/aws"
	if _, err := os.Stat(awsPath); err != nil {
		t.Skip("exact AWS CLI fixture unavailable")
	}
	invocationID := "bedrock-standard-chain"
	submission := submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	)
	arguments := submissionToolArguments(t, submission)
	var argumentValue map[string]any
	if json.Unmarshal([]byte(arguments), &argumentValue) != nil {
		t.Fatal("invalid submission argument fixture")
	}
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
		if request.URL.Path != "/model/exact-model/converse" ||
			!strings.Contains(authorization, "Credential=AKIAEXAMPLE1234/") ||
			strings.Contains(authorization, "secret-example-value") ||
			bytes.Contains(body, []byte("secret-example-value")) {
			t.Errorf("Bedrock request = %s, auth=%q, body=%s", request.URL, authorization, body)
		}
		writeJSONResponse(t, writer, map[string]any{
			"output": map[string]any{"message": map[string]any{
				"role": "assistant",
				"content": []any{map[string]any{
					"toolUse": map[string]any{
						"toolUseId": "bedrock-submit",
						"name":      "sworn_submit",
						"input":     argumentValue,
					},
				}},
			}},
			"stopReason": "tool_use",
			"usage": map[string]any{
				"inputTokens": 23, "outputTokens": 29, "totalTokens": 52,
			},
		})
	}))
	defer server.Close()
	spec := AWSChainSpec{
		CLI:              ExecutableIdentity{Path: awsPath, Digest: AWSCLIDigest},
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
	var environment [][]byte
	resolver := func(context.Context, string) ([][]byte, error) {
		environment = [][]byte{
			[]byte("AWS_ACCESS_KEY_ID=AKIAEXAMPLE1234"),
			[]byte("AWS_SECRET_ACCESS_KEY=secret-example-value"),
			[]byte("AWS_REGION=ap-southeast-2"),
			[]byte("AWS_DEFAULT_REGION=ap-southeast-2"),
		}
		return environment, nil
	}
	adapter, err := NewBedrockAdapter(
		BedrockProfileConfig{
			Key: "bedrock-adapter", ID: "sworn.bedrock", Version: "1.0.0",
			Endpoint:       server.URL,
			CredentialRefs: []string{"credential-ref"},
			ResponseBytes:  MaxProviderResponseBytes,
			Chain:          spec,
		},
		resolver,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	loop := adapter.(*loopAdapter)
	transport := loop.transport.(*bedrockTransport)
	expiration := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	var chainCalls atomic.Int64
	transport.runAWS = func(
		_ context.Context,
		_ AWSChainSpec,
		_ [][]byte,
		arguments ...string,
	) ([]byte, error) {
		chainCalls.Add(1)
		if strings.Join(arguments, " ") ==
			"configure export-credentials --format process" {
			return []byte(
				`{"Version":1,"AccessKeyId":"AKIAEXAMPLE1234","SecretAccessKey":"secret-example-value","Expiration":"` +
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
	if err != nil || observation.Handoff == nil || requests.Load() != 1 ||
		chainCalls.Load() != 3 ||
		observation.Usage.InputTokens == nil ||
		*observation.Usage.InputTokens != 23 ||
		observation.Usage.OutputTokens == nil ||
		*observation.Usage.OutputTokens != 29 {
		t.Fatalf(
			"Bedrock observation = %#v, requests=%d, chain=%d, error=%v",
			observation,
			requests.Load(),
			chainCalls.Load(),
			err,
		)
	}
	for _, entry := range environment {
		if !bytes.Equal(entry, make([]byte, len(entry))) {
			t.Fatalf("AWS environment not cleared: %q", entry)
		}
	}
	observationBody, _ := json.Marshal(observation)
	for _, secret := range [][]byte{
		[]byte("AKIAEXAMPLE1234"),
		[]byte("secret-example-value"),
	} {
		if bytes.Contains(observationBody, secret) {
			t.Fatalf("AWS secret escaped observation: %s", observationBody)
		}
	}
}
