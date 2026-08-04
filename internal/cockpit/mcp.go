package cockpit

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	mcpProtocolVersion = "2025-03-26"
	mcpApprovalTool    = "sworn_approve"
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
		response.Result = map[string]any{"tools": []any{approvalMCPTool()}}
	case "tools/call":
		response = h.callMCPApproval(r, request)
	default:
		response.Error = &mcpError{Code: -32601, Message: "Method not found"}
	}
	h.writeMCP(w, r, response)
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
