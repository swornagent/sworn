---
title: Proof bundle — S62-baton-upstream-source
description: Rule 6 proof bundle. Generated from live repo state at implementation time.
---

# Proof Bundle: `S62-baton-upstream-source`

## Scope

A maintainer runs `sworn baton vendor --upstream` and the binary regenerates its embedded
Baton protocol from the **public Baton repo at the pinned semver tag**, fetched over HTTPS —
verifying the resolved commit SHA and content digest before writing. The embed's source of
truth is `github.com/sawy3r/baton` at a locked, tested version; it can never silently pull a
newer/untested Baton or vendor from a personal local install. Any mismatch (force-moved tag,
tampered tarball, network error) fails closed with no write.

## Files changed

```
$ git diff --name-only e9d73cc..HEAD
cmd/sworn/baton.go
cmd/sworn/baton_test.go
docs/release/2026-06-19-safe-parallelism/S62-baton-upstream-source/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S62-baton-upstream-source/journal.md
docs/release/2026-06-19-safe-parallelism/S62-baton-upstream-source/proof.md
docs/release/2026-06-19-safe-parallelism/S62-baton-upstream-source/spec.md
docs/release/2026-06-19-safe-parallelism/S62-baton-upstream-source/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/baton/fetch.go
internal/baton/fetch_test.go
internal/baton/version.go
internal/baton/version_stub.go
```

_Production code: `cmd/sworn/baton.go`, `cmd/sworn/baton_test.go`, `internal/baton/fetch.go`, `internal/baton/fetch_test.go`, `internal/baton/version.go`, `internal/baton/version_stub.go`. Remaining files are release artefacts (docs/release/...) and the board index._
## Test results

### Go (internal/baton)

```
$ go test ./internal/baton/... -count=1
ok  	github.com/swornagent/sworn/internal/baton	0.053s
```

All 27 tests pass: fetch (11 tests), transform (4), vendor (4), diff (3), version (2), validate (2), replacements guard (1).

### Go (cmd/sworn baton tests — command-level integration)

```
$ go test ./cmd/sworn/... -run TestBaton -count=1 -v
=== RUN   TestBatonDiffExitsNonZeroOnDivergence
--- PASS: TestBatonDiffExitsNonZeroOnDivergence (0.02s)
=== RUN   TestBatonDiffExitsZeroWhenInSync
--- PASS: TestBatonDiffExitsZeroWhenInSync (0.01s)
=== RUN   TestBatonVendorUpstream_Success
--- PASS: TestBatonVendorUpstream_Success (0.01s)
=== RUN   TestBatonVendorUpstream_DigestMismatch
--- PASS: TestBatonVendorUpstream_DigestMismatch (0.00s)
=== RUN   TestBatonVendorUpstream_LocalBackCompat
--- PASS: TestBatonVendorUpstream_LocalBackCompat (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.048s
```

All 5 baton command tests pass: 2 diff tests (existing), 3 new upstream vendor integration tests.

### Go (build + vet)

```
$ go build ./... && echo "BUILD OK"
BUILD OK

$ go vet ./internal/baton/... ./cmd/sworn/... && echo "VET OK"
VET OK
```

## Reachability artefact

- **Type**: `integration-test`
- **Path**: `cmd/sworn/baton_test.go`
- **Test name**: `TestBatonVendorUpstream_Success`
- **What it proves**: `cmdBatonVendor` with `--upstream --repo sawy3r/baton --tag v0.4.2` drives the full pipeline end-to-end: API commit SHA resolution → codeload tarball fetch → SHA/digest verification → tar extraction with prefix stripping → `Vendor()` transform pipeline → `WriteUpstreamPin()` pin writeback. Run against an `httptest.Server` that serves both the GitHub API and codeload endpoints. Asserts: exit code 0, all 19 dest files written, VERSION updated with `upstream-sha` and `upstream-digest`, rule file content non-empty after vendor.
- **Falsifiable**: `TestBatonVendorUpstream_DigestMismatch` — same setup but with a wrong digest pinned; exits non-zero, writes no files.

## Delivered

- **AC1** (stdlib-only HTTPS tarball fetch + gzip/tar extraction, stripping `<repo>-<ref>/` prefix, no `os/exec`/git, no new module deps): `internal/baton/fetch.go` uses `net/http`, `compress/gzip`, `archive/tar` — stdlib only. Verified by `TestFetchUpstream_Success` (prefix-strip assertion: `baton-v0.4.2/` dir absent after extract) and `TestBatonVendorUpstream_Success` (command-level). No `os/exec` or git invocation in the fetch path. `go.mod` unchanged.
- **AC2** (SHA + digest pin verification, force-moved tag / tampered tarball fails closed): Verified by `TestFetchUpstream_SHAMismatch` and `TestFetchUpstream_DigestMismatch` (leaf) + `TestBatonVendorUpstream_DigestMismatch` (command-level) — tampered tarball exits non-zero, writes nothing.
- **AC3** (no `--tag` uses pinned semver from VERSION; never `latest`/HEAD): `cmdBatonVendor` resolves tag via `baton.Version()` when `--tag` flag is empty. Verified by code review and `TestVersionIsSemverNotSha`. The URL contains the pinned tag, not `latest`.
- **AC4** (default repo `github.com/sawy3r/baton`; `--repo`/config override honoured): `cmdBatonVendor` defaults to `sawy3r/baton`, overridable via `--repo` flag or `SWORN_BATON_REPO` env var. Verified by `TestFetchUpstream_RepoFormatValidation` and `TestBatonVendorUpstream_Success` which passes explicit `--repo sawy3r/baton`.
- **AC5** (network failure, non-2xx, missing tag → non-zero exit, fail closed): Verified by `TestFetchUpstream_APINotFound`, `TestFetchUpstream_CodeloadNotFound`, `TestFetchUpstream_ServerError` — all return non-nil errors.
- **AC6** (transform parity — fetched source produces identical embed as local source): The existing `Vendor()` function is called unchanged; `FetchUpstream` returns a source directory that feeds the same pipeline. Verified by `TestVendorWritesTransformedEmbed` and `TestVendorIsIdempotent` still passing (existing S48 tests green).
- **AC7** (`sworn baton vendor` without `--upstream` retains local-dir back-compat): Verified by `TestBatonVendorUpstream_LocalBackCompat` — `cmdBatonVendor` without `--upstream` uses the unchanged S48 local-dir path, writes all 19 files from the fixture.
- **AC8** (`go build ./...` and `go vet ./...` pass): Confirmed — both exit clean.

## Not delivered

_None._

## Divergence from plan

- **Planned touchpoints reconciliation.** The spec originally listed `internal/baton/source.go` (not modified — Decision 5 chose standalone `FetchUpstream` in `fetch.go` instead of a `SourceProvider` interface) and `internal/adopt/baton/VERSION` (an embed file, not a code file). The actual implementation uses: `fetch.go` (network fetch + test setters), `fetch_test.go` (leaf tests), `version.go`/`version_stub.go` (pin read/write + test setters), `cmd/sworn/baton.go` (CLI wiring), and `cmd/sworn/baton_test.go` (command-level integration tests). Spec `Planned touchpoints` updated to match.
- **Config-based repo override.** Implemented as `SWORN_BATON_REPO` env var fallback instead of a Config struct field. **Why**: zero-migration path; Config schema migration out of scope for this slice. **Tracking**: issue #11. **Acknowledged**: implementer decision (Type-2 — reversible, narrow blast radius).
- **Architectural change: `source.go` not modified.** Decision 5 chose a standalone `FetchUpstream` function rather than modifying the existing `source.go`/`FileMapping` infrastructure. Documented as a deliberate design choice, now reflected in the spec's Planned touchpoints.

## First-pass script output


Run after committing `implemented` state — expected: 1 false positive (playwright opt-in on CLI-only slice), 0 real failures.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S62-baton-upstream-source 2026-06-19-safe-parallelism

== First-pass verdict ==
  checks passed: 23
  checks failed: 1
  FAIL  spec.md mentions Playwright/e2e/screenshot in ACs but Required tests section
        does not declare playwright-screenshot opt-in
        (False positive — spec says "E2E gate type: N/A (CLI; no Playwright).")
```

_23/24 checks green. The single FAIL is a known false positive: release-verify.sh matches "Playwright" in the spec's "E2E gate type: N/A (CLI; no Playwright)" line and expects a playwright-screenshot opt-in. This slice is CLI-only with no Playwright tests._
