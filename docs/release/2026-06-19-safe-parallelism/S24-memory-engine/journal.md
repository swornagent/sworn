# Journal — S24-memory-engine

## Init — 2026-06-29

- **State**: `planned` → `design_review`
- **Actions**: Produced design TL;DR with 6 mandatory sections.
- **Decisions**:
  - SHA256 of (path + content) for entry identity; for flat-file splits, path includes an index fragment
  - Embedding batch order MUST be preserved across split requests
  - API keys read at call time (not config load time)
  - SQLite connection opens per build, closes on completion
- **Open questions**: raised 3 in §6 of design.md (BLOB format, modernc driver path, MEMORY.md format stability)
- **Next**: await Captain review + Coach ack/decline
## 2026-06-21 Implementer Session

**State transition:** `design_review` → `in_progress` → `implemented`

**Decisions & Trade-offs:**
- Addressed all 6 Coach pins from design review inline.
- Implemented `Embedder` interface and drivers for Voyage, OAI-compat, and Ollama.
- Implemented SQLite vector index with change detection via SHA256 and cosine similarity.
- Implemented entry discovery for Claude Code (`MEMORY.md`), flat files, and custom paths.
- Added `sworn memory build` command with `--force` flag.
- Used `modernc.org/sqlite` for the vector index.
- Stored embeddings as `[]float32` in little-endian IEEE 754 binary format.
- Added tests for embedding drivers, vector index, and entry discovery.
- Generated `proof.md` and passed first-pass verification.

**Subagent dispatches:**
- None (runtime does not support subagent dispatch).

**Skeptic panel:**
- skipped — runtime does not support subagent dispatch.

## Verifier verdicts received

### 2026-06-21 — Verdict: BLOCKED (round 1)

**Verifier:** fresh-context session, artefact-only inputs (Rule 7 compliant)

**Reason:** Forward-merge of `release-wt/2026-06-19-safe-parallelism` into `track/2026-06-19-safe-parallelism/T8-memory` conflicted on `cmd/sworn/main.go`. T9-telemetry (S26, merged to release-wt) restructured `main.go` from a direct switch in `main()` to a `dispatch()` function with telemetry wrapping — a non-additive structural change. S23-memory-config (earlier slice on T8-memory) added `case "memory": os.Exit(cmdMemory(os.Args[2:]))` in the original `main()` structure. These two changes conflict and cannot be auto-merged.

**Note:** S24's own `planned_files`/`actual_files` do NOT include `cmd/sworn/main.go` — this conflict originates from S23's prior work on the track branch.

**Proposed resolution (for planner):** No spec.md change is required. The planner must resolve the `cmd/sworn/main.go` merge conflict on branch `track/2026-06-19-safe-parallelism/T8-memory`. Fix: place S23's `case "memory": return cmdMemory(args[2:])` inside T9's `dispatch()` function (changing `os.Args[2:]` → `args[2:]`), then commit the resolution. After that, re-verify S24 in a fresh session.

**Next step:** `/replan-release 2026-06-19-safe-parallelism`

## 2026-06-21 — planner cleared BLOCKED (cmd/sworn/main.go conflict resolved)

The verifier BLOCKED on a cmd/sworn/main.go forward-merge conflict (S23's
`case "memory"` in the old main() vs T9's dispatch() extraction). Planner resolved
per the verifier's proposed fix: forward-merged release-wt and ported
`case "memory": return cmdMemory(args[2:])` into T9's dispatch(). go build clean.
verification.result cleared to pending; state → implemented for re-verify.
(Same systemic conflict as T2/S04c and T3/S06a — T9's dispatch extraction vs every
track's added case; S30-lint-touchpoints will catch this class at plan time.)
