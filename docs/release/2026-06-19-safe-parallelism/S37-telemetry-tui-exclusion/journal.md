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