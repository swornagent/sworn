# Journal — S10-conformance-sit

## 2026-07-10 — resumed dispatch (credit-exhaustion continuation)

### Continuation handshake (regenerated from live repo state)

This was a RESUMED implementer dispatch. Before new work I reconciled the track
branch `track/2026-06-28-driver-contract/T6-proof` against the prior session:

- `git log 97dc24b0..HEAD` (start_commit..HEAD) showed the already-landed work:
  - `be5b9a2` — drivertest conformance suite + fail-closed registry enrolment (AC-01/AC-02).
  - `108c945` — WIP: RunSlice verified-path did not commit its status write (the sworn#93 fix).
  - `b80c7ad` + `15c1808` — the sworn#93 spec-fold replan (spec amendment + AC-06), forward-merged.
  - `fec832a` — Coach supplementary ack.
- Verified `108c945` is present and correct in `internal/run/slice.go`: the PASS
  branch now `repo.Stage(statusPath)` + `repo.Commit("chore(run): slice
  verified — verdict consumed by state machine")` after the verified transition,
  with a comment tying it to the parallel router's committed-ref re-read. Did
  NOT rewrite it (per dispatch instruction).
- Missing (the remaining work): `internal/run/loop_sit_test.go` +
  `internal/run/testdata/sit-fixture/` (AC-03/04/05/06).

### How the SIT is wired (AC-03) — "not a mocked leaf"

`TestLoopSIT` boots the REAL `RunParallel -> RunTrack -> RunSlice` path. Only
the model transport is stubbed:

- `RunSliceFn` is the real `run.RunSlice`, wired exactly like `cmd/sworn/run.go`
  wires production but with an offline registry holding the exported
  `drivertest.StubDriver` (design D4 — the SIT's dispatched driver IS the
  conformance-certified stub, so it can never silently diverge from what AC-01/02
  certified).
- `Router` is left nil, so `RunParallel` auto-constructs the PRODUCTION router
  (`board.NewOracleReaderAdapterFromRepo` + `productionSliceRouter`). This is
  load-bearing: the production router re-reads COMMITTED track-ref state via git
  — the exact reader whose blindness sworn#93 exploited.
- `MergeTrackFn` nil (D7): AC-03's boundary is "at least one slice reaches
  verified", not "track merged". The router's merge decision pauses the track.
- The fixture's `release_worktree_path` points at a NON-existent temp path so
  `RunParallel`/`RunTrack` genuinely `git worktree add` the release+track
  worktrees (AC-03 "worktrees materialise").

### Key mechanism (how the DoR gate shaped the fixture)

`implement.Run` runs a Definition-of-Ready gate on the `design_review ->
in_progress` transition (RTM + reqverify + reqvalidate). The happy-path unit
tests skip it only because their fake captain output lacks §1–§6, so
`design.Generate` fails and the whole design gate is deferred. My stub returns a
proper six-section design, which correctly drives the slice THROUGH
design_review and triggers the DoR gate — so the fixture must be DoR-complete
(pin 2). First pass I had not scripted the reqverify leg, and attempt 1 failed
"missing ## RESULTS section", recovering only via the retry-reset that forces
`in_progress` and bypasses the gate. I then made it clean and pin-2-faithful:

- intake.md: need `N-01:` (the RTM need-decl regex rejects `**bold**` and
  em-dash separators — must be `N-01:` with a colon) + a `## Release goal`.
- spec.md ACs each cite `(N-01)`; a `## Required tests` bullet gives the slice a
  linked test (else `orphaned_ac_no_test`).
- status.json carries a human-ratified `validation` record (positive + negative
  scenarios + benefit hypothesis) so reqvalidate passes.
- The stub's captain handler branches on the system prompt: the reqverify
  "requirements quality gate" prompt gets a `## RESULTS` section grading every AC
  (scanned from the payload) `PASS`; design/review get the six-section text.

Result: attempt 1 now goes straight from captain-review to verifying — no DoR
failure, no retry-bypass.

### AC-06 assertion + non-tautology (teeth) demonstration

The SIT reads the status.json COMMITTED on the track ref via `git show
track/.../status.json` and asserts `state == verified`. It deliberately does NOT
read the worktree file, which is `verified` in BOTH the fixed and the buggy case
— reading it would be tautological. The fixture starts the slice at `implemented`
(the exact sworn#93 shape), so the first router decision is `verify`.

Teeth demo (proving the assertion has teeth):

1. Temporarily reverted the verified-path `repo.Stage`/`repo.Commit` in
   `internal/run/slice.go` and re-ran `TestLoopSIT` (deadline temporarily
   shortened to 8s):
   ```
   --- FAIL: TestLoopSIT (8.15s)
       loop_sit_test.go: AC-04: SIT loop STALLED — RunParallel did not return within 8s.
       The committed track ref never advanced to a terminal state, so the router
       re-dispatched forever (sworn#93 verified-commit regression).
       $ git show track/sit-fixture/T1-sit:.../status.json
         "state": "implemented",
   ```
   The router logged `S01-sit-slice -> verify` dozens of times (the spin) until
   the bounded deadline. Committed ref stuck at `implemented`.
2. Restored `internal/run/slice.go` via `git checkout -- internal/run/slice.go`
   (it was already committed at `108c945`; the restore is byte-exact — verified
   the commit line is back and `git status` is clean) and restored the deadline
   to 30s. `TestLoopSIT` passes again (0.29s).

This is AC-06's non-tautology proof: with the fix the committed ref is
`verified` and the loop terminates; without it the committed ref is stuck at
`implemented` and the loop stalls to the bounded deadline.

### Decisions

- **Type-1 (recorded in status.json design_decisions[7])**: the sworn#93
  verified-path commit fix is FOLDED into S10, not routed to owning slice S06
  (merged/immutable) nor cut as a separate slice, because AC-06's SIT cannot
  reach a stable committed `verified` state without it — the bug blocks this
  slice's own acceptance. Human decision: Brad (Coach), captain-proceed.md
  "Supplementary Coach decision — sworn#93 fold". Scope ceiling: exactly the
  single verified-path commit.
- Fixture starts at `implemented` (Type-2 default): the faithful sworn#93 shape;
  `planned` would spin identically but the first route would be `implement`.
- SIT deadline 30s (Type-2): the loop terminates in <1s with the fix; the
  ceiling exists only so a revert stalls to a bounded deadline (AC-04) rather
  than hanging CI.

### Verification

- Slice-scoped: `go test ./internal/driver/... ./internal/run/...` — all ok.
- Full suite: `go test -count=1 -timeout 300s ./...` — 47 packages ok, 0 FAIL.
- `gofmt -l` clean, `go vet ./internal/run/ ./internal/driver/...` clean,
  newline-eating-corruption grep clean on the changed .go files.

State transition: `in_progress -> implemented`. Handing off to a fresh-context
`/verify-slice S10-conformance-sit 2026-06-28-driver-contract` (Rule 7).
