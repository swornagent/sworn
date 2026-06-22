# Design TL;DR — S24-memory-engine

## §1. User-visible change

A developer runs `sworn memory build` (or `sworn memory build --force`) and sees their configured memory paths scanned, each entry compared against the existing SQLite index via SHA256, only new/changed entries submitted to the configured embedding provider, and results upserted into the index at `~/.sworn/memory.db` (default). The command exits 0 with a one-line summary: "Indexed N entries (X new, Y unchanged) via <model> in <duration>." On `--force`, all entries are re-embedded regardless of change-detection state. If an embedding provider is unreachable (e.g. Ollama not running), the command exits non-zero with a clear error message — not a panic.

## §2. Design decisions not in spec (max 5)

1. **Claude Code MEMORY.md → linked-file resolution.** The spec says parse MEMORY.md for `- [Title](file.md)` links, then read each linked file as one entry. I'll read MEMORY.md, extract links via regex (not a full Markdown parser — overkill for a known format), resolve relative links against the MEMORY.md parent directory, and read each into an entry with `path = resolved link path`, `title = link text` (or first line of the file), `content = full file text`, `harness = claude-code`.

2. **Flat-file harness entry splitting.** For Gemini CLI (`GEMINI.md`), Cursor (`.cursorrules`), Windsurf (`.windsurfrules`), OpenCode (`AGENTS.md`), and custom paths: read the whole file. If it contains `---` separators, split into N entries (each separator-delimited block is one entry); otherwise the whole file is one entry. Each entry's `path` includes a fragment or index to disambiguate entries from the same file for SHA256 change detection.

3. **Embedding batch concatenation order.** The Embedder interface accepts `[]string` and returns `[][]float32`. When the input exceeds the provider's batch size, the driver splits into multiple HTTP requests and concatenates the result embeddings in the same order as the input texts (not shuffled). This is critical — any batch-splitting driver MUST preserve input ordering. Each driver's test validates with >1 batch worth of texts.

4. **SQLite connection lifecycle.** Open the database once at build start, close after all upserts are committed. Use a single `*sql.DB` (from `database/sql` + `modernc.org/sqlite` driver) per build operation. No connection pooling, no WAL mode toggle (the default modernc.org/sqlite behaviour is fine for <500-entry corpora). The index path is created automatically; if the parent directory doesn't exist, create it.

5. **API key reading at call time, not at config load time.** `os.Getenv(cfg.APIKeyEnv)` is called inside each driver's `Embed()` method, not during `NewEmbedder()`. This means a key set mid-session is picked up on the next `Embed()` call, and a missing key produces an error at embed time (not at config-parse time, giving the user a chance to set it between `sworn memory status` and `sworn memory build`).

## §3. Files I'll touch grouped by purpose

**Embedding interface + drivers (new files):**
- `internal/memory/embed.go` — `Embedder` interface, `NewEmbedder` factory
- `internal/memory/embed_voyage.go` — Voyage AI driver (stdlib net/http, no SDK)
- `internal/memory/embed_oai.go` — OAI-compatible driver (covers OpenAI, Fireworks, Together AI, etc.)
- `internal/memory/embed_ollama.go` — Ollama native driver

**Tests for embedding (new file):**
- `internal/memory/embed_test.go` — httptest.NewServer fakes for each driver, batch-splitting, auth header, error responses

**Vector index (new files):**
- `internal/memory/index.go` — SQLite schema, upsert logic, change detection, cosine similarity
- `internal/memory/index_test.go` — TestUpsertAndRetrieve, TestChangeDetection, TestCosine

**Entry discovery (new files):**
- `internal/memory/discover.go` — Discover entries per harness (MEMORY.md parsing, flat-file reading, separator splitting)
- `internal/memory/discover_test.go` — Tests for each harness parsing strategy

**CLI command (existing file, extended):**
- `cmd/sworn/memory.go` — Add `build` subcommand handler (`sworn memory build [--force]`)

## §4. Things I'm NOT doing

- Search query execution (S25 — separate slice)
- Index pruning / deleted entry cleanup (post-R3)
- Multiple index files or per-project indices (one global index only)
- TUI progress bar for large builds (post-R3)
- FAISS/hnswlib vector search acceleration (S25 scope, post-R3 if needed)
- Voyage `input_type: "query"` parameter (S25's concern — build uses `input_type: "document"`)
- Embedding caching beyond the SQLite SHA256 change-detection approach
- Test coverage for provider-connectivity failures beyond httptest fakes (e2e coverage deferred)
- Custom entry parsing for non-standard directory structures

## §5. Reachability plan

1. `go test -race ./internal/memory/...` — all embedding, index, discovery tests pass with httptest fakes (no live API calls)
2. `go build ./...` — binary compiles with the `build` subcommand wired in
3. Integration test: create a temp directory containing a fixture MEMORY.md + linked files, configure memory.json to point at it (voyage provider with a httptest listener), run `sworn memory build`, verify exit 0 + output line matching the summary pattern. Capture output as reachability artefact in proof.md.
4. Re-run to verify change detection: second build reports 0 new entries.
5. `--force` variant re-embeds all entries regardless of SHA256 match.

## §6. Open questions for the Coach

1. **Embedding BLOB storage format.** The spec says `[]float32 as little-endian IEEE 754`. I'll write a helper to encode `[]float32` → `[]byte` (4 bytes per float, LE) and decode back. Is this the expected binary format, or would JSON-encoded `[]float32` be acceptable for R3 (simpler to inspect)? I propose LE binary for storage efficiency (50 float32 = 200 bytes vs ~300+ bytes as JSON); the round-trip test validates correctness.

2. **Modernc.org/sqlite driver import path.** I've confirmed `modernc.org/sqlite` is in go.mod. The standard import is `_ "modernc.org/sqlite"` with driver name `"sqlite"`. Has this been tested in the S01 verifier-core code that uses it? (The grep shows it's used in `internal/db/db.go` and `internal/supervisor/supervisor.go`.)

3. **Claude Code MEMORY.md format stability.** The spec assumes `MEMORY.md` exists at the Claude Code memory directory with `- [Title](file.md)` link format. Is it safe to assume Markdown link syntax, or could entries also be plain bulleted text (e.g. `- Some note` without a link)? I'll accept only linked entries; unlinked bullet text is skipped. If this is too restrictive, flag it now.