package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const openRouterFixtureCostMicroUnits int64 = 484

func deleteJSONObjectKeys(t *testing.T, body []byte, path []string, keys ...string) []byte {
	t.Helper()
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	current := root
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("path %v is not an object at %q", path, key)
		}
		next, present := object[key]
		if !present {
			t.Fatalf("path %v missing %q", path, key)
		}
		current = next
	}
	object, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("path %v is not an object", path)
	}
	for _, key := range keys {
		delete(object, key)
	}
	stripped, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return stripped
}

func captureResponsesLiveStream(t *testing.T, fn func()) string {
	t.Helper()
	saved := liveStream
	var buffer bytes.Buffer
	liveStream = &streamRenderer{out: &buffer}
	defer func() { liveStream = saved }()
	fn()
	return buffer.String()
}

func newResponsesFixtureConversation(t *testing.T, dialect providerDialect) *responsesConversation {
	t.Helper()
	conversation, err := newResponsesConversation(
		"https://openrouter.example.invalid/api/v1/responses",
		"qwen/qwen3.8-max",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		"medium",
		nil,
		false,
		dialect,
	)
	if err != nil {
		t.Fatal(err)
	}
	return conversation
}

func newChatFixtureConversation(t *testing.T, dialect providerDialect) *openAIConversation {
	t.Helper()
	conversation, err := newOpenAIConversation(
		"https://openrouter.example.invalid/api/v1/chat/completions",
		"qwen/qwen3.8-max",
		toolDefinitions(ReadOnly),
		[]byte(`{"prompt":"bounded"}`),
		dialect,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	return conversation
}

func requireReportedUSDCost(t *testing.T, cost *CostObservation, micro int64) {
	t.Helper()
	if cost == nil ||
		cost.MicroUnits != micro ||
		cost.Currency != "USD" ||
		cost.Source != CostSourceProviderReported {
		t.Fatalf("cost = %#v, want %d USD provider_reported", cost, micro)
	}
}

func requireReportedCostReceipt(t *testing.T, usage *Usage, cost *CostObservation, micro int64) {
	t.Helper()
	receipt, err := NormalizeUsage(usage, cost, "sworn.test")
	if err != nil ||
		receipt.CostStatus != UsageReported ||
		receipt.CostMicroUnits == nil || *receipt.CostMicroUnits != micro ||
		receipt.Currency == nil || *receipt.Currency != "USD" ||
		receipt.Source == nil || *receipt.Source != CostSourceProviderReported {
		t.Fatalf("receipt = %#v, %v", receipt, err)
	}
}

// A1: OpenRouter usage decorations are admitted on the shipped responses
// decoder and cost is captured as provider-reported USD micro-units.
func TestOpenRouterResponsesUsageDecorationsAreCaptured(t *testing.T) {
	t.Parallel()
	conversation := newResponsesFixtureConversation(t, providerDialectOpenAIResponses)
	defer conversation.close()
	turn, err := conversation.accept(providerDialectFixture(
		t,
		"openrouter_responses_usage.json",
	))
	if err != nil || !turn.Prose || len(turn.Calls) != 0 ||
		turn.Usage == nil ||
		turn.Usage.InputTokens != 52 ||
		turn.Usage.OutputTokens != 44 ||
		turn.Usage.CacheReadTokens == nil ||
		*turn.Usage.CacheReadTokens != 8 ||
		turn.Usage.ReasoningTokens == nil ||
		*turn.Usage.ReasoningTokens != 12 {
		t.Fatalf("openrouter responses turn = %#v, %v", turn, err)
	}
	requireReportedUSDCost(t, turn.Cost, openRouterFixtureCostMicroUnits)
	requireReportedCostReceipt(
		t,
		turn.Usage,
		turn.Cost,
		openRouterFixtureCostMicroUnits,
	)
}

func TestOpenRouterResponsesUnknownUsageSiblingStillFails(t *testing.T) {
	t.Parallel()
	conversation := newResponsesFixtureConversation(t, providerDialectOpenAIResponses)
	defer conversation.close()
	mutated := bytes.Replace(
		providerDialectFixture(t, "openrouter_responses_usage.json"),
		[]byte(`"is_byok": false`),
		[]byte(`"is_byok": false, "unlisted_vendor_field": 1`),
		1,
	)
	if _, err := conversation.accept(mutated); !IsCode(err, "INVALID_USAGE") {
		t.Fatalf("unknown usage sibling error = %v", err)
	}
}

func TestOpenRouterResponsesMalformedCostFailsClosed(t *testing.T) {
	t.Parallel()
	base := providerDialectFixture(t, "openrouter_responses_usage.json")
	cases := map[string][]byte{
		"string":   bytes.Replace(base, []byte(`"cost": 0.000484`), []byte(`"cost": "0.000484"`), 1),
		"negative": bytes.Replace(base, []byte(`"cost": 0.000484`), []byte(`"cost": -0.01`), 1),
		"null":     bytes.Replace(base, []byte(`"cost": 0.000484`), []byte(`"cost": null`), 1),
		"object":   bytes.Replace(base, []byte(`"cost": 0.000484`), []byte(`"cost": {}`), 1),
		"overflow": bytes.Replace(base, []byte(`"cost": 0.000484`), []byte(`"cost": 9007199255`), 1),
	}
	for name, body := range cases {
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			conversation := newResponsesFixtureConversation(t, providerDialectOpenAIResponses)
			defer conversation.close()
			if _, err := conversation.accept(body); !IsCode(err, "INVALID_USAGE") {
				t.Fatalf("%s error = %v", name, err)
			}
		})
	}
}

func TestProviderReportedUSDCostRoundingAndAbsence(t *testing.T) {
	t.Parallel()
	halfUp, err := costObservationFromUSD(json.Number("0.0000005"))
	if err != nil {
		t.Fatal(err)
	}
	requireReportedUSDCost(t, halfUp, 1)
	halfDown, err := costObservationFromUSD(json.Number("0.0000004"))
	if err != nil {
		t.Fatal(err)
	}
	requireReportedUSDCost(t, halfDown, 0)
	integer, err := costObservationFromUSD(json.Number("1"))
	if err != nil {
		t.Fatal(err)
	}
	requireReportedUSDCost(t, integer, 1_000_000)
	absent, err := optionalProviderReportedUSDCost(map[string]any{
		"input_tokens": json.Number("1"),
	})
	if err != nil || absent != nil {
		t.Fatalf("absent cost = %#v, %v", absent, err)
	}
	if _, err := costObservationFromUSD(json.Number("Infinity")); !IsCode(err, "INVALID_USAGE") {
		t.Fatalf("infinity error = %v", err)
	}
	if math.Round(0.5) != 1 {
		t.Fatal("math.Round half-away contract changed")
	}
}

func TestAddTurnCostSumsSameCurrencyAndFailsClosedOtherwise(t *testing.T) {
	t.Parallel()
	var total *CostObservation
	first := &CostObservation{
		MicroUnits: 484,
		Currency:   "USD",
		Source:     CostSourceProviderReported,
	}
	if err := addTurnCost(&total, nil); err != nil || total != nil {
		t.Fatalf("nil turn changed total: %#v, %v", total, err)
	}
	if err := addTurnCost(&total, first); err != nil {
		t.Fatal(err)
	}
	requireReportedUSDCost(t, total, 484)
	second := &CostObservation{
		MicroUnits: 16,
		Currency:   "USD",
		Source:     CostSourceProviderReported,
	}
	if err := addTurnCost(&total, second); err != nil {
		t.Fatal(err)
	}
	requireReportedUSDCost(t, total, 500)

	mixedCurrency := total
	if err := addTurnCost(&mixedCurrency, &CostObservation{
		MicroUnits: 1,
		Currency:   "EUR",
		Source:     CostSourceProviderReported,
	}); !IsCode(err, "INVALID_USAGE") {
		t.Fatalf("mixed currency error = %v", err)
	}
	mixedSource := total
	if err := addTurnCost(&mixedSource, &CostObservation{
		MicroUnits: 1,
		Currency:   "USD",
		Source:     "estimated",
	}); !IsCode(err, "INVALID_USAGE") {
		t.Fatalf("mixed source error = %v", err)
	}
	overflow := &CostObservation{
		MicroUnits: MaxSafeInteger,
		Currency:   "USD",
		Source:     CostSourceProviderReported,
	}
	if err := addTurnCost(&overflow, first); !IsCode(err, "INVALID_USAGE") {
		t.Fatalf("overflow error = %v", err)
	}
}

// A2: a data-only OpenRouter stream falls back to the payload type field.
func TestReadStreamedResponseRecognizesDataOnlyTerminal(t *testing.T) {
	var terminal []byte
	var terminalErr error
	captureResponsesLiveStream(t, func() {
		terminal, terminalErr = readStreamedResponse(
			bytes.NewReader(providerDialectFixture(
				t,
				"openrouter_responses_stream.sse",
			)),
			MaxProviderResponseBytes,
		)
	})
	if terminalErr != nil {
		t.Fatalf("data-only stream = %v", terminalErr)
	}
	conversation := newResponsesFixtureConversation(t, providerDialectOpenAIResponses)
	defer conversation.close()
	turn, err := conversation.accept(terminal)
	if err != nil || !turn.Prose ||
		turn.Usage == nil ||
		turn.Usage.InputTokens != 12 ||
		turn.Usage.OutputTokens != 3 {
		t.Fatalf("accepted data-only terminal = %#v, %v", turn, err)
	}
	requireReportedUSDCost(t, turn.Cost, openRouterFixtureCostMicroUnits)
}

func TestReadStreamedResponseKeepsLabeledTerminal(t *testing.T) {
	var terminal []byte
	var terminalErr error
	captureResponsesLiveStream(t, func() {
		terminal, terminalErr = readStreamedResponse(
			bytes.NewReader(providerDialectFixture(
				t,
				"responses_labeled_stream.sse",
			)),
			MaxProviderResponseBytes,
		)
	})
	if terminalErr != nil {
		t.Fatal(terminalErr)
	}
	want := []byte(
		`{"id":"resp-labeled-01","object":"response","status":"completed","error":null,"output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15}}`,
	)
	if !bytes.Equal(terminal, want) {
		t.Fatalf("labeled terminal = %s", terminal)
	}
}

func TestReadStreamedResponseDataOnlyWithoutTerminalStillFails(t *testing.T) {
	sse := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"x\"}}\n\n" +
		"data: {\"delta\":\"no type\"}\n\n"
	var err error
	captureResponsesLiveStream(t, func() {
		_, err = readStreamedResponse(
			bytes.NewReader([]byte(sse)),
			MaxProviderResponseBytes,
		)
	})
	if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
		t.Fatalf("missing-terminal error = %v", err)
	}
}

func TestReadStreamedResponseEventNameWinsOverPayloadType(t *testing.T) {
	labeledNonTerminal := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"should-not-extract\"}}\n\n"
	var err error
	captureResponsesLiveStream(t, func() {
		_, err = readStreamedResponse(
			bytes.NewReader([]byte(labeledNonTerminal)),
			MaxProviderResponseBytes,
		)
	})
	if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
		t.Fatalf("event: should have won; error = %v", err)
	}

	labeledTerminal := "event: response.completed\n" +
		"data: {\"type\":\"response.output_text.delta\",\"response\":{\"id\":\"resp-labeled-conflict\",\"object\":\"response\",\"status\":\"completed\",\"error\":null,\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n"
	var terminal []byte
	captureResponsesLiveStream(t, func() {
		terminal, err = readStreamedResponse(
			bytes.NewReader([]byte(labeledTerminal)),
			MaxProviderResponseBytes,
		)
	})
	if err != nil || !bytes.Contains(terminal, []byte(`"id":"resp-labeled-conflict"`)) {
		t.Fatalf("labeled conflict terminal = %s, %v", terminal, err)
	}
}

// A3: OpenRouter chat admits the root provider field and the same usage
// decorations; other chat dialects stay closed.
func TestOpenRouterChatAdmitsRootProviderAndUsageDecorations(t *testing.T) {
	t.Parallel()
	body := providerDialectFixture(t, "openrouter_chat_provider.json")
	conversation := newChatFixtureConversation(t, providerDialectOpenRouterChat)
	defer conversation.close()
	turn, err := conversation.accept(body)
	if err != nil || !turn.Prose || len(turn.Calls) != 0 ||
		turn.Usage == nil ||
		turn.Usage.InputTokens != 12 ||
		turn.Usage.OutputTokens != 6 {
		t.Fatalf("openrouter chat turn = %#v, %v", turn, err)
	}
	requireReportedUSDCost(t, turn.Cost, openRouterFixtureCostMicroUnits)
	requireReportedCostReceipt(
		t,
		turn.Usage,
		turn.Cost,
		openRouterFixtureCostMicroUnits,
	)
	if len(conversation.messages) != 2 ||
		bytes.Contains(conversation.messages[1].Content, []byte("Together")) {
		t.Fatalf("provider leaked into continuation: %#v", conversation.messages[1])
	}
	request, err := conversation.request()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(request.Body, []byte(`"provider"`)) ||
		bytes.Contains(request.Body, []byte("Together")) {
		t.Fatalf("provider leaked into replay: %s", request.Body)
	}
}

func TestStrictChatDialectsStillRejectRootProvider(t *testing.T) {
	t.Parallel()
	body := providerDialectFixture(t, "openrouter_chat_provider.json")
	for _, dialect := range []providerDialect{
		providerDialectOpenAIChat,
		providerDialectOpaqueChat,
		providerDialectGoogleChat,
		providerDialectXAIChat,
	} {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			t.Parallel()
			conversation := newChatFixtureConversation(t, dialect)
			defer conversation.close()
			if _, err := conversation.accept(body); !IsCode(err, "CONTINUATION_INVALID") {
				t.Fatalf("%s error = %v", dialect, err)
			}
		})
	}
}

func TestOpenAIChatStillRejectsUsageCostDecorations(t *testing.T) {
	t.Parallel()
	body := deleteJSONObjectKeys(
		t,
		providerDialectFixture(t, "openrouter_chat_provider.json"),
		nil,
		"provider",
	)
	conversation := newChatFixtureConversation(t, providerDialectOpenAIChat)
	defer conversation.close()
	if _, err := conversation.accept(body); !IsCode(err, "INVALID_USAGE") {
		t.Fatalf("strict chat usage error = %v", err)
	}
}

// A4: decoration stays decoration. Only captured cost differs.
func TestOpenRouterResponsesDecorationIdentity(t *testing.T) {
	t.Parallel()
	decoratedBody := providerDialectFixture(t, "openrouter_responses_usage.json")
	// Delete only the three usage decorations so the retained output item
	// stays byte-identical to the recorded fixture.
	strippedBody := bytes.Replace(
		decoratedBody,
		[]byte(",\n    \"cost\": 0.000484,\n    \"cost_details\": {},\n    \"is_byok\": false\n"),
		[]byte("\n"),
		1,
	)
	if bytes.Equal(decoratedBody, strippedBody) ||
		bytes.Contains(strippedBody, []byte(`"cost"`)) {
		t.Fatalf("failed to strip usage decorations: %s", strippedBody)
	}
	decorated := newResponsesFixtureConversation(t, providerDialectOpenAIResponses)
	defer decorated.close()
	stripped := newResponsesFixtureConversation(t, providerDialectOpenAIResponses)
	defer stripped.close()
	decoratedTurn, err := decorated.accept(decoratedBody)
	if err != nil {
		t.Fatal(err)
	}
	strippedTurn, err := stripped.accept(strippedBody)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoratedTurn.Calls, strippedTurn.Calls) ||
		decoratedTurn.Prose != strippedTurn.Prose ||
		!reflect.DeepEqual(decoratedTurn.Usage, strippedTurn.Usage) {
		t.Fatalf(
			"non-cost fields diverged\ndecorated: %#v\nstripped:  %#v",
			decoratedTurn,
			strippedTurn,
		)
	}
	if !bytes.Equal(decorated.input[1], stripped.input[1]) {
		t.Fatalf(
			"continuation input diverged\ndecorated: %s\nstripped:  %s",
			decorated.input[1],
			stripped.input[1],
		)
	}
	if strippedTurn.Cost != nil {
		t.Fatalf("stripped cost = %#v", strippedTurn.Cost)
	}
	requireReportedUSDCost(t, decoratedTurn.Cost, openRouterFixtureCostMicroUnits)
	if decoratedTurn.Usage.CacheReadTokens == nil ||
		decoratedTurn.Cost.MicroUnits == *decoratedTurn.Usage.CacheReadTokens {
		t.Fatal("cost_details or is_byok leaked into CostObservation")
	}
}

func TestOpenRouterChatDecorationIdentity(t *testing.T) {
	t.Parallel()
	decoratedBody := providerDialectFixture(t, "openrouter_chat_provider.json")
	strippedBody := bytes.Replace(
		decoratedBody,
		[]byte("\n  \"provider\": \"Together\",\n"),
		[]byte("\n"),
		1,
	)
	if bytes.Equal(decoratedBody, strippedBody) ||
		bytes.Contains(strippedBody, []byte(`"provider"`)) {
		t.Fatalf("failed to strip provider: %s", strippedBody)
	}
	decorated := newChatFixtureConversation(t, providerDialectOpenRouterChat)
	defer decorated.close()
	stripped := newChatFixtureConversation(t, providerDialectOpenRouterChat)
	defer stripped.close()
	decoratedTurn, err := decorated.accept(decoratedBody)
	if err != nil {
		t.Fatal(err)
	}
	strippedTurn, err := stripped.accept(strippedBody)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoratedTurn, strippedTurn) {
		t.Fatalf(
			"turns diverged after dropping provider\ndecorated: %#v\nstripped:  %#v",
			decoratedTurn,
			strippedTurn,
		)
	}
	if !bytes.Equal(decorated.messages[1].Content, stripped.messages[1].Content) {
		t.Fatalf(
			"replayed content diverged: %s vs %s",
			decorated.messages[1].Content,
			stripped.messages[1].Content,
		)
	}
}

func TestXAICostInUSDTicksStillDoesNotBecomeCostObservation(t *testing.T) {
	t.Parallel()
	conversation := newResponsesFixtureConversation(t, providerDialectXAIResponses)
	defer conversation.close()
	turn, err := conversation.accept(providerDialectFixture(
		t,
		"grok_responses_response_usage.json",
	))
	if err != nil || turn.Cost != nil {
		t.Fatalf("xAI turn cost = %#v, %v", turn.Cost, err)
	}
	receipt, err := NormalizeUsage(turn.Usage, turn.Cost, "sworn.test")
	if err != nil ||
		receipt.CostStatus != UsageUnavailable ||
		receipt.CostMicroUnits != nil ||
		receipt.Source != nil {
		t.Fatalf("xAI receipt = %#v, %v", receipt, err)
	}
}

// A1 never-discarded: captured cost reaches NormalizeUsage on the dispatch
// loop, not just the parser.
func TestRunConversationReportsCapturedOpenRouterCost(t *testing.T) {
	submission := submissionFixture(
		t,
		"openrouter-cost-loop",
		ImplementerImplementation,
		"",
	)
	arguments := dialectToleranceSubmissionArguments(t, submission)
	transport := &openRouterCostTransport{arguments: arguments}
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-openrouter-cost", ID: "sworn.openai.openrouter.cost",
				Version:          "1.0.0",
				Endpoint:         "https://openrouter.example.invalid/api/v1/responses",
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
	loop := adapter.(*loopAdapter)
	loop.transport = transport
	host := t.TempDir()
	invocation := economyInvocationFixture(
		t,
		adapter,
		"openrouter-cost-loop",
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536},
		host,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil {
		t.Fatalf("observation = %#v, %v", observation, err)
	}
	usage := observation.Usage
	if usage.CostStatus != UsageReported ||
		usage.CostMicroUnits == nil ||
		*usage.CostMicroUnits != openRouterFixtureCostMicroUnits ||
		usage.Currency == nil || *usage.Currency != "USD" ||
		usage.Source == nil || *usage.Source != CostSourceProviderReported {
		t.Fatalf("loop discarded cost: %#v", usage)
	}
}

type openRouterCostTransport struct {
	arguments string
}

func dialectToleranceSubmissionArguments(
	t *testing.T,
	submission Submission,
) string {
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

func (transport *openRouterCostTransport) roundTrip(
	context.Context,
	*string,
	providerRequest,
) ([]byte, error) {
	arguments, err := json.Marshal(transport.arguments)
	if err != nil {
		return nil, err
	}
	return []byte(`{"id":"resp-openrouter-loop-01","object":"response","status":"completed","error":null,"output":[{"type":"function_call","id":"item-1","call_id":"submit-1","name":"sworn_submit","arguments":` +
		string(arguments) +
		`,"status":"completed"}],"usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12,"cost":0.000484,"cost_details":{},"is_byok":false}}`), nil
}

func (*openRouterCostTransport) check(
	context.Context,
	profileCheckKind,
	*string,
	string,
) (ReadinessState, string) {
	return ReadinessPass, "test"
}

func TestOpenRouterFixturesArePresentBesideExistingDialectHome(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"openrouter_responses_usage.json",
		"openrouter_chat_provider.json",
		"openrouter_responses_stream.sse",
		"responses_labeled_stream.sse",
	} {
		if _, err := os.Stat(filepath.Join("testdata", "provider_dialects", name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
	if !strings.Contains(
		string(providerDialectFixture(t, "openrouter_responses_usage.json")),
		`"cost"`,
	) {
		t.Fatal("responses fixture lost the named cost decoration")
	}
}
