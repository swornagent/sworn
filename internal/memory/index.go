package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Entry represents a single memory entry.
type Entry struct {
	ID        string
	Path      string
	Harness   string
	Title     string
	Content   string
	Embedding []float32
	Model     string
	IndexedAt time.Time
}

// ComputeID computes the SHA256 hash of the path and content.
func ComputeID(path, content string) string {
	h := sha256.New()
	h.Write([]byte(path))
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// EncodeEmbedding converts a float32 slice to a little-endian byte slice.
func EncodeEmbedding(emb []float32) []byte {
	buf := make([]byte, len(emb)*4)
	for i, f := range emb {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// DecodeEmbedding converts a little-endian byte slice to a float32 slice.
func DecodeEmbedding(buf []byte) ([]float32, error) {
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding byte length: %d", len(buf))
	}
	emb := make([]float32, len(buf)/4)
	for i := range emb {
		emb[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return emb, nil
}

// CosineSimilarity computes the cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// Index represents the SQLite-backed vector index.
type Index struct {
	db *sql.DB
}

// OpenIndex opens the SQLite database at the given path, creating it and its
// parent directories if necessary.
func OpenIndex(path string) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating index directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return &Index{db: db}, nil
}

func initSchema(db *sql.DB) error {
	query := `
CREATE TABLE IF NOT EXISTS memory_entries (
  id TEXT PRIMARY KEY,
  path TEXT NOT NULL,
  harness TEXT NOT NULL,
  title TEXT,
  content TEXT NOT NULL,
  embedding BLOB NOT NULL,
  model TEXT NOT NULL,
  indexed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_harness ON memory_entries(harness);
`
	_, err := db.Exec(query)
	return err
}

// Close closes the database connection.
func (idx *Index) Close() error {
	return idx.db.Close()
}

// HasEntry returns true if an entry with the given ID exists in the index.
func (idx *Index) HasEntry(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := idx.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM memory_entries WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

// UpsertEntry inserts or replaces an entry in the index.
func (idx *Index) UpsertEntry(ctx context.Context, entry Entry) error {
	query := `
INSERT OR REPLACE INTO memory_entries (id, path, harness, title, content, embedding, model, indexed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`
	_, err := idx.db.ExecContext(ctx, query,
		entry.ID,
		entry.Path,
		entry.Harness,
		entry.Title,
		entry.Content,
		EncodeEmbedding(entry.Embedding),
		entry.Model,
		entry.IndexedAt.Format(time.RFC3339),
	)
	return err
}

// GetEntry retrieves an entry by ID.
func (idx *Index) GetEntry(ctx context.Context, id string) (*Entry, error) {
	query := `SELECT path, harness, title, content, embedding, model, indexed_at FROM memory_entries WHERE id = ?`
	row := idx.db.QueryRowContext(ctx, query, id)

	var e Entry
	e.ID = id
	var embBytes []byte
	var indexedAtStr string

	err := row.Scan(&e.Path, &e.Harness, &e.Title, &e.Content, &embBytes, &e.Model, &indexedAtStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	e.Embedding, err = DecodeEmbedding(embBytes)
	if err != nil {
		return nil, fmt.Errorf("decoding embedding: %w", err)
	}

	e.IndexedAt, err = time.Parse(time.RFC3339, indexedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing indexed_at: %w", err)
	}

	return &e, nil
}