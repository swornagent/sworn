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

### 2026-06-21 — Verdict: PASS (round 3)

**Verifier:** fresh-context session, artefact-only inputs (Rule 7 compliant)
**Verified against:** `40cb8d6` (HEAD after drift-gate forward-merge)

**Gate 1 — User-reachable outcome exists:** PASS
`case "memory"` dispatches to `cmdMemory()` in `cmd/sworn/main.go:106`. `cmdMemoryBuild()` is fully implemented — discovers entries, opens index, embeds, upserts, prints summary. No stubs.

**Gate 2 — Planned touchpoints match actual diff:** PASS
All 10 planned files present in `git diff --name-only d441b4c..HEAD`. Non-planned files fully accounted for in "Divergence from plan": `cmd/sworn/main.go` (dispatch case porting, forward-merge resolution); `cmd/sworn/telemetry.go`, `internal/telemetry/*`, `internal/git/*` (S26/S28 forward-merge noise, already-verified slices); `docs/release/**` + `.gitignore` (docs-only from two release-wt syncs). Every non-planned file has a clean causal explanation.

**Gate 3 — Required tests exist and exercise the integration point:** PASS
All 18 tests pass fresh (`go clean -testcache && go test -race ./internal/memory/... -v`). Voyage batch splitting verified: 150 texts → `embeddings[128][0] == 0.0` confirms second batch started at index 0 (FAIL if single-request). Auth header and key-from-env tested. `TestUpsertAndRetrieve`, `TestChangeDetection` (row count stays 1), `TestCosine` (4 subcases), `TestEmbeddingEncoding` (round-trip BLOB). `TestDiscoverClaudeCode`, `TestDiscoverFlatFile` (`---` splitting into 2 entries, path fragments `#0`/`#1`), `TestDiscoverCustomPath`. All httptest.NewServer fakes; no live API calls.

**Gate 4 — Reachability artefact proves the user path:** PASS
proof.md shows live `./sworn memory build` output: "Indexed 3 entries (3 new, 0 unchanged) via nomic-embed-text in 129ms"; re-run "0 new, 3 unchanged" (change detection); `--force` "3 new". Consistent with spec user outcome. Provider and count present.

**Gate 5 — No silent deferrals:** PASS
`grep -rn "TODO|FIXME|deferred|later|placeholder|XXX|HACK"` over S24 implementation files returned nothing. "Not delivered" items have all three Rule 2 elements (why, tracking, acknowledgement).

**Gate 6 — Claimed scope matches implemented scope:** PASS
All four Delivered items verified against files and test names. Not-delivered items (index pruning, TUI progress) are in spec's "Deferrals allowed?" section; all carry Rule 2 cards with planner acknowledgement.

```
PASS

Slice: `S24-memory-engine`
Verified against: `40cb8d6`
Verifier session: `fresh, artefact-only`
```


### 2026-06-21 — Verdict: FAIL (round 2)

**Verifier:** fresh-context session, artefact-only inputs (Rule 7 compliant)
**Verified against:** `dfa666cef8e277de98f74125d1972823f6b79e79`

**Gate 2 — Planned touchpoints vs actual diff**

`start_commit` in `status.json` is `16c0a8b` — the coach-ack commit (`chore: coach ack — approved Captain suggested reply`), **not** the start-implementation commit (`d441b4c`, `docs(...): start implementation`).

As a result, `git diff --name-only 16c0a8b` includes 6 files not in `spec.md` "Planned touchpoints":
- `cmd/sworn/main.go` (S26-telemetry)
- `cmd/sworn/telemetry.go` (S26-telemetry)
- `internal/telemetry/telemetry.go` (S26-telemetry)
- `internal/telemetry/telemetry_test.go` (S26-telemetry)
- `internal/git/git.go` (S28-git-dir-guard)
- `internal/git/git_test.go` (S28-git-dir-guard)

`proof.md` "Divergence from plan" says "None" — this is inconsistent with the actual diff from `start_commit`.

**Gates 1, 3, 4, 5, 6 all pass.** The S24 implementation itself is correct. The failure is solely on Gate 2 due to the wrong `start_commit` boundary.

**Required fix:**
Set `start_commit` in `status.json` to `d441b4c` (the `docs(...): start implementation` commit). With the correct boundary, `git diff --name-only d441b4c..HEAD` shows only S24 planned files plus expected docs artefacts. Proof.md "Divergence from plan: None" would then be accurate.



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

## 2026-06-21 — implementer re-entry: corrected Gate 2 start_commit + honest Divergence

Round-1 FAIL was Gate 2 only (Gates 1, 3, 4, 5, 6 passed; the S24 code is unchanged).
Two corrections, no production-code change:

1. **`start_commit` 16c0a8b → d441b4c.** `16c0a8b` was the coach-ack commit; the
   start-implementation commit is `d441b4c`. This is a one-time correction of a *wrong*
   boundary, NOT a re-entry overwrite of a correct one (cf. the S02b round-2 regression
   that the implement-slice re-entry guard was added to prevent) — the verifier explicitly
   directed it.
2. **proof.md "Files changed" + "Divergence from plan" rewritten from live state.** The
   verifier's prescribed one-line fix ("set start_commit to d441b4c") was based on the
   pre-re-merge graph; it is now insufficient because two forward-merges of release-wt
   (`761b8bf`, `dfa666c`) landed *after* d441b4c, so `d441b4c..HEAD` still carries the 6
   S26/S28 code files + 46 replan/other-slice docs. Rather than chase an impossible "clean"
   boundary, proof.md now lists the S24 footprint explicitly and accounts for every
   inherited file under Divergence (main.go = dispatch resolution; the rest = forward-merge
   content from already-verified slices). The diff and the proof are now consistent — which
   is what Gate 2 actually checks.

`go test -race ./internal/memory/...` ok; `go build ./...` clean. state → implemented,
verification.result → pending. Ready for fresh-context re-verify.
