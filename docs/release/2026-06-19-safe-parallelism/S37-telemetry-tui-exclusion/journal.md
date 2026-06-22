---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S37-telemetry-tui-exclusion`

## 2026-06-21 — planned (replan)

Sliced in (rather than left as issue #7) per Coach — plenty of release remains. Surfaced
while resolving the cmd/sworn/main.go T9/T2 merge conflict: the no-args TUI launch fires
telemetry with cmd="" + session-length duration. Fix lives in internal/telemetry.Fire()
(the meta-command exclusion chokepoint), deliberately NOT in the shared main.go. Track
T12-harness-hardening. Supersedes swornagent/sworn#7.

## Open questions

None.

## Deferrals surfaced

None.

## 2026-07-04 — implemented

Design TL;DR reviewed by Captain (2026-06-23: 1 mechanical pin), Coach approved
(PROCEED). Pin 1 (design_decisions in status.json) addressed inline before
transitioning to in_progress.

Implementation: added `cmd == ""` early-return to `Fire()` immediately after the
existing `cmd == "telemetry"` check — same shape, same synchronous pattern. Two
new tests: `TestFireSkipsEmptyCmd` (negative — transport not hit for empty cmd)
and `TestFireStillFiresRealCmd` (positive guard — `Fire("verify", ...)` still
fires). All 21 tests pass including the unchanged `TestFireTelemetryMetaCommandExcluded`.

No divergences from spec. No cross-slice touchpoints — only
`internal/telemetry/telemetry.go` and `internal/telemetry/telemetry_test.go`
touched. `cmd/sworn/main.go` untouched per spec.

Design decisions (5 Type-2) populated in status.json per Coach ack.
Skeptic panel: skipped — runtime does not support subagent dispatch.

First-pass verify script: 20/22 PASS. 2 FAILs are expected implementer-terminal
artifacts: state=in_progress (now implemente d) and dark-code false positives
(enum values + comments from prior track slices, not S37 code).

## Verifier verdicts received

*(None yet.)*
### 2026-07-04 — PASS

**Actor**: verifier (fresh session `/verify-slice`)

**Verdict**: **PASS** — all six gates satisfied:

1. **User-reachable outcome exists** (`cmd/sworn/main.go:37-49`: `cmd=""` when no args; `telemetry.Fire(cmd, ...)` at line 49).
2. **Planned touchpoints match actual changed files** — `internal/telemetry/telemetry.go` + `internal/telemetry/telemetry_test.go` (docs artefacts are expected noise).
3. **Required tests exist and exercise the integration point** — `TestFireSkipsEmptyCmd` PASS, `TestFireStillFiresRealCmd` PASS, `TestFireTelemetryMetaCommandExcluded` PASS; all exercise `Fire()` directly.
4. **Reachability artefact proves the user path** — `go test ./internal/telemetry/... -v` confirms all 21 tests PASS including the two new tests.
5. **No silent deferrals or placeholder logic** — zero TODO/FIXME/deferred/placeholder/hack/XXX hits in changed files.
6. **Claimed scope matches implemented scope** — all four acceptance checks have verifiable evidence references.

**Next**: `/implement-slice S38-verifier-blocked-violations 2026-06-19-safe-parallelism` (next in T12-harness-hardening).
