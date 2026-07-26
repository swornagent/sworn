package driver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	MaxBrokerBodyBytes   = 524_288
	MaxBrokerCalls       = 512
	MaxBrokerConnections = 8
)

type brokerState uint8

const (
	brokerClosed brokerState = iota + 1
	brokerOpen
	brokerTerminal
	brokerCancelled
)

type nativeBroker struct {
	mu          sync.Mutex
	callMu      sync.Mutex
	state       brokerState
	calls       int
	address     string
	token       []byte
	session     *toolSession
	server      *http.Server
	listener    net.Listener
	terminal    chan struct{}
	closeOnce   sync.Once
	connMu      sync.Mutex
	connections map[net.Conn]struct{}
}

func newNativeBroker(session *toolSession) (*nativeBroker, error) {
	if session == nil {
		return nil, fail("INVALID_BROKER")
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return nil, fail("ENDPOINT_UNAVAILABLE")
	}
	token := []byte(base64.RawURLEncoding.EncodeToString(rawToken))
	clearBytes(rawToken)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		clearBytes(token)
		return nil, fail("ENDPOINT_UNAVAILABLE")
	}
	broker := &nativeBroker{
		state: brokerClosed, address: listener.Addr().String(),
		token: token, session: session, listener: listener,
		terminal: make(chan struct{}), connections: make(map[net.Conn]struct{}),
	}
	broker.server = &http.Server{
		Handler:           broker,
		ReadHeaderTimeout: 2 * time.Second,
		IdleTimeout:       2 * time.Second,
		MaxHeaderBytes:    8_192,
		ConnState:         broker.connectionState,
	}
	go func() {
		_ = broker.server.Serve(listener)
	}()
	return broker, nil
}

func (broker *nativeBroker) URL() string {
	return "http://" + broker.address + "/mcp"
}

func (broker *nativeBroker) capability() []byte {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	return append([]byte(nil), broker.token...)
}

func (broker *nativeBroker) Open() error {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.state != brokerClosed {
		return fail("BROKER_STATE_INVALID")
	}
	broker.state = brokerOpen
	return nil
}

func (broker *nativeBroker) Cancel() {
	broker.finish(brokerCancelled)
}

func (broker *nativeBroker) finish(state brokerState) {
	broker.mu.Lock()
	if broker.state != brokerTerminal && broker.state != brokerCancelled {
		broker.state = state
		broker.closeOnce.Do(func() { close(broker.terminal) })
	}
	broker.mu.Unlock()
}

func (broker *nativeBroker) Terminal() <-chan struct{} {
	return broker.terminal
}

func (broker *nativeBroker) Close() {
	if broker == nil {
		return
	}
	broker.Cancel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = broker.server.Shutdown(ctx)
	_ = broker.listener.Close()
	broker.mu.Lock()
	clearBytes(broker.token)
	broker.token = nil
	broker.mu.Unlock()
}

func (broker *nativeBroker) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost ||
		request.URL.Path != "/mcp" || request.URL.RawQuery != "" ||
		request.Host != broker.address ||
		request.Header.Get("Content-Type") != "application/json" {
		writeBrokerError(writer, http.StatusBadRequest, nil, -32600, "invalid_request")
		return
	}
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") {
		writeBrokerError(writer, http.StatusUnauthorized, nil, -32600, "unauthorized")
		return
	}
	supplied := []byte(strings.TrimPrefix(authorization, "Bearer "))
	broker.mu.Lock()
	expected := append([]byte(nil), broker.token...)
	broker.mu.Unlock()
	authorized := len(supplied) == len(expected) &&
		subtle.ConstantTimeCompare(supplied, expected) == 1
	clearBytes(supplied)
	clearBytes(expected)
	if !authorized {
		writeBrokerError(writer, http.StatusUnauthorized, nil, -32600, "unauthorized")
		return
	}
	broker.mu.Lock()
	state := broker.state
	broker.calls++
	calls := broker.calls
	broker.mu.Unlock()
	if state == brokerTerminal || state == brokerCancelled || calls > MaxBrokerCalls {
		writeBrokerError(writer, http.StatusConflict, nil, -32000, "closed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaxBrokerBodyBytes+1))
	if err != nil || len(body) == 0 || len(body) > MaxBrokerBodyBytes {
		clearBytes(body)
		writeBrokerError(writer, http.StatusBadRequest, nil, -32700, "invalid_json")
		return
	}
	defer clearBytes(body)
	value, err := decodeStrict(body, MaxBrokerBodyBytes)
	if err != nil {
		writeBrokerError(writer, http.StatusBadRequest, nil, -32700, "invalid_json")
		return
	}
	root, err := closedObject(
		value,
		[]string{"jsonrpc", "method"},
		[]string{"id", "params"},
	)
	if err != nil || root["jsonrpc"] != "2.0" {
		writeBrokerError(writer, http.StatusBadRequest, nil, -32600, "invalid_request")
		return
	}
	id := brokerID(root["id"])
	method, _ := root["method"].(string)
	switch method {
	case "initialize":
		if _, present := root["id"]; !present {
			writeBrokerError(writer, http.StatusBadRequest, id, -32600, "invalid_request")
			return
		}
		broker.initialize(writer, id, root["params"])
	case "notifications/initialized":
		if _, present := root["id"]; present ||
			!emptyBrokerParams(root["params"]) {
			writeBrokerError(writer, http.StatusBadRequest, id, -32600, "invalid_request")
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	case "tools/list":
		if _, present := root["id"]; !present ||
			!emptyBrokerParams(root["params"]) {
			writeBrokerError(writer, http.StatusBadRequest, id, -32600, "invalid_request")
			return
		}
		broker.listTools(writer, id)
	case "tools/call":
		if _, present := root["id"]; !present {
			writeBrokerError(writer, http.StatusBadRequest, id, -32600, "invalid_request")
			return
		}
		broker.callTool(request.Context(), writer, id, root["params"])
	default:
		writeBrokerError(writer, http.StatusNotFound, id, -32601, "method_not_found")
	}
}

func (broker *nativeBroker) initialize(
	writer http.ResponseWriter,
	id json.RawMessage,
	params any,
) {
	object, err := closedObject(
		params,
		[]string{"protocolVersion", "capabilities", "clientInfo"},
		[]string{"_meta"},
	)
	if err != nil {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "invalid_params")
		return
	}
	protocol, _ := object["protocolVersion"].(string)
	if protocol != "2024-11-05" && protocol != "2025-03-26" &&
		protocol != "2025-06-18" {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "protocol_refused")
		return
	}
	capabilities, capabilitiesOK := object["capabilities"].(map[string]any)
	clientInfo, clientInfoErr := closedObject(
		object["clientInfo"],
		[]string{"name", "version"},
		[]string{"title"},
	)
	clientName, nameOK := clientInfo["name"].(string)
	clientVersion, versionOK := clientInfo["version"].(string)
	if !capabilitiesOK ||
		!closedBrokerCapabilities(capabilities) ||
		clientInfoErr != nil || !nameOK || !versionOK ||
		validateText(clientName, 256, false) != nil ||
		validateText(clientVersion, 256, false) != nil {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "invalid_params")
		return
	}
	writeBrokerResult(writer, id, map[string]any{
		"protocolVersion": protocol,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "sworn", "version": "1.0.0"},
	})
}

func (broker *nativeBroker) listTools(writer http.ResponseWriter, id json.RawMessage) {
	definitions := toolDefinitions(broker.session.invocation.Request.Workspace.Access)
	tools := make([]map[string]any, len(definitions))
	for index, definition := range definitions {
		var schema any
		if json.Unmarshal(definition.InputSchema, &schema) != nil {
			writeBrokerError(writer, http.StatusInternalServerError, id, -32603, "internal")
			return
		}
		tools[index] = map[string]any{
			"name": definition.Name, "description": definition.Description,
			"inputSchema": schema,
		}
	}
	writeBrokerResult(writer, id, map[string]any{"tools": tools})
}

func (broker *nativeBroker) callTool(
	ctx context.Context,
	writer http.ResponseWriter,
	id json.RawMessage,
	params any,
) {
	broker.mu.Lock()
	state := broker.state
	broker.mu.Unlock()
	if state != brokerOpen {
		writeBrokerError(writer, http.StatusConflict, id, -32000, "not_open")
		return
	}
	if !broker.callMu.TryLock() {
		writeBrokerError(writer, http.StatusConflict, id, -32000, "concurrent_call")
		return
	}
	defer broker.callMu.Unlock()
	object, err := closedObject(params, []string{"name", "arguments"}, nil)
	if err != nil {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "invalid_params")
		return
	}
	name, ok := object["name"].(string)
	arguments, marshalErr := canonicalJSON(object["arguments"])
	if !ok || marshalErr != nil {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "invalid_params")
		return
	}
	broker.mu.Lock()
	callNumber := broker.calls
	broker.mu.Unlock()
	result := broker.session.execute(ctx, providerToolCall{
		ID: "mcp-" + itoa(callNumber), Name: name, Arguments: arguments,
	})
	writeBrokerResult(writer, id, map[string]any{
		"content": []map[string]any{{
			"type": "text", "text": string(result.Content),
		}},
		"isError": result.Failed,
	})
	if submitted, submitErr := broker.session.submitted(); submitted || submitErr != nil {
		broker.finish(brokerTerminal)
	}
}

func writeBrokerResult(writer http.ResponseWriter, id json.RawMessage, result any) {
	writeBrokerJSON(writer, http.StatusOK, map[string]any{
		"jsonrpc": "2.0", "id": id, "result": result,
	})
}

func writeBrokerError(
	writer http.ResponseWriter,
	status int,
	id json.RawMessage,
	code int,
	message string,
) {
	writeBrokerJSON(writer, status, map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": message},
	})
}

func writeBrokerJSON(writer http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > MaxToolResultBytes {
		http.Error(writer, "internal", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
	clearBytes(body)
}

func brokerID(value any) json.RawMessage {
	if value == nil {
		return json.RawMessage("null")
	}
	body, err := canonicalJSON(value)
	if err != nil || len(body) > 256 {
		return json.RawMessage("null")
	}
	return body
}

func containsCapability(body, capability []byte) bool {
	return len(capability) != 0 && bytes.Contains(body, capability)
}

func (broker *nativeBroker) connectionState(
	connection net.Conn,
	state http.ConnState,
) {
	broker.connMu.Lock()
	defer broker.connMu.Unlock()
	switch state {
	case http.StateNew:
		if len(broker.connections) >= MaxBrokerConnections {
			go connection.Close()
			return
		}
		broker.connections[connection] = struct{}{}
	case http.StateHijacked, http.StateClosed:
		delete(broker.connections, connection)
	}
}

func emptyBrokerParams(value any) bool {
	if value == nil {
		return true
	}
	object, ok := value.(map[string]any)
	return ok && len(object) == 0
}

func closedBrokerCapabilities(capabilities map[string]any) bool {
	allowed := map[string]struct{}{
		"roots": {}, "sampling": {}, "elicitation": {}, "experimental": {},
	}
	for name, value := range capabilities {
		if _, ok := allowed[name]; !ok {
			return false
		}
		if _, ok := value.(map[string]any); !ok {
			return false
		}
	}
	return true
}
