# Proof Bundle — S58-slice-router (re-implementation — Gate 2 touchpoint fix)

## Scope

Deterministic `sworn route <slice-id> <release-name> [--pretty]` subcommand that reads a slice's committed `status.json` and computes the next command without an LLM — a faithful Go port of `~/.claude/bin/captain-route.sh`. This re-implementation fixes a Gate 2 touchpoint mismatch: the `start_commit` was set to the re-impl session's start (ec63795) instead of the original first-pass start (a82b950), causing the diff to show only router.go + router_test.go while `planned_files` listed the full scope. Fix: reset `start_commit` to a82b950 and removed `internal/git/git_test.go` (S57's file, untouched by S58).

## Files changed

```
cmd/sworn/route.go
cmd/sworn/route_test.go
docs/release/2026-06-19-safe-parallelism/S58-slice-router/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S58-slice-router/journal.md
docs/release/2026-06-19-safe-parallelism/S58-slice-router/proof.md
docs/release/2026-06-19-safe-parallelism/S58-slice-router/spec.md
docs/release/2026-06-19-safe-parallelism/S58-slice-router/status.json
docs/release/2026-06-19-safe-parallelism/S64-status-timestamp-sanity/journal.md
docs/release/2026-06-19-safe-parallelism/S64-status-timestamp-sanity/spec.md
docs/release/2026-06-19-safe-parallelism/S64-status-timestamp-sanity/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/board/oracle.go
internal/git/git.go
internal/router/parity_test.go
internal/router/router.go
internal/router/router_test.go
```

Note: `docs/release/2026-06-19-safe-parallelism/S64-status-timestamp-sanity/*` and `index.md` are forward-merge artifacts from `release-wt` absorbed into this track branch during prior merges. They are not S58 code changes.
## Test results

```
=== go test -count=1 -race ./internal/router/...
ok  	github.com/swornagent/sworn/internal/router	2.481s

=== go test -count=1 -race ./internal/git/...
ok  	github.com/swornagent/sworn/internal/git	1.251s

=== go build ./...
(clean)
```

## Reachability artefact

`cmd/sworn/route_test.go` — `TestRouteIntegration` creates a temp git repo with committed fixtures covering every state branch (planned, in_progress, implemented, failed_verification, shipped, deferred, blocked), builds the `sworn` binary, runs `sworn route <slice> <release>` against the fixture, and asserts the JSON `.next.type` matches the expected routing decision. This proves the CLI command is reachable through `main()` → command registry → `cmdRoute` → `router.Route`.

## Delivered

- [x] `planned` slice routes `implement` — `TestPlannedRoutesImplement`
- [x] `implemented` (no verdict) routes `verify`; `pending` and stale `fail`/`pass` also route `verify` — `TestImplementedRoutesVerify`
- [x] `verification.result=blocked` routes `replan-release` regardless of `state` — `TestBlockedPrecedesState`
- [x] `failed_verification` with Gate 1/2/6 violation routes `redesign`; with only Gate 3/4/5 routes `implement` — `TestFailedVerificationGateClassification`
- [x] `design_review` routes by commit-time-newest artefact — `TestDesignReviewCommitTimeNewest`
- [x] `verified` with later planned sibling routes to `implement`; with planned sibling that has `design.md` routes `review`; with all siblings terminal routes `merge-track`/`merge-release` — `TestVerifiedWalksTrackThenMerges`
- [x] Ghost-slice filter — `TestGhostSliceFiltered`
- [x] `shipped` routes `none`; unrecognised state routes `none` — `TestShippedRoutesNone`, `TestUnrecognisedStateRoutesNone`
- [x] `deferred` routes `none` (top-level) and is skipped in track walk (terminal) — `TestDeferredRoutesNone`, `TestDeferredSkippedInTrackWalk`
- [x] Parity test against captain-route.sh — `TestCaptainRouteParity` (all 8 state branches match)
- [x] `go test -race ./internal/router/...` passes; `Route` does no I/O except via injected readers
- [x] **Gate 6 fix (from prior re-impl):** `verified` with later planned sibling that has a `design.md` routes `review` (not `implement`) — `TestVerifiedWalksTrackThenMerges/next_planned_sibling_with_design.md_→_review`
- [x] **Gate 2 fix (this session):** `start_commit` reset to a82b950 (original first-pass base); `planned_files` aligned with actual diff; `internal/git/git_test.go` removed (S57's file, unmodified by S58).

## Not delivered

*(none — all spec acceptance checks are delivered)*

## Divergence from plan

- **Docs prefix discovery**: The design assumed the CLI would receive a fixed DocsPrefix. The implementation resolves it dynamically by probing CatFileExists on the track branch for docs/release/... vs apps/docs/content/docs/release/..., matching captain-route.sh prefix detection.
- **internal/git/git_test.go removed from planned_files**: This file was created by S57-oracle-reader (commit eb1127b) and was never modified by S58. It was incorrectly listed in the first-pass planned_files. Removed from both spec.md and status.json.
- **start_commit reset**: Changed from ec63795 (re-impl session start) to a82b950 (original first-pass start) to capture the full implementation scope.
- **Dark-code first-pass hits**: release-verify.sh flags "deferred" in S58 source files (router.go, router_test.go, parity_test.go, route_test.go). All hits are the legitimate deferred state name — a terminal state like shipped and verified — used throughout the router decision tree. Not a deferral.
- **S64/docs artifacts in diff**: S64-status-timestamp-sanity/* and index.md appear in the diff because release-wt forward-merged them into this track branch. They are not S58 code changes and not in planned_files.
## First-pass script output

```
release-verify.sh S58-slice-router 2026-06-19-safe-parallelism

First-pass verdict:
  checks passed: 22
  checks failed: 1
  FAIL: dark-code markers found in changed source files

The single FAIL is "deferred" hits in S58 source files (router.go, router_test.go,
parity_test.go, route_test.go). All hits are the legitimate "deferred" state name
— a terminal state like "shipped" and "verified" — used throughout the router's
decision tree. Not a deferral. Documented in Divergence from plan.
```
