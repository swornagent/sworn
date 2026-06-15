package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func fakeServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// oaiResp builds a ChatResponse JSON blob for test handlers.
// finishReason defaults to "stop" when choices are present.
func oaiResp(choices []struct {
	content string
}, usage *UsageBlock) []byte {
	cr := ChatResponse{}
	finish := "stop"
	for _, c := range choices {
		cr.Choices = append(cr.Choices, struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{Message: struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		}{Content: c.content}, FinishReason: finish})
	}
	cr.Usage = usage
	b, _ := json.Marshal(cr)
	return b
}

func TestOAI_Verify(t *testing.T) {
	tests := []struct {
		name        string
		handler     func(w http.ResponseWriter, r *http.Request)
		client      *http.Client
		wantErr     bool
		wantText    string
		wantCostGt0 bool
	}{
		{
			name: "PASS",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(oaiResp([]struct{ content string }{{"PASS - all checks pass"}}, &UsageBlock{
					PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
				}))
			},
			wantErr:     false,
			wantText:    "PASS - all checks pass",
			wantCostGt0: true,
		},
		{
			name: "FAIL",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(oaiResp([]struct{ content string }{{"FAIL: missing proof bundle"}}, &UsageBlock{
					PromptTokens: 80, CompletionTokens: 30, TotalTokens: 110,
				}))
			},
			wantErr:     false,
			wantText:    "FAIL: missing proof bundle",
			wantCostGt0: true,
		},
		{
			name: "HTTP 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "internal"}`))
			},
			wantErr: true,
		},
		{
			name: "timeout",
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(200 * time.Millisecond)
			},
			client:  &http.Client{Timeout: 50 * time.Millisecond},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeServer(t, tt.handler)
			o := &OAI{
				BaseURL: srv.URL,
				Model:   "gpt-4.1-mini",
				APIKey:  "sk-test",
				Client:  tt.client,
			}
			text, cost, err := o.Verify(context.Background(), "be strict", "verify this diff")
			if tt.wantErr && err == nil {
				t.Fatal("want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantText != "" && text != tt.wantText {
				t.Fatalf("want %q, got %q", tt.wantText, text)
			}
			if tt.wantCostGt0 && cost <= 0 {
				t.Fatalf("want cost > 0, got %f", cost)
			}
		})
	}
}
func TestOAI_Verify_GarbledJSON(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not json`))
	})
	o := &OAI{BaseURL: srv.URL, Model: "gpt-4.1-mini", APIKey: "sk-test"}
	_, _, err := o.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want unmarshal error, got nil")
	}
}

func TestOAI_Verify_MissingUsageBlock(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// usage block omitted — should get zero cost, not an error
		w.Write(oaiResp([]struct{ content string }{{"PASS"}}, nil))
	})
	o := &OAI{BaseURL: srv.URL, Model: "gpt-4.1-mini", APIKey: "sk-test"}
	_, cost, err := o.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cost != 0 {
		t.Fatalf("want zero cost, got %f", cost)
	}
}

func TestOAI_Verify_EmptyChoices(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(oaiResp(nil, &UsageBlock{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}))
	})
	o := &OAI{BaseURL: srv.URL, Model: "gpt-4.1-mini", APIKey: "sk-test"}
	_, _, err := o.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error on empty choices, got nil")
	}
}

func TestComputeCost(t *testing.T) {
	tests := []struct {
		name  string
		model string
		usage *UsageBlock
		want  float64
	}{
		{
			name:  "nil usage",
			model: "gpt-4.1-mini",
			usage: nil,
			want:  0,
		},
		{
			name:  "unknown model",
			model: "unknown-model",
			usage: &UsageBlock{PromptTokens: 1000, CompletionTokens: 500},
			want:  0,
		},
		{
			name:  "gpt-4.1-mini exact",
			model: "gpt-4.1-mini",
			usage: &UsageBlock{PromptTokens: 1_000_000, CompletionTokens: 1_000_000},
			want:  0.30 + 0.80,
		}, {
			name:  "gpt-4.1 exact",
			model: "gpt-4.1",
			usage: &UsageBlock{PromptTokens: 500_000, CompletionTokens: 250_000},
			want:  1.00 + 2.00,
		},
		{
			name:  "gpt-4o exact",
			model: "gpt-4o",
			usage: &UsageBlock{PromptTokens: 1_000_000, CompletionTokens: 1_000_000},
			want:  2.50 + 10.00,
		},
		{
			name:  "o3 exact",
			model: "o3",
			usage: &UsageBlock{PromptTokens: 1_000_000, CompletionTokens: 1_000_000},
			want:  10.00 + 40.00,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeCost(tt.model, tt.usage)
			// tolerate float rounding
			if got < tt.want-0.01 || got > tt.want+0.01 {
				t.Fatalf("want ~%f, got %f", tt.want, got)
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		modelID string
		wantErr bool
	}{
		{
			name:    "empty model ID",
			modelID: "",
			wantErr: true,
		},
		{
			name:    "no slash",
			modelID: "gpt-4.1",
			wantErr: true,
		},
		{
			name:    "empty provider",
			modelID: "/gpt-4.1",
			wantErr: true,
		},
		{
			name:    "empty model",
			modelID: "openai/",
			wantErr: true,
		},
		{
			name:    "missing key",
			modelID: "openai/gpt-4.1",
			wantErr: true,
		},
		{
			name: "openai with key, no base URL → uses default",
			env: map[string]string{
				"SWORN_OPENAI_API_KEY": "sk-test",
			},
			modelID: "openai/gpt-4.1",
			wantErr: false,
		},
		{
			name: "custom provider with key but no base URL",
			env: map[string]string{
				"SWORN_AZURE_API_KEY": "sk-test",
			},
			modelID: "azure/gpt-4",
			wantErr: true,
		},
		{
			name: "custom provider with key and base URL",
			env: map[string]string{
				"SWORN_AZURE_API_KEY":  "sk-test",
				"SWORN_AZURE_BASE_URL": "https://example.openai.azure.com",
			},
			modelID: "azure/gpt-4",
			wantErr: false,
		},
		{
			name: "env model override",
			env: map[string]string{
				"SWORN_OPENAI_API_KEY": "sk-test",
				"SWORN_OPENAI_MODEL":   "gpt-4.1-nano",
			},
			modelID: "openai/gpt-4.1", // flag says gpt-4.1 but env overrides
			wantErr: false,
		},
		{
			name: "invalid base URL",
			env: map[string]string{
				"SWORN_OPENAI_API_KEY":  "sk-test",
				"SWORN_OPENAI_BASE_URL": "://bad",
			},
			modelID: "openai/gpt-4.1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear relevant env vars first
			for _, k := range []string{
				"SWORN_OPENAI_API_KEY", "SWORN_OPENAI_BASE_URL", "SWORN_OPENAI_MODEL",
				"SWORN_AZURE_API_KEY", "SWORN_AZURE_BASE_URL", "SWORN_AZURE_MODEL",
			} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			v, err := FromEnv(tt.modelID)
			if tt.wantErr && err == nil {
				t.Fatal("want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantErr {
				if _, ok := v.(*OAI); !ok {
					t.Fatalf("want *OAI, got %T", v)
				}
			}
		})
	}
}
