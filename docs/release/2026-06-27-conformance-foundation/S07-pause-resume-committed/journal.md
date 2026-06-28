# Journal — S07-pause-resume-committed

## 2026-06-28 — Replan (Planner/Coach, revision mode)

**Trigger:** Captain design review returned `DECISION: NEEDS_COACH` on design commit 926d66b. Two critical pins required Coach/spec authority before code was safe.

**Diagnosis (verified against live code):**
- Pin 2 (confirmed): the original spec premise — "`findFirstNonTerminal` reads the working-tree copy" — is false. `findFirstNonTerminal` (worker.go:536) returns `slices[0]` unconditionally and reads no state. Routing already flows through committed git refs via the router + `oracle.ReadSliceStatus`. The described "dirty working-tree re-runs the wrong slice" bug is not in the frontier-selection path.
- Pin 1 (confirmed): original AC2 ("skip past `implemented`") regresses forward-only resume. The router walk (`routeVerified`, router.go:271) never returns; skipping an `implemented` slice abandons it instead of re-verifying it. The router already treats `implemented` as non-terminal (`routeImplemented`, router.go:251).
- Pin 3 (confirmed): terminal-set is defined twice in the router (router.go:307, :393) as `{verified, shipped, deferred}`; the original design introduced a third, divergent set in the scheduler.
- Untracked finding (confirmed): worker.go:232 — the all-terminal `return finishTrack(...)` is fused onto its comment line and is commented out. Dead today (seed never returns ""), but this slice's committed-read change makes "" reachable → a fully-terminal track on resume would fail to merge.

**Coach decision (Brad, 2026-06-28): replan properly** (chosen over defer / narrow-fix). Re-anchor the spec to the real behaviour: seed from committed state via the oracle; treat `implemented` as non-terminal (DD-1); unify the terminal-set in one exported `router.IsTerminal` helper (DD-2); fix the worker.go:232 fused-line bug (AC4). Original AC2 replaced.

**Artefacts updated:** spec.md (re-scoped, EARS ACs AC1-AC6, Coach decision section), status.json (design_decisions DD-1/2/3; planned_files += internal/router/router.go; test_commands updated; verification.result reset pending; state planned). Design gate artefacts (design.md/review.md/captain-proceed.md) stripped so the Design TL;DR gate re-fires fresh against the corrected spec.

**Next:** loop re-dispatches `/implement-slice S07` against the corrected spec.

## 2026-06-28 — Implementation session

**State transition:** `design_review` → `in_progress` → `implemented`.

**Design review gate:** Captain review `DECISION: PROCEED` with 5 inline pins, all addressed:
1. **Oracle wiring (CRITICAL):** Hoisted `ora` to function scope in `internal/run/parallel.go`; added `Oracle router.OracleReader` to `WorkerOptions`; wired at both construction sites (parallel.go:275 and :337 retry block).
2. **Prove wiring (CRITICAL, Rule 1):** `TestFindFirstNonTerminalCommitted` exercises the real oracle-read path through `findFirstNonTerminal`; the production integration point (`RunParallel`) hoists `ora` and sets `WorkerOptions.Oracle` — a nil `Oracle` gracefully falls back to `slices[0]`.
3. **Path citations:** Fixed design.md's wrong paths; edits are in `internal/run/parallel.go` (not `internal/scheduler/run_parallel.go`) and `SliceRouter` is in `internal/scheduler/worker.go:48` (not `model.go`).
4. **Import-cycle hedge:** Dropped the "type-alias to avoid circular import" — no cycle risk; `internal/router` imports only `board`+`git`. New `scheduler → router` edge is safe.
5. **AC3 seed-don't-skip:** `findFirstNonTerminal` seeds AT the unreadable slice on error (not skip past it). `CatFileExists` already swallows missing-ref; the seed-at behaviour covers the residual hard-error case consistently with DD-1.

**Implementation summary:**
- Added `router.IsTerminal(state string) bool` — single terminal-set `{verified, shipped, deferred}`.
- Replaced two inline terminal-set switch blocks in `router.go` with `IsTerminal` calls.
- Rewrote `findFirstNonTerminal` to accept `oracle router.OracleReader` + release/track context; reads committed state; returns first non-terminal slice or `""` if all terminal.
- Fixed worker.go:232 fused-line bug: separated `// All slices already in a terminal state.` from `return finishTrack(...)`.
- Added `--resume` flag + usage gate (`--resume` without `--parallel` exits 64) in `cmd/sworn/run.go`.
- Added 6 tests (AC1-AC5) to `internal/scheduler/worker_test.go` and `TestIsTerminal` (9 cases) to `internal/router/router_test.go`.

**Snapshot:** 7 new tests, 6 changed files, 242 insertions / 53 deletions. All existing tests pass. `sworn verify` requires API key not available in this environment.

**Next:** `/verify-slice S07-pause-resume-committed 2026-06-27-conformance-foundation`

## 2026-07-28 — Verifier verdict

### Verifier verdicts received

**PASS** — All six gates passed:

| Gate | Result | Detail |
|------|--------|--------|
| Gate 1 — User-reachable outcome | PASS | `sworn run --parallel [--resume]` → `RunParallel` → `WorkerOptions.Oracle` → `runTrackRouter` → `findFirstNonTerminal(ctx, opts.Oracle, ...)`. Wired end-to-end. |
| Gate 2 — Planned touchpoints | PASS | All three planned files changed; `internal/run/parallel.go` is necessary oracle-threading plumbing described in design.md. Test + docs files expected adjuncts. |
| Gate 3 — Required tests | PASS | 6 scheduler tests + 1 router table test (9 cases) = 7 test functions. All PASS with `go test`. Integration-point reachability via committed-state oracle mock. |
| Gate 4 — Reachability artefact | PASS | Go test output for `TestFindFirstNonTerminal*` + `TestIsTerminal` with user-gesture description referencing the `RunParallel` integration path. |
| Gate 5 — No silent deferrals | PASS | Zero TODO/FIXME/deferred/placeholder hits in production code. |
| Gate 6 — Design conformance | PASS | Non-UI project (no design-fidelity.json). |
| Gate 7 — Claimed scope | PASS | All 6 ACs mapped to deliverable evidence; all evidence references verified. |

- `go vet` clean
- `go build ./...` compiles
- `go test ./internal/scheduler/... ./internal/router/...` — all pass

**Next step:** `/verify-slice S27-parallel-dispatch-fix 2026-06-27-conformance-foundation` (next unverified slice in T1-orchestration).