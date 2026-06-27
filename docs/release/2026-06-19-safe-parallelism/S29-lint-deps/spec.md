---
title: 'S29-lint-deps — `sworn lint deps` fails closed on undeclared go.mod/go.sum changes'
description: 'A slice that adds or removes a Go dependency must declare go.mod/go.sum in its status.json planned_files, or the dependency diff trips Gate 2 at verify. Add a `sworn lint deps` subcommand that diffs go.mod/go.sum against the slice''s planned_files and fails closed (exit 1) when a dep change is undeclared, plus a planner-side note to auto-add go.mod/go.sum to planned_files on any dependency change. Harvested from the Captain trial-log analysis (§3a #1, theme T-B).'
---

# Slice: `S29-lint-deps`

## User outcome

A planner or implementer running `sworn lint deps <slice-id> <release>` learns
**before verify** whether a slice's `go.mod` / `go.sum` changes are declared in
that slice's `status.json` `planned_files`. If a dependency change is present in
the diff but `go.mod`/`go.sum` are absent from `planned_files`, the command exits
non-zero with a message naming the undeclared files. This closes the recurring
Gate-2 surprise where a new dependency appears in the verifier's diff but was never
declared — turning a late verify failure into a cheap pre-flight check.

## Entry point

`sworn lint deps <slice-id> <release>` (CLI). Verifiable by: an integration-style
test that constructs a temp release with a slice whose diff touches `go.mod` but
whose `planned_files` omits it, runs the `deps` lint, and asserts a non-zero result
with the undeclared file named; and the inverse (declared → exit 0).

## In scope

- New `sworn lint deps` target dispatched from `cmd/sworn/lint.go` (alongside the
  existing `ac` and `trace` targets).
- A `internal/lint` helper package (`deps.go`) that:
  - determines whether `go.mod` / `go.sum` changed for the slice (diff against the
    slice's `start_commit`, or against a supplied base ref);
  - reads the slice's `status.json` `planned_files`;
  - records a violation (fail closed, exit 1) when a changed dep file is not in
    `planned_files`.
- A **planner-side note** (prose) instructing the planner to auto-add `go.mod` and
  `go.sum` to a slice's `planned_files` whenever the slice introduces or removes a
  dependency. This lands in `internal/prompt/planner.md` as a checklist line.

## Out of scope

- Auto-*editing* slice `status.json` files to insert the dep files — the lint
  reports the gap; fixing it is a planner/human action (consistent with the other
  `sworn lint` targets, which report rather than mutate).
- Validating that the dependency itself is justified by an ADR (Rule: minimal
  justified deps) — that is a human/Captain judgement, not a mechanical lint.
- The `touchpoints` and `symbols` lint targets (S30, S31 — separate slices).

## Planned touchpoints

- `internal/lint/deps.go` (new helper package + dep-diff reconciliation)
- `internal/lint/deps_test.go` (new)
- `cmd/sworn/lint.go` (extend the target switch with `deps`)
- `internal/prompt/planner.md` (planner checklist line: auto-add go.mod/go.sum on a dep change)

> **Touchpoint note:** the existing `sworn lint` targets (`ac`, `trace`) are
> implemented in `cmd/sworn/lint.go` and delegate to helper packages
> (`internal/ears`, `internal/rtm`). This slice follows the same convention with a
> new `internal/lint` package. `internal/lint` does **not** exist yet — it is
> created by this slice.

## Acceptance checks

- [ ] `sworn lint deps <slice> <release>` exits **non-zero** when the slice's diff
  changes `go.mod` and/or `go.sum` but those files are absent from the slice's
  `status.json` `planned_files`; the message names the undeclared file(s)
- [ ] `sworn lint deps <slice> <release>` exits **0** when the dep files that
  changed are all present in `planned_files` (and exits 0 when no dep files changed)
- [ ] `internal/prompt/planner.md` contains a checklist line directing the planner to
  add `go.mod` and `go.sum` to `planned_files` whenever a slice adds/removes a dep
- [ ] `go build ./...` and `go vet ./internal/lint/...` pass

## Required tests

- **Unit / integration** `internal/lint/deps_test.go`:
  - `TestDepsUndeclaredFails`: slice diff touches `go.mod`, `planned_files` omits it
    → violation recorded, non-zero result
  - `TestDepsDeclaredPasses`: `go.mod` in `planned_files` → no violation
  - `TestDepsNoChangePasses`: no dep-file change → no violation
- **Reachability artefact**: run `sworn lint deps` against a fixture release with an
  undeclared dep change; capture the non-zero exit + message. Document in `proof.md`.

## Risks

- Determining "did go.mod change for this slice" requires a base ref. Mitigation:
  diff against the slice's `start_commit` (already read by the verifier flow — see
  `internal/state/state.go:141` for the `status.json` schema fields and
  `cmd/sworn/lint.go` for how the existing targets locate the slice dir); accept an
  explicit base-ref flag for tests so the test does not depend on git history.

## Deferrals allowed?

None.
