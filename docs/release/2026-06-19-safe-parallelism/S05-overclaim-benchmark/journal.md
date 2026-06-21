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
