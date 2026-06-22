## §1. User-visible change

Adds a new `sworn memory search` CLI subcommand that returns semantically similar memory entries from the index, replacing the existing `captain-memory-search.py` shim. The command supports JSON output, harness filtering, and configurable result limits.

## §2. Design decisions not in spec (max 5)

- **Linear scan over index** – Chosen for simplicity and acceptable performance for <50K entries; avoids adding ANN dependencies.
- **Float32 similarity scores** – Uses `float32` for cosine similarity to balance precision and storage size.
- **Default embedder selection** – Uses the embedder configured in `S23-memory-config`; supports Voyage or OAI‑compat/Ollama based on config.
- **CLI defaults** – `--top-k` defaults to 10 results; `--json` flag must be explicitly requested for structured output.
- **Shim behavior for legacy flags** – The updated `captain-memory-search.py` shim detects `--batch`/`--rebuild-only` and prints a migration notice before exiting 0, preserving backward compatibility.

## §3. Files I'll touch grouped by purpose

- **Implementation**: `internal/memory/search.go`, `internal/memory/search_test.go`
- **CLI integration**: `cmd/sworn/memory.go`
- **Compatibility shim**: `~/.claude/bin/captain-memory-search.py`

## §4. Things I'm NOT doing

- Not implementing approximate nearest‑neighbour (ANN) indexing; deferred to a future release.
- Not adding date or similarity‑threshold filtering options.
- Not providing a `--batch` implementation in the Go CLI; the shim will handle migration.

## §5. Reachability plan

- Run `sworn memory search "sqlite concurrency"` against the repository's memory index and capture the output in `proof.md`.
- Verify the shim `captain-memory-search.py` delegates correctly by invoking it with a query and checking the exit code and output.

## §6. Open questions for the Coach

- None at this time.