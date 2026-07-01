package memory

import (
	"context"
	"fmt"
	"sort"
)

// SearchResult is a ranked search result from the memory index.
type SearchResult struct {
	ID      string  `json:"id"`
	Path    string  `json:"path"`
	Harness string  `json:"harness"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float32 `json:"score"`
	Model   string  `json:"model"`
}

// SearchOptions controls search behaviour.
type SearchOptions struct {
	TopK    int    // max results to return (default 10 if <= 0)
	Harness string // filter to this harness (empty = no filter)
}

// queryEmbedder is an optional interface for embedders that can embed a single
// query with the correct input_type. Voyage implements this; OAI-compat/Ollama
// fall through to Embed([]string{query}).
type queryEmbedder interface {
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
}

// Search performs semantic search over the memory index.
//
//  1. Embeds the query (using EmbedQuery if the embedder supports it, else Embed)
//  2. Loads all entries from the index
//  3. Computes cosine similarity between the query embedding and each entry
//  4. Returns top-K results sorted descending by score
func Search(ctx context.Context, idx *Index, emb Embedder, query string, opts SearchOptions) ([]SearchResult, error) {
	if opts.TopK <= 0 {
		opts.TopK = 10
	}

	// 1. Embed the query. Prefer EmbedQuery (Voyage input_type: "query") if
	//    the embedder supports it; fall back to Embed for OAI-compat / Ollama.
	var queryEmb []float32
	if qe, ok := emb.(queryEmbedder); ok {
		var err error
		queryEmb, err = qe.EmbedQuery(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("embedding query: %w", err)
		}
	} else {
		embs, err := emb.Embed(ctx, []string{query})
		if err != nil {
			return nil, fmt.Errorf("embedding query: %w", err)
		}
		if len(embs) > 0 {
			queryEmb = embs[0]
		}
	}

	if len(queryEmb) == 0 {
		return nil, nil
	}

	// 2. Load all entries from the index.
	entries, err := idx.AllEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading entries: %w", err)
	}

	// 3. Score every entry against the query embedding, applying harness filter.
	var results []SearchResult
	for _, e := range entries {
		if opts.Harness != "" && e.Harness != opts.Harness {
			continue
		}
		score := CosineSimilarity(queryEmb, e.Embedding)
		results = append(results, SearchResult{
			ID:      e.ID,
			Path:    e.Path,
			Harness: e.Harness,
			Title:   e.Title,
			Content: e.Content,
			Score:   score,
			Model:   e.Model,
		})
	}

	// 4. Sort descending by score.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 5. Truncate to top-K.
	if len(results) > opts.TopK {
		results = results[:opts.TopK]
	}

	return results, nil
}
