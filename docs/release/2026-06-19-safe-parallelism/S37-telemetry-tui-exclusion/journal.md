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

## Verifier verdicts received

*(None yet.)*
