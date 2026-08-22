package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// geminiNativeFixture loads one recorded generateContent exchange under
// internal/driver/testdata/gemini-native. These are the recorded exchanges the
// slice contract cites as acceptance evidence (captured live against the real
// endpoint on 2026-08-17), never shapes derived from documented formats.
func geminiNativeFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(
		filepath.Join("testdata", "gemini-native", name),
	)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// geminiNativeProbeConversation builds the exact conversation the recorded
// n3/n6 requests were captured against: model gemini-3.7-flash, one probe
// tool, output limit 2000 (the recorded maxOutputTokens), and the operator's
// thinking level from configuration.
func geminiNativeProbeConversation(
	t *testing.T,
	thinkingLevel string,
	prompt string,
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
		[]byte(prompt),
		2000,
		thinkingLevel,
	)
	if err != nil {
		t.Fatal(err)
	}
	return conversation
}

func geminiNativeJSONValue(t *testing.T, body []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// A1/A2/A3: the recorded n2 tool-call exchange decodes into provider tool
// calls, the result rides the fixed single-key envelope, the thought
// signature is retained opaque and replayed byte-exact, and the built request
// is semantically equal to the recorded n3 round-trip request (contents/parts,
// functionDeclarations with parameters, generationConfig carrying
// maxOutputTokens from the invocation limits and thinkingLevel from adapter
// configuration).
func TestGeminiNativeRoundTripMatchesRecordedRequest(t *testing.T) {
	conversation := geminiNativeProbeConversation(
		t,
		"LOW",
		"Call the probe tool with value 7.",
	)
	defer conversation.close()

	turn, err := conversation.accept(geminiNativeFixture(t, "n2-toolcall.json"))
	if err != nil || len(turn.Calls) != 1 ||
		turn.Calls[0].ID != "call_4392591" ||
		turn.Calls[0].Name != "probe" ||
		string(turn.Calls[0].Arguments) != `{"value":7}` ||
		turn.Usage == nil || turn.Usage.InputTokens != 47 ||
		turn.Usage.OutputTokens != 12 ||
		turn.Usage.ReasoningTokens == nil ||
		*turn.Usage.ReasoningTokens != 20 {
		t.Fatalf("n2 turn = %#v, %v", turn, err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "call_4392591", Name: "probe", Content: []byte("49"),
	}}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	built := geminiNativeJSONValue(t, request.Body)
	recorded := geminiNativeJSONValue(
		t,
		geminiNativeFixture(t, "n3-roundtrip-req.json"),
	)
	if !reflect.DeepEqual(built, recorded) {
		t.Fatalf(
			"built request != recorded n3 request\nbuilt:    %s\nrecorded: %s",
			request.Body,
			geminiNativeFixture(t, "n3-roundtrip-req.json"),
		)
	}
	// The replayed thoughtSignature must be byte-exact: the semantic equality
	// above already binds it, and the explicit byte check makes the opaque
	// retention rule mechanically detectable.
	builtSignature := geminiNativeReplayedSignature(t, request.Body)
	recordedSignature := geminiNativeReplayedSignature(
		t,
		geminiNativeFixture(t, "n3-roundtrip-req.json"),
	)
	if builtSignature != recordedSignature || builtSignature == "" {
		t.Fatalf(
			"replayed thoughtSignature %q != recorded %q",
			builtSignature,
			recordedSignature,
		)
	}
}

// geminiNativeReplayedSignature extracts the thoughtSignature on the model
// functionCall part of a built request.
func geminiNativeReplayedSignature(t *testing.T, body []byte) string {
	t.Helper()
	var value struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				ThoughtSignature string `json:"thoughtSignature"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if json.Unmarshal(body, &value) != nil || len(value.Contents) < 2 {
		t.Fatalf("invalid Gemini request body: %s", body)
	}
	return value.Contents[1].Parts[0].ThoughtSignature
}

// A2 constraint: a tool result whose text is itself valid JSON still rides as
// text inside the fixed single-key envelope, never parsed and embedded
// structured. The recorded n6 request pins the exact wire shape; a worker who
// took the shortcut would build a structurally different request.
func TestGeminiNativeEnvelopeAdversarialRoundTrip(t *testing.T) {
	conversation := geminiNativeProbeConversation(
		t,
		"HIGH",
		"Call the probe tool with value 7.",
	)
	defer conversation.close()
	if _, err := conversation.accept(
		geminiNativeFixture(t, "n2-toolcall.json"),
	); err != nil {
		t.Fatal(err)
	}
	adversarial := `{"a":1,"nested":{"ok":true}}`
	if err := conversation.appendResults([]providerToolResult{{
		ID: "call_4392591", Name: "probe", Content: []byte(adversarial),
	}}); err != nil {
		t.Fatal(err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	built := geminiNativeJSONValue(t, request.Body)
	recorded := geminiNativeJSONValue(
		t,
		geminiNativeFixture(t, "n6-envelope-adversarial-req.json"),
	)
	if !reflect.DeepEqual(built, recorded) {
		t.Fatalf(
			"built request != recorded n6 request\nbuilt:    %s\nrecorded: %s",
			request.Body,
			geminiNativeFixture(t, "n6-envelope-adversarial-req.json"),
		)
	}
	var envelope struct {
		Contents []struct {
			Parts []struct {
				FunctionResponse *geminiFunctionResponse `json:"functionResponse"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if json.Unmarshal(request.Body, &envelope) != nil ||
		len(envelope.Contents) != 3 ||
		len(envelope.Contents[2].Parts) != 1 ||
		envelope.Contents[2].Parts[0].FunctionResponse == nil ||
		envelope.Contents[2].Parts[0].FunctionResponse.Response.Result !=
			adversarial {
		t.Fatalf("adversarial envelope = %s", request.Body)
	}
}

// A3: a signature-less functionCall part fails closed with the labelled error
// on the model family that requires thought signatures.
func TestGeminiNativeThoughtSignatureFailsClosed(t *testing.T) {
	conversation := geminiNativeProbeConversation(
		t,
		"",
		"Call the probe tool with value 7.",
	)
	defer conversation.close()
	var value map[string]any
	if err := json.Unmarshal(
		geminiNativeFixture(t, "n2-toolcall.json"),
		&value,
	); err != nil {
		t.Fatal(err)
	}
	part := value["candidates"].([]any)[0].(map[string]any)["content"].(map[string]any)["parts"].([]any)[0].(map[string]any)
	delete(part, "thoughtSignature")
	missing, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.accept(missing); !IsCode(
		err,
		"CONTINUATION_INVALID",
	) {
		t.Fatalf("signature-less functionCall error = %v", err)
	}
}

// A4: usage maps completely across the recorded n5 pair - the second call's
// 12,263-of-15,912 cached read and its 1,779 reasoning tokens - and reasoning
// sums across turns exactly like cache reads into the journal receipt.
func TestGeminiNativeUsageMapsReasoningAndCache(t *testing.T) {
	conversation := geminiNativeProbeConversation(
		t,
		"",
		"Call the probe tool with value 7.",
	)
	defer conversation.close()
	first, err := conversation.accept(
		geminiNativeFixture(t, "n5-cache-large-try1.json"),
	)
	if err != nil || first.Usage == nil ||
		first.Usage.InputTokens != 15912 ||
		first.Usage.OutputTokens != 4 ||
		first.Usage.CacheReadTokens != nil ||
		first.Usage.ReasoningTokens == nil ||
		*first.Usage.ReasoningTokens != 1917 {
		t.Fatalf("n5 try1 = %#v, %v", first, err)
	}
	second, err := conversation.accept(
		geminiNativeFixture(t, "n5-cache-large-try2.json"),
	)
	if err != nil || second.Usage == nil ||
		second.Usage.InputTokens != 15912 ||
		second.Usage.OutputTokens != 4 ||
		second.Usage.CacheReadTokens == nil ||
		*second.Usage.CacheReadTokens != 12263 ||
		second.Usage.ReasoningTokens == nil ||
		*second.Usage.ReasoningTokens != 1779 {
		t.Fatalf("n5 try2 = %#v, %v", second, err)
	}
	var total Usage
	if err := addTurnUsage(&total, first.Usage); err != nil {
		t.Fatal(err)
	}
	if err := addTurnUsage(&total, second.Usage); err != nil {
		t.Fatal(err)
	}
	if total.InputTokens != 31824 || total.OutputTokens != 8 ||
		total.CacheReadTokens == nil || *total.CacheReadTokens != 12263 ||
		total.ReasoningTokens == nil || *total.ReasoningTokens != 3696 {
		t.Fatalf("summed usage = %#v", total)
	}
	receipt, err := NormalizeUsage(&total, nil, "sworn.test")
	if err != nil || receipt.CacheStatus != UsageReported ||
		receipt.CacheReadTokens == nil ||
		*receipt.CacheReadTokens != 12263 ||
		receipt.ReasoningTokens == nil ||
		*receipt.ReasoningTokens != 3696 {
		t.Fatalf("receipt = %#v, %v", receipt, err)
	}
}

// A5: the response decoder admits exactly the recorded field set at every
// structural position and refuses unknowns with labelled errors.
func TestGeminiNativeClosedAllowlistAdmitsRecordedFixtures(t *testing.T) {
	for _, name := range []string{
		"n1-basic.json",
		"n2-toolcall.json",
		"n3-roundtrip-resp.json",
		"n4-cache-try1.json",
		"n4-cache-try2.json",
		"n5-cache-large-try1.json",
		"n5-cache-large-try2.json",
		"n6-envelope-adversarial-resp.json",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			conversation := geminiNativeProbeConversation(
				t,
				"",
				"Call the probe tool with value 7.",
			)
			defer conversation.close()
			if _, err := conversation.accept(
				geminiNativeFixture(t, name),
			); err != nil {
				t.Fatalf("%s accept = %v", name, err)
			}
		})
	}
}

func TestGeminiNativeClosedAllowlistRefusesUnknownFields(t *testing.T) {
	base := string(geminiNativeFixture(t, "n1-basic.json"))
	tests := []struct {
		name string
		body string
		code string
	}{
		{
			name: "root",
			body: strings.Replace(
				base,
				`"modelVersion": "gemini-3.7-flash"`,
				`"modelVersion": "gemini-3.7-flash", "unknownRoot": 1`,
				1,
			),
			code: "CONTINUATION_INVALID",
		},
		{
			name: "candidate",
			body: strings.Replace(
				base,
				`"finishReason": "STOP",`,
				`"finishReason": "STOP", "unknownCandidate": 1,`,
				1,
			),
			code: "CONTINUATION_INVALID",
		},
		{
			name: "content",
			body: strings.Replace(
				base,
				`"role": "model"`,
				`"role": "model", "unknownContent": 1`,
				1,
			),
			code: "CONTINUATION_INVALID",
		},
		{
			name: "part",
			body: strings.Replace(
				base,
				`"text": "OK.",`,
				`"text": "OK.", "executableCode": {"code": "x"},`,
				1,
			),
			code: "CONTINUATION_INVALID",
		},
		{
			name: "usage",
			body: strings.Replace(
				base,
				`"serviceTier": "standard"`,
				`"serviceTier": "standard", "unknownUsage": 1`,
				1,
			),
			code: "INVALID_USAGE",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			conversation := geminiNativeProbeConversation(
				t,
				"",
				"Call the probe tool with value 7.",
			)
			defer conversation.close()
			if _, err := conversation.accept([]byte(test.body)); !IsCode(
				err,
				test.code,
			) {
				t.Fatalf("%s error = %v, want %s", test.name, err, test.code)
			}
		})
	}
}

// A6: driver certification passes for a google-native profile with
// gemini-3.7-flash against the recorded certification fixtures reduced from
// n1-n5. The evolved Gemini adapter carries the google_native adapter key and
// the google-native profile key; the OpenAI-compat surface keeps its own
// certification untouched.
func TestGeminiNativeCertificationPassesForGoogleNativeProfile(t *testing.T) {
	var probe ProfileLiveProbe
	var adapter Adapter
	probe = func(_ context.Context, _ string, model string) error {
		conversation, err := newGeminiConversation(
			"https://generativelanguage.example.invalid",
			model,
			[]providerToolDefinition{{
				Name:        "probe",
				Description: "probe tool",
				InputSchema: []byte(
					`{"type":"object","properties":{"value":{"type":"number"}},"required":["value"]}`,
				),
			}},
			[]byte(`Call the probe tool with value 7.`),
			2000,
			"LOW",
		)
		if err != nil {
			return err
		}
		defer conversation.close()
		// Reduced n1-n5: prose, tool call, tool result, cached continuation.
		if _, err := conversation.accept(
			geminiNativeFixture(t, "n1-basic.json"),
		); err != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		turn, err := conversation.accept(
			geminiNativeFixture(t, "n2-toolcall.json"),
		)
		if err != nil || len(turn.Calls) != 1 {
			return fail("LIVE_PROBE_FAILED")
		}
		if err := conversation.appendResults([]providerToolResult{{
			ID: turn.Calls[0].ID, Name: turn.Calls[0].Name,
			Content: []byte("49"),
		}}); err != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		if _, err := conversation.accept(
			geminiNativeFixture(t, "n3-roundtrip-resp.json"),
		); err != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		if _, err := conversation.accept(
			geminiNativeFixture(t, "n4-cache-try1.json"),
		); err != nil {
			return fail("LIVE_PROBE_FAILED")
		}
		cached, err := conversation.accept(
			geminiNativeFixture(t, "n5-cache-large-try2.json"),
		)
		if err != nil || cached.Usage == nil ||
			cached.Usage.CacheReadTokens == nil ||
			*cached.Usage.CacheReadTokens != 12263 ||
			cached.Usage.ReasoningTokens == nil ||
			*cached.Usage.ReasoningTokens != 1779 {
			return fail("LIVE_PROBE_FAILED")
		}
		return nil
	}
	var err error
	adapter, err = NewGeminiAdapter(
		HTTPProfileConfig{
			Key: "google_native", ID: "sworn.google.native",
			Version:          "1.0.0",
			Endpoint:         "https://generativelanguage.example.invalid",
			CredentialHeader: "x-goog-api-key",
			CredentialPrefix: "",
			CredentialRefs:   []string{"credential-ref"},
			ResponseBytes:    MaxProviderResponseBytes,
			ThinkingLevel:    "HIGH",
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("google-native-secret"), nil
		},
		probe,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Identity().Key != "google_native" {
		t.Fatalf("adapter key = %q", adapter.Identity().Key)
	}
	ref := "credential-ref"
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: "google-native", Adapter: adapter.Identity().Key,
			Network: NetworkRequired, CredentialRef: &ref,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	report := registry.Certify(
		context.Background(),
		"google-native",
		"gemini-3.7-flash",
	)
	if report.State != ReadinessPass || report.Code != "live_probe_passed" ||
		report.Family != ProfileGemini ||
		report.Profile != "google-native" ||
		report.Model != "gemini-3.7-flash" {
		t.Fatalf("google-native certification report = %#v", report)
	}
	// The request built by the certified adapter carries the operator's
	// configured thinking level (never a hardcoded default).
	conversation, err := adapter.(*loopAdapter).new(
		[]byte(`Call the probe tool with value 7.`),
		"gemini-3.7-flash",
		[]providerToolDefinition{{
			Name:        "probe",
			Description: "probe tool",
			InputSchema: []byte(`{"type":"object"}`),
		}},
		Limits{TimeoutMillis: 5_000, OutputBytes: 2000},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.close()
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(request.Body, []byte(`"thinkingLevel":"HIGH"`)) {
		t.Fatalf("certified adapter thinking knob missing: %s", request.Body)
	}
}

// A7: the one additive reasoning_tokens field is omitempty and backward
// compatible. A legacy receipt with cache, effort, finish and truncation but
// no reasoning re-encodes byte-identically, and a receipt carrying reasoning
// stays byte-stable across re-encoding.
func TestUsageReceiptReasoningTokensIsAdditiveAndByteStable(t *testing.T) {
	legacy := `{"token_status":"reported","input_tokens":7,"output_tokens":5,` +
		`"cost_status":"unavailable","cost_micro_units":null,"currency":null,` +
		`"source":null,"cache_status":"reported","cache_read_tokens":40,` +
		`"effort_requested":"high","effort_reported":"high",` +
		`"finish_reason":"length","truncated":true}`
	var receipt UsageReceipt
	if err := json.Unmarshal([]byte(legacy), &receipt); err != nil {
		t.Fatal(err)
	}
	body, err := EncodeUsageReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != legacy {
		t.Fatalf("legacy re-encode changed:\n%s\nwant:\n%s", body, legacy)
	}
	if bytes.Contains(body, []byte("reasoning_tokens")) {
		t.Fatalf("legacy receipt gained reasoning_tokens: %s", body)
	}
	var again UsageReceipt
	if err := json.Unmarshal(body, &again); err != nil {
		t.Fatal(err)
	}
	body2, err := EncodeUsageReceipt(again)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, body2) {
		t.Fatalf("legacy receipt not byte-stable:\n%s\n%s", body, body2)
	}

	// A receipt carrying reasoning encodes the additive field and stays
	// byte-stable across re-encoding.
	reasoning := int64(1779)
	input, output, cachedRead := int64(15912), int64(4), int64(12263)
	reported := UsageReceipt{
		TokenStatus:     UsageReported,
		InputTokens:     &input,
		OutputTokens:    &output,
		CostStatus:      UsageUnavailable,
		CacheStatus:     UsageReported,
		CacheReadTokens: &cachedRead,
		ReasoningTokens: &reasoning,
	}
	withReasoning, err := EncodeUsageReceipt(reported)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(withReasoning, []byte(`"reasoning_tokens":1779`)) {
		t.Fatalf("reasoning_tokens missing: %s", withReasoning)
	}
	var reparsed UsageReceipt
	if err := json.Unmarshal(withReasoning, &reparsed); err != nil {
		t.Fatal(err)
	}
	withReasoningAgain, err := EncodeUsageReceipt(reparsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(withReasoning, withReasoningAgain) {
		t.Fatalf(
			"reasoning receipt not byte-stable:\n%s\n%s",
			withReasoning,
			withReasoningAgain,
		)
	}
}
