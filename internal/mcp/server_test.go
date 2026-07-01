package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

// testRoundTrip creates a server connected via io.Pipe, returns the write end
// (for sending requests) and a reader for responses.
func testRoundTrip(t *testing.T) (stdinWriter io.Writer, stdoutReader *bufio.Reader, s *Server) {
	t.Helper()
	s = New()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	go func() {
		err := s.Run(context.Background(), stdinR, stdoutW)
		if err != nil && err != io.ErrClosedPipe {
			t.Logf("server.Run returned: %v", err)
		}
	}()
	return stdinW, bufio.NewReader(stdoutR), s
}

// readResponse reads one JSON object line from the response reader.
func readResponse(t *testing.T, r *bufio.Reader) map[string]json.RawMessage {
	t.Helper()
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("decode response: %v (body: %q)", err, string(line))
	}
	return resp
}

// sendRequest writes a JSON-RPC request line.
func sendRequest(t *testing.T, w io.Writer, method string, id json.RawMessage, params json.RawMessage) {
	t.Helper()
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	if err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func jsonID(n int) json.RawMessage {
	b, _ := json.Marshal(n)
	return b
}

func TestInitializeHandshake(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	params := json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`)
	sendRequest(t, w, "initialize", jsonID(1), params)

	resp := readResponse(t, r)
	if id := string(resp["id"]); id != "1" {
		t.Errorf("response id = %q, want %q", id, "1")
	}
	if _, hasErr := resp["error"]; hasErr {
		t.Errorf("unexpected error in initialize response: %s", resp["error"])
	}

	var result initializeResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal initialize result: %v", err)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocolVersion = %q, want %q", result.ProtocolVersion, "2024-11-05")
	}
	if result.ServerInfo.Name != "sworn-mcp" {
		t.Errorf("serverInfo.name = %q, want %q", result.ServerInfo.Name, "sworn-mcp")
	}
}

func TestInitializedNotification(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	// Initialize first (required before initialized)
	params := json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`)
	sendRequest(t, w, "initialize", jsonID(1), params)
	readResponse(t, r)

	// Send initialized notification (no response expected — server should not crash)
	sendRequest(t, w, "initialized", nil, nil)

	// Send a ping to confirm server is still alive
	sendRequest(t, w, "tools/list", jsonID(2), nil)
	resp := readResponse(t, r)
	if _, hasErr := resp["error"]; hasErr {
		t.Errorf("unexpected error after initialized: %s", resp["error"])
	}
}

func TestToolsListEmpty(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	// Initialize
	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)

	// tools/list
	sendRequest(t, w, "tools/list", jsonID(2), nil)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("tools/list returned error: %s", resp["error"])
	}
	var result toolsListResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal tools/list result: %v", err)
	}
	if result.Tools == nil {
		t.Fatal("tools/list result.Tools is nil, want empty array")
	}
	if len(result.Tools) != 0 {
		t.Errorf("tools/list = %d tools, want 0", len(result.Tools))
	}
}

func TestUnknownMethod(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	sendRequest(t, w, "bogus_method", jsonID(1), nil)
	resp := readResponse(t, r)

	if _, hasResult := resp["result"]; hasResult {
		t.Fatal("unknown method returned a result, want error")
	}
	var rpcErr struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp["error"], &rpcErr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if rpcErr.Code != codeMethodNotFound {
		t.Errorf("error code = %d, want %d", rpcErr.Code, codeMethodNotFound)
	}
}

func TestUnregisteredToolCall(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	// Initialize
	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)

	// tools/call for unregistered tool
	params := json.RawMessage(`{"name":"nonexistent","arguments":{}}`)
	sendRequest(t, w, "tools/call", jsonID(2), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("tools/call returned JSON-RPC error for unregistered tool: %s", resp["error"])
	}
	result, hasResult := resp["result"]
	if !hasResult {
		t.Fatal("tools/call has no result")
	}
	var tr ToolResult
	if err := json.Unmarshal(result, &tr); err != nil {
		t.Fatalf("unmarshal ToolResult: %v", err)
	}
	if !tr.IsError {
		t.Error("ToolResult.IsError = false, want true for unregistered tool")
	}
	if len(tr.Content) == 0 {
		t.Fatal("ToolResult.Content is empty")
	}
	if tr.Content[0].Type != "text" {
		t.Errorf("Content[0].Type = %q, want %q", tr.Content[0].Type, "text")
	}
	if !strings.Contains(tr.Content[0].Text, "not implemented") {
		t.Errorf("Content[0].Text = %q, want substring %q", tr.Content[0].Text, "not implemented")
	}
}

func TestRegisteredToolStub(t *testing.T) {
	w, r, s := testRoundTrip(t)

	// Register a no-op tool before initializing
	called := false
	s.RegisterTool("echo", json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`),
		func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
			called = true
			return &ToolResult{
				Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("called with: %s", string(params))}},
			}, nil
		})

	// Initialize
	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)

	// tools/list should show the registered tool
	sendRequest(t, w, "tools/list", jsonID(2), nil)
	listResp := readResponse(t, r)
	var listResult toolsListResult
	if err := json.Unmarshal(listResp["result"], &listResult); err != nil {
		t.Fatalf("unmarshal tools/list result: %v", err)
	}
	if len(listResult.Tools) != 1 {
		t.Fatalf("tools/list = %d tools, want 1", len(listResult.Tools))
	}
	if listResult.Tools[0].Name != "echo" {
		t.Errorf("tool name = %q, want %q", listResult.Tools[0].Name, "echo")
	}

	// tools/call for registered tool
	params := json.RawMessage(`{"name":"echo","arguments":{"msg":"hello"}}`)
	sendRequest(t, w, "tools/call", jsonID(3), params)
	callResp := readResponse(t, r)

	if _, hasErr := callResp["error"]; hasErr {
		t.Fatalf("tools/call returned error: %s", callResp["error"])
	}
	var tr ToolResult
	if err := json.Unmarshal(callResp["result"], &tr); err != nil {
		t.Fatalf("unmarshal ToolResult: %v", err)
	}
	if tr.IsError {
		t.Error("ToolResult.IsError = true, want false for registered tool")
	}
	if len(tr.Content) == 0 || tr.Content[0].Text != "called with: {\"msg\":\"hello\"}" {
		t.Errorf("Content[0].Text = %q, want %q", tr.Content[0].Text, "called with: {\"msg\":\"hello\"}")
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestResourcesList(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)

	sendRequest(t, w, "resources/list", jsonID(2), nil)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/list returned error: %s", resp["error"])
	}
	var result resourcesListResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal resources/list result: %v", err)
	}
	if result.Resources == nil {
		t.Fatal("resources/list result.Resources is nil, want empty array")
	}
}

func TestPromptsList(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)

	sendRequest(t, w, "prompts/list", jsonID(2), nil)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("prompts/list returned error: %s", resp["error"])
	}
	var result promptsListResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal prompts/list result: %v", err)
	}
	if result.Prompts == nil {
		t.Fatal("prompts/list result.Prompts is nil, want empty array")
	}
}

func TestBatchRejection(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	// Send a batch request (starts with [)
	batch := json.RawMessage(`[{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}]`)
	batch = append(batch, '\n')
	_, err := w.Write(batch)
	if err != nil {
		t.Fatalf("write batch: %v", err)
	}

	resp := readResponse(t, r)
	var rpcErr struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp["error"], &rpcErr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if rpcErr.Code != codeInvalidRequest {
		t.Errorf("error code = %d, want %d", rpcErr.Code, codeInvalidRequest)
	}
}

func TestInvalidJSON(t *testing.T) {
	w, r, _ := testRoundTrip(t)

	_, err := w.Write([]byte("not json\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	resp := readResponse(t, r)
	var rpcErr struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp["error"], &rpcErr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if rpcErr.Code != codeParseError {
		t.Errorf("error code = %d, want %d", rpcErr.Code, codeParseError)
	}
}

func TestServerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := New()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(ctx, stdinR, stdoutW)
	}()

	cancel()
	stdinW.Close()

	err := <-errCh
	if err != context.Canceled && err != io.ErrClosedPipe {
		t.Errorf("Run returned %v, want context.Canceled or io.ErrClosedPipe", err)
	}
	_ = stdoutR // suppress unused
}
