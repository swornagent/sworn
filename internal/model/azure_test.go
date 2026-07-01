package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// azureChatResponse builds a valid /chat/completions JSON blob for test
// handlers. Returns a response with one choice containing the given content.
func azureChatResponse(content string) []byte {
	resp := map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     100,
			"completion_tokens": 50,
			"total_tokens":      150,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestAzureVerify_CorrectURL(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write(azureChatResponse("PASS"))
	}))
	defer srv.Close()

	// Extract host from test server URL to use as the Azure endpoint.
	endpoint := srv.URL

	a, err := NewAzureOAI("gpt-4o", endpoint, "test-key", "2024-12-01-preview")
	if err != nil {
		t.Fatalf("NewAzureOAI: %v", err)
	}
	a.Client = srv.Client()

	_, _, _, _, err = a.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// Assert URL matches the Azure pattern:
	// /openai/deployments/gpt-4o/chat/completions?api-version=2024-12-01-preview
	if !strings.Contains(gotURL, "/openai/deployments/gpt-4o/chat/completions") {
		t.Fatalf("URL missing Azure path: got %q, want /openai/deployments/gpt-4o/chat/completions", gotURL)
	}
	if !strings.Contains(gotURL, "api-version=2024-12-01-preview") {
		t.Fatalf("URL missing api-version: got %q", gotURL)
	}
}

func TestAzureVerify_APIKeyHeader(t *testing.T) {
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("api-key")
		w.Header().Set("Content-Type", "application/json")
		w.Write(azureChatResponse("PASS"))
	}))
	defer srv.Close()

	endpoint := srv.URL

	a, err := NewAzureOAI("gpt-4o", endpoint, "test-api-key-123", "2024-12-01-preview")
	if err != nil {
		t.Fatalf("NewAzureOAI: %v", err)
	}
	a.Client = srv.Client()

	_, _, _, _, err = a.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if gotAPIKey != "test-api-key-123" {
		t.Fatalf("api-key header: got %q, want %q", gotAPIKey, "test-api-key-123")
	}
}

func TestAzureVerify_AuthorizationHeaderAbsent(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write(azureChatResponse("PASS"))
	}))
	defer srv.Close()

	endpoint := srv.URL

	a, err := NewAzureOAI("gpt-4o", endpoint, "test-key", "2024-12-01-preview")
	if err != nil {
		t.Fatalf("NewAzureOAI: %v", err)
	}
	a.Client = srv.Client()

	_, _, _, _, err = a.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if gotAuth != "" {
		t.Fatalf("Authorization header present: got %q, want empty (Azure uses api-key, not Bearer)", gotAuth)
	}
}

func TestAzureVerify_DefaultAPIVersion(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write(azureChatResponse("PASS"))
	}))
	defer srv.Close()

	endpoint := srv.URL

	// Pass empty apiVersion — should default to "2024-12-01-preview".
	a, err := NewAzureOAI("gpt-4o", endpoint, "test-key", "")
	if err != nil {
		t.Fatalf("NewAzureOAI: %v", err)
	}
	a.Client = srv.Client()

	_, _, _, _, err = a.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !strings.Contains(gotURL, "api-version=2024-12-01-preview") {
		t.Fatalf("URL missing default api-version: got %q, want api-version=2024-12-01-preview", gotURL)
	}
	if a.APIVersion != "2024-12-01-preview" {
		t.Fatalf("APIVersion field: got %q, want %q", a.APIVersion, "2024-12-01-preview")
	}
}

func TestAzureVerify_ReturnsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(azureChatResponse("PASS - all checks pass"))
	}))
	defer srv.Close()

	endpoint := srv.URL

	a, err := NewAzureOAI("gpt-4o", endpoint, "test-key", "2024-12-01-preview")
	if err != nil {
		t.Fatalf("NewAzureOAI: %v", err)
	}
	a.Client = srv.Client()

	text, cost, _, _, err := a.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if text != "PASS - all checks pass" {
		t.Fatalf("text: got %q, want %q", text, "PASS - all checks pass")
	}
	// Azure cost is not modelled — always 0.
	if cost != 0 {
		t.Fatalf("cost: got %f, want 0 (Azure cost not modelled)", cost)
	}
}

func TestNewClient_AzureRouted(t *testing.T) {
	pcfg := ProviderConfig{
		AzureAPIKey:     "test-key",
		AzureEndpoint:   "myendpoint.openai.azure.com",
		AzureAPIVersion: "2024-12-01-preview",
	}

	v, err := NewClient("azure/gpt-4o", pcfg)
	if err != nil {
		t.Fatalf("NewClient(azure/gpt-4o): %v", err)
	}
	if v == nil {
		t.Fatal("NewClient returned nil Verifier")
	}

	az, ok := v.(*AzureOAI)
	if !ok {
		t.Fatalf("NewClient returned %T, want *AzureOAI", v)
	}

	if az.Deployment != "gpt-4o" {
		t.Fatalf("Deployment: got %q, want %q", az.Deployment, "gpt-4o")
	}
	if az.APIKey != "test-key" {
		t.Fatalf("APIKey: got %q, want %q", az.APIKey, "test-key")
	}
	if az.APIVersion != "2024-12-01-preview" {
		t.Fatalf("APIVersion: got %q, want %q", az.APIVersion, "2024-12-01-preview")
	}
	// Endpoint should have https:// prepended.
	if !strings.HasPrefix(az.Endpoint, "https://") {
		t.Fatalf("Endpoint: got %q, want https:// prefix", az.Endpoint)
	}
}

func TestNewAzureOAI_Errors(t *testing.T) {
	tests := []struct {
		name       string
		deployment string
		endpoint   string
		apiKey     string
	}{
		{"empty deployment", "", "endpoint", "key"},
		{"empty apiKey", "gpt-4o", "endpoint", ""},
		{"empty endpoint", "gpt-4o", "", "key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAzureOAI(tt.deployment, tt.endpoint, tt.apiKey, "")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestAzureVerify_EndpointNormalisation(t *testing.T) {
	// Trailing slash stripped, https:// prepended.
	a, err := NewAzureOAI("gpt-4o", "myendpoint.openai.azure.com/", "key", "2024-12-01-preview")
	if err != nil {
		t.Fatalf("NewAzureOAI: %v", err)
	}
	if a.Endpoint != "https://myendpoint.openai.azure.com" {
		t.Fatalf("Endpoint normalisation failed: got %q, want %q", a.Endpoint, "https://myendpoint.openai.azure.com")
	}
}

func TestAzureVerify_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
	}))
	defer srv.Close()

	endpoint := srv.URL

	a, err := NewAzureOAI("gpt-4o", endpoint, "test-key", "2024-12-01-preview")
	if err != nil {
		t.Fatalf("NewAzureOAI: %v", err)
	}
	a.Client = srv.Client()

	_, _, _, _, err = a.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}

	me, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type: got %T, want *model.Error", err)
	}
	if me.Provider != "azure" {
		t.Fatalf("Provider: got %q, want %q", me.Provider, "azure")
	}
	if me.Kind != KindRateLimit {
		t.Fatalf("Kind: got %v, want KindRateLimit", me.Kind)
	}
}
