# Journal — S05-overclaim-benchmark

## Session: 2026-06-22 (Implementer)

### State transition: design_review → in_progress

Coach approved design TL;DR via `approved-ack.md` with 5 mechanical pins and 3 flags. All addressed inline:

**Pin 1 — SQLite DB init.** Before calling `RunParallel`, the benchmark opens an in-memory SQLite DB (`sql.Open("sqlite", ":memory:")`) and runs the same CREATE TABLE statements as `parallel_test.go` (tracks + events tables). Pattern copied from `internal/run/parallel_test.go`.

**Pin 2 — Pre-create per-track temp dirs.** The fixture generator creates a temp dir for EACH track's `worktree_path` entry in index.md frontmatter, not just the release worktree dir. This prevents `scheduler.RunTrack` from trying `git worktree add` (which would fail — no `release-wt/<fixture>` branch exists).

**Pin 3 — 5× repetition + `runs` field.** The harness runs 5 iterations per N value. The result struct includes a `runs` field. Since mocks are deterministic, all 5 runs produce identical results; the repetition confirms no non-determinism was introduced.

**Pin 4 — Mock counter race safety.** The mock RunSliceFn records per-slice results via a `sync.Mutex`-protected slice. Results are accumulated after `wg.Wait()` completes (i.e., after `RunParallel` returns). Verified with `go test -race`.

**Pin 5 — Spec Risk mitigation ack.** The mock's independence from spec content and worktree paths makes the "similar spec content, same worktree root" fixture constraint inapplicable — accepted because the mock tests scheduler correctness, not verifier content-sensitivity.

**Flags:**
- (a) Function named `RunOverclaimBenchmark`, not `Run`, to avoid conflict with existing `bench.Run`.
- (b) `--publish` output committed as part of the slice diff.
- (c) `design_decisions` field not added to status.json (vacuous for now; S32 parity is out of scope for this slice).

### Design decisions carried forward

- D1: Mock RunSliceFn always returns nil (PASS). Overclaim/underclaim computed from recorded ground truth + simulated verifier verdict, not from the return value.
- D2: Fixture slices distributed evenly across N tracks. 12 slices / N tracks = slices per track (12, 6, 3 for N=1, 2, 4).
- D3: Ground truth stored in status.json `owner` field ("PASS" or "FAIL").
- D4: Overclaim rate = overclaims / total slices (not / FAIL slices). Spec is explicit: 4/12 = 33.3%.
- D5: `--publish` writes the file but does not auto-commit. The implementer session commits the artefact.
### State transition: in_progress → implemented

All 6 acceptance checks delivered:
- AC1: `sworn bench overclaim` runs to completion without live API calls (all mock).
- AC2: Output includes a table with rows for N=1, N=2, N=4 with overclaim/underclaim counts and rates.
- AC3: Overclaim rate is 0% at N=1, N=2, N=4 on the deterministic fixture.
- AC4: Running `sworn bench overclaim` 5× produces identical output (md5sum verified).
- AC5: `sworn bench overclaim --publish` writes valid Markdown to `docs/benchmark/overclaim-concurrent-1to4.md`.
- AC6: `go test ./internal/bench/...` covers overclaim rate calculation (7 tests, all pass).

Test results: `go test ./internal/bench/...` PASS (0.406s). `go test -race` PASS (2.324s). `go vet` clean. `gofmt` clean.

First-pass `release-verify.sh`: PASS (23/23 checks green).

No deferrals. No divergences from plan. Skeptic panel skipped (runtime does not support subagent dispatch in this session).

## Verifier verdicts received

### 2026-06-28 — Verifier verdict: PASS

- **Actor**: verifier (`/verify-slice`)
- **Fresh context**: yes (Rule 7 compliant, artefact-only inputs)
- **Verdict**: PASS — All six gates passed.
  - Gate 1: Entry point `sworn bench overclaim` wired through `cmd/sworn/main.go` → `cmdBench` → `cmdBenchOverclaim` → `bench.RunOverclaimBenchmark`.
  - Gate 2: All 4 planned touchpoints present in implementer diff. Drift-merge noise (S10/S44 spec.md, index.md) correctly identified as expected exception.
  - Gate 3: `go test ./internal/bench/...` PASS (0.423s), `go test -race` PASS (2.176s), `go vet` clean. All 12 tests pass.
  - Gate 4: Reachability artefact `docs/benchmark/overclaim-concurrent-1to4.md` exists (936 bytes). Determinism confirmed — 5× identical MD5 `60e3079241c07430ae5282901669e987`.
  - Gate 5: No TODOs/FIXMEs/deferred/placeholders in changed source files.
  - Gate 6: All 6 ACs delivered with concrete evidence references. `--publish` writes correct output.
- **Verified against**: commit `bb24fdd`
- **Next step**: `/implement-slice S34-tui-merge-actor 2026-06-19-safe-parallelism` in a fresh session (next incomplete slice in T2-monitoring).