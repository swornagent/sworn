---
title: 'S24-memory-engine — embedding adapter + SQLite vector index + sworn memory build'
description: 'Embedding adapter (voyage-code-3 / OAI-compat / Ollama) and SQLite-backed vector index. sworn memory build scans configured memory paths, embeds entries, stores them for semantic search.'
---

# Slice: `S24-memory-engine`

## User outcome

A developer running `sworn memory build` sees their configured memory paths
scanned, all memory entries embedded via the configured provider, and the
result stored at the configured index path (default `~/.sworn/memory.db`).
On re-run, only changed or new entries are re-embedded (incremental). The
build exits 0 with a summary: N entries indexed, M skipped (unchanged),
provider used, time taken.

## Entry point

`sworn memory build [--force]` — reads `MemoryConfig` from S23, discovers
entries in configured paths, calls the embedding provider, writes/updates
the SQLite index. `--force` re-embeds all entries regardless of change
detection.

## In scope

### Embedder interface + drivers

`internal/memory/embed.go` — `Embedder` interface:
```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Model() string
}
func NewEmbedder(cfg EmbeddingConfig) (Embedder, error)
```

Three drivers (no external SDK deps — stdlib `net/http` + `encoding/json`):

**`internal/memory/embed_voyage.go`** — Voyage AI driver:
- Endpoint: `https://api.voyageai.com/v1/embeddings`
- Default model: `voyage-code-3`
- `input_type: "document"` on build, `input_type: "query"` on search (S25)
- Bearer auth from `os.Getenv(cfg.APIKeyEnv)`
- Batch size: 128 texts per request (Voyage API limit)

**`internal/memory/embed_oai.go`** — OAI-compatible driver:
- Endpoint: `cfg.BaseURL + "/v1/embeddings"` (default: `https://api.openai.com`)
- Covers: OpenAI (`text-embedding-3-large`), nomic via Fireworks
  (`nomic-ai/nomic-embed-text-v1.5`, base `https://api.fireworks.ai`), nomic via
  Together AI (base `https://api.together.xyz`), any other OAI-compat endpoint
- Bearer auth from `os.Getenv(cfg.APIKeyEnv)`
- Batch size: 100 texts per request

**`internal/memory/embed_ollama.go`** — Ollama native driver:
- Endpoint: `http://localhost:11434/api/embed` (default; overridable via `base_url`)
- Default model: `nomic-embed-text`
- No auth required
- Batch size: 50 texts per request (conservative for local GPU memory)

### Vector index

`internal/memory/index.go` — SQLite-backed store using `modernc.org/sqlite`
(already in go.mod from S01):
```sql
CREATE TABLE IF NOT EXISTS memory_entries (
  id TEXT PRIMARY KEY,          -- SHA256 of (path + content)
  path TEXT NOT NULL,           -- source file path
  harness TEXT NOT NULL,        -- claude-code | gemini-cli | opencode | ...
  title TEXT,                   -- first heading or first line
  content TEXT NOT NULL,        -- raw text of the entry
  embedding BLOB NOT NULL,      -- []float32 as little-endian IEEE 754
  model TEXT NOT NULL,          -- embedding model used
  indexed_at TEXT NOT NULL      -- RFC3339
);
CREATE INDEX IF NOT EXISTS idx_harness ON memory_entries(harness);
```

Cosine similarity computed in Go (no SQLite extension needed for corpora
under ~50K entries; typical memory corpus is 50–500 entries):
```go
func CosineSimilarity(a, b []float32) float32
```

### Entry discovery

`internal/memory/discover.go` — scans each configured path:
- For Claude Code: reads `MEMORY.md` index file, parses `- [Title](file.md)` links,
  reads each linked file as one entry
- For Gemini CLI / cursor / windsurf / opencode: reads the single flat file as
  one entry (or N entries split by `---` separators if present)
- For custom paths: each file in the directory is one entry

Change detection: entry is unchanged if SHA256(content) matches `id` in the DB.

### `sworn memory build`

`cmd/sworn/memory.go` (extend S23's command):
- Loads config (S23)
- Discovers entries per harness
- Diffs against existing index (skip unchanged)
- Batches new/changed entries → embedding provider
- Upserts into SQLite
- Prints summary: `Indexed 47 entries (12 new, 35 unchanged) via voyage-code-3 in 2.3s`

## Out of scope

- Search query execution (S25)
- Index pruning (deleted entries stay in DB; S25 search still returns them — post-R3)
- Multiple index files (one global index only in R3)
- Embeddings caching beyond the SQLite change-detection approach
- TUI progress for large builds (post-R3)

## Planned touchpoints

- `internal/memory/embed.go` (new)
- `internal/memory/embed_voyage.go` (new)
- `internal/memory/embed_oai.go` (new)
- `internal/memory/embed_ollama.go` (new)
- `internal/memory/embed_test.go` (new)
- `internal/memory/index.go` (new)
- `internal/memory/index_test.go` (new)
- `internal/memory/discover.go` (new)
- `internal/memory/discover_test.go` (new)
- `cmd/sworn/memory.go` (extend — add `build` subcommand)

## Acceptance checks

- [ ] `sworn memory build` with voyage provider configured and `VOYAGE_API_KEY`
  set embeds all discovered entries and writes them to the index; exits 0 with
  a count summary
- [ ] Re-running `sworn memory build` immediately after a successful build
  reports 0 new entries and skips all embedding API calls (change detection
  works via SHA256)
- [ ] `sworn memory build` with `provider: "oai-compat"` and a Fireworks AI
  endpoint (`base_url: "https://api.fireworks.ai"`) successfully embeds entries
  using `nomic-ai/nomic-embed-text-v1.5`
- [ ] `sworn memory build` with `provider: "ollama"` uses the local Ollama
  `/api/embed` endpoint; if Ollama is not running, exits non-zero with a clear
  error (not a panic)
- [ ] `sworn memory build --force` re-embeds all entries even when SHA256
  matches existing index rows
- [ ] `api_key_env` value is never logged or written to the index; the key
  is read only at call time from `os.Getenv`
- [ ] `go test -race ./internal/memory/...` passes; embedding tests use
  httptest.NewServer fakes (no live API calls in tests)

## Required tests

- **Unit**: `internal/memory/embed_test.go`
  - Each driver tested against an `httptest.NewServer` that validates request
    shape and returns a well-formed embedding response
  - Batch splitting tested: >128 texts for voyage driver produces multiple
    requests, all embeddings concatenated correctly
  - Auth header present and correct; key comes from env, not hardcoded

- **Unit**: `internal/memory/index_test.go`
  - `TestUpsertAndRetrieve`: insert entry, retrieve by id, content matches
  - `TestChangeDetection`: upsert same entry twice; second upsert is a no-op
    (same SHA256); DB row count stays at 1
  - `TestCosine`: known vectors with known similarity ≈0.0, ≈0.5, ≈1.0

- **Unit**: `internal/memory/discover_test.go`
  - Claude Code MEMORY.md parsing: `[Title](file.md)` links resolved to entries
  - Flat-file harness (cursor `.cursorrules`): entire file = one entry
  - `---` separator splitting for multi-entry flat files

- **Reachability artefact**: `sworn memory build` run against this repo's
  Claude Code memory directory (or a fixture); output captured in proof.md
  including entry count and provider used.

## Risks

- Voyage AI HTTP API shape may differ from the OpenAI embedding response format
  (different `object` field, `usage` field). **Mitigation**: parse only the
  fields we use (`data[].embedding`); add a test against the exact Voyage
  response shape from their public docs.
- Cosine similarity in Go for large corpora (>10K entries) may be slow.
  **Mitigation**: not a risk for R3 (typical corpus is <500 entries); note in
  ADR and plan FAISS/hnswlib for post-R3 if needed.
- `modernc.org/sqlite` BLOB storage for `[]float32` requires correct
  little-endian IEEE 754 encoding. **Mitigation**: add a round-trip test
  (encode → store → retrieve → decode → compare).

## Deferrals allowed?

Yes, with Rule 2 cards:
- Index pruning (entries for deleted memory files stay in DB) — post-R3,
  low impact since corpus is small
- TUI progress bar for large builds — post-R3
