# Journal — S25-memory-search

## 2026-06-22 — Implementation

### State transition: design_review → in_progress → implemented

**Skeptic panel**: skipped — runtime does not support subagent dispatch.

**Coach ack received** (approved-ack.md): "clean single-query sworn memory search (no --batch), correct the false spec claim, note batching → in-process S46 and the python shim is deleted by the T-baton-integration transform (not S25 itself)."
### Captain pins addressed

1. **Pin 1 (design_decisions)**: Added `design_decisions` array to status.json with all 5 §2 decisions typed Type-2.
2. **Pin 2 (--batch migration)**: Resolved by Coach — no shim changes in S25. Spec Risks §2 corrected. AC5/AC6 marked DEFERRED.
3. **Pin 3 (no-index detection)**: Added `os.Stat(cfg.IndexPath)` before `OpenIndex` in `cmdMemorySearch`. File absent → print error message → exit 1.
4. **Pin 4 (Voyage input_type)**: Added `EmbedQuery(ctx, query string) ([]float32, error)` to voyageEmbedder with `InputType: "query"`. Internal `queryEmbedder` interface type-asserted in `Search()` — backward-compatible, does not break S24.
5. **Pin 5 (shim in proof.md)**: Shim deferred to T14; documented in proof.md "Not delivered" with Rule 2 cards.

### Implementation decisions

- **Search algorithm**: Linear scan over all entries loaded from SQLite via `AllEntries()`. Cosine similarity ranking, sort descending, top-K truncation. Fast for <50K entries.
- **AllEntries()**: Added to Index to support search. Loads all entries with decoded embeddings in one query.
- **queryEmbedder interface**: Un-exported internal interface; only voyageEmbedder implements it. OAI-compat and Ollama fall through to `Embed([]string{query})` — no interface change needed.
- **CLI output**: Human-readable table (rank, score, harness, title, 120-char content preview); JSON output via `--json` flag with `json.MarshalIndent`.
- **No shim changes**: Per Coach directive, `captain-memory-search.py` untouched. T14-baton-integration owns shim deletion.

### Test coverage

6 new test cases in `internal/memory/search_test.go`:
- TestSearchTopK — 10 entries, entry #3 closest to query, ranks #1
- TestSearchFilterHarness — mixed harness index, filter returns only matching
- TestSearchEmptyIndex — empty index returns empty slice, no error
- TestSearchNoBuild — validates os.Stat sentinel for CLI no-index check
- TestSearchTopKTruncation — top-K=3 from 10 entries returns exactly 3
- TestSearchDeterministic — same query/index twice → identical results

All 23 tests pass with race detector. `go vet` clean.

### Reachability

- CLI no-index path: `sworn memory search "test query"` → exits 1 with "No memory index found"
- CLI usage: `sworn memory search` (no args) → exits 64 with usage text
- Unit: `TestSearchTopK` confirms cosine similarity ranking works correctly
- Unit: `TestSearchFilterHarness` confirms harness filtering

### Deferrals (Rule 2)

| Item | Why | Tracking | Acknowledged |
|------|-----|----------|--------------|
| captain-memory-search.py shim | Coach directive; deleted by T14 | S46 + T14-baton-integration | Coach, 2026-06-22 |
| --batch search mode | Coach directive; lands in S46 | S46 (captain-review) | Coach, 2026-06-22 |
| ANN/FAISS | Post-R3 | Future release | Spec-blessed |
| Reranking | Post-R3 | Future release | Spec-blessed |