# Journal — S23-memory-config

## 2026-06-27 — Implementation session start

**State transition:** `design_review` → `in_progress` (Coach ack received via approved-ack.md)

### Coach pins applied (from approved-ack.md)

1. **Path-encoding test** — `TestEncodeProjectPath` added to `config_test.go` per Spec Risk 1 mitigation.
2. **design_decisions** — Populated status.json with 5 decisions (D1, D3 Type-1 with human_decision citing approved-ack.md; D2, D4, D5 Type-2).
3. **memory_test.go** — Added `cmd/sworn/memory_test.go` to `planned_files`. CLI integration test lives there (was referenced in design.md §5 but missing from planned_files).
4. **Cross-track main.go merge** — Added deferral to `open_deferrals` noting `cmd/sworn/main.go` is a 3-way additive merge touchpoint with T3 (S06a) and T4 (S08a).

### Design decisions (re-stated)

See `status.json.design_decisions`.

- D1 (Type-1): Config arrays replaced, not appended
- D2 (Type-2): Path encoding `/` → `-`
- D3 (Type-1): Embedding config in memory.json
- D4 (Type-2): Existence as boolean
- D5 (Type-2): API key sentinel

### Files to create

- `internal/memory/config.go` — MemoryConfig struct, Load(), Defaults(), EncodeProjectPath()
- `internal/memory/harness.go` — KnownHarness, ListHarnesses(), HarnessMemoryPath()
- `internal/memory/config_test.go` — unit tests per spec ACs
- `cmd/sworn/memory.go` — `sworn memory status` CLI
- `cmd/sworn/memory_test.go` — CLI integration test

### Files to edit

- `cmd/sworn/main.go` — additive dispatch for `memory` subcommand
- `docs/release/2026-06-19-safe-parallelism/S23-memory-config/status.json` — already updated

### Cross-track awareness

- `cmd/sworn/main.go` shared with T3 (S06a-sworn-login-auth) and T4 (S08a-mcp-transport) — additive merge only.
- `internal/memory/` package is exclusive to T8-memory.