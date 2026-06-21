# Proof Bundle: S24-memory-engine

## Scope
Implemented the `sworn memory build` command, embedding drivers (Voyage, OAI-compat, Ollama), SQLite vector index, and entry discovery for AI coding harnesses.

## Files changed
```
cmd/sworn/memory.go
docs/release/2026-06-19-safe-parallelism/S24-memory-engine/design.md
docs/release/2026-06-19-safe-parallelism/S24-memory-engine/status.json
internal/memory/discover.go
internal/memory/discover_test.go
internal/memory/embed.go
internal/memory/embed_oai.go
internal/memory/embed_ollama.go
internal/memory/embed_test.go
internal/memory/embed_voyage.go
internal/memory/index.go
internal/memory/index_test.go
```

## Test results
```
$ go test -race ./internal/memory/...
ok  	github.com/swornagent/sworn/internal/memory	1.163s
```

## Reachability artefact
```
$ ./sworn memory build
Indexed 3 entries (3 new, 0 unchanged) via nomic-embed-text in 129ms

$ ./sworn memory build
Indexed 3 entries (0 new, 3 unchanged) via nomic-embed-text in 1ms

$ ./sworn memory build --force
Indexed 3 entries (3 new, 0 unchanged) via nomic-embed-text in 74ms
```

## Delivered
- `sworn memory build` command with `--force` flag (cmd/sworn/memory.go)
- Embedder interface and drivers for Voyage, OAI-compat, Ollama (internal/memory/embed*.go)
- SQLite vector index with change detection and cosine similarity (internal/memory/index.go)
- Entry discovery for Claude Code, flat files, and custom paths (internal/memory/discover.go)
- Addressed all 6 Coach pins from design review.

## Not delivered
- Index pruning (entries for deleted memory files stay in DB)
  - **Why**: Post-R3, low impact since corpus is small.
  - **Tracking**: Rule 2 deferral.
  - **Acknowledged**: Coach, 2026-06-21
- TUI progress bar for large builds
  - **Why**: Post-R3 enhancement.
  - **Tracking**: Rule 2 deferral.
  - **Acknowledged**: Coach, 2026-06-21

## Divergence from plan
None.
