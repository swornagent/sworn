package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// economyScriptedTransport serves a canned chat-completions response for
// every request. Each turn carries one Read tool call with a unique call id
// (the conversation rejects duplicate ids) and a fixed token usage.
type economyScriptedTransport struct {
	inputTokens  int64
	outputTokens int64
	turn         atomic.Int64
	// terminal serves a sworn_submit terminal call on the named turn
	// instead of the Read call.
	terminalTurn      int64
	terminalArguments string
}

func (transport *economyScriptedTransport) roundTrip(
	_ context.Context,
	_ *string,
	_ providerRequest,
) ([]byte, error) {
	turn := transport.turn.Add(1)
	if transport.terminalTurn > 0 && turn == transport.terminalTurn {
		return mustJSONMap(openAIToolCallResponse(
			"terminal-call",
			"sworn_submit",
			transport.terminalArguments,
			transport.inputTokens,
			transport.outputTokens,
		)), nil
	}
	return mustJSONMap(openAIToolCallResponse(
		"read-call-"+itoa(int(turn)),
		"Read",
		`{"path":"/workspace/economy-tool-target"}`,
		transport.inputTokens,
		transport.outputTokens,
	)), nil
}

func (*economyScriptedTransport) check(
	context.Context,
	profileCheckKind,
	*string,
	string,
) (ReadinessState, string) {
	return ReadinessPass, "test"
}

func mustJSONMap(value map[string]any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

// economyBlockingTransport blocks until the request context ends, then
// returns the context error: the wall-clock deadline is the only thing that
// can end a dispatch through it.
type economyBlockingTransport struct{}

func (*economyBlockingTransport) roundTrip(
	ctx context.Context,
	_ *string,
	_ providerRequest,
) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*economyBlockingTransport) check(
	context.Context,
	profileCheckKind,
	*string,
	string,
) (ReadinessState, string) {
	return ReadinessPass, "test"
}

// economyTestAdapter builds a real OpenAI chat-completions loop adapter
// whose transport is replaced by the test script, and writes the Read target
// file into a host workspace it returns.
func economyTestAdapter(
	t *testing.T,
	transport providerTransport,
) (Adapter, string) {
	t.Helper()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-economy", ID: "sworn.openai.economy",
				Version:          "1.0.0",
				Endpoint:         "https://provider.example.invalid/chat/completions",
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
	loop, ok := adapter.(*loopAdapter)
	if !ok {
		t.Fatalf("adapter type = %T", adapter)
	}
	loop.transport = transport
	host := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(host, "economy-tool-target"),
		[]byte("economy tool target"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	return adapter, host
}

func economyInvocationFixture(
	t *testing.T,
	adapter Adapter,
	invocationID string,
	limits Limits,
	host string,
) Invocation {
	t.Helper()
	ref := "credential-ref"
	profile := ProfileConfig{
		Key: "economy-profile", Adapter: adapter.Identity().Key,
		Network: NetworkRequired, CredentialRef: &ref,
	}
	selected := SelectedProfile{
		Profile: profile, Adapter: adapter.Identity(), Model: "exact-model",
		adapter: adapter,
	}
	request, err := NewRequest(
		invocationID,
		RoleImplementer,
		profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: ReadWrite},
		nil,
		true,
		limits,
	)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		ContainmentReadWrite,
		ImplementerImplementation,
	)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request: request, HostWorkspace: host,
		Selected: selected, Permission: permission,
	}
}

func TestEconomyTurnBudgetCrossingFailsAtLoopTopWithReceipt(t *testing.T) {
	t.Parallel()
	const budget = 4
	transport := &economyScriptedTransport{
		inputTokens:  10,
		outputTokens: 20,
	}
	adapter, host := economyTestAdapter(t, transport)
	invocation := economyInvocationFixture(
		t,
		adapter,
		"economy-turn-crossing",
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536, MaxTurnsPerWork: budget},
		host,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "ECONOMY_TURN_BUDGET_EXCEEDED") {
		t.Fatalf("error = %v, want ECONOMY_TURN_BUDGET_EXCEEDED", err)
	}
	if observation.TransportStatus != RunnerError ||
		observation.Diagnostic.Code != "economy_turn_budget" ||
		observation.Handoff != nil {
		t.Fatalf("observation = %#v", observation)
	}
	usage := observation.Usage
	if usage.Turns == nil || *usage.Turns != budget ||
		usage.ToolCalls == nil || *usage.ToolCalls != budget ||
		usage.OutputTokens == nil || *usage.OutputTokens != budget*20 ||
		usage.InputTokens == nil || *usage.InputTokens != budget*10 {
		t.Fatalf("crossing usage = %#v", usage)
	}
	// No request is ever issued past the budget boundary.
	if got := transport.turn.Load(); got != budget {
		t.Fatalf("requests issued = %d, want %d", got, budget)
	}
}

func TestEconomyOutputTokenBudgetCrossingReportsAccumulatedTokens(t *testing.T) {
	t.Parallel()
	transport := &economyScriptedTransport{
		inputTokens:  5,
		outputTokens: 30,
	}
	adapter, host := economyTestAdapter(t, transport)
	invocation := economyInvocationFixture(
		t,
		adapter,
		"economy-token-crossing",
		Limits{
			TimeoutMillis:          5_000,
			OutputBytes:            65_536,
			MaxOutputTokensPerWork: 100,
		},
		host,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "ECONOMY_OUTPUT_BUDGET_EXCEEDED") {
		t.Fatalf("error = %v, want ECONOMY_OUTPUT_BUDGET_EXCEEDED", err)
	}
	if observation.TransportStatus != RunnerError ||
		observation.Diagnostic.Code != "economy_output_budget" ||
		observation.Handoff != nil {
		t.Fatalf("observation = %#v", observation)
	}
	usage := observation.Usage
	// Four turns of 30 tokens accumulate 120 >= 100; the fifth request is
	// never sent and the receipt carries the accumulated facts.
	if usage.Turns == nil || *usage.Turns != 4 ||
		usage.ToolCalls == nil || *usage.ToolCalls != 4 ||
		usage.OutputTokens == nil || *usage.OutputTokens != 120 {
		t.Fatalf("crossing usage = %#v", usage)
	}
	if got := transport.turn.Load(); got != 4 {
		t.Fatalf("requests issued = %d, want 4", got)
	}
}

func TestTerminalSubmitLandingExactlyAtBudgetCompletes(t *testing.T) {
	t.Parallel()
	const budget = 4
	submission := submissionFixture(
		t,
		"economy-terminal-boundary",
		ImplementerImplementation,
		"",
	)
	transport := &economyScriptedTransport{
		inputTokens:       7,
		outputTokens:      5,
		terminalTurn:      budget,
		terminalArguments: submissionToolArguments(t, submission),
	}
	adapter, host := economyTestAdapter(t, transport)
	invocation := economyInvocationFixture(
		t,
		adapter,
		"economy-terminal-boundary",
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536, MaxTurnsPerWork: budget},
		host,
	)
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil {
		t.Fatalf("terminal turn at the budget boundary failed: %v", err)
	}
	if observation.Handoff == nil ||
		observation.TransportStatus != Completed {
		t.Fatalf("observation = %#v", observation)
	}
	if got := transport.turn.Load(); got != budget {
		t.Fatalf("requests issued = %d, want %d", got, budget)
	}
}

func TestTimeoutMillisBoundsAPIConversationWallClock(t *testing.T) {
	t.Parallel()
	transport := &economyBlockingTransport{}
	adapter, host := economyTestAdapter(t, transport)
	invocation := economyInvocationFixture(
		t,
		adapter,
		"economy-deadline",
		Limits{TimeoutMillis: 50, OutputBytes: 65_536},
		host,
	)
	started := time.Now()
	_, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	elapsed := time.Since(started)
	if !IsCode(err, "INVOCATION_TIMEOUT") {
		t.Fatalf("error = %v, want INVOCATION_TIMEOUT", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("wall clock = %s, want bounded by TimeoutMillis", elapsed)
	}
	if elapsed < 30*time.Millisecond {
		t.Fatalf("wall clock = %s, deadline did not bind the transport", elapsed)
	}
}

func TestContinuationInvokeDeadlineBoundsConversation(t *testing.T) {
	t.Parallel()
	transport := &economyBlockingTransport{}
	adapter, host := economyTestAdapter(t, transport)
	invocation := economyInvocationFixture(
		t,
		adapter,
		"economy-continuation-deadline",
		Limits{TimeoutMillis: 50, OutputBytes: 65_536},
		host,
	)
	loop := adapter.(*loopAdapter)
	started := time.Now()
	_, _, err := loop.invokeContinuation(
		context.Background(),
		invocation,
	)
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 5*time.Second || elapsed < 30*time.Millisecond {
		t.Fatalf("wall clock = %s, want bounded by TimeoutMillis", elapsed)
	}
}

func TestOutputBytesWiredToRecordedRequestSurfaces(t *testing.T) {
	t.Parallel()
	const limit = 65_536

	chat, err := newOpenAIConversation(
		"https://provider.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		providerDialectOpenAIChat,
		"",
		limit,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer chat.close()
	request, err := chat.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(request.Body, []byte(`"max_completion_tokens":65536`)) {
		t.Fatalf("openai chat request lacks the output limit: %s", request.Body)
	}

	// The xAI chat dialect has no recorded wire vocabulary for the field
	// and stays deliberately unwired.
	xai, err := newOpenAIConversation(
		"https://provider.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		providerDialectXAIChat,
		"",
		limit,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer xai.close()
	request, err = xai.request()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(request.Body, []byte("max_completion_tokens")) {
		t.Fatalf("xAI chat request carries unrecorded vocabulary: %s", request.Body)
	}

	for _, dialect := range []providerDialect{
		providerDialectOpenAIResponses,
		providerDialectXAIResponses,
	} {
		responses, responsesErr := newResponsesConversation(
			"https://provider.example.invalid/v1/responses",
			"exact-model",
			toolDefinitions(ReadWrite),
			[]byte(`{}`),
			"medium",
			nil,
			false,
			dialect,
			limit,
		)
		if responsesErr != nil {
			t.Fatal(responsesErr)
		}
		responsesRequest, responsesReqErr := responses.request()
		responses.close()
		if responsesReqErr != nil {
			t.Fatal(responsesReqErr)
		}
		if !bytes.Contains(
			responsesRequest.Body,
			[]byte(`"max_output_tokens":65536`),
		) {
			t.Fatalf("%s request lacks the output limit: %s", dialect, responsesRequest.Body)
		}
	}

	bedrock, err := newBedrockConversation(
		BedrockProfileConfig{Endpoint: "https://bedrock-runtime.us-east-1.amazonaws.com"},
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		limit,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bedrock.close()
	request, err = bedrock.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(request.Body, []byte(`"inferenceConfig":{"maxTokens":65536}`)) {
		t.Fatalf("bedrock request lacks the output limit: %s", request.Body)
	}
}

// TestContextWindowClampLowersOutputCeilingAfterReportedUsage anchors A2's
// clamp arithmetic on the chat-completions surface: the first request of a
// dispatch is unaffected (no prior turn to clamp against, byte-identical to
// an unclamped ceiling), and the request after an accepted turn clamps
// max_completion_tokens to context_window_tokens - last_input_tokens - the
// declared safety margin, strictly below the configured ceiling.
func TestContextWindowClampLowersOutputCeilingAfterReportedUsage(t *testing.T) {
	t.Parallel()
	const ceiling = 100_000
	const window = 50_000
	chat, err := newOpenAIConversation(
		"https://provider.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		providerDialectOpenAIChat,
		"",
		ceiling,
		window,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer chat.close()
	first, err := chat.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first.Body, []byte(`"max_completion_tokens":100000`)) {
		t.Fatalf("first request clamped before any turn reported usage: %s", first.Body)
	}
	if _, err := chat.accept([]byte(
		`{"choices":[{"message":{"role":"assistant","content":"chosen."},` +
			`"finish_reason":"stop"}],"usage":{"prompt_tokens":20000,"completion_tokens":5}}`,
	)); err != nil {
		t.Fatal(err)
	}
	second, err := chat.request()
	if err != nil {
		t.Fatal(err)
	}
	// room = 50000 - 20000 - 1024 (margin) = 28976, below the 100000 ceiling.
	if !bytes.Contains(second.Body, []byte(`"max_completion_tokens":28976`)) {
		t.Fatalf("second request = %s, want max_completion_tokens:28976", second.Body)
	}
}

// TestContextWindowClampAppliesToResponsesSurface anchors the identical
// arithmetic on the responses surface, and that a ceiling stricter than the
// clamped room stays unchanged (the clamp only ever lowers, never raises).
func TestContextWindowClampAppliesToResponsesSurface(t *testing.T) {
	t.Parallel()
	const ceiling = 10_000
	const window = 50_000
	responses, err := newResponsesConversation(
		"https://provider.example.invalid/v1/responses",
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		"medium",
		nil,
		false,
		providerDialectOpenAIResponses,
		ceiling,
		window,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer responses.close()
	first, err := responses.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first.Body, []byte(`"max_output_tokens":10000`)) {
		t.Fatalf("first request clamped before any turn reported usage: %s", first.Body)
	}
	if _, err := responses.accept([]byte(
		`{"id":"r","object":"response","status":"completed","error":null,"output":[` +
			`{"type":"message","role":"assistant","status":"completed",` +
			`"content":[{"type":"output_text","text":"ok"}]}],` +
			`"usage":{"input_tokens":20000,"output_tokens":5,"total_tokens":20005}}`,
	)); err != nil {
		t.Fatal(err)
	}
	second, err := responses.request()
	if err != nil {
		t.Fatal(err)
	}
	// room = 50000 - 20000 - 1024 = 28976, above the 10000 ceiling: the
	// ceiling stays the stricter, unchanged value.
	if !bytes.Contains(second.Body, []byte(`"max_output_tokens":10000`)) {
		t.Fatalf("second request = %s, want the unchanged ceiling max_output_tokens:10000", second.Body)
	}
}

// TestContextWindowUnsetFieldRequestsStayByteIdentical anchors A2's
// constraint that an unset context_window_tokens leaves the sent output
// ceiling exactly today's configured value, even after a turn reports a
// huge input-token usage that would otherwise force a deep clamp: the
// clamp helper is disabled entirely, not merely never triggered by chance.
// (The request body as a whole necessarily grows turn to turn - each
// accepted turn appends its own message - so the ceiling field alone, not
// the full body, is the byte-identical fact this constraint names.)
func TestContextWindowUnsetFieldRequestsStayByteIdentical(t *testing.T) {
	t.Parallel()
	chat, err := newOpenAIConversation(
		"https://provider.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		providerDialectOpenAIChat,
		"",
		65_536,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer chat.close()
	first, err := chat.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first.Body, []byte(`"max_completion_tokens":65536`)) {
		t.Fatalf("first request = %s, want max_completion_tokens:65536", first.Body)
	}
	if _, err := chat.accept([]byte(
		`{"choices":[{"message":{"role":"assistant","content":"chosen."},` +
			`"finish_reason":"stop"}],"usage":{"prompt_tokens":999999,"completion_tokens":5}}`,
	)); err != nil {
		t.Fatal(err)
	}
	second, err := chat.request()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(second.Body, []byte(`"max_completion_tokens":65536`)) {
		t.Fatalf("second request = %s, want the unchanged max_completion_tokens:65536 (clamp disabled)", second.Body)
	}
}

// TestContextWindowClampRefusesRequestBelowMinimalOutput anchors A3: when
// the room left cannot fit the declared minimal output, request() refuses
// before sending, with the typed code and a detail naming the window, the
// last input tokens, and the ceiling.
func TestContextWindowClampRefusesRequestBelowMinimalOutput(t *testing.T) {
	t.Parallel()
	const ceiling = 100_000
	const window = 50_000
	chat, err := newOpenAIConversation(
		"https://provider.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		providerDialectOpenAIChat,
		"",
		ceiling,
		window,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer chat.close()
	if _, err := chat.accept([]byte(
		`{"choices":[{"message":{"role":"assistant","content":"chosen."},` +
			`"finish_reason":"stop"}],"usage":{"prompt_tokens":49900,"completion_tokens":5}}`,
	)); err != nil {
		t.Fatal(err)
	}
	_, requestErr := chat.request()
	if !IsCode(requestErr, "ECONOMY_CONTEXT_EXHAUSTED") {
		t.Fatalf("error = %v, want ECONOMY_CONTEXT_EXHAUSTED", requestErr)
	}
	var contractErr *ContractError
	if !errors.As(requestErr, &contractErr) {
		t.Fatal("error is not a *ContractError")
	}
	for _, want := range []string{
		"context_window_tokens=50000", "last_input_tokens=49900", "ceiling=100000", "fix:",
	} {
		if !strings.Contains(contractErr.Detail, want) {
			t.Fatalf("detail = %q, want it to contain %q", contractErr.Detail, want)
		}
	}
	if validateText(contractErr.Detail, maxProviderErrorDetailBytes, false) != nil {
		t.Fatalf("detail fails the bounded provider-detail validation: %q", contractErr.Detail)
	}
}

func TestOptionalOutputLimitRejectsOutOfBoundsValues(t *testing.T) {
	t.Parallel()
	if _, err := newOpenAIConversation(
		"https://provider.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		providerDialectOpenAIChat,
		"",
		MaxProviderOutputBytes+1,
	); err == nil {
		t.Fatal("out-of-bounds output limit admitted")
	}
	if _, err := newOpenAIConversation(
		"https://provider.example.invalid/chat/completions",
		"exact-model",
		toolDefinitions(ReadWrite),
		[]byte(`{}`),
		providerDialectOpenAIChat,
		"",
		-1,
	); err == nil {
		t.Fatal("negative output limit admitted")
	}
}
