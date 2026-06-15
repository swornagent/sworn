---
title: S09-distribution
description: goreleaser single binary + Homebrew tap + container (GHCR) + versioned release workflow.
---

# Slice: `S09-distribution`

## User outcome

A developer installs `sworn` via Homebrew, `go install`, or a container, and runs
it immediately — the turnkey install side of "download and get value".

## Entry point

`brew install swornagent/tap/sworn` · `go install github.com/swornagent/sworn/cmd/sworn@latest`
· `docker run ghcr.io/swornagent/sworn`.

## In scope

- `goreleaser` (single static binary, `CGO_ENABLED=0`, cross-platform matrix, GH
  Releases, version via ldflags), a Homebrew tap formula, a container image
  (GHCR), and a release GitHub workflow.

## Out of scope

- The GitHub Action *gate* (the secondary verify-on-top mode) — separate.

## Planned touchpoints

- `.goreleaser.yaml`, `.github/workflows/release.yml`, `Dockerfile`, `packaging/`

## Acceptance checks

- [ ] `go install .../cmd/sworn@latest` and `brew install swornagent/tap/sworn`
      each produce a working `sworn`.
- [ ] The container runs `sworn verify`.
- [ ] `sworn version` reflects the release tag.

## Required tests

- **CI**: the release workflow builds all artifacts; a smoke step runs the built
  binary (`sworn version` + a stub `verify`).

## Risks

- Static linking / cgo — `CGO_ENABLED=0`.
- Cross-platform breakage — goreleaser build matrix.

## Deferrals allowed?

No.
