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