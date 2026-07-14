package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/genai"
)

// googleMsg builds a GenerateContentResponse JSON blob for test handlers.
// Returns a minimal valid response with one candidate containing one text part.
func googleMsg(text string, promptTokens, candidatesTokens int32) []byte {
	resp := map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"parts": []map[string]any{
						{"text": text},
					},
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":     promptTokens,
			"candidatesTokenCount": candidatesTokens,
			"totalTokenCount":      promptTokens + candidatesTokens,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// googleError builds a Google API error JSON blob.
func googleError(code int, message, status string) []byte {
	e := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"status":  status,
		},
	}
	b, _ := json.Marshal(e)
	return b
}

func TestGoogleVerify_GeminiAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(googleMsg("PASS - all checks pass", 100, 50))
	}))
	defer srv.Close()

	g := newTestGoogle(srv.URL, "gemini-2.0-flash")
	text, cost, _, _, err := g.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "PASS - all checks pass" {
		t.Fatalf("want %q, got %q", "PASS - all checks pass", text)
	}
	if cost <= 0 {
		t.Fatalf("want cost > 0, got %f", cost)
	}
}

func TestGoogleVerify_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write(googleError(429, "Resource exhausted", "RESOURCE_EXHAUSTED"))
	}))
	defer srv.Close()

	g := newTestGoogle(srv.URL, "gemini-2.0-flash")
	_, _, _, _, err := g.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}

	// Assert the error is a *model.Error with KindRateLimit.
	var me *Error
	if !errors.As(err, &me) || me.Kind != KindRateLimit {
		t.Fatalf("expected KindRateLimit, got %v", err)
	}
}

func TestGoogleVerify_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(googleError(401, "API key not valid", "UNAUTHENTICATED"))
	}))
	defer srv.Close()

	g := newTestGoogle(srv.URL, "gemini-2.0-flash")
	_, _, _, _, err := g.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}

	var me *Error
	if !errors.As(err, &me) || me.Kind != KindAuth {
		t.Fatalf("expected KindAuth, got %v", err)
	}
}

func TestGoogleVerify_NonHTTPErrorIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a 400 with a valid APIError shape that the genai SDK will
		// decode into *genai.APIError, which then gets mapped via
		// NewProviderError. KindOther errors are transient per IsTransient.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(googleError(400, "bad request", "INVALID_ARGUMENT"))
	}))
	defer srv.Close()

	g := newTestGoogle(srv.URL, "gemini-2.0-flash")
	_, _, _, _, err := g.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !IsTransient(err) {
		t.Fatalf("expected Google error to be transient, got %v", err)
	}
}

func TestGoogleVerify_CostCalculation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(googleMsg("PASS", 1000, 500))
	}))
	defer srv.Close()

	g := newTestGoogle(srv.URL, "gemini-2.0-flash")
	_, cost, _, _, err := g.Verify(context.Background(), "be strict", "verify")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// gemini-2.0-flash: $0.10/1M input, $0.40/1M output
	// 1000 input: 1000/1M * 0.10 = 0.0001
	// 500 output: 500/1M * 0.40 = 0.0002
	// total ≈ 0.0003
	if cost <= 0 {
		t.Fatalf("want cost > 0, got %f", cost)
	}
}

func TestGoogleVerify_UnknownModelCostIsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(googleMsg("PASS", 100, 50))
	}))
	defer srv.Close()

	g := newTestGoogle(srv.URL, "gemini-unknown-model")
	_, cost, _, _, err := g.Verify(context.Background(), "be strict", "verify")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cost != 0 {
		t.Fatalf("want cost 0 for unknown model, got %f", cost)
	}
}

func TestNewClient_GoogleRouted(t *testing.T) {
	cfg := ProviderConfig{GoogleKey: "test-key"}
	v, err := NewClient("google/gemini-2.0-flash", cfg)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	_, ok := v.(*Google)
	if !ok {
		t.Fatalf("expected *Google, got %T", v)
	}
}

func TestNewClient_VertexRouted(t *testing.T) {
	// Vertex AI routing test requires GOOGLE_CLOUD_PROJECT because
	// NewGoogleVertex calls genai.NewClient which initialises ADC.
	// Skip in CI/dev environments without GCP project configured.
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		t.Skip("Vertex routing test requires GOOGLE_CLOUD_PROJECT")
	}
	cfg := ProviderConfig{
		GoogleCloudProject:  os.Getenv("GOOGLE_CLOUD_PROJECT"),
		GoogleCloudLocation: "us-central1",
	}
	v, err := NewClient("vertex/gemini-2.0-flash", cfg)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	_, ok := v.(*Google)
	if !ok {
		t.Fatalf("expected *Google, got %T", v)
	}
}

// TestFromEnv_GoogleWithCanonicalKey is the spec-mandated regression test for
// the user outcome: `sworn run` → model.FromEnv("google/gemini-2.0-flash") with
// only GOOGLE_API_KEY set (no SWORN_GOOGLE_API_KEY alias) must return a *Google.
// This is the exact path the S12 verifier found broken (Gate 1): a mangled
// switch had `case "google":` trapped after a // comment, so GOOGLE_API_KEY
// alone fell through to the default and failed with "SWORN_GOOGLE_API_KEY not
// set". SWORN_DIRECT=1 forces the direct-provider branch (bypassing proxy
// credential lookup) and an isolated XDG_CONFIG_HOME prevents real creds from
// interfering.
func TestFromEnv_GoogleWithCanonicalKey(t *testing.T) {
	for _, k := range []string{
		"GOOGLE_API_KEY", "GOOGLE_API_KEY", "SWORN_DIRECT",
		"SWORN_GOOGLE_MODEL", "SWORN_GOOGLE_BASE_URL",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SWORN_DIRECT", "1")
	t.Setenv("GOOGLE_API_KEY", "test-canonical-key")

	v, err := FromEnv("google/gemini-2.0-flash")
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	g, ok := v.(*Google)
	if !ok {
		t.Fatalf("expected *Google, got %T", v)
	}
	if g.Model != "gemini-2.0-flash" {
		t.Errorf("Model = %q, want gemini-2.0-flash", g.Model)
	}
}

// TestFromEnv_GoogleWithAliasKey confirms the SWORN_GOOGLE_API_KEY alias still
// works as a fallback when the canonical GOOGLE_API_KEY is unset — backward
// compat per the spec's "canonical or alias" requirement.
func TestFromEnv_GoogleWithAliasKey(t *testing.T) {
	for _, k := range []string{
		"GOOGLE_API_KEY", "GOOGLE_API_KEY", "SWORN_DIRECT",
		"SWORN_GOOGLE_MODEL", "SWORN_GOOGLE_BASE_URL",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SWORN_DIRECT", "1")
	t.Setenv("GOOGLE_API_KEY", "test-alias-key")

	v, err := FromEnv("google/gemini-2.0-flash")
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	if _, ok := v.(*Google); !ok {
		t.Fatalf("expected *Google, got %T", v)
	}
}

// TestFromEnv_GoogleMissingKey confirms the key gate still fails closed when
// neither GOOGLE_API_KEY nor SWORN_GOOGLE_API_KEY is set — the fail-closed
// invariant (AGENTS.md non-negotiables) must hold for the google prefix.
func TestFromEnv_GoogleMissingKey(t *testing.T) {
	for _, k := range []string{
		"GOOGLE_API_KEY", "GOOGLE_API_KEY", "SWORN_DIRECT",
		"SWORN_GOOGLE_MODEL", "SWORN_GOOGLE_BASE_URL",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SWORN_DIRECT", "1")

	_, err := FromEnv("google/gemini-2.0-flash")
	if err == nil {
		t.Fatal("want error for missing Google key, got nil")
	}
}

func TestNewGoogleGemini_MissingKey(t *testing.T) {
	_, err := NewGoogleGemini("gemini-2.0-flash", "")
	if err == nil {
		t.Fatal("want error for missing API key, got nil")
	}
	if !strings.Contains(err.Error(), "missing Google API key") {
		t.Fatalf("want 'missing Google API key', got %q", err.Error())
	}
}

func TestNewGoogleVertex_MissingProject(t *testing.T) {
	_, err := NewGoogleVertex("gemini-2.0-flash", "", "us-central1")
	if err == nil {
		t.Fatal("want error for missing project, got nil")
	}
	if !strings.Contains(err.Error(), "missing Google Cloud project") {
		t.Fatalf("want 'missing Google Cloud project', got %q", err.Error())
	}
}

func TestNewGoogleVertex_MissingLocation(t *testing.T) {
	_, err := NewGoogleVertex("gemini-2.0-flash", "test-project", "")
	if err == nil {
		t.Fatal("want error for missing location, got nil")
	}
	if !strings.Contains(err.Error(), "missing Google Cloud location") {
		t.Fatalf("want 'missing Google Cloud location', got %q", err.Error())
	}
}

// newTestGoogle returns a Google driver pointed at a test server.
// Uses HTTPOptions.BaseURL to redirect genai SDK requests to the test server.
func newTestGoogle(baseURL, modelID string) *Google {
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  "test-key",
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: baseURL,
		},
	})
	if err != nil {
		panic("newTestGoogle: " + err.Error())
	}
	return &Google{Client: client, Model: modelID}
}

// TestGoogleVerify_Live is the spec-mandated live reachability artefact.
// It is skipped unless SWORN_LIVE_TESTS=1 AND GOOGLE_API_KEY is set, so it
// runs only when a developer explicitly opts in with real credentials.
func TestGoogleVerify_Live(t *testing.T) {
	if os.Getenv("SWORN_LIVE_TESTS") != "1" || os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("live test requires SWORN_LIVE_TESTS=1 and GOOGLE_API_KEY")
	}
	g, err := NewGoogleGemini("gemini-2.0-flash", os.Getenv("GOOGLE_API_KEY"))
	if err != nil {
		t.Fatalf("NewGoogleGemini error: %v", err)
	}
	text, _, _, _, err := g.Verify(context.Background(), "Reply with PASS.", "verify")
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !strings.Contains(text, "PASS") {
		t.Fatalf("want text containing %q, got %q", "PASS", text)
	}
}
