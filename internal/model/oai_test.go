package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
text, cost, _, _, err := o.Verify(context.Background(), "be strict", "verify this diff")
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
_, _, _, _, err := o.Verify(context.Background(), "be strict", "verify this diff")
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
_, cost, _, _, err := o.Verify(context.Background(), "be strict", "verify this diff")
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
_, _, _, _, err := o.Verify(context.Background(), "be strict", "verify this diff")
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
			name: "groq provider with key, no base URL — uses preset",
			env: map[string]string{
				"SWORN_GROQ_API_KEY": "sk-test",
			},
			modelID: "groq/llama-3.3-70b",
			wantErr: false,
		},
		{
			name: "groq provider with key and base URL override",
			env: map[string]string{
				"SWORN_GROQ_API_KEY":  "sk-test",
				"SWORN_GROQ_BASE_URL": "https://custom-groq.example.com/v1",
			},
			modelID: "groq/llama-3.3-70b",
			wantErr: false,
		}, {
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
				"SWORN_GROQ_API_KEY", "SWORN_GROQ_BASE_URL", "SWORN_GROQ_MODEL",
				"SWORN_DIRECT", "SWORN_PROXY_URL",
			} {
				t.Setenv(k, "")
			}
			// Point config to an empty dir so no real credentials interfere.
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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

// --- S06b proxy routing tests ---

// writeTestCreds writes a credentials file into a temp config dir and sets
// XDG_CONFIG_HOME so configDir() resolves to it. configDir() appends "/sworn"
// to the XDG base, so the file lands at <dir>/sworn/credentials.json.
func writeTestCreds(t *testing.T, dir string) {
	t.Helper()
	swornDir := filepath.Join(dir, "sworn")
	if err := os.MkdirAll(swornDir, 0700); err != nil {
		t.Fatalf("mkdir %s: %v", swornDir, err)
	}
	credsJSON := `{"token":"tok_proxy","email":"user@example.com","tier":"pro","expires_at":"2030-01-01T00:00:00Z"}`
	credsPath := filepath.Join(swornDir, "credentials.json")
	if err := os.WriteFile(credsPath, []byte(credsJSON), 0600); err != nil {
		t.Fatalf("writing credentials: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// TestFromEnvUsesProxy verifies that when sworn credentials are present,
// FromEnv routes through the proxy URL (not the direct provider URL).
func TestFromEnvUsesProxy(t *testing.T) {
	// Set up a mock proxy server that records requests.
	var proxyHit bool
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHit = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(oaiResp([]struct{ content string }{{"PASS"}}, &UsageBlock{
			PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
		}))
	}))
	defer proxySrv.Close()

	// Set SWORN_PROXY_URL to the mock proxy.
	t.Setenv("SWORN_PROXY_URL", proxySrv.URL)

	// Write credentials file into a temp config dir.
	writeTestCreds(t, t.TempDir())

	// Clear direct provider key to ensure we're using proxy, not direct.
	t.Setenv("SWORN_OPENAI_API_KEY", "")
	t.Setenv("SWORN_DIRECT", "")

	v, err := FromEnv("openai/gpt-4.1")
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}

	// Dispatch a request through the verifier to confirm it hits the proxy.
_, _, _, _, err = v.Verify(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !proxyHit {
		t.Error("expected request to hit the proxy server, but it did not")
	}
}

// TestFromEnvBypassProxy verifies that SWORN_DIRECT=1 bypasses the proxy
// and sends requests to the provider URL even when credentials are present.
func TestFromEnvBypassProxy(t *testing.T) {
	var proxyHit, providerHit bool

	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHit = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(oaiResp([]struct{ content string }{{"PASS"}}, nil))
	}))
	defer proxySrv.Close()

	providerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerHit = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(oaiResp([]struct{ content string }{{"PASS"}}, nil))
	}))
	defer providerSrv.Close()

	t.Setenv("SWORN_PROXY_URL", proxySrv.URL)

	// Write credentials file into a temp config dir.
	writeTestCreds(t, t.TempDir())

	// Set SWORN_DIRECT=1 and provider key + base URL.
	t.Setenv("SWORN_DIRECT", "1")
	t.Setenv("SWORN_OPENAI_API_KEY", "sk-direct")
	t.Setenv("SWORN_OPENAI_BASE_URL", providerSrv.URL)
	v, err := FromEnv("openai/gpt-4.1")
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}

_, _, _, _, err = v.Verify(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if proxyHit {
		t.Error("proxy should NOT be hit when SWORN_DIRECT=1")
	}
	if !providerHit {
		t.Error("provider should be hit when SWORN_DIRECT=1")
	}
}

// TestFromEnvProxyDefaultHost (pin B) verifies that when SWORN_PROXY_URL is
// unset, the bearer token is sent only to the compiled-in default host.
func TestFromEnvProxyDefaultHost(t *testing.T) {
	// Ensure SWORN_PROXY_URL is unset.
	t.Setenv("SWORN_PROXY_URL", "")

	// Write credentials file into a temp config dir.
	writeTestCreds(t, t.TempDir())

	t.Setenv("SWORN_OPENAI_API_KEY", "")
	t.Setenv("SWORN_DIRECT", "")
	v, err := FromEnv("openai/gpt-4.1")
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}

	oai, ok := v.(*OAI)
	if !ok {
		t.Fatalf("expected *OAI, got %T", v)
	}

	// The base URL should start with the compiled-in default host.
	if !strings.HasPrefix(oai.BaseURL, "https://api.swornagent.com") {
		t.Errorf("expected base URL to use compiled-in default host, got %q", oai.BaseURL)
	}
	// The API key should be the sworn token, not a provider key.
	if oai.APIKey != "tok_proxy" {
		t.Errorf("expected API key to be the sworn token, got %q", oai.APIKey)
	}
}

// TestFromEnvProxyOverrideWarns (pin B) verifies that when SWORN_PROXY_URL
// is set, a stderr warning is emitted about non-default credential routing.
func TestFromEnvProxyOverrideWarns(t *testing.T) {
	// Capture stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	t.Setenv("SWORN_PROXY_URL", "http://localhost:12345")

	// Write credentials file into a temp config dir.
	writeTestCreds(t, t.TempDir())

	t.Setenv("SWORN_OPENAI_API_KEY", "")
	t.Setenv("SWORN_DIRECT", "")
	v, err := FromEnv("openai/gpt-4.1")
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}

	// Restore stderr and read captured output.
	w.Close()
	os.Stderr = oldStderr
	captured, _ := io.ReadAll(r)

	oai, ok := v.(*OAI)
	if !ok {
		t.Fatalf("expected *OAI, got %T", v)
	}

	// The base URL should use the override host.
	if !strings.HasPrefix(oai.BaseURL, "http://localhost:12345") {
		t.Errorf("expected base URL to use override host, got %q", oai.BaseURL)
	}

	// A stderr warning should have been emitted.
	warning := string(captured)
	if !strings.Contains(warning, "SWORN_PROXY_URL") || !strings.Contains(warning, "warning") {
		t.Errorf("expected stderr warning about SWORN_PROXY_URL, got %q", warning)
	}
}

// TestFromEnvInsufficientCredits (pin C) verifies that when the proxy
// returns 402, the client returns ErrInsufficientCredits and does not
// fall back to a direct provider call.
func TestFromEnvInsufficientCredits(t *testing.T) {
	var providerHit bool

	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error":"insufficient credits"}`))
	}))
	defer proxySrv.Close()

	providerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerHit = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(oaiResp([]struct{ content string }{{"PASS"}}, nil))
	}))
	defer providerSrv.Close()

	t.Setenv("SWORN_PROXY_URL", proxySrv.URL)

	// Write credentials file into a temp config dir.
	writeTestCreds(t, t.TempDir())

	// Set provider key + URL so a fallback *could* happen if the code were buggy.
	t.Setenv("SWORN_OPENAI_API_KEY", "sk-direct")
	t.Setenv("SWORN_OPENAI_BASE_URL", providerSrv.URL)
	t.Setenv("SWORN_DIRECT", "")
	v, err := FromEnv("openai/gpt-4.1")
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}

_, _, _, _, err = v.Verify(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error for 402, got nil")
	}

	// Error message should point to `sworn account buy`.
	if !strings.Contains(err.Error(), "sworn account buy") {
		t.Errorf("expected error to mention 'sworn account buy', got %q", err.Error())
	}

	// Provider should NOT have been hit (no silent fallback).
	if providerHit {
		t.Error("provider should NOT be hit on 402 — no silent fallback")
	}
}

// TestFromEnvNoCredsUnchanged verifies that with no credentials file,
// FromEnv behaviour is unchanged from before this slice (direct to provider
// or error if no API key).
func TestFromEnvNoCredsUnchanged(t *testing.T) {
	// Point config to an empty dir (no credentials file).
	credsDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", credsDir)

	t.Setenv("SWORN_PROXY_URL", "")
	t.Setenv("SWORN_DIRECT", "")
	t.Setenv("SWORN_OPENAI_API_KEY", "sk-test")

	v, err := FromEnv("openai/gpt-4.1")
	if err != nil {
		t.Fatalf("FromEnv failed: %v", err)
	}

	oai, ok := v.(*OAI)
	if !ok {
		t.Fatalf("expected *OAI, got %T", v)
	}

	// Should use the direct provider URL, not the proxy.
	if !strings.HasPrefix(oai.BaseURL, "https://api.openai.com") {
		t.Errorf("expected direct provider URL, got %q", oai.BaseURL)
	}
	// API key should be the provider key, not a sworn token.
	if oai.APIKey != "sk-test" {
		t.Errorf("expected provider API key, got %q", oai.APIKey)
	}
}
