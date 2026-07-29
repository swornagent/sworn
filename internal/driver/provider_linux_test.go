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

func TestOpenAICompatibleFakeServerCorpusCoversEveryRole(t *testing.T) {
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
					Model string `json:"model"`
					Tools []struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
				}
				if json.NewDecoder(request.Body).Decode(&body) != nil ||
					body.Model != "exact-model" {
					t.Errorf("provider request = %#v", body)
				}
				names := make(map[string]struct{}, len(body.Tools))
				for _, tool := range body.Tools {
					names[tool.Function.Name] = struct{}{}
				}
				_, hasWrite := names["Write"]
				_, hasEdit := names["Edit"]
				if (test.access == ReadWrite) != hasWrite ||
					(test.access == ReadWrite) != hasEdit {
					t.Errorf("access-derived tools = %#v", names)
				}
				writeJSONResponse(t, writer, openAIToolCallResponse(
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
			adapter, err := NewOpenAICompatibleAdapter(
				HTTPProfileConfig{
					Key: "openai-adapter", ID: "sworn.openai", Version: "1.0.0",
					Endpoint:         server.URL + "/chat/completions",
					CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
					CredentialRefs: []string{"credential-ref"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				resolver,
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
	adapter, err := NewOpenAICompatibleAdapter(
		HTTPProfileConfig{
			Key: "openai-order", ID: "sworn.openai.order", Version: "1.0.0",
			Endpoint:         server.URL + "/chat/completions",
			CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
			CredentialRefs: []string{"credential-ref"},
			ResponseBytes:  MaxProviderResponseBytes,
		},
		func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
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
			adapter, err := NewOpenAICompatibleAdapter(
				HTTPProfileConfig{
					Key: "failure-adapter", ID: "sworn.failure", Version: "1.0.0",
					Endpoint:         server.URL + "/chat/completions",
					CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
					CredentialRefs: []string{"credential-ref"},
					ResponseBytes:  MaxProviderResponseBytes,
				},
				func(context.Context, string) ([]byte, error) {
					return []byte("credential-canary"), nil
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

func TestDeepSeekAndGeminiFakeServersPreserveTheirWireContracts(t *testing.T) {
	t.Run("DeepSeek reasoning replay", func(t *testing.T) {
		invocationID := "deepseek-replay"
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
								"deepseek-read",
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
				!bytes.Contains(body, []byte(`"tool_call_id":"deepseek-read"`)) {
				t.Errorf("DeepSeek continuation not replayed: %s", body)
			}
			writeJSONResponse(t, writer, openAIToolCallResponse(
				"deepseek-submit",
				"sworn_submit",
				submissionToolArguments(t, submission),
				3,
				4,
			))
		}))
		defer server.Close()
		adapter, err := NewDeepSeekAdapter(
			HTTPProfileConfig{
				Key: "deepseek-adapter", ID: "sworn.deepseek", Version: "1.0.0",
				Endpoint:         server.URL + "/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
			nil,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		invocation := productionInvocationFixture(
			t,
			adapter,
			ProfileDeepSeek,
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
			t.Fatalf("DeepSeek observation = %#v, error=%v", observation, err)
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
				!bytes.Contains(body, []byte(`"parametersJsonSchema"`)) ||
				bytes.Contains(body, []byte(`"parameters":`)) {
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

func writeJSONResponse(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Error(err)
	}
}
