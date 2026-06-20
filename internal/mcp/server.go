// Package mcp implements a JSON-RPC 2.0 server over stdio that speaks the
// Model Context Protocol (MCP) 2024-11-05. It handles the initialize/initialized
// handshake and provides a registration API for tool handlers consumed by later
// slices (S08b, S08c).
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// ---- MCP 2024-11-05 wire types ----

// ToolResult is the response payload for a tools/call request.
// Maps to the MCP 2024-11-05 wire shape: {isError: bool, content: [{type, text}]}.
type ToolResult struct {
	IsError bool          `json:"isError"`
	Content []ContentItem `json:"content"`
}

// ContentItem is a single item in a tool call result's content array.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolHandler processes a tools/call request. Implemented by S08b and S08c.
type ToolHandler func(ctx context.Context, params json.RawMessage) (*ToolResult, error)

// ---- JSON-RPC 2.0 wire types ----

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	codeMethodNotFound = -32601
	codeInvalidRequest = -32600
	codeParseError     = -32700
)

// ---- Server ----

// Server is an MCP JSON-RPC 2.0 server over stdio. Create with New(), then Run().
type Server struct {
	mu      sync.Mutex
	tools   map[string]ToolHandler            // name -> handler
	schemas map[string]json.RawMessage        // name -> input schema
}

// New creates a new MCP server with no registered tools.
func New() *Server {
	return &Server{
		tools:   make(map[string]ToolHandler),
		schemas: make(map[string]json.RawMessage),
	}
}

// RegisterTool registers a tool handler and its input schema. Handlers are
// invoked by tools/call requests. S08b and S08c call this from their init.
func (s *Server) RegisterTool(name string, inputSchema json.RawMessage, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[name] = handler
	s.schemas[name] = inputSchema
}

// Run starts the MCP server, reading JSON-RPC 2.0 requests from r (typically
// stdin) and writing responses to w (typically stdout). It blocks until r
// returns EOF or ctx is cancelled.
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	// All logging goes to stderr — stdout is reserved for the MCP protocol.
	logger := log.New(os.Stderr, "[sworn mcp] ", log.LstdFlags|log.Lmsgprefix)

	scanner := bufio.NewScanner(r)
	// Pin 1: 4 MB token limit — large enough for tool-call payloads carrying
	// file contents, diffs, and spec text that S08b/S08c will send.
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	scanner.Split(bufio.ScanLines)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	methodHandlers := s.buildMethodHandlers()

	// Channel-based read loop so ctx cancellation unblocks immediately.
	type lineOrErr struct {
		line string
		err  error
	}
	lines := make(chan lineOrErr, 1)
	go func() {
		defer close(lines)
		for scanner.Scan() {
			select {
			case lines <- lineOrErr{line: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			lines <- lineOrErr{err: err}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case loe, ok := <-lines:
			if !ok {
				return nil // scanner finished cleanly (stdin EOF)
			}
			if loe.err != nil {
				return fmt.Errorf("read error: %w", loe.err)
			}
			line := loe.line
			if line == "" {
				continue
			}

			// Batch-request rejection: per spec Risk, MCP clients rarely send
			// batches over stdio. Log the limitation and return an error.
			if len(line) > 0 && line[0] == '[' {
				logger.Printf("batch request rejected (limitation: single-request only): %.100s", line)
				s.writeError(enc, nil, codeInvalidRequest, "Batch requests not supported")
				continue
			}

			var req jsonRPCRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				logger.Printf("parse error: %v", err)
				s.writeError(enc, nil, codeParseError, "Parse error")
				continue
			}

			if req.JSONRPC != "2.0" {
				s.writeError(enc, req.ID, codeInvalidRequest, "Invalid JSON-RPC version")
				continue
			}

			handler, ok := methodHandlers[req.Method]
			if !ok {
				s.writeError(enc, req.ID, codeMethodNotFound, "Method not found")
				continue
			}

			handler(ctx, &req, enc, logger)
		}
	}
}
// methodHandler processes a single JSON-RPC request and writes a response.
type methodHandler func(ctx context.Context, req *jsonRPCRequest, enc *json.Encoder, logger *log.Logger)

func (s *Server) buildMethodHandlers() map[string]methodHandler {
	return map[string]methodHandler{
		"initialize":    s.handleInitialize,
		"initialized":   s.handleInitialized,
		"tools/list":    s.handleToolsList,
		"tools/call":    s.handleToolsCall,
		"resources/list": s.handleResourcesList,
		"prompts/list":  s.handlePromptsList,
	}
}

// ---- Handlers ----

type initializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities"`
	ServerInfo      serverInfo      `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

var capabilities = json.RawMessage(`{"tools":{},"resources":{"listChanged":false},"prompts":{}}`)

func (s *Server) handleInitialize(ctx context.Context, req *jsonRPCRequest, enc *json.Encoder, logger *log.Logger) {
	logger.Println("initialize handshake received")
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  mustMarshal(initializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    capabilities,
			ServerInfo: serverInfo{
				Name:    "sworn-mcp",
				Version: "0.1.0",
			},
		}),
	}
	_ = enc.Encode(resp)
}

func (s *Server) handleInitialized(ctx context.Context, req *jsonRPCRequest, enc *json.Encoder, logger *log.Logger) {
	// initialized is a notification (no response expected), but we accept it
	// gracefully either way. MCP 2024-11-05: client sends this after receiving
	// the initialize result to confirm the session is established.
	logger.Println("initialized notification received — session established")
}

type toolDescription struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []toolDescription `json:"tools"`
}

func (s *Server) handleToolsList(ctx context.Context, req *jsonRPCRequest, enc *json.Encoder, logger *log.Logger) {
	s.mu.Lock()
	tools := make([]toolDescription, 0, len(s.tools))
	for name := range s.tools {
		tools = append(tools, toolDescription{
			Name:        name,
			InputSchema: s.schemas[name],
		})
	}
	s.mu.Unlock()

	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  mustMarshal(toolsListResult{Tools: tools}),
	}
	_ = enc.Encode(resp)
}

func (s *Server) handleToolsCall(ctx context.Context, req *jsonRPCRequest, enc *json.Encoder, logger *log.Logger) {
	// tools/call params: {name: string, arguments: object}
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		s.writeError(enc, req.ID, codeInvalidRequest, "Invalid tools/call params")
		return
	}

	s.mu.Lock()
	handler, ok := s.tools[params.Name]
	s.mu.Unlock()

	if !ok {
		// Unknown tool: return isError:true stub (not a JSON-RPC error)
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  mustMarshal(ToolResult{IsError: true, Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("tool %q not implemented", params.Name)}}}),
		}
		_ = enc.Encode(resp)
		return
	}

	result, err := handler(ctx, params.Arguments)
	if err != nil {
		logger.Printf("tool %q handler error: %v", params.Name, err)
		result = &ToolResult{IsError: true, Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("tool %q error: %v", params.Name, err)}}}
	}
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  mustMarshal(result),
	}
	_ = enc.Encode(resp)
}

type resourcesListResult struct {
	Resources []json.RawMessage `json:"resources"`
}

func (s *Server) handleResourcesList(ctx context.Context, req *jsonRPCRequest, enc *json.Encoder, logger *log.Logger) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  mustMarshal(resourcesListResult{Resources: []json.RawMessage{}}),
	}
	_ = enc.Encode(resp)
}

type promptsListResult struct {
	Prompts []json.RawMessage `json:"prompts"`
}

func (s *Server) handlePromptsList(ctx context.Context, req *jsonRPCRequest, enc *json.Encoder, logger *log.Logger) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  mustMarshal(promptsListResult{Prompts: []json.RawMessage{}}),
	}
	_ = enc.Encode(resp)
}

// ---- Helpers ----

func (s *Server) writeError(enc *json.Encoder, id json.RawMessage, code int, message string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonRPCError{
			Code:    code,
			Message: message,
		},
	}
	_ = enc.Encode(resp)
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mcp: mustMarshal: %v", err))
	}
	return b
}