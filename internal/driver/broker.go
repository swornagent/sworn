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

type nativeHandshakeEvidence struct {
	Protocol           string
	ClientName         string
	ClientVersion      string
	InitializeDigest   string
	NotificationDigest string
	ListDigest         string
	ToolDigest         string
}

type nativeBrokerSession interface {
	brokerToolDefinitions() []providerToolDefinition
	execute(context.Context, providerToolCall) providerToolResult
	terminated() (bool, error)
	// observeToolResultTurn projects one turn of results that crossed
	// back into the model onto the runtime-provided hook. The automation
	// session implements it as a no-op: automation dispatches journal
	// nothing by construction.
	observeToolResultTurn(turn int64, results []providerToolResult)
	// redactionSecrets returns the credentials the engine holds at the
	// broker seam; the projection redacts them before emission.
	redactionSecrets() [][]byte
}

type nativeBroker struct {
	mu                 sync.Mutex
	callMu             sync.Mutex
	state              brokerState
	armed              bool
	initialized        bool
	notified           bool
	listed             bool
	ready              bool
	protocol           string
	clientName         string
	clientVer          string
	initializeDigest   string
	notificationDigest string
	listDigest         string
	expected           *nativeHandshakeEvidence
	calls              int
	callsByName        map[string]int64
	address            string
	token              []byte
	session            nativeBrokerSession
	server             *http.Server
	listener           net.Listener
	terminal           chan struct{}
	closeOnce          sync.Once
	connMu             sync.Mutex
	connections        map[net.Conn]struct{}
	// turnSource attributes crossings to the native state's turn counter;
	// pending coalesces one turn until it changes or the broker finishes.
	turnSource  func() int64
	pendingTurn int64
	pending     []providerToolResult
}

func newNativeBroker(
	session nativeBrokerSession,
	expected ...nativeHandshakeEvidence,
) (*nativeBroker, error) {
	if session == nil {
		return nil, fail("INVALID_BROKER")
	}
	if len(expected) > 1 {
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
		callsByName: make(map[string]int64),
	}
	if len(expected) == 1 {
		value := expected[0]
		if value.Protocol == "" || value.ClientName == "" ||
			value.ClientVersion == "" ||
			!digestPattern.MatchString(value.InitializeDigest) ||
			!digestPattern.MatchString(value.NotificationDigest) ||
			!digestPattern.MatchString(value.ListDigest) ||
			value.ToolDigest != nativeBrokerToolSurfaceDigest(session) {
			_ = listener.Close()
			clearBytes(token)
			return nil, fail("INVALID_BROKER")
		}
		broker.expected = &value
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

// bindTurnSource wires the native state's turn counter into the broker so
// per-turn coalescing attributes each crossing to the turn the model was
// in. Production runNative always binds it; an unbound broker (unit
// fixtures) falls back to per-call turn attribution by call ordinal.
func (broker *nativeBroker) bindTurnSource(source func() int64) {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	broker.turnSource = source
}

func (broker *nativeBroker) currentTurnLocked(callNumber int64) int64 {
	if broker.turnSource == nil {
		return callNumber
	}
	return broker.turnSource()
}

// observeCallResult records one crossing into the current turn's pending
// coalescing bucket and flushes the previous turn when the turn changes.
// It is called only after the RECOVERY_STEP_REFUSED early return, so a
// result that never crosses is never journaled.
func (broker *nativeBroker) observeCallResult(
	callNumber int64,
	result providerToolResult,
) {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	turn := broker.currentTurnLocked(callNumber)
	if len(broker.pending) != 0 && broker.pendingTurn != turn {
		broker.flushPendingLocked()
	}
	broker.pendingTurn = turn
	broker.pending = append(broker.pending, result)
}

// flushPending emits the current pending turn, if any. It is idempotent and
// is also called explicitly by runNative before the tool session closes.
func (broker *nativeBroker) flushPending() {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	broker.flushPendingLocked()
}

func (broker *nativeBroker) flushPendingLocked() {
	if len(broker.pending) == 0 {
		return
	}
	turn := broker.pendingTurn
	records := broker.pending
	broker.pending = nil
	broker.pendingTurn = 0
	broker.session.observeToolResultTurn(turn, records)
}

func (broker *nativeBroker) capability() []byte {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	return append([]byte(nil), broker.token...)
}

func (broker *nativeBroker) Arm() error {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.state != brokerClosed || broker.armed {
		return fail("BROKER_STATE_INVALID")
	}
	broker.armed = true
	broker.maybeOpenLocked()
	return nil
}

func (broker *nativeBroker) Ready() bool {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	return broker.ready
}

// toolCallTotal and toolCallsByName snapshot the per-name executed-call
// counts for turn-economics capture at the native observation seam.
func (broker *nativeBroker) toolCallTotal() int64 {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	var total int64
	for _, count := range broker.callsByName {
		total += count
	}
	return total
}

func (broker *nativeBroker) toolCallsByName() map[string]int64 {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	result := make(map[string]int64, len(broker.callsByName))
	for name, count := range broker.callsByName {
		result[name] = count
	}
	return result
}

func (broker *nativeBroker) HandshakeEvidence() (nativeHandshakeEvidence, error) {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if !broker.ready {
		return nativeHandshakeEvidence{}, fail("BROKER_STATE_INVALID")
	}
	return nativeHandshakeEvidence{
		Protocol:           broker.protocol,
		ClientName:         broker.clientName,
		ClientVersion:      broker.clientVer,
		InitializeDigest:   broker.initializeDigest,
		NotificationDigest: broker.notificationDigest,
		ListDigest:         broker.listDigest,
		ToolDigest:         nativeBrokerToolSurfaceDigest(broker.session),
	}, nil
}

func (broker *nativeBroker) maybeOpenLocked() {
	if broker.state == brokerClosed && broker.armed &&
		broker.initialized && broker.notified && broker.listed &&
		broker.matchesExpectedLocked() {
		broker.state = brokerOpen
		broker.ready = true
	}
}

func (broker *nativeBroker) matchesExpectedLocked() bool {
	return broker.expected == nil ||
		(broker.protocol == broker.expected.Protocol &&
			broker.clientName == broker.expected.ClientName &&
			broker.clientVer == broker.expected.ClientVersion &&
			broker.initializeDigest == broker.expected.InitializeDigest &&
			broker.notificationDigest == broker.expected.NotificationDigest &&
			broker.listDigest == broker.expected.ListDigest &&
			nativeBrokerToolSurfaceDigest(broker.session) ==
				broker.expected.ToolDigest)
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
	// Results that already crossed still belong in the durable stream:
	// flush the pending turn before the broker goes quiet.
	broker.flushPendingLocked()
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
		if err := broker.markNotified(root["params"]); err != nil {
			writeBrokerError(writer, http.StatusConflict, id, -32000, "state_invalid")
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	case "tools/list":
		if _, present := root["id"]; !present ||
			!validBrokerListParams(root["params"]) {
			writeBrokerError(writer, http.StatusBadRequest, id, -32600, "invalid_request")
			return
		}
		broker.listTools(writer, id, root["params"])
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
		protocol != "2025-06-18" && protocol != "2025-11-25" {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "protocol_refused")
		return
	}
	capabilities, capabilitiesOK := object["capabilities"].(map[string]any)
	clientInfo, clientInfoErr := closedObject(
		object["clientInfo"],
		[]string{"name", "version"},
		[]string{"title", "description", "websiteUrl"},
	)
	clientName, nameOK := clientInfo["name"].(string)
	clientVersion, versionOK := clientInfo["version"].(string)
	clientInfoOK := true
	for _, field := range []string{"title", "description", "websiteUrl"} {
		if value, present := clientInfo[field]; present {
			text, ok := value.(string)
			clientInfoOK = clientInfoOK &&
				ok && validateText(text, 512, false) == nil
		}
	}
	initializeBody, initializeErr := canonicalJSON(params)
	defer clearBytes(initializeBody)
	if !capabilitiesOK ||
		!closedBrokerCapabilities(capabilities) ||
		clientInfoErr != nil || !clientInfoOK || !nameOK || !versionOK ||
		validateText(clientName, 256, false) != nil ||
		validateText(clientVersion, 256, false) != nil ||
		initializeErr != nil {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "invalid_params")
		return
	}
	broker.mu.Lock()
	if broker.state != brokerClosed || broker.initialized ||
		(broker.expected != nil &&
			(protocol != broker.expected.Protocol ||
				clientName != broker.expected.ClientName ||
				clientVersion != broker.expected.ClientVersion ||
				Digest(initializeBody) != broker.expected.InitializeDigest)) {
		broker.mu.Unlock()
		writeBrokerError(writer, http.StatusConflict, id, -32000, "state_invalid")
		return
	}
	broker.initialized = true
	broker.protocol = protocol
	broker.clientName = clientName
	broker.clientVer = clientVersion
	broker.initializeDigest = Digest(initializeBody)
	broker.mu.Unlock()
	writeBrokerResult(writer, id, map[string]any{
		"protocolVersion": protocol,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "sworn", "version": "1.0.0"},
	})
}

func (broker *nativeBroker) markNotified(params any) error {
	body, err := canonicalJSON(params)
	if err != nil {
		return fail("BROKER_STATE_INVALID")
	}
	defer clearBytes(body)
	broker.mu.Lock()
	defer broker.mu.Unlock()
	digest := Digest(body)
	if broker.state != brokerClosed || !broker.initialized || broker.notified ||
		(broker.expected != nil &&
			digest != broker.expected.NotificationDigest) {
		return fail("BROKER_STATE_INVALID")
	}
	broker.notified = true
	broker.notificationDigest = digest
	return nil
}

func (broker *nativeBroker) listTools(
	writer http.ResponseWriter,
	id json.RawMessage,
	params any,
) {
	paramsBody, err := canonicalJSON(params)
	if err != nil {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "invalid_params")
		return
	}
	defer clearBytes(paramsBody)
	paramsDigest := Digest(paramsBody)
	broker.mu.Lock()
	if broker.state != brokerClosed || !broker.initialized ||
		!broker.notified || broker.listed ||
		(broker.expected != nil &&
			paramsDigest != broker.expected.ListDigest) {
		broker.mu.Unlock()
		writeBrokerError(writer, http.StatusConflict, id, -32000, "state_invalid")
		return
	}
	broker.mu.Unlock()
	definitions := broker.session.brokerToolDefinitions()
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
	broker.mu.Lock()
	if broker.state != brokerClosed || broker.listed {
		broker.mu.Unlock()
		writeBrokerError(writer, http.StatusConflict, id, -32000, "state_invalid")
		return
	}
	broker.listed = true
	broker.listDigest = paramsDigest
	broker.maybeOpenLocked()
	broker.mu.Unlock()
	writeBrokerResult(writer, id, map[string]any{"tools": tools})
}

func (session *toolSession) brokerToolDefinitions() []providerToolDefinition {
	if session == nil {
		return nil
	}
	return toolDefinitions(session.invocation.Request.Workspace.Access)
}

func nativeBrokerToolSurfaceDigest(session nativeBrokerSession) string {
	if session == nil {
		return ""
	}
	return nativeToolDefinitionsDigest(session.brokerToolDefinitions())
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
	object, err := closedObject(
		params,
		[]string{"name", "arguments"},
		[]string{"_meta"},
	)
	if err != nil {
		writeBrokerError(writer, http.StatusBadRequest, id, -32602, "invalid_params")
		return
	}
	if metadata, present := object["_meta"]; present {
		if _, ok := metadata.(map[string]any); !ok {
			writeBrokerError(writer, http.StatusBadRequest, id, -32602, "invalid_params")
			return
		}
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
	broker.mu.Lock()
	broker.callsByName[name]++
	broker.mu.Unlock()
	terminated, terminalErr := broker.session.terminated()
	if terminated && IsCode(terminalErr, "RECOVERY_STEP_REFUSED") {
		broker.finish(brokerTerminal)
		writeBrokerError(
			writer,
			http.StatusConflict,
			id,
			-32000,
			"closed",
		)
		return
	}
	// The result crosses into the model here; project it at this exact
	// seam, after the refused early return and before writeBrokerResult,
	// so the journaled bytes are exactly what the native model receives
	// as `"text": string(result.Content)`.
	broker.observeCallResult(int64(callNumber), result)
	writeBrokerResult(writer, id, map[string]any{
		"content": []map[string]any{{
			"type": "text", "text": string(result.Content),
		}},
		"isError": result.Failed,
	})
	if terminated {
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

func validBrokerListParams(value any) bool {
	if emptyBrokerParams(value) {
		return true
	}
	object, err := closedObject(value, nil, []string{"cursor", "_meta"})
	if err != nil {
		return false
	}
	if cursor, present := object["cursor"]; present {
		if cursor != nil {
			text, ok := cursor.(string)
			if !ok || validateText(text, 4_096, false) != nil {
				return false
			}
		}
	}
	if meta, present := object["_meta"]; present {
		metaObject, err := closedObject(meta, nil, []string{"progressToken"})
		if err != nil {
			return false
		}
		if token, present := metaObject["progressToken"]; present {
			if _, ok := safeJSONInt(token); !ok {
				text, textOK := token.(string)
				if !textOK || validateText(text, 256, false) != nil {
					return false
				}
			}
		}
	}
	return true
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
