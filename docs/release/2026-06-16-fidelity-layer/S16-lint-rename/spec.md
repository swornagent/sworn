---
title: 'S16-lint-rename'
description: 'Documentation sweep — sworn lint ac / sworn lint trace naming consistency across the full release doc tree; restore S02 to implemented.'
track: T1-fidelity-core
---

# Slice: `S16-lint-rename`

## Background

After S01 and S02 were implemented under their original names (`rtm`, `ears`), both
command names were found to be opaque: `ears` is borrowed jargon (EARS = Easy Approach
to Requirements Syntax — meaningless without knowing the spec), and `rtm` is an
acronym (Requirements Traceability Matrix) equally opaque to newcomers. The
decision was made to group all quality-checking gates under a `sworn lint`
namespace — matching the developer-familiar lint mental model (`golint`, `eslint`,
etc.) and using plain-English target names:

- `sworn lint ac <release>` — acceptance-criteria format check (replaces original `ears`)
- `sworn lint trace <release>` — traceability matrix check (replaces original `rtm`)
The rationale and supersession of the original "standalone verbs" decision are
recorded in `intake.md` under `2026-06-18 — Lint namespace`. Internal packages
(`internal/ears`, `internal/rtm`) keep their precise names; only the CLI surface
changed.

The code rename landed in commit `6518f3b` on the T1-fidelity-core track branch
out-of-band (without a replan slice). This slice performs the remaining cleanup:

- Sweeps all release documentation for stale bare-verb references (`ears`, `rtm`)
  references and replaces them with the canonical names.
- Regenerates the S02-ears-ac-format proof.md so it captures the full diff
  from `start_commit` to HEAD (including `6518f3b`) — the prior proof was
  written before the rename and is missing those files from "Files changed".
- Transitions S02 back to `implemented` with a clean proof bundle, so
  verification can proceed on an accurate record.

## User outcome

All documentation in `docs/release/2026-06-16-fidelity-layer/` consistently
refers to `sworn lint ac` and `sworn lint trace`. No stale references to the original bare-verb names (`ears`, `rtm`)
remain in any spec.md, proof.md, index.md, intake.md,
or status.json. The S02-ears-ac-format proof bundle accurately reflects every
file in the diff and S02 is in `implemented` state, ready for fresh
verification.

## Entry point

Documentation-only slice — no new binary behaviour. All Go code changes
(`cmd/sworn/lint.go`, `cmd/sworn/lint_ac_test.go`, `cmd/sworn/lint_trace_test.go`)
were completed in commit `6518f3b`; this slice only updates artefacts.

## In scope

- Regenerate `docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/proof.md`
  to reflect the current diff (start_commit `cd462364` to HEAD), capturing all
  files introduced/deleted by the rename commit.
- Update `S02-ears-ac-format/status.json`: clear `verification.result`
  violations, update `actual_files`, set `state` to `implemented`.
- Update `docs/release/2026-06-16-fidelity-layer/intake.md`: replace the
  decision record reference to the original `rtm` name with `sworn lint trace`.
- Verify no remaining stale bare-verb references exist in any
  `.md` or `.json` under `docs/release/2026-06-16-fidelity-layer/` (excluding
  `docs/captures/` historical snapshots, which are time-stamped records).
- Update `S01-rtm-spine/status.json` `planned_files`: replace
  `cmd/sworn/rtm.go` and `cmd/sworn/rtm_test.go` with `cmd/sworn/lint.go`
  and `cmd/sworn/lint_trace_test.go`.

## Out of scope

- Any changes to `internal/ears/` or `internal/rtm/` package internals.
- Any changes to `docs/captures/` historical snapshots.
- Any new command behaviour or API surface changes.

## Acceptance checks

- [ ] THE SYSTEM SHALL have no stale references to the old bare-verb command names (`ears`, `rtm`) as `sworn` subcommands in any `.md` or `.json` file under `docs/release/2026-06-16-fidelity-layer/`, excluding `docs/captures/` and the S16-lint-rename artefacts that define this sweep. Compliance is verified by the grep gate described in Required tests. (N-S16-01)
- [ ] WHEN `sworn lint ac 2026-06-16-fidelity-layer` is run, THE SYSTEM SHALL exit 0 — confirming the renamed command works as documented and all release ACs remain well-formed EARS. (N-S16-02)
- [ ] THE SYSTEM SHALL have `S02-ears-ac-format` in `implemented` state with a proof.md whose "Files changed" section lists every file in `git diff --name-only cd462364..HEAD`, including `cmd/sworn/lint.go`, `cmd/sworn/lint_ac_test.go`, `cmd/sworn/lint_trace_test.go`, and the deleted `cmd/sworn/ears.go`, `cmd/sworn/rtm.go`. (N-S16-03)
- [ ] WHERE `cmd/sworn/ears.go` or `cmd/sworn/rtm.go` appear in any `status.json` `planned_files` or `actual_files` array, THE SYSTEM SHALL replace them with `cmd/sworn/lint.go`. (N-S16-04)
## Planned touchpoints

- `docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/proof.md` (regenerate)
- `docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/status.json` (clear violations, actual_files, state→implemented)
- `docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/status.json` (planned_files correction)
- `docs/release/2026-06-16-fidelity-layer/intake.md` (decision record update)

## Required tests

- **Grep gate**: Search `docs/release/2026-06-16-fidelity-layer/` for stale references to the old bare-verb `sworn` subcommand names (`ears`, `rtm`) — must produce no matches outside `docs/captures/` and S16's own sweep-defining artefacts.
- **Integration**: `go test ./cmd/sworn/ -run TestLintAC` and `go test ./cmd/sworn/ -run TestLintTrace` — both pass (confirms the binary works as documented in updated specs).
- **Reachability artefact**: `sworn lint ac 2026-06-16-fidelity-layer` exits 0 (the live release passes its own AC format gate with the renamed command).
## E2E gate type

`local` — no persona creds; all assertions are grep-based or binary-invocation.
