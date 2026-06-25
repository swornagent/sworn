---
title: 'S32-designfit-decisions-gate — designfit fails closed when a slice implies Type-1 work but design_decisions is empty'
description: 'Today the designfit gate (internal/designfit) trivially passes a slice whose status.json design_decisions array is empty/absent — so a slice that does architecturally-significant (Type-1) work bypasses the gate entirely. Extend the gate to fail closed when a slice''s design implies Type-1 work but design_decisions is empty/absent. Harvested from the trial-log analysis §3a #6 (theme T-K); evidence S23-memory-config.'
---

# Slice: `S32-designfit-decisions-gate`

## User outcome

A release where a slice does architecturally-significant (Type-1) work but records **no**
`design_decisions` in its `status.json` now **fails** the designfit gate instead of
silently passing. This closes the bypass where an empty/absent `design_decisions` array
trivially clears the gate — the exact hole that let `S23-memory-config`'s Type-1
decisions (D1+D3) skip designfit.

## Entry point

`sworn designfit <release>` (CLI) → `internal/designfit.Run()`. Verifiable by: a unit
test on `designfit.Run()` over a fixture release with a slice whose design implies Type-1
work but whose `design_decisions` is empty → a violation is recorded and the report has
violations (gate fails). The CLI entry (`cmd/sworn/designfit.go`) already maps a
violations report to a non-zero exit.

## Background

`internal/designfit/designfit.go:126`:

```go
if len(st.DesignDecisions) == 0 {
    // No design decisions means no design-fit gate to enforce.
    continue
}
```

This `continue` is the bypass: a slice with empty `design_decisions` is skipped wholesale,
so a Type-1 slice that simply omits the array clears the gate. The fix makes an empty
`design_decisions` a violation **when the slice's design implies Type-1 work**.

## In scope

- Extend `internal/designfit.Run()` so that, before the early `continue` on empty
  `design_decisions`, it determines whether the slice's design implies Type-1
  (architecturally-significant) work. If it does and `design_decisions` is empty/absent,
  record a violation (gate fails closed).
- A mechanism to determine "design implies Type-1 work" — e.g. the slice's spec/design
  declaring architectural significance, or a marker the planner sets. Read the
  `internal/designfit` package and `internal/state` (the `DesignDecision` / `StakeClass`
  schema at `internal/state/state.go:83`) first and match the existing API; prefer the
  least-invasive signal already available in the slice artefacts.
- A new `Violation` describing "Type-1 work implied but design_decisions empty".

## Out of scope

- Changing the existing two checks (architecturally-significant-but-Type-2;
  Type-1-without-human-decision) — those stay as-is.
- Inferring Type-1-ness from arbitrary prose heuristics if a cleaner explicit signal
  exists in the artefacts; pick the explicit signal.
- The `sworn lint` family (S29–S31) — unrelated gate.

## Planned touchpoints

- `internal/designfit/designfit.go` (extend `Run()` — replace the unconditional empty
  `continue` with a Type-1-implied check)
- `internal/designfit/designfit_test.go` (new cases)

> **Touchpoint note:** verify the exact "design implies Type-1" signal against the live
> `internal/designfit` + `internal/state` packages before implementing — the spec
> intentionally leaves the signal choice to the implementer to match the existing API
> rather than inventing a new field.

## Acceptance checks

- [ ] `designfit.Run()` over a release with a slice that implies Type-1 work but has
  empty/absent `design_decisions` records a violation (the report `HasViolations()`)
- [ ] `designfit.Run()` over a release with a slice that has **no** Type-1-implied work
  and empty `design_decisions` records **no** violation (the benign empty case still passes)
- [ ] the two existing checks (arch-significant-but-Type-2; Type-1-without-human-decision)
  continue to behave as before
- [ ] `sworn designfit <release>` exits non-zero when the new violation is present
- [ ] `go build ./...` and `go vet ./internal/designfit/...` pass

## Required tests

- **Unit** `internal/designfit/designfit_test.go`:
  - `TestType1ImpliedEmptyDecisionsFails`: slice implies Type-1 work, `design_decisions`
    empty → violation
  - `TestNoType1EmptyDecisionsPasses`: no Type-1-implied work, empty `design_decisions`
    → no violation (benign empty still passes)
  - existing designfit tests continue to pass unchanged
- **Reachability artefact**: run `sworn designfit` against a fixture release reproducing
  the `S23-memory-config` shape (Type-1 work, empty `design_decisions`); capture the
  non-zero exit. Document in `proof.md`.

## Risks

- If "implies Type-1 work" is inferred too loosely, benign slices with legitimately empty
  `design_decisions` would falsely fail. Mitigation: drive the determination from an
  explicit artefact signal (see `internal/designfit/designfit.go:126` and the
  `DesignDecision`/`StakeClass` schema at `internal/state/state.go:83`), not a broad prose
  heuristic; the benign-empty test pins that boundary.

## Deferrals allowed?

None.
