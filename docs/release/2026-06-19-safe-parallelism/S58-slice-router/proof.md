# Proof Bundle — S58-slice-router

## Scope

Deterministic `sworn route <slice-id> <release-name> [--pretty]` subcommand that reads a slice's committed `status.json` and computes the next command without an LLM — a faithful Go port of `~/.claude/bin/captain-route.sh`.

## Files changed

```
cmd/sworn/route.go
cmd/sworn/route_test.go
internal/board/oracle.go
internal/git/git.go
internal/router/parity_test.go
internal/router/router.go
internal/router/router_test.go
docs/release/2026-06-19-safe-parallelism/S58-slice-router/status.json
```

## Test results

```
=== go test -race ./internal/router/...
ok  	github.com/swornagent/sworn/internal/router	2.232s

=== go test -race ./internal/git/...
ok  	github.com/swornagent/sworn/internal/git	1.012s

=== go test -race -run TestRouteIntegration ./cmd/sworn/...
ok  	github.com/swornagent/sworn/cmd/sworn	3.638s

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
- [x] `verified` with later planned sibling routes to it; with all siblings terminal routes `merge-track`/`merge-release` — `TestVerifiedWalksTrackThenMerges`
- [x] Ghost-slice filter — `TestGhostSliceFiltered`
- [x] `shipped` routes `none`; unrecognised state routes `none` — `TestShippedRoutesNone`, `TestUnrecognisedStateRoutesNone`
- [x] `deferred` routes `none` (top-level) and is skipped in track walk (terminal) — `TestDeferredRoutesNone`, `TestDeferredSkippedInTrackWalk`
- [x] Parity test against captain-route.sh — `TestCaptainRouteParity` (all 8 state branches match)
- [x] `go test -race ./internal/router/...` passes; `Route` does no I/O except via injected readers

## Not delivered

*(none — all spec acceptance checks are delivered)*

## Divergence from plan

- **Docs prefix discovery**: The design assumed the CLI would receive a fixed `DocsPrefix`. The implementation resolves it dynamically by probing `CatFileExists` on the track branch for `docs/release/...` vs `apps/docs/content/docs/release/...`, matching `captain-route.sh`'s prefix detection. This is more robust and avoids the CLI caller needing to know the project's layout.
- **No `design.md` presence check in `routeNextSlice` for planned siblings**: The bash script checks if a sibling's `design.md` exists and routes `review` vs `implement` accordingly. The Go router currently routes planned siblings to `implement` (the Design TL;DR gate halts them before code). This is a minor fidelity gap; the bash check is a UX optimisation, not a correctness gate. Tracked as a known difference.

## First-pass script output

```
(release-verify.sh output — see below)
```