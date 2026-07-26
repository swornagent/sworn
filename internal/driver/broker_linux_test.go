//go:build linux

package driver

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNativeBrokerEnforcesExactCapabilityStateAndTerminalProtocol(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
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
	if err := broker.Open(); err != nil {
		t.Fatal(err)
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		toolCallRequest(4, "Read", map[string]any{
			"path": GuestWorkspacePath + "/broker.txt",
		}),
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusOK ||
		!bytes.Contains(body, []byte(`"text":"broker body"`)) {
		t.Fatalf("open call = %d %s", status, body)
	}

	firstDone := make(chan brokerHTTPResult, 1)
	go func() {
		firstDone <- rawBrokerRequest(
			broker,
			capability,
			toolCallRequest(5, "Bash", map[string]any{
				"script": "sleep 0.15; printf first",
			}),
			"",
			"",
			"",
		)
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if !broker.callMu.TryLock() {
			break
		}
		broker.callMu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("first broker call did not enter tool execution")
		}
		time.Sleep(time.Millisecond)
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		toolCallRequest(6, "Read", map[string]any{
			"path": GuestWorkspacePath + "/broker.txt",
		}),
	)
	responseBodies = append(responseBodies, body)
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"concurrent_call"`)) {
		t.Fatalf("concurrent call = %d %s", status, body)
	}
	first := <-firstDone
	responseBodies = append(responseBodies, first.body)
	if first.err != nil || first.status != http.StatusOK ||
		!bytes.Contains(first.body, []byte(`"text":"first"`)) {
		t.Fatalf("first concurrent call = %#v", first)
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

	if err := broker.Open(); err != nil {
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

type brokerHTTPResult struct {
	status int
	body   []byte
	err    error
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
