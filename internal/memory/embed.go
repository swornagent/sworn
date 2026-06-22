package memory

import (
	"context"
	"fmt"
)

// Embedder is the interface for embedding providers.
type Embedder interface {
	// Embed takes a list of texts and returns their embeddings.
	// The returned slice must have the same length as the input slice,
	// and the embeddings must be in the same order.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	
	// Model returns the name of the model being used.
	Model() string
}

// NewEmbedder creates a new Embedder based on the configuration.
func NewEmbedder(cfg EmbeddingConfig) (Embedder, error) {
	switch cfg.Provider {
	case ProviderVoyage:
		return newVoyageEmbedder(cfg)
	case ProviderOAICompat:
		return newOAICompatEmbedder(cfg)
	case ProviderOllama:
		return newOllamaEmbedder(cfg)
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Provider)
	}
}
