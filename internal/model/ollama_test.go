package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOllamaVerify_ReturnsContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s, want /api/chat", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"message":{"role":"assistant","content":"PASS"},"done":true}`)
	}))
	defer srv.Close()

	o := NewOllama("llama3.2", srv.URL)
	text, cost, err := o.Verify(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if text != "PASS" {
		t.Errorf("Verify() = %q, want %q", text, "PASS")
	}
	if cost != 0 {
		t.Errorf("cost = %f, want 0", cost)
	}
}

func TestOllamaVerify_ErrorField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"error":"model not found"}`)
	}))
	defer srv.Close()

	o := NewOllama("nonexistent", srv.URL)
	_, _, err := o.Verify(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("Verify() returned nil error, want non-nil")
	}
}

func TestOllamaVerify_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	o := NewOllama("llama3.2", srv.URL)
	_, _, err := o.Verify(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("Verify() returned nil error for 503, want non-nil")
	}
}

func TestOllamaDefaultHost(t *testing.T) {
	// Clear OLLAMA_HOST to test the built-in default.
	os.Unsetenv("OLLAMA_HOST")
	o := NewOllama("llama3.2", "")
	if o.Host != "http://localhost:11434" {
		t.Errorf("Host = %q, want http://localhost:11434", o.Host)
	}
}

func TestOllamaHostFromEnv(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://gpu-box:9999")
	o := NewOllama("llama3.2", "")
	if o.Host != "http://gpu-box:9999" {
		t.Errorf("Host = %q, want http://gpu-box:9999", o.Host)
	}
}

func TestOllamaRequestFormat(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"message":{"role":"assistant","content":"OK"},"done":true}`)
	}))
	defer srv.Close()

	o := NewOllama("llama3.2", srv.URL)
	_, _, err := o.Verify(context.Background(), "You are a verifier.", "Reply with PASS.")
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}

	var req ollamaChatRequest
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("unmarshal captured request: %v", err)
	}
	if req.Stream {
		t.Error("stream = true, want false")
	}
	if req.Model != "llama3.2" {
		t.Errorf("model = %q, want llama3.2", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages count = %d, want 2", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "You are a verifier." {
		t.Errorf("system message: role=%q content=%q", req.Messages[0].Role, req.Messages[0].Content)
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != "Reply with PASS." {
		t.Errorf("user message: role=%q content=%q", req.Messages[1].Role, req.Messages[1].Content)
	}
}

func TestNewClient_OllamaIsNative(t *testing.T) {
	cfg := ProviderConfig{OllamaHost: "http://ollama.local:11434"}
	v, err := NewClient("ollama/llama3.2", cfg)
	if err != nil {
		t.Fatalf("NewClient(ollama/llama3.2) error: %v", err)
	}
	o, ok := v.(*Ollama)
	if !ok {
		t.Fatalf("NewClient(ollama/llama3.2) returned %T, want *Ollama", v)
	}
	if o.Host != "http://ollama.local:11434" {
		t.Errorf("Host = %q, want http://ollama.local:11434", o.Host)
	}
	if o.Model != "llama3.2" {
		t.Errorf("Model = %q, want llama3.2", o.Model)
	}
}
