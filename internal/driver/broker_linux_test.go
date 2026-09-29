//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNativeBrokerRefusedCorrectionClosesWithoutResultBytes(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.RecoveryStepHook = func(
		_ context.Context,
		kind RecoveryStepKind,
		_ *SubmitRefusal,
	) error {
		if kind != RecoveryStepSubmissionCorrection {
			t.Fatalf("recovery kind = %s", kind)
		}
		return fail("TEST_REFUSAL")
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	capability := broker.capability()
	defer clearBytes(capability)
	openNativeBrokerForTest(t, broker, capability)

	malformed := toolCallRequest(
		1,
		"sworn_submit",
		map[string]any{"submission": map[string]any{}},
	)
	status, body := brokerRequest(t, broker, capability, malformed)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"closed"`)) ||
		bytes.Contains(body, []byte(`"result"`)) ||
		bytes.Contains(body, []byte(`"content"`)) ||
		bytes.Contains(body, []byte("error:")) {
		t.Fatalf("refused correction = %d %s", status, body)
	}
	select {
	case <-broker.Terminal():
	default:
		t.Fatal("refused correction did not close broker")
	}
	status, body = brokerRequest(t, broker, capability, malformed)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"closed"`)) {
		t.Fatalf("second call = %d %s", status, body)
	}
}

func TestNativeBrokerEnforcesExactCapabilityStateAndTerminalProtocol(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	// Planner plan bytes may leave the driver only from the answer-resume
	// shape, so the broker session models the answered turn.
	invocation.recoverableInput = &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "resume answer",
	}
	if err := osWriteProviderFixture(
		invocation.HostWorkspace,
		"broker.txt",
		"broker body",
	); err != nil {
		t.Fatal(err)
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	capability := broker.capability()
	defer clearBytes(capability)
	defer broker.Close()
	var responseBodies [][]byte

	status, body := brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name": "codex", "version": CodexCLIVersion,
				},
			},
		},
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusOK ||
		!bytes.Contains(body, []byte(`"protocolVersion":"2025-06-18"`)) {
		t.Fatalf("initialize = %d %s", status, body)
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "method": "notifications/initialized",
			"params": map[string]any{},
		},
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusAccepted {
		t.Fatalf("initialized notification = %d %s", status, body)
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "tools/list",
			"params": map[string]any{},
		},
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusOK ||
		!bytes.Contains(body, []byte(`"name":"Read"`)) ||
		!bytes.Contains(body, []byte(`"name":"Write"`)) {
		t.Fatalf("tools/list = %d %s", status, body)
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		toolCallRequest(3, "Read", map[string]any{
			"path": GuestWorkspacePath + "/broker.txt",
		}),
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"not_open"`)) {
		t.Fatalf("pre-open call = %d %s", status, body)
	}
	if err := broker.Arm(); err != nil {
		t.Fatal(err)
	}
	openCall := toolCallRequest(4, "Read", map[string]any{
		"path": GuestWorkspacePath + "/broker.txt",
	})
	openCall["params"].(map[string]any)["_meta"] = map[string]any{
		"claudecode/toolUseId": "tool-1",
		"progressToken":        1,
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		openCall,
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusOK ||
		!bytes.Contains(body, []byte(`"text":"broker body"`)) {
		t.Fatalf("open call = %d %s", status, body)
	}
	for name, value := range map[string]any{
		"scalar metadata": "invalid",
		"unknown sibling": map[string]any{},
	} {
		request := toolCallRequest(40, "Read", map[string]any{
			"path": GuestWorkspacePath + "/broker.txt",
		})
		params := request["params"].(map[string]any)
		if name == "scalar metadata" {
			params["_meta"] = value
		} else {
			params["unknown"] = value
		}
		status, body = brokerRequest(t, broker, capability, request)
		if status != http.StatusBadRequest ||
			!bytes.Contains(body, []byte(`"message":"invalid_params"`)) {
			t.Fatalf("%s = %d %s", name, status, body)
		}
	}

	// sworn#359: a call that arrives while another is running waits for the
	// slot and then runs, instead of being refused; a waiting call whose
	// client gives up returns without running and without taking the slot.
	firstDone := make(chan brokerHTTPResult, 1)
	go func() {
		firstDone <- rawBrokerRequest(
			broker,
			capability,
			toolCallRequest(5, "Bash", map[string]any{
				"script": "sleep 1; printf first",
			}),
			"",
			"",
			"",
		)
	}()
	deadline := time.Now().Add(time.Second)
	for len(broker.callSlot) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("first broker call did not enter tool execution")
		}
		time.Sleep(time.Millisecond)
	}
	abandoned, abandon := context.WithTimeout(context.Background(), 50*time.Millisecond)
	status, body = brokerRequestWithContext(
		t,
		abandoned,
		broker,
		capability,
		toolCallRequest(60, "Bash", map[string]any{
			"script": "printf must-not-run",
		}),
	)
	abandon()
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"cancelled"`)) {
		t.Fatalf("abandoned queued call = %d %s", status, body)
	}
	secondDone := make(chan brokerHTTPResult, 1)
	go func() {
		secondDone <- rawBrokerRequest(
			broker,
			capability,
			toolCallRequest(6, "Read", map[string]any{
				"path": GuestWorkspacePath + "/broker.txt",
			}),
			"",
			"",
			"",
		)
	}()
	// The first call holds the slot for about a second; the queued call
	// must not complete while it does. Completion order is not asserted
	// directly: once both are ready a select picks between them at random.
	var first, second brokerHTTPResult
	select {
	case second = <-secondDone:
		t.Fatalf("queued call finished while the running call held the slot: %#v", second)
	case <-time.After(200 * time.Millisecond):
	}
	if len(broker.callSlot) != 1 {
		t.Fatal("running call released the slot early")
	}
	first = <-firstDone
	second = <-secondDone
	responseBodies = append(responseBodies, first.body, second.body)
	if first.err != nil || first.status != http.StatusOK ||
		!bytes.Contains(first.body, []byte(`"text":"first"`)) {
		t.Fatalf("first concurrent call = %#v", first)
	}
	if second.err != nil || second.status != http.StatusOK ||
		!bytes.Contains(second.body, []byte(`"text":"broker body"`)) {
		t.Fatalf("queued concurrent call = %#v", second)
	}
	if bytes.Contains(first.body, []byte("must-not-run")) ||
		bytes.Contains(second.body, []byte("must-not-run")) {
		t.Fatal("an abandoned queued call executed")
	}

	submission := submissionFixture(
		t,
		invocation.Request.InvocationID,
		PlannerProposal,
		"",
	)
	var submitArguments map[string]any
	if json.Unmarshal(
		[]byte(submissionToolArguments(t, submission)),
		&submitArguments,
	) != nil {
		t.Fatal("invalid submit fixture")
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		toolCallRequest(7, "sworn_submit", submitArguments),
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusOK ||
		!bytes.Contains(body, []byte(`"text":"accepted"`)) {
		t.Fatalf("submit = %d %s", status, body)
	}
	select {
	case <-broker.Terminal():
	default:
		t.Fatal("terminal submission did not close broker")
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		toolCallRequest(8, "sworn_submit", submitArguments),
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"closed"`)) {
		t.Fatalf("post-submit replay = %d %s", status, body)
	}
	for _, response := range responseBodies {
		if bytes.Contains(response, capability) {
			t.Fatalf("capability escaped broker response: %s", response)
		}
	}
}

func openNativeBrokerForTest(
	t *testing.T,
	broker *nativeBroker,
	capability []byte,
) {
	t.Helper()
	for _, request := range []map[string]any{
		{
			"jsonrpc": "2.0", "id": 101, "method": "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name": "codex", "version": CodexCLIVersion,
				},
			},
		},
		{
			"jsonrpc": "2.0", "method": "notifications/initialized",
			"params": map[string]any{},
		},
		{
			"jsonrpc": "2.0", "id": 102, "method": "tools/list",
			"params": map[string]any{},
		},
	} {
		status, body := brokerRequest(t, broker, capability, request)
		if status != http.StatusOK && status != http.StatusAccepted {
			t.Fatalf("broker handshake = %d %s", status, body)
		}
	}
	if err := broker.Arm(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeBrokerRejectsMalformedUnauthorizedCancelledAndExcessUse(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	capability := broker.capability()
	defer clearBytes(capability)
	defer broker.Close()

	valid := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list",
		"params": map[string]any{},
	}
	for name, mutation := range []struct {
		token       []byte
		host        string
		path        string
		contentType string
	}{
		{token: []byte("wrong")},
		{token: capability, host: "127.0.0.1:1"},
		{token: capability, path: "/other"},
		{token: capability, contentType: "application/json; charset=utf-8"},
	} {
		result := rawBrokerRequest(
			broker,
			mutation.token,
			valid,
			mutation.host,
			mutation.path,
			mutation.contentType,
		)
		if result.err != nil || result.status < 400 {
			t.Fatalf("mutation %d accepted: %#v", name, result)
		}
	}
	withoutPrefix := rawBrokerRequestWithAuthorization(
		broker,
		string(capability),
		valid,
	)
	if withoutPrefix.err != nil || withoutPrefix.status != http.StatusUnauthorized {
		t.Fatalf("missing Bearer prefix = %#v", withoutPrefix)
	}
	status, body := brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities": map[string]any{
					"ambient": map[string]any{},
				},
				"clientInfo": map[string]any{"name": "x", "version": "1"},
			},
		},
	)
	if status != http.StatusBadRequest ||
		!bytes.Contains(body, []byte(`"message":"invalid_params"`)) {
		t.Fatalf("unknown capability = %d %s", status, body)
	}
	oversized := bytes.Repeat([]byte("x"), MaxBrokerBodyBytes+1)
	request, _ := http.NewRequest(
		http.MethodPost,
		broker.URL(),
		bytes.NewReader(oversized),
	)
	request.Header.Set("Authorization", "Bearer "+string(capability))
	request.Header.Set("Content-Type", "application/json")
	response, requestErr := (&http.Client{Timeout: time.Second}).Do(request)
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversize status = %d", response.StatusCode)
	}

	if err := broker.Arm(); err != nil {
		t.Fatal(err)
	}
	broker.Cancel()
	status, body = brokerRequest(
		t,
		broker,
		capability,
		toolCallRequest(3, "Read", map[string]any{
			"path": GuestWorkspacePath,
		}),
	)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"closed"`)) {
		t.Fatalf("post-cancel = %d %s", status, body)
	}
}

func TestNativeBrokerConnectionLimitIsFixed(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	var peers []net.Conn
	for index := 0; index < MaxBrokerConnections+1; index++ {
		left, right := net.Pipe()
		peers = append(peers, left, right)
		broker.connectionState(left, http.StateNew)
	}
	broker.connMu.Lock()
	count := len(broker.connections)
	broker.connMu.Unlock()
	if count != MaxBrokerConnections {
		t.Fatalf("connections = %d", count)
	}
	for _, connection := range peers {
		_ = connection.Close()
		broker.connectionState(connection, http.StateClosed)
	}
}

// TestNativeBrokerCallBudgetCrossingMovesBrokerTerminal pins A1
// (S4-broker-budget-and-turn-cap): the exact request whose own count
// crosses MaxBrokerCalls moves the broker to brokerTerminal, closes
// Terminal(), and records the crossing so runNative can build its typed
// failure - all as one atomic step under the broker's own lock, mirroring
// finish()'s own transition rather than merely answering "closed" without
// ever transitioning state (the bug this fix closes).
func TestNativeBrokerCallBudgetCrossingMovesBrokerTerminal(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	capability := broker.capability()
	defer clearBytes(capability)
	openNativeBrokerForTest(t, broker, capability)

	select {
	case <-broker.Terminal():
		t.Fatal("broker already terminal after handshake")
	default:
	}
	if broker.BudgetExhausted() {
		t.Fatal("BudgetExhausted before any crossing")
	}

	// The handshake in openNativeBrokerForTest already spent 3 calls
	// (initialize, notifications/initialized, tools/list). Drive the
	// remaining calls up to and past MaxBrokerCalls with cheap,
	// already-listed tools/list requests: the top gate counts every
	// request regardless of what it asks, so the exact response each of
	// these gets (a "state_invalid" conflict, since the broker is already
	// listed) is irrelevant to the crossing itself.
	listRequest := map[string]any{
		"jsonrpc": "2.0", "id": 200, "method": "tools/list",
		"params": map[string]any{},
	}
	const handshakeCalls = 3
	needed := MaxBrokerCalls - handshakeCalls + 1
	var lastStatus int
	var lastBody []byte
	for index := 0; index < needed; index++ {
		lastStatus, lastBody = brokerRequestWithContext(
			t, context.Background(), broker, capability, listRequest,
		)
	}
	if lastStatus != http.StatusConflict ||
		!bytes.Contains(lastBody, []byte(`"message":"closed"`)) {
		t.Fatalf("crossing request = %d %s", lastStatus, lastBody)
	}
	select {
	case <-broker.Terminal():
	default:
		t.Fatal("budget crossing did not close broker")
	}
	if !broker.BudgetExhausted() {
		t.Fatal("BudgetExhausted stayed false after crossing")
	}
	if got := broker.BudgetExhaustedCalls(); got != MaxBrokerCalls+1 {
		t.Fatalf("BudgetExhaustedCalls = %d, want %d", got, MaxBrokerCalls+1)
	}
	broker.mu.Lock()
	state := broker.state
	broker.mu.Unlock()
	if state != brokerTerminal {
		t.Fatalf("state = %v, want brokerTerminal", state)
	}
}

// TestNativeBrokerStrayRequestAfterUnrelatedTerminalNeverSetsBudgetFlag
// pins A1's other half: a request that arrives after the broker is
// already terminal for an unrelated reason (an accepted submission, a
// RECOVERY_STEP_REFUSED close) with the calls counter already past
// MaxBrokerCalls must never retroactively claim the budget crossing -
// BudgetExhausted stays false, because this exact request's own crossing
// is not what moved the broker into terminal.
func TestNativeBrokerStrayRequestAfterUnrelatedTerminalNeverSetsBudgetFlag(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	capability := broker.capability()
	defer clearBytes(capability)
	openNativeBrokerForTest(t, broker, capability)

	// Simulate the broker finishing for an unrelated reason (as an
	// accepted submission or a RECOVERY_STEP_REFUSED close would) while
	// the calls counter is already past MaxBrokerCalls.
	broker.mu.Lock()
	broker.calls = MaxBrokerCalls + 1
	broker.mu.Unlock()
	broker.finish(brokerTerminal)
	if broker.BudgetExhausted() {
		t.Fatal("finishing for an unrelated reason set BudgetExhausted")
	}

	status, body := brokerRequestWithContext(
		t,
		context.Background(),
		broker,
		capability,
		toolCallRequest(999, "Read", map[string]any{
			"path": GuestWorkspacePath,
		}),
	)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"closed"`)) {
		t.Fatalf("stray request = %d %s", status, body)
	}
	if broker.BudgetExhausted() {
		t.Fatal("stray request after unrelated terminal set BudgetExhausted")
	}
	if got := broker.BudgetExhaustedCalls(); got != 0 {
		t.Fatalf("BudgetExhaustedCalls = %d, want 0", got)
	}
}

// TestNativeBrokerRefusedCallCounterSaturatesAtMaxRefusedBrokerCalls pins
// A3's saturating source-side ceiling: requests keep arriving (and would
// keep counting as refused) after the broker goes terminal until the
// engine's SIGTERM actually lands, so the counter must stop growing well
// before UsageReceipt.RefusedToolCalls' own encode-time bound - the same
// MaxRefusedBrokerCalls constant - could ever reject the failure receipt
// that carries it.
func TestNativeBrokerRefusedCallCounterSaturatesAtMaxRefusedBrokerCalls(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	capability := broker.capability()
	defer clearBytes(capability)
	openNativeBrokerForTest(t, broker, capability)
	broker.Cancel()

	request := toolCallRequest(300, "Read", map[string]any{
		"path": GuestWorkspacePath,
	})
	for index := 0; index < MaxRefusedBrokerCalls+50; index++ {
		brokerRequestWithContext(t, context.Background(), broker, capability, request)
	}
	if got := broker.refusedCallTotal(); got != MaxRefusedBrokerCalls {
		t.Fatalf("refusedCallTotal = %d, want saturated at %d", got, MaxRefusedBrokerCalls)
	}
}

type brokerHTTPResult struct {
	status int
	body   []byte
	err    error
}

func brokerRequestWithContext(
	t *testing.T,
	ctx context.Context,
	broker *nativeBroker,
	token []byte,
	value any,
) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, broker.URL(), bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(token))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	broker.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.Bytes()
}

func brokerRequest(
	t *testing.T,
	broker *nativeBroker,
	token []byte,
	value any,
) (int, []byte) {
	t.Helper()
	result := rawBrokerRequest(broker, token, value, "", "", "")
	if result.err != nil {
		t.Fatal(result.err)
	}
	return result.status, result.body
}

func rawBrokerRequest(
	broker *nativeBroker,
	token []byte,
	value any,
	host string,
	path string,
	contentType string,
) brokerHTTPResult {
	body, err := json.Marshal(value)
	if err != nil {
		return brokerHTTPResult{err: err}
	}
	target := broker.URL()
	if path != "" {
		target = strings.TrimSuffix(target, "/mcp") + path
	}
	request, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return brokerHTTPResult{err: err}
	}
	request.Header.Set("Authorization", "Bearer "+string(token))
	if contentType == "" {
		contentType = "application/json"
	}
	request.Header.Set("Content-Type", contentType)
	if host != "" {
		request.Host = host
	}
	return executeBrokerHTTPRequest(request)
}

func rawBrokerRequestWithAuthorization(
	broker *nativeBroker,
	authorization string,
	value any,
) brokerHTTPResult {
	body, err := json.Marshal(value)
	if err != nil {
		return brokerHTTPResult{err: err}
	}
	request, err := http.NewRequest(
		http.MethodPost,
		broker.URL(),
		bytes.NewReader(body),
	)
	if err != nil {
		return brokerHTTPResult{err: err}
	}
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Content-Type", "application/json")
	return executeBrokerHTTPRequest(request)
}

func executeBrokerHTTPRequest(request *http.Request) brokerHTTPResult {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return brokerHTTPResult{err: err}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxToolResultBytes+1))
	return brokerHTTPResult{status: response.StatusCode, body: body, err: err}
}

func toolCallRequest(id int, name string, arguments any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": arguments},
	}
}

// stubBrokerSession is a minimal nativeBrokerSession whose tool schema can
// be flipped malformed on demand, so a test can deterministically drive
// listTools' own -32603 "internal" branch (A3, S9-broker-budget-and-
// turn-cap-repair) without needing a real, always-valid toolSession.
type stubBrokerSession struct {
	malformedSchema bool
}

func (session *stubBrokerSession) brokerToolDefinitions() []providerToolDefinition {
	schema := json.RawMessage(`{"type":"object"}`)
	if session.malformedSchema {
		schema = json.RawMessage(`{`)
	}
	return []providerToolDefinition{
		{Name: "Probe", Description: "probe", InputSchema: schema},
	}
}

func (session *stubBrokerSession) execute(
	context.Context,
	providerToolCall,
) providerToolResult {
	return providerToolResult{Content: []byte("ok")}
}

func (session *stubBrokerSession) terminated() (bool, error) { return false, nil }
func (session *stubBrokerSession) observeToolResultTurn(int64, []providerToolResult) {
}
func (session *stubBrokerSession) observeWorkerTurn(WorkerTurn) {}
func (session *stubBrokerSession) dropWorkerTurnEvent()         {}
func (session *stubBrokerSession) redactionSecrets() [][]byte   { return nil }

// TestNativeBrokerCountsEveryRefusalKindAndExcludesPreAuthRequests pins A3
// (S9-broker-budget-and-turn-cap-repair): every broker request that spends
// a calls unit is counted as executed, refused, or handshake, so the
// counts add up to the calls made - including the three real gaps this
// slice closes (initialize's protocol_refused/invalid_params/state_invalid
// branches, the notifications/initialized error branch, and listTools'
// state_invalid and -32603 internal branches, none of which called
// incrementRefused before this fix) - while the two pre-auth request
// shapes that spend no calls unit at all (a malformed transport request,
// an unauthorized request) are proven to land in neither total.
func TestNativeBrokerCountsEveryRefusalKindAndExcludesPreAuthRequests(t *testing.T) {
	// Broker 1 never completes the handshake: it exercises the pre-auth
	// exclusions, method_not_found, initialize's protocol_refused and
	// invalid_params branches (both repeatable, since a failed initialize
	// never sets broker.initialized), and tools/call's not_open.
	invocation, _, _ := memoryInvocationFixture(t)
	session1, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session1.Close()
	broker1, err := newNativeBroker(session1)
	if err != nil {
		t.Fatal(err)
	}
	defer broker1.Close()
	capability1 := broker1.capability()
	defer clearBytes(capability1)

	badContentType := rawBrokerRequest(
		broker1, capability1,
		map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/list",
			"params": map[string]any{},
		},
		"", "", "text/plain",
	)
	if badContentType.err != nil || badContentType.status != http.StatusBadRequest {
		t.Fatalf("pre-auth transport-shape rejection = %#v", badContentType)
	}
	badAuth := rawBrokerRequestWithAuthorization(
		broker1, "Bearer wrong-token-value",
		map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "tools/list",
			"params": map[string]any{},
		},
	)
	if badAuth.err != nil || badAuth.status != http.StatusUnauthorized {
		t.Fatalf("pre-auth unauthorized rejection = %#v", badAuth)
	}
	broker1.mu.Lock()
	preAuthCalls := broker1.calls
	broker1.mu.Unlock()
	if preAuthCalls != 0 {
		t.Fatalf(
			"pre-auth rejections spent a calls unit: calls = %d, want 0",
			preAuthCalls,
		)
	}
	if got := broker1.refusedCallTotal(); got != 0 {
		t.Fatalf(
			"pre-auth rejections counted as refused: refusedCallTotal = %d, want 0",
			got,
		)
	}

	var wantRefused1 int64

	status, body := brokerRequest(t, broker1, capability1, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "unknown/method",
		"params": map[string]any{},
	})
	if status != http.StatusNotFound ||
		!bytes.Contains(body, []byte(`"message":"method_not_found"`)) {
		t.Fatalf("method_not_found = %d %s", status, body)
	}
	wantRefused1++

	for index := 0; index < 2; index++ {
		status, body = brokerRequest(t, broker1, capability1, map[string]any{
			"jsonrpc": "2.0", "id": 10 + index, "method": "initialize",
			"params": map[string]any{
				"protocolVersion": "1999-01-01",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name": "probe", "version": "1",
				},
			},
		})
		if status != http.StatusBadRequest ||
			!bytes.Contains(body, []byte(`"message":"protocol_refused"`)) {
			t.Fatalf("protocol_refused = %d %s", status, body)
		}
		wantRefused1++
	}

	status, body = brokerRequest(t, broker1, capability1, map[string]any{
		"jsonrpc": "2.0", "id": 12, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"ambient": map[string]any{}},
			"clientInfo":      map[string]any{"name": "probe", "version": "1"},
		},
	})
	if status != http.StatusBadRequest ||
		!bytes.Contains(body, []byte(`"message":"invalid_params"`)) {
		t.Fatalf("initialize invalid_params = %d %s", status, body)
	}
	wantRefused1++

	status, body = brokerRequest(
		t, broker1, capability1,
		toolCallRequest(13, "Read", map[string]any{"path": GuestWorkspacePath}),
	)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"not_open"`)) {
		t.Fatalf("not_open = %d %s", status, body)
	}
	wantRefused1++

	if got := broker1.refusedCallTotal(); got != wantRefused1 {
		t.Fatalf("broker1 refusedCallTotal = %d, want %d", got, wantRefused1)
	}
	broker1.mu.Lock()
	calls1 := broker1.calls
	broker1.mu.Unlock()
	if int64(calls1) != wantRefused1 {
		t.Fatalf(
			"broker1 calls = %d, want %d (every counted request here was refused)",
			calls1, wantRefused1,
		)
	}

	// Broker 2 completes a real handshake around a deliberately malformed
	// tool schema, exercising the state_invalid and internal branches this
	// slice's A3 fix now counts.
	session2 := &stubBrokerSession{malformedSchema: true}
	broker2, err := newNativeBroker(session2)
	if err != nil {
		t.Fatal(err)
	}
	defer broker2.Close()
	capability2 := broker2.capability()
	defer clearBytes(capability2)
	if err := broker2.Arm(); err != nil {
		t.Fatal(err)
	}

	initializeRequest := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "probe", "version": "1"},
		},
	}
	status, body = brokerRequest(t, broker2, capability2, initializeRequest)
	if status != http.StatusOK {
		t.Fatalf("broker2 initialize = %d %s", status, body)
	}
	notifyRequest := map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized",
		"params": map[string]any{},
	}
	status, body = brokerRequest(t, broker2, capability2, notifyRequest)
	if status != http.StatusAccepted {
		t.Fatalf("broker2 notify = %d %s", status, body)
	}
	status, body = brokerRequest(t, broker2, capability2, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
		"params": map[string]any{},
	})
	if status != http.StatusInternalServerError ||
		!bytes.Contains(body, []byte(`"message":"internal"`)) {
		t.Fatalf("tools/list internal = %d %s", status, body)
	}
	var wantRefused2 int64 = 1

	status, body = brokerRequest(t, broker2, capability2, notifyRequest)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"state_invalid"`)) {
		t.Fatalf("notifications/initialized repeat = %d %s", status, body)
	}
	wantRefused2++

	status, body = brokerRequest(t, broker2, capability2, initializeRequest)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"state_invalid"`)) {
		t.Fatalf("initialize repeat = %d %s", status, body)
	}
	wantRefused2++

	if got := broker2.refusedCallTotal(); got != wantRefused2 {
		t.Fatalf(
			"broker2 refusedCallTotal before real handshake = %d, want %d",
			got, wantRefused2,
		)
	}

	session2.malformedSchema = false
	status, body = brokerRequest(t, broker2, capability2, map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "tools/list",
		"params": map[string]any{},
	})
	if status != http.StatusOK {
		t.Fatalf("broker2 tools/list = %d %s", status, body)
	}
	if !broker2.Ready() {
		t.Fatal("broker2 not ready after completing its handshake")
	}

	status, body = brokerRequest(t, broker2, capability2, map[string]any{
		"jsonrpc": "2.0", "id": 5, "method": "tools/list",
		"params": map[string]any{},
	})
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"state_invalid"`)) {
		t.Fatalf("tools/list repeat = %d %s", status, body)
	}
	wantRefused2++

	status, body = brokerRequest(t, broker2, capability2, map[string]any{
		"jsonrpc": "2.0", "id": 6, "method": "tools/call",
		"params": map[string]any{
			"name": "Probe", "arguments": map[string]any{},
			"unexpected": true,
		},
	})
	if status != http.StatusBadRequest ||
		!bytes.Contains(body, []byte(`"message":"invalid_params"`)) {
		t.Fatalf("tools/call invalid_params = %d %s", status, body)
	}
	wantRefused2++

	if got := broker2.refusedCallTotal(); got != wantRefused2 {
		t.Fatalf("broker2 refusedCallTotal = %d, want %d", got, wantRefused2)
	}
	broker2.mu.Lock()
	calls2 := broker2.calls
	executedProbeCalls := broker2.callsByName["Probe"]
	broker2.mu.Unlock()
	const broker2SuccessfulHandshakeCalls = 3 // initialize, notify, the successful tools/list
	if int64(calls2) != wantRefused2+broker2SuccessfulHandshakeCalls {
		t.Fatalf(
			"broker2 calls = %d, want %d (refused + %d successful handshake)",
			calls2, wantRefused2+broker2SuccessfulHandshakeCalls,
			broker2SuccessfulHandshakeCalls,
		)
	}
	if executedProbeCalls != 0 {
		t.Fatalf(
			"broker2 executed Probe calls = %d, want 0 (no tools/call ever executed)",
			executedProbeCalls,
		)
	}

	if total := wantRefused1 + wantRefused2; total >= MaxRefusedBrokerCalls {
		t.Fatalf(
			"flood total %d reaches the saturating MaxRefusedBrokerCalls bound %d; the sum would no longer be exact",
			total, MaxRefusedBrokerCalls,
		)
	}
}
