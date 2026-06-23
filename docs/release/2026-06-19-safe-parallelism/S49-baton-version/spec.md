---
title: 'S49-baton-version — reconcile the Baton pin from a raw SHA to a semver tag; `sworn version`/`doctor` report and gate "on Baton vX.Y.Z"'
description: 'The embed pins Baton by a raw 40-char SHA in internal/adopt/baton/VERSION, while internal/prompt/VERSION.txt carries a separate prompt-vendor version — two divergent sources, neither a clean semver tag. S49 reconciles them to a single semver tag (v0.4.0 at adoption), surfaces the released protocol version in `sworn version` (the existing two-line output `sworn <version>` / `baton-protocol on Baton vX.Y.Z`, via S49-owned `prompt.BatonVersion()`), and adds a `sworn doctor` check that fails closed when the pin is a SHA rather than a tag. depends_on S48 (internal/baton package). See ADR-0006.'
---

# Slice: `S49-baton-version`

## User outcome

`sworn version` prints, truthfully, which **released** Baton protocol version the
binary implements. The T15-owned `version` command (`cmd/sworn/main.go`, left
**unedited** per the design-review pin) prints two lines — `sworn <version>` then
`baton-protocol on Baton vX.Y.Z` — where the **`on Baton vX.Y.Z`** segment is a
semver tag (e.g. `v0.4.0`), not a raw commit SHA, and is supplied by the
S49-owned `prompt.BatonVersion()` → `baton.Version()` accessor. `sworn doctor`
reports the same `on Baton vX.Y.Z` line and **fails closed** if the embedded pin
is a 40-char SHA instead of a semver tag, so a binary can never ship claiming a
protocol version it can't name.

## Entry point

- `sworn version` (the `version` command registered via the S51/T15 `command` registry) —
  already prints `baton-protocol %s` via `prompt.BatonVersion()`; reframed to the semver "on
  Baton" line **by changing `baton.Version()` / `prompt.BatonVersion()` (both S49-owned), NOT by
  editing `cmd/sworn/main.go` or `cmd/sworn/commands.go`** (those are T15-owned). The version
  command handler is unchanged.
- `sworn doctor` (`cmd/sworn/doctor.go`, owned by S22/T4 — **merged**, so this is a
  sequential additive edit) — gains the pin-is-a-tag check.

## Background

Two divergent version sources exist today:

1. `internal/adopt/baton/VERSION` → `baton-protocol: cf158423f65c20860a3d4ec0310acb6cc7fb5aa0`
   — a **raw SHA** (the smell ADR-0006 names). It also records `rules-added: 08/09/10`.
2. `internal/prompt/VERSION.txt` → drives `prompt.BatonVersion()` (returns `batonVer`,
   a semver-ish string).

A user asking "what protocol am I on?" gets a SHA from one source and a string from
another. Baton now publishes tags `v0.1.0`…`v0.4.0` (upstream `VERSION`-file +
tag-discipline tracked at sawy3r/baton#31). S49 makes the pin a single semver tag and
surfaces/gates it.

This slice `depends_on S48-baton-vendor`: it reads the Baton version through the
`internal/baton` package S48 introduces (a single `baton.Version()` accessor), rather
than re-deriving from raw files in two places.

## In scope

- Reconcile the pin to a **semver tag** (`v0.4.0` at adoption) as the single source of
  truth: set it in `internal/adopt/baton/VERSION` (replace the SHA line with
  `baton-protocol: v0.4.0`, keeping the `upstream:`/`vendored:`/`rules-added:` lines)
  and make `internal/prompt/VERSION.txt` agree (or derive from the same value).
- `internal/baton/version.go` (new, in S48's package) — `Version() string` returns the
  pinned semver tag; `IsSemverTag(s string) bool` (matches `vMAJOR.MINOR.PATCH`,
  rejects a 40-hex-char SHA).
- Surface the semver tag in `sworn version` via the `baton-protocol on Baton vX.Y.Z`
  line of the existing two-line output (`sworn <version>` / `baton-protocol …`). The
  T15-owned `cmd/sworn/main.go` print site is **unchanged** (design-review pin); the
  "on Baton vX.Y.Z" text is produced inside `prompt.BatonVersion()`, which delegates to
  `baton.Version()` so there is one accessor.
- `sworn doctor` check: read the embedded pin; if `!IsSemverTag(pin)` emit an `[ERROR]`
  and exit non-zero (fail closed); otherwise print `on Baton vX.Y.Z`. Follow doctor's
  existing ERROR/WARN conventions (per S22 ack: never ERROR on an otherwise-clean repo
  for *unbuilt* features — but a SHA pin is a real present defect, so ERROR is correct).

## Out of scope

- The vendor/transform mechanism that *produces* the embed — **S48**.
- `sworn baton diff` and the governance process doc — **S50**.
- Changing protocol content or the `rules-added` provenance — that reconverges via the
  upstream PR tracked at sawy3r/baton#31, not here.
- Networked verification that the tag exists upstream — the pin is a local string; S50's
  `diff` is the divergence check.

## Planned touchpoints

- `internal/adopt/baton/VERSION` (SHA → semver tag; T3/S21-owned, sequential via dep)
- `internal/prompt/VERSION.txt` (agree with the tag; T3-owned, sequential via dep)
- `internal/prompt/prompt.go` (`BatonVersion()` delegates to `baton.Version()`)
- `internal/baton/version.go` (new — `Version`, `IsSemverTag`; the version-string source the
  `version` command renders — reframing happens here, not in `main.go`/`commands.go`)
- `internal/baton/version_stub.go` (new — build-tagged default pin accessor)
- `internal/baton/version_test.go` (new)
- `cmd/sworn/doctor.go` (pin-is-a-tag check; S22/T4 merged — sequential)
- `cmd/sworn/doctor_test.go` (the new check)

## Acceptance checks

- [ ] `baton.IsSemverTag("v0.3.0")` is true; `IsSemverTag("cf158423f65c20860a3d4ec0310acb6cc7fb5aa0")`
  is false; `IsSemverTag("0.3.0")` and `IsSemverTag("")` are false
- [ ] `baton.Version()` returns the semver tag (`v0.4.0`), not a SHA, read from the
  reconciled pin
- [ ] `internal/adopt/baton/VERSION` no longer contains a 40-hex-char SHA on the
  `baton-protocol:` line (assert by reading the embedded bytes)
- [ ] `sworn version` output contains "on Baton v" followed by a semver tag (assert via
  the command's output, not just the accessor — Rule 1 integration point)
- [ ] `sworn doctor` on the reconciled repo prints `on Baton vX.Y.Z` and exits 0
- [ ] `sworn doctor` fails closed (non-zero) when the pin is forced to a SHA (test with
  a fixture/injected pin), with an `[ERROR]` naming the SHA-not-tag defect
- [ ] `go test -race ./internal/baton/... ./internal/prompt/... ./cmd/sworn/...` passes;
  `go build ./...` clean

## Required tests

- **Unit**: `internal/baton/version_test.go` — `TestIsSemverTag` (table),
  `TestVersionIsSemverNotSha`. `cmd/sworn/doctor_test.go` —
  `TestDoctorFailsOnShaPin`, `TestDoctorReportsBatonTag` (and `TestDoctorAllOK` from
  S22 must still exit 0 against the reconciled embed).
- **Reachability artefact**: paste `sworn version` and `sworn doctor` output in
  `proof.md` showing the "on Baton v0.4.0" line and a forced-SHA failure run.

## Risks

- `internal/adopt/baton/VERSION` and `internal/prompt/VERSION.txt` are S21/T3-owned;
  S49 edits them only after T3 merges (the `depends_on T3` track edge). The implementer
  must forward-merge release-wt (Step 0) so the S21 embed layout is present before
  editing the pin.
- The S22 doctor suite (`TestDoctorAllOK`) must stay green — the new check must pass on
  the reconciled (tag) pin, only failing on a SHA. Don't regress S22's "clean repo
  exits 0" contract.
- Keep one accessor (`baton.Version()`); don't leave `prompt.BatonVersion()` reading a
  different file than the doctor check, or the two surfaces can disagree again.

## Deferrals allowed?

No deferrals expected — a pin-format reconciliation + one accessor + one doctor check,
all over data that already exists. Any deferral is a Rule-2 surfacing in `proof.md`.
