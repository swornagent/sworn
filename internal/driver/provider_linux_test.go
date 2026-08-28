//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAIResponsesFakeServerCorpusCoversEveryRole(t *testing.T) {
	tests := []struct {
		name           string
		role           Role
		responsibility Responsibility
		decision       DecisionOutcome
		access         WorkspaceAccess
	}{
		{"planner", RolePlanner, PlannerProposal, "", ReadWrite},
		{"implementer", RoleImplementer, ImplementerImplementation, "", ReadWrite},
		{"captain", RoleCaptain, CaptainReview, DecisionProceed, ReadOnly},
		{"verifier", RoleVerifier, WorkVerification, DecisionPass, ReadOnly},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var requestCount atomic.Int64
			var returnedSecret []byte
			submission := submissionFixture(
				t,
				"provider-"+test.name,
				test.responsibility,
				test.decision,
			)
			arguments := submissionToolArguments(t, submission)
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				requestCount.Add(1)
				if request.Header.Get("Authorization") != "Bearer credential-canary" ||
					request.Header.Get("Content-Type") != "application/json" {
					t.Errorf("headers = %#v", request.Header)
				}
				var body struct {
					Model     string `json:"model"`
					Input     []any  `json:"input"`
					Store     *bool  `json:"store"`
					Stream    *bool  `json:"stream"`
					Reasoning struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
					Tools []struct {
						Name   string `json:"name"`
						Strict bool   `json:"strict"`
					} `json:"tools"`
				}
				if json.NewDecoder(request.Body).Decode(&body) != nil ||
					body.Model != "exact-model" ||
					len(body.Input) != 1 ||
					body.Store == nil || *body.Store ||
					body.Stream == nil || *body.Stream ||
					body.Reasoning.Effort != "medium" ||
					request.URL.Path != "/v1/responses" {
					t.Errorf("provider request = %#v", body)
				}
				names := make(map[string]struct{}, len(body.Tools))
				for _, tool := range body.Tools {
					if tool.Strict {
						t.Errorf("strict tool = %#v", tool)
					}
					names[tool.Name] = struct{}{}
				}
				_, hasWrite := names["Write"]
				_, hasEdit := names["Edit"]
				if (test.access == ReadWrite) != hasWrite ||
					(test.access == ReadWrite) != hasEdit {
					t.Errorf("access-derived tools = %#v", names)
				}
				writeJSONResponse(t, writer, responsesToolCallResponse(
					"response-1",
					"function-1",
					"submit-1",
					"sworn_submit",
					arguments,
					7,
					5,
				))
			}))
			defer server.Close()
			resolver := func(context.Context, string) ([]byte, error) {
				returnedSecret = []byte("credential-canary")
				return returnedSecret, nil
			}
			adapter, err := NewOpenAIAdapter(
				OpenAIProfileConfig{
					HTTPProfileConfig: HTTPProfileConfig{
						Key: "openai-adapter", ID: "sworn.openai", Version: "1.0.0",
						Endpoint:         server.URL + "/v1/responses",
						CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
						CredentialRefs: []string{"credential-ref"},
						ResponseBytes:  MaxProviderResponseBytes,
					},
					API:             OpenAIResponsesAPI,
					ReasoningEffort: "medium",
				},
				resolver,
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
				ProfileOpenAIHTTP,
				"provider-"+test.name,
				test.role,
				test.responsibility,
				test.access,
			)
			if test.responsibility == PlannerProposal {
				invocation.recoverableInput = &RecoverableTurnInput{
					SchemaVersion: RecoverableTurnInputSchemaVersion,
					Kind:          RecoverableInputAnswer,
					Answer:        "Continue with the approved planner turn.",
				}
			}
			observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
			if err != nil || observation.Handoff == nil ||
				observation.TransportStatus != Completed ||
				observation.Usage.TokenStatus != UsageReported ||
				observation.Usage.InputTokens == nil ||
				*observation.Usage.InputTokens != 7 ||
				observation.Usage.OutputTokens == nil ||
				*observation.Usage.OutputTokens != 5 ||
				requestCount.Load() != 1 {
				t.Fatalf("observation = %#v, requests=%d, error=%v", observation, requestCount.Load(), err)
			}
			if !bytes.Equal(returnedSecret, make([]byte, len(returnedSecret))) {
				t.Fatalf("resolver-owned secret not cleared: %q", returnedSecret)
			}
			observationBody, _ := json.Marshal(observation)
			if bytes.Contains(observationBody, []byte("credential-canary")) {
				t.Fatalf("secret escaped observation: %s", observationBody)
			}
		})
	}
}

func TestProviderWorkerYieldIsTerminalWithoutSealedBatonAuthority(t *testing.T) {
	t.Parallel()
	const invocationID = "provider-yield"
	arguments, err := json.Marshal(map[string]any{
		"yield": Yield{
			SchemaVersion: YieldSchemaVersion,
			InvocationID:  invocationID,
			Kind:          YieldQuestion,
			Message:       "Which exact prepared base should I use?",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if json.NewDecoder(request.Body).Decode(&body) != nil {
			t.Error("invalid provider request")
		}
		names := make(map[string]struct{}, len(body.Tools))
		for _, tool := range body.Tools {
			names[tool.Name] = struct{}{}
		}
		if _, ok := names["sworn_submit"]; !ok {
			t.Error("submission terminal missing")
		}
		if _, ok := names["sworn_yield"]; !ok {
			t.Error("yield terminal missing")
		}
		writeJSONResponse(t, writer, responsesToolCallResponse(
			"yield-response",
			"yield-function",
			"yield-call",
			"sworn_yield",
			string(arguments),
			3,
			2,
		))
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-yield", ID: "sworn.openai.yield", Version: "1.0.0",
				Endpoint:         server.URL + "/v1/responses",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:             OpenAIResponsesAPI,
			ReasoningEffort: "none",
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("secret"), nil
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
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Yield == nil ||
		observation.Handoff != nil ||
		observation.Yield.InvocationID != invocationID ||
		observation.Yield.Kind != YieldQuestion {
		t.Fatalf("observation = %#v, error = %v", observation, err)
	}
}

func TestProviderMalformedSubmissionsAreCorrectedUntilValid(
	t *testing.T,
) {
	t.Parallel()
	const invocationID = "provider-submit-corrections"
	const correctionRounds = 5
	valid := submissionToolArguments(t, submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	))
	var turns atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		turn := turns.Add(1)
		arguments := `{"submission":{}}`
		if turn == int64(correctionRounds+1) {
			arguments = valid
		}
		writeJSONResponse(t, writer, responsesToolCallResponse(
			"correction-response-"+itoa(int(turn)),
			"correction-function-"+itoa(int(turn)),
			"correction-call-"+itoa(int(turn)),
			"sworn_submit",
			arguments,
			1,
			1,
		))
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-corrections", ID: "sworn.openai.corrections",
				Version: "1.0.0", Endpoint: server.URL + "/v1/responses",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:             OpenAIResponsesAPI,
			ReasoningEffort: "none",
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("secret"), nil
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
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	var reservations atomic.Int64
	invocation.RecoveryStepHook = func(
		_ context.Context,
		kind RecoveryStepKind,
	) error {
		if kind != RecoveryStepSubmissionCorrection {
			t.Fatalf("reservation kind = %s", kind)
		}
		reservations.Add(1)
		return nil
	}
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil ||
		observation.Yield != nil ||
		turns.Load() != int64(correctionRounds+1) ||
		reservations.Load() != int64(correctionRounds) {
		t.Fatalf(
			"observation = %#v, turns=%d, error=%v",
			observation,
			turns.Load(),
			err,
		)
	}
}

// toolCallCorrectionTransport serves a malformed Responses function_call for
// a fixed number of turns, then a well-formed sworn_submit reusing the exact
// same call_id: it exercises A1's transactional ledger commit (a failed
// decode never leaves an earlier turn's call_id permanently claimed) end to
// end, through the real adapter loop, without any real network connection.
type toolCallCorrectionTransport struct {
	turn             atomic.Int64
	correctionRounds int64
	validArguments   string
}

func (transport *toolCallCorrectionTransport) roundTrip(
	_ context.Context,
	_ *string,
	_ providerRequest,
) ([]byte, error) {
	turn := transport.turn.Add(1)
	arguments := "{malformed json"
	if turn > transport.correctionRounds {
		arguments = transport.validArguments
	}
	return mustJSONMap(responsesToolCallResponse(
		"toolcall-response",
		"toolcall-function",
		"toolcall-call",
		"sworn_submit",
		arguments,
		1,
		1,
	)), nil
}

func (*toolCallCorrectionTransport) check(
	context.Context,
	profileCheckKind,
	*string,
	string,
) (ReadinessState, string) {
	return ReadinessPass, "test"
}

// toolCallCorrectionTestAdapter builds a real Responses-dialect loop adapter
// whose transport is replaced by the test script, mirroring
// economyTestAdapter's shape for the chat-completions dialect.
func toolCallCorrectionTestAdapter(
	t *testing.T,
	transport providerTransport,
) Adapter {
	t.Helper()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-toolcall", ID: "sworn.openai.toolcall",
				Version:          "1.0.0",
				Endpoint:         "https://provider.example.invalid/v1/responses",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:             OpenAIResponsesAPI,
			ReasoningEffort: "none",
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("secret"), nil
		},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	loop, ok := adapter.(*loopAdapter)
	if !ok {
		t.Fatalf("adapter type = %T", adapter)
	}
	loop.transport = transport
	return adapter
}

func TestProviderMalformedToolCallsAreCorrectedUntilValid(t *testing.T) {
	t.Parallel()
	const invocationID = "provider-toolcall-corrections"
	const correctionRounds = 3
	valid := submissionToolArguments(t, submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	))
	// The same call_id, item id, and response id are reused on every turn,
	// including the corrected one: the transactional ledger commit (A1)
	// never leaves an earlier, still-uncorrected turn's call_id claimed, so
	// the model reusing it is not a defect.
	transport := &toolCallCorrectionTransport{
		correctionRounds: correctionRounds,
		validArguments:   valid,
	}
	adapter := toolCallCorrectionTestAdapter(t, transport)
	invocation := economyInvocationFixture(
		t,
		adapter,
		invocationID,
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536},
		t.TempDir(),
	)
	var reservations atomic.Int64
	invocation.RecoveryStepHook = func(
		_ context.Context,
		kind RecoveryStepKind,
	) error {
		if kind != RecoveryStepMalformedToolCall {
			t.Fatalf("reservation kind = %s", kind)
		}
		reservations.Add(1)
		return nil
	}
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil ||
		observation.Yield != nil ||
		transport.turn.Load() != int64(correctionRounds+1) ||
		reservations.Load() != int64(correctionRounds) {
		t.Fatalf(
			"observation = %#v, turns=%d, error=%v",
			observation,
			transport.turn.Load(),
			err,
		)
	}
}

// TestProviderPersistentMalformedToolCallsPreserveOriginalClassification
// pins the Captain's required correction: with the operator's turn budget
// set to the maximum admitted value (equal to MaxProviderTurns), a provider
// that never emits a well-formed tool call must still fail with the
// original continuation.toolcall_decode.* classification once
// MaxToolCallCorrections is spent - never a generic RESOURCE_LIMIT from the
// runaway-loop guard's own iteration bound, and never a different economy
// code either.
func TestProviderPersistentMalformedToolCallsPreserveOriginalClassification(
	t *testing.T,
) {
	t.Parallel()
	const invocationID = "provider-toolcall-persistent"
	transport := &toolCallCorrectionTransport{
		// Never crosses into "valid": correctionRounds names the last turn
		// index MaxProviderTurns can ever reach, so every one of them stays
		// malformed.
		correctionRounds: MaxProviderTurns,
		validArguments:   "unused",
	}
	adapter := toolCallCorrectionTestAdapter(t, transport)
	invocation := economyInvocationFixture(
		t,
		adapter,
		invocationID,
		Limits{
			// MaxProviderTurns in-process round trips run comfortably
			// inside the default 5s TimeoutMillis, but -race's
			// instrumentation overhead and loaded CI runners can push
			// that far past the edge (a 30s budget expired on CI as
			// INVOCATION_TIMEOUT before the correction rounds finished);
			// a generous budget keeps this a correctness test, not a
			// scheduler timing test.
			TimeoutMillis:   180_000,
			OutputBytes:     65_536,
			MaxTurnsPerWork: MaxTurnsPerWorkLimit,
		},
		t.TempDir(),
	)
	var reservations atomic.Int64
	invocation.RecoveryStepHook = func(
		_ context.Context,
		kind RecoveryStepKind,
	) error {
		if kind != RecoveryStepMalformedToolCall {
			t.Fatalf("reservation kind = %s", kind)
		}
		reservations.Add(1)
		return nil
	}
	_, invokeErr := (Dispatcher{}).Invoke(context.Background(), invocation)
	// finishAdapterInvocation's normalizeAdapterError strips Detail from
	// every non-provider-status code before it reaches this boundary (by
	// design: "adapter-provided wrapping text cannot escape"), so the
	// classification is checked at the Code alone here; the exact
	// continuation.toolcall_decode.* label surviving up to the point of
	// normalization is pinned directly against responsesFunctionCall and
	// accept() in continuation_labels_test.go and continuation_test.go.
	// What this asserts is the Captain's required correction: persistent
	// malformation must still fail CONTINUATION_INVALID, never the
	// runaway-loop guard's own RESOURCE_LIMIT and never an economy code.
	if !IsCode(invokeErr, "CONTINUATION_INVALID") {
		t.Fatalf(
			"expected CONTINUATION_INVALID (not RESOURCE_LIMIT or an economy code), got: %v",
			invokeErr,
		)
	}
	if transport.turn.Load() != int64(MaxProviderTurns) {
		t.Fatalf(
			"requests = %d, want %d",
			transport.turn.Load(),
			MaxProviderTurns,
		)
	}
	if reservations.Load() != int64(MaxToolCallCorrections) {
		t.Fatalf(
			"reservations = %d, want %d",
			reservations.Load(),
			MaxToolCallCorrections,
		)
	}
}

func TestProviderProseNudgesFlowUntilCompletionOrTurnBudget(t *testing.T) {
	tests := []struct {
		name             string
		refuse           bool
		secondProse      bool
		wantRequests     int64
		wantReservations int64
		wantCode         string
	}{
		{name: "submit_after_nudge", wantRequests: 2, wantReservations: 1},
		{
			name: "reservation_refused", refuse: true,
			wantRequests: 1, wantReservations: 1,
			wantCode: "RECOVERY_STEP_REFUSED",
		},
		// A model that answers in prose forever is nudged every turn, each
		// nudge durably reserved as eval data, until the per-work turn
		// budget - the first bound - ends the invocation with the named
		// economy code.
		{
			name: "prose_forever_is_nudged_to_the_turn_budget", secondProse: true,
			wantRequests:     int64(DefaultMaxTurnsPerWork),
			wantReservations: int64(DefaultMaxTurnsPerWork),
			wantCode:         "ECONOMY_TURN_BUDGET_EXCEEDED",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			const invocationID = "provider-prose-nudge"
			valid := submissionToolArguments(t, submissionFixture(
				t,
				invocationID,
				ImplementerImplementation,
				"",
			))
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				turn := requests.Add(1)
				var body struct {
					Input []map[string]any `json:"input"`
				}
				if json.NewDecoder(request.Body).Decode(&body) != nil {
					t.Error("invalid provider request")
				}
				if turn == 2 {
					last := body.Input[len(body.Input)-1]
					if last["role"] != "user" ||
						last["content"] != providerProseNudge {
						t.Errorf("nudge = %#v", last)
					}
				}
				if turn == 1 || test.secondProse {
					writeJSONResponse(t, writer, map[string]any{
						"status": "completed",
						"output": []any{map[string]any{
							"type": "message", "role": "assistant",
							"status": "completed",
							"content": []any{map[string]any{
								"type": "output_text",
								"text": "I have finished.",
							}},
						}},
						"usage": map[string]any{
							"input_tokens": 1, "output_tokens": 1,
							"total_tokens": 2,
						},
					})
					return
				}
				writeJSONResponse(t, writer, responsesToolCallResponse(
					"nudge-response",
					"nudge-function",
					"nudge-call",
					"sworn_submit",
					valid,
					1,
					1,
				))
			}))
			defer server.Close()
			adapter, err := NewOpenAIAdapter(
				OpenAIProfileConfig{
					HTTPProfileConfig: HTTPProfileConfig{
						Key: "openai-prose", ID: "sworn.openai.prose",
						Version:          "1.0.0",
						Endpoint:         server.URL + "/v1/responses",
						CredentialHeader: "Authorization",
						CredentialPrefix: "Bearer ",
						CredentialRefs:   []string{"credential-ref"},
						ResponseBytes:    MaxProviderResponseBytes,
					},
					API: OpenAIResponsesAPI, ReasoningEffort: "none",
				},
				func(context.Context, string) ([]byte, error) {
					return []byte("secret"), nil
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
				ProfileOpenAIHTTP,
				invocationID,
				RoleImplementer,
				ImplementerImplementation,
				ReadWrite,
			)
			var reservations atomic.Int64
			invocation.RecoveryStepHook = func(
				_ context.Context,
				kind RecoveryStepKind,
			) error {
				if kind != RecoveryStepProseNudge {
					t.Fatalf("reservation kind = %s", kind)
				}
				reservations.Add(1)
				if test.refuse {
					return fail("TEST_REFUSAL")
				}
				return nil
			}
			observation, invokeErr :=
				(Dispatcher{}).Invoke(context.Background(), invocation)
			if requests.Load() != test.wantRequests ||
				reservations.Load() != test.wantReservations {
				t.Fatalf(
					"requests=%d reservations=%d",
					requests.Load(),
					reservations.Load(),
				)
			}
			if test.wantCode == "" {
				if invokeErr != nil || observation.Handoff == nil {
					t.Fatalf(
						"observation=%#v error=%v",
						observation,
						invokeErr,
					)
				}
			} else if !IsCode(invokeErr, test.wantCode) {
				t.Fatalf("error=%v, want %s", invokeErr, test.wantCode)
			}
		})
	}
}

func TestProviderLoopPreservesParallelToolResultOrder(t *testing.T) {
	invocationID := "provider-tool-order"
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
		var chatEnvelope struct {
			ReasoningEffort string `json:"reasoning_effort"`
		}
		if json.Unmarshal(body, &chatEnvelope) != nil ||
			chatEnvelope.ReasoningEffort != "none" {
			t.Errorf("chat reasoning effort = %s", body)
		}
		if turn == 1 {
			writeJSONResponse(t, writer, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"tool_calls": []any{
							openAIToolCallFixture("read-a", "Read", `{"path":"/workspace/a.txt"}`),
							openAIToolCallFixture("read-b", "Read", `{"path":"/workspace/b.txt"}`),
						},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3},
			})
			return
		}
		var requestBody struct {
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
				Content    string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &requestBody) != nil ||
			len(requestBody.Messages) != 4 ||
			requestBody.Messages[2].ToolCallID != "read-a" ||
			requestBody.Messages[2].Content != "alpha" ||
			requestBody.Messages[3].ToolCallID != "read-b" ||
			requestBody.Messages[3].Content != "beta" {
			t.Errorf("second request order = %s", body)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"submit-final",
			"sworn_submit",
			submitArguments,
			5,
			7,
		))
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-order", ID: "sworn.openai.order", Version: "1.0.0",
				Endpoint:         server.URL + "/v1/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:             OpenAIChatCompletionsAPI,
			ReasoningEffort: "none",
		},
		func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
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
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	if err := osWriteProviderFixture(invocation.HostWorkspace, "a.txt", "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := osWriteProviderFixture(invocation.HostWorkspace, "b.txt", "beta"); err != nil {
		t.Fatal(err)
	}
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil || requests.Load() != 2 ||
		observation.Usage.InputTokens == nil ||
		*observation.Usage.InputTokens != 7 ||
		observation.Usage.OutputTokens == nil ||
		*observation.Usage.OutputTokens != 10 {
		t.Fatalf("observation = %#v, requests=%d, error=%v", observation, requests.Load(), err)
	}
}

func TestProviderChatCompletionsCarriesDeclaredReasoningEffort(t *testing.T) {
	const invocationID = "provider-chat-effort"
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
		requests.Add(1)
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Error(err)
			return
		}
		var chatEnvelope struct {
			ReasoningEffort string `json:"reasoning_effort"`
		}
		if json.Unmarshal(body, &chatEnvelope) != nil ||
			chatEnvelope.ReasoningEffort != "high" {
			t.Errorf("chat reasoning effort = %s", body)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"chat-effort-submit",
			"sworn_submit",
			submitArguments,
			5,
			7,
		))
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-chat-effort", ID: "sworn.openai.chat.effort",
				Version: "1.0.0", Endpoint: server.URL + "/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:              OpenAIChatCompletionsAPI,
			ReasoningEffort:  "high",
			ReasoningEfforts: []string{"high", "max"},
		},
		func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
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
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil || requests.Load() != 1 ||
		observation.Usage.InputTokens == nil ||
		*observation.Usage.InputTokens != 5 ||
		observation.Usage.OutputTokens == nil ||
		*observation.Usage.OutputTokens != 7 {
		t.Fatalf(
			"observation = %#v, requests=%d, error=%v",
			observation,
			requests.Load(),
			err,
		)
	}
}

func TestHTTPTransportDoesNotRetryRedirectOrPublishVerdictOnFailure(t *testing.T) {
	for _, test := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "provider error",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				http.Error(writer, "credential-canary", http.StatusInternalServerError)
			},
		},
		{
			name: "redirect",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				http.Redirect(writer, request, "/unexpected", http.StatusTemporaryRedirect)
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				requests.Add(1)
				test.handler(writer, request)
			}))
			defer server.Close()
			adapter, err := NewOpenAIAdapter(
				OpenAIProfileConfig{
					HTTPProfileConfig: HTTPProfileConfig{
						Key: "failure-adapter", ID: "sworn.failure", Version: "1.0.0",
						Endpoint:         server.URL + "/v1/chat/completions",
						CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
						CredentialRefs: []string{"credential-ref"},
						ResponseBytes:  MaxProviderResponseBytes,
					},
					API: OpenAIChatCompletionsAPI,
				},
				func(context.Context, string) ([]byte, error) {
					return []byte("credential-canary"), nil
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
				ProfileOpenAIHTTP,
				"provider-failure",
				RoleImplementer,
				ImplementerImplementation,
				ReadWrite,
			)
			observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
			if err == nil || observation.Handoff != nil ||
				observation.TransportStatus != RunnerError ||
				requests.Load() != 1 ||
				strings.Contains(err.Error(), "credential-canary") {
				t.Fatalf("failure observation = %#v, requests=%d, error=%v", observation, requests.Load(), err)
			}
			body, _ := json.Marshal(observation)
			if bytes.Contains(body, []byte("credential-canary")) {
				t.Fatalf("secret escaped failure observation: %s", body)
			}
		})
	}
}

func TestOpenAIOpaqueChatAndGeminiFakeServersPreserveTheirWireContracts(t *testing.T) {
	t.Run("opaque chat reasoning replay", func(t *testing.T) {
		invocationID := "opaque-chat-replay"
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
			turn := requests.Add(1)
			body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
			if err != nil {
				t.Error(err)
				return
			}
			if turn == 1 {
				writeJSONResponse(t, writer, map[string]any{
					"choices": []any{map[string]any{
						"message": map[string]any{
							"role": "assistant", "content": nil,
							"reasoning_content": "provider-private-reasoning",
							"tool_calls": []any{openAIToolCallFixture(
								"opaque-chat-read",
								"Read",
								`{"path":"/workspace/input.txt"}`,
							)},
						},
						"finish_reason": "tool_calls",
					}},
				})
				return
			}
			if !bytes.Contains(body, []byte(`"reasoning_content":"provider-private-reasoning"`)) ||
				!bytes.Contains(body, []byte(`"tool_call_id":"opaque-chat-read"`)) {
				t.Errorf("opaque chat continuation not replayed: %s", body)
			}
			writeJSONResponse(t, writer, openAIToolCallResponse(
				"opaque-chat-submit",
				"sworn_submit",
				submissionToolArguments(t, submission),
				3,
				4,
			))
		}))
		defer server.Close()
		opaque := true
		adapter, err := NewOpenAIAdapter(
			OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "opaque-chat-adapter", ID: "sworn.opaque.chat",
					Version:          "1.0.0",
					Endpoint:         server.URL + "/chat/completions",
					CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
					CredentialRefs: []string{"credential-ref"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				API:             OpenAIChatCompletionsAPI,
				OpaqueReasoning: &opaque,
			},
			func(context.Context, string) ([]byte, error) {
				return []byte("secret"), nil
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
		if err != nil || observation.Handoff == nil || requests.Load() != 2 {
			t.Fatalf("opaque chat observation = %#v, error=%v", observation, err)
		}
		observationBody, _ := json.Marshal(observation)
		if bytes.Contains(observationBody, []byte("provider-private-reasoning")) {
			t.Fatalf("reasoning escaped observation: %s", observationBody)
		}
	})

	t.Run("Gemini generateContent", func(t *testing.T) {
		invocationID := "gemini-generate-content"
		submission := submissionFixture(
			t,
			invocationID,
			ImplementerImplementation,
			"",
		)
		arguments := submissionToolArguments(t, submission)
		var argumentValue map[string]any
		if json.Unmarshal([]byte(arguments), &argumentValue) != nil {
			t.Fatal("invalid Gemini arguments fixture")
		}
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			if request.URL.Path != "/v1beta/models/exact-model:generateContent" ||
				request.Header.Get("x-goog-api-key") != "gemini-secret" {
				t.Errorf("Gemini request = %s, headers=%#v", request.URL, request.Header)
			}
			body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
			if err != nil ||
				!bytes.Contains(body, []byte(`"parameters":`)) ||
				bytes.Contains(body, []byte(`"parametersJsonSchema"`)) {
				t.Errorf("Gemini JSON Schema field = %s, error=%v", body, err)
			}
			writeJSONResponse(t, writer, map[string]any{
				"candidates": []any{map[string]any{
					"content": map[string]any{
						"role": "model",
						"parts": []any{map[string]any{
							"functionCall": map[string]any{
								"id": "gemini-submit", "name": "sworn_submit",
								"args": argumentValue,
							},
						}},
					},
					"finishReason":  "STOP",
					"finishMessage": "tool call completed",
				}},
				"usageMetadata": map[string]any{
					"promptTokenCount": 11, "candidatesTokenCount": 13,
					"totalTokenCount": 24, "serviceTier": "STANDARD",
				},
			})
		}))
		defer server.Close()
		adapter, err := NewGeminiAdapter(
			HTTPProfileConfig{
				Key: "gemini-adapter", ID: "sworn.gemini", Version: "1.0.0",
				Endpoint:         server.URL,
				CredentialHeader: "x-goog-api-key", CredentialPrefix: "",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			func(context.Context, string) ([]byte, error) {
				return []byte("gemini-secret"), nil
			},
			nil,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		invocation := productionInvocationFixture(
			t,
			adapter,
			ProfileGemini,
			invocationID,
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
		)
		observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
		if err != nil || observation.Handoff == nil || requests.Load() != 1 ||
			observation.Usage.InputTokens == nil ||
			*observation.Usage.InputTokens != 11 ||
			observation.Usage.OutputTokens == nil ||
			*observation.Usage.OutputTokens != 13 {
			t.Fatalf("Gemini observation = %#v, error=%v", observation, err)
		}
	})
}

func productionInvocationFixture(
	t *testing.T,
	adapter Adapter,
	family ProfileFamily,
	invocationID string,
	role Role,
	responsibility Responsibility,
	access WorkspaceAccess,
) Invocation {
	t.Helper()
	ref := "credential-ref"
	profile := ProfileConfig{
		Key: "production-profile", Adapter: adapter.Identity().Key,
		Network: NetworkRequired, CredentialRef: &ref,
	}
	selected := SelectedProfile{
		Profile: profile, Adapter: adapter.Identity(), Model: "exact-model",
		adapter: adapter,
	}
	checker, ok := adapter.(profileChecker)
	if !ok || checker.profileFamily() != family {
		t.Fatalf("adapter family = %T", adapter)
	}
	request, err := NewRequest(
		invocationID,
		role,
		profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: access},
		nil,
		true,
		// The turn budget, not the clock, is what these fixtures exist to
		// pin: a work that runs DefaultMaxTurnsPerWork in-process round
		// trips fits comfortably in 5s unloaded, but -race instrumentation
		// on a loaded CI runner does not (200 turns reached only 190 before
		// the 5s budget expired). Match the generous budget the economy
		// fixture already uses for the same reason, so a wall clock never
		// races the behaviour under test.
		Limits{TimeoutMillis: 180_000, OutputBytes: 65_536},
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
	}
}

func ioReadAllBounded(reader io.Reader, maximum int) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, int64(maximum)+1))
	if err != nil || len(body) > maximum {
		return nil, fail("RESOURCE_LIMIT")
	}
	return body, nil
}

func osWriteProviderFixture(root, name, body string) error {
	return os.WriteFile(filepath.Join(root, name), []byte(body), 0o600)
}

func submissionToolArguments(t *testing.T, submission Submission) string {
	t.Helper()
	body, err := EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if json.Unmarshal(body, &value) != nil {
		t.Fatal("submission fixture is not JSON")
	}
	arguments, err := json.Marshal(map[string]any{"submission": value})
	if err != nil {
		t.Fatal(err)
	}
	return string(arguments)
}

func openAIToolCallResponse(
	id string,
	name string,
	arguments string,
	inputTokens int64,
	outputTokens int64,
) map[string]any {
	return map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []any{openAIToolCallFixture(id, name, arguments)},
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{
			"prompt_tokens": inputTokens, "completion_tokens": outputTokens,
		},
	}
}

func openAIToolCallFixture(id, name, arguments string) map[string]any {
	return map[string]any{
		"id": id, "type": "function",
		"function": map[string]any{"name": name, "arguments": arguments},
	}
}

func responsesToolCallResponse(
	responseID string,
	itemID string,
	callID string,
	name string,
	arguments string,
	inputTokens int64,
	outputTokens int64,
) map[string]any {
	return map[string]any{
		"id": responseID, "object": "response", "status": "completed",
		"error": nil,
		"output": []any{map[string]any{
			"type": "function_call", "id": itemID, "call_id": callID,
			"name": name, "arguments": arguments, "status": "completed",
		}},
		"usage": map[string]any{
			"input_tokens": inputTokens, "output_tokens": outputTokens,
			"total_tokens": inputTokens + outputTokens,
		},
		"future_inert_metadata": map[string]any{"ignored": true},
	}
}

func writeJSONResponse(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Error(err)
	}
}

// S3-provider-observability acceptance evidence: cache accounting, requested
// vs reported reasoning effort, honest absence, and truncation-as-named-failure
// are all driven through the real adapter loop with fake loopback providers.

func TestOpenAIChatSurfacesCacheAccountingAndReportedEffort(t *testing.T) {
	const invocationID = "provider-chat-cache-effort"
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
		requests.Add(1)
		writeJSONResponse(t, writer, map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role": "assistant", "content": nil,
					"tool_calls": []any{openAIToolCallFixture(
						"cache-effort-submit",
						"sworn_submit",
						submitArguments,
					)},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{
				"prompt_tokens": 10, "completion_tokens": 5,
				"prompt_cache_hit_tokens":  6,
				"prompt_cache_miss_tokens": 4,
			},
			// The provider echoes a different effort than the profile
			// requested; the receipt must keep the two distinct.
			"reasoning_effort": "max",
		})
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-chat-cache", ID: "sworn.openai.chat.cache",
				Version:          "1.0.0",
				Endpoint:         server.URL + "/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:              OpenAIChatCompletionsAPI,
			ReasoningEffort:  "high",
			ReasoningEfforts: []string{"high", "max"},
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("secret"), nil
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
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil || requests.Load() != 1 {
		t.Fatalf("observation = %#v, error=%v", observation, err)
	}
	usage := observation.Usage
	if usage.CacheStatus != UsageReported ||
		usage.CacheReadTokens == nil || *usage.CacheReadTokens != 6 ||
		usage.CacheWriteTokens == nil || *usage.CacheWriteTokens != 4 ||
		usage.EffortRequested == nil || *usage.EffortRequested != "high" ||
		usage.EffortReported == nil || *usage.EffortReported != "max" {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestProviderWithoutCacheOrEffortYieldsHonestAbsence(t *testing.T) {
	const invocationID = "provider-honest-absence"
	submission := submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	)
	submitArguments := submissionToolArguments(t, submission)
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"absence-submit",
			"sworn_submit",
			submitArguments,
			5,
			7,
		))
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-absence", ID: "sworn.openai.absence",
				Version:          "1.0.0",
				Endpoint:         server.URL + "/chat/completions",
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
	if err != nil {
		t.Fatal(err)
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
	if err != nil || observation.Handoff == nil {
		t.Fatalf("observation = %#v, error=%v", observation, err)
	}
	usage := observation.Usage
	if usage.CacheStatus != UsageUnavailable ||
		usage.CacheReadTokens != nil || usage.CacheWriteTokens != nil ||
		usage.EffortRequested != nil || usage.EffortReported != nil ||
		usage.FinishReason != nil || usage.Truncated != nil {
		t.Fatalf("honest absence = %#v", usage)
	}
	if usage.InputTokens == nil || *usage.InputTokens != 5 ||
		usage.OutputTokens == nil || *usage.OutputTokens != 7 {
		t.Fatalf("tokens = %#v", usage)
	}
}

func TestOpenAIChatTruncationIsNamedProviderFailure(t *testing.T) {
	const invocationID = "provider-chat-truncated"
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writeJSONResponse(t, writer, map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role": "assistant", "content": "partial answer",
				},
				"finish_reason": "length",
			}},
			"usage": map[string]any{
				"prompt_tokens": 4, "completion_tokens": 3,
				"prompt_cache_hit_tokens":  2,
				"prompt_cache_miss_tokens": 2,
			},
		})
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-truncated", ID: "sworn.openai.truncated",
				Version:          "1.0.0",
				Endpoint:         server.URL + "/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:              OpenAIChatCompletionsAPI,
			ReasoningEffort:  "high",
			ReasoningEfforts: []string{"high", "max"},
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("secret"), nil
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
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "PROVIDER_TRUNCATED") || observation.Handoff != nil ||
		observation.TransportStatus != RunnerError ||
		observation.Diagnostic.Code != "provider_truncated" {
		t.Fatalf("truncation observation = %#v, error=%v", observation, err)
	}
	usage := observation.Usage
	if usage.FinishReason == nil || *usage.FinishReason != "length" ||
		usage.Truncated == nil || !*usage.Truncated ||
		usage.CacheStatus != UsageReported ||
		usage.CacheReadTokens == nil || *usage.CacheReadTokens != 2 ||
		usage.CacheWriteTokens == nil || *usage.CacheWriteTokens != 2 ||
		usage.EffortRequested == nil || *usage.EffortRequested != "high" ||
		usage.InputTokens == nil || *usage.InputTokens != 4 ||
		usage.OutputTokens == nil || *usage.OutputTokens != 3 {
		t.Fatalf("truncation usage = %#v", usage)
	}
}

func TestResponsesTruncationAndCacheAreSurfaced(t *testing.T) {
	const invocationID = "provider-responses-truncated"
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writeJSONResponse(t, writer, map[string]any{
			"id": "response-truncated", "object": "response",
			"status": "incomplete",
			"incomplete_details": map[string]any{
				"reason": "max_output_tokens",
			},
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{
					"type": "output_text", "text": "partial",
				}},
			}},
			"usage": map[string]any{
				"input_tokens": 9, "output_tokens": 6, "total_tokens": 15,
				"input_tokens_details": map[string]any{
					"cached_tokens": 7,
					"audio_tokens":  1,
				},
			},
		})
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-responses-truncated", ID: "sworn.openai.responses.truncated",
				Version:          "1.0.0",
				Endpoint:         server.URL + "/v1/responses",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:             OpenAIResponsesAPI,
			ReasoningEffort: "medium",
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("secret"), nil
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
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "PROVIDER_TRUNCATED") || observation.Handoff != nil ||
		observation.TransportStatus != RunnerError ||
		observation.Diagnostic.Code != "provider_truncated" {
		t.Fatalf("responses truncation = %#v, error=%v", observation, err)
	}
	usage := observation.Usage
	if usage.FinishReason == nil || *usage.FinishReason != "max_output_tokens" ||
		usage.Truncated == nil || !*usage.Truncated ||
		usage.CacheStatus != UsageReported ||
		usage.CacheReadTokens == nil || *usage.CacheReadTokens != 7 ||
		usage.CacheWriteTokens != nil ||
		usage.EffortRequested == nil || *usage.EffortRequested != "medium" ||
		usage.InputTokens == nil || *usage.InputTokens != 9 ||
		usage.OutputTokens == nil || *usage.OutputTokens != 6 {
		t.Fatalf("responses truncation usage = %#v", usage)
	}
}

func TestBedrockConversationSurfacesCacheAndTruncation(t *testing.T) {
	config := BedrockProfileConfig{
		Endpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
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
	// Normal end_turn with cache read/write is surfaced, not discarded.
	turn, err := conversation.accept([]byte(`{
	  "output":{"message":{"role":"assistant","content":[{"text":"done"}]}},
	  "stopReason":"end_turn",
	  "usage":{"inputTokens":5,"outputTokens":6,"totalTokens":11,
	           "cacheReadInputTokens":3,"cacheWriteInputTokens":1}
	}`))
	if err != nil || turn.Truncated || turn.FinishReason != nil ||
		turn.Usage == nil ||
		turn.Usage.CacheReadTokens == nil || *turn.Usage.CacheReadTokens != 3 ||
		turn.Usage.CacheWriteTokens == nil || *turn.Usage.CacheWriteTokens != 1 {
		t.Fatalf("bedrock cache turn = %#v, error=%v", turn, err)
	}
	// max_tokens is recognized as the provider's output-ceiling finish reason.
	truncated, err := conversation.accept([]byte(`{
	  "output":{"message":{"role":"assistant","content":[{"text":"partial"}]}},
	  "stopReason":"max_tokens",
	  "usage":{"inputTokens":7,"outputTokens":8,"totalTokens":15,
	           "cacheReadInputTokens":4}
	}`))
	if err != nil || !truncated.Truncated ||
		truncated.FinishReason == nil || *truncated.FinishReason != "max_tokens" ||
		truncated.Usage == nil ||
		truncated.Usage.CacheReadTokens == nil || *truncated.Usage.CacheReadTokens != 4 ||
		truncated.Usage.CacheWriteTokens != nil {
		t.Fatalf("bedrock truncation turn = %#v, error=%v", truncated, err)
	}
}

func TestGeminiConversationSurfacesCacheAndTruncation(t *testing.T) {
	conversation, err := newGeminiConversation(
		"https://generativelanguage.googleapis.com",
		"gemini-2.5-pro",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		0,
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	// Normal STOP with cached-content reads is surfaced, not discarded.
	turn, err := conversation.accept([]byte(`{
	  "candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},
	                 "finishReason":"STOP"}],
	  "usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":9,
	                   "totalTokenCount":16,"cachedContentTokenCount":4}
	}`))
	if err != nil || turn.Truncated || turn.FinishReason != nil ||
		turn.Usage == nil ||
		turn.Usage.CacheReadTokens == nil || *turn.Usage.CacheReadTokens != 4 ||
		turn.Usage.CacheWriteTokens != nil {
		t.Fatalf("gemini cache turn = %#v, error=%v", turn, err)
	}
	// MAX_TOKENS is recognized as the provider's output-ceiling finish reason.
	truncated, err := conversation.accept([]byte(`{
	  "candidates":[{"content":{"role":"model","parts":[{"text":"partial"}]},
	                 "finishReason":"MAX_TOKENS"}],
	  "usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,
	                   "totalTokenCount":5,"cachedContentTokenCount":1}
	}`))
	if err != nil || !truncated.Truncated ||
		truncated.FinishReason == nil || *truncated.FinishReason != "MAX_TOKENS" ||
		truncated.Usage == nil ||
		truncated.Usage.CacheReadTokens == nil || *truncated.Usage.CacheReadTokens != 1 ||
		truncated.Usage.CacheWriteTokens != nil {
		t.Fatalf("gemini truncation turn = %#v, error=%v", truncated, err)
	}
}

// A2: Failure isolation — one failing call in a batch produces its bounded
// failure result in place while remaining calls still execute and return;
// proven by a 3-call turn where the middle call fails.
func TestProviderParallelToolFailureIsolation(t *testing.T) {
	invocationID := "provider-tool-isolation"
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
		if turn == 1 {
			// Turn 1: Assistant returns 3 tool calls: valid, missing path, valid
			writeJSONResponse(t, writer, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"tool_calls": []any{
							openAIToolCallFixture("read-1", "Read", `{"path":"/workspace/file1.txt"}`),
							openAIToolCallFixture("read-2", "Read", `{"path":"/workspace/missing.txt"}`),
							openAIToolCallFixture("read-3", "Read", `{"path":"/workspace/file2.txt"}`),
						},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 6},
			})
			return
		}
		// Turn 2: Server verifies that all 3 tool results are returned in correlation order,
		// with the middle call having the bounded error string.
		var requestBody struct {
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
				Content    string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &requestBody) != nil ||
			len(requestBody.Messages) != 5 ||
			requestBody.Messages[2].ToolCallID != "read-1" ||
			requestBody.Messages[2].Content != "first content" ||
			requestBody.Messages[3].ToolCallID != "read-2" ||
			requestBody.Messages[3].Content != "error:TOOL_PATH_INVALID" ||
			requestBody.Messages[4].ToolCallID != "read-3" ||
			requestBody.Messages[4].Content != "second content" {
			t.Errorf("second request failure isolation order = %s", body)
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"submit-final",
			"sworn_submit",
			submitArguments,
			10,
			12,
		))
	}))
	defer server.Close()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-isolation", ID: "sworn.openai.isolation", Version: "1.0.0",
				Endpoint:         server.URL + "/v1/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:             OpenAIChatCompletionsAPI,
			ReasoningEffort: "none",
		},
		func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
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
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	if err := osWriteProviderFixture(invocation.HostWorkspace, "file1.txt", "first content"); err != nil {
		t.Fatal(err)
	}
	if err := osWriteProviderFixture(invocation.HostWorkspace, "file2.txt", "second content"); err != nil {
		t.Fatal(err)
	}
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil || requests.Load() != 2 {
		t.Fatalf("observation = %#v, requests=%d, error=%v", observation, requests.Load(), err)
	}
}

// A5: A driver-level test proves a discovery sequence that took N single-call turns
// completes in a bounded fraction of the turns with batching (throughput claim).
func TestProviderBatchedDiscoveryReducesTurns(t *testing.T) {
	invocationID := "provider-throughput-test"
	submission := submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	)
	submitArguments := submissionToolArguments(t, submission)

	// 1. Single-call baseline: 4 discovery turns + 1 submit turn = 5 turns
	var singleCallRequests atomic.Int64
	singleServer := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		turn := singleCallRequests.Add(1)
		switch turn {
		case 1:
			writeJSONResponse(t, writer, openAIToolCallResponse("read-doc1", "Read", `{"path":"/workspace/doc1.txt"}`, 1, 1))
		case 2:
			writeJSONResponse(t, writer, openAIToolCallResponse("read-doc2", "Read", `{"path":"/workspace/doc2.txt"}`, 1, 1))
		case 3:
			writeJSONResponse(t, writer, openAIToolCallResponse("read-doc3", "Read", `{"path":"/workspace/doc3.txt"}`, 1, 1))
		case 4:
			writeJSONResponse(t, writer, openAIToolCallResponse("read-doc4", "Read", `{"path":"/workspace/doc4.txt"}`, 1, 1))
		case 5:
			writeJSONResponse(t, writer, openAIToolCallResponse("submit-single", "sworn_submit", submitArguments, 2, 2))
		default:
			t.Errorf("unexpected single-call turn %d", turn)
		}
	}))
	defer singleServer.Close()

	// 2. Batched execution: 1 batched discovery turn (4 Read calls) + 1 submit turn = 2 turns
	var batchedRequests atomic.Int64
	batchedServer := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		turn := batchedRequests.Add(1)
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Error(err)
			return
		}
		switch turn {
		case 1:
			writeJSONResponse(t, writer, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"tool_calls": []any{
							openAIToolCallFixture("batch-1", "Read", `{"path":"/workspace/doc1.txt"}`),
							openAIToolCallFixture("batch-2", "Read", `{"path":"/workspace/doc2.txt"}`),
							openAIToolCallFixture("batch-3", "Read", `{"path":"/workspace/doc3.txt"}`),
							openAIToolCallFixture("batch-4", "Read", `{"path":"/workspace/doc4.txt"}`),
						},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 8},
			})
		case 2:
			var requestBody struct {
				Messages []struct {
					Role       string `json:"role"`
					ToolCallID string `json:"tool_call_id"`
					Content    string `json:"content"`
				} `json:"messages"`
			}
			if json.Unmarshal(body, &requestBody) != nil ||
				len(requestBody.Messages) != 6 ||
				requestBody.Messages[2].Content != "doc 1 content" ||
				requestBody.Messages[3].Content != "doc 2 content" ||
				requestBody.Messages[4].Content != "doc 3 content" ||
				requestBody.Messages[5].Content != "doc 4 content" {
				t.Errorf("batched request messages = %s", body)
			}
			writeJSONResponse(t, writer, openAIToolCallResponse("submit-batched", "sworn_submit", submitArguments, 10, 10))
		default:
			t.Errorf("unexpected batched turn %d", turn)
		}
	}))
	defer batchedServer.Close()

	newAdapter := func(serverURL string, key string) Adapter {
		adapter, err := NewOpenAIAdapter(
			OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: key, ID: "sworn." + key, Version: "1.0.0",
					Endpoint:         serverURL + "/v1/chat/completions",
					CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
					CredentialRefs: []string{"credential-ref"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				API:             OpenAIChatCompletionsAPI,
				ReasoningEffort: "none",
			},
			func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
			nil,
			nil,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		return adapter
	}

	singleAdapter := newAdapter(singleServer.URL, "openai-single")
	batchedAdapter := newAdapter(batchedServer.URL, "openai-batched")

	singleInvocation := productionInvocationFixture(
		t, singleAdapter, ProfileOpenAIHTTP, invocationID, RoleImplementer, ImplementerImplementation, ReadWrite,
	)
	batchedInvocation := productionInvocationFixture(
		t, batchedAdapter, ProfileOpenAIHTTP, invocationID, RoleImplementer, ImplementerImplementation, ReadWrite,
	)

	for _, inv := range []Invocation{singleInvocation, batchedInvocation} {
		for i, name := range []string{"doc1.txt", "doc2.txt", "doc3.txt", "doc4.txt"} {
			if err := osWriteProviderFixture(inv.HostWorkspace, name, "doc "+itoa(i+1)+" content"); err != nil {
				t.Fatal(err)
			}
		}
	}

	singleObs, singleErr := (Dispatcher{}).Invoke(context.Background(), singleInvocation)
	if singleErr != nil || singleObs.Handoff == nil {
		t.Fatalf("single invocation failed: %#v, %v", singleObs, singleErr)
	}

	batchedObs, batchedErr := (Dispatcher{}).Invoke(context.Background(), batchedInvocation)
	if batchedErr != nil || batchedObs.Handoff == nil {
		t.Fatalf("batched invocation failed: %#v, %v", batchedObs, batchedErr)
	}

	singleTurns := singleCallRequests.Load()
	batchedTurns := batchedRequests.Load()
	if singleTurns != 5 {
		t.Fatalf("single turns = %d, want 5", singleTurns)
	}
	if batchedTurns != 2 {
		t.Fatalf("batched turns = %d, want 2", batchedTurns)
	}
	// Verify throughput claim: batched turns is a bounded fraction (40% <= 50%) of single turns
	if float64(batchedTurns)/float64(singleTurns) > 0.5 {
		t.Fatalf("batched/single turn ratio %.2f > 0.50", float64(batchedTurns)/float64(singleTurns))
	}
}

// A3: Budgets bind per turn, not per call. Exceeding MaxToolCalls on a turn fails
// with continuation.openai.accept_tool_calls_invalid; combining sworn_submit or
// sworn_yield with another call fails SUBMISSION_PROTOCOL_FAILED; and cumulative session
// limits fail with RESOURCE_LIMIT.
func TestProviderParallelToolLimits(t *testing.T) {
	invocationID := "provider-limits-test"
	submission := submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	)
	submitArguments := submissionToolArguments(t, submission)

	t.Run("exceeding MaxToolCalls fails CONTINUATION_INVALID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			toolCalls := make([]any, MaxToolCalls+1)
			for i := 0; i <= MaxToolCalls; i++ {
				toolCalls[i] = openAIToolCallFixture("call-"+itoa(i), "Read", `{"path":"/workspace/a.txt"}`)
			}
			writeJSONResponse(t, writer, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"tool_calls": toolCalls,
					},
					"finish_reason": "tool_calls",
				}},
			})
		}))
		defer server.Close()
		adapter, err := NewOpenAIAdapter(
			OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "openai-limit-calls", ID: "sworn.openai.limit.calls", Version: "1.0.0",
					Endpoint:         server.URL + "/v1/chat/completions",
					CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
					CredentialRefs: []string{"credential-ref"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				API: OpenAIChatCompletionsAPI,
			},
			func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
			nil, nil, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		invocation := productionInvocationFixture(
			t, adapter, ProfileOpenAIHTTP, invocationID, RoleImplementer, ImplementerImplementation, ReadWrite,
		)
		_, err = (Dispatcher{}).Invoke(context.Background(), invocation)
		if !IsCode(err, "CONTINUATION_INVALID") {
			t.Fatalf("exceeded MaxToolCalls err = %v, want CONTINUATION_INVALID", err)
		}
	})

	t.Run("combining sworn_submit with another tool call fails SUBMISSION_PROTOCOL_FAILED", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			writeJSONResponse(t, writer, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"tool_calls": []any{
							openAIToolCallFixture("call-1", "Read", `{"path":"/workspace/a.txt"}`),
							openAIToolCallFixture("call-2", "sworn_submit", submitArguments),
						},
					},
					"finish_reason": "tool_calls",
				}},
			})
		}))
		defer server.Close()
		adapter, err := NewOpenAIAdapter(
			OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: "openai-limit-terminal", ID: "sworn.openai.limit.terminal", Version: "1.0.0",
					Endpoint:         server.URL + "/v1/chat/completions",
					CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
					CredentialRefs: []string{"credential-ref"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				API: OpenAIChatCompletionsAPI,
			},
			func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
			nil, nil, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		invocation := productionInvocationFixture(
			t, adapter, ProfileOpenAIHTTP, invocationID, RoleImplementer, ImplementerImplementation, ReadWrite,
		)
		_, err = (Dispatcher{}).Invoke(context.Background(), invocation)
		if !IsCode(err, "SUBMISSION_PROTOCOL_FAILED") {
			t.Fatalf("batch with sworn_submit err = %v, want SUBMISSION_PROTOCOL_FAILED", err)
		}
	})
}
