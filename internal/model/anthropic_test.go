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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicMsg builds a Message JSON blob for test handlers.
// Returns a minimal valid Messages API response with one text block.
func anthropicMsg(text string, inputTokens, outputTokens int64) []byte {
	msg := map[string]any{
		"id":   "msg_test001",
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"model":         "claude-sonnet-4-6-20250514",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":                inputTokens,
			"output_tokens":               outputTokens,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens":     0,
		},
	}
	b, _ := json.Marshal(msg)
	return b
}

// anthropicError builds an Anthropic API error JSON blob.
func anthropicError(errType, message string) []byte {
	e := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	}
	b, _ := json.Marshal(e)
	return b
}

func TestAnthropicVerify_ReturnsTextBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicMsg("PASS - all checks pass", 100, 50))
	}))
	defer srv.Close()

	a := newTestAnthropic(srv.URL, "claude-sonnet-4-6")
	text, cost, err := a.Verify(context.Background(), "be strict", "verify this diff")
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

func TestAnthropicVerify_MultiBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Two content blocks — first text block is the verdict.
		msg := map[string]any{
			"id":   "msg_test002",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "PASS"},
				{"type": "text", "text": "extra analysis"},
			},
			"model":       "claude-sonnet-4-6-20250514",
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  80,
				"output_tokens": 30,
			},
		}
		json.NewEncoder(w).Encode(msg)
	}))
	defer srv.Close()

	a := newTestAnthropic(srv.URL, "claude-sonnet-4-6")
	text, _, err := a.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "PASS" {
		t.Fatalf("want first text block %q, got %q", "PASS", text)
	}
}

func TestAnthropicVerify_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write(anthropicError("rate_limit_error", "Too many requests"))
	}))
	defer srv.Close()

	a := newTestAnthropic(srv.URL, "claude-sonnet-4-6")
	_, _, err := a.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}

	// Pin 4: assert the error is a *model.Error with KindRateLimit so the
	// taxonomy bridge (Pin 3) is confirmed live.
	var me *Error
	if !errors.As(err, &me) || me.Kind != KindRateLimit {
		t.Fatalf("expected KindRateLimit, got %v", err)
	}
}

func TestAnthropicNewClient_RoutedCorrectly(t *testing.T) {
	// Set an arbitrary key so NewClient can construct the driver.
	cfg := ProviderConfig{AnthropicKey: "sk-ant-test"}
	v, err := NewClient("anthropic/claude-opus-4-8", cfg)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	_, ok := v.(*Anthropic)
	if !ok {
		t.Fatalf("expected *Anthropic, got %T", v)
	}
}

// TestAnthropicVerify_NonHTTPErrorIsTransient confirms the fallback path in
// Verify() for non-HTTP errors (DNS failure, TLS handshake, etc.) returns an
// error that S44's retry policy treats as transient. This is the Pin 2
// error-taxonomy gap covered by IsTransient's "unknown errors are assumed
// transient" contract.
func TestAnthropicVerify_NonHTTPErrorIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force a non-HTTP error by returning JSON that the SDK cannot decode
		// into the expected Message shape, producing an unclassified error that
		// bypasses anthropicStatusCode's `": ` heuristic.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`))
	}))
	defer srv.Close()

	a := newTestAnthropic(srv.URL, "claude-sonnet-4-6")
	_, _, err := a.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !IsTransient(err) {
		t.Fatalf("expected non-HTTP Anthropic error to be transient, got %v", err)
	}
}

// newTestAnthropic returns an Anthropic driver pointed at a test server.
// Uses option.WithHTTPClient and option.WithBaseURL to avoid hitting the
// real API.
func newTestAnthropic(baseURL, modelID string) *Anthropic {
	client := anthropic.NewClient(
		option.WithAPIKey("sk-ant-test"),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(http.DefaultClient),
	)
	return &Anthropic{
		Client:    &client,
		Model:     modelID,
		MaxTokens: 8192,
	}
}

// TestAnthropicVerify_Live is the spec-mandated live reachability artefact.
// It is skipped unless SWORN_LIVE_TESTS=1 AND ANTHROPIC_API_KEY is set, so it
// runs only when a developer explicitly opts in with real credentials.
func TestAnthropicVerify_Live(t *testing.T) {
	if os.Getenv("SWORN_LIVE_TESTS") != "1" || os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("live test requires SWORN_LIVE_TESTS=1 and ANTHROPIC_API_KEY")
	}
	a, err := NewAnthropic("claude-sonnet-4-6", os.Getenv("ANTHROPIC_API_KEY"))
	if err != nil {
		t.Fatalf("NewAnthropic error: %v", err)
	}
	text, _, err := a.Verify(context.Background(), "Reply with PASS.", "verify")
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !strings.Contains(text, "PASS") {
		t.Fatalf("want text containing %q, got %q", "PASS", text)
	}
}
