package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestVoyageEmbedder(t *testing.T) {
	os.Setenv("TEST_VOYAGE_KEY", "test-key")
	defer os.Unsetenv("TEST_VOYAGE_KEY")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		var req voyageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}

		resp := voyageResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				Embedding: []float32{float32(i), 1.0, 2.0},
				Index:     i,
			})
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	cfg := EmbeddingConfig{
		Provider:  ProviderVoyage,
		Model:     "voyage-code-3",
		APIKeyEnv: "TEST_VOYAGE_KEY",
		BaseURL:   ts.URL,
	}

	embedder, err := NewEmbedder(cfg)
	if err != nil {
		t.Fatal(err)
	}

	texts := make([]string, 150)
	for i := range texts {
		texts[i] = "text"
	}

	embeddings, err := embedder.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}

	if len(embeddings) != 150 {
		t.Fatalf("expected 150 embeddings, got %d", len(embeddings))
	}
	if embeddings[0][0] != 0.0 {
		t.Errorf("expected 0.0, got %f", embeddings[0][0])
	}
	if embeddings[128][0] != 0.0 { // First element of second batch
		t.Errorf("expected 0.0, got %f", embeddings[128][0])
	}
}

func TestOAICompatEmbedder(t *testing.T) {
	os.Setenv("TEST_OAI_KEY", "test-key")
	defer os.Unsetenv("TEST_OAI_KEY")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/embeddings") {
			t.Errorf("expected path to end with /v1/embeddings, got %s", r.URL.Path)
		}
		var req oaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}

		resp := oaiResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				Embedding: []float32{float32(i), 1.0, 2.0},
				Index:     i,
			})
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	cfg := EmbeddingConfig{
		Provider:  ProviderOAICompat,
		Model:     "text-embedding-3-large",
		APIKeyEnv: "TEST_OAI_KEY",
		BaseURL:   ts.URL,
	}

	embedder, err := NewEmbedder(cfg)
	if err != nil {
		t.Fatal(err)
	}

	texts := make([]string, 150)
	for i := range texts {
		texts[i] = "text"
	}

	embeddings, err := embedder.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}

	if len(embeddings) != 150 {
		t.Fatalf("expected 150 embeddings, got %d", len(embeddings))
	}
}

func TestOllamaEmbedder(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/embed") {
			t.Errorf("expected path to end with /api/embed, got %s", r.URL.Path)
		}
		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}

		resp := ollamaResponse{}
		for i := range req.Input {
			resp.Embeddings = append(resp.Embeddings, []float32{float32(i), 1.0, 2.0})
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	cfg := EmbeddingConfig{
		Provider: ProviderOllama,
		Model:    "nomic-embed-text",
		BaseURL:  ts.URL,
	}

	embedder, err := NewEmbedder(cfg)
	if err != nil {
		t.Fatal(err)
	}

	texts := make([]string, 60)
	for i := range texts {
		texts[i] = "text"
	}

	embeddings, err := embedder.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}

	if len(embeddings) != 60 {
		t.Fatalf("expected 60 embeddings, got %d", len(embeddings))
	}
}

func TestEmbedderAPIKeyEnvNotLeaked(t *testing.T) {
	sentinel := "SECRET_SENTINEL_KEY_123"
	os.Setenv("TEST_LEAK_KEY", sentinel)
	defer os.Unsetenv("TEST_LEAK_KEY")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer ts.Close()

	cfg := EmbeddingConfig{
		Provider:  ProviderVoyage,
		Model:     "voyage-code-3",
		APIKeyEnv: "TEST_LEAK_KEY",
		BaseURL:   ts.URL,
	}

	embedder, err := NewEmbedder(cfg)
	if err != nil {
		t.Fatal(err)
	}

	_, err = embedder.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error")
	}

	if strings.Contains(err.Error(), sentinel) {
		t.Errorf("error message leaked API key: %v", err)
	}
}
