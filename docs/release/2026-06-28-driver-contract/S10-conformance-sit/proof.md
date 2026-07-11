# Proof bundle — S10-conformance-sit

Rendered from `proof.json` (proof-v1). Generated from live repo state.

## Scope

Every registered driver passes one exported behavioural conformance suite
(`internal/driver/drivertest.Run`), and the assembled `sworn` loop boots
end-to-end over a hermetic fixture release with a stub driver from a **cold
board** (`internal/run.TestLoopSIT`) — so a contract-violating driver or a dead
loop wiring fails a test in CI instead of shipping a DOA release. Folds in the
sworn#93 verified-path commit fix (`internal/run/slice.go`), whose regression
assertion is AC-06.

## Files changed

`git diff --name-only release-wt/2026-06-28-driver-contract..HEAD` (code +
fixture; the S10 planning artefacts design.md/review.md/captain-proceed.md land
too):

- `internal/driver/drivertest/conformance.go`, `conformance_test.go`, `stub.go`, `wiring.go` (AC-01/02, @be5b9a2)
- `internal/driver/conformance_all_test.go` (AC-02 fail-closed enrolment, @be5b9a2)
- `internal/run/slice.go` (sworn#93 verified-path commit, @108c945)
- `internal/run/loop_sit_test.go` (AC-03/04/05/06 SIT)
- `internal/run/testdata/sit-fixture/{board.json,index.md,intake.md,S01-sit-slice/spec.md,S01-sit-slice/reqverify.md}` (hermetic cold-board fixture)
- `docs/release/2026-06-28-driver-contract/S10-conformance-sit/status.json`

## Test results

| Command | Result |
|---|---|
| `go test ./internal/run/ -run TestLoopSIT -count=1` | PASS (exit 0) |
| `go test ./internal/driver/... ./internal/run/... -count=1` (AC-05 slice-scoped) | PASS (exit 0) |
| `go test -count=1 -timeout 300s ./...` (full suite) | PASS — 47 packages ok, 0 FAIL |
| `gofmt -l` (changed .go, empty) + `go vet ./internal/run/ ./internal/driver/...` | clean |

## Reachability artefact

`internal/run/loop_sit_test.go:TestLoopSIT` boots the **real** `RunParallel ->
RunTrack -> RunSlice` path (production router auto-constructed, re-reading
committed track-ref state via git; `RunSliceFn` is the real `run.RunSlice` wired
only with an offline stub registry — **not a mocked leaf**) over the hermetic
`sit-fixture` release from a **cold board** (release+track worktrees do not
pre-exist — `RunParallel`/`RunTrack` `git worktree add` them). It reads the
status.json **committed on the track ref** and asserts `state == verified`
(AC-06 — never the worktree file).

**Teeth demo (AC-06 non-tautology):** reverting the verified-path
`repo.Stage`/`repo.Commit` in `internal/run/slice.go` makes `TestLoopSIT`
`--- FAIL ... AC-04: SIT loop STALLED` with the committed ref stuck at
`"state": "implemented"` and the router re-dispatching `verify` dozens of times;
restoring slice.go (`git checkout`) makes it pass again. See `journal.md`.

**Gate:** the model-backed `sworn verify` could not produce a verdict here (no
provider key; keyless `claude-cli/sonnet` returns INCONCLUSIVE
`verifier_structured_unsupported`). Same environment class as verified sibling
S05. Canonical adversarial gate is the fresh-context `/verify-slice` handoff.

## Delivered

- **AC-01** exported conformance suite (`drivertest.Run(t, NewDriver)`) asserting the S01 contract clauses — success Result well-formed; error paths Status=error+ErrKind, never panic; undeclared role fails at `Registry.Resolve`; verifier StructuredJSON parses or fails closed; Rule-11 guard fires. → `internal/driver/drivertest/conformance.go`, `conformance_test.go`.
- **AC-02** suite runs over all four compiled-in drivers (fake claude/codex binaries, in-process oai/responses via httptest through the SWORN_PROXY_URL route) + the reference stub, via `conformance_all_test.go` iterating `DefaultRegistry` with fail-closed enrolment. → `internal/driver/conformance_all_test.go`, `wiring.go`.
- **AC-03** `TestLoopSIT` boots the real RunParallel over the cold-board fixture; worktrees materialise, captain/implement/verify legs dispatch through the registry, verdict consumed, slice reaches verified, no panic. → `loop_sit_test.go` + `testdata/sit-fixture/`.
- **AC-04** conformance failures name driver/clause (subtests); the SIT dumps board state on stall (exercised live in the teeth demo). → `conformance.go` subtests; `loop_sit_test.go` `sitBoardDump` + deadline branch.
- **AC-05** whole slice offline; slice-scoped + full `go test` green. → test results.
- **AC-06** SIT asserts the `verified` transition is committed to the track ref (router does not re-dispatch) and is demonstrated non-tautological. → `sitCommittedState` + teeth demo.
- **sworn#93 fix** present/correct in `slice.go` @108c945; recorded as a Type-1 design_decision citing captain-proceed.md.
- **Pin 2** fixture is DoR-complete (need N-01, ACs citing it, human-ratified validation, stub scripting design + review + reqverify PASS) so `implement.Run`'s DoR gate passes on the first pass.

## Not delivered

- Real-provider / real-CLI dispatch tests — declared `out_of_scope` (Rule-10 cutover journey owns real-infra validation). Acknowledged: Coach, spec.json out_of_scope[0].
- Performance benchmarking — declared `out_of_scope` (spec.json out_of_scope[1]).

## Divergence from plan

1. SIT terminal is a merge-**release** pause (single-track single-slice fixture ⇒ whole release terminal on the one verify), not merge-track; either way the track pauses (D7) and the committed-state assertion is unaffected.
2. AC-03/AC-06 use the transport-less `StubDriver` (D4), so disposition 3's exported-seam escape hatch was not needed; the in-process conformance legs use the landed SWORN_PROXY_URL route (no exported production seam).
3. Fixture starts the slice at `implemented` (the exact sworn#93 shape) rather than `planned`; both are valid cold-board states.
4. Fixture ships `start_commit=""` (RunSlice self-bootstraps) and a `__RELEASE_WT__` board.json token the test substitutes.
5. Model-backed gate could not run (no key / structured-output-unsupported keyless driver); no `spec.md` manufactured for the spec-v1 first-pass false-FAIL.
6. Touchpoint expansion (prior session @be5b9a2): `drivertest/stub.go` (D4 StubDriver) and `drivertest/wiring.go` (AC-02 fixture wiring) are in the diff beyond the two `drivertest` files spec.json enumerates — both within the conformance-suite package this slice owns and described in design.md. Nothing outside the slice's packages was touched.
