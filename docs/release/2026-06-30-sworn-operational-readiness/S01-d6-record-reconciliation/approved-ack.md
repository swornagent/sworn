# Coach acknowledgement — S01-d6-record-reconciliation

The Captain's design review (`review.md`) returned `DECISION: NEEDS_COACH` with two
items only the Coach could resolve (Rule 9 + a scope/coherence judgement). Both are now
resolved; this marker records the Coach's PROCEED so implementation may resume.

DECISION: PROCEED
ACKNOWLEDGED_BY: Brad (Coach)
DATE: 2026-07-01

## Resolution of the two NEEDS_COACH items

- **Pin 2 — D1 Type-1 ratification (DONE).** Coach ratified the carrier representation:
  `Deferral`/`Violation` structs + `Extra` overflow map + custom `(Un)MarshalJSON`.
  Recorded in `status.json` `design_decisions[0].human_decision`; `sworn designfit` PASSES.

- **Pin 1 — write-back validation gap (RESOLVED, Option A).** Coach ratified reconciling the
  schema rather than read-only. Grounding: 127 real fired deferrals use `acknowledged_by`,
  none carry the schema-required `acknowledgement`, so `state.Write` validation (not just read)
  fails on real data. Landed via `/replan-release` as **AC-10** (relax `slice-status-v1`
  `open_deferrals` required-set to `anyOf[acknowledgement, acknowledged_by]`, preserving Rule 2
  intent; negative test keeps a no-ack-key deferral failing closed). Upstream baton mirror
  tracked as **#38**.

## Implementer carries these into in_progress (Captain pins 3–6 + flags)

- Byte-stable round-trip: AC-02 fixture asserts identical bytes on read→write→read (map-based
  `MarshalJSON`, sorted keys) — phantom diffs break the drift gate.
- Compile-thread the new types: `slice.go:712/718` (via `violationsFromStrings`),
  `tools_ops.go:601`, `tools_plan.go:70` (`[]Deferral{}`), and `verify.Input.OpenDeferrals`
  through `RunFirstPass`/`CheckBoundaryMocks`/`isDeclared`. `go build ./...` is the first gate.
- Edit-corruption guard: `grep -rnE '//.*\t+(return|[a-z]+\()'` on touched files; satisfy AC-09
  with a FULL `go test ./...` + per-package timeout (a hung test is the signature) — do not
  trust the in-loop judge.
- Update the now-stale `internal/verdict/verdict.go:42` `// Kept as []string ...` comment inline.
- Confirm no `switch result` defaults `inconclusive` into pass (AC-07 is state-gated, safe).
- Oracle `blockedReason` reads `ViolationStrings()[0]` — confirm the projected string is acceptable.
- Grep-confirm the not-touched report types don't alias `state.Verification.Violations`.

Gate satisfied: slice may transition `design_review → in_progress` on the next `/implement-slice`.
