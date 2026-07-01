---
title: 'Release intake — 2026-07-01-release-hygiene'
description: 'sworn reports its real version from a single embedded source regardless of build method, and a CI gate blocks merge into main without a version bump.'
---

# Release Intake: `2026-07-01-release-hygiene`

## Release goal

Fix two release-hygiene gaps surfaced while preparing the operational push: (1) `sworn
--version` reports `0.0.0-dev` unless built via `make` (ldflags), because the version is a
build-time-injected literal with a dev placeholder default — a plain `go build`/`go install`
loses it; (2) nothing stops a merge into `main` without bumping the version. Establish a
single embedded version source so every build reports correctly, and a fail-closed CI gate
that requires a version bump on PRs to `main`. "Shipped" = `go build ./cmd/sworn && ./sworn
version` prints the real version, and a PR to `main` without a bump is red.

This is an engine/CI release — no UI surface. The floor considerations that bear:
correctness (the reported version must match the source), and CI fail-closed behaviour.

## Source of truth

- **Human stakeholder**: Brad.
- **Tracking issue / epic**: (file one — "version source + bump gate"); related to the
  operational-readiness push (where the 0.0.0-dev report was noticed).
- **Related captures**: this release was split out of the operational-readiness conversation
  so its golden thread (run real coach releases) stayed clean.

## What's currently broken or missing

- `cmd/sworn/main.go:27` — `var version = "0.0.0-dev"`, overridden only by
  `make`'s `-ldflags "-X main.version=$(VERSION)"` (Makefile `VERSION ?= 0.1.0`). So
  `go install ./cmd/sworn` (how the live `/home/brad/go/bin/sworn` was built) reports
  `0.0.0-dev` instead of `0.1.0`. There is no `VERSION` file; the version lives only in the
  Makefile literal.
- No CI gate enforces a version bump on merge to `main`, so a release can ship without the
  reported version advancing.

## What the human wants

- `sworn --version` reports the correct version from a single source, no matter how the binary
  was built (`go build`, `go install`, or `make`).
- A CI gate: a PR into `main` cannot merge without a version bump (fail closed).

### Needs (RTM anchors)

- N-01: sworn reports its real version (from a single embedded source) for ANY build method (go build / go install / make), not the 0.0.0-dev placeholder; ldflags may still override for release builds.
- N-02: a pull request that would merge into main without a version bump is blocked by a fail-closed CI gate.

## Constraints and non-negotiables

- Single source of truth for the version — no second copy to drift (Makefile + embedded must read the same file).
- ldflags override preserved (release builds can still stamp a version explicitly).
- CI gate fails closed (absence of a bump is a failure, not a pass).
- Minimal deps; stdlib `go:embed`.

## Adjacent / out of scope

- **Auto-tagging / release automation** (cutting git tags from the version). **Why deferred**:
  this release establishes the source + the gate; automated tagging is a separate concern.
  **Tracking**: future release-automation work. **Acknowledged**: 2026-07-01.
- **Changelog enforcement**. **Why deferred**: separate governance concern. **Acknowledged**: 2026-07-01.

## Decisions made during planning

### 2026-07-01 — Single embedded version source under internal/version

- **Context**: `go:embed` cannot reach a repo-root `VERSION` file from `cmd/sworn`, and an
  ldflags-injected literal is lost on plain `go build`.
- **Decision**: put the version in `internal/version/version.txt`, embed it via
  `internal/version/version.go` (`//go:embed version.txt`), and make `cmd/sworn/main.go` use it
  as the default while still allowing the ldflags `-X main.version` override (ldflags wins when
  set, else the embedded value). The Makefile sources the SAME file
  (`VERSION ?= $(shell cat internal/version/version.txt)`). The CI gate compares that file.
- **Why**: one file is the single source of truth — Makefile, runtime default, and CI gate all
  read it, so they cannot drift.

### 2026-07-01 — Two slices, one track (S02 depends on S01)

- **Decision**: S01-embedded-version (the source + runtime + Makefile), then
  S02-version-bump-ci-gate (the CI workflow comparing the file). S02 depends on S01 because the
  gate compares the version source S01 establishes. Same track, serial.

## Proposed slice decomposition (draft)

- `S01-embedded-version` — `sworn version` reports the real version from `internal/version/version.txt` for any build method.
- `S02-version-bump-ci-gate` — a PR to `main` without bumping `internal/version/version.txt` fails CI (fail closed).

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Should the gate compare semver strictly-greater, or just "changed"? | S02 AC precision | resolved: strictly-greater semver (a no-op edit must not pass) |
| A-02 | New workflow file vs a job appended to ci.yml | S02 touchpoint | new `.github/workflows/version-bump.yml` (isolated, clearest) |

## Screenshots / references

- (none — engine/CI release)
