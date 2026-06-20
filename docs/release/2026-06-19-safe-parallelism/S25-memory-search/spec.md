---
title: 'S25-memory-search — sworn memory search + captain integration'
description: 'sworn memory search <query> returns semantically similar memory entries from the index. Replaces captain-memory-search.py with a sworn-binary shim that preserves the existing coach-loop output contract.'
---

# Slice: `S25-memory-search`

## User outcome

A developer or the coach-loop running `sworn memory search "encryption at rest
decisions"` receives a ranked list of relevant memory entries from the
configured index, with titles, harness source, and similarity scores.
The existing `captain-memory-search.py` is replaced by a thin shim that
delegates to `sworn memory search --json`, preserving the output contract
the coach-loop reads today. Captain's `/design-review` sessions now use
sworn's semantic search rather than the Python script, and work on any
machine — with or without local Ollama — by routing to the configured
cloud provider.

## Entry point

`sworn memory search <query> [--top-k N] [--json] [--harness <id>]`

Also: `~/.claude/bin/captain-memory-search.py` updated to be a thin shim
that calls `sworn memory search "$@" --json` and passes through stdout/stderr.

## In scope

### Search implementation

`internal/memory/search.go`:
```go
func Search(ctx context.Context, db *Index, emb Embedder, query string, topK int) ([]Result, error)

type Result struct {
    ID         string
    Path       string
    Harness    string
    Title      string
    Content    string
    Score      float32  // cosine similarity
    Model      string
}
```

Flow:
1. Embed the query using the configured provider with `input_type: "query"`
   (Voyage) or no `input_type` (OAI-compat / Ollama)
2. Load all embeddings from SQLite into memory (for corpora <50K entries this
   is fast; ~2MB for 500 entries at 1024 dims × float32)
3. Compute cosine similarity between query embedding and all stored embeddings
4. Return top-K results sorted descending by score

`internal/memory/search_test.go` — tests against a pre-seeded fixture index.

### CLI command

`cmd/sworn/memory.go` (extend — add `search` subcommand):
- Loads config (S23)
- Opens index at configured path (S24)
- Constructs embedder (S24)
- Calls `Search()`
- Human-readable output (default): table with rank, score, harness, title,
  first 120 chars of content
- JSON output (`--json`): array of `Result` objects
- `--harness <id>` filters results to a single harness
- `--top-k N` (default: 10)
- Exits non-zero if index does not exist (tell user to run `sworn memory build`)

### captain-memory-search.py shim

Update `~/.claude/bin/captain-memory-search.py`:
```python
#!/usr/bin/env python3
"""Thin shim: delegates to sworn memory search. Preserves coach-loop contract."""
import subprocess, sys, os
result = subprocess.run(
    ["sworn", "memory", "search", "--json", "--top-k", "10"] + sys.argv[1:],
    capture_output=False
)
sys.exit(result.returncode)
```

The shim preserves exact output format (JSON array) so the coach-loop needs
no changes. The `--batch` and `--rebuild-only` flags from the original Python
script map to `sworn memory build` — the shim does not implement them; callers
that used `--batch` should call `sworn memory build` directly. Document the
migration in a one-paragraph note at the top of the shim.

## Out of scope

- FAISS/ANN approximate nearest-neighbour (post-R3; linear scan fast enough
  for <50K entries)
- Reranking (post-R3)
- Filtering by date or similarity threshold (post-R3)
- Updating `captain-memory-search.py` to handle `--batch`/`--rebuild-only`
  (those flags now route to `sworn memory build`; coach-loop uses `sworn memory
  search` directly)
- Semantic diff across two queries (post-R3)

## Planned touchpoints

- `internal/memory/search.go` (new)
- `internal/memory/search_test.go` (new)
- `cmd/sworn/memory.go` (extend — add `search` subcommand)
- `~/.claude/bin/captain-memory-search.py` (update to shim)

## Acceptance checks

- [ ] `sworn memory search "key rotation"` returns ranked results from the
  index built by S24; output includes title, harness, and truncated content
- [ ] `sworn memory search "key rotation" --json` returns a valid JSON array
  of Result objects with `score`, `title`, `harness`, `path`, `content` fields
- [ ] `sworn memory search "key rotation" --harness claude-code` returns only
  entries sourced from the claude-code harness
- [ ] `sworn memory search` with no index file (S24 never run) exits non-zero
  with a clear message: "No memory index found. Run `sworn memory build` first."
- [ ] `captain-memory-search.py` with a query argument delegates to
  `sworn memory search --json` and exits with the same code
- [ ] `captain-memory-search.py --batch` prints a migration notice and exits 0
  (does not fail; tells user to run `sworn memory build` instead)
- [ ] `go test -race ./internal/memory/...` passes
- [ ] Search results are deterministic for the same query and index
  (cosine similarity is pure float arithmetic, no randomness)

## Required tests

- **Unit**: `internal/memory/search_test.go`
  - `TestSearchTopK`: pre-seeded fixture index with 10 known entries; query
    known to be closest to entry #3; confirm entry #3 is rank 1
  - `TestSearchFilterHarness`: 5 claude-code entries + 5 gemini-cli entries;
    `--harness claude-code` returns only the 5 claude-code entries
  - `TestSearchEmptyIndex`: Search on an empty index returns empty slice, no error
  - `TestSearchNoBuild`: Index file absent → named error, not panic

- **Integration** (optional, skipped in CI without Ollama): run
  `sworn memory build` on a fixture memory directory with Ollama, then
  `sworn memory search` and confirm rank-1 result is the expected entry.
  Guard with `t.Skip("requires local Ollama")` when OLLAMA not available.

- **Reachability artefact**: `sworn memory search "sqlite concurrency"` run
  against this repo's Claude Code memory index (or a fixture); output captured
  in proof.md showing at least one result related to the orchestration state
  ADR (confirming S01/T1 memory entries are searchable).

## Risks

- `float32` precision drift between build and search could produce slightly
  different similarity scores if the embedding is re-computed. **Mitigation**:
  embeddings are stored and retrieved from the DB; query embedding is freshly
  computed at search time — this is by design, not a bug.
- Coach-loop `captain-memory-search.py --batch` calls will break after the
  shim is deployed. **Mitigation**: shim catches `--batch` and `--rebuild-only`
  flags and prints a migration message (`sworn memory build`) before exiting 0.
  Coach-loop does not use `--batch` in the current version per captain-handbook §5.

## Deferrals allowed?

Yes, with Rule 2 cards:
- ANN / approximate search (linear scan is fast enough for R3 corpus sizes)
- Reranking (out of scope; add after measuring recall quality in production)
