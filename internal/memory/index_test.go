package memory

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestUpsertAndRetrieve(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")

	idx, err := OpenIndex(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	ctx := context.Background()

	entry := Entry{
		ID:        ComputeID("path/to/file.md", "content"),
		Path:      "path/to/file.md",
		Harness:   "claude-code",
		Title:     "Test Title",
		Content:   "content",
		Embedding: []float32{0.1, 0.2, 0.3},
		Model:     "test-model",
		IndexedAt: time.Now().Truncate(time.Second).UTC(),
	}

	if err := idx.UpsertEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}

	retrieved, err := idx.GetEntry(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retrieved == nil {
		t.Fatal("expected entry, got nil")
	}

	if retrieved.Path != entry.Path {
		t.Errorf("expected path %s, got %s", entry.Path, retrieved.Path)
	}
	if retrieved.Content != entry.Content {
		t.Errorf("expected content %s, got %s", entry.Content, retrieved.Content)
	}
	if len(retrieved.Embedding) != len(entry.Embedding) {
		t.Fatalf("expected embedding length %d, got %d", len(entry.Embedding), len(retrieved.Embedding))
	}
	for i, v := range entry.Embedding {
		if retrieved.Embedding[i] != v {
			t.Errorf("expected embedding[%d] = %f, got %f", i, v, retrieved.Embedding[i])
		}
	}
	if !retrieved.IndexedAt.Equal(entry.IndexedAt) {
		t.Errorf("expected indexed_at %v, got %v", entry.IndexedAt, retrieved.IndexedAt)
	}
}

func TestChangeDetection(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")

	idx, err := OpenIndex(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	ctx := context.Background()

	entry := Entry{
		ID:        ComputeID("path/to/file.md", "content"),
		Path:      "path/to/file.md",
		Harness:   "claude-code",
		Title:     "Test Title",
		Content:   "content",
		Embedding: []float32{0.1, 0.2, 0.3},
		Model:     "test-model",
		IndexedAt: time.Now().Truncate(time.Second).UTC(),
	}

	if err := idx.UpsertEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}

	exists, err := idx.HasEntry(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected entry to exist")
	}

	// Upsert again
	if err := idx.UpsertEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := idx.db.QueryRow("SELECT COUNT(*) FROM memory_entries").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

func TestCosine(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		expected float32
	}{
		{
			name:     "identical",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "half",
			a:        []float32{1, 0},
			b:        []float32{1, 1},
			expected: float32(1.0 / math.Sqrt(2)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if math.Abs(float64(got-tt.expected)) > 1e-6 {
				t.Errorf("expected %f, got %f", tt.expected, got)
			}
		})
	}
}

func TestEmbeddingEncoding(t *testing.T) {
	emb := []float32{0.1, -0.2, 3.14159, 0.0}
	encoded := EncodeEmbedding(emb)
	decoded, err := DecodeEmbedding(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(emb) {
		t.Fatalf("expected length %d, got %d", len(emb), len(decoded))
	}
	for i, v := range emb {
		if decoded[i] != v {
			t.Errorf("expected %f, got %f", v, decoded[i])
		}
	}
}
