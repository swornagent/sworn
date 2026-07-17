package model

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/swornagent/sworn/internal/baton/schemas"
)

// sampleSchema is a lenient canonical-style schema: additionalProperties:true
// (implicit), thin required, a nested object, an array, and an optional field.
var sampleSchema = []byte(`{
  "$id": "https://baton.sawy3r.net/schemas/verifier-verdict-v1.json",
  "type": "object",
  "properties": {
    "verdict": {"type": "string", "enum": ["PASS", "FAIL", "BLOCKED"]},
    "summary": {"type": "string"},
    "violations": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "criterion": {"type": "string"},
          "detail": {"type": "string"}
        },
        "required": ["criterion"]
      }
    },
    "evidence": {
      "type": "object",
      "properties": {
        "files": {"type": "array", "items": {"type": "string"}}
      }
    }
  },
  "required": ["verdict"]
}`)

// --- strict projection -------------------------------------------------------

func TestStrictProjection(t *testing.T) {
	out, err := strictProjection(sampleSchema)
	if err != nil {
		t.Fatalf("strictProjection: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal projected: %v", err)
	}

	// Root: additionalProperties:false and ALL keys required.
	if got["additionalProperties"] != false {
		t.Errorf("root additionalProperties = %v, want false", got["additionalProperties"])
	}
	req := toStringSet(got["required"])
	for _, k := range []string{"verdict", "summary", "violations", "evidence"} {
		if !req[k] {
			t.Errorf("root required missing %q (got %v)", k, got["required"])
		}
	}

	props := got["properties"].(map[string]any)

	// Optional-in-lenient field (summary) widened to nullable.
	summaryType := props["summary"].(map[string]any)["type"]
	if !typeIncludes(summaryType, "null") {
		t.Errorf("summary type = %v, want to include null", summaryType)
	}
	// Required-in-lenient field (verdict) NOT widened.
	verdictType := props["verdict"].(map[string]any)["type"]
	if typeIncludes(verdictType, "null") {
		t.Errorf("verdict type = %v, should not include null", verdictType)
	}

	// Nested object inside array items is sealed and all-required.
	items := props["violations"].(map[string]any)["items"].(map[string]any)
	if items["additionalProperties"] != false {
		t.Errorf("violations.items additionalProperties = %v, want false", items["additionalProperties"])
	}
	itemReq := toStringSet(items["required"])
	if !itemReq["criterion"] || !itemReq["detail"] {
		t.Errorf("violations.items required = %v, want criterion+detail", items["required"])
	}
	// detail was optional in the item → nullable now.
	detailType := items["properties"].(map[string]any)["detail"].(map[string]any)["type"]
	if !typeIncludes(detailType, "null") {
		t.Errorf("violations.items.detail type = %v, want nullable", detailType)
	}

	// Nested plain object (evidence) sealed too.
	evidence := props["evidence"].(map[string]any)
	if evidence["additionalProperties"] != false {
		t.Errorf("evidence additionalProperties = %v, want false", evidence["additionalProperties"])
	}
}

func TestStrictProjection_Invalid(t *testing.T) {
	if _, err := strictProjection([]byte(`not json`)); err == nil {
		t.Fatal("want error for non-JSON schema, got nil")
	}
}

func TestSchemaName(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		want   string
	}{
		{"from $id basename", `{"$id":"https://x/schemas/verifier-verdict-v1.json"}`, "verifier-verdict-v1"},
		{"from title when no id", `{"title":"My Schema"}`, "My_Schema"},
		{"default when empty", `{"type":"object"}`, "structured_output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := schemaName([]byte(tt.schema)); got != tt.want {
				t.Errorf("schemaName = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- OAI ChatStructured: native response_format path -------------------------

func TestOAI_ChatStructured_ResponseFormat(t *testing.T) {
	var captured chatRequest
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.Write(oaiResp([]struct{ content string }{{`{"verdict":"PASS","summary":"ok","violations":[],"evidence":{"files":[]}}`}}, nil))
	})
	o := &OAI{BaseURL: srv.URL, Model: "gpt-4.1-mini", APIKey: "sk-test", Structured: StructuredResponseFormat}

	cr, err := o.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "verify"}}, sampleSchema)
	if err != nil {
		t.Fatalf("ChatStructured: %v", err)
	}

	// The request carried a strict json_schema response_format with a projected schema.
	if captured.ResponseFormat == nil || captured.ResponseFormat.Type != "json_schema" {
		t.Fatalf("response_format not set to json_schema: %+v", captured.ResponseFormat)
	}
	if !captured.ResponseFormat.JSONSchema.Strict {
		t.Error("response_format.json_schema.strict = false, want true")
	}
	var projected map[string]any
	if err := json.Unmarshal(captured.ResponseFormat.JSONSchema.Schema, &projected); err != nil {
		t.Fatalf("projected schema not JSON: %v", err)
	}
	if projected["additionalProperties"] != false {
		t.Error("projected schema not sealed (additionalProperties != false)")
	}
	if captured.ResponseFormat.JSONSchema.Name != "verifier-verdict-v1" {
		t.Errorf("schema name = %q, want verifier-verdict-v1", captured.ResponseFormat.JSONSchema.Name)
	}

	// The emitted object is normalised into Content.
	if got := cr.Choices[0].Message.Content; got == "" {
		t.Error("expected non-empty structured content")
	}
}

// TestXAI_ChatStructured_ResponseFormat (S03 AC-03) proves the xai/ driver,
// as resolved through NewClient, drives the native strict json_schema
// structured path end-to-end: the emitted request carries a strict
// json_schema response_format and the response normalises into Content. The
// httptest server stands in for api.x.ai (no live dispatch). This confirms
// verifier/captain (which need ChatStructured) work on xai/ — the honest
// declared role set. Strict-schema acceptance by the LIVE xAI API is
// doc-confirmed (docs.x.ai structured-outputs), not asserted here; if a live
// wire quirk ever surfaces, D2's StructuredToolCall is the contained fallback.
func TestXAI_ChatStructured_ResponseFormat(t *testing.T) {
	var captured chatRequest
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.Write(oaiResp([]struct{ content string }{{`{"verdict":"PASS","summary":"ok","violations":[],"evidence":{"files":[]}}`}}, nil))
	})

	v, err := NewClient("xai/grok-4.5", ProviderConfig{XAIKey: "sk-xai"})
	if err != nil {
		t.Fatalf("NewClient(xai/grok-4.5): %v", err)
	}
	o := v.(*OAI)
	o.BaseURL = srv.URL // redirect the resolved xai client at the fixture

	cr, err := o.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "verify"}}, sampleSchema)
	if err != nil {
		t.Fatalf("ChatStructured: %v", err)
	}
	if captured.ResponseFormat == nil || captured.ResponseFormat.Type != "json_schema" {
		t.Fatalf("response_format not set to json_schema: %+v", captured.ResponseFormat)
	}
	if !captured.ResponseFormat.JSONSchema.Strict {
		t.Error("response_format.json_schema.strict = false, want true")
	}
	if got := cr.Choices[0].Message.Content; got == "" {
		t.Error("expected non-empty structured content")
	}
}

// TestOAIChatStructuredUsesLLMCheckEnvelopeOnlyForOpenAICompletions proves
// the profile is carried by both direct and proxy construction; no concrete
// OAI type or endpoint resemblance is enough to select the envelope.
func TestOAIChatStructuredUsesLLMCheckEnvelopeOnlyForOpenAICompletions(t *testing.T) {
	tests := []struct {
		name string
		new  func(t *testing.T, baseURL string) *OAI
	}{
		{
			name: "direct factory",
			new: func(t *testing.T, baseURL string) *OAI {
				t.Helper()
				v, err := NewClient("openai-completions/test-model", ProviderConfig{OpenAIKey: "synthetic-key"})
				if err != nil {
					t.Fatal(err)
				}
				client := v.(*OAI)
				client.BaseURL = baseURL
				return client
			},
		},
		{
			name: "proxy factory",
			new: func(t *testing.T, baseURL string) *OAI {
				t.Helper()
				client, ok := proxyClient("openai-completions", "test-model", baseURL, "proxy-token").(*OAI)
				if !ok {
					t.Fatalf("proxy client = %T, want *OAI", proxyClient("openai-completions", "test-model", baseURL, "proxy-token"))
				}
				return client
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured chatRequest
			srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &captured); err != nil {
					t.Errorf("unmarshal request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write(oaiResp([]struct{ content string }{{`{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`}}, nil))
			})
			client := tt.new(t, srv.URL)

			if _, err := client.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "verify"}}, schemas.LLMCheckReportV1); err != nil {
				t.Fatalf("ChatStructured: %v", err)
			}
			if captured.ResponseFormat == nil || captured.ResponseFormat.JSONSchema == nil {
				t.Fatal("response_format.json_schema not sent")
			}
			if got := captured.ResponseFormat.JSONSchema.Name; got != openAILLMCheckEnvelopeName {
				t.Errorf("schema name = %q, want %q", got, openAILLMCheckEnvelopeName)
			}
			if !captured.ResponseFormat.JSONSchema.Strict {
				t.Error("response_format strict = false, want true")
			}
			assertModelEnvelopeShape(t, captured.ResponseFormat.JSONSchema.Schema)
		})
	}
}

func TestXAIChatStructuredRetainsRawSchemaProfile(t *testing.T) {
	tests := []struct {
		name string
		new  func(t *testing.T, baseURL string) *OAI
	}{
		{
			name: "direct factory",
			new: func(t *testing.T, baseURL string) *OAI {
				t.Helper()
				v, err := NewClient("xai/test-model", ProviderConfig{XAIKey: "synthetic-key"})
				if err != nil {
					t.Fatal(err)
				}
				client := v.(*OAI)
				client.BaseURL = baseURL
				return client
			},
		},
		{
			name: "proxy factory",
			new: func(t *testing.T, baseURL string) *OAI {
				t.Helper()
				client, ok := proxyClient("xai", "test-model", baseURL, "proxy-token").(*OAI)
				if !ok {
					t.Fatal("xAI proxy did not return OAI")
				}
				return client
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured chatRequest
			srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &captured); err != nil {
					t.Errorf("unmarshal request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write(oaiResp([]struct{ content string }{{`{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`}}, nil))
			})
			client := tt.new(t, srv.URL)

			if _, err := client.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "verify"}}, schemas.LLMCheckReportV1); err != nil {
				t.Fatalf("ChatStructured: %v", err)
			}
			if captured.ResponseFormat == nil || captured.ResponseFormat.JSONSchema == nil {
				t.Fatal("xAI response_format.json_schema not sent")
			}
			if got := captured.ResponseFormat.JSONSchema.Name; got != "llm-check-report-v1" {
				t.Errorf("xAI schema name = %q, want original schema name", got)
			}
			if bytes.Contains(captured.ResponseFormat.JSONSchema.Schema, []byte(openAILLMCheckEnvelopeName)) || !bytes.Contains(captured.ResponseFormat.JSONSchema.Schema, []byte(`"allOf"`)) {
				t.Fatalf("xAI schema was transformed into an OpenAI envelope: %s", captured.ResponseFormat.JSONSchema.Schema)
			}
		})
	}
}

// --- OAI ChatStructured: tool-call fallback path -----------------------------

func TestOAI_ChatStructured_ToolCall(t *testing.T) {
	// ToolDef serialises via custom MarshalJSON (no Unmarshal), so inspect the
	// raw wire body as a generic map rather than round-tripping into chatRequest.
	var raw map[string]any
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		w.Header().Set("Content-Type", "application/json")
		// Forced tool call: the object is in the tool call arguments.
		w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"emit_structured_output","arguments":"{\"verdict\":\"FAIL\"}"}}]},"finish_reason":"tool_calls"}]}`))
	})
	o := &OAI{BaseURL: srv.URL, Model: "deepseek-chat", APIKey: "sk-test", Structured: StructuredToolCall}

	cr, err := o.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "verify"}}, sampleSchema)
	if err != nil {
		t.Fatalf("ChatStructured: %v", err)
	}

	// The request forced a single emit tool whose parameters ARE the schema.
	tools, _ := raw["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected one forced tool, got %v", raw["tools"])
	}
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != structuredToolName {
		t.Errorf("tool name = %v, want %q", fn["name"], structuredToolName)
	}
	if _, ok := fn["parameters"].(map[string]any); !ok {
		t.Error("tool parameters not carried as the schema")
	}
	if _, ok := raw["tool_choice"]; !ok {
		t.Error("tool_choice not set to force the tool")
	}
	if _, ok := raw["response_format"]; ok {
		t.Error("tool-call path must not set response_format")
	}
	// The tool arguments were lifted into Content.
	if got := cr.Choices[0].Message.Content; got != `{"verdict":"FAIL"}` {
		t.Errorf("content = %q, want the tool arguments JSON", got)
	}
}

func TestToolCallChatStructuredRetainsRawSchema(t *testing.T) {
	tests := []struct {
		name string
		new  func(t *testing.T, baseURL string) *OAI
	}{
		{
			name: "direct factory",
			new: func(t *testing.T, baseURL string) *OAI {
				t.Helper()
				v, err := NewClient("deepseek/test-model", ProviderConfig{DeepSeekKey: "synthetic-key"})
				if err != nil {
					t.Fatal(err)
				}
				client := v.(*OAI)
				client.BaseURL = baseURL
				return client
			},
		},
		{
			name: "proxy factory",
			new: func(t *testing.T, baseURL string) *OAI {
				t.Helper()
				client, ok := proxyClient("deepseek", "test-model", baseURL, "proxy-token").(*OAI)
				if !ok {
					t.Fatal("DeepSeek proxy did not return OAI")
				}
				return client
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request struct {
				Tools []struct {
					Function struct {
						Parameters json.RawMessage `json:"parameters"`
					} `json:"function"`
				} `json:"tools"`
				ResponseFormat json.RawMessage `json:"response_format"`
			}
			srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &request); err != nil {
					t.Errorf("unmarshal request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"emit_structured_output","arguments":"{\"check\":\"ac-satisfaction\",\"verdict\":\"PASS\",\"findings\":[]}"}}]}}]}`))
			})
			client := tt.new(t, srv.URL)

			if _, err := client.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "verify"}}, schemas.LLMCheckReportV1); err != nil {
				t.Fatalf("ChatStructured: %v", err)
			}
			if len(request.Tools) != 1 {
				t.Fatalf("tool count = %d, want 1", len(request.Tools))
			}
			if len(request.ResponseFormat) != 0 {
				t.Fatalf("tool-call path emitted response_format: %s", request.ResponseFormat)
			}
			if bytes.Contains(request.Tools[0].Function.Parameters, []byte(openAILLMCheckEnvelopeName)) || !bytes.Contains(request.Tools[0].Function.Parameters, []byte(`"allOf"`)) {
				t.Fatalf("tool parameters were transformed into an envelope: %s", request.Tools[0].Function.Parameters)
			}
		})
	}
}

func TestOpenRouterChatStructuredUsesCanonicalForcedTool(t *testing.T) {
	var request struct {
		Model string `json:"model"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name       string          `json:"name"`
				Parameters json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
		ToolChoice struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tool_choice"`
		ResponseFormat json.RawMessage `json:"response_format"`
	}
	server := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(structuredToolCallResponse([]ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: FunctionCall{
				Name:      structuredToolName,
				Arguments: `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`,
			},
		}}))
	})

	v, err := NewClient("openrouter/z-ai/glm-5.2", ProviderConfig{OpenRouterKey: "synthetic-openrouter-key"})
	if err != nil {
		t.Fatalf("NewClient(openrouter/...): %v", err)
	}
	client := v.(*OAI)
	client.BaseURL = server.URL
	response, err := client.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "verify"}}, schemas.LLMCheckReportV1)
	if err != nil {
		t.Fatalf("ChatStructured: %v", err)
	}
	if response.Choices[0].Message.Content != `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}` {
		t.Errorf("structured content = %q, want tool arguments", response.Choices[0].Message.Content)
	}
	if request.Model != "z-ai/glm-5.2" {
		t.Errorf("model = %q, want z-ai/glm-5.2", request.Model)
	}
	if len(request.Tools) != 1 || request.Tools[0].Type != "function" || request.Tools[0].Function.Name != structuredToolName {
		t.Fatalf("tools = %+v, want one nested forced tool", request.Tools)
	}
	canonical, err := json.Marshal(json.RawMessage(schemas.LLMCheckReportV1))
	if err != nil {
		t.Fatalf("marshal canonical report as wire JSON: %v", err)
	}
	if !bytes.Equal(request.Tools[0].Function.Parameters, canonical) {
		t.Error("OpenRouter parameters were not the supplied canonical report")
	}
	if request.ToolChoice.Type != "function" || request.ToolChoice.Function.Name != structuredToolName {
		t.Errorf("tool_choice = %+v, want forced emit_structured_output", request.ToolChoice)
	}
	if len(request.ResponseFormat) != 0 {
		t.Errorf("OpenRouter request unexpectedly sent response_format: %s", request.ResponseFormat)
	}
}

func TestOpenRouterStructuredRejectsInvalidToolCall(t *testing.T) {
	valid := ToolCall{Type: "function", Function: FunctionCall{Name: structuredToolName, Arguments: `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`}}
	tests := []struct {
		name  string
		calls []ToolCall
	}{
		{name: "zero calls"},
		{name: "multiple calls", calls: []ToolCall{valid, valid}},
		{name: "wrong name", calls: []ToolCall{{Type: "function", Function: FunctionCall{Name: "other_function", Arguments: valid.Function.Arguments}}}},
		{name: "non-object arguments", calls: []ToolCall{{Type: "function", Function: FunctionCall{Name: structuredToolName, Arguments: `[]`}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.Write(structuredToolCallResponse(tt.calls))
			})
			v, err := NewClient("openrouter/z-ai/glm-5.2", ProviderConfig{OpenRouterKey: "synthetic-openrouter-key"})
			if err != nil {
				t.Fatal(err)
			}
			client := v.(*OAI)
			client.BaseURL = server.URL
			if _, err := client.ChatStructured(context.Background(), nil, schemas.LLMCheckReportV1); err == nil {
				t.Fatal("invalid OpenRouter tool response unexpectedly succeeded")
			}
			if calls != 1 {
				t.Fatalf("OpenRouter dispatch count = %d, want 1 with no retry or fallback", calls)
			}
		})
	}
}

func TestDeepSeekForcedToolPathRetainsLegacyFirstCallBehavior(t *testing.T) {
	dispatches := 0
	server := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		dispatches++
		w.Header().Set("Content-Type", "application/json")
		w.Write(structuredToolCallResponse([]ToolCall{{
			Type: "function",
			Function: FunctionCall{
				Name:      "legacy_first_tool",
				Arguments: `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`,
			},
		}}))
	})
	v, err := NewClient("deepseek/test-model", ProviderConfig{DeepSeekKey: "synthetic-deepseek-key"})
	if err != nil {
		t.Fatal(err)
	}
	client := v.(*OAI)
	client.BaseURL = server.URL
	response, err := client.ChatStructured(context.Background(), nil, schemas.LLMCheckReportV1)
	if err != nil {
		t.Fatalf("DeepSeek legacy forced-tool response changed behavior: %v", err)
	}
	if response.Choices[0].Message.Content != `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}` {
		t.Errorf("legacy DeepSeek content = %q, want first tool arguments", response.Choices[0].Message.Content)
	}
	if dispatches != 1 {
		t.Fatalf("DeepSeek dispatch count = %d, want 1", dispatches)
	}
}

func TestOAI_ChatStructured_ToolCall_NoCall(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Model answered in prose instead of calling the tool.
		w.Write(oaiResp([]struct{ content string }{{"sorry, I can't"}}, nil))
	})
	o := &OAI{BaseURL: srv.URL, Model: "deepseek-chat", APIKey: "sk-test", Structured: StructuredToolCall}
	if _, err := o.ChatStructured(context.Background(), nil, sampleSchema); err == nil {
		t.Fatal("want error when model returns no tool call, got nil")
	}
}

func structuredToolCallResponse(calls []ToolCall) []byte {
	response := ChatResponse{}
	response.Choices = append(response.Choices, struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}{})
	response.Choices[0].Message.ToolCalls = calls
	response.Choices[0].FinishReason = "tool_calls"
	encoded, _ := json.Marshal(response)
	return encoded
}

// --- fail-closed guards ------------------------------------------------------

func TestOAI_ChatStructured_FailClosed(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"prose not JSON", "PASS — looks good to me"},
		{"empty", ""},
		{"JSON array not object", "[1,2,3]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(oaiResp([]struct{ content string }{{tt.content}}, nil))
			})
			o := &OAI{BaseURL: srv.URL, Model: "gpt-4.1-mini", APIKey: "sk-test", Structured: StructuredResponseFormat}
			if _, err := o.ChatStructured(context.Background(), nil, sampleSchema); err == nil {
				t.Fatalf("want fail-closed error for %s, got nil", tt.name)
			}
		})
	}
}

func TestOAI_ChatStructured_Unsupported(t *testing.T) {
	o := &OAI{BaseURL: "http://unused", Model: "groq-llama", APIKey: "sk-test"} // no Structured mode
	if _, err := o.ChatStructured(context.Background(), nil, sampleSchema); err == nil {
		t.Fatal("want error from driver without structured support, got nil")
	}
}

// --- OpenAIResponses ChatStructured ------------------------------------------

func TestOpenAIResponses_ChatStructured(t *testing.T) {
	var captured responsesRequest
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		resp := responsesAPIResponse{
			Output: []responsesOutput{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: `{"verdict":"PASS"}`}}},
			},
		}
		b, _ := json.Marshal(resp)
		w.Write(b)
	})
	c := &OpenAIResponses{BaseURL: srv.URL, Model: "gpt-5.5", APIKey: "sk-test", ReasoningEffort: "medium"}

	cr, err := c.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "verify"}}, sampleSchema)
	if err != nil {
		t.Fatalf("ChatStructured: %v", err)
	}
	if captured.Text == nil || captured.Text.Format == nil || captured.Text.Format.Type != "json_schema" {
		t.Fatalf("text.format not set to json_schema: %+v", captured.Text)
	}
	if !captured.Text.Format.Strict {
		t.Error("text.format.strict = false, want true")
	}
	if got := cr.Choices[0].Message.Content; got != `{"verdict":"PASS"}` {
		t.Errorf("content = %q, want the emitted object", got)
	}
}

func TestOpenAIResponses_ChatStructured_FailClosed(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := responsesAPIResponse{
			Output: []responsesOutput{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "not json"}}},
			},
		}
		b, _ := json.Marshal(resp)
		w.Write(b)
	})
	c := &OpenAIResponses{BaseURL: srv.URL, Model: "gpt-5.5", APIKey: "sk-test"}
	if _, err := c.ChatStructured(context.Background(), nil, sampleSchema); err == nil {
		t.Fatal("want fail-closed error for non-JSON output, got nil")
	}
}

// --- helpers -----------------------------------------------------------------

func toStringSet(v any) map[string]bool {
	out := map[string]bool{}
	if arr, ok := v.([]any); ok {
		for _, e := range arr {
			if s, ok := e.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}

func typeIncludes(t any, want string) bool {
	switch v := t.(type) {
	case string:
		return v == want
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}
