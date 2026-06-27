---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S31-lint-symbols`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest fix §3a #3 ("grep the symbol", theme T-C,
~25 rows) from the trial-log analysis (`2026-06-21-captain-trial-log-harvest.md`).
Designs name a function/field/constant/table that does not exist or is the wrong one —
a guaranteed compile error or empty query if shipped. Evidence rows: `S04b-tui-live`
(`started_at` in the wrong table), `S30-fullstate-journey-snapshot` (wrong constant/
function names), `S05-drift-api` re-review (`LoadEnvelopeByID` did not exist),
`S16-other-asset-change-rate-engine` (`Calculate` vs `CalculateFIRE`).

**Rationale:** extract back-ticked identifiers from the design and grep them against the
live codebase; unresolved → advisory warning. Advisory (not hard fail) because the lint
cannot distinguish a symbol the slice introduces from a typo.

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`); shares
the `internal/lint` package with S29/S30, serialised within T12.

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-06-29 — VERDICT: PASS

**Verifier**: fresh-context session, artefact-only inputs (Rule 7 compliant).
**Verdict**: PASS — all six gates satisfied.

- **Gate 1 (User-reachable outcome)**: PASS. `sworn lint symbols` dispatches
  through `cmd/sworn/lint.go` → `cmdLintSymbols` → `lint.CheckSymbols`. CLI
  invocation confirmed (exit 3 with unresolved symbols on stderr).
- **Gate 2 (Planned touchpoints match actual)**: PASS. Planned:
  `internal/lint/symbols.go`, `internal/lint/symbols_test.go`, `cmd/sworn/lint.go`.
  Actual (impl commit `acd8b1d`): exactly those three + `status.json` (doc update).
  Memory files in the broader diff are from the T8-memory forward-merge, not this
  slice.
- **Gate 3 (Required tests exist and pass)**: PASS. 9 test functions in
  `symbols_test.go`, all PASS. `go build ./...` and `go vet ./internal/lint/...`
  clean.
- **Gate 4 (Reachability artefact)**: PASS. CLI invocation `sworn lint symbols
  S31-lint-symbols 2026-06-19-safe-parallelism` produces exit 3 with unresolved
  symbols on stderr.
- **Gate 5 (No silent deferrals)**: PASS. Zero dark-code markers
  (TODO/FIXME/deferred/placeholder) in changed Go files.
- **Gate 6 (Claimed scope matches implemented)**: PASS. All five Delivered items
  map to spec acceptance checks with verifiable evidence: CLI invocation (exit 3),
  test functions (TestSymbolsResolvedQuiet, TestSymbolsAllResolvedExitZero,
  TestSymbolsUnresolvedWarns), build + vet clean.

**Next**: `/implement-slice S32-designfit-decisions-gate 2026-06-19-safe-parallelism` in a fresh session (next incomplete slice in T12-harness-hardening).
## 2026-06-29 — implemented

Design accepted by Coach (PROCEED, 3 pins + 2 flags). Implementation
complete.

**Decisions:**
- Exit code 3 for advisory (pin 1) — distinct from hard-fail 1 and I/O error 2.
- `CheckSymbols(sliceDir, repoRoot string) error` (pin 2) — caller passes cwd.
- `design_decisions` populated in status.json (pin 3) — 5 Type-2 entries.
- Usage strings updated to include `symbols` target (flag a).
- Comment on snake_case regex noting single-word lowercase intentional exclusion (flag b).

**Trade-offs:**
- `grep -F` (fixed string) for literal symbol matching. This means code
  snippets like `func CalculateFIRE() {}` inside backticks are searched
  verbatim and won't resolve against the codebase where the return type
  differs. The design accepts this: examples/illustrations in backticks are
  inherently false positives, and the advisory nature of the gate means they
  don't block.
- Clone of `grepOne` shell-out instead of in-process file-walk. Simpler to
  reason about and follows the existing `CheckDeps` pattern of shelling out
  to git.

**Tests:** 9 new test functions in `symbols_test.go`, all passing. Covers
CamelCase, dotted, snake_case, single-word-lowercase exclusion, deduplication,
no-backtick, and all-resolved exit-zero paths.

**Reachability:** `sworn lint symbols S31-lint-symbols 2026-06-19-safe-parallelism`
runs from the worktree, exits 3 with unresolved symbols (false positives from
the slice's own design examples, as expected).
**Skeptic panel:** skipped — runtime does not support subagent dispatch.

**First-pass:** 23/23 PASS.
