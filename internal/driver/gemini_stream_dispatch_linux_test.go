package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

// geminiStreamSubmitArguments renders the recorded n2-style sworn_submit
// functionCall argument value for the production dispatch fixtures.
func geminiStreamSubmitArguments(t *testing.T, invocationID string) map[string]any {
	t.Helper()
	submission := submissionFixture(
		t,
		invocationID,
		ImplementerImplementation,
		"",
	)
	arguments := submissionToolArguments(t, submission)
	var value map[string]any
	if json.Unmarshal([]byte(arguments), &value) != nil {
		t.Fatal("invalid Gemini arguments fixture")
	}
	return value
}

// geminiStreamSubmitResponse renders the streamed equivalent of the
// unstreamed production fixture: one functionCall chunk, one finishReason
// chunk, one usage chunk.
func geminiStreamSubmitResponse(arguments map[string]any) string {
	callChunk, _ := json.Marshal(map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{
				"role": "model",
				"parts": []any{map[string]any{
					"functionCall": map[string]any{
						"id": "gemini-submit", "name": "sworn_submit",
						"args": arguments,
					},
				}},
			},
		}},
	})
	finishChunk, _ := json.Marshal(map[string]any{
		"candidates": []any{map[string]any{
			"finishReason":  "STOP",
			"finishMessage": "tool call completed",
		}},
	})
	usageChunk, _ := json.Marshal(map[string]any{
		"usageMetadata": map[string]any{
			"promptTokenCount": 11, "candidatesTokenCount": 13,
			"totalTokenCount": 24, "serviceTier": "STANDARD",
		},
	})
	return "data: " + string(callChunk) + "\n\n" +
		"data: " + string(finishChunk) + "\n\n" +
		"data: " + string(usageChunk) + "\n\n"
}

func geminiStreamSubmitJSON(arguments map[string]any) string {
	body, _ := json.Marshal(map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{
				"role": "model",
				"parts": []any{map[string]any{
					"functionCall": map[string]any{
						"id": "gemini-submit", "name": "sworn_submit",
						"args": arguments,
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
	return string(body)
}

// geminiStreamDispatchAdapter builds a Gemini adapter against one scripted
// round tripper with the stream knob and the proactive pacing cap set.
func geminiStreamDispatchAdapter(
	server *httptest.Server,
	stream bool,
	inputTokensPerMinute int64,
	roundTripper http.RoundTripper,
) (Adapter, error) {
	return NewGeminiAdapter(
		HTTPProfileConfig{
			Key: "gemini-adapter", ID: "sworn.gemini", Version: "1.0.0",
			Endpoint:             server.URL,
			CredentialHeader:     "x-goog-api-key",
			CredentialPrefix:     "",
			CredentialRefs:       []string{"credential-ref"},
			ResponseBytes:        MaxProviderResponseBytes,
			InputTokensPerMinute: inputTokensPerMinute,
			Stream:               stream,
			IncludeThoughts:      false,
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("gemini-secret"), nil
		},
		nil,
		roundTripper,
	)
}

// A2/A3: a streamed dispatch with proactive pacing configured produces the
// identical usage receipt and handoff as the unstreamed dispatch of the same
// fixture — the ledger estimates from request-body bytes (identical between
// modes) and records from accepted usage (identical by reconstruction), and
// the terminal validation is the same accept path.
func TestGeminiStreamedDispatchParityWithUnstreamed(t *testing.T) {
	invocationID := "gemini-stream-parity"
	arguments := geminiStreamSubmitArguments(t, invocationID)
	type capture struct {
		body   []byte
		url    string
		path   string
		query  string
		accept string
	}
	run := func(stream bool) (Observation, capture) {
		var captured capture
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			if request.Header.Get("x-goog-api-key") != "gemini-secret" {
				t.Errorf("credential header = %#v", request.Header)
			}
			body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
			if err != nil {
				t.Errorf("request body read = %v", err)
			}
			captured = capture{
				body:   append([]byte(nil), body...),
				url:    request.URL.String(),
				path:   request.URL.Path,
				query:  request.URL.RawQuery,
				accept: request.Header.Get("Accept"),
			}
			if stream {
				if _, err := writer.Write(
					[]byte(geminiStreamSubmitResponse(arguments)),
				); err != nil {
					t.Errorf("sse write = %v", err)
				}
				return
			}
			writeJSONResponse(t, writer, json.RawMessage(
				geminiStreamSubmitJSON(arguments),
			))
		}))
		defer server.Close()
		adapter, err := geminiStreamDispatchAdapter(
			server,
			stream,
			1_000_000,
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
		observation, err := (Dispatcher{}).Invoke(
			context.Background(),
			invocation,
		)
		if err != nil || observation.Handoff == nil ||
			requests.Load() != 1 {
			t.Fatalf(
				"stream=%v observation = %#v, error=%v, requests=%d",
				stream,
				observation,
				err,
				requests.Load(),
			)
		}
		return observation, captured
	}

	streamed, streamedRequest := run(true)
	plain, plainRequest := run(false)

	if streamedRequest.path != "/v1beta/models/exact-model:streamGenerateContent" ||
		streamedRequest.query != "alt=sse" ||
		streamedRequest.accept != "text/event-stream" {
		t.Fatalf(
			"streamed request = %s (path=%s query=%s accept=%s)",
			streamedRequest.url,
			streamedRequest.path,
			streamedRequest.query,
			streamedRequest.accept,
		)
	}
	if plainRequest.path != "/v1beta/models/exact-model:generateContent" {
		t.Fatalf("unstreamed request = %s", plainRequest.url)
	}
	if !bytes.Equal(streamedRequest.body, plainRequest.body) {
		t.Fatalf(
			"streamed body != unstreamed body\nstreamed:   %s\nunstreamed: %s",
			streamedRequest.body,
			plainRequest.body,
		)
	}
	if !reflect.DeepEqual(streamed.Usage, plain.Usage) {
		t.Fatalf(
			"usage receipts diverged\nstreamed:   %#v\nunstreamed: %#v",
			streamed.Usage,
			plain.Usage,
		)
	}
	if streamed.Usage.InputTokens == nil ||
		*streamed.Usage.InputTokens != 11 ||
		streamed.Usage.OutputTokens == nil ||
		*streamed.Usage.OutputTokens != 13 {
		t.Fatalf("streamed receipt = %#v", streamed.Usage)
	}
}

// A2 reactive path: a 429-answered streamed dispatch retries the identical
// streamed request (same URL suffix, same bytes) and records usage once on
// the eventual success.
func TestGeminiStreamedPacedRetryResendsIdenticalRequest(t *testing.T) {
	invocationID := "gemini-stream-paced-retry"
	arguments := geminiStreamSubmitArguments(t, invocationID)
	var attempts atomic.Int64
	var firstBody, secondBody []byte
	var firstURL, secondURL string
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if attempts.Add(1) == 1 {
			body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
			if err != nil {
				t.Errorf("request body read = %v", err)
			}
			firstBody = append([]byte(nil), body...)
			firstURL = request.URL.String()
			writer.Header().Set("Retry-After", "1")
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = writer.Write([]byte(
				`{"error":{"code":429,"message":"quota window","status":"RESOURCE_EXHAUSTED"}}`,
			))
			return
		}
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Errorf("request body read = %v", err)
		}
		secondBody = append([]byte(nil), body...)
		secondURL = request.URL.String()
		if _, err := writer.Write(
			[]byte(geminiStreamSubmitResponse(arguments)),
		); err != nil {
			t.Errorf("sse write = %v", err)
		}
	}))
	defer server.Close()
	adapter, err := geminiStreamDispatchAdapter(server, true, 0, nil)
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
	observation, err := (Dispatcher{}).Invoke(
		context.Background(),
		invocation,
	)
	if err != nil || observation.Handoff == nil || attempts.Load() != 2 {
		t.Fatalf(
			"observation = %#v, error=%v, attempts=%d",
			observation,
			err,
			attempts.Load(),
		)
	}
	if !bytes.Equal(firstBody, secondBody) || firstURL != secondURL {
		t.Fatalf(
			"retry re-sent a different request\nfirst:  %s %s\nsecond: %s %s",
			firstURL,
			firstBody,
			secondURL,
			secondBody,
		)
	}
	if observation.Usage.InputTokens == nil ||
		*observation.Usage.InputTokens != 11 ||
		observation.Usage.OutputTokens == nil ||
		*observation.Usage.OutputTokens != 13 {
		t.Fatalf("paced streamed receipt = %#v", observation.Usage)
	}
}
