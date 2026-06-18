---
title: 'S16-lint-rename'
description: 'Documentation sweep — sworn lint ac / sworn lint trace naming consistency across the full release doc tree; restore S02 to implemented.'
track: T1-fidelity-core
---

# Slice: `S16-lint-rename`

## Background

During implementation of S01 and S02, the user-facing commands were renamed
out-of-band (without a replan slice): `sworn ears` → `sworn lint ac` and
`sworn rtm` → `sworn lint trace`. The code rename landed in commit `6518f3b`
on the T1-fidelity-core track branch. This slice performs the remaining cleanup:

- Sweeps all release documentation for stale `sworn ears` / `sworn rtm`
  references and replaces them with the canonical names.
- Regenerates the S02-ears-ac-format proof.md so it captures the full diff
  from `start_commit` to HEAD (including `6518f3b`) — the prior proof was
  written before the rename and is missing those files from "Files changed".
- Transitions S02 back to `implemented` with a clean proof bundle, so
  verification can proceed on an accurate record.

## User outcome

All documentation in `docs/release/2026-06-16-fidelity-layer/` consistently
refers to `sworn lint ac` and `sworn lint trace`. No stale `sworn ears` or
`sworn rtm` references remain in any spec.md, proof.md, index.md, intake.md,
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
  decision record reference to `sworn rtm` with `sworn lint trace`.
- Verify no remaining `sworn ears` or `sworn rtm` references exist in any
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

- [ ] WHEN `grep -rn "sworn ears\|sworn rtm\b" docs/release/2026-06-16-fidelity-layer/ --include="*.md" --include="*.json"` is run from the repo root, THE SYSTEM SHALL produce no output (zero matches outside `docs/captures/`). (N-S16-01)
- [ ] WHEN `sworn lint ac 2026-06-16-fidelity-layer` is run, THE SYSTEM SHALL exit 0 — confirming the renamed command works as documented and all release ACs remain well-formed EARS. (N-S16-02)
- [ ] THE SYSTEM SHALL have `S02-ears-ac-format` in `implemented` state with a proof.md whose "Files changed" section lists every file in `git diff --name-only cd462364..HEAD`, including `cmd/sworn/lint.go`, `cmd/sworn/lint_ac_test.go`, `cmd/sworn/lint_trace_test.go`, and the deleted `cmd/sworn/ears.go`, `cmd/sworn/rtm.go`. (N-S16-03)
- [ ] WHERE `cmd/sworn/ears.go` or `cmd/sworn/rtm.go` appear in any `status.json` `planned_files` or `actual_files` array, THE SYSTEM SHALL replace them with `cmd/sworn/lint.go`. (N-S16-04)

## Planned touchpoints

- `docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/proof.md` (regenerate)
- `docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/status.json` (clear violations, actual_files, state→implemented)
- `docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/status.json` (planned_files correction)
- `docs/release/2026-06-16-fidelity-layer/intake.md` (decision record update)

## Required tests

- **Grep gate**: `grep -rn "sworn ears\|sworn rtm\b" docs/release/2026-06-16-fidelity-layer/ --include="*.md" --include="*.json"` → must produce no output.
- **Integration**: `go test ./cmd/sworn/ -run TestLintAC` and `go test ./cmd/sworn/ -run TestLintTrace` — both pass (confirms the binary works as documented in updated specs).
- **Reachability artefact**: `sworn lint ac 2026-06-16-fidelity-layer` exits 0 (the live release passes its own AC format gate with the renamed command).

## E2E gate type

`local` — no persona creds; all assertions are grep-based or binary-invocation.
