# Journal — S26-eval-projections

## Session 2026-07-28 — Implementation

### State transition: design_review → in_progress → implemented

**Design review acknowledged.** review.md had DECISION: PROCEED with 5 apply-inline pins. All addressed:

1. **Pin 1 (DB-path drop confirmed):** Verified the supervisor events table schema is `(track_id, release, event, detail, ts)` — no token/duration/cost columns. Report reads status.json exclusively. Confirmed intentional.
2. **Pin 2 (Fumadocs citation fixed):** Changed `board.go §49-56` → `internal/board/oracle.go:132,381,516` in design.md. Fumadocs prefix treated as out-of-scope (both report and ledger.go only handle `docs/release/`).
3. **Pin 3 (Test fixture pattern reused):** Tests use `t.TempDir()` + `mustMkdir` + `mustWrite` — same pattern as board_test.go:13-61. Root resolution uses `findRepoRoot()` (matching ledger.go), not `os.Getwd()`.
4. **Pin 4 (Ledger reuse audited):** Stated explicitly in telemetryReport doc comment: aggregation is separate from ledger.Project. ledger.Project builds verdict-line corpus entries (pass-rate, cost per verdict); this report computes dispatch-level metrics (rework rate, mean tokens, mean duration) — a different aggregation axis.
5. **Pin 5 (Help text updated):** Updated doc comment and both usage strings from `on|off|status|events` → `on|off|status|events|report`.

### Reviewer flags addressed

- (a) Choice #4 extends zero-exclusion to input_tokens/output_tokens beyond AC4's duration-only mandate → confirmed intentional (prevents skew from pre-S24 dispatches).
- (b) S02 (T1, planned) also touches telemetry.go's switch → trivial conflict, resolves at merge-track. No action now.
- (c) No design_decisions in status.json → no Type-1 choices, no gate reads it.
- (d) AC5 test includes attempt>0 dispatch to exercise non-zero rework rate.

### Implementation notes

- Added `telemetryReport()`, `collectDispatches()`, `aggregateByModel()`, `outputTable()`, `outputJSON()`, and `modelReport` struct to `cmd/sworn/telemetry.go`.
- Walk pattern follows `cmdLedgerSync` — `filepath.Glob("docs/release/<release>/*/status.json")` using `findRepoRoot()`.
- Grouping key: `model_id_confirmed` with fallback to `model`.
- Zero-valued fields (duration_ms, input_tokens, output_tokens) excluded from means per AC4.
- `formatVal()` guards against NaN/Inf display.
- 17 tests pass across all test functions (unit + integration).

### Pre-existing issue fixed

- `reqverify_test.go` had a build break from S24: `fakeVerifier.Verify` and `errVerifier.Verify` returned 3 values but the interface now requires 5 (added `int64, int64` for input/output tokens). Mechanical fix: added `0, 0` return values. Noted here for transparency.
## Verifier verdicts received

### 2026-06-28T06:08:39Z — BLOCKED (fresh-context verifier, Rule 7)

**Verdict: BLOCKED — drift gate (Step 0.5) forward-merge conflict, route to /replan-release.**

Before any verification gate could run, the mandatory drift gate forward-merged
`release-wt/2026-06-27-conformance-foundation` into the track worktree
(`track/2026-06-27-conformance-foundation/T7-telemetry-eval`, 62 commits behind).
The merge conflicted on **code and test** files, not just docs:

- `internal/model/openai_responses.go`
- `internal/verify/verify.go`
- `internal/verify/verify_test.go`
- `docs/release/2026-06-27-conformance-foundation/index.md` (docs-only)

Per the verifier contract, a code/test conflict on the `release-wt` forward-merge
is a track-mode invariant-2 (touchpoint-disjointness) / invariant-4 failure — the
touchpoint matrix was wrong. The merge was aborted (`git merge --abort`,
tree restored clean at `d103796`); the verifier does not resolve cross-track
semantic merges (editing production code is forbidden).

**Root cause (cross-track touchpoint collision):**
- T7 track commit `1c5bb53` (`feat(model): land S24-dispatch-enrich`) changed
  `model.Verify` → `(string, float64, int64, int64, error)` (token accounting) and
  edited `internal/verify/verify.go` + `verify_test.go`.
- Already-merged T3-agentic-verifier (`release-wt` commits `5369850` S11,
  `df48e66` S12) rewrote the same files: `Verify` is `(string, float64, error)`,
  `verify.Run` → `RunFirstPass`, and `RunAgentic` + `verifierRolePrompt` added.

The two changesets are semantically incompatible. **S26-eval-projections itself is
innocent** (scope: `cmd/sworn/telemetry.go` + tests) — but it cannot reach
`verified` while its track cannot integrate with `release-wt`.

**Proposed spec/replan amendment:** declare
`internal/model/openai_responses.go`, `internal/verify/verify.go`, and
`internal/verify/verify_test.go` as a SHARED touchpoint between
T3-agentic-verifier and T7-telemetry-eval; re-group/re-sequence so T7 rebases
onto the merged T3 work, re-applying S24's token-accounting return values on top
of T3's RunFirstPass/RunAgentic split. After the planner forward-syncs the
corrected base into the T7 track branch (conflict-free), re-run
`/verify-slice S26-eval-projections`.

**Next step:** `/replan-release 2026-06-27-conformance-foundation`

### 2026-06-28T17:30:00Z — PASS (fresh-context verifier, Rule 7)

**Verdict: PASS**

Slice: `S26-eval-projections`
Verified against: `227729d feat(telemetry): land S26-eval-projections — per-model eval report`

All seven verification gates passed:

1. **Gate 1 — User-reachable outcome exists**: `sworn telemetry report --release <name>` wired via `commands.go:89` → `cmdTelemetry` → `telemetryReport`, builds and exits 0.
2. **Gate 2 — Planned touchpoints match actual**: `cmd/sworn/telemetry.go` matched plan; `telemetry_test.go` expected (required tests); `reqverify_test.go` mechanical fix documented in divergence.
3. **Gate 3 — Required tests exist and exercise integration point**: 17 tests pass across `TestTelemetryReportIntegration`, `TestAggregateByModel_*`, `TestCollectDispatches_IntegratesWithTempDir`, `TestFormatVal`, `TestOutputTable`, `TestTelemetryReportNoDispatches`, `TestTelemetryReport_ReworkRateFromAttempt`.
4. **Gate 3b — AC satisfaction**: All 5 acceptance criteria verified — table output (AC2), JSON output (AC3), zero-duration exclusion (AC4), two-slice rework+token test (AC5), compile+exit (AC1).
5. **Gate 4 — Reachability artefact**: `sworn telemetry report --release 2026-06-27-conformance-foundation` exits 0 (both table and --json paths).
6. **Gate 5 — No silent deferrals**: Zero TODOs/FIXMEs/placeholders in production or test code.
7. **Gate 6 — Design conformance**: No design-fidelity.json (CLI project) — auto-pass.
8. **Gate 7 — Claimed scope matches implemented**: All 7 delivered items verified with live evidence.

Verifier session: fresh, artefact-only, Rule 7 compliant.