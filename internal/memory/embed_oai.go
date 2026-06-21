package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type oaiCompatEmbedder struct {
	cfg EmbeddingConfig
}

func newOAICompatEmbedder(cfg EmbeddingConfig) (*oaiCompatEmbedder, error) {
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-large"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	return &oaiCompatEmbedder{cfg: cfg}, nil
}

func (e *oaiCompatEmbedder) Model() string {
	return e.cfg.Model
}

type oaiRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type oaiResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (e *oaiCompatEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	apiKey := os.Getenv(e.cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("missing API key in environment variable %s", e.cfg.APIKeyEnv)
	}

	// OAI batch size is typically 100
	const batchSize = 100
	var allEmbeddings [][]float32

	endpoint := e.cfg.BaseURL
	if !strings.HasSuffix(endpoint, "/v1/embeddings") {
		endpoint = strings.TrimRight(endpoint, "/") + "/v1/embeddings"
	}

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		reqBody := oaiRequest{
			Input: batch,
			Model: e.cfg.Model,
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
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}

		var oResp oaiResponse
		if err := json.NewDecoder(resp.Body).Decode(&oResp); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		if len(oResp.Data) != len(batch) {
			return nil, fmt.Errorf("expected %d embeddings, got %d", len(batch), len(oResp.Data))
		}

		batchEmbeddings := make([][]float32, len(batch))
		for _, d := range oResp.Data {
			if d.Index < 0 || d.Index >= len(batch) {
				return nil, fmt.Errorf("invalid index %d in response", d.Index)
			}
			batchEmbeddings[d.Index] = d.Embedding
		}

		allEmbeddings = append(allEmbeddings, batchEmbeddings...)
	}

	return allEmbeddings, nil
}
