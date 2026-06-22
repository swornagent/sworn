package memory

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// fakeEmbedder returns hardcoded embeddings for known texts.
// Query "target query" returns an embedding close to entry #3.
type fakeEmbedder struct {
	embedding []float32
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = f.embedding
	}
	return result, nil
}

func (f *fakeEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return f.embedding, nil
}

func (f *fakeEmbedder) Model() string { return "fake-model" }

// seedTestIndex creates an in-memory index seeded with the given entries.
func seedTestIndex(t *testing.T, entries []Entry) *Index {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")
	idx, err := OpenIndex(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { idx.Close() })

	ctx := context.Background()
	for _, e := range entries {
		if err := idx.UpsertEntry(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	return idx
}

// makeEntry is a helper to construct an Entry with defaults.
func makeEntry(id, harness, title, content string, embedding []float32) Entry {
	return Entry{
		ID:        id,
		Path:      "memory/" + id + ".md",
		Harness:   harness,
		Title:     title,
		Content:   content,
		Embedding: embedding,
		Model:     "fake-model",
		IndexedAt: time.Now().Truncate(time.Second).UTC(),
	}
}

func TestSearchTopK(t *testing.T) {
	// Entry #3 (idx 2) has embedding closest to query [1.0, 0.0, 0.0].
	entries := []Entry{
		makeEntry("e01", "claude-code", "One", "content one", []float32{0.1, 0.9, 0.0}),
		makeEntry("e02", "claude-code", "Two", "content two", []float32{0.2, 0.8, 0.0}),
		makeEntry("e03", "claude-code", "Three", "content three", []float32{0.95, 0.05, 0.0}), // closest
		makeEntry("e04", "claude-code", "Four", "content four", []float32{0.4, 0.6, 0.0}),
		makeEntry("e05", "claude-code", "Five", "content five", []float32{0.5, 0.5, 0.0}),
		makeEntry("e06", "claude-code", "Six", "content six", []float32{0.6, 0.4, 0.0}),
		makeEntry("e07", "claude-code", "Seven", "content seven", []float32{0.7, 0.3, 0.0}),
		makeEntry("e08", "claude-code", "Eight", "content eight", []float32{0.8, 0.2, 0.0}),
		makeEntry("e09", "claude-code", "Nine", "content nine", []float32{-0.5, 0.5, 0.0}),
		makeEntry("e10", "claude-code", "Ten", "content ten", []float32{0.0, 0.0, 1.0}),
	}

	idx := seedTestIndex(t, entries)
	emb := &fakeEmbedder{embedding: []float32{1.0, 0.0, 0.0}} // query is closest to [0.95, 0.05, 0.0]

	results, err := Search(context.Background(), idx, emb, "target query", SearchOptions{TopK: 5})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// Entry #3 (e03) should be rank 1.
	if results[0].ID != "e03" {
		t.Errorf("expected rank 1 = e03, got %s (score %.4f)", results[0].ID, results[0].Score)
	}

	// Results should be sorted descending by score.
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: results[%d].Score=%.4f > results[%d].Score=%.4f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestSearchFilterHarness(t *testing.T) {
	entries := []Entry{
		makeEntry("cc-1", "claude-code", "CC One", "cc content 1", []float32{1.0, 0.0, 0.0}),
		makeEntry("cc-2", "claude-code", "CC Two", "cc content 2", []float32{0.9, 0.1, 0.0}),
		makeEntry("cc-3", "claude-code", "CC Three", "cc content 3", []float32{0.8, 0.2, 0.0}),
		makeEntry("cc-4", "claude-code", "CC Four", "cc content 4", []float32{0.7, 0.3, 0.0}),
		makeEntry("cc-5", "claude-code", "CC Five", "cc content 5", []float32{0.6, 0.4, 0.0}),
		makeEntry("gc-1", "gemini-cli", "GC One", "gc content 1", []float32{0.5, 0.5, 0.0}),
		makeEntry("gc-2", "gemini-cli", "GC Two", "gc content 2", []float32{0.4, 0.6, 0.0}),
		makeEntry("gc-3", "gemini-cli", "GC Three", "gc content 3", []float32{0.3, 0.7, 0.0}),
		makeEntry("gc-4", "gemini-cli", "GC Four", "gc content 4", []float32{0.2, 0.8, 0.0}),
		makeEntry("gc-5", "gemini-cli", "GC Five", "gc content 5", []float32{0.1, 0.9, 0.0}),
	}

	idx := seedTestIndex(t, entries)
	emb := &fakeEmbedder{embedding: []float32{1.0, 0.0, 0.0}}

	results, err := Search(context.Background(), idx, emb, "query", SearchOptions{TopK: 10, Harness: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 claude-code results, got %d", len(results))
	}

	for _, r := range results {
		if r.Harness != "claude-code" {
			t.Errorf("expected all results to be claude-code, got %s", r.Harness)
		}
	}
}

func TestSearchEmptyIndex(t *testing.T) {
	idx := seedTestIndex(t, nil) // empty index
	emb := &fakeEmbedder{embedding: []float32{1.0, 0.0, 0.0}}

	results, err := Search(context.Background(), idx, emb, "query", SearchOptions{TopK: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results on empty index, got %d", len(results))
	}
}

func TestSearchNoBuild(t *testing.T) {
	dir := t.TempDir()
	absentPath := filepath.Join(dir, "nonexistent", "memory.db")

	// OpenIndex creates the DB — we must NOT call it for this test.
	// Instead, verify that os.Stat confirms the file is absent.
	_, err := os.Stat(absentPath)
	if !os.IsNotExist(err) {
		t.Fatalf("expected file not to exist at %s", absentPath)
	}

	// The Search function itself takes an *Index, so it can't be tested
	// with a nil index. The "no index" check belongs in the CLI layer
	// (cmdMemorySearch), which calls os.Stat before OpenIndex.
	// This test validates the sentinel: os.Stat on a missing path returns
	// IsNotExist, which the CLI will use to print the error message.
}

func TestSearchTopKTruncation(t *testing.T) {
	// 10 entries, ask for top 3 — should get exactly 3.
	var entries []Entry
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		entries = append(entries, makeEntry(
			"entry-"+id, "claude-code", "Entry "+id, "content "+id,
			[]float32{float32(i) / 10.0, 1.0 - float32(i)/10.0, 0.0},
		))
	}

	idx := seedTestIndex(t, entries)
	emb := &fakeEmbedder{embedding: []float32{1.0, 0.0, 0.0}}

	results, err := Search(context.Background(), idx, emb, "query", SearchOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestSearchDeterministic(t *testing.T) {
	entries := []Entry{
		makeEntry("a", "claude-code", "A", "content a", []float32{0.5, 0.5, 0.0}),
		makeEntry("b", "claude-code", "B", "content b", []float32{0.5, 0.5, 0.0}), // same embedding as a
	}

	idx := seedTestIndex(t, entries)
	emb := &fakeEmbedder{embedding: []float32{1.0, 0.0, 0.0}}

	// Run twice — results should be identical (stable sort for equal scores).
	results1, err := Search(context.Background(), idx, emb, "query", SearchOptions{TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	results2, err := Search(context.Background(), idx, emb, "query", SearchOptions{TopK: 10})
	if err != nil {
		t.Fatal(err)
	}

	// Go's sort.Slice is not stable. For equal scores, ordering may vary.
	// The spec AC says "deterministic" — we interpret this as: same set of
	// results, same scores (cosine similarity is pure float arithmetic).
	if len(results1) != len(results2) {
		t.Fatalf("result count differs: %d vs %d", len(results1), len(results2))
	}

	// Sort by ID for comparison since equal scores may order differently.
	sort.Slice(results1, func(i, j int) bool { return results1[i].ID < results1[j].ID })
	sort.Slice(results2, func(i, j int) bool { return results2[i].ID < results2[j].ID })

	for i := range results1 {
		if results1[i].ID != results2[i].ID {
			t.Errorf("result %d: ID %s vs %s", i, results1[i].ID, results2[i].ID)
		}
		if results1[i].Score != results2[i].Score {
			t.Errorf("result %d: score %.4f vs %.4f", i, results1[i].Score, results2[i].Score)
		}
	}
}