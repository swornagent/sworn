//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBedrockSigningServiceIsClosedBySurface(t *testing.T) {
	t.Parallel()
	for surface, expected := range map[ProfileSurface]string{
		ProfileSurfaceBedrockRuntimeConverse: "bedrock",
		ProfileSurfaceBedrockMantleChat:      "bedrock-mantle",
		ProfileSurface("user-configured"):    "",
	} {
		if actual := bedrockSigningService(surface); actual != expected {
			t.Fatalf("surface %q service = %q, want %q", surface, actual, expected)
		}
	}
}

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
	assistant := append([]byte(nil), conversation.messages[1]...)
	if err := conversation.resume(
		[]byte(`{"invocation_id":"implementation"}`),
		toolDefinitions(ReadWrite),
	); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(conversation.messages[1], assistant) {
		t.Fatal("Bedrock resume changed the admitted assistant message")
	}
	request, err = conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	var resumed struct {
		Messages   []json.RawMessage `json:"messages"`
		ToolConfig struct {
			Tools []bedrockTool `json:"tools"`
		} `json:"toolConfig"`
	}
	if json.Unmarshal(request.Body, &resumed) != nil ||
		len(resumed.Messages) != 3 {
		t.Fatalf("Bedrock resumed replay = %s", request.Body)
	}
	var finalUser struct {
		Role    string `json:"role"`
		Content []struct {
			ToolResult *struct {
				ToolUseID string `json:"toolUseId"`
				Content   []struct {
					Text string `json:"text"`
				} `json:"content"`
				Status string `json:"status"`
			} `json:"toolResult"`
			Text *string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(resumed.Messages[2], &finalUser) != nil ||
		finalUser.Role != "user" ||
		len(finalUser.Content) != 2 ||
		finalUser.Content[0].ToolResult == nil ||
		finalUser.Content[0].ToolResult.ToolUseID != "tool-1" ||
		len(finalUser.Content[0].ToolResult.Content) != 1 ||
		finalUser.Content[0].ToolResult.Content[0].Text != "file body" ||
		finalUser.Content[0].ToolResult.Status != "success" ||
		finalUser.Content[1].Text == nil ||
		*finalUser.Content[1].Text !=
			`{"invocation_id":"implementation"}` {
		t.Fatalf("Bedrock resumed user message = %s", resumed.Messages[2])
	}
	hasWrite := false
	for _, tool := range resumed.ToolConfig.Tools {
		hasWrite = hasWrite || tool.ToolSpec.Name == "Write"
	}
	if !hasWrite {
		t.Fatalf("Bedrock resumed tools = %s", request.Body)
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

type bedrockTerminalTestTransport struct {
	response []byte
	request  providerRequest
}

func (transport *bedrockTerminalTestTransport) roundTrip(
	_ context.Context,
	_ *string,
	request providerRequest,
) ([]byte, error) {
	transport.request = request
	transport.request.Body = append([]byte(nil), request.Body...)
	return append([]byte(nil), transport.response...), nil
}

func (*bedrockTerminalTestTransport) check(
	context.Context,
	profileCheckKind,
	*string,
	string,
) (ReadinessState, string) {
	return ReadinessPass, "test"
}

func bedrockTerminalTestAdapter(
	t *testing.T,
	transport providerTransport,
) *loopAdapter {
	t.Helper()
	config := BedrockProfileConfig{
		Endpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
	}
	adapter, err := newLoopAdapter(
		"bedrock-terminal-test",
		"sworn.bedrock.terminal-test",
		"1.0.0",
		ProfileBedrock,
		ProfileSurfaceBedrockRuntimeConverse,
		providerDialectBedrockConverse,
		map[string]string{"fixture": "bedrock-terminal"},
		func(
			prompt []byte,
			model string,
			definitions []providerToolDefinition,
			_ Limits,
		) (providerConversation, error) {
			return newBedrockConversation(
				config,
				model,
				definitions,
				prompt,
			)
		},
		transport,
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func bedrockTerminalTestSelected(adapter Adapter) SelectedProfile {
	credential := "credential-ref"
	profile := ProfileConfig{
		Key:           "bedrock-terminal-profile",
		Adapter:       adapter.Identity().Key,
		Network:       NetworkRequired,
		CredentialRef: &credential,
	}
	return SelectedProfile{
		Profile: profile,
		Adapter: adapter.Identity(),
		Model:   "exact-model",
		adapter: adapter,
	}
}

func bedrockTerminalTestResponse(
	t *testing.T,
	id string,
	name string,
	input any,
) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"output": map[string]any{"message": map[string]any{
			"role": "assistant",
			"content": []any{map[string]any{
				"toolUse": map[string]any{
					"toolUseId": id,
					"name":      name,
					"input":     input,
				},
			}},
		}},
		"stopReason": "tool_use",
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func bedrockTerminalRequest(
	t *testing.T,
	request providerRequest,
) (string, []string) {
	t.Helper()
	var body struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		ToolConfig struct {
			Tools []bedrockTool `json:"tools"`
		} `json:"toolConfig"`
	}
	if json.Unmarshal(request.Body, &body) != nil ||
		len(body.System) != 1 {
		t.Fatalf("Bedrock terminal request = %s", request.Body)
	}
	names := make([]string, len(body.ToolConfig.Tools))
	for index, tool := range body.ToolConfig.Tools {
		names[index] = tool.ToolSpec.Name
	}
	return body.System[0].Text, names
}

func TestBedrockAutomationUsesItsExactRecoveryAndAdvisoryTerminal(
	t *testing.T,
) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		terminal   string
		invocation func(SelectedProfile) AutomationInvocation
		input      any
	}{
		{
			name:     "recovery",
			terminal: "sworn_recovery_decide",
			invocation: func(selected SelectedProfile) AutomationInvocation {
				selection := ModelSelection{
					Profile: selected.Profile.Key,
					Model:   selected.Model,
				}
				return AutomationInvocation{
					Selected: selected,
					Recovery: pointerTo(
						recoveryInvocationFixture(selection),
					),
				}
			},
			input: map[string]any{"decision": RecoveryDecision{
				SchemaVersion: RecoveryDecisionSchemaVersion,
				InvocationID:  "recovery-1",
				Action:        RecoveryAskCaptain,
			}},
		},
		{
			name:     "advisory",
			terminal: "sworn_advisory_respond",
			invocation: func(selected SelectedProfile) AutomationInvocation {
				selection := ModelSelection{
					Profile: selected.Profile.Key,
					Model:   selected.Model,
				}
				return AutomationInvocation{
					Selected: selected,
					Advisory: &AdvisoryInvocation{
						SchemaVersion: AdvisoryInvocationSchemaVersion,
						InvocationID:  "advisory-1",
						Binding:       automationBindingFixture(),
						Selection:     selection,
						Question:      "Can the admitted facts answer this?",
						Facts: []AutomationFact{{
							Name: FactCurrentStatus, Value: "bounded",
						}},
					},
				}
			},
			input: map[string]any{"result": AdvisoryResult{
				SchemaVersion: AdvisoryResultSchemaVersion,
				InvocationID:  "advisory-1",
				Outcome:       AdvisoryCannotAnswer,
			}},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			transport := &bedrockTerminalTestTransport{}
			adapter := bedrockTerminalTestAdapter(t, transport)
			selected := bedrockTerminalTestSelected(adapter)
			invocation := test.invocation(selected)
			transport.response = bedrockTerminalTestResponse(
				t,
				test.name+"-call",
				test.terminal,
				test.input,
			)
			observation, err := (Dispatcher{}).InvokeAutomation(
				context.Background(),
				invocation,
			)
			if err != nil ||
				(observation.Recovery == nil) ==
					(observation.Advisory == nil) {
				t.Fatalf(
					"Bedrock %s observation = %#v, error=%v",
					test.name,
					observation,
					err,
				)
			}
			system, names := bedrockTerminalRequest(
				t,
				transport.request,
			)
			expected := "Use only the supplied Sworn tools and terminate with exactly one call to " +
				test.terminal + "."
			if system != expected ||
				len(names) != 1 ||
				names[0] != test.terminal ||
				strings.Contains(system, "sworn_submit") {
				t.Fatalf(
					"Bedrock %s system=%q tools=%q",
					test.name,
					system,
					names,
				)
			}
		})
	}
}

func TestBedrockWorkerSurfaceAdvertisesAndAcceptsYield(t *testing.T) {
	t.Parallel()
	const invocationID = "bedrock-worker-yield"
	yield := Yield{
		SchemaVersion: YieldSchemaVersion,
		InvocationID:  invocationID,
		Kind:          YieldQuestion,
		Message:       "Which exact prepared base should I use?",
	}
	transport := &bedrockTerminalTestTransport{
		response: bedrockTerminalTestResponse(
			t,
			"yield-call",
			"sworn_yield",
			map[string]any{"yield": yield},
		),
	}
	adapter := bedrockTerminalTestAdapter(t, transport)
	invocation := productionInvocationFixture(
		t,
		adapter,
		ProfileBedrock,
		invocationID,
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
	)
	observation, err := (Dispatcher{}).Invoke(
		context.Background(),
		invocation,
	)
	if err != nil || observation.Yield == nil ||
		*observation.Yield != yield || observation.Handoff != nil {
		t.Fatalf("Bedrock yield observation = %#v, error=%v", observation, err)
	}
	system, names := bedrockTerminalRequest(t, transport.request)
	expected := "Use only the supplied Sworn tools and terminate with exactly one call to sworn_submit or sworn_yield."
	if system != expected ||
		!slicesContain(names, "sworn_submit") ||
		!slicesContain(names, "sworn_yield") {
		t.Fatalf("Bedrock worker system=%q tools=%q", system, names)
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
			!strings.Contains(authorization, "/bedrock/aws4_request") ||
			strings.Contains(authorization, "/bedrock-mantle/aws4_request") ||
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
				"serverToolUsage": map[string]any{},
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
	var chainCalls atomic.Int64
	transport.runAWS = func(
		_ context.Context,
		_ AWSChainSpec,
		_ [][]byte,
		_ ...string,
	) ([]byte, error) {
		chainCalls.Add(1)
		return nil, errors.New("AWS CLI must not run for direct environment credentials")
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
		chainCalls.Load() != 0 ||
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
