---
title: 'S30-lint-touchpoints — `sworn lint touchpoints` reconciles design files against planned_files + the collision matrix'
description: 'The dominant Captain-catch class (theme T-A, ~35 rows) is a slice whose design references files/packages it never declared in planned_files, or that collide with another slice across tracks. Add a `sworn lint touchpoints` subcommand that parses a slice''s design for referenced files/packages, reconciles them against planned_files AND the release index.md touchpoint matrix (flagging cross-slice file collisions), and detects duplicate migration numbers across slices. Fail closed on an undeclared touchpoint or an unacknowledged collision. Harvested from the trial-log analysis §3a #2 and #4.'
---

# Slice: `S30-lint-touchpoints`

## User outcome

A planner or implementer running `sworn lint touchpoints <slice-id> <release>` learns
**before code** whether the slice's design references files or packages that are not in
its `status.json` `planned_files`, whether any of its planned files collide with another
slice/track in the release `index.md` touchpoint matrix, and whether two slices declare
the same migration number. Undeclared touchpoints and unacknowledged collisions fail
closed (exit 1). This directly mechanises the single most common Captain catch — designs
that touch files they never declared, and cross-slice file/migration collisions that
break parallel-safe track grouping.

## Entry point

`sworn lint touchpoints <slice-id> <release>` (CLI). Verifiable by: an integration-style
test with a temp release whose design references a file absent from `planned_files` →
non-zero; a release whose two slices mark the same file in the matrix → non-zero
collision; two slices with the same migration number → non-zero; the clean inverse →
exit 0.

## In scope

- New `sworn lint touchpoints` target dispatched from `cmd/sworn/lint.go`.
- A `internal/lint` helper (`touchpoints.go`) that:
  - parses the slice's design/spec for referenced files and packages (back-ticked
    paths, `internal/...` package refs, `.go`/`.ts` filenames);
  - reconciles them against the slice's `status.json` `planned_files` — undeclared
    reference → violation;
  - cross-checks the release `index.md` touchpoint matrix and flags any file claimed
    by more than one slice/track that is not acknowledged (cross-slice collision);
  - detects duplicate migration numbers across slices (same `NNNNNN` migration id in
    two slices' planned files).
- Fail closed (exit 1) on any undeclared touchpoint or unacknowledged collision.

## Out of scope

- Symbol/identifier resolution (functions, fields, constants) — that is S31
  (`lint symbols`), a separate slice.
- go.mod/go.sum dependency reconciliation — that is S29 (`lint deps`).
- Auto-editing `planned_files` or the matrix to fix the gap — report only.

## Planned touchpoints

- `internal/lint/touchpoints.go` (new)
- `internal/lint/touchpoints_test.go` (new)
- `cmd/sworn/lint.go` (extend the target switch with `touchpoints`)

> **Touchpoint note:** the release touchpoint matrix lives in
> `docs/release/<release>/index.md` under the `### Touchpoint matrix` heading
> (confirmed present in this release's `index.md`). `internal/lint` is the shared new
> package introduced by S29 — this slice adds `touchpoints.go` to it. If S29 and S30
> land on the same track (T12) sequentially there is no `internal/lint` collision; both
> are listed in the matrix as T12-owned.

## Acceptance checks

- [ ] `sworn lint touchpoints <slice> <release>` exits **non-zero** when the slice's
  design references a file/package absent from its `planned_files`; the message names
  the undeclared reference
- [ ] `sworn lint touchpoints <slice> <release>` exits **non-zero** when a file is
  claimed by more than one slice/track in the `index.md` touchpoint matrix without
  acknowledgement (cross-slice collision)
- [ ] `sworn lint touchpoints <slice> <release>` exits **non-zero** when two slices
  declare the same migration number
- [ ] `sworn lint touchpoints <slice> <release>` exits **0** on a clean slice (all
  references declared, no collision, no duplicate migration)
- [ ] `go build ./...` and `go vet ./internal/lint/...` pass

## Required tests

- **Unit / integration** `internal/lint/touchpoints_test.go`:
  - `TestTouchpointUndeclaredFails`: design references `internal/foo/bar.go` absent
    from `planned_files` → violation
  - `TestTouchpointCollisionFails`: two slices mark the same matrix file → collision
    violation
  - `TestMigrationCollisionFails`: two slices share migration number `000012` → violation
  - `TestTouchpointCleanPasses`: all references declared, no collision → no violation
- **Reachability artefact**: run `sworn lint touchpoints` against a fixture release with
  an undeclared touchpoint and against one with a matrix collision; capture both
  non-zero exits. Document in `proof.md`.

## Risks

- Parsing "referenced files/packages" from prose design risks false positives (a file
  named in an Out-of-scope section). Mitigation: scope extraction to back-ticked
  identifiers that look like paths (contain `/` or a known extension), and to the
  In-scope / Planned-touchpoints sections — mirroring how `cmd/sworn/lint.go`'s `trace`
  target already locates structured content; verify the section anchors against a real
  spec (`docs/release/2026-06-19-safe-parallelism/S02b-concurrent-scheduler/spec.md`)
  before relying on them.

## Deferrals allowed?

None.
