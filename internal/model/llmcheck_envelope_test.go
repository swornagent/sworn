package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/swornagent/sworn/internal/baton/schemas"
)

func TestCompileOpenAILLMCheckEnvelopeExactIdentity(t *testing.T) {
	source := append([]byte(nil), schemas.LLMCheckReportV1...)
	before := append([]byte(nil), source...)
	sum := sha256.Sum256(source)
	if got := hex.EncodeToString(sum[:]); got != canonicalLLMCheckReportDigest {
		t.Fatalf("canonical digest = %s, want %s", got, canonicalLLMCheckReportDigest)
	}

	for _, route := range []structuredProviderRoute{
		structuredRouteForProvider("openai"),
		structuredRouteForProvider("openai-responses"),
		structuredRouteForProvider("openai-completions"),
	} {
		t.Run(route.String(), func(t *testing.T) {
			envelope, ok, err := compileOpenAILLMCheckEnvelope(route.profile, route.wire, source)
			if err != nil {
				t.Fatalf("compileOpenAILLMCheckEnvelope: %v", err)
			}
			if !ok {
				t.Fatal("exact OpenAI route did not select the envelope")
			}
			if envelope.Name != openAILLMCheckEnvelopeName {
				t.Errorf("envelope name = %q, want %q", envelope.Name, openAILLMCheckEnvelopeName)
			}
			assertModelEnvelopeShape(t, envelope.Schema)
		})
	}
	if !bytes.Equal(source, before) {
		t.Fatal("compiler mutated canonical input bytes")
	}
}

func TestCompileOpenAILLMCheckEnvelopeClosedWorld(t *testing.T) {
	canonicalDigestMismatch := bytes.Replace(
		append([]byte(nil), schemas.LLMCheckReportV1...),
		[]byte(`"Baton LLM Check Report"`), []byte(`"CANARY-REPORT"`), 1,
	)
	futureFamily := bytes.Replace(
		append([]byte(nil), schemas.LLMCheckReportV1...),
		[]byte(`llm-check-report-v1.json`), []byte(`llm-check-report-v2.json`), 1,
	)
	unrelated := []byte(`{"$id":"https://example.invalid/schemas/not-a-llm-check.json","type":"object"}`)
	route := structuredRouteForProvider("openai-completions")

	tests := []struct {
		name       string
		source     []byte
		wantErr    error
		wantSelect bool
	}{
		{name: "canonical digest mismatch", source: canonicalDigestMismatch, wantErr: errOpenAIEnvelopeDigestMismatch},
		{name: "future generic-report family", source: futureFamily, wantErr: errOpenAIEnvelopeUnsupportedFamily},
		{name: "dedicated ambiguity map", source: schemas.SpecAmbiguityReportV1, wantErr: errOpenAIEnvelopeSpecAmbiguity},
		{name: "unrelated schema retains existing path", source: unrelated, wantSelect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope, selected, err := compileOpenAILLMCheckEnvelope(route.profile, route.wire, tt.source)
			if tt.wantErr != nil {
				if err == nil || err.Error() != tt.wantErr.Error() {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				if bytes.Contains([]byte(err.Error()), []byte("CANARY")) {
					t.Fatalf("local error leaked schema source: %q", err)
				}
				if selected || envelope.Name != "" || len(envelope.Schema) != 0 {
					t.Fatalf("rejected source selected envelope: %+v selected=%v", envelope, selected)
				}
				return
			}
			if err != nil || selected != tt.wantSelect {
				t.Fatalf("selected/error = %v/%v, want %v/nil", selected, err, tt.wantSelect)
			}
		})
	}

	for _, route := range []structuredProviderRoute{
		structuredRouteForProvider("xai"),
		structuredRouteForProvider("deepseek"),
		{},
	} {
		t.Run("default deny "+route.String(), func(t *testing.T) {
			envelope, selected, err := compileOpenAILLMCheckEnvelope(route.profile, route.wire, schemas.LLMCheckReportV1)
			if err != nil || selected || envelope.Name != "" || len(envelope.Schema) != 0 {
				t.Fatalf("default-deny route selected=%v envelope=%+v err=%v", selected, envelope, err)
			}
		})
	}
}

func TestStructuredRouteOpenRouterIsDirectOnly(t *testing.T) {
	direct := structuredRouteForProvider("openrouter")
	if direct.wire != structuredWireToolCall || direct.oaiMode != StructuredToolCall || direct.toolCallPolicy != structuredToolCallRequireExactEmit {
		t.Fatalf("direct OpenRouter route = %+v, want explicit exact forced-tool route", direct)
	}
	proxy := structuredRouteForProxyProvider("openrouter")
	if proxy != (structuredProviderRoute{}) {
		t.Fatalf("proxy OpenRouter route = %+v, want default deny", proxy)
	}

	for _, provider := range []string{"deepseek"} {
		direct := structuredRouteForProvider(provider)
		proxy := structuredRouteForProxyProvider(provider)
		if direct.wire != structuredWireToolCall || direct.oaiMode != StructuredToolCall || direct.toolCallPolicy != structuredToolCallLegacy {
			t.Fatalf("direct %s route = %+v, want unchanged legacy tool route", provider, direct)
		}
		if proxy.wire != structuredWireToolCall || proxy.oaiMode != StructuredToolCall || proxy.toolCallPolicy != structuredToolCallLegacy {
			t.Fatalf("proxy %s route = %+v, want unchanged legacy tool route", provider, proxy)
		}
	}
}

func TestOpenAIEnvelopeProfileRejectsUnsupportedCanonicalSchemasBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	canonicalDigestMismatch := bytes.Replace(
		append([]byte(nil), schemas.LLMCheckReportV1...),
		[]byte(`"Baton LLM Check Report"`), []byte(`"CANARY-REPORT"`), 1,
	)
	futureFamily := bytes.Replace(
		append([]byte(nil), schemas.LLMCheckReportV1...),
		[]byte(`llm-check-report-v1.json`), []byte(`llm-check-report-v2.json`), 1,
	)

	for _, route := range []struct {
		name string
		new  func(t *testing.T) StructuredOutput
	}{
		{
			name: "Responses",
			new: func(t *testing.T) StructuredOutput {
				t.Helper()
				v, err := NewClient("openai/test-model", ProviderConfig{OpenAIKey: "synthetic-key"})
				if err != nil {
					t.Fatal(err)
				}
				client := v.(*OpenAIResponses)
				client.BaseURL = server.URL
				return client
			},
		},
		{
			name: "chat completions",
			new: func(t *testing.T) StructuredOutput {
				t.Helper()
				v, err := NewClient("openai-completions/test-model", ProviderConfig{OpenAIKey: "synthetic-key"})
				if err != nil {
					t.Fatal(err)
				}
				client := v.(*OAI)
				client.BaseURL = server.URL
				return client
			},
		},
	} {
		for _, tt := range []struct {
			name   string
			schema []byte
		}{
			{name: "digest mismatch", schema: canonicalDigestMismatch},
			{name: "future generic family", schema: futureFamily},
			{name: "dedicated ambiguity map", schema: schemas.SpecAmbiguityReportV1},
		} {
			t.Run(route.name+"/"+tt.name, func(t *testing.T) {
				calls.Store(0)
				client := route.new(t)
				_, err := client.ChatStructured(context.Background(), []ChatMessage{{Role: "user", Content: "payload-canary"}}, tt.schema)
				if err == nil {
					t.Fatal("unsupported canonical schema unexpectedly dispatched")
				}
				if calls.Load() != 0 {
					t.Fatalf("HTTP calls = %d, want 0", calls.Load())
				}
				for _, leaked := range []string{"CANARY", "payload-canary", "synthetic-key"} {
					if bytes.Contains([]byte(err.Error()), []byte(leaked)) {
						t.Fatalf("local error leaked %q: %q", leaked, err)
					}
				}
			})
		}
	}
}

func assertModelEnvelopeShape(t *testing.T, raw []byte) {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Errorf("root = %#v, want sealed object", schema)
	}
	if !modelJSONStringsContain(schema["required"], "check", "verdict", "findings") {
		t.Errorf("root required = %#v", schema["required"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", schema["properties"])
	}
	check, ok := properties["check"].(map[string]any)
	if !ok || !modelJSONStringsContain(check["enum"], "spec-ambiguity", "design-review", "ac-satisfaction", "security-review", "semantic-coverage", "maintainability-review") {
		t.Errorf("check enum = %#v", check)
	}
	findings, ok := properties["findings"].(map[string]any)
	if !ok {
		t.Fatalf("findings = %#v", properties["findings"])
	}
	items, ok := findings["items"].(map[string]any)
	if !ok {
		t.Fatalf("findings.items = %#v", findings["items"])
	}
	if items["type"] != "object" || items["additionalProperties"] != false {
		t.Errorf("items = %#v, want sealed object", items)
	}
	if !modelJSONStringsContain(items["required"], "id", "severity", "blocking", "title", "detail") {
		t.Errorf("findings.items.required = %#v", items["required"])
	}
	assertModelNoForbiddenStrictKeyword(t, schema)
}

func modelJSONStringsContain(value any, wants ...string) bool {
	got := map[string]bool{}
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if text, ok := value.(string); ok {
			got[text] = true
		}
	}
	for _, want := range wants {
		if !got[want] {
			return false
		}
	}
	return true
}

func assertModelNoForbiddenStrictKeyword(t *testing.T, value any) {
	t.Helper()
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case "allOf", "if", "then", "else", "not":
				t.Errorf("envelope contains forbidden keyword %q", key)
			}
			assertModelNoForbiddenStrictKeyword(t, child)
		}
	case []any:
		for _, child := range node {
			assertModelNoForbiddenStrictKeyword(t, child)
		}
	}
}
