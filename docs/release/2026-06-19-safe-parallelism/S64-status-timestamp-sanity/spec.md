---
title: 'S64-status-timestamp-sanity — fail-closed lint/doctor gate for impossible future status timestamps'
description: 'A status.json can currently carry a last_updated_at or verifier_verdict_at date days/weeks in the future; the board faithfully renders it and no deterministic gate rejects it. Add a Sworn-owned lint/doctor hardening gate with fixed-clock tests so impossible status metadata cannot pass silently.'
---

# Slice: `S64-status-timestamp-sanity`

## User outcome

A maintainer runs `sworn lint` or `sworn doctor` on a release tree and gets a fail-closed error when any Baton `status.json` timestamp is in the future beyond a small clock-skew allowance. The board should no longer be the first place a human notices impossible dates such as `2026-07-15` on 2026-06-25; deterministic tooling names the offending slice and field.

## Entry point

- `sworn lint <release>` — add a status metadata check alongside the existing lint hardening gates.
- `sworn doctor` — add a repository health check that scans release artefacts and exits non-zero when future status timestamps exist.

## Background

The release oracle exposed multiple future-dated `last_updated_at` values in `docs/release/**/status.json`. Git history showed the commits were authored on the real date; the future values were written into the JSON artefacts and then rendered faithfully by the board. This is Sworn implementation hygiene, not an upstream Baton protocol defect: Sworn owns the deterministic readers, lint gates, and doctor surface that should reject impossible metadata.

## In scope

- Add a reusable status-timestamp validator under `internal/lint` (or the existing lint-adjacent package if implementation finds a better local home).
- Validate `last_updated_at` and `verification.verifier_verdict_at` when present.
- Reject unparsable RFC3339 timestamps and timestamps greater than `now + 5m`.
- Inject a fixed clock in tests; do not make tests depend on wall-clock time.
- Wire the validator through `sworn lint` and `sworn doctor` so the failure is user-reachable and exits non-zero.
- Error messages name release, slice id, field path, timestamp value, and allowed maximum.

## Out of scope

- Auto-rewriting bad timestamps. The gate reports defects; repair stays human-owned.
- Upstream Baton rule/template changes. If this proves broadly useful later, PR optional guidance upstream; this slice lands in Sworn only.
- TUI/board rendering changes. The deterministic gate is the scope; visual warning styling can be a later UI polish slice.

## Planned touchpoints

- `internal/lint/status_time.go` (new)
- `internal/lint/status_time_test.go` (new)
- `cmd/sworn/lint.go`
- `cmd/sworn/lint_trace_test.go` or a new command-level lint test
- `cmd/sworn/doctor.go`
- `cmd/sworn/doctor_test.go`

## Acceptance checks

- [ ] A fixture release with `last_updated_at` after `now + 5m` causes `sworn lint <release>` to exit non-zero and print the slice id plus `last_updated_at`.
- [ ] A fixture release with `verification.verifier_verdict_at` after `now + 5m` causes `sworn lint <release>` to exit non-zero and print the slice id plus `verification.verifier_verdict_at`.
- [ ] The same future-timestamp fixture causes `sworn doctor` to exit non-zero with an `[ERROR]` line naming the offending slice and field.
- [ ] Valid past timestamps and timestamps within the 5-minute skew allowance pass.
- [ ] Malformed timestamp strings fail closed with a parse error naming the field.
- [ ] Tests use an injected fixed clock; no test compares against the real current time.
- [ ] `go test -race ./internal/lint/... ./cmd/sworn/...` and `go build ./...` pass.

## Required tests

- **Unit**: `internal/lint/status_time_test.go` — table tests for future, within-skew, past, missing optional verifier timestamp, and malformed timestamps.
- **Reachability artefact (Rule 1)**: command-level tests driving `sworn lint <fixture-release>` and `sworn doctor` against fixture repos with future timestamps; assert non-zero exit code and field-specific output.

## Risks

- The current release contains existing future-dated artefacts. Implementing the gate may make `sworn doctor` fail on the live repo until those artefacts are repaired. That is intended: the defect exists today and should be explicit.
- If implementation scans every historical release, many old fixture/status files may fail. Prefer scanning release artefacts under the current repo with clear filtering rules; if a narrower scope is chosen, document it in proof.md.

## Deferrals allowed?

Only board/TUI visual warning styling may be deferred, because this slice is the deterministic lint/doctor gate. If deferred, record why + tracking + acknowledgement in `proof.md`.
