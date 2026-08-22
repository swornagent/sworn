package driver

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The SSE fixtures under testdata/gemini-native used here are honestly
// labelled by provenance:
//
//   - n1-basic.sse and n2-toolcall.sse are recorded-derived: byte-level
//     chunkings of the recorded n1-basic.json and n2-toolcall.json exchanges
//     (captured live 2026-08-17), split into the chunk shapes
//     streamGenerateContent delivers. They are never claimed to be live
//     captures of the streaming endpoint.
//   - thought-documented.sse is documented-format-derived: it follows the
//     documented streamGenerateContent SSE shape including the provider's
//     "thought": true part vocabulary. It is not a live capture.
func geminiStreamFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(
		filepath.Join("testdata", "gemini-native", name),
	)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// geminiStreamConversation builds the same probe conversation the recorded
// fixtures were captured against, with explicit stream and includeThoughts
// knobs.
func geminiStreamConversation(
	t *testing.T,
	stream bool,
	includeThoughts bool,
	thinkingLevel string,
) *geminiConversation {
	t.Helper()
	conversation, err := newGeminiConversation(
		"https://generativelanguage.example.invalid",
		"gemini-3.7-flash",
		[]providerToolDefinition{{
			Name:        "probe",
			Description: "probe tool",
			InputSchema: []byte(
				`{"type":"object","properties":{"value":{"type":"number"}},"required":["value"]}`,
			),
		}},
		[]byte(`Call the probe tool with value 7.`),
		2000,
		thinkingLevel,
		stream,
		includeThoughts,
	)
	if err != nil {
		t.Fatal(err)
	}
	return conversation
}

// captureGeminiLiveStream swaps the global live renderer for a buffer-backed
// one for the duration of fn and returns everything rendered. Tests using it
// must not call t.Parallel: parallel tests are paused while sequential tests
// run, so the swap cannot race them.
func captureGeminiLiveStream(t *testing.T, fn func()) string {
	t.Helper()
	saved := liveStream
	var buffer bytes.Buffer
	liveStream = &streamRenderer{out: &buffer}
	defer func() { liveStream = saved }()
	fn()
	return buffer.String()
}

// contractSite extracts the Code and Detail of a contract error for parity
// comparison.
func contractSite(err error) (string, string) {
	var contractErr *ContractError
	if errors.As(err, &contractErr) {
		return contractErr.Code, contractErr.Detail
	}
	return "", ""
}

// A1: the streaming knob changes the URL suffix and the request's stream
// fields, never the request body — streaming is requested on the URL, so
// both modes carry byte-identical JSON and pacing estimates them identically.
func TestGeminiStreamRequestShapeAndModeParity(t *testing.T) {
	unstreamed := geminiStreamConversation(t, false, false, "")
	defer unstreamed.close()
	plain, err := unstreamed.request()
	if err != nil {
		t.Fatal(err)
	}
	if plain.Stream || plain.StreamFormat != "" || plain.StreamModel != "" {
		t.Fatalf("unstreamed request gained stream fields: %#v", plain)
	}
	if !strings.HasSuffix(
		plain.URL,
		"/v1beta/models/gemini-3.7-flash:generateContent",
	) {
		t.Fatalf("unstreamed URL = %s", plain.URL)
	}

	streamed := geminiStreamConversation(t, true, false, "")
	defer streamed.close()
	sse, err := streamed.request()
	if err != nil {
		t.Fatal(err)
	}
	if !sse.Stream || sse.StreamFormat != geminiStreamFormat ||
		sse.StreamModel != "gemini-3.7-flash" {
		t.Fatalf("streamed request fields = %#v", sse)
	}
	if !strings.HasSuffix(
		sse.URL,
		"/v1beta/models/gemini-3.7-flash:streamGenerateContent?alt=sse",
	) {
		t.Fatalf("streamed URL = %s", sse.URL)
	}
	if !bytes.Equal(plain.Body, sse.Body) {
		t.Fatalf(
			"streamed body != unstreamed body\nstreamed:   %s\nunstreamed: %s",
			sse.Body,
			plain.Body,
		)
	}
}

// A4 request side: includeThoughts is an independent operator knob in the
// provider's own vocabulary. Its absence changes nothing — no thinkingConfig
// is emitted unless a knob is set — and it never implies a thinking level.
func TestGeminiStreamIncludeThoughtsRequestShape(t *testing.T) {
	levelOnly := geminiStreamConversation(t, false, false, "LOW")
	defer levelOnly.close()
	body, err := levelOnly.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body.Body, []byte(`"thinkingLevel":"LOW"`)) ||
		bytes.Contains(body.Body, []byte("includeThoughts")) {
		t.Fatalf("thinkingLevel-only request = %s", body.Body)
	}

	thoughtsOnly := geminiStreamConversation(t, false, true, "")
	defer thoughtsOnly.close()
	body, err = thoughtsOnly.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body.Body, []byte(`"includeThoughts":true`)) ||
		bytes.Contains(body.Body, []byte("thinkingLevel")) {
		t.Fatalf("includeThoughts-only request = %s", body.Body)
	}

	both := geminiStreamConversation(t, false, true, "LOW")
	defer both.close()
	body, err = both.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body.Body, []byte(`"thinkingLevel":"LOW"`)) ||
		!bytes.Contains(body.Body, []byte(`"includeThoughts":true`)) {
		t.Fatalf("both-knobs request = %s", body.Body)
	}

	neither := geminiStreamConversation(t, false, false, "")
	defer neither.close()
	body, err = neither.request()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body.Body, []byte("thinkingConfig")) {
		t.Fatalf("knob-less request gained thinkingConfig: %s", body.Body)
	}
}

// A1/A3: the recorded n1 stream, chunked into SSE deltas, reconstructs to the
// semantic equal of the recorded unstreamed response, and both paths produce
// identical turns and identical subsequent request bytes.
func TestGeminiStreamReconstructionParity(t *testing.T) {
	var terminal []byte
	rendered := captureGeminiLiveStream(t, func() {
		var err error
		terminal, err = readGeminiStream(
			bytes.NewReader(geminiStreamFixture(t, "n1-basic.sse")),
			MaxProviderResponseBytes,
			"gemini-3.7-flash",
		)
		if err != nil {
			t.Fatalf("readGeminiStream = %v", err)
		}
	})
	if !strings.Contains(rendered, "── gemini-3.7-flash turn ──") ||
		!strings.Contains(rendered, "OK.") ||
		!strings.Contains(rendered, "── status=STOP in=4 out=2 reasoning=53 ──") {
		t.Fatalf("live rendering = %q", rendered)
	}
	streamedValue := geminiNativeJSONValue(t, terminal)
	recordedValue := geminiNativeJSONValue(
		t,
		geminiNativeFixture(t, "n1-basic.json"),
	)
	if !reflect.DeepEqual(streamedValue, recordedValue) {
		t.Fatalf(
			"reconstructed != recorded\nreconstructed: %s\nrecorded:      %s",
			terminal,
			geminiNativeFixture(t, "n1-basic.json"),
		)
	}

	plain := geminiStreamConversation(t, false, false, "")
	defer plain.close()
	plainTurn, err := plain.accept(geminiNativeFixture(t, "n1-basic.json"))
	if err != nil {
		t.Fatal(err)
	}
	streamed := geminiStreamConversation(t, true, false, "")
	defer streamed.close()
	streamedTurn, err := streamed.accept(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plainTurn, streamedTurn) {
		t.Fatalf(
			"streamed turn != unstreamed turn\nstreamed:   %#v\nunstreamed: %#v",
			streamedTurn,
			plainTurn,
		)
	}
	plainRequest, err := plain.request()
	if err != nil {
		t.Fatal(err)
	}
	streamedRequest, err := streamed.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plainRequest.Body, streamedRequest.Body) {
		t.Fatalf(
			"replay diverged after parity accept\nstreamed:   %s\nunstreamed: %s",
			streamedRequest.Body,
			plainRequest.Body,
		)
	}
}

// A3: the recorded n2 tool-call exchange streams and unstreams to identical
// tool calls, identical usage, and byte-identical follow-up requests
// (replayed thought signature included).
func TestGeminiStreamToolCallParity(t *testing.T) {
	var terminal []byte
	rendered := captureGeminiLiveStream(t, func() {
		var err error
		terminal, err = readGeminiStream(
			bytes.NewReader(geminiStreamFixture(t, "n2-toolcall.sse")),
			MaxProviderResponseBytes,
			"gemini-3.7-flash",
		)
		if err != nil {
			t.Fatalf("readGeminiStream = %v", err)
		}
	})
	if !strings.Contains(rendered, "⚙ probe ") {
		t.Fatalf("tool-call rendering = %q", rendered)
	}
	streamedValue := geminiNativeJSONValue(t, terminal)
	recordedValue := geminiNativeJSONValue(
		t,
		geminiNativeFixture(t, "n2-toolcall.json"),
	)
	if !reflect.DeepEqual(streamedValue, recordedValue) {
		t.Fatalf(
			"reconstructed != recorded\nreconstructed: %s\nrecorded:      %s",
			terminal,
			geminiNativeFixture(t, "n2-toolcall.json"),
		)
	}

	plain := geminiStreamConversation(t, false, false, "")
	defer plain.close()
	plainTurn, err := plain.accept(geminiNativeFixture(t, "n2-toolcall.json"))
	if err != nil {
		t.Fatal(err)
	}
	streamed := geminiStreamConversation(t, true, false, "")
	defer streamed.close()
	streamedTurn, err := streamed.accept(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plainTurn, streamedTurn) {
		t.Fatalf(
			"streamed turn != unstreamed turn\nstreamed:   %#v\nunstreamed: %#v",
			streamedTurn,
			plainTurn,
		)
	}
	if len(plainTurn.Calls) != 1 ||
		plainTurn.Calls[0].ID != "call_4392591" ||
		plainTurn.Calls[0].Name != "probe" ||
		string(plainTurn.Calls[0].Arguments) != `{"value":7}` {
		t.Fatalf("tool call = %#v", plainTurn.Calls)
	}
	results := []providerToolResult{{
		ID: plainTurn.Calls[0].ID, Name: plainTurn.Calls[0].Name,
		Content: []byte("49"),
	}}
	if err := plain.appendResults(results); err != nil {
		t.Fatal(err)
	}
	if err := streamed.appendResults(results); err != nil {
		t.Fatal(err)
	}
	plainRequest, err := plain.request()
	if err != nil {
		t.Fatal(err)
	}
	streamedRequest, err := streamed.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plainRequest.Body, streamedRequest.Body) {
		t.Fatalf(
			"tool-call replay diverged\nstreamed:   %s\nunstreamed: %s",
			streamedRequest.Body,
			plainRequest.Body,
		)
	}
}

// C3: chunks are merged to EOF, usageMetadata is taken last-seen — never
// summed and never a terminal marker — and a stream without a candidate
// finishReason fails the missing-terminal way.
func TestGeminiStreamTerminalConditions(t *testing.T) {
	t.Run("parts without finish reason fail transport", func(t *testing.T) {
		sse := "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"partial\"}]}}]}\n\n"
		_, err := captureErr(t, sse)
		if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
			t.Fatalf("missing-terminal error = %v", err)
		}
	})

	t.Run("usage-only chunk is not a terminal", func(t *testing.T) {
		sse := "data: {\"usageMetadata\":{\"promptTokenCount\":4,\"candidatesTokenCount\":2}}\n\n"
		_, err := captureErr(t, sse)
		if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
			t.Fatalf("usage-only terminal error = %v", err)
		}
	})

	t.Run("merges chunks after finish reason to EOF, usage last-seen", func(t *testing.T) {
		sse := strings.Join([]string{
			`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"first"}]},"finishReason":"STOP"}]}`,
			``,
			`data: {"candidates":[{"content":{"role":"model","parts":[{"text":" second"}]}}]}`,
			``,
			`data: {"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2}}`,
			``,
			`data: {"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}}`,
			``,
		}, "\n")
		var terminal []byte
		captureGeminiLiveStream(t, func() {
			var err error
			terminal, err = readGeminiStream(
				bytes.NewReader([]byte(sse)),
				MaxProviderResponseBytes,
				"gemini-3.7-flash",
			)
			if err != nil {
				t.Fatalf("readGeminiStream = %v", err)
			}
		})
		conversation := geminiStreamConversation(t, true, false, "")
		defer conversation.close()
		turn, err := conversation.accept(terminal)
		if err != nil {
			t.Fatal(err)
		}
		// Merged to EOF: both text deltas in one part. Usage last-seen:
		// prompt 10, never 1+10.
		if !turn.Prose || len(turn.Calls) != 0 ||
			turn.Usage == nil || turn.Usage.InputTokens != 10 ||
			turn.Usage.OutputTokens != 20 {
			t.Fatalf("merged terminal turn = %#v, %v", turn, err)
		}
		if len(conversation.contents) != 2 ||
			len(conversation.contents[1].Parts) != 1 ||
			conversation.contents[1].Parts[0].Text == nil ||
			*conversation.contents[1].Parts[0].Text != "first second" {
			t.Fatalf("merged parts = %#v", conversation.contents[1])
		}
	})
}

func captureErr(t *testing.T, sse string) ([]byte, error) {
	t.Helper()
	var terminal []byte
	var terminalErr error
	captureGeminiLiveStream(t, func() {
		terminal, terminalErr = readGeminiStream(
			bytes.NewReader([]byte(sse)),
			MaxProviderResponseBytes,
			"gemini-3.7-flash",
		)
	})
	return terminal, terminalErr
}

// C2: the reader never duplicates accept's allowlists. Unknown keys anywhere
// in a chunk are carried through into the reconstruction, so accept() classifies
// them with the exact site and code the non-streaming path produces for the
// same bytes.
func TestGeminiStreamErrorClassificationParity(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		chunks []string
	}{
		{
			name: "root",
			body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2},"evil":1}`,
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"OK."}]},"finishReason":"STOP"}]}`,
				`{"evil":1}`,
			},
		},
		{
			name: "candidate",
			body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK."}]},"finishReason":"STOP","evil":1}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2}}`,
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"OK."}]},"finishReason":"STOP","evil":1}]}`,
			},
		},
		{
			name: "content",
			body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK."}],"evil":1},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2}}`,
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"OK."}],"evil":1},"finishReason":"STOP"}]}`,
			},
		},
		{
			name: "part",
			body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK.","evil":1}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2}}`,
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"OK.","evil":1}]},"finishReason":"STOP"}]}`,
			},
		},
		{
			name: "usage",
			body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"evil":1}}`,
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"OK."}]},"finishReason":"STOP"}]}`,
				`{"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"evil":1}}`,
			},
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			plain := geminiStreamConversation(t, false, false, "")
			defer plain.close()
			_, plainErr := plain.accept([]byte(test.body))
			if plainErr == nil {
				t.Fatalf("unstreamed accept admitted %s", test.body)
			}
			streamed := geminiStreamConversation(t, true, false, "")
			defer streamed.close()
			sse := make([]string, len(test.chunks))
			for index, chunk := range test.chunks {
				sse[index] = "data: " + chunk
			}
			terminal, terminalErr := captureErr(
				t,
				strings.Join(sse, "\n\n")+"\n\n",
			)
			if terminalErr != nil {
				t.Fatalf("readGeminiStream = %v", terminalErr)
			}
			_, streamedErr := streamed.accept(terminal)
			if streamedErr == nil {
				t.Fatalf("streamed accept admitted %s", terminal)
			}
			plainCode, plainDetail := contractSite(plainErr)
			streamedCode, streamedDetail := contractSite(streamedErr)
			if plainCode != streamedCode || plainDetail != streamedDetail {
				t.Fatalf(
					"classification diverged\nunstreamed: %v\nstreamed:   %v",
					plainErr,
					streamedErr,
				)
			}
		})
	}
}

// A4: thought parts are admitted, rendered through the reasoning channel,
// measured, and never replayed; a turn whose parts are all thoughts fails
// closed; and "thought": false is equivalent to absence (C5).
func TestGeminiStreamThoughtParts(t *testing.T) {
	var terminal []byte
	rendered := captureGeminiLiveStream(t, func() {
		var err error
		terminal, err = readGeminiStream(
			bytes.NewReader(
				geminiStreamFixture(t, "thought-documented.sse"),
			),
			MaxProviderResponseBytes,
			"gemini-3.7-flash",
		)
		if err != nil {
			t.Fatalf("readGeminiStream = %v", err)
		}
	})
	if !strings.Contains(rendered, "── gemini-3.7-flash turn ──") ||
		!strings.Contains(rendered, "· Let me think this through.") ||
		!strings.Contains(rendered, "The probe returned 49.") ||
		!strings.Contains(rendered, "── status=STOP in=92 out=13 reasoning=15 ──") {
		t.Fatalf("thought rendering = %q", rendered)
	}
	conversation := geminiStreamConversation(t, true, true, "")
	defer conversation.close()
	turn, err := conversation.accept(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if !turn.Prose || len(turn.Calls) != 0 ||
		turn.Usage == nil || turn.Usage.InputTokens != 92 ||
		turn.Usage.OutputTokens != 13 ||
		turn.Usage.ReasoningTokens == nil ||
		*turn.Usage.ReasoningTokens != 15 {
		t.Fatalf("thought turn = %#v, %v", turn, err)
	}
	// The thought part is never replayed: only the visible part survives.
	if len(conversation.contents) != 2 ||
		len(conversation.contents[1].Parts) != 1 ||
		conversation.contents[1].Parts[0].Text == nil ||
		*conversation.contents[1].Parts[0].Text != "The probe returned 49." {
		t.Fatalf("thought part replayed: %#v", conversation.contents)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(request.Body, []byte("Let me think this through")) {
		t.Fatalf("thought text replayed: %s", request.Body)
	}

	t.Run("all-thought turn fails closed", func(t *testing.T) {
		body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"only thoughts","thought":true}]},"finishReason":"STOP"}]}`
		plain := geminiStreamConversation(t, false, true, "")
		defer plain.close()
		_, plainErr := plain.accept([]byte(body))
		code, detail := contractSite(plainErr)
		if code != "CONTINUATION_INVALID" ||
			detail != "continuation.gemini.accept_all_parts_thought" {
			t.Fatalf("all-thought unstreamed error = %v", plainErr)
		}
		terminal, terminalErr := captureErr(
			t,
			"data: "+body+"\n\n",
		)
		if terminalErr != nil {
			t.Fatalf("readGeminiStream = %v", terminalErr)
		}
		streamed := geminiStreamConversation(t, true, true, "")
		defer streamed.close()
		_, streamedErr := streamed.accept(terminal)
		streamedCode, streamedDetail := contractSite(streamedErr)
		if streamedCode != "CONTINUATION_INVALID" || streamedDetail != detail {
			t.Fatalf("all-thought streamed error = %v", streamedErr)
		}
	})

	t.Run("thought on function call fails closed", func(t *testing.T) {
		body := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"probe","args":{"value":7}},"thought":true}]},"finishReason":"STOP"}]}`
		plain := geminiStreamConversation(t, false, true, "")
		defer plain.close()
		_, err := plain.accept([]byte(body))
		code, detail := contractSite(err)
		if code != "CONTINUATION_INVALID" ||
			detail != "continuation.gemini.accept_thought_on_function_call_invalid" {
			t.Fatalf("thought-on-call error = %v", err)
		}
	})

	t.Run("non-bool thought fails closed", func(t *testing.T) {
		body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK.","thought":"yes"}]},"finishReason":"STOP"}]}`
		plain := geminiStreamConversation(t, false, true, "")
		defer plain.close()
		if _, err := plain.accept([]byte(body)); !IsCode(err, "CONTINUATION_INVALID") {
			t.Fatalf("non-bool thought error = %v", err)
		}
	})

	t.Run("thought false equals absence", func(t *testing.T) {
		body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK.","thought":false}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2}}`
		plain := geminiStreamConversation(t, false, false, "")
		defer plain.close()
		plainTurn, err := plain.accept([]byte(body))
		if err != nil || !plainTurn.Prose {
			t.Fatalf("thought-false unstreamed turn = %#v, %v", plainTurn, err)
		}
		if len(plain.contents) != 2 ||
			plain.contents[1].Parts[0].Text == nil ||
			*plain.contents[1].Parts[0].Text != "OK." {
			t.Fatalf("thought-false part lost: %#v", plain.contents)
		}
		terminal, terminalErr := captureErr(
			t,
			"data: "+body+"\n\n",
		)
		if terminalErr != nil {
			t.Fatalf("readGeminiStream = %v", terminalErr)
		}
		streamed := geminiStreamConversation(t, true, false, "")
		defer streamed.close()
		streamedTurn, err := streamed.accept(terminal)
		if err != nil || !reflect.DeepEqual(plainTurn, streamedTurn) {
			t.Fatalf(
				"thought-false parity\nstreamed:   %#v, %v\nunstreamed: %#v, %v",
				streamedTurn,
				err,
				plainTurn,
				nil,
			)
		}
	})
}

// A1/A4 rendering: the gemini renderer cases draw exactly like the responses
// cases — bold turn header, dim reasoning channel for thoughts, plain text,
// tool glyph with arguments, dim usage summary.
func TestGeminiStreamRendererCases(t *testing.T) {
	var buffer bytes.Buffer
	renderer := &streamRenderer{out: &buffer}
	renderer.event("gemini.turn", []byte(`{"model":"gemini-3.7-flash"}`))
	renderer.event("gemini.reasoning.delta", []byte(`{"delta":"thinking"}`))
	renderer.event("gemini.text.delta", []byte(`{"delta":"doing"}`))
	renderer.event("gemini.function_call", []byte(`{"name":"Read"}`))
	renderer.event(
		"gemini.function_call.arguments.delta",
		[]byte(`{"delta":"{\"path\":\"/w\"}"}`),
	)
	renderer.event("gemini.function_call.arguments.done", nil)
	renderer.event(
		"gemini.completed",
		[]byte(`{"finish_reason":"STOP","usage":{"prompt_tokens":4,"candidates_tokens":2,"cached_tokens":1,"thoughts_tokens":3}}`),
	)
	want := "── gemini-3.7-flash turn ──\n" +
		"· thinking\n" +
		"doing\n" +
		"⚙ Read {\"path\":\"/w\"}\n" +
		"── status=STOP in=4 out=2 cached=1 reasoning=3 ──\n"
	if got := buffer.String(); got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

// C1: the two new knobs are omitempty, so every existing operator document
// without them stays canonical byte-for-byte, and a document with them
// round-trips canonically with a different configuration digest (a
// config-digest change at the operator's hand).
func TestGeminiStreamKnobsCanonicalConfigRoundTrip(t *testing.T) {
	base := completeDriverConfigFixture(t)
	baseBody, err := EncodeDriverConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(baseBody, []byte(`"stream"`)) ||
		bytes.Contains(baseBody, []byte(`"include_thoughts"`)) {
		t.Fatalf("knob-less document gained stream keys: %s", baseBody)
	}
	baseLoaded, err := DecodeDriverConfig(baseBody)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baseLoaded.CanonicalJSON(), baseBody) {
		t.Fatalf(
			"knob-less document no longer canonical\nencoded:  %s\ncanonical: %s",
			baseBody,
			baseLoaded.CanonicalJSON(),
		)
	}
	baseDigest := baseLoaded.ConfigurationDigest()

	knobbed := completeDriverConfigFixture(t)
	found := false
	for index := range knobbed.Adapters {
		if knobbed.Adapters[index].Gemini != nil {
			knobbed.Adapters[index].Gemini.Stream = true
			knobbed.Adapters[index].Gemini.IncludeThoughts = true
			found = true
		}
	}
	if !found {
		t.Fatal("fixture carries no gemini adapter")
	}
	knobbedBody, err := EncodeDriverConfig(knobbed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(knobbedBody, []byte(`"stream":true`)) ||
		!bytes.Contains(knobbedBody, []byte(`"include_thoughts":true`)) {
		t.Fatalf("knobbed document lost knobs: %s", knobbedBody)
	}
	knobbedLoaded, err := DecodeDriverConfig(knobbedBody)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(knobbedLoaded.CanonicalJSON(), knobbedBody) {
		t.Fatalf("knobbed document not canonical: %s", knobbedBody)
	}
	if knobbedLoaded.ConfigurationDigest() == baseDigest {
		t.Fatalf("knobs did not change the configuration digest")
	}
}

// Risk 4 pin: an OpenAI profile's own shallower "stream" key keeps binding
// its responses-flavour knob, while the gemini profile binds the new knobs on
// HTTPProfileConfig itself.
func TestGeminiStreamKnobDecodeDirection(t *testing.T) {
	var openai OpenAIProfileConfig
	if err := json.Unmarshal([]byte(`{"stream":true}`), &openai); err != nil {
		t.Fatal(err)
	}
	if !openai.Stream || openai.HTTPProfileConfig.Stream {
		t.Fatalf("openai decode = %#v", openai)
	}
	var gemini HTTPProfileConfig
	if err := json.Unmarshal(
		[]byte(`{"stream":true,"include_thoughts":true}`),
		&gemini,
	); err != nil {
		t.Fatal(err)
	}
	if !gemini.Stream || !gemini.IncludeThoughts {
		t.Fatalf("gemini decode = %#v", gemini)
	}
}
