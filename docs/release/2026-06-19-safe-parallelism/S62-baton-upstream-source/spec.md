---
title: 'Slice spec — S62-baton-upstream-source'
description: '`sworn baton vendor` fetches the version-locked Baton release from the public repo over stdlib HTTPS, verified by tag + commit SHA / content digest, fail-closed — so the embed source-of-truth is the public Baton repo at a pinned, tested version, never a local install.'
---

# Slice: `S62-baton-upstream-source`

> Tracking: GitHub issue #11 (the Rule-2 home for S48's deferred network fetch).
> `depends_on S48-baton-vendor` (source resolver + transform) and `S49-baton-version`
> (semver pin). See ADR-0006.

## User outcome

A maintainer runs `sworn baton vendor --upstream` and the binary regenerates its embedded
Baton protocol from the **public Baton repo at the pinned semver tag**, fetched over HTTPS —
verifying the resolved commit SHA and content digest before writing. The embed's source of
truth is `github.com/sawy3r/baton` at a locked, tested version; it can never silently pull a
newer/untested Baton or vendor from a personal local install. Any mismatch (force-moved tag,
tampered tarball, network error) fails closed with no write.

## Entry point

`sworn baton vendor --upstream [--tag vX.Y.Z] [--repo <owner/name>]` (`cmd/sworn/baton.go`).
Default (no `--upstream`) preserves S48's local-dir behaviour; `--upstream` selects the
network source provider.

## In scope

- A network **source provider** in `internal/baton`: fetch the pinned tag's release tarball
  `https://codeload.github.com/<owner>/<repo>/tar.gz/refs/tags/<tag>` via `net/http`,
  decompress with `compress/gzip`, extract with `archive/tar` (stripping GitHub's
  `<repo>-<ref>/` top-level prefix) into a temp source dir — **stdlib only, no git binary,
  no module dependency**.
- **Version lock**: the resolved upstream commit SHA and a content digest are verified
  against the pin recorded alongside the tag; on mismatch, fail closed (non-zero exit, no
  write). With no `--tag`, the pinned semver tag (S49) is used — never `latest`/HEAD.
- **Canonical default repo** `github.com/sawy3r/baton`, overridable via `--repo` / config.
- The existing S48 `Transform` runs unchanged on the fetched source (network is just another
  source provider feeding the same pipeline).
- Record the resolved commit SHA / digest in the pin record so re-vendor is reproducible and
  a force-moved tag is detected (extends S49's VERSION format; sequential via dep).

## Out of scope

- **Private-repo auth / PAT** — public repo only for now (issue #11 notes PAT as a later need).
- **git clone / go-git transport** — explicitly decided against; tarball-over-HTTPS only.
- Changing the **vendor file mapping** (which roles/rules are embedded) — the captain /
  requirements-verifier mapping reconciliation is a separate post-R3 governance item.
- A live-remote `baton diff` (S50 diffs against the pinned local source).

## Planned touchpoints

- `internal/baton/fetch.go` (new) — HTTPS tarball fetch + extract + SHA/digest verify; exported `SetBaseURLForTest` / `ClearBaseURLForTest` for integration tests
- `internal/baton/fetch_test.go` (new) — `httptest.Server` fixtures: success, digest/SHA
  mismatch, 404/network-error, bad-gzip, prefix-strip, bootstrap
- `internal/baton/version.go` — upstream pin read/write (`ReadUpstreamPin`, `WriteUpstreamPin`, `UpstreamPin` struct)
- `internal/baton/version_stub.go` — exported test setters: `SetUpstreamPinForTest` / `ClearUpstreamPinForTest`
- `cmd/sworn/baton.go` — `--upstream` / `--tag` / `--repo` flags wiring; `cmdBatonVendor` calls `FetchUpstream` + `Vendor` + `WriteUpstreamPin`
- `cmd/sworn/baton_test.go` — command-level integration tests: `TestBatonVendorUpstream_Success`, `TestBatonVendorUpstream_DigestMismatch`, `TestBatonVendorUpstream_LocalBackCompat` (Rule 1 reachability through CLI entry point)
## Acceptance checks

- [ ] With `--upstream`, `internal/baton` fetches `codeload.github.com/<owner>/<repo>/tar.gz/refs/tags/<tag>` via `net/http` and extracts via `compress/gzip` + `archive/tar`, stripping the `<repo>-<ref>/` prefix. Falsifiable: no `os/exec`/git invocation in the fetch path; `go.mod` `require` is unchanged (stdlib only). Verified by `fetch_test.go` against an `httptest.Server`.
- [ ] The fetched content is verified against the recorded commit SHA and content digest; a simulated force-moved tag or tampered tarball returns an error and writes nothing. Verified by `fetch_test.go` mismatch cases.
- [ ] With no `--tag`, vendor resolves the pinned semver tag from the VERSION pin (S49) and never fetches `latest`/HEAD. Falsifiable: test asserts the requested URL carries the pinned tag.
- [ ] Default repo is `github.com/sawy3r/baton`; `--repo`/config override is honoured. Verified by test.
- [ ] Network failure, non-2xx, or missing tag → non-zero exit, no embed write (fail closed). Verified by `fetch_test.go` stub-server error paths.
- [ ] The S48 `Transform` produces an identical embed from a fetched source as from the equivalent local source (network is source-transparent). Verified by transform parity test.
- [ ] `sworn baton vendor` without `--upstream` still vendors from a local dir (back-compat). Verified by existing S48 tests staying green.
- [ ] `go build ./...` and `go vet ./...` pass.

## Required tests

- **Unit**: `internal/baton/fetch_test.go` — `httptest.Server` serving a fixture tarball; SHA/digest match + mismatch; 404 / network-error / bad-gzip; prefix-strip; pinned-tag URL assertion.
- **Integration**: `sworn baton vendor --upstream --repo <test> --tag <t>` driven end-to-end against an `httptest.Server` through `cmd/sworn/baton.go` (Rule 1 — through the command, not just the leaf fetch).
- **Reachability artefact**: `proof.md` transcript — `sworn baton vendor --upstream` against a local `httptest` fixture (and, once `sawy3r/baton` is tagged, a real fetch of the pinned tag) showing fetch → verify → transform → write; plus a tampered-digest run failing closed with a non-zero exit and no file change.
- **E2E gate type**: N/A (CLI; no Playwright).

## Risks

- **GitHub tarball prefix**: archives wrap content in a top-level `<repo>-<ref>/` dir; the extractor must strip it or every mapped path misses. Named, covered by a prefix-strip test.
- **Network non-determinism**: fetch happens only at vendor time (a maintainer action); the embed stays a committed `go:embed`, so normal builds never fetch. Fail-closed on any mismatch keeps re-vendor reproducible.
- **Tag force-move / supply chain**: a moved upstream tag would change bytes — caught by the commit-SHA + digest lock (the whole point of this slice).

## Deferrals allowed?

No — except private-repo auth (PAT), which is explicitly out of scope above and tracked in issue #11, not a mid-implementation carve-out.
