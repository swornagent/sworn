package cockpit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	mcpProtocolVersion       = "2025-03-26"
	mcpApprovalTool          = "sworn_approve"
	mcpCaptainDelegationTool = "sworn_captain_delegation"
	mcpStartDelegatedTool    = "sworn_start_delegated"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func strictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func (h *HTTPHandler) serveMCP(w http.ResponseWriter, r *http.Request) {
	if !h.localRequest(r) {
		writeHTTPError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeHTTPError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	mediaBody := json.RawMessage{}
	if !decodeRequest(w, r, &mediaBody) {
		return
	}
	var request mcpRequest
	if strictJSON(mediaBody, &request) != nil || request.JSONRPC != "2.0" ||
		len(request.ID) == 0 || request.Method == "" {
		h.writeMCP(w, r, mcpResponse{JSONRPC: "2.0", Error: &mcpError{
			Code: -32600, Message: "Invalid Request",
		}})
		return
	}
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		response.Result = map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "sworn", "version": "1"},
		}
	case "tools/list":
		if len(request.Params) != 0 && string(request.Params) != "{}" {
			response.Error = &mcpError{Code: -32602, Message: "Invalid params"}
			break
		}
		response.Result = map[string]any{"tools": []any{approvalMCPTool(), captainDelegationMCPTool(), startDelegatedMCPTool()}}
	case "tools/call":
		response = h.callMCPTool(r, request)
	default:
		response.Error = &mcpError{Code: -32601, Message: "Method not found"}
	}
	h.writeMCP(w, r, response)
}

func startDelegatedMCPTool() map[string]any {
	return map[string]any{"name": mcpStartDelegatedTool, "description": "Start an admitted manifest with one exact Captain delegation before Planner dispatch.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"manifest_digest": map[string]any{"type": "string"}, "envelope_bytes": map[string]any{"type": "string"}}, "required": []string{"manifest_digest", "envelope_bytes"}}}
}

func captainDelegationMCPTool() map[string]any {
	properties := map[string]any{}
	for _, name := range []string{"schema_version", "action", "run_id", "manifest_digest", "actor_class", "actor_authority", "current_digest", "envelope_digest", "envelope_bytes"} {
		properties[name] = map[string]any{"type": "string"}
	}
	properties["action"] = map[string]any{"type": "string", "enum": []string{"admit", "revoke", "replace"}}
	properties["current_epoch"] = map[string]any{"type": "integer", "minimum": 0}
	return map[string]any{
		"name":        mcpCaptainDelegationTool,
		"description": "Admit, revoke, or replace one exact externally authorized Captain delegation envelope.",
		"inputSchema": map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": properties,
			"required":   []string{"schema_version", "action", "run_id", "manifest_digest", "actor_class", "actor_authority", "current_epoch", "current_digest", "envelope_digest", "envelope_bytes"},
		},
	}
}

type mcpCaptainDelegationCommands interface {
	CaptainDelegation(context.Context, runtimepkg.CaptainDelegationCommand) (runtimepkg.CaptainDelegationResult, error)
}
type mcpDelegatedStarter interface {
	StartDelegated(context.Context, StartDelegatedCommand) (runtimepkg.RunStatus, error)
}

func (h *HTTPHandler) callMCPTool(r *http.Request, request mcpRequest) mcpResponse {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if strictJSON(request.Params, &call) != nil {
		return mcpResponse{JSONRPC: "2.0", ID: request.ID, Error: &mcpError{Code: -32602, Message: "Invalid params"}}
	}
	if call.Name == mcpApprovalTool {
		return h.callMCPApproval(r, request)
	}
	if call.Name == mcpStartDelegatedTool {
		response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
		commands, ok := h.commands.(mcpDelegatedStarter)
		if !ok {
			response.Error = &mcpError{Code: -32601, Message: "Method not found"}
			return response
		}
		var command StartDelegatedCommand
		if strictJSON(call.Arguments, &command) != nil {
			response.Error = &mcpError{Code: -32602, Message: "Invalid params"}
			return response
		}
		result, err := commands.StartDelegated(r.Context(), command)
		if err != nil {
			response.Result = map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": errorCode(err)}}}
			return response
		}
		body, _ := json.Marshal(result)
		response.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": string(body)}}, "structuredContent": result}
		return response
	}
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	if call.Name != mcpCaptainDelegationTool {
		response.Error = &mcpError{Code: -32602, Message: "Invalid params"}
		return response
	}
	commands, ok := h.commands.(mcpCaptainDelegationCommands)
	if !ok {
		response.Error = &mcpError{Code: -32601, Message: "Method not found"}
		return response
	}
	var command runtimepkg.CaptainDelegationCommand
	if strictJSON(call.Arguments, &command) != nil {
		response.Error = &mcpError{Code: -32602, Message: "Invalid params"}
		return response
	}
	result, err := commands.CaptainDelegation(r.Context(), command)
	if err != nil {
		response.Result = map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": errorCode(err)}}}
		return response
	}
	body, _ := json.Marshal(result)
	response.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": string(body)}}, "structuredContent": result}
	return response
}

func approvalMCPTool() map[string]any {
	properties := map[string]any{}
	for _, name := range []string{
		"schema_version", "run_id", "manifest_digest", "project", "release",
		"release_ref", "release_head", "proposal_replay_key", "prior_plan",
		"plan_digest", "target_ref", "target_head", "decision_class",
		"decision", "actor_class", "actor_authority",
	} {
		properties[name] = map[string]any{"type": "string"}
	}
	properties["plan_revision"] = map[string]any{"type": "integer", "minimum": 1}
	return map[string]any{
		"name":        mcpApprovalTool,
		"description": "Admit one exact current plan approval over the local operator transport.",
		"inputSchema": map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": properties,
			"required": []string{
				"schema_version", "run_id", "manifest_digest", "project", "release",
				"release_ref", "proposal_replay_key", "plan_revision", "plan_digest",
				"target_ref", "target_head", "decision_class", "decision",
				"actor_class", "actor_authority",
			},
		},
	}
}

func (h *HTTPHandler) callMCPApproval(
	r *http.Request,
	request mcpRequest,
) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if strictJSON(request.Params, &call) != nil || call.Name != mcpApprovalTool {
		response.Error = &mcpError{Code: -32602, Message: "Invalid params"}
		return response
	}
	var command runtimepkg.ApprovalCommand
	if strictJSON(call.Arguments, &command) != nil {
		response.Error = &mcpError{Code: -32602, Message: "Invalid params"}
		return response
	}
	result, err := h.commands.Approve(r.Context(), command)
	if err != nil {
		response.Result = map[string]any{
			"isError": true,
			"content": []any{map[string]any{
				"type": "text", "text": errorCode(err),
			}},
		}
		return response
	}
	body, _ := json.Marshal(result)
	response.Result = map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": string(body)}},
		"structuredContent": result,
	}
	return response
}

func (h *HTTPHandler) writeMCP(
	w http.ResponseWriter,
	r *http.Request,
	response mcpResponse,
) {
	writeJSON(w, r, http.StatusOK, response)
}
