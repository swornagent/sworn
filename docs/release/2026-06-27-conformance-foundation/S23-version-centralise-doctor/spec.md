---
title: 'S23 — Centralise VERSION; doctor SHA-vs-HEAD + pre-JSON-prompt drift check'
description: 'Centralise the three coexisting VERSION strings to a single canonical source; add doctor checks for SHA-vs-HEAD pin drift and pre-records-as-JSON prompt detection; both checks fail closed.'
---

# Slice: `S23-version-centralise-doctor`

## User outcome

`sworn doctor` reports PIN-STALE when the embedded vendor pin predates the `baton/` layout; reports PROMPT-STALE when embedded prompts contain pre-records-as-JSON markers; `internal/baton/version.go` and `internal/prompt/prompt.go` read version from the same canonical source (not hardcoded strings); `sworn version` outputs a single consistent version string.

## Reuse existing work (do not redo the centralisation half)

The **VERSION-centralisation** part of this slice is **already implemented** on branch `fix/centralise-baton-version` (origin `4d17e35`), with a green proof bundle at `docs/captures/2026-06-27-centralise-baton-version-ref.md` (it was closed as sworn#24, not merged). That work: deletes the zombie `internal/prompt/VERSION.txt` + `internal/prompt/baton/VERSION.txt`, consolidates the doctor version check into one, adds `TestNoEmbeddedVersionFile` + `TestUpstreamPinComplete`, and repoints `internal/prompt/prompt.go`'s origin comments to the canonical git repo. **Lift it (cherry-pick `4d17e35`)** rather than redoing it. Then ADD the two NEW doctor drift checks below (`baton/pin-currency`, `baton/prompt-currency`) — those were **not** in sworn#24 and are the new scope of this slice.

NB the audit's "three version strings" were `v0.4.2` (internal/prompt), `v0.5.0` (the adopt VERSION's `baton-protocol:`), and `v1.0.0` (internal/prompt/baton/VERSION.txt). The `4d17e35` branch already removes the two `internal/prompt` ones; confirm the centralisation against that branch's state.

## Entry point

`cmd/sworn/doctor.go` (audit ref: `cmd/sworn/doctor.go:419-449`); `internal/baton/version.go`; `internal/prompt/prompt.go`.

## In scope

- **VERSION centralisation**: audit shows three version strings: `v0.4.2` (internal/baton/ and internal/prompt/), `v0.5.0` (internal/adopt/baton/VERSION `baton-protocol:` field), `v1.0.0` (somewhere else). Centralise to a single `internal/version/version.go` or use `internal/adopt/baton/VERSION` as the canonical source read at startup; remove the other hardcoded strings. The canonical version is the `baton-protocol:` field from `internal/adopt/baton/VERSION`.
- **SHA-vs-HEAD doctor check**: new check in `cmd/sworn/doctor.go`:
  - Name: `baton/pin-currency`
  - Pass: the `upstream-sha` in `internal/adopt/baton/VERSION` matches a commit in the Baton canonical repo that contains a `baton/` directory (i.e. is post-baton-layout)
  - Fail: the SHA predates `baton/` — report "PIN-STALE: upstream-sha <sha> predates baton/ layout — re-vendor required"
  - Implementation: check whether the vendored `internal/adopt/baton/` directory contains any file whose path starts with `baton/` — if not (pre-layout pin), the check fails. No network call required.
- **Pre-JSON-prompt drift doctor check**: new check in `cmd/sworn/doctor.go`:
  - Name: `baton/prompt-currency`
  - Pass: embedded prompts do not contain `v0.4.2`, `proof.md-primary`, `PROOF-optional`, or `scripts/release-verify.sh` references
  - Fail: any such marker found — report the offending file and marker
  - Implementation: scan the embedded prompt files (`internal/prompt/*.md`) for these patterns; grep-style check

## Out of scope

- Bumping the actual pin (S22)
- Re-vendoring prompts (S20)
- The doctor embed-integrity check (already full per audit — no changes needed)
- Any changes to the Go binary version string (`go build -ldflags "-X main.version=..."`)

## Planned touchpoints

- `cmd/sworn/doctor.go` (add two new check functions + wire into the doctor check list)
- `internal/prompt/prompt.go` (read BatonVersion from canonical source, not hardcoded)
- `internal/baton/version.go` (read from canonical source, not hardcoded)

## Acceptance checks

- [ ] WHEN `sworn doctor` is run with the current pin (pre-baton/ layout, SHA `9ae08fb`), THE SYSTEM SHALL report "PIN-STALE" for the pin-currency check
- [ ] WHEN `sworn doctor` is run with the current embedded prompts containing `v0.4.2` references, THE SYSTEM SHALL report "PROMPT-STALE" for the prompt-currency check
- [ ] WHEN `sworn doctor` is run after S22 (new pin with baton/ layout) and S20 (re-vendored prompts), THE SYSTEM SHALL report PASS for both new checks
- [ ] `internal/prompt/prompt.go` `BatonVersion()` reads from the `internal/adopt/baton/VERSION` embed (not a hardcoded `"v0.4.2"` or similar)
- [ ] `grep -rn '"v0.4.2"\|"v1.0.0"' internal/baton/ internal/prompt/ cmd/sworn/` returns zero results after this slice (only the VERSION file itself contains the version string)

## Required tests

- **Unit**: `cmd/sworn/doctor_test.go` (extend existing) — add tests for pin-currency check (pre-baton/ layout → FAIL; post-baton/ layout → PASS) and prompt-currency check (v0.4.2 in prompt → FAIL; clean prompt → PASS)
- **Reachability artefact**: `go test ./cmd/sworn/... -v -run TestDoctorPin` exits 0; `sworn doctor` output includes the new check names

## Risks

- VERSION centralisation may break callers that currently import the hardcoded version strings directly; the implementer must audit all call sites before removing them

## Deferrals allowed?

No.
