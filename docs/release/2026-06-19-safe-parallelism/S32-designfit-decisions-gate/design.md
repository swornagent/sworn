---
title: 'Design TL;DR: S32-designfit-decisions-gate'
description: 'Design-fit gate fails closed when a slice implies Type-1 work but design_decisions is empty'
---

# Design TL;DR: S32-designfit-decisions-gate

## §1. User-visible change

When a release slice touches architecturally-significant packages (CLI surface,
state machine, verdict contract) but records no `design_decisions` in its
`status.json`, `sworn designfit <release>` now exits non-zero with a violation.
Previously such slices silently passed the gate — the exact bypass that let
`S23-memory-config` skip designfit. Benign slices with legitimately empty
`design_decisions` remain unaffected.

## §2. Design decisions not in spec

1. **Signal for "Type-1 implied": `planned_files` checked against
   architecturally-significant path prefixes** (`cmd/sworn/`,
   `internal/state/`, `internal/verdict/`). Uses existing data; no new schema
   field. Rationale: these three packages are the contract/control-plane
   surface — the spec says "prefer the least-invasive signal already available."

2. **Violation uses existing `Violation` struct with empty `ChoiceName`.**
   The `String()` method already handles this gracefully (formats as `"Sxx:
   <description>"`). No new struct needed.

3. **Named function `impliesType1Work`.** Keeps the architecturally-significant
   prefix set in one place. Called only when `design_decisions` is empty — the
   hot path (decisions present) is unchanged.

4. **No git-history dependency.** `Run()` only sees `status.json` from disk.
   Adding git would expand scope beyond the spec. Path-prefix check catches
   the highest-risk surface; the planner owns the rest.

5. **Existing two checks untouched.** The arch-significant-but-Type-2 and
   Type-1-without-human-decision checks continue exactly as before.

## §3. Files I'll touch grouped by purpose

- **`internal/designfit/designfit.go`** — Add `impliesType1Work()` function +
  replace unconditional `continue` with a Type-1-implied check
- **`internal/designfit/designfit_test.go`** — Add two new test cases
  (`TestType1ImpliedEmptyDecisionsFails`, `TestNoType1EmptyDecisionsPasses`);
  existing tests pass unchanged

## §4. Things I'm NOT doing

- Not adding a new field to `internal/state/state.go` (the spec says prefer
  existing signals)
- Not changing the CLI entry point (`cmd/sworn/designfit.go`) — it already
  maps `HasViolations()` to exit 1
- Not touching `internal/lint/` packages (S29–S31 territory)

## §5. Reachability plan

Run `sworn designfit` against a fixture release with a Type-1-implied slice
(touching `cmd/sworn/`) + empty `design_decisions` → captures non-zero exit.
Documented in `proof.md`.

## §6. Open questions for the Coach

None.