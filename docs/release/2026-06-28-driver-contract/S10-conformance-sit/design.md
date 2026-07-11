# Design TL;DR — S10-conformance-sit

## User outcome

Every registered driver passes one exported behavioural conformance suite,
and the assembled `sworn loop` boots end-to-end over a fixture release with a
stub driver from a cold board — so a contract-violating driver or a dead loop
wiring fails a test in CI instead of shipping a DOA release (the 2026-06-28
three-model eval's failure mode).

## Approach

Two independent artefacts, both exercising real landed surfaces — no mocks
of the things under test:

1. **`internal/driver/drivertest`** — an exported, table-driven conformance
   suite (`Run(t *testing.T, newDriver func() driver.Driver)`) asserting the
   behavioural clauses from ADR-0012 against a `driver.Driver` value the
   caller constructs for real (fake CLI binary, `httptest` server — never a
   test double of `Driver` itself). `internal/driver/conformance_all_test.go`
   iterates `registry.Default(...).Drivers()` (via `Registry.Drivers()`,
   AC-05's dispatch-free enumeration) and calls `drivertest.Run` once per
   registered entry, plus once for a reference stub driver, so a fifth
   driver registered in a future slice is auto-enrolled with zero suite
   edits.
2. **`internal/run/loop_sit_test.go`** (`TestLoopSIT`) — boots
   `run.RunParallel` (the exact function `cmd/sworn/run.go`'s `--parallel`
   path calls) over a hermetic fixture release built in `t.TempDir()`, with
   `RunSliceFn` wired to the **real** `run.RunSlice` (not a fake — this is
   the AC-03 requirement and the gap the eval found: every existing
   `parallel_test.go` case injects `fakeRunSlicePass`, so `RunSlice` itself
   has never been driven through `RunParallel` in a test). The stub driver
   is registered in a scratch `registry.Registry` for all three roles and
   passed via `RunSliceOptions.Registry`, mirroring `cmd/sworn/run.go`'s
   `runSliceFn` closure verbatim except for the registry source.

Neither artefact touches the four production drivers' code, the registry,
or `RunParallel`/`RunSlice` themselves — this slice is pure test surface
plus one new leaf type (the stub driver), consistent with `out_of_scope`
("fixing driver bugs the suite finds").

## Key design choices + rationale

**D1 — `drivertest.Run` takes a driver *factory*, not a driver instance.**
Some clauses (missing `WorktreeRoot`, unsupported role) need a fresh,
unstarted driver per subtest so one subtest's dispatch can't leak state
(open subprocess, HTTP round count) into the next. `func() driver.Driver`
matches the constructor shape every driver already exposes
(`NewClaudeDriver`, `NewCodexDriver`, `inprocess.NewOAIChat(cfg)`,
`inprocess.NewOAIResponses(cfg)`) — callers close over their fake-binary
path or `httptest` URL. Type: 2 (narrow, local, easy to change if a clause
needs shared state later).

**D2 — Conformance clauses are driver-agnostic contract clauses (ADR-0012 +
driver.go doc comments), not per-driver behaviour.** Per spec R-02, a clause
that cannot run against all five (four registered + stub) is a contract-doc
defect, surfaced to the human, not special-cased into the suite. Concretely,
from AC-01:
  - successful dispatch → well-formed `Result` (`Status` set; `Duration`,
    `InputTokens`/`OutputTokens` non-negative; `ResultText` or
    `StructuredJSON` present per role — verifier roles get `StructuredJSON`,
    others `ResultText`);
  - every error path → `Status=error`, non-empty `ErrKind`, never panics
    (driven via each driver's existing fake-failure mode: subprocess fake
    binary exit-nonzero, `httptest` 500/malformed-JSON server, stub driver's
    injectable error hook);
  - a `Role` outside `Roles()` → error, no panic (call `Dispatch` with
    `Role: "captain"` against a driver whose `RoleSet` excludes it —
    verified statically from `d.Roles().Has(role)` before the subtest even
    dispatches, so the assertion is "declared-incapable roles error, never
    panic" rather than a hardcoded role name);
  - verifier-role dispatch → `StructuredJSON` parses as a JSON object, or
    the driver fails closed (`Status=error`) — never returns malformed JSON
    as if it were a verdict;
  - `AssertWorktree`-class fail-closed check on a missing/non-worktree
    `WorktreeRoot` fires before any work (a subprocess driver spawns nothing;
    an in-process driver either doesn't require `WorktreeRoot` — asserted
    explicitly as N/A per driver via a `RequiresWorktree bool` the test
    factory declares — or checks it the same way).

**D3 — Subtest naming is `<driver-name>/<clause-id>` (AC-04).** `t.Run` is
nested `t.Run(driverName, func(t){ t.Run(clauseID, ...) })` so `go test -run
TestConformance/codex-subprocess/AC01-error-nonzero` is directly addressable
and a CI failure names both dimensions without parsing log text.

**D4 — The reference stub driver lives in `drivertest` itself
(`drivertest.StubDriver`), exported.** It is both (a) the fifth conformance
subject proving the suite runs against a maximally-conformant, contract-only
implementation with no transport at all (catches clauses that accidentally
assert subprocess/HTTP-specific behaviour), and (b) the exact driver
`TestLoopSIT` registers for the cold-board smoke — one implementation, two
consumers, so the SIT's stub can never silently diverge from what the
conformance suite already certified as contract-compliant. `StubDriver`
supports scripted `Result`s per role (a queue or a callback) so `TestLoopSIT`
can hand it a canned PASS verdict for the verifier leg.

**D5 — `TestLoopSIT` builds its fixture release with real `git`, not a fake
filesystem tree.** `RunParallel`'s cold-start bootstrap (`parallel.go`
~line 196: "branch %s absent — creating it from HEAD") and the production
router's oracle (`board.NewOracleReaderAdapterFromRepo`, git-ref reads
against `release-wt/<release>`) are exactly the two mechanisms the eval's
nil-factory SIGSEGV and dead-router paths hid behind. A fake filesystem
tree would recreate the mock-the-thing-under-test problem the S08-drop
rationale calls out. TestMain-style hermetic setup: `git init` +
`git add`/`git commit` in `t.TempDir()`, committing `docs/release/<sit
release>/board.json`, one track (`T1`), one slice
(`S01-sit-fixture/spec.json` + `status.json` at `state: planned`) — no
`release-wt` branch, no worktrees pre-made (AC-03's "COLD board"
requirement). `opts.Router` is left nil so `RunParallel` auto-constructs the
production router (the same code path `cmd/sworn/run.go` uses), reading the
committed status off the `release-wt/<release>` ref it just self-bootstrapped
— never a fake `SliceRouter`, which is what every existing `parallel_test.go`
case injects and which is precisely the leaf-mocking this slice exists to
stop doing at the top of the stack.

**D6 — Bounded-deadline stall guard (AC-04).** `TestLoopSIT` runs
`RunParallel` in a goroutine against a `context.WithTimeout` (60s, generous
for an all-stub hermetic run but far under a CI job timeout) and selects on
completion vs. `ctx.Done()`. On timeout, before `t.Fatal`, it dumps the
fixture release's live `status.json`/`board.json` and DB `tracks`/`events`
table contents into the test log — "board state dump on stall" per AC-04 —
using the same `sqlite` in-memory DB pattern `TestRunParallel_Basic` already
uses (`db.Exec` DDL for `tracks`/`events`), so the dump is a plain `SELECT *`
against tables the test itself created.

**D7 — No `MergeTrackFn` in `TestLoopSIT`.** AC-03 only requires "at least
one slice reaches verified" — track-merge-to-release-wt is `T6`'s neighbour
concern (`ProductionMergeTrack`, exercised elsewhere) and pulling it in here
would make a stall/failure ambiguous between "the loop never verified" and
"merge itself broke". Leaving `opts.MergeTrackFn` nil (auto-skip, per its
existing doc comment) keeps the assertion surface to exactly what AC-03
names. Type: 2 — reversible, noted here rather than escalated.

## Files touched (all new; nothing existing edited)

| File | Purpose | ACs |
|---|---|---|
| `internal/driver/drivertest/conformance.go` | Exported `Run(t, newDriver, opts)` + `StubDriver` type | AC-01, AC-02 (stub half) |
| `internal/driver/drivertest/conformance_test.go` | Suite's own self-test (stub driver against itself, sanity that clauses fire/fail as expected) | AC-01 |
| `internal/driver/conformance_all_test.go` | Iterates `registry.Default(fakeCfg).Drivers()` + fake CLI/httptest wiring for the four real drivers, calls `drivertest.Run` per entry | AC-02, AC-04 |
| `internal/run/loop_sit_test.go` | `TestLoopSIT`: hermetic fixture + real `RunParallel`/`RunSlice`/registry path | AC-03, AC-04 |
| `internal/run/testdata/sit-fixture/` | Static spec.json/status.json templates the fixture-builder copies and stamps (kept as data, not inlined strings, so the fixture reads like a real slice folder) | AC-03 |

## Design-level risks / pins for the reviewer

- **R-01 (spec's own risk, restated):** hermetic git-in-tempdir + real
  `RunParallel` self-bootstrap is the exact combination the 2026-06-28 eval
  found flaky/broken. Mitigation is D5/D6 above; flagging for the reviewer
  because this is the highest-uncertainty part of the design — if
  `board.NewOracleReaderAdapterFromRepo` has an undocumented requirement
  (e.g. a specific ref shape beyond what `git commit` on a fresh repo
  produces) it surfaces here first, not in production.
- **Pin for reviewer:** D7 (no merge-track in the SIT) is a scope reading of
  AC-03's exact wording ("at least one slice reaches verified") — confirm
  that's the intended boundary, not "track reaches merged".
- **Pin for reviewer:** D4's shared `StubDriver` (one type, two call sites)
  is a Type-2 default; an alternative is two separate minimal stubs
  (conformance-only vs. SIT-only) to avoid any coupling between the two
  test suites. Chose shared because divergence between "what conformance
  certified" and "what the SIT actually dispatches" is the failure mode
  AC-03's rationale is written against.

## Divergence from spec touchpoints

None — the five touchpoints in spec.json map 1:1 to the files-touched table
above (the `sit-fixture/` touchpoint materialised as a directory of
static templates rather than a single file, which the spec's plural
`testdata/sit-fixture/` already anticipates).
