---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S32-designfit-decisions-gate`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest fix §3a #6 (theme T-K) from the trial-log
analysis (`2026-06-21-captain-trial-log-harvest.md`). The designfit gate
(`internal/designfit/designfit.go:126`) trivially passes a slice whose `design_decisions`
is empty/absent — so a slice doing architecturally-significant (Type-1) work bypasses the
gate entirely. Evidence row: `S23-memory-config` — Type-1 decisions D1+D3 bypassed
designfit because `design_decisions` was absent from status.json (empty trivially passes).

**Rationale:** replace the unconditional `continue` on empty `design_decisions` with a
Type-1-implied check so an empty array fails closed when the slice's design implies
architecturally-significant work, while leaving the benign empty case passing.

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`). Touches
`internal/designfit/` only — disjoint from the `internal/lint` slices and `captain.md`.

## Open questions

- The exact "design implies Type-1 work" signal is left to the implementer to choose
  against the live `internal/designfit` + `internal/state` API (see spec Touchpoint note),
  to avoid inventing a new status.json field if an existing signal suffices.

## Deferrals surfaced

None.

## Verifier verdicts received

None yet.

## 2026-06-22 — implemented

**State transition:** design_review → in_progress → implemented.

**Coach acks:** Coach approved design (PROCEED) via `approved-ack.md` with 2 mechanical pins + 1 optional flag. Both pins addressed inline:
1. **Pin 1 (prefix set rationale):** `impliesType1Work()` doc comment documents why `{cmd/sworn/, internal/state/, internal/verdict/}` is the intended scope — CLI entrypoint, state machine schema, verdict contract are the artefact surface external consumers depend on. Other `internal/` packages (run, scheduler, verify, supervisor, etc.) are implementation detail. Audited full `internal/` directory listing (27 packages); no other package belongs in the set.
2. **Pin 2 (D1 rationale gap):** `impliesType1Work()` doc comment now includes "When design_decisions is empty, DesignDecision.ArchitecturallySignificant cannot be checked — planned_files prefix-matching is the correct fallback."
3. **Flag (a):** D1 recorded as Type-2 design_decision in S32's own status.json for harness completeness.

**Implementation:**
- Added `impliesType1Work(*state.Status) bool` function checking `PlannedFiles` against architecturally-significant path prefixes.
- Replaced unconditional `continue` on empty `design_decisions` with Type-1-implied check that records a violation when the slice touches `cmd/sworn/`, `internal/state/`, or `internal/verdict/`.
- Existing two checks (arch-significant-but-Type-2; Type-1-without-human-decision) unchanged.
- Added `TestType1ImpliedEmptyDecisionsFails` and `TestNoType1EmptyDecisionsPasses` — all 11 tests pass.

**Reachability:** `TestType1ImpliedEmptyDecisionsFails` exercises `designfit.Run()` with the S23-memory-config fixture shape (planned_files touching `cmd/sworn/`, empty `design_decisions`). The CLI entry point (`cmd/sworn/designfit.go`) maps `HasViolations()` → exit 1 — that wiring is unchanged.

**No deferrals.**

**No skeptic panel** — runtime does not support subagent dispatch (single-threaded API call mode without subagent primitive). Noted per implementer.md "Runtime support check."
