package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type voyageEmbedder struct {
	cfg EmbeddingConfig
}

func newVoyageEmbedder(cfg EmbeddingConfig) (*voyageEmbedder, error) {
	if cfg.Model == "" {
		cfg.Model = "voyage-code-3"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.voyageai.com/v1/embeddings"
	}
	return &voyageEmbedder{cfg: cfg}, nil
}

func (e *voyageEmbedder) Model() string {
	return e.cfg.Model
}

type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"`
}

type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// EmbedQuery embeds a single query string with input_type set to "query".
// This improves asymmetric search recall compared to the default "document" type.
func (e *voyageEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	apiKey := os.Getenv(e.cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("missing API key in environment variable %s", e.cfg.APIKeyEnv)
	}

	reqBody := voyageRequest{
		Input:     []string{query},
		Model:     e.cfg.Model,
		InputType: "query",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.cfg.BaseURL, bytes.NewReader(bodyBytes))
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

	var vResp voyageResponse
	if err := json.NewDecoder(resp.Body).Decode(&vResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(vResp.Data) != 1 {
		return nil, fmt.Errorf("expected 1 embedding, got %d", len(vResp.Data))
	}

	return vResp.Data[0].Embedding, nil
}

func (e *voyageEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {	if len(texts) == 0 {
		return nil, nil
	}

	apiKey := os.Getenv(e.cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("missing API key in environment variable %s", e.cfg.APIKeyEnv)
	}

	// Voyage API limit is 128 texts per request
	const batchSize = 128
	var allEmbeddings [][]float32

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		reqBody := voyageRequest{
			Input:     batch,
			Model:     e.cfg.Model,
			InputType: "document",
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshaling request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", e.cfg.BaseURL, bytes.NewReader(bodyBytes))
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

		var vResp voyageResponse
		if err := json.NewDecoder(resp.Body).Decode(&vResp); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		if len(vResp.Data) != len(batch) {
			return nil, fmt.Errorf("expected %d embeddings, got %d", len(batch), len(vResp.Data))
		}

		// Ensure order matches input
		batchEmbeddings := make([][]float32, len(batch))
		for _, d := range vResp.Data {
			if d.Index < 0 || d.Index >= len(batch) {
				return nil, fmt.Errorf("invalid index %d in response", d.Index)
			}
			batchEmbeddings[d.Index] = d.Embedding
		}

		allEmbeddings = append(allEmbeddings, batchEmbeddings...)
	}

	return allEmbeddings, nil
}
