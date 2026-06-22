package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ollamaEmbedder struct {
	cfg EmbeddingConfig
}

func newOllamaEmbedder(cfg EmbeddingConfig) (*ollamaEmbedder, error) {
	if cfg.Model == "" {
		cfg.Model = "nomic-embed-text"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	return &ollamaEmbedder{cfg: cfg}, nil
}

func (e *ollamaEmbedder) Model() string {
	return e.cfg.Model
}

type ollamaRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (e *ollamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Ollama batch size conservative for local GPU
	const batchSize = 50
	var allEmbeddings [][]float32

	endpoint := e.cfg.BaseURL
	if !strings.HasSuffix(endpoint, "/api/embed") {
		endpoint = strings.TrimRight(endpoint, "/") + "/api/embed"
	}

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		reqBody := ollamaRequest{
			Model: e.cfg.Model,
			Input: batch,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshaling request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}

		var oResp ollamaResponse
		if err := json.NewDecoder(resp.Body).Decode(&oResp); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		if len(oResp.Embeddings) != len(batch) {
			return nil, fmt.Errorf("expected %d embeddings, got %d", len(batch), len(oResp.Embeddings))
		}

		allEmbeddings = append(allEmbeddings, oResp.Embeddings...)
	}

	return allEmbeddings, nil
}
