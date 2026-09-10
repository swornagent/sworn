package driver

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// geminiCorrelateConversation builds a bare native conversation on a model
// whose family requires a thought signature on the first call part, matching
// the lane #291 was recorded on.
func geminiCorrelateConversation(t *testing.T) *geminiConversation {
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
		[]byte(`{"invocation_id":"implementation"}`),
		2000,
		"LOW",
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	return conversation
}

// geminiCallPart renders one functionCall part. An empty id omits the field
// entirely, which is what makes accept synthesise an internal id.
func geminiCallPart(id string, signed bool) string {
	call := `{"name":"probe","args":{"value":7}`
	if id != "" {
		call += `,"id":"` + id + `"`
	}
	call += `}`
	part := `{"functionCall":` + call
	if signed {
		part += `,"thoughtSignature":"c2ln"`
	}
	return part + `}`
}

func geminiCallTurn(parts ...string) []byte {
	return []byte(`{"candidates":[{"content":{"role":"model","parts":[` +
		strings.Join(parts, ",") +
		`]},"finishReason":"STOP"}]}`)
}

// correlateDetailOf decodes the structured Detail a correlation failure now
// carries, after the normalizeAdapterError funnel - the same bytes the
// dispatch record keeps, not the pre-funnel error.
func correlateDetailOf(t *testing.T, err error) correlateFailureDetail {
	t.Helper()
	normalized := normalizeAdapterError(err)
	var contractErr *ContractError
	if !errors.As(normalized, &contractErr) ||
		contractErr.Code != "CONTINUATION_INVALID" {
		t.Fatalf("normalized error = %v, want CONTINUATION_INVALID", normalized)
	}
	var detail correlateFailureDetail
	decoder := json.NewDecoder(strings.NewReader(contractErr.Detail))
	decoder.DisallowUnknownFields()
	if decodeErr := decoder.Decode(&detail); decodeErr != nil {
		t.Fatalf(
			"detail %q did not survive the funnel as an envelope: %v",
			contractErr.Detail,
			decodeErr,
		)
	}
	return detail
}

// A repeated provider function-call id is the wire quirk #291 records: the
// provider returned the same id on two separate turns and the dispatch died
// after 47 minutes. The engine now disambiguates the internal id
// deterministically, and the proof that this is safe is here: the provider's
// own id is replayed byte-for-byte in both model turns, the engine's suffix
// never reaches the wire, the tool results still correlate, and the ledger
// holds both ids exactly once.
func TestGeminiRepeatedProviderCallIDDisambiguatesAndReplaysProviderID(t *testing.T) {
	t.Parallel()
	conversation := geminiCorrelateConversation(t)
	defer conversation.close()

	first, err := conversation.accept(
		geminiCallTurn(geminiCallPart("call_1", true)),
	)
	if err != nil || len(first.Calls) != 1 || first.Calls[0].ID != "call_1" {
		t.Fatalf("first turn = %#v, %v", first, err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "call_1", Name: "probe", Content: []byte("49"),
	}}); err != nil {
		t.Fatal(err)
	}

	second, err := conversation.accept(
		geminiCallTurn(geminiCallPart("call_1", true)),
	)
	if err != nil || len(second.Calls) != 1 {
		t.Fatalf("repeated-id turn = %#v, %v", second, err)
	}
	if second.Calls[0].ID != "call_1#gemini-2-1" {
		t.Fatalf("internal id = %q, want the disambiguated id", second.Calls[0].ID)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: second.Calls[0].ID, Name: "probe", Content: []byte("49"),
	}}); err != nil {
		t.Fatalf("results for the disambiguated call: %v", err)
	}

	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(request.Body, []byte("#gemini-")) {
		t.Fatalf("the engine suffix reached the wire: %s", request.Body)
	}
	var replay struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				FunctionCall *struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if json.Unmarshal(request.Body, &replay) != nil {
		t.Fatalf("replay = %s", request.Body)
	}
	replayed := 0
	for _, content := range replay.Contents {
		for _, part := range content.Parts {
			if part.FunctionCall == nil {
				continue
			}
			replayed++
			if part.FunctionCall.ID != "call_1" {
				t.Fatalf(
					"replayed functionCall id = %q, want the provider's own id",
					part.FunctionCall.ID,
				)
			}
		}
	}
	if replayed != 2 {
		t.Fatalf("replayed function calls = %d, want 2: %s", replayed, request.Body)
	}
	// The ledger holds both ids exactly once: each is now a duplicate, and
	// nothing else was admitted along the way.
	if !IsCode(conversation.ledger.correlate("call_1"), "CONTINUATION_INVALID") ||
		!IsCode(
			conversation.ledger.correlate("call_1#gemini-2-1"),
			"CONTINUATION_INVALID",
		) {
		t.Fatal("ledger did not retain both correlation ids")
	}
	if len(conversation.ledger.ids) != 2 {
		t.Fatalf("ledger ids = %d, want 2", len(conversation.ledger.ids))
	}
}

// The same quirk within a single turn: two function-call parts sharing one
// id. Both calls survive, in order, and both replay the provider's id.
func TestGeminiRepeatedProviderCallIDWithinOneTurn(t *testing.T) {
	t.Parallel()
	conversation := geminiCorrelateConversation(t)
	defer conversation.close()

	turn, err := conversation.accept(geminiCallTurn(
		geminiCallPart("call_1", true),
		geminiCallPart("call_1", false),
	))
	if err != nil || len(turn.Calls) != 2 {
		t.Fatalf("same-turn repeated id = %#v, %v", turn, err)
	}
	if turn.Calls[0].ID != "call_1" ||
		turn.Calls[1].ID != "call_1#gemini-1-2" {
		t.Fatalf("internal ids = %q, %q", turn.Calls[0].ID, turn.Calls[1].ID)
	}
	if err := conversation.appendResults([]providerToolResult{
		{ID: "call_1", Name: "probe", Content: []byte("49")},
		{ID: "call_1#gemini-1-2", Name: "probe", Content: []byte("49")},
	}); err != nil {
		t.Fatalf("results for the same-turn calls: %v", err)
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(request.Body, []byte("#gemini-")) {
		t.Fatalf("the engine suffix reached the wire: %s", request.Body)
	}
	if bytes.Count(request.Body, []byte(`"id":"call_1"`)) != 2 {
		t.Fatalf("provider id not replayed twice: %s", request.Body)
	}
}

// An engine-synthesised id that collides is not a provider quirk and is never
// papered over: it fails, and the failure now names the offending id, its
// provenance, its position, and which side of the turn boundary it clashed
// with - the four facts #291 could not read out of the dispatch record.
func TestGeminiSynthesisedCallIDCollisionRecordsDetail(t *testing.T) {
	t.Parallel()
	conversation := geminiCorrelateConversation(t)
	defer conversation.close()

	// The provider names an id that happens to equal the id the engine will
	// synthesise for the next turn's first call part.
	if _, err := conversation.accept(
		geminiCallTurn(geminiCallPart("gemini-2-1", true)),
	); err != nil {
		t.Fatal(err)
	}
	if err := conversation.appendResults([]providerToolResult{{
		ID: "gemini-2-1", Name: "probe", Content: []byte("49"),
	}}); err != nil {
		t.Fatal(err)
	}

	_, err := conversation.accept(geminiCallTurn(geminiCallPart("", true)))
	if !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("synthesised collision error = %v", err)
	}
	detail := correlateDetailOf(t, err)
	want := correlateFailureDetail{
		Site:           "continuation.gemini.accept_function_correlate_failed",
		Cause:          correlateCauseDuplicate,
		IDSource:       correlateIDSynthesised,
		ID:             "gemini-2-1",
		IDBytes:        len("gemini-2-1"),
		Step:           2,
		Part:           1,
		DuplicateScope: correlateDuplicateEarlierTurn,
	}
	if detail != want {
		t.Fatalf("recorded detail = %#v, want %#v", detail, want)
	}
}

// A provider id already at the correlation bound cannot take the engine's
// suffix. The dispatch still fails - but the record says so explicitly
// (disambiguation_attempted), says the clash was inside this same turn, and
// reports the id by length because its shape is not one this record will
// carry verbatim.
func TestGeminiUndisambiguatableDuplicateRecordsAttempt(t *testing.T) {
	t.Parallel()
	conversation := geminiCorrelateConversation(t)
	defer conversation.close()

	long := strings.Repeat("a", 250)
	_, err := conversation.accept(geminiCallTurn(
		geminiCallPart(long, true),
		geminiCallPart(long, false),
	))
	if !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("undisambiguatable duplicate error = %v", err)
	}
	detail := correlateDetailOf(t, err)
	want := correlateFailureDetail{
		Site:                    "continuation.gemini.accept_function_correlate_failed",
		Cause:                   correlateCauseDuplicate,
		IDSource:                correlateIDProvider,
		IDBytes:                 250,
		Step:                    1,
		Part:                    2,
		DuplicateScope:          correlateDuplicateSameTurn,
		DisambiguationAttempted: true,
	}
	if detail != want {
		t.Fatalf("recorded detail = %#v, want %#v", detail, want)
	}
}

// The funnel is a re-validating seam, not a pipe: engine vocabulary crosses
// it, anything else drops exactly as every CONTINUATION_INVALID Detail did
// before.
func TestContinuationDetailFunnelAdmitsOnlyEngineVocabulary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		detail string
		want   string
	}{
		{
			name:   "plain site label",
			detail: "continuation.gemini.accept_root_invalid",
			want:   "continuation.gemini.accept_root_invalid",
		},
		{
			name: "correlate envelope",
			detail: `{"site":"continuation.gemini.accept_function_correlate_failed",` +
				`"cause":"duplicate_id","id_source":"provider","id":"call_1",` +
				`"id_bytes":6,"step":2,"part":1,"duplicate_scope":"same_turn"}`,
			want: `{"site":"continuation.gemini.accept_function_correlate_failed",` +
				`"cause":"duplicate_id","id_source":"provider","id":"call_1",` +
				`"id_bytes":6,"step":2,"part":1,"duplicate_scope":"same_turn"}`,
		},
		{name: "empty detail", detail: "", want: ""},
		{name: "provider prose", detail: "the model said something long", want: ""},
		{
			name: "envelope naming an unknown site",
			detail: `{"site":"continuation.gemini.invented","cause":"duplicate_id",` +
				`"id_source":"provider","id":"call_1","id_bytes":6,"step":1,"part":1}`,
			want: "",
		},
		{
			name: "envelope carrying an unrecordable id",
			detail: `{"site":"continuation.gemini.accept_function_correlate_failed",` +
				`"cause":"duplicate_id","id_source":"provider",` +
				`"id":"the model wrote a sentence here","id_bytes":31,` +
				`"step":1,"part":1,"duplicate_scope":"same_turn"}`,
			want: "",
		},
		{
			name: "envelope with an unknown field",
			detail: `{"site":"continuation.gemini.accept_function_correlate_failed",` +
				`"cause":"duplicate_id","id_source":"provider","id":"call_1",` +
				`"id_bytes":6,"step":1,"part":1,"prompt":"leaked"}`,
			want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// The Detail rides through a named value so the raised
			// literal is a labelled one: an unlabelled
			// CONTINUATION_INVALID literal is exactly what
			// TestNoUnlabelledContinuationInvalidConstructor forbids.
			detail := test.detail
			normalized := normalizeAdapterError(
				&ContractError{
					Code:   "CONTINUATION_INVALID",
					Detail: detail,
				},
			)
			var contractErr *ContractError
			if !errors.As(normalized, &contractErr) ||
				contractErr.Code != "CONTINUATION_INVALID" {
				t.Fatalf("normalized = %v", normalized)
			}
			if contractErr.Detail != test.want {
				t.Fatalf(
					"funnelled detail = %q, want %q",
					contractErr.Detail,
					test.want,
				)
			}
		})
	}
}
