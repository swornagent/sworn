//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAPIContinuationAppendsAcceptedResultEnvelopeAndFreshTools(t *testing.T) {
	const designID = "api-continuation-design"
	const implementationID = "api-continuation-implementation"
	designSubmission := submissionFixture(
		t,
		designID,
		ImplementerDesign,
		"",
	)
	implementationSubmission := submissionFixture(
		t,
		implementationID,
		ImplementerImplementation,
		"",
	)
	var requests atomic.Int64
	adapter := apiContinuationChatAdapter(t, func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		turn := requests.Add(1)
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Error(err)
			return
		}
		var envelope struct {
			Messages []openAIMessage `json:"messages"`
			Tools    []openAITool    `json:"tools"`
		}
		if json.Unmarshal(body, &envelope) != nil {
			t.Errorf("request = %s", body)
			return
		}
		toolNames := make(map[string]struct{}, len(envelope.Tools))
		for _, tool := range envelope.Tools {
			toolNames[tool.Function.Name] = struct{}{}
		}
		if turn == 1 {
			if len(envelope.Messages) != 1 {
				t.Errorf("design messages = %s", body)
			}
			if _, writable := toolNames["Write"]; writable {
				t.Errorf("design tools admitted Write: %s", body)
			}
			writeJSONResponse(t, writer, openAIToolCallResponse(
				"design-submit",
				"sworn_submit",
				submissionToolArguments(t, designSubmission),
				2,
				3,
			))
			return
		}
		if turn != 2 || len(envelope.Messages) != 4 ||
			envelope.Messages[0].Role != "user" ||
			envelope.Messages[1].Role != "assistant" ||
			envelope.Messages[2].Role != "tool" ||
			envelope.Messages[2].ToolCallID != "design-submit" ||
			string(envelope.Messages[2].Content) != `"accepted"` ||
			envelope.Messages[3].Role != "user" {
			t.Errorf("resume order = %s", body)
		}
		if _, writable := toolNames["Write"]; !writable {
			t.Errorf("resume tools omitted Write: %s", body)
		}
		if _, writable := toolNames["Edit"]; !writable {
			t.Errorf("resume tools omitted Edit: %s", body)
		}
		var prompt string
		if json.Unmarshal(envelope.Messages[3].Content, &prompt) != nil {
			t.Errorf("implementation prompt = %s", envelope.Messages[3].Content)
		}
		var modelEnvelope struct {
			InvocationID string  `json:"invocation_id"`
			Inputs       []Input `json:"inputs"`
		}
		if json.Unmarshal([]byte(prompt), &modelEnvelope) != nil ||
			modelEnvelope.InvocationID != implementationID ||
			len(modelEnvelope.Inputs) != 1 ||
			modelEnvelope.Inputs[0].Name != "captain-receipt" {
			t.Errorf("implementation envelope = %s", prompt)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"implementation-submit",
			"sworn_submit",
			submissionToolArguments(t, implementationSubmission),
			5,
			7,
		))
	})

	design := apiContinuationInvocation(
		t,
		adapter,
		designID,
		ImplementerDesign,
		ReadOnly,
		true,
		nil,
	)
	observation, handle, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		design,
		continuationContractBinding(),
		nil,
	)
	if err != nil || observation.Handoff == nil || handle == nil ||
		result.Mode != ContinuationModeTranscriptReplay ||
		result.Status != ContinuationStatusSuspended ||
		requests.Load() != 1 {
		t.Fatalf(
			"design observation=%#v handle=%p result=%#v requests=%d error=%v",
			observation,
			handle,
			result,
			requests.Load(),
			err,
		)
	}
	cell := continuationCellFor(handle)
	cell.mu.Lock()
	state, ok := cell.state.(*apiContinuationState)
	if !ok {
		actual := cell.state
		cell.mu.Unlock()
		t.Fatalf("suspended state = %#v", actual)
	}
	conversation, conversationOK := state.conversation.(*openAIConversation)
	if !conversationOK || len(conversation.messages) != 3 ||
		conversation.messages[2].Role != "tool" ||
		string(conversation.messages[2].Content) != `"accepted"` {
		cell.mu.Unlock()
		t.Fatalf("suspended state = %#v", state)
	}
	accepted := conversation.messages[2].Content
	cell.mu.Unlock()

	receipt := []byte(`{"decision":"proceed"}`)
	input := Input{
		Name:   "captain-receipt",
		Path:   "captain/review.json",
		Digest: Digest(receipt),
	}
	implementation := apiContinuationInvocation(
		t,
		adapter,
		implementationID,
		ImplementerImplementation,
		ReadWrite,
		false,
		[]InputContent{{Input: input, Bytes: receipt}},
	)
	observation, next, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		implementation,
		continuationContractBinding(),
		handle,
	)
	if err != nil || observation.Handoff == nil || next != nil ||
		result.Mode != ContinuationModeTranscriptReplay ||
		result.Status != ContinuationStatusResumed ||
		requests.Load() != 2 {
		t.Fatalf(
			"resume observation=%#v next=%p result=%#v requests=%d error=%v",
			observation,
			next,
			result,
			requests.Load(),
			err,
		)
	}
	if state.conversation != nil || state.bytes != 0 ||
		state.mode != "" || state.dialect != "" || !state.closed ||
		!bytes.Equal(accepted, make([]byte, len(accepted))) {
		t.Fatalf("continuation state was not zeroed: %#v", state)
	}
}

func TestAPIContinuationInvalidPreflightFallsBackBeforeEffects(t *testing.T) {
	for _, test := range []struct {
		name    string
		id      string
		corrupt func(*openAIConversation) []byte
	}{
		{
			name: "complete request exceeds one MiB",
			id:   "api-preflight-size-design",
			corrupt: func(conversation *openAIConversation) []byte {
				oversized, _ := json.Marshal(
					strings.Repeat("x", MaxProviderRequestBytes),
				)
				clearBytes(conversation.messages[0].Content)
				conversation.messages[0].Content = oversized
				return oversized
			},
		},
		{
			name: "tool result correlation is invalid",
			id:   "api-preflight-correlation-design",
			corrupt: func(conversation *openAIConversation) []byte {
				conversation.messages[len(conversation.messages)-1].
					ToolCallID = "wrong-call"
				return nil
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			designID := test.id
			var requests atomic.Int64
			adapter := apiContinuationChatAdapter(t, func(
				writer http.ResponseWriter,
				_ *http.Request,
			) {
				if requests.Add(1) != 1 {
					t.Error("invalid continuation reached provider transport")
				}
				submission := submissionFixture(
					t,
					designID,
					ImplementerDesign,
					"",
				)
				writeJSONResponse(t, writer, openAIToolCallResponse(
					"preflight-submit",
					"sworn_submit",
					submissionToolArguments(t, submission),
					1,
					1,
				))
			})
			design := apiContinuationInvocation(
				t,
				adapter,
				designID,
				ImplementerDesign,
				ReadOnly,
				true,
				nil,
			)
			_, handle, _, err := (Dispatcher{}).InvokeTurn(
				context.Background(),
				design,
				continuationContractBinding(),
				nil,
			)
			if err != nil || handle == nil {
				t.Fatalf("design handle=%p error=%v", handle, err)
			}
			cell := continuationCellFor(handle)
			cell.mu.Lock()
			state := cell.state.(*apiContinuationState)
			conversation := state.conversation.(*openAIConversation)
			retained := test.corrupt(conversation)
			cell.mu.Unlock()

			implementation := apiContinuationInvocation(
				t,
				adapter,
				"api-preflight-implementation",
				ImplementerImplementation,
				ReadWrite,
				false,
				nil,
			)
			observation, next, result, err := (Dispatcher{}).InvokeTurn(
				context.Background(),
				implementation,
				continuationContractBinding(),
				handle,
			)
			if err != nil || next != nil ||
				!equalObservation(observation, Observation{}) ||
				result.Mode != ContinuationModeFreshRehydrate ||
				result.Status != ContinuationStatusMismatch ||
				requests.Load() != 1 {
				t.Fatalf(
					"fallback observation=%#v next=%p result=%#v requests=%d error=%v",
					observation,
					next,
					result,
					requests.Load(),
					err,
				)
			}
			if !state.closed || state.conversation != nil {
				t.Fatalf("invalid state was not closed: %#v", state)
			}
			if retained != nil &&
				!bytes.Equal(retained, make([]byte, len(retained))) {
				t.Fatal("invalid state bytes were not cleared")
			}
		})
	}
}

func TestAPIContinuationPostTransportContractFailureDoesNotFallback(t *testing.T) {
	const designID = "api-post-effect-design"
	var requests atomic.Int64
	adapter := apiContinuationChatAdapter(t, func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		if requests.Add(1) == 1 {
			submission := submissionFixture(
				t,
				designID,
				ImplementerDesign,
				"",
			)
			writeJSONResponse(t, writer, openAIToolCallResponse(
				"post-effect-submit",
				"sworn_submit",
				submissionToolArguments(t, submission),
				1,
				1,
			))
			return
		}
		writeJSONResponse(t, writer, map[string]any{
			"choices": []any{},
		})
	})
	design := apiContinuationInvocation(
		t,
		adapter,
		designID,
		ImplementerDesign,
		ReadOnly,
		true,
		nil,
	)
	_, handle, _, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		design,
		continuationContractBinding(),
		nil,
	)
	if err != nil || handle == nil {
		t.Fatalf("design handle=%p error=%v", handle, err)
	}
	implementation := apiContinuationInvocation(
		t,
		adapter,
		"api-post-effect-implementation",
		ImplementerImplementation,
		ReadWrite,
		false,
		nil,
	)
	_, next, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		implementation,
		continuationContractBinding(),
		handle,
	)
	if !IsCode(err, "PROTOCOL_FAILURE") || next != nil ||
		result.Mode != ContinuationModeTranscriptReplay ||
		result.Status != ContinuationStatusResumed ||
		requests.Load() != 2 {
		t.Fatalf(
			"post-effect next=%p result=%#v requests=%d error=%v",
			next,
			result,
			requests.Load(),
			err,
		)
	}
}

func apiContinuationChatAdapter(
	t *testing.T,
	handler http.HandlerFunc,
) Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "api-continuation", ID: "sworn.api.continuation",
				Version: "1.0.0", Endpoint: server.URL + "/v1/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API: OpenAIChatCompletionsAPI,
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("credential"), nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func apiContinuationInvocation(
	t *testing.T,
	adapter Adapter,
	invocationID string,
	responsibility Responsibility,
	access WorkspaceAccess,
	fresh bool,
	contents []InputContent,
) Invocation {
	t.Helper()
	ref := "credential-ref"
	profile := ProfileConfig{
		Key: "api-continuation-profile", Adapter: adapter.Identity().Key,
		Network: NetworkRequired, CredentialRef: &ref,
	}
	selected := SelectedProfile{
		Profile: profile, Adapter: adapter.Identity(), Model: "exact-model",
		adapter: adapter,
	}
	inputs := make([]Input, len(contents))
	for index := range contents {
		inputs[index] = contents[index].Input
	}
	request, err := NewRequest(
		invocationID,
		RoleImplementer,
		profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: access},
		inputs,
		fresh,
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	containment := ContainmentReadWrite
	if access == ReadOnly {
		containment = ContainmentReadOnly
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		containment,
		responsibility,
	)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request: request, HostWorkspace: t.TempDir(),
		Selected: selected, Permission: permission,
		Inputs: contents,
	}
}

func equalObservation(left, right Observation) bool {
	leftBody, _ := json.Marshal(left)
	rightBody, _ := json.Marshal(right)
	return bytes.Equal(leftBody, rightBody)
}
