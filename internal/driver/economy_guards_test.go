package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
