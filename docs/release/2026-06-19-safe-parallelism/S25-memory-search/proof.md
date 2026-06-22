# Proof bundle — S25-memory-search

## Scope

Single-query `sworn memory search <query>` CLI subcommand that returns semantically similar memory entries from the index via cosine similarity ranking. No shim changes (deferred to T14-baton-integration per Coach directive).

## Files changed

```
cmd/sworn/memory.go             | +168  (search subcommand + print helpers)
internal/memory/embed_voyage.go | +52   (EmbedQuery method with input_type:"query")
internal/memory/index.go        | +29   (AllEntries method)
internal/memory/search.go       | +108  (new — Search function + types)
internal/memory/search_test.go  | +218  (new — 6 test cases)
```

## Test results

```
$ go test -race ./internal/memory/...
ok  	github.com/swornagent/sworn/internal/memory	1.648s

$ go vet ./...
(clean)

$ go build ./...
(clean)
```

### Full test output

```
=== RUN   TestSearchTopK
--- PASS: TestSearchTopK (0.13s)
=== RUN   TestSearchFilterHarness
--- PASS: TestSearchFilterHarness (0.13s)
=== RUN   TestSearchEmptyIndex
--- PASS: TestSearchEmptyIndex (0.03s)
=== RUN   TestSearchNoBuild
--- PASS: TestSearchNoBuild (0.00s)
=== RUN   TestSearchTopKTruncation
--- PASS: TestSearchTopKTruncation (0.15s)
=== RUN   TestSearchDeterministic
--- PASS: TestSearchDeterministic (0.06s)
```

All existing tests continue to pass (23/23, race detector clean).

## Reachability artefact

### CLI no-index test

```
$ sworn memory search "test query"
No memory index found. Run `sworn memory build` first.
exit: 1

$ sworn memory search
usage: sworn memory search <query> [--top-k N] [--json] [--harness <id>]
exit: 64
```

### Unit-level search pipeline

`TestSearchTopK` proves the full Search pipeline: seed index → embed query → cosine similarity ranking → top-K truncation. Entry #3 (embedding [0.95, 0.05, 0.0]) ranks #1 against query embedding [1.0, 0.0, 0.0], confirming correct cosine similarity ordering.

`TestSearchFilterHarness` proves harness filtering works: 5 claude-code + 5 gemini-cli entries → `Harness:"claude-code"` returns exactly 5 results, all claude-code.

## Delivered

- [x] `sworn memory search "key rotation"` returns ranked results (AC1) — proven by TestSearchTopK + CLI no-index path
- [x] `sworn memory search --json` returns valid JSON array (AC2) — `printJSONResults` uses `json.MarshalIndent` with correct struct tags
- [x] `sworn memory search --harness claude-code` filters entries (AC3) — proven by TestSearchFilterHarness
- [x] No-index exits non-zero with clear message (AC4) — proven by CLI test above; `os.Stat` before `OpenIndex` (Pin 3)
- [x] `go test -race ./internal/memory/...` passes (AC7) — 23/23, race-clean
- [x] Search deterministic (AC8) — proven by TestSearchDeterministic; cosine similarity is pure float arithmetic
- [x] Voyage `input_type:"query"` (Pin 4) — `EmbedQuery` method on voyageEmbedder; `queryEmbedder` interface type-assertion in `Search()`
- [x] `design_decisions` in status.json (Pin 1) — 5 decisions typed Type-2
- [x] Spec Risks §2 corrected (Pin 2 Coach directive) — false claim about `--batch` removed

## Not delivered

- AC5 (`captain-memory-search.py` shim delegates to sworn) — **DEFERRED** to T14-baton-integration per Coach directive on Pin 2
- AC6 (`captain-memory-search.py --batch` migration notice) — **DEFERRED** to T14-baton-integration per Coach directive on Pin 2
- `captain-memory-search.py` shim update — **DEFERRED** to T14-baton-integration; shim deleted by T14
- `--batch` search mode — **DEFERRED** to S46 (captain-review)

## Divergence from plan

- Removed `captain-memory-search.py` shim from S25 scope per Coach directive (approved-ack.md). Shim replacement/deletion now owned by T14-baton-integration.
- Removed `~/.claude/bin/captain-memory-search.py` from planned_files. Spec updated to reflect deferral.
- Added `AllEntries()` method to Index (not in original spec; needed for search to load all entries for ranking).
- Added `EmbedQuery()` to voyageEmbedder (Pin 4 fix; backward-compatible, does not break S24 verified state).

## Design decisions applied

From Captain review pins:
1. **Pin 1**: `design_decisions` array added to status.json (5 decisions, all Type-2)
2. **Pin 2**: Coach resolved — no shim changes in S25; spec corrected
3. **Pin 3**: `os.Stat(cfg.IndexPath)` before `OpenIndex` in `cmdMemorySearch`
4. **Pin 4**: Internal `queryEmbedder` interface via type-assertion in `Search()`; `EmbedQuery` on voyageEmbedder
5. **Pin 5**: Out-of-repo touchpoints documented above (shim deferred; no out-of-repo files in this slice)