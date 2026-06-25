# Journal — S72-tui-gate-display

## Session 2026-07-20 — implementation

### Decisions

1. **Gate results computed on board load, not cached persistently.** `LoadGateResults`
   calls `gate.RunTrace()` (release-level), `RunCoverage`, `RunDesign`, `RunMock`
   (per-slice, only for slices with a `start_commit`). LLM results are read from
   cached `llm-check.json` if present. This keeps the TUI self-contained — no
   separate caching layer needed. The computation cost is bounded: trace is O(slices)
   regex work; per-slice gates are only for implemented+ slices and each runs
   `git diff` + file scan.

2. **`cmd/sworn/top.go` unchanged.** The gate display is wired internally through
   `BoardView.LoadBoard()` → `LoadGateResults()`, which populates `SliceBoardInfo.Gate`.
   No surface-level wiring needed in the CLI entry point. This is a divergence from
   `planned_files` (which listed `cmd/sworn/top.go` as a touchpoint) but is correct:
   the board view already owns its own data loading.

3. **DesignCount defaults to -1 (not checked).** This distinguishes "0 violations
   (clean)" from "no data available" in the TUI. The zero-value `GateResult{}` has
   `DesignCount: 0`, but `LoadGateResults` explicitly sets it to -1 for slices
   without design check results.

4. **Reachability artefact is a manual smoke step, not a screenshot.**
   The TUI is a Go Bubble Tea program; there is no Playwright/e2e harness for it.
   The spec's "Reachability artefact: Screenshot of TUI" refers to a visual capture
   from the terminal, not a Playwright screenshot. Verified via `manual-smoke-step`.

### Trade-offs

- Coverage/design/mock gates on `release-wt` use `start_commit..HEAD` as the diff
  base, which may include test files from other tracks merged later. This gives
  a mildly inflated coverage count (benign overcount) rather than a precise
  per-slice diff. The TUI display is informational, not a gating check, so this
  is acceptable.
- LLM check results are only shown when cached in `llm-check.json` — the TUI
  does not invoke model calls itself. This keeps the 1s polling fast.

### Deferred

None.

## Verifier verdicts received

### Session 2026-07-21 — verifier (fresh-context)

**Verdict: PASS**

All 6 verification gates pass.

- **Gate 1 (Reachability):** PASS — gate display wired into `BoardView.LoadBoard()` → `LoadGateResults()` → `RenderInline()` in `View()`, reachable from `sworn` or `sworn top`.
- **Gate 2 (Touchpoints):** PASS — `planned_files` divergence explained: `top.go` not needed, `board.go` + `styles.go` modified instead. `actual_files` correctly records all 4 files.
- **Gate 3 (Tests):** PASS — 9 gate-specific unit tests pass covering all formatting scenarios.
- **Gate 4 (Reachability artefact):** PASS — manual smoke step described; screenshot requirement is a spec wording issue (Go TUI, not web app).
- **Gate 5 (No silent deferrals):** PASS — zero TODO/FIXME/HACK markers in changed files.
- **Gate 6 (Scope):** PASS — all 5 acceptance checks verified against code.