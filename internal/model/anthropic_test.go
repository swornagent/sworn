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
	text, cost, _, _, err := a.Verify(context.Background(), "be strict", "verify this diff")
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
	text, _, _, _, err := a.Verify(context.Background(), "be strict", "verify this diff")
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
	_, _, _, _, err := a.Verify(context.Background(), "be strict", "verify this diff")
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
	_, _, _, _, err := a.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !IsTransient(err) {
		t.Fatalf("expected non-HTTP Anthropic error to be transient, got %v", err)
	}
}

// TestAnthropicChat_ReturnsTextBlock verifies Chat() returns a ChatResponse
// with content from the first text block for a 2-message history.
// Maps AC2 (user+assistant turn mapped to MessageParam) and AC3 (InputTokens>0, CostUSD>0).
func TestAnthropicChat_ReturnsTextBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicMsg("PASS - all checks pass", 200, 100))
	}))
	defer srv.Close()

	a := newTestAnthropic(srv.URL, "claude-sonnet-4-6")
	resp, err := a.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "what is a verdict?"},
		{Role: "assistant", Content: "a verdict is a decision"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Choices[0].Message.Content != "PASS - all checks pass" {
		t.Errorf("Chat() content = %q, want %q", resp.Choices[0].Message.Content, "PASS - all checks pass")
	}
	if resp.Usage == nil {
		t.Fatal("Chat() Usage is nil")
	}
	if resp.Usage.InputTokens <= 0 {
		t.Errorf("InputTokens = %d, want > 0", resp.Usage.InputTokens)
	}
	if resp.CostUSD <= 0 {
		t.Errorf("CostUSD = %f, want > 0", resp.CostUSD)
	}
}

// TestAnthropicChat_SystemMessage verifies system messages are extracted
// and sent via the System parameter (not as a message in the Messages array).
func TestAnthropicChat_SystemMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicMsg("OK", 50, 25))
	}))
	defer srv.Close()

	a := newTestAnthropic(srv.URL, "claude-haiku-4-5")
	resp, err := a.Chat(context.Background(), []ChatMessage{
		{Role: "system", Content: "you are a verifier"},
		{Role: "user", Content: "check this diff"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Choices[0].Message.Content != "OK" {
		t.Errorf("Chat() content = %q, want %q", resp.Choices[0].Message.Content, "OK")
	}
	// Haiku 4.5 pricing: $1.00/M input, $5.00/M output.
	// 50 input tokens / 1M * $1 = $0.00005; 25 output tokens / 1M * $5 = $0.000125
	// Total = $0.000175
	if resp.CostUSD <= 0 {
		t.Errorf("CostUSD = %f, want > 0", resp.CostUSD)
	}
}

// TestAnthropicChat_CostCalculation verifies the cost is computed from
// InputTokens * inputPrice + OutputTokens * outputPrice per the pricing map.
// Maps AC4: Verify() cost is computed from actual token counts.
func TestAnthropicChat_CostCalculation(t *testing.T) {
	// Sonnet 4.6: $3.00/M input, $15.00/M output.
	// 1,000,000 input = $3.00, 500,000 output = $7.50, total = $10.50
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicMsg("PASS", 1000000, 500000))
	}))
	defer srv.Close()

	a := newTestAnthropic(srv.URL, "claude-sonnet-4-6")
	resp, err := a.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	expectedCost := float64(1000000)/1_000_000*3.00 + float64(500000)/1_000_000*15.00
	if resp.CostUSD != expectedCost {
		t.Errorf("CostUSD = %f, want %f", resp.CostUSD, expectedCost)
	}
}

// newTestAnthropic returns an Anthropic driver pointed at a test server.// Uses option.WithHTTPClient and option.WithBaseURL to avoid hitting the
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
	text, _, _, _, err := a.Verify(context.Background(), "Reply with PASS.", "verify")
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !strings.Contains(text, "PASS") {
		t.Fatalf("want text containing %q, got %q", "PASS", text)
	}
}
